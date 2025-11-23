package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/graphpost/graphpost/internal/config"
	_ "github.com/lib/pq"
)

// Connection manages the database connection pool
type Connection struct {
	db     *sql.DB
	config *config.DatabaseConfig
	mu     sync.RWMutex
}

// NewConnection creates a new database connection
func NewConnection(cfg *config.DatabaseConfig) (*Connection, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Connection{
		db:     db,
		config: cfg,
	}, nil
}

// DB returns the underlying sql.DB
func (c *Connection) DB() *sql.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.db
}

// Close closes the database connection
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.db.Close()
}

// Ping tests the database connection
func (c *Connection) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Stats returns database connection statistics
func (c *Connection) Stats() sql.DBStats {
	return c.db.Stats()
}

// Transaction executes a function within a database transaction
func (c *Connection) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx err: %v, rb err: %v", err, rbErr)
		}
		return err
	}

	return tx.Commit()
}

// QueryRow executes a query that returns at most one row
func (c *Connection) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return c.db.QueryRowContext(ctx, query, args...)
}

// Query executes a query that returns rows
func (c *Connection) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return c.db.QueryContext(ctx, query, args...)
}

// Exec executes a query that doesn't return rows
func (c *Connection) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

// Prepare creates a prepared statement
func (c *Connection) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return c.db.PrepareContext(ctx, query)
}

// ListenNotify sets up PostgreSQL LISTEN/NOTIFY
type ListenNotify struct {
	conn     *Connection
	channel  string
	callback func(payload string)
}

// NewListenNotify creates a new LISTEN/NOTIFY handler
func NewListenNotify(conn *Connection, channel string, callback func(payload string)) *ListenNotify {
	return &ListenNotify{
		conn:     conn,
		channel:  channel,
		callback: callback,
	}
}

// Start begins listening for notifications
func (ln *ListenNotify) Start(ctx context.Context) error {
	_, err := ln.conn.Exec(ctx, fmt.Sprintf("LISTEN %s", ln.channel))
	if err != nil {
		return err
	}

	// Note: For production, use github.com/lib/pq's Listener for proper LISTEN/NOTIFY
	return nil
}

// Stop stops listening for notifications
func (ln *ListenNotify) Stop(ctx context.Context) error {
	_, err := ln.conn.Exec(ctx, fmt.Sprintf("UNLISTEN %s", ln.channel))
	return err
}
