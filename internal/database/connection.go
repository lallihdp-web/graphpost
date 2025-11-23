package database

import (
	"context"
	"fmt"
	"time"

	"github.com/graphpost/graphpost/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connection manages the database connection pool using pgx
type Connection struct {
	pool   *pgxpool.Pool
	config *config.DatabaseConfig
}

// NewConnection creates a new database connection pool
func NewConnection(cfg *config.DatabaseConfig) (*Connection, error) {
	// Build connection string
	connString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	// Parse config
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Apply pgx pool configuration
	poolCfg := cfg.Pool

	// Pool size settings
	if poolCfg.MinConns > 0 {
		poolConfig.MinConns = poolCfg.MinConns
	} else if cfg.MaxIdleConns > 0 {
		// Legacy fallback
		poolConfig.MinConns = int32(cfg.MaxIdleConns)
	} else {
		poolConfig.MinConns = 5
	}

	if poolCfg.MaxConns > 0 {
		poolConfig.MaxConns = poolCfg.MaxConns
	} else if cfg.MaxOpenConns > 0 {
		// Legacy fallback
		poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	} else {
		poolConfig.MaxConns = 50
	}

	// Connection lifetime settings
	if poolCfg.MaxConnLifetime > 0 {
		poolConfig.MaxConnLifetime = poolCfg.MaxConnLifetime
	} else if cfg.ConnMaxLifetime > 0 {
		// Legacy fallback
		poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime
	} else {
		poolConfig.MaxConnLifetime = 1 * time.Hour
	}

	if poolCfg.MaxConnIdleTime > 0 {
		poolConfig.MaxConnIdleTime = poolCfg.MaxConnIdleTime
	} else {
		poolConfig.MaxConnIdleTime = 30 * time.Minute
	}

	// Health check settings
	if poolCfg.HealthCheckPeriod > 0 {
		poolConfig.HealthCheckPeriod = poolCfg.HealthCheckPeriod
	} else {
		poolConfig.HealthCheckPeriod = 1 * time.Minute
	}

	// Connection timeout
	connectTimeout := 10 * time.Second
	if poolCfg.ConnectTimeout > 0 {
		connectTimeout = poolCfg.ConnectTimeout
	}
	poolConfig.ConnConfig.ConnectTimeout = connectTimeout

	// Simple protocol mode (useful for PgBouncer transaction pooling)
	if poolCfg.PreferSimpleProtocol {
		poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	// Add after connect hook for setting search_path
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if cfg.Schema != "" && cfg.Schema != "public" {
			_, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s, public", cfg.Schema))
			return err
		}
		return nil
	}

	// Create connection pool
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection (unless lazy connect is enabled)
	if !poolCfg.LazyConnect {
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to ping database: %w", err)
		}
	}

	return &Connection{
		pool:   pool,
		config: cfg,
	}, nil
}

// Pool returns the underlying pgxpool.Pool
func (c *Connection) Pool() *pgxpool.Pool {
	return c.pool
}

// Close closes the database connection pool
func (c *Connection) Close() error {
	c.pool.Close()
	return nil
}

// Ping tests the database connection
func (c *Connection) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// Stats returns database connection pool statistics
func (c *Connection) Stats() *pgxpool.Stat {
	return c.pool.Stat()
}

// Acquire acquires a connection from the pool
func (c *Connection) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	return c.pool.Acquire(ctx)
}

// Exec executes a query that doesn't return rows
func (c *Connection) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return c.pool.Exec(ctx, sql, args...)
}

// Query executes a query that returns rows
func (c *Connection) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return c.pool.Query(ctx, sql, args...)
}

// QueryRow executes a query that returns at most one row
func (c *Connection) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return c.pool.QueryRow(ctx, sql, args...)
}

// Begin starts a transaction
func (c *Connection) Begin(ctx context.Context) (pgx.Tx, error) {
	return c.pool.Begin(ctx)
}

// BeginTx starts a transaction with the given options
func (c *Connection) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	return c.pool.BeginTx(ctx, txOptions)
}

// Transaction executes a function within a database transaction
func (c *Connection) Transaction(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx err: %v, rb err: %v", err, rbErr)
		}
		return err
	}

	return tx.Commit(ctx)
}

// SendBatch sends a batch of queries
func (c *Connection) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	return c.pool.SendBatch(ctx, batch)
}

// CopyFrom efficiently copies data using PostgreSQL's COPY protocol
func (c *Connection) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return c.pool.CopyFrom(ctx, tableName, columnNames, rowSrc)
}

// Notify sends a notification on a channel
func (c *Connection) Notify(ctx context.Context, channel, payload string) error {
	_, err := c.pool.Exec(ctx, "SELECT pg_notify($1, $2)", channel, payload)
	return err
}

// HealthCheck returns detailed health check information
func (c *Connection) HealthCheck(ctx context.Context) map[string]interface{} {
	stats := c.pool.Stat()

	health := map[string]interface{}{
		"status": "healthy",
		"pool": map[string]interface{}{
			"total_connections":          stats.TotalConns(),
			"acquired_connections":       stats.AcquiredConns(),
			"idle_connections":           stats.IdleConns(),
			"max_connections":            stats.MaxConns(),
			"constructing_connections":   stats.ConstructingConns(),
			"new_connections_count":      stats.NewConnsCount(),
			"max_lifetime_destroy_count": stats.MaxLifetimeDestroyCount(),
			"max_idle_destroy_count":     stats.MaxIdleDestroyCount(),
		},
	}

	// Test actual connection
	if err := c.pool.Ping(ctx); err != nil {
		health["status"] = "unhealthy"
		health["error"] = err.Error()
	}

	return health
}

// GetDSN returns the connection DSN
func (c *Connection) GetDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.config.User, c.config.Password, c.config.Host, c.config.Port, c.config.Database, c.config.SSLMode)
}

// Config returns the database configuration
func (c *Connection) Config() *config.DatabaseConfig {
	return c.config
}

// Listener handles PostgreSQL LISTEN/NOTIFY using pgx
type Listener struct {
	conn        *pgx.Conn
	connConfig  *pgx.ConnConfig
	channel     string
	callback    func(notification *pgconn.Notification)
	stopChan    chan struct{}
}

// NewListener creates a new LISTEN/NOTIFY listener
func NewListener(cfg *config.DatabaseConfig, channel string, callback func(notification *pgconn.Notification)) (*Listener, error) {
	connString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	connConfig, err := pgx.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	return &Listener{
		connConfig: connConfig,
		channel:    channel,
		callback:   callback,
		stopChan:   make(chan struct{}),
	}, nil
}

// Start begins listening for notifications
func (l *Listener) Start(ctx context.Context) error {
	conn, err := pgx.ConnectConfig(ctx, l.connConfig)
	if err != nil {
		return fmt.Errorf("failed to connect for listen: %w", err)
	}
	l.conn = conn

	// Start listening
	_, err = conn.Exec(ctx, fmt.Sprintf("LISTEN %s", l.channel))
	if err != nil {
		conn.Close(ctx)
		return fmt.Errorf("failed to listen on channel %s: %w", l.channel, err)
	}

	// Start notification handler goroutine
	go l.handleNotifications(ctx)

	return nil
}

// handleNotifications handles incoming notifications
func (l *Listener) handleNotifications(ctx context.Context) {
	for {
		select {
		case <-l.stopChan:
			return
		case <-ctx.Done():
			return
		default:
			// Wait for notification with timeout
			waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			notification, err := l.conn.WaitForNotification(waitCtx)
			cancel()

			if err != nil {
				// Check if context was cancelled or timeout
				if ctx.Err() != nil {
					return
				}
				// Timeout is expected, continue
				continue
			}

			if l.callback != nil && notification != nil {
				l.callback(notification)
			}
		}
	}
}

// Stop stops listening for notifications
func (l *Listener) Stop(ctx context.Context) error {
	close(l.stopChan)
	if l.conn != nil {
		return l.conn.Close(ctx)
	}
	return nil
}

// ListenNotify is a legacy wrapper for compatibility
type ListenNotify struct {
	listener *Listener
}

// NewListenNotify creates a new LISTEN/NOTIFY handler (legacy interface)
func NewListenNotify(conn *Connection, channel string, callback func(payload string)) *ListenNotify {
	listener, _ := NewListener(conn.config, channel, func(n *pgconn.Notification) {
		if callback != nil {
			callback(n.Payload)
		}
	})
	return &ListenNotify{listener: listener}
}

// Start begins listening for notifications
func (ln *ListenNotify) Start(ctx context.Context) error {
	if ln.listener != nil {
		return ln.listener.Start(ctx)
	}
	return nil
}

// Stop stops listening for notifications
func (ln *ListenNotify) Stop(ctx context.Context) error {
	if ln.listener != nil {
		return ln.listener.Stop(ctx)
	}
	return nil
}
