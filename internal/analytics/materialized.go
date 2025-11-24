package analytics

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// AggregateType represents the type of aggregation
type AggregateType string

const (
	AggregateCount   AggregateType = "count"
	AggregateSum     AggregateType = "sum"
	AggregateAvg     AggregateType = "avg"
	AggregateMin     AggregateType = "min"
	AggregateMax     AggregateType = "max"
	AggregateCountDistinct AggregateType = "count_distinct"
)

// RefreshStrategy represents how aggregates are refreshed
type RefreshStrategy string

const (
	// RefreshScheduled uses cron-based scheduling
	RefreshScheduled RefreshStrategy = "scheduled"

	// RefreshOnDemand refreshes only when explicitly requested
	RefreshOnDemand RefreshStrategy = "on_demand"

	// RefreshCDC uses change data capture for real-time updates
	RefreshCDC RefreshStrategy = "cdc"

	// RefreshLazy computes on cache miss
	RefreshLazy RefreshStrategy = "lazy"
)

// AggregateDefinition defines a materialized aggregate
type AggregateDefinition struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	SourceTable     string          `json:"source_table"`
	AggregateType   AggregateType   `json:"aggregate_type"`
	ColumnName      string          `json:"column_name,omitempty"`
	GroupByColumns  []string        `json:"group_by_columns,omitempty"`
	FilterCondition string          `json:"filter_condition,omitempty"`
	RefreshStrategy RefreshStrategy `json:"refresh_strategy"`
	RefreshInterval time.Duration   `json:"refresh_interval,omitempty"`
	CronSchedule    string          `json:"cron_schedule,omitempty"`
	LastRefreshed   time.Time       `json:"last_refreshed,omitempty"`
	NextRefresh     time.Time       `json:"next_refresh,omitempty"`
	IsEnabled       bool            `json:"is_enabled"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// AggregateValue holds a computed aggregate value
type AggregateValue struct {
	AggregateID string             `json:"aggregate_id"`
	GroupKey    string             `json:"group_key"`
	Value       float64            `json:"value,omitempty"`
	Count       int64              `json:"count,omitempty"`
	Sum         float64            `json:"sum,omitempty"`
	Min         float64            `json:"min,omitempty"`
	Max         float64            `json:"max,omitempty"`
	Avg         float64            `json:"avg,omitempty"`
	ComputedAt  time.Time          `json:"computed_at"`
	GroupValues map[string]interface{} `json:"group_values,omitempty"`
}

// AggregateRefreshLog records refresh operations
type AggregateRefreshLog struct {
	ID            int64     `json:"id"`
	AggregateID   string    `json:"aggregate_id"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	Status        string    `json:"status"`
	RowsProcessed int64     `json:"rows_processed"`
	DurationMs    int64     `json:"duration_ms"`
	ErrorMessage  string    `json:"error_message,omitempty"`
}

// MaterializedStore manages materialized aggregates
type MaterializedStore struct {
	duckdb *DuckDB
	mu     sync.RWMutex

	// In-memory cache of definitions for fast lookup
	definitions map[string]*AggregateDefinition

	// Callbacks for CDC notifications
	cdcCallbacks []func(tableName string, operation string)
}

// NewMaterializedStore creates a new materialized aggregate store
func NewMaterializedStore(duckdb *DuckDB) (*MaterializedStore, error) {
	store := &MaterializedStore{
		duckdb:      duckdb,
		definitions: make(map[string]*AggregateDefinition),
	}

	// Load existing definitions
	if err := store.loadDefinitions(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load aggregate definitions: %w", err)
	}

	return store, nil
}

// loadDefinitions loads all aggregate definitions from DuckDB
func (s *MaterializedStore) loadDefinitions(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.duckdb.Query(ctx, `
		SELECT id, name, source_table, aggregate_type, column_name,
			   group_by_columns, filter_condition, refresh_strategy,
			   refresh_interval_seconds, cron_schedule, last_refreshed,
			   next_refresh, is_enabled, created_at, updated_at
		FROM aggregate_definitions
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var def AggregateDefinition
		var groupByCols, filterCond, cronSchedule sql.NullString
		var lastRefreshed, nextRefresh sql.NullTime
		var refreshIntervalSec sql.NullInt64
		var columnName sql.NullString

		err := rows.Scan(
			&def.ID, &def.Name, &def.SourceTable, &def.AggregateType,
			&columnName, &groupByCols, &filterCond, &def.RefreshStrategy,
			&refreshIntervalSec, &cronSchedule, &lastRefreshed, &nextRefresh,
			&def.IsEnabled, &def.CreatedAt, &def.UpdatedAt,
		)
		if err != nil {
			return err
		}

		if columnName.Valid {
			def.ColumnName = columnName.String
		}
		if groupByCols.Valid && groupByCols.String != "" {
			def.GroupByColumns = strings.Split(groupByCols.String, ",")
		}
		if filterCond.Valid {
			def.FilterCondition = filterCond.String
		}
		if cronSchedule.Valid {
			def.CronSchedule = cronSchedule.String
		}
		if lastRefreshed.Valid {
			def.LastRefreshed = lastRefreshed.Time
		}
		if nextRefresh.Valid {
			def.NextRefresh = nextRefresh.Time
		}
		if refreshIntervalSec.Valid {
			def.RefreshInterval = time.Duration(refreshIntervalSec.Int64) * time.Second
		}

		s.definitions[def.ID] = &def
	}

	return rows.Err()
}

// CreateAggregate creates a new materialized aggregate definition
func (s *MaterializedStore) CreateAggregate(ctx context.Context, def *AggregateDefinition) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID if not provided
	if def.ID == "" {
		def.ID = s.generateID(def)
	}

	def.CreatedAt = time.Now()
	def.UpdatedAt = time.Now()
	def.IsEnabled = true

	// Calculate next refresh time
	if def.RefreshStrategy == RefreshScheduled && def.RefreshInterval > 0 {
		def.NextRefresh = time.Now().Add(def.RefreshInterval)
	}

	groupByCols := strings.Join(def.GroupByColumns, ",")

	_, err := s.duckdb.Exec(ctx, `
		INSERT INTO aggregate_definitions (
			id, name, source_table, aggregate_type, column_name,
			group_by_columns, filter_condition, refresh_strategy,
			refresh_interval_seconds, cron_schedule, next_refresh,
			is_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		def.ID, def.Name, def.SourceTable, def.AggregateType, def.ColumnName,
		groupByCols, def.FilterCondition, def.RefreshStrategy,
		int64(def.RefreshInterval.Seconds()), def.CronSchedule, def.NextRefresh,
		def.IsEnabled, def.CreatedAt, def.UpdatedAt,
	)

	if err != nil {
		return err
	}

	s.definitions[def.ID] = def
	return nil
}

// GetAggregate retrieves an aggregate definition by ID
func (s *MaterializedStore) GetAggregate(id string) (*AggregateDefinition, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	def, ok := s.definitions[id]
	return def, ok
}

// GetAggregateByName retrieves an aggregate definition by name
func (s *MaterializedStore) GetAggregateByName(name string) (*AggregateDefinition, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, def := range s.definitions {
		if def.Name == name {
			return def, true
		}
	}
	return nil, false
}

// ListAggregates returns all aggregate definitions
func (s *MaterializedStore) ListAggregates() []*AggregateDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AggregateDefinition, 0, len(s.definitions))
	for _, def := range s.definitions {
		result = append(result, def)
	}
	return result
}

// GetAggregatesForTable returns aggregates for a specific source table
func (s *MaterializedStore) GetAggregatesForTable(tableName string) []*AggregateDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*AggregateDefinition
	for _, def := range s.definitions {
		if def.SourceTable == tableName {
			result = append(result, def)
		}
	}
	return result
}

// GetAggregateValue retrieves a computed aggregate value
func (s *MaterializedStore) GetAggregateValue(ctx context.Context, aggregateID, groupKey string) (*AggregateValue, error) {
	row := s.duckdb.QueryRow(ctx, `
		SELECT aggregate_id, group_key, value, count, sum, min, max, avg, computed_at
		FROM aggregate_values
		WHERE aggregate_id = ? AND group_key = ?
	`, aggregateID, groupKey)

	var val AggregateValue
	var value, sum, min, max, avg sql.NullFloat64
	var count sql.NullInt64

	err := row.Scan(
		&val.AggregateID, &val.GroupKey, &value, &count,
		&sum, &min, &max, &avg, &val.ComputedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if value.Valid {
		val.Value = value.Float64
	}
	if count.Valid {
		val.Count = count.Int64
	}
	if sum.Valid {
		val.Sum = sum.Float64
	}
	if min.Valid {
		val.Min = min.Float64
	}
	if max.Valid {
		val.Max = max.Float64
	}
	if avg.Valid {
		val.Avg = avg.Float64
	}

	return &val, nil
}

// GetAllAggregateValues retrieves all values for an aggregate
func (s *MaterializedStore) GetAllAggregateValues(ctx context.Context, aggregateID string) ([]*AggregateValue, error) {
	rows, err := s.duckdb.Query(ctx, `
		SELECT aggregate_id, group_key, value, count, sum, min, max, avg, computed_at
		FROM aggregate_values
		WHERE aggregate_id = ?
	`, aggregateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*AggregateValue
	for rows.Next() {
		var val AggregateValue
		var value, sum, min, max, avg sql.NullFloat64
		var count sql.NullInt64

		err := rows.Scan(
			&val.AggregateID, &val.GroupKey, &value, &count,
			&sum, &min, &max, &avg, &val.ComputedAt,
		)
		if err != nil {
			return nil, err
		}

		if value.Valid {
			val.Value = value.Float64
		}
		if count.Valid {
			val.Count = count.Int64
		}
		if sum.Valid {
			val.Sum = sum.Float64
		}
		if min.Valid {
			val.Min = min.Float64
		}
		if max.Valid {
			val.Max = max.Float64
		}
		if avg.Valid {
			val.Avg = avg.Float64
		}

		results = append(results, &val)
	}

	return results, rows.Err()
}

// StoreAggregateValue stores a computed aggregate value
func (s *MaterializedStore) StoreAggregateValue(ctx context.Context, val *AggregateValue) error {
	val.ComputedAt = time.Now()

	_, err := s.duckdb.Exec(ctx, `
		INSERT OR REPLACE INTO aggregate_values (
			aggregate_id, group_key, value, count, sum, min, max, avg, computed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		val.AggregateID, val.GroupKey, val.Value, val.Count,
		val.Sum, val.Min, val.Max, val.Avg, val.ComputedAt,
	)

	return err
}

// UpdateLastRefreshed updates the last refresh time for an aggregate
func (s *MaterializedStore) UpdateLastRefreshed(ctx context.Context, aggregateID string, refreshedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	def, ok := s.definitions[aggregateID]
	if !ok {
		return fmt.Errorf("aggregate not found: %s", aggregateID)
	}

	def.LastRefreshed = refreshedAt
	def.UpdatedAt = time.Now()

	// Calculate next refresh time for scheduled refreshes
	if def.RefreshStrategy == RefreshScheduled && def.RefreshInterval > 0 {
		def.NextRefresh = refreshedAt.Add(def.RefreshInterval)
	}

	_, err := s.duckdb.Exec(ctx, `
		UPDATE aggregate_definitions
		SET last_refreshed = ?, next_refresh = ?, updated_at = ?
		WHERE id = ?
	`, def.LastRefreshed, def.NextRefresh, def.UpdatedAt, aggregateID)

	return err
}

// LogRefresh logs a refresh operation
func (s *MaterializedStore) LogRefresh(ctx context.Context, log *AggregateRefreshLog) error {
	_, err := s.duckdb.Exec(ctx, `
		INSERT INTO aggregate_refresh_log (
			id, aggregate_id, started_at, completed_at, status,
			rows_processed, duration_ms, error_message
		) VALUES (nextval('refresh_log_seq'), ?, ?, ?, ?, ?, ?, ?)
	`,
		log.AggregateID, log.StartedAt, log.CompletedAt, log.Status,
		log.RowsProcessed, log.DurationMs, log.ErrorMessage,
	)

	return err
}

// GetRefreshLogs retrieves refresh logs for an aggregate
func (s *MaterializedStore) GetRefreshLogs(ctx context.Context, aggregateID string, limit int) ([]*AggregateRefreshLog, error) {
	rows, err := s.duckdb.Query(ctx, `
		SELECT id, aggregate_id, started_at, completed_at, status,
			   rows_processed, duration_ms, error_message
		FROM aggregate_refresh_log
		WHERE aggregate_id = ?
		ORDER BY started_at DESC
		LIMIT ?
	`, aggregateID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*AggregateRefreshLog
	for rows.Next() {
		var log AggregateRefreshLog
		var completedAt sql.NullTime
		var errorMsg sql.NullString

		err := rows.Scan(
			&log.ID, &log.AggregateID, &log.StartedAt, &completedAt,
			&log.Status, &log.RowsProcessed, &log.DurationMs, &errorMsg,
		)
		if err != nil {
			return nil, err
		}

		if completedAt.Valid {
			log.CompletedAt = completedAt.Time
		}
		if errorMsg.Valid {
			log.ErrorMessage = errorMsg.String
		}

		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// DeleteAggregate removes an aggregate and its values
func (s *MaterializedStore) DeleteAggregate(ctx context.Context, aggregateID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete values first
	if _, err := s.duckdb.Exec(ctx, "DELETE FROM aggregate_values WHERE aggregate_id = ?", aggregateID); err != nil {
		return err
	}

	// Delete definition
	if _, err := s.duckdb.Exec(ctx, "DELETE FROM aggregate_definitions WHERE id = ?", aggregateID); err != nil {
		return err
	}

	delete(s.definitions, aggregateID)
	return nil
}

// ClearAggregateValues removes all values for an aggregate (before refresh)
func (s *MaterializedStore) ClearAggregateValues(ctx context.Context, aggregateID string) error {
	_, err := s.duckdb.Exec(ctx, "DELETE FROM aggregate_values WHERE aggregate_id = ?", aggregateID)
	return err
}

// GetPendingScheduledRefreshes returns aggregates that need scheduled refresh
func (s *MaterializedStore) GetPendingScheduledRefreshes() []*AggregateDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var pending []*AggregateDefinition

	for _, def := range s.definitions {
		if def.IsEnabled && def.RefreshStrategy == RefreshScheduled {
			if def.NextRefresh.IsZero() || now.After(def.NextRefresh) {
				pending = append(pending, def)
			}
		}
	}

	return pending
}

// GetLazyAggregates returns aggregates configured for lazy refresh
func (s *MaterializedStore) GetLazyAggregates() []*AggregateDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lazy []*AggregateDefinition
	for _, def := range s.definitions {
		if def.IsEnabled && def.RefreshStrategy == RefreshLazy {
			lazy = append(lazy, def)
		}
	}

	return lazy
}

// RegisterCDCCallback registers a callback for CDC events
func (s *MaterializedStore) RegisterCDCCallback(callback func(tableName string, operation string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cdcCallbacks = append(s.cdcCallbacks, callback)
}

// NotifyCDC notifies all CDC callbacks about a change
func (s *MaterializedStore) NotifyCDC(tableName, operation string) {
	s.mu.RLock()
	callbacks := s.cdcCallbacks
	s.mu.RUnlock()

	for _, cb := range callbacks {
		cb(tableName, operation)
	}
}

// generateID generates a unique ID for an aggregate definition
func (s *MaterializedStore) generateID(def *AggregateDefinition) string {
	data, _ := json.Marshal(map[string]interface{}{
		"name":        def.Name,
		"table":       def.SourceTable,
		"type":        def.AggregateType,
		"column":      def.ColumnName,
		"group_by":    def.GroupByColumns,
		"filter":      def.FilterCondition,
	})

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8])
}

// BuildGroupKey builds a group key from values
func BuildGroupKey(values map[string]interface{}) string {
	if len(values) == 0 {
		return "_total_"
	}

	data, _ := json.Marshal(values)
	return string(data)
}

// ParseGroupKey parses a group key back to values
func ParseGroupKey(key string) (map[string]interface{}, error) {
	if key == "_total_" {
		return nil, nil
	}

	var values map[string]interface{}
	err := json.Unmarshal([]byte(key), &values)
	return values, err
}
