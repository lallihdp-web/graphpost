package analytics

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// QueryRouter routes queries between PostgreSQL and DuckDB analytics
type QueryRouter struct {
	store    *MaterializedStore
	refresh  *RefreshManager
	duckdb   *DuckDB
	config   RouterConfig

	// Query statistics
	stats RouterStats
	mu    sync.RWMutex
}

// RouterConfig holds query router configuration
type RouterConfig struct {
	// Enabled enables analytics routing
	Enabled bool

	// PreferMaterialized prefers materialized aggregates over live queries
	PreferMaterialized bool

	// FallbackToLive falls back to PostgreSQL if materialized data is stale
	FallbackToLive bool

	// StalenessThreshold defines when materialized data is considered stale
	StalenessThreshold time.Duration

	// AggregateQueryPatterns are patterns that should be routed to analytics
	AggregateQueryPatterns []string
}

// RouterStats holds query routing statistics
type RouterStats struct {
	TotalQueries        int64
	MaterializedHits    int64
	MaterializedMisses  int64
	LiveFallbacks       int64
	AnalyticsQueries    int64
	PostgresQueries     int64
	AvgMaterializedTime time.Duration
	AvgLiveTime         time.Duration
}

// NewQueryRouter creates a new query router
func NewQueryRouter(store *MaterializedStore, refresh *RefreshManager, duckdb *DuckDB, config RouterConfig) *QueryRouter {
	if config.StalenessThreshold == 0 {
		config.StalenessThreshold = 5 * time.Minute
	}

	return &QueryRouter{
		store:   store,
		refresh: refresh,
		duckdb:  duckdb,
		config:  config,
	}
}

// QueryResult represents the result of a routed query
type QueryResult struct {
	Data          interface{}
	Source        string // "materialized", "live", "duckdb"
	Duration      time.Duration
	IsCached      bool
	AggregateID   string
	GroupKey      string
	LastRefreshed time.Time
}

// RouteAggregateQuery routes an aggregate query to the best data source
func (r *QueryRouter) RouteAggregateQuery(ctx context.Context, req AggregateRequest) (*QueryResult, error) {
	start := time.Now()

	r.mu.Lock()
	r.stats.TotalQueries++
	r.mu.Unlock()

	// Check if we have a matching materialized aggregate
	aggregate := r.findMatchingAggregate(req)

	if aggregate != nil && aggregate.IsEnabled {
		// Try to get from materialized store
		result, err := r.getFromMaterialized(ctx, aggregate, req)
		if err == nil && result != nil {
			r.mu.Lock()
			r.stats.MaterializedHits++
			r.stats.AnalyticsQueries++
			r.mu.Unlock()

			result.Duration = time.Since(start)
			return result, nil
		}

		r.mu.Lock()
		r.stats.MaterializedMisses++
		r.mu.Unlock()

		// Try lazy refresh if configured
		if aggregate.RefreshStrategy == RefreshLazy {
			lazyResult, err := r.refresh.GetOrComputeLazy(ctx, aggregate.ID, BuildGroupKey(req.GroupValues))
			if err == nil && lazyResult != nil {
				r.mu.Lock()
				r.stats.AnalyticsQueries++
				r.mu.Unlock()

				return &QueryResult{
					Data:          lazyResult.Value,
					Source:        "lazy",
					Duration:      time.Since(start),
					IsCached:      false,
					AggregateID:   aggregate.ID,
					LastRefreshed: lazyResult.ComputedAt,
				}, nil
			}
		}
	}

	// Fallback to live query if enabled
	if r.config.FallbackToLive {
		r.mu.Lock()
		r.stats.LiveFallbacks++
		r.stats.PostgresQueries++
		r.mu.Unlock()

		return &QueryResult{
			Data:     nil, // Caller should execute live query
			Source:   "live",
			Duration: time.Since(start),
			IsCached: false,
		}, nil
	}

	return nil, fmt.Errorf("no materialized aggregate found and live fallback disabled")
}

// AggregateRequest represents a request for aggregate data
type AggregateRequest struct {
	Table         string
	AggregateType AggregateType
	Column        string
	GroupBy       []string
	GroupValues   map[string]interface{}
	Filter        string
}

// findMatchingAggregate finds a materialized aggregate that matches the request
func (r *QueryRouter) findMatchingAggregate(req AggregateRequest) *AggregateDefinition {
	aggregates := r.store.GetAggregatesForTable(req.Table)

	for _, agg := range aggregates {
		if agg.AggregateType != req.AggregateType {
			continue
		}

		if req.Column != "" && agg.ColumnName != req.Column {
			continue
		}

		// Check if group by columns match
		if !stringSliceEqual(agg.GroupByColumns, req.GroupBy) {
			continue
		}

		// If we have a filter, check if it matches
		if req.Filter != "" && agg.FilterCondition != req.Filter {
			continue
		}

		return agg
	}

	return nil
}

// getFromMaterialized retrieves data from materialized aggregates
func (r *QueryRouter) getFromMaterialized(ctx context.Context, agg *AggregateDefinition, req AggregateRequest) (*QueryResult, error) {
	groupKey := BuildGroupKey(req.GroupValues)

	value, err := r.store.GetAggregateValue(ctx, agg.ID, groupKey)
	if err != nil {
		return nil, err
	}

	if value == nil {
		return nil, fmt.Errorf("no value found for group key: %s", groupKey)
	}

	// Check staleness
	if r.config.StalenessThreshold > 0 {
		age := time.Since(value.ComputedAt)
		if age > r.config.StalenessThreshold {
			return nil, fmt.Errorf("materialized data is stale (age: %v)", age)
		}
	}

	return &QueryResult{
		Data:          value.Value,
		Source:        "materialized",
		IsCached:      true,
		AggregateID:   agg.ID,
		GroupKey:      groupKey,
		LastRefreshed: value.ComputedAt,
	}, nil
}

// GetAllAggregateValues retrieves all values for an aggregate type on a table
func (r *QueryRouter) GetAllAggregateValues(ctx context.Context, table string, aggType AggregateType) ([]*AggregateValue, error) {
	aggregates := r.store.GetAggregatesForTable(table)

	for _, agg := range aggregates {
		if agg.AggregateType == aggType && agg.IsEnabled {
			return r.store.GetAllAggregateValues(ctx, agg.ID)
		}
	}

	return nil, fmt.Errorf("no matching aggregate found")
}

// ExecuteDuckDBQuery executes a direct analytical query on DuckDB
func (r *QueryRouter) ExecuteDuckDBQuery(ctx context.Context, query string) (*QueryResult, error) {
	start := time.Now()

	r.mu.Lock()
	r.stats.TotalQueries++
	r.stats.AnalyticsQueries++
	r.mu.Unlock()

	rows, err := r.duckdb.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect results
	var results []map[string]interface{}
	cols, _ := rows.Columns()

	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return &QueryResult{
		Data:     results,
		Source:   "duckdb",
		Duration: time.Since(start),
		IsCached: false,
	}, nil
}

// ShouldRouteToAnalytics determines if a query should be routed to analytics
func (r *QueryRouter) ShouldRouteToAnalytics(queryType string, tableName string, hasAggregates bool) bool {
	if !r.config.Enabled {
		return false
	}

	// Check if table has materialized aggregates
	aggregates := r.store.GetAggregatesForTable(tableName)
	if len(aggregates) == 0 {
		return false
	}

	// Route aggregate queries to analytics
	if hasAggregates {
		return true
	}

	// Check against configured patterns
	for _, pattern := range r.config.AggregateQueryPatterns {
		if strings.Contains(strings.ToLower(queryType), pattern) {
			return true
		}
	}

	return false
}

// CreateMaterializedAggregate creates a new materialized aggregate through the router
func (r *QueryRouter) CreateMaterializedAggregate(ctx context.Context, def *AggregateDefinition) error {
	if err := r.store.CreateAggregate(ctx, def); err != nil {
		return err
	}

	// Optionally trigger initial refresh
	if def.RefreshStrategy != RefreshLazy {
		go func() {
			refreshCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := r.refresh.RefreshAggregate(refreshCtx, def.ID); err != nil {
				log.Printf("[Analytics] Initial refresh failed for %s: %v", def.Name, err)
			}
		}()
	}

	return nil
}

// DeleteMaterializedAggregate removes a materialized aggregate
func (r *QueryRouter) DeleteMaterializedAggregate(ctx context.Context, aggregateID string) error {
	return r.store.DeleteAggregate(ctx, aggregateID)
}

// RefreshAggregate triggers an on-demand refresh
func (r *QueryRouter) RefreshAggregate(ctx context.Context, aggregateID string) error {
	return r.refresh.RefreshAggregate(ctx, aggregateID)
}

// RefreshAll triggers refresh for all aggregates
func (r *QueryRouter) RefreshAll(ctx context.Context) error {
	return r.refresh.RefreshAll(ctx)
}

// RefreshTable refreshes all aggregates for a specific table
func (r *QueryRouter) RefreshTable(ctx context.Context, tableName string) error {
	return r.refresh.RefreshByTable(ctx, tableName)
}

// ListAggregates returns all aggregate definitions
func (r *QueryRouter) ListAggregates() []*AggregateDefinition {
	return r.store.ListAggregates()
}

// GetAggregate returns a specific aggregate definition
func (r *QueryRouter) GetAggregate(id string) (*AggregateDefinition, bool) {
	return r.store.GetAggregate(id)
}

// GetRefreshLogs returns refresh logs for an aggregate
func (r *QueryRouter) GetRefreshLogs(ctx context.Context, aggregateID string, limit int) ([]*AggregateRefreshLog, error) {
	return r.store.GetRefreshLogs(ctx, aggregateID, limit)
}

// Stats returns routing statistics
func (r *QueryRouter) Stats() RouterStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stats
}

// ResetStats resets routing statistics
func (r *QueryRouter) ResetStats() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stats = RouterStats{}
}

// NotifyCDC notifies the router of a table change (for CDC)
func (r *QueryRouter) NotifyCDC(tableName, operation string) {
	r.store.NotifyCDC(tableName, operation)
}

// Helper function to compare string slices
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// AnalyticsEngine is the main entry point for the analytics subsystem
type AnalyticsEngine struct {
	DuckDB   *DuckDB
	Store    *MaterializedStore
	Refresh  *RefreshManager
	Router   *QueryRouter
}

// NewAnalyticsEngine creates a new analytics engine with all components
func NewAnalyticsEngine(duckdbConfig DuckDBConfig, refreshConfig RefreshConfig, routerConfig RouterConfig, pgPool interface{}) (*AnalyticsEngine, error) {
	// Import pgxpool type
	pool, ok := pgPool.(*pgxpool.Pool)
	if !ok {
		return nil, fmt.Errorf("invalid PostgreSQL pool type")
	}

	// Initialize DuckDB
	duckdb, err := NewDuckDB(duckdbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DuckDB: %w", err)
	}

	// Initialize materialized store
	store, err := NewMaterializedStore(duckdb)
	if err != nil {
		duckdb.Close()
		return nil, fmt.Errorf("failed to initialize materialized store: %w", err)
	}

	// Initialize refresh manager
	refresh := NewRefreshManager(store, duckdb, pool, refreshConfig)

	// Initialize router
	router := NewQueryRouter(store, refresh, duckdb, routerConfig)

	engine := &AnalyticsEngine{
		DuckDB:  duckdb,
		Store:   store,
		Refresh: refresh,
		Router:  router,
	}

	return engine, nil
}

// Start starts the analytics engine background workers
func (e *AnalyticsEngine) Start() error {
	return e.Refresh.Start()
}

// Stop stops the analytics engine
func (e *AnalyticsEngine) Stop() {
	e.Refresh.Stop()
	e.DuckDB.Close()
}

// Import pgxpool for type assertion
import "github.com/jackc/pgx/v5/pgxpool"
