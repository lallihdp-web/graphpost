//go:build !cgo

package analytics

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

// ErrDuckDBNotAvailable is returned when DuckDB is not available (no CGO)
var ErrDuckDBNotAvailable = errors.New("DuckDB is not available: binary compiled without CGO support")

// DuckDB provides an interface for analytical queries using DuckDB
// This is a stub implementation for builds without CGO
type DuckDB struct {
	config DuckDBConfig
	mu     sync.RWMutex
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

	// Enabled controls whether DuckDB analytics is enabled
	Enabled bool
}

// NewDuckDB creates a new DuckDB instance
// In non-CGO builds, this returns nil with no error if disabled, or an error if enabled
func NewDuckDB(cfg DuckDBConfig) (*DuckDB, error) {
	if cfg.Enabled {
		return nil, ErrDuckDBNotAvailable
	}
	// Return a stub instance when disabled
	return &DuckDB{config: cfg}, nil
}

// Query executes a read-only analytical query
func (d *DuckDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, ErrDuckDBNotAvailable
}

// QueryRow executes a query that returns a single row
func (d *DuckDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return nil
}

// Exec executes a write query
func (d *DuckDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, ErrDuckDBNotAvailable
}

// ExecBatch executes multiple statements in a transaction
func (d *DuckDB) ExecBatch(ctx context.Context, queries []string) error {
	return ErrDuckDBNotAvailable
}

// ImportFromPostgres imports data from PostgreSQL using DuckDB's postgres_scanner
func (d *DuckDB) ImportFromPostgres(ctx context.Context, pgConnStr, query, targetTable string) error {
	return ErrDuckDBNotAvailable
}

// AttachPostgres attaches a PostgreSQL database for direct querying
func (d *DuckDB) AttachPostgres(ctx context.Context, name, connStr string) error {
	return ErrDuckDBNotAvailable
}

// CreateAggregateTable creates an optimized table for aggregate storage
func (d *DuckDB) CreateAggregateTable(ctx context.Context, tableName string, columns []ColumnDef) error {
	return ErrDuckDBNotAvailable
}

// ColumnDef defines a column for table creation
type ColumnDef struct {
	Name string
	Type string
}

// Stats returns DuckDB statistics
func (d *DuckDB) Stats() DuckDBStats {
	return DuckDBStats{}
}

// DuckDBStats holds DuckDB statistics
type DuckDBStats struct {
	QueryCount       int64
	TotalDuration    time.Duration
	AvgQueryDuration time.Duration
}

// Close closes the DuckDB connection
func (d *DuckDB) Close() error {
	return nil
}

// Ping checks if DuckDB is responsive
func (d *DuckDB) Ping(ctx context.Context) error {
	return ErrDuckDBNotAvailable
}

// GetDB returns the underlying database connection
func (d *DuckDB) GetDB() *sql.DB {
	return nil
}

// IsAvailable returns whether DuckDB is available in this build
func (d *DuckDB) IsAvailable() bool {
	return false
}
