# Database Connection & Pooling

## Overview

GraphPost uses [pgx](https://github.com/jackc/pgx) v5, the most performant PostgreSQL driver for Go, with built-in connection pooling via `pgxpool`.

## Why pgx Over lib/pq?

| Feature | pgx | lib/pq |
|---------|-----|--------|
| Protocol | Binary | Text |
| Connection Pooling | Built-in (pgxpool) | External required |
| LISTEN/NOTIFY | Native support | Separate listener |
| Batch Operations | SendBatch() | Not supported |
| COPY Protocol | CopyFrom() | Limited |
| Type Support | Excellent (native Go types) | Basic |
| Performance | ~2-3x faster | Baseline |

## Connection Configuration

### Configuration Structure

```go
// internal/config/config.go
type DatabaseConfig struct {
    Host         string `json:"host"`
    Port         int    `json:"port"`
    User         string `json:"user"`
    Password     string `json:"password"`
    Database     string `json:"database"`
    SSLMode      string `json:"sslmode"`
    Schema       string `json:"schema"`
    MaxOpenConns int    `json:"max_open_conns"`
    MaxIdleConns int    `json:"max_idle_conns"`
    PoolMinConns int    `json:"pool_min_conns"`
    PoolMaxConns int    `json:"pool_max_conns"`
}
```

### Environment Variables

```bash
GRAPHPOST_DATABASE_URL="postgres://user:password@host:5432/database?sslmode=disable"
# Or individual settings:
GRAPHPOST_DB_HOST="localhost"
GRAPHPOST_DB_PORT="5432"
GRAPHPOST_DB_USER="postgres"
GRAPHPOST_DB_PASSWORD="secret"
GRAPHPOST_DB_NAME="mydb"
GRAPHPOST_DB_SSLMODE="disable"
GRAPHPOST_DB_SCHEMA="public"
```

## Connection Pool

### Pool Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      pgxpool.Pool                            │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Configuration:                                              │
│  ├── MinConns: 5 (always kept alive)                        │
│  ├── MaxConns: 50 (maximum concurrent)                      │
│  ├── MaxConnLifetime: 1 hour                                │
│  ├── MaxConnIdleTime: 30 minutes                            │
│  └── HealthCheckPeriod: 1 minute                            │
│                                                              │
│  Connection States:                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │  Idle    │  │  In Use  │  │ Creating │                  │
│  │ (ready)  │  │ (active) │  │  (new)   │                  │
│  └──────────┘  └──────────┘  └──────────┘                  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Pool Implementation

```go
// internal/database/connection.go

// NewConnection creates a new database connection with pooling
func NewConnection(cfg *config.DatabaseConfig) (*Connection, error) {
    connString := fmt.Sprintf(
        "host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
    )

    poolConfig, err := pgxpool.ParseConfig(connString)
    if err != nil {
        return nil, err
    }

    // Pool settings
    poolConfig.MinConns = int32(cfg.PoolMinConns)  // Default: 5
    poolConfig.MaxConns = int32(cfg.PoolMaxConns)  // Default: 50
    poolConfig.MaxConnLifetime = time.Hour
    poolConfig.MaxConnIdleTime = 30 * time.Minute
    poolConfig.HealthCheckPeriod = time.Minute

    // Set search_path on each new connection
    poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
        _, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s", cfg.Schema))
        return err
    }

    pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
    if err != nil {
        return nil, err
    }

    return &Connection{pool: pool, config: cfg}, nil
}
```

### Connection Acquisition

```go
// Automatic connection management
func (c *Connection) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
    // 1. Acquires connection from pool
    // 2. Executes query
    // 3. Returns rows (connection released when rows closed)
    return c.pool.Query(ctx, sql, args...)
}

func (c *Connection) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
    // 1. Acquires connection from pool
    // 2. Executes statement
    // 3. Releases connection immediately
    return c.pool.Exec(ctx, sql, args...)
}
```

## Health Checks

### Health Check Implementation

```go
// internal/database/connection.go

func (c *Connection) HealthCheck(ctx context.Context) (*HealthStatus, error) {
    stat := c.pool.Stat()

    status := &HealthStatus{
        Status:           "healthy",
        TotalConnections: stat.TotalConns(),
        IdleConnections:  stat.IdleConns(),
        InUseConnections: stat.AcquiredConns(),
        MaxConnections:   stat.MaxConns(),
    }

    // Verify actual connectivity
    if err := c.Ping(ctx); err != nil {
        status.Status = "unhealthy"
        status.Error = err.Error()
    }

    return status, nil
}
```

### Health Endpoint Response

```json
{
    "database": {
        "status": "healthy",
        "total_connections": 10,
        "idle_connections": 8,
        "in_use_connections": 2,
        "max_connections": 50
    }
}
```

## LISTEN/NOTIFY Support

### Listener Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                       Listener                               │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Dedicated Connection (separate from pool)                   │
│  ├── LISTEN channel_name                                    │
│  └── WaitForNotification (blocking)                         │
│                                                              │
│  Notification Flow:                                          │
│  1. Database: NOTIFY channel, 'payload'                     │
│  2. Listener: WaitForNotification returns                   │
│  3. Callback: Process notification                          │
│  4. Loop: Wait for next notification                        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Listener Implementation

```go
// internal/database/connection.go

type Listener struct {
    conn      *pgx.Conn
    channel   string
    callback  func(*pgconn.Notification)
    stopChan  chan struct{}
}

func NewListener(cfg *pgxpool.Config, channel string, callback func(*pgconn.Notification)) (*Listener, error) {
    // Create dedicated connection for listening
    conn, err := pgx.ConnectConfig(context.Background(), cfg.ConnConfig)
    if err != nil {
        return nil, err
    }

    return &Listener{
        conn:     conn,
        channel:  channel,
        callback: callback,
        stopChan: make(chan struct{}),
    }, nil
}

func (l *Listener) Start(ctx context.Context) error {
    // Subscribe to channel
    _, err := l.conn.Exec(ctx, fmt.Sprintf("LISTEN %s", l.channel))
    if err != nil {
        return err
    }

    // Start listening goroutine
    go l.listen(ctx)
    return nil
}

func (l *Listener) listen(ctx context.Context) {
    for {
        select {
        case <-l.stopChan:
            return
        case <-ctx.Done():
            return
        default:
            // Wait for notification with timeout
            notification, err := l.conn.WaitForNotification(ctx)
            if err != nil {
                continue
            }
            // Invoke callback
            l.callback(notification)
        }
    }
}
```

## Batch Operations

### SendBatch for Multiple Queries

```go
// Execute multiple queries in single round-trip
func (c *Connection) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
    return c.pool.SendBatch(ctx, batch)
}

// Usage example
batch := &pgx.Batch{}
batch.Queue("SELECT * FROM users WHERE id = $1", 1)
batch.Queue("SELECT * FROM orders WHERE user_id = $1", 1)
batch.Queue("SELECT COUNT(*) FROM products")

results := conn.SendBatch(ctx, batch)
defer results.Close()

// Process results in order
userRows, _ := results.Query()
orderRows, _ := results.Query()
countRow := results.QueryRow()
```

### Performance Impact

```
Without Batch (3 queries):
  Network Round Trips: 3
  Total Latency: ~15ms (5ms × 3)

With Batch (3 queries):
  Network Round Trips: 1
  Total Latency: ~6ms

Improvement: ~60% latency reduction
```

## COPY Protocol

### Bulk Insert with CopyFrom

```go
// internal/database/connection.go

func (c *Connection) CopyFrom(
    ctx context.Context,
    tableName pgx.Identifier,
    columnNames []string,
    rowSrc pgx.CopyFromSource,
) (int64, error) {
    return c.pool.CopyFrom(ctx, tableName, columnNames, rowSrc)
}

// Usage example
rows := [][]interface{}{
    {1, "John", "john@example.com"},
    {2, "Jane", "jane@example.com"},
    {3, "Bob", "bob@example.com"},
}

copyCount, err := conn.CopyFrom(
    ctx,
    pgx.Identifier{"users"},
    []string{"id", "name", "email"},
    pgx.CopyFromRows(rows),
)
```

### Performance Comparison

```
INSERT (1000 rows):
  Individual INSERTs: ~2000ms
  Batch INSERT: ~200ms
  COPY Protocol: ~20ms

COPY is 100x faster than individual INSERTs
```

## Schema Introspection

### Introspector Implementation

```go
// internal/database/introspector.go

type Introspector struct {
    db     *pgxpool.Pool
    schema string
}

func (i *Introspector) IntrospectSchema(ctx context.Context) (*Schema, error) {
    schema := &Schema{
        Tables: make(map[string]*Table),
        Views:  make(map[string]*View),
    }

    // 1. Get all tables
    tables, err := i.getTables(ctx)

    // 2. For each table, get columns
    for _, table := range tables {
        columns, err := i.getColumns(ctx, table.Name)
        table.Columns = columns
    }

    // 3. Get primary keys
    for _, table := range tables {
        pk, err := i.getPrimaryKey(ctx, table.Name)
        table.PrimaryKey = pk
    }

    // 4. Get foreign keys (relationships)
    for _, table := range tables {
        fks, err := i.getForeignKeys(ctx, table.Name)
        table.ForeignKeys = fks
    }

    return schema, nil
}
```

### Introspection Queries

```sql
-- Get tables
SELECT table_name, table_type
FROM information_schema.tables
WHERE table_schema = $1
  AND table_type IN ('BASE TABLE', 'VIEW');

-- Get columns
SELECT
    column_name,
    data_type,
    udt_name,
    is_nullable,
    column_default,
    character_maximum_length,
    numeric_precision,
    numeric_scale
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2
ORDER BY ordinal_position;

-- Get primary key
SELECT a.attname
FROM pg_index i
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE i.indrelid = $1::regclass AND i.indisprimary;

-- Get foreign keys
SELECT
    kcu.column_name,
    ccu.table_name AS foreign_table_name,
    ccu.column_name AS foreign_column_name,
    tc.constraint_name
FROM information_schema.table_constraints AS tc
JOIN information_schema.key_column_usage AS kcu
    ON tc.constraint_name = kcu.constraint_name
JOIN information_schema.constraint_column_usage AS ccu
    ON ccu.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'FOREIGN KEY'
  AND tc.table_schema = $1
  AND tc.table_name = $2;
```

## Connection Lifecycle

```
Application Start
       │
       ▼
┌─────────────────┐
│ NewConnection() │ ──► Create pgxpool.Pool
└────────┬────────┘     (MinConns connections created)
         │
         ▼
┌─────────────────┐
│ Initialize()    │ ──► Pool health check
└────────┬────────┘
         │
         ▼
    [Running]
    ┌─────────────────────────────────────┐
    │ Query/Exec Operations               │
    │ ├── Acquire connection from pool    │
    │ ├── Execute SQL                     │
    │ └── Release connection to pool      │
    │                                     │
    │ Background Tasks:                   │
    │ ├── Health checks (1 min interval) │
    │ ├── Idle connection cleanup        │
    │ └── Connection lifetime management │
    └─────────────────────────────────────┘
         │
         ▼
┌─────────────────┐
│ Close()         │ ──► Close all connections
└─────────────────┘     Release resources
```

## Error Handling

### Connection Errors

```go
// Retry logic for transient errors
func (c *Connection) execWithRetry(ctx context.Context, sql string, args ...interface{}) error {
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        _, err := c.pool.Exec(ctx, sql, args...)
        if err == nil {
            return nil
        }

        // Check if retryable
        if !isRetryableError(err) {
            return err
        }

        // Exponential backoff
        time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond)
    }
    return errors.New("max retries exceeded")
}

func isRetryableError(err error) bool {
    // Connection reset, timeout, etc.
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        switch pgErr.Code {
        case "57P01", // admin_shutdown
             "57P02", // crash_shutdown
             "57P03": // cannot_connect_now
            return true
        }
    }
    return false
}
```

## Best Practices

1. **Pool Sizing**
   - MinConns: Set to expected minimum concurrent queries
   - MaxConns: Set based on PostgreSQL max_connections and other apps

2. **Connection Lifetime**
   - Use MaxConnLifetime to prevent stale connections
   - Balance between connection reuse and freshness

3. **Timeout Configuration**
   - Set appropriate query timeouts
   - Use context with timeout for all operations

4. **Health Monitoring**
   - Monitor pool statistics
   - Alert on high connection usage
   - Track query latencies
