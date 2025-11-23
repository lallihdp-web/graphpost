package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RefreshManager manages aggregate refresh operations
type RefreshManager struct {
	store      *MaterializedStore
	duckdb     *DuckDB
	pgPool     *pgxpool.Pool
	config     RefreshConfig

	// Scheduler state
	schedulerRunning bool
	schedulerStop    chan struct{}
	schedulerWg      sync.WaitGroup

	// CDC listener state
	cdcRunning bool
	cdcStop    chan struct{}
	cdcWg      sync.WaitGroup

	mu sync.Mutex
}

// RefreshConfig holds refresh manager configuration
type RefreshConfig struct {
	// SchedulerEnabled enables the scheduled refresh worker
	SchedulerEnabled bool

	// SchedulerInterval is how often to check for pending refreshes
	SchedulerInterval time.Duration

	// CDCEnabled enables CDC-based refresh
	CDCEnabled bool

	// CDCPollInterval is how often to poll for CDC changes
	CDCPollInterval time.Duration

	// LazyTTL is the TTL for lazy-refreshed aggregates
	LazyTTL time.Duration

	// MaxConcurrentRefreshes limits concurrent refresh operations
	MaxConcurrentRefreshes int

	// RefreshTimeout is the timeout for a single refresh operation
	RefreshTimeout time.Duration

	// PostgresConnectionString for querying source data
	PostgresConnectionString string
}

// NewRefreshManager creates a new refresh manager
func NewRefreshManager(store *MaterializedStore, duckdb *DuckDB, pgPool *pgxpool.Pool, config RefreshConfig) *RefreshManager {
	if config.SchedulerInterval == 0 {
		config.SchedulerInterval = 30 * time.Second
	}
	if config.CDCPollInterval == 0 {
		config.CDCPollInterval = 5 * time.Second
	}
	if config.LazyTTL == 0 {
		config.LazyTTL = 5 * time.Minute
	}
	if config.MaxConcurrentRefreshes == 0 {
		config.MaxConcurrentRefreshes = 4
	}
	if config.RefreshTimeout == 0 {
		config.RefreshTimeout = 5 * time.Minute
	}

	return &RefreshManager{
		store:  store,
		duckdb: duckdb,
		pgPool: pgPool,
		config: config,
	}
}

// Start starts the refresh manager background workers
func (rm *RefreshManager) Start() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.config.SchedulerEnabled {
		rm.startScheduler()
	}

	if rm.config.CDCEnabled {
		rm.startCDCListener()
	}

	return nil
}

// Stop stops all background workers
func (rm *RefreshManager) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.schedulerRunning {
		close(rm.schedulerStop)
		rm.schedulerWg.Wait()
		rm.schedulerRunning = false
	}

	if rm.cdcRunning {
		close(rm.cdcStop)
		rm.cdcWg.Wait()
		rm.cdcRunning = false
	}
}

// startScheduler starts the scheduled refresh worker
func (rm *RefreshManager) startScheduler() {
	if rm.schedulerRunning {
		return
	}

	rm.schedulerStop = make(chan struct{})
	rm.schedulerRunning = true

	rm.schedulerWg.Add(1)
	go func() {
		defer rm.schedulerWg.Done()
		ticker := time.NewTicker(rm.config.SchedulerInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rm.runScheduledRefreshes()
			case <-rm.schedulerStop:
				return
			}
		}
	}()

	log.Printf("[Analytics] Scheduled refresh worker started (interval: %v)", rm.config.SchedulerInterval)
}

// runScheduledRefreshes processes pending scheduled refreshes
func (rm *RefreshManager) runScheduledRefreshes() {
	pending := rm.store.GetPendingScheduledRefreshes()
	if len(pending) == 0 {
		return
	}

	// Use a semaphore to limit concurrent refreshes
	sem := make(chan struct{}, rm.config.MaxConcurrentRefreshes)
	var wg sync.WaitGroup

	for _, def := range pending {
		sem <- struct{}{} // Acquire
		wg.Add(1)

		go func(d *AggregateDefinition) {
			defer wg.Done()
			defer func() { <-sem }() // Release

			ctx, cancel := context.WithTimeout(context.Background(), rm.config.RefreshTimeout)
			defer cancel()

			if err := rm.RefreshAggregate(ctx, d.ID); err != nil {
				log.Printf("[Analytics] Scheduled refresh failed for %s: %v", d.Name, err)
			} else {
				log.Printf("[Analytics] Scheduled refresh completed for %s", d.Name)
			}
		}(def)
	}

	wg.Wait()
}

// startCDCListener starts the CDC-based refresh listener
func (rm *RefreshManager) startCDCListener() {
	if rm.cdcRunning {
		return
	}

	rm.cdcStop = make(chan struct{})
	rm.cdcRunning = true

	// Register callback for CDC notifications
	rm.store.RegisterCDCCallback(func(tableName, operation string) {
		rm.handleCDCEvent(tableName, operation)
	})

	rm.cdcWg.Add(1)
	go func() {
		defer rm.cdcWg.Done()
		ticker := time.NewTicker(rm.config.CDCPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rm.pollCDCChanges()
			case <-rm.cdcStop:
				return
			}
		}
	}()

	log.Printf("[Analytics] CDC listener started (poll interval: %v)", rm.config.CDCPollInterval)
}

// pollCDCChanges polls PostgreSQL for changes (using pg_stat_user_tables)
func (rm *RefreshManager) pollCDCChanges() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query for recent table modifications
	// This is a simplified CDC - production should use LISTEN/NOTIFY or logical replication
	rows, err := rm.pgPool.Query(ctx, `
		SELECT relname, n_tup_ins, n_tup_upd, n_tup_del
		FROM pg_stat_user_tables
		WHERE schemaname = 'public'
	`)
	if err != nil {
		log.Printf("[Analytics] CDC poll error: %v", err)
		return
	}
	defer rows.Close()

	// Track changes and trigger refreshes as needed
	for rows.Next() {
		var tableName string
		var inserts, updates, deletes int64
		if err := rows.Scan(&tableName, &inserts, &updates, &deletes); err != nil {
			continue
		}

		// Check if any aggregates depend on this table
		aggregates := rm.store.GetAggregatesForTable(tableName)
		for _, agg := range aggregates {
			if agg.RefreshStrategy == RefreshCDC && agg.IsEnabled {
				// Trigger refresh for CDC aggregates
				go func(id string) {
					ctx, cancel := context.WithTimeout(context.Background(), rm.config.RefreshTimeout)
					defer cancel()
					if err := rm.RefreshAggregate(ctx, id); err != nil {
						log.Printf("[Analytics] CDC refresh failed for %s: %v", id, err)
					}
				}(agg.ID)
			}
		}
	}
}

// handleCDCEvent handles a CDC event notification
func (rm *RefreshManager) handleCDCEvent(tableName, operation string) {
	aggregates := rm.store.GetAggregatesForTable(tableName)
	for _, agg := range aggregates {
		if agg.RefreshStrategy == RefreshCDC && agg.IsEnabled {
			go func(id string) {
				ctx, cancel := context.WithTimeout(context.Background(), rm.config.RefreshTimeout)
				defer cancel()
				if err := rm.RefreshAggregate(ctx, id); err != nil {
					log.Printf("[Analytics] CDC refresh failed for %s: %v", id, err)
				}
			}(agg.ID)
		}
	}
}

// RefreshAggregate refreshes a single aggregate (On-Demand strategy)
func (rm *RefreshManager) RefreshAggregate(ctx context.Context, aggregateID string) error {
	def, ok := rm.store.GetAggregate(aggregateID)
	if !ok {
		return fmt.Errorf("aggregate not found: %s", aggregateID)
	}

	startTime := time.Now()
	refreshLog := &AggregateRefreshLog{
		AggregateID: aggregateID,
		StartedAt:   startTime,
		Status:      "running",
	}

	// Log start of refresh
	if err := rm.store.LogRefresh(ctx, refreshLog); err != nil {
		log.Printf("[Analytics] Failed to log refresh start: %v", err)
	}

	// Build and execute the aggregation query
	rowsProcessed, err := rm.executeAggregation(ctx, def)

	// Update refresh log
	refreshLog.CompletedAt = time.Now()
	refreshLog.DurationMs = time.Since(startTime).Milliseconds()
	refreshLog.RowsProcessed = rowsProcessed

	if err != nil {
		refreshLog.Status = "failed"
		refreshLog.ErrorMessage = err.Error()
		rm.store.LogRefresh(ctx, refreshLog)
		return err
	}

	refreshLog.Status = "completed"
	rm.store.LogRefresh(ctx, refreshLog)

	// Update last refreshed timestamp
	return rm.store.UpdateLastRefreshed(ctx, aggregateID, time.Now())
}

// executeAggregation executes the aggregation query and stores results
func (rm *RefreshManager) executeAggregation(ctx context.Context, def *AggregateDefinition) (int64, error) {
	// Clear existing values
	if err := rm.store.ClearAggregateValues(ctx, def.ID); err != nil {
		return 0, fmt.Errorf("failed to clear old values: %w", err)
	}

	// Build the aggregation query
	query := rm.buildAggregationQuery(def)

	// Execute against PostgreSQL
	rows, err := rm.pgPool.Query(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("aggregation query failed: %w", err)
	}
	defer rows.Close()

	var rowsProcessed int64

	// Get column descriptions
	fieldDescs := rows.FieldDescriptions()
	numGroupCols := len(def.GroupByColumns)

	for rows.Next() {
		// Scan values dynamically
		values := make([]interface{}, len(fieldDescs))
		valuePtrs := make([]interface{}, len(fieldDescs))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return rowsProcessed, fmt.Errorf("scan failed: %w", err)
		}

		// Build group key from group-by values
		groupValues := make(map[string]interface{})
		for i := 0; i < numGroupCols && i < len(def.GroupByColumns); i++ {
			groupValues[def.GroupByColumns[i]] = values[i]
		}
		groupKey := BuildGroupKey(groupValues)

		// Create aggregate value
		aggValue := &AggregateValue{
			AggregateID: def.ID,
			GroupKey:    groupKey,
			GroupValues: groupValues,
		}

		// Extract aggregate value(s)
		aggIdx := numGroupCols
		switch def.AggregateType {
		case AggregateCount, AggregateCountDistinct:
			if v, ok := values[aggIdx].(int64); ok {
				aggValue.Count = v
				aggValue.Value = float64(v)
			}
		case AggregateSum:
			aggValue.Sum = toFloat64(values[aggIdx])
			aggValue.Value = aggValue.Sum
		case AggregateAvg:
			aggValue.Avg = toFloat64(values[aggIdx])
			aggValue.Value = aggValue.Avg
		case AggregateMin:
			aggValue.Min = toFloat64(values[aggIdx])
			aggValue.Value = aggValue.Min
		case AggregateMax:
			aggValue.Max = toFloat64(values[aggIdx])
			aggValue.Value = aggValue.Max
		}

		// Store the value
		if err := rm.store.StoreAggregateValue(ctx, aggValue); err != nil {
			return rowsProcessed, fmt.Errorf("failed to store value: %w", err)
		}

		rowsProcessed++
	}

	return rowsProcessed, rows.Err()
}

// buildAggregationQuery builds the SQL aggregation query
func (rm *RefreshManager) buildAggregationQuery(def *AggregateDefinition) string {
	var sb strings.Builder

	sb.WriteString("SELECT ")

	// Group by columns
	for i, col := range def.GroupByColumns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(col)
	}

	// Add comma if we have group by columns
	if len(def.GroupByColumns) > 0 {
		sb.WriteString(", ")
	}

	// Aggregate function
	switch def.AggregateType {
	case AggregateCount:
		sb.WriteString("COUNT(*)")
	case AggregateCountDistinct:
		sb.WriteString(fmt.Sprintf("COUNT(DISTINCT %s)", def.ColumnName))
	case AggregateSum:
		sb.WriteString(fmt.Sprintf("SUM(%s)", def.ColumnName))
	case AggregateAvg:
		sb.WriteString(fmt.Sprintf("AVG(%s)", def.ColumnName))
	case AggregateMin:
		sb.WriteString(fmt.Sprintf("MIN(%s)", def.ColumnName))
	case AggregateMax:
		sb.WriteString(fmt.Sprintf("MAX(%s)", def.ColumnName))
	}

	sb.WriteString(fmt.Sprintf(" FROM %s", def.SourceTable))

	// WHERE clause
	if def.FilterCondition != "" {
		sb.WriteString(fmt.Sprintf(" WHERE %s", def.FilterCondition))
	}

	// GROUP BY clause
	if len(def.GroupByColumns) > 0 {
		sb.WriteString(" GROUP BY ")
		for i, col := range def.GroupByColumns {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(col)
		}
	}

	return sb.String()
}

// GetOrComputeLazy gets a lazy aggregate value, computing if stale or missing
func (rm *RefreshManager) GetOrComputeLazy(ctx context.Context, aggregateID, groupKey string) (*AggregateValue, error) {
	def, ok := rm.store.GetAggregate(aggregateID)
	if !ok {
		return nil, fmt.Errorf("aggregate not found: %s", aggregateID)
	}

	if def.RefreshStrategy != RefreshLazy {
		return nil, fmt.Errorf("aggregate %s is not configured for lazy refresh", aggregateID)
	}

	// Try to get existing value
	value, err := rm.store.GetAggregateValue(ctx, aggregateID, groupKey)
	if err != nil {
		return nil, err
	}

	// Check if value exists and is fresh
	if value != nil {
		age := time.Since(value.ComputedAt)
		if age < rm.config.LazyTTL {
			return value, nil
		}
	}

	// Compute fresh value
	return rm.computeSingleAggregate(ctx, def, groupKey)
}

// computeSingleAggregate computes a single aggregate value for lazy refresh
func (rm *RefreshManager) computeSingleAggregate(ctx context.Context, def *AggregateDefinition, groupKey string) (*AggregateValue, error) {
	// Parse group key to get filter values
	groupValues, err := ParseGroupKey(groupKey)
	if err != nil {
		return nil, err
	}

	// Build query for specific group
	query := rm.buildSingleValueQuery(def, groupValues)

	row := rm.pgPool.QueryRow(ctx, query)

	var result interface{}
	if err := row.Scan(&result); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	aggValue := &AggregateValue{
		AggregateID: def.ID,
		GroupKey:    groupKey,
		GroupValues: groupValues,
	}

	switch def.AggregateType {
	case AggregateCount, AggregateCountDistinct:
		if v, ok := result.(int64); ok {
			aggValue.Count = v
			aggValue.Value = float64(v)
		}
	case AggregateSum:
		aggValue.Sum = toFloat64(result)
		aggValue.Value = aggValue.Sum
	case AggregateAvg:
		aggValue.Avg = toFloat64(result)
		aggValue.Value = aggValue.Avg
	case AggregateMin:
		aggValue.Min = toFloat64(result)
		aggValue.Value = aggValue.Min
	case AggregateMax:
		aggValue.Max = toFloat64(result)
		aggValue.Value = aggValue.Max
	}

	// Store computed value
	if err := rm.store.StoreAggregateValue(ctx, aggValue); err != nil {
		log.Printf("[Analytics] Failed to cache lazy aggregate: %v", err)
	}

	return aggValue, nil
}

// buildSingleValueQuery builds a query for a single aggregate value
func (rm *RefreshManager) buildSingleValueQuery(def *AggregateDefinition, groupValues map[string]interface{}) string {
	var sb strings.Builder

	sb.WriteString("SELECT ")

	// Aggregate function
	switch def.AggregateType {
	case AggregateCount:
		sb.WriteString("COUNT(*)")
	case AggregateCountDistinct:
		sb.WriteString(fmt.Sprintf("COUNT(DISTINCT %s)", def.ColumnName))
	case AggregateSum:
		sb.WriteString(fmt.Sprintf("SUM(%s)", def.ColumnName))
	case AggregateAvg:
		sb.WriteString(fmt.Sprintf("AVG(%s)", def.ColumnName))
	case AggregateMin:
		sb.WriteString(fmt.Sprintf("MIN(%s)", def.ColumnName))
	case AggregateMax:
		sb.WriteString(fmt.Sprintf("MAX(%s)", def.ColumnName))
	}

	sb.WriteString(fmt.Sprintf(" FROM %s", def.SourceTable))

	// Build WHERE clause from group values and filter condition
	var conditions []string

	if def.FilterCondition != "" {
		conditions = append(conditions, def.FilterCondition)
	}

	for col, val := range groupValues {
		conditions = append(conditions, fmt.Sprintf("%s = '%v'", col, val))
	}

	if len(conditions) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conditions, " AND "))
	}

	return sb.String()
}

// RefreshAll refreshes all enabled aggregates
func (rm *RefreshManager) RefreshAll(ctx context.Context) error {
	aggregates := rm.store.ListAggregates()

	var errs []string
	for _, def := range aggregates {
		if !def.IsEnabled {
			continue
		}
		if err := rm.RefreshAggregate(ctx, def.ID); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", def.Name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("some refreshes failed: %s", strings.Join(errs, "; "))
	}

	return nil
}

// RefreshByTable refreshes all aggregates for a specific table
func (rm *RefreshManager) RefreshByTable(ctx context.Context, tableName string) error {
	aggregates := rm.store.GetAggregatesForTable(tableName)

	var errs []string
	for _, def := range aggregates {
		if !def.IsEnabled {
			continue
		}
		if err := rm.RefreshAggregate(ctx, def.ID); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", def.Name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("some refreshes failed: %s", strings.Join(errs, "; "))
	}

	return nil
}

// toFloat64 converts various numeric types to float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case float32:
		return float64(val)
	case float64:
		return val
	case []uint8:
		// Handle numeric/decimal types returned as []byte
		var f float64
		fmt.Sscanf(string(val), "%f", &f)
		return f
	default:
		return 0
	}
}

// Status returns the current status of the refresh manager
func (rm *RefreshManager) Status() RefreshManagerStatus {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	return RefreshManagerStatus{
		SchedulerRunning: rm.schedulerRunning,
		CDCRunning:       rm.cdcRunning,
		Config:           rm.config,
	}
}

// RefreshManagerStatus represents the refresh manager status
type RefreshManagerStatus struct {
	SchedulerRunning bool
	CDCRunning       bool
	Config           RefreshConfig
}
