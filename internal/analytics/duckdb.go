package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

// DuckDB provides an interface for analytical queries using DuckDB
type DuckDB struct {
	db     *sql.DB
	config DuckDBConfig
	mu     sync.RWMutex

	// Stats
	queryCount    int64
	totalDuration time.Duration
}

// DuckDBConfig holds DuckDB configuration
type DuckDBConfig struct {
	// Path is the database file path (empty for in-memory)
	Path string

	// MemoryLimit is the maximum memory DuckDB can use (e.g., "4GB")
	MemoryLimit string

	// Threads is the number of threads for query execution (0 = auto)
	Threads int

	// AccessMode is the database access mode: "read_write" or "read_only"
	AccessMode string

	// TempDirectory for spilling large operations
	TempDirectory string
}

// NewDuckDB creates a new DuckDB instance
func NewDuckDB(cfg DuckDBConfig) (*DuckDB, error) {
	// Build connection string
	dsn := cfg.Path
	if dsn == "" {
		dsn = ":memory:"
	}

	// Add configuration options
	opts := ""
	if cfg.MemoryLimit != "" {
		opts += fmt.Sprintf("&memory_limit=%s", cfg.MemoryLimit)
	}
	if cfg.Threads > 0 {
		opts += fmt.Sprintf("&threads=%d", cfg.Threads)
	}
	if cfg.AccessMode != "" {
		opts += fmt.Sprintf("&access_mode=%s", cfg.AccessMode)
	}
	if cfg.TempDirectory != "" {
		opts += fmt.Sprintf("&temp_directory=%s", cfg.TempDirectory)
	}

	if opts != "" {
		dsn += "?" + opts[1:] // Remove leading &
	}

	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // DuckDB is single-writer
	db.SetMaxIdleConns(1)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping DuckDB: %w", err)
	}

	duck := &DuckDB{
		db:     db,
		config: cfg,
	}

	// Initialize schema for materialized aggregates
	if err := duck.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize DuckDB schema: %w", err)
	}

	return duck, nil
}

// initSchema creates the necessary tables for materialized aggregates
func (d *DuckDB) initSchema() error {
	schema := `
		-- Materialized aggregate definitions
		CREATE TABLE IF NOT EXISTS aggregate_definitions (
			id VARCHAR PRIMARY KEY,
			name VARCHAR NOT NULL,
			source_table VARCHAR NOT NULL,
			aggregate_type VARCHAR NOT NULL,
			column_name VARCHAR,
			group_by_columns VARCHAR,
			filter_condition VARCHAR,
			refresh_strategy VARCHAR NOT NULL,
			refresh_interval_seconds INTEGER,
			cron_schedule VARCHAR,
			last_refreshed TIMESTAMP,
			next_refresh TIMESTAMP,
			is_enabled BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		-- Materialized aggregate values (key-value store for results)
		CREATE TABLE IF NOT EXISTS aggregate_values (
			aggregate_id VARCHAR NOT NULL,
			group_key VARCHAR NOT NULL,
			value DOUBLE,
			count BIGINT,
			sum DOUBLE,
			min DOUBLE,
			max DOUBLE,
			avg DOUBLE,
			computed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (aggregate_id, group_key)
		);

		-- Aggregate refresh history for monitoring
		CREATE TABLE IF NOT EXISTS aggregate_refresh_log (
			id INTEGER PRIMARY KEY,
			aggregate_id VARCHAR NOT NULL,
			started_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP,
			status VARCHAR NOT NULL,
			rows_processed BIGINT,
			duration_ms BIGINT,
			error_message VARCHAR
		);

		-- Create sequence for refresh log
		CREATE SEQUENCE IF NOT EXISTS refresh_log_seq START 1;

		-- Index for faster lookups
		CREATE INDEX IF NOT EXISTS idx_aggregate_values_id ON aggregate_values(aggregate_id);
		CREATE INDEX IF NOT EXISTS idx_refresh_log_aggregate ON aggregate_refresh_log(aggregate_id);
	`

	_, err := d.db.Exec(schema)
	return err
}

// Query executes a read-only analytical query
func (d *DuckDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	start := time.Now()
	rows, err := d.db.QueryContext(ctx, query, args...)
	d.totalDuration += time.Since(start)
	d.queryCount++

	return rows, err
}

// QueryRow executes a query that returns a single row
func (d *DuckDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	d.mu.RLock()
	defer d.mu.RUnlock()

	start := time.Now()
	row := d.db.QueryRowContext(ctx, query, args...)
	d.totalDuration += time.Since(start)
	d.queryCount++

	return row
}

// Exec executes a write query
func (d *DuckDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.db.ExecContext(ctx, query, args...)
}

// ExecBatch executes multiple statements in a transaction
func (d *DuckDB) ExecBatch(ctx context.Context, queries []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return tx.Commit()
}

// ImportFromPostgres imports data from PostgreSQL using DuckDB's postgres_scanner
func (d *DuckDB) ImportFromPostgres(ctx context.Context, pgConnStr, query, targetTable string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Install and load postgres extension
	if _, err := d.db.ExecContext(ctx, "INSTALL postgres; LOAD postgres;"); err != nil {
		return fmt.Errorf("failed to load postgres extension: %w", err)
	}

	// Create or replace the target table with data from PostgreSQL
	importQuery := fmt.Sprintf(`
		CREATE OR REPLACE TABLE %s AS
		SELECT * FROM postgres_query('%s', '%s')
	`, targetTable, pgConnStr, query)

	_, err := d.db.ExecContext(ctx, importQuery)
	return err
}

// AttachPostgres attaches a PostgreSQL database for direct querying
func (d *DuckDB) AttachPostgres(ctx context.Context, name, connStr string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Install and load postgres extension
	if _, err := d.db.ExecContext(ctx, "INSTALL postgres; LOAD postgres;"); err != nil {
		return fmt.Errorf("failed to load postgres extension: %w", err)
	}

	// Attach PostgreSQL database
	attachQuery := fmt.Sprintf("ATTACH '%s' AS %s (TYPE POSTGRES, READ_ONLY)", connStr, name)
	_, err := d.db.ExecContext(ctx, attachQuery)
	return err
}

// CreateAggregateTable creates an optimized table for aggregate storage
func (d *DuckDB) CreateAggregateTable(ctx context.Context, tableName string, columns []ColumnDef) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Build CREATE TABLE statement
	var columnDefs string
	for i, col := range columns {
		if i > 0 {
			columnDefs += ", "
		}
		columnDefs += fmt.Sprintf("%s %s", col.Name, col.Type)
	}

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", tableName, columnDefs)
	_, err := d.db.ExecContext(ctx, query)
	return err
}

// ColumnDef defines a column for table creation
type ColumnDef struct {
	Name string
	Type string
}

// Stats returns DuckDB statistics
func (d *DuckDB) Stats() DuckDBStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return DuckDBStats{
		QueryCount:       d.queryCount,
		TotalDuration:    d.totalDuration,
		AvgQueryDuration: time.Duration(int64(d.totalDuration) / max(d.queryCount, 1)),
	}
}

// DuckDBStats holds DuckDB statistics
type DuckDBStats struct {
	QueryCount       int64
	TotalDuration    time.Duration
	AvgQueryDuration time.Duration
}

// Close closes the DuckDB connection
func (d *DuckDB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.db.Close()
}

// Ping checks if DuckDB is responsive
func (d *DuckDB) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// GetDB returns the underlying database connection (use with caution)
func (d *DuckDB) GetDB() *sql.DB {
	return d.db
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
