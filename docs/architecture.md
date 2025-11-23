# Architecture Overview

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              GraphPost                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐              │
│  │   HTTP       │    │  WebSocket   │    │   Console    │              │
│  │   Server     │    │   Handler    │    │   UI         │              │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘              │
│         │                   │                    │                      │
│         └───────────────────┼────────────────────┘                      │
│                             │                                           │
│                    ┌────────▼────────┐                                  │
│                    │   Authenticator  │                                  │
│                    │  (JWT/Secret/    │                                  │
│                    │   Webhook)       │                                  │
│                    └────────┬────────┘                                  │
│                             │                                           │
│                    ┌────────▼────────┐                                  │
│                    │     Engine       │                                  │
│                    │  (Orchestrator)  │                                  │
│                    └────────┬────────┘                                  │
│                             │                                           │
│         ┌───────────────────┼───────────────────┐                       │
│         │                   │                   │                       │
│  ┌──────▼──────┐    ┌──────▼──────┐    ┌──────▼──────┐                │
│  │   Schema    │    │  Resolver   │    │Subscription │                │
│  │  Generator  │    │             │    │  Manager    │                │
│  └──────┬──────┘    └──────┬──────┘    └──────┬──────┘                │
│         │                   │                   │                       │
│         └───────────────────┼───────────────────┘                       │
│                             │                                           │
│                    ┌────────▼────────┐                                  │
│                    │   Connection    │                                  │
│                    │   (pgxpool)     │                                  │
│                    └────────┬────────┘                                  │
│                             │                                           │
└─────────────────────────────┼───────────────────────────────────────────┘
                              │
                     ┌────────▼────────┐
                     │   PostgreSQL    │
                     │   Database      │
                     └─────────────────┘
```

## Component Interactions

### Request Flow (Query/Mutation)

```
1. HTTP Request
       │
       ▼
2. Authentication Check
       │
       ├── Admin Secret Header?
       │   └── Validate against config
       │
       ├── JWT Token?
       │   └── Validate signature & claims
       │
       └── Webhook Auth?
           └── Call external auth service
       │
       ▼
3. GraphQL Parser
       │
       └── Parse query string
       │
       ▼
4. Schema Validation
       │
       └── Validate against generated schema
       │
       ▼
5. Resolver
       │
       ├── Build SQL query from GraphQL
       ├── Apply permissions/filters
       └── Execute against PostgreSQL
       │
       ▼
6. Response Formation
       │
       └── Format results as GraphQL response
       │
       ▼
7. HTTP Response
```

### Subscription Flow

```
1. WebSocket Connection
       │
       ▼
2. Authentication
       │
       ▼
3. Subscribe to Table
       │
       ├── Create PostgreSQL LISTEN
       └── Register subscriber
       │
       ▼
4. Data Change (INSERT/UPDATE/DELETE)
       │
       ▼
5. PostgreSQL NOTIFY
       │
       ▼
6. Notification Received
       │
       ▼
7. Broadcast to Subscribers
       │
       ▼
8. WebSocket Message
```

## Data Flow

### Schema Generation

```go
// internal/database/introspector.go
1. Connect to PostgreSQL
2. Query information_schema.tables
3. Query information_schema.columns for each table
4. Query pg_constraint for relationships
5. Query pg_index for primary keys
6. Build Schema struct

// internal/schema/generator.go
7. For each table:
   - Create GraphQL Object Type
   - Create Input Types (insert, update)
   - Create Filter Types (where, order_by)
   - Create Query Fields (select, select_by_pk)
   - Create Mutation Fields (insert, update, delete)
   - Create Subscription Fields
8. Register resolvers for each field
9. Return complete GraphQL Schema
```

### Query Execution

```go
// internal/resolver/resolver.go
1. Receive QueryParams:
   - TableName
   - Columns to select
   - Where conditions
   - Order by
   - Limit/Offset

2. Build SQL:
   SELECT col1, col2, ...
   FROM schema.table
   WHERE condition1 AND condition2 ...
   ORDER BY col1 ASC
   LIMIT 10 OFFSET 0

3. Execute Query via pgxpool

4. Scan Results:
   - Use rows.FieldDescriptions() for column names
   - Use rows.Values() for row data
   - Map to []map[string]interface{}

5. Return results
```

## Memory Model

```
┌─────────────────────────────────────────────────────────────────┐
│                         Engine                                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  dbConn (*database.Connection)                                   │
│  ├── pool (*pgxpool.Pool)                                       │
│  │   ├── connections []*pgx.Conn (managed by pgxpool)           │
│  │   └── config (pool settings)                                 │
│  └── config (*config.DatabaseConfig)                            │
│                                                                  │
│  dbSchema (*database.Schema)                                     │
│  ├── Tables map[string]*Table                                   │
│  │   └── [tableName] -> *Table                                  │
│  │       ├── Name, Schema string                                │
│  │       ├── Columns []*Column                                  │
│  │       ├── PrimaryKey []string                                │
│  │       └── ForeignKeys []*ForeignKey                          │
│  └── Views map[string]*View                                     │
│                                                                  │
│  graphqlSchema (*graphql.Schema)                                │
│  ├── QueryType                                                  │
│  ├── MutationType                                               │
│  └── SubscriptionType                                           │
│                                                                  │
│  resolver (*resolver.Resolver)                                   │
│  ├── db (*pgxpool.Pool)                                         │
│  ├── schema (*database.Schema)                                  │
│  └── defaultSchema string                                       │
│                                                                  │
│  subManager (*subscription.Manager)                              │
│  ├── listeners map[string]*database.Listener                    │
│  └── subscribers map[string]map[string]*Subscriber              │
│                                                                  │
│  triggerManager (*events.TriggerManager)                         │
│  ├── triggers map[string]*EventTrigger                          │
│  └── eventQueue chan *Event                                     │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Concurrency Model

### Connection Pool

```
pgxpool.Pool
├── MinConns: 5 (always maintained)
├── MaxConns: 50 (ceiling)
└── Connections are acquired/released per query

Query Execution:
1. Acquire connection from pool
2. Execute SQL
3. Release connection back to pool
```

### Subscription Handling

```
Main Goroutine
├── HTTP Server goroutine
├── Subscription Manager goroutine (per table listener)
│   └── Listens for PostgreSQL NOTIFY
├── Event Trigger poller goroutine
└── Event processor worker pool
    └── N workers for webhook delivery
```

### Thread Safety

| Component | Synchronization | Purpose |
|-----------|-----------------|---------|
| Engine | sync.RWMutex | Protect schema updates |
| Subscription Manager | sync.RWMutex | Protect subscriber maps |
| Event Trigger Manager | sync.RWMutex | Protect trigger registry |
| Connection Pool | Internal (pgxpool) | Connection management |

## Error Handling

### Error Categories

1. **Connection Errors**: Database unreachable
   - Retry with exponential backoff
   - Report unhealthy status

2. **Query Errors**: SQL execution failures
   - Return GraphQL error response
   - Log for debugging

3. **Authentication Errors**: Invalid credentials
   - Return 401/403 HTTP status
   - Include error in GraphQL response

4. **Validation Errors**: Invalid GraphQL query
   - Return descriptive error message
   - Include location information

### Error Propagation

```go
// Errors bubble up through the call stack
Resolver.ResolveQuery()
    └── returns error if SQL fails
        │
Engine.ExecuteQuery()
    └── wraps in GraphQL error format
        │
Server.handleGraphQL()
    └── returns HTTP response with errors array
```

## Configuration Hierarchy

```
1. Default Values (config.DefaultConfig())
       │
       ▼
2. Config File (--config flag)
       │
       ▼
3. Command Line Flags
       │
       ▼
4. Environment Variables (highest priority)
```

## Security Model

```
┌─────────────────────────────────────────────────────────┐
│                    Request                               │
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│              Authentication Layer                        │
│  ┌─────────────┐  ┌───────────┐  ┌─────────────────┐   │
│  │Admin Secret │  │   JWT     │  │ Webhook Auth    │   │
│  │ Header      │  │  Token    │  │ (External)      │   │
│  └─────────────┘  └───────────┘  └─────────────────┘   │
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│              Authorization Layer                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Permission Rules (per role, per table)           │   │
│  │ - SELECT: row-level filters                      │   │
│  │ - INSERT: column presets, validation             │   │
│  │ - UPDATE: column restrictions, filters           │   │
│  │ - DELETE: row-level filters                      │   │
│  └─────────────────────────────────────────────────┘   │
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│              SQL Execution                               │
│  - Parameterized queries (SQL injection prevention)     │
│  - Schema-qualified table names                         │
└─────────────────────────────────────────────────────────┘
```

## Observability Architecture

### Telemetry (OpenTelemetry)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           GraphPost                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    Telemetry Package                              │   │
│  │  internal/telemetry/telemetry.go                                 │   │
│  ├──────────────────────────────────────────────────────────────────┤   │
│  │                                                                   │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │   │
│  │  │ Tracer      │  │ Metrics     │  │ Span Propagation        │  │   │
│  │  │ Provider    │  │ Provider    │  │ (W3C TraceContext)      │  │   │
│  │  └──────┬──────┘  └──────┬──────┘  └─────────────────────────┘  │   │
│  │         │                │                                       │   │
│  │         └────────┬───────┘                                       │   │
│  │                  │                                                │   │
│  │         ┌────────▼────────┐                                      │   │
│  │         │  OTLP Exporter  │                                      │   │
│  │         │  (gRPC / HTTP)  │                                      │   │
│  │         └────────┬────────┘                                      │   │
│  └──────────────────┼───────────────────────────────────────────────┘   │
│                     │                                                    │
└─────────────────────┼────────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                   OpenTelemetry Collector                                │
│           (Jaeger / Grafana Tempo / Datadog / etc.)                     │
└─────────────────────────────────────────────────────────────────────────┘
```

### Trace Spans

```
HTTP Request (parent span)
├── graphql.query (operation)
│   ├── graphql.resolve (field: users)
│   │   ├── db.query (SELECT * FROM users WHERE...)
│   │   │   └── Attributes: db.system, db.operation, db.sql.table, duration_ms
│   │   └── graphql.resolve (field: posts - nested)
│   │       └── db.query (SELECT * FROM posts WHERE user_id IN...)
│   └── graphql.resolve (field: count)
│       └── db.query (SELECT COUNT(*) FROM...)
└── Response sent
```

### Logging Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Logging Package                                       │
│  internal/logging/logging.go                                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                     Logger                                       │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌───────────────────────┐   │    │
│  │  │ Level       │  │ Format      │  │ Output                 │   │    │
│  │  │ Filter      │  │ (JSON/Text) │  │ (stdout/file)          │   │    │
│  │  └─────────────┘  └─────────────┘  └───────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                 Query Logger                                     │    │
│  │  ┌──────────────────┐  ┌────────────────────────────────────┐  │    │
│  │  │ Query Logging    │  │ Slow Query Detection               │  │    │
│  │  │ - SQL statement  │  │ - Threshold comparison             │  │    │
│  │  │ - Duration       │  │ - Optimization suggestions         │  │    │
│  │  │ - Parameters     │  │ - Index recommendations            │  │    │
│  │  └──────────────────┘  └────────────────────────────────────┘  │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Query Log Flow

```
1. Query Received
       │
       ▼
2. Start Timer
       │
       ▼
3. Execute Query
       │
       ▼
4. Stop Timer
       │
       ▼
5. Duration > SlowQueryThreshold?
       │
       ├── YES → Log as WARN with optimization suggestions
       │         - Check for missing indexes
       │         - Suggest EXPLAIN ANALYZE
       │         - Recommend partitioning
       │
       └── NO → Log as DEBUG (if QueryLog enabled)
```

### Sample Log Output

```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "warn",
  "message": "Slow query detected",
  "fields": {
    "query": "SELECT * FROM orders WHERE customer_id = $1 ORDER BY created_at DESC",
    "duration": "1.234s",
    "duration_ms": 1234,
    "table": "orders",
    "operation": "SELECT",
    "slow_query": true,
    "threshold": "500ms",
    "suggestions": "ORDER BY without LIMIT may be slow; Check indexes on table 'orders'"
  }
}
```

---

## Analytics Architecture (DuckDB)

### Overview

GraphPost includes an embedded DuckDB analytics engine for high-performance aggregate queries with materialized data.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Analytics Engine                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐     │
│  │                      Query Router                                     │     │
│  │  ┌──────────────────┐   ┌───────────────────┐   ┌────────────────┐  │     │
│  │  │ Aggregate        │   │ Live Query        │   │ DuckDB Query   │  │     │
│  │  │ Request          │   │ Fallback          │   │ Execution      │  │     │
│  │  └────────┬─────────┘   └─────────┬─────────┘   └────────┬───────┘  │     │
│  │           │                       │                      │          │     │
│  │           └───────────┬───────────┴──────────────────────┘          │     │
│  └───────────────────────┼─────────────────────────────────────────────┘     │
│                          │                                                    │
│  ┌───────────────────────┼─────────────────────────────────────────────┐     │
│  │                       ▼                                               │     │
│  │             Materialized Store                                        │     │
│  │  ┌─────────────────────────────────────────────────────────────┐    │     │
│  │  │ Aggregate Definitions    │    Aggregate Values               │    │     │
│  │  │ - Source table           │    - Group key                   │    │     │
│  │  │ - Aggregate type         │    - Computed value              │    │     │
│  │  │ - Group by columns       │    - Last computed at            │    │     │
│  │  │ - Refresh strategy       │                                   │    │     │
│  │  └─────────────────────────────────────────────────────────────┘    │     │
│  └──────────────────────────────────────────────────────────────────────┘     │
│                                                                               │
│  ┌──────────────────────────────────────────────────────────────────────┐    │
│  │                      Refresh Manager                                   │    │
│  │                                                                        │    │
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────────────┐ │    │
│  │  │ Scheduled  │ │ On-Demand  │ │ CDC-Based  │ │ Lazy (On Miss)     │ │    │
│  │  │ (Cron)     │ │ (API)      │ │ (Realtime) │ │ (Compute & Cache)  │ │    │
│  │  └─────┬──────┘ └─────┬──────┘ └─────┬──────┘ └─────────┬──────────┘ │    │
│  │        │              │              │                   │           │    │
│  │        └──────────────┴──────────────┴───────────────────┘           │    │
│  │                               │                                       │    │
│  │                    ┌──────────▼──────────┐                           │    │
│  │                    │  PostgreSQL Query   │                           │    │
│  │                    │  (Fetch Source)     │                           │    │
│  │                    └──────────┬──────────┘                           │    │
│  │                               │                                       │    │
│  │                    ┌──────────▼──────────┐                           │    │
│  │                    │  DuckDB Storage     │                           │    │
│  │                    │  (Persist Result)   │                           │    │
│  │                    └─────────────────────┘                           │    │
│  └──────────────────────────────────────────────────────────────────────┘    │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Refresh Strategy Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Refresh Strategy Selection                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     SCHEDULED                                         │    │
│  │  ┌─────────┐    ┌──────────────┐    ┌──────────────┐    ┌────────┐  │    │
│  │  │ Timer   │───▶│ Check Due    │───▶│ Query Source │───▶│ Store  │  │    │
│  │  │ Tick    │    │ Aggregates   │    │ PostgreSQL   │    │ Result │  │    │
│  │  └─────────┘    └──────────────┘    └──────────────┘    └────────┘  │    │
│  │  Best for: Dashboards, regular reports                               │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     ON-DEMAND                                         │    │
│  │  ┌─────────┐    ┌──────────────┐    ┌──────────────┐    ┌────────┐  │    │
│  │  │ API     │───▶│ Trigger      │───▶│ Query Source │───▶│ Store  │  │    │
│  │  │ Request │    │ Refresh      │    │ PostgreSQL   │    │ Result │  │    │
│  │  └─────────┘    └──────────────┘    └──────────────┘    └────────┘  │    │
│  │  Best for: User-triggered reports, ad-hoc analytics                  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     CDC (Change Data Capture)                         │    │
│  │  ┌─────────┐    ┌──────────────┐    ┌──────────────┐    ┌────────┐  │    │
│  │  │ Table   │───▶│ Detect       │───▶│ Query Source │───▶│ Store  │  │    │
│  │  │ Change  │    │ Affected     │    │ PostgreSQL   │    │ Result │  │    │
│  │  └─────────┘    └──────────────┘    └──────────────┘    └────────┘  │    │
│  │  Best for: Near real-time analytics, operational dashboards          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     LAZY                                              │    │
│  │  ┌─────────┐    ┌──────────────┐    ┌──────────────┐    ┌────────┐  │    │
│  │  │ Query   │───▶│ Check Cache  │───▶│ Miss? Query  │───▶│ Store  │  │    │
│  │  │ Request │    │ Freshness    │    │ PostgreSQL   │    │ & Return │  │    │
│  │  └─────────┘    └──────────────┘    └──────────────┘    └────────┘  │    │
│  │  Best for: Infrequent queries, cost-sensitive workloads              │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Query Routing Decision Tree

```
                      Aggregate Query Request
                              │
                              ▼
                ┌─────────────────────────────┐
                │  Analytics Router Enabled?   │
                └─────────────┬───────────────┘
                              │
              ┌───────────────┴───────────────┐
              │                               │
             YES                              NO
              │                               │
              ▼                               ▼
    ┌─────────────────────┐         ┌─────────────────────┐
    │ Find Matching       │         │ Execute Live        │
    │ Aggregate Definition │         │ PostgreSQL Query    │
    └──────────┬──────────┘         └─────────────────────┘
               │
    ┌──────────┴──────────┐
    │                     │
  Found              Not Found
    │                     │
    ▼                     ▼
┌─────────────────┐  ┌──────────────────────┐
│ Get Cached      │  │ Fallback Enabled?    │
│ Value           │  └───────────┬──────────┘
└────────┬────────┘              │
         │              ┌────────┴────────┐
   ┌─────┴─────┐       YES               NO
   │           │        │                 │
 Fresh      Stale       ▼                 ▼
   │           │   ┌─────────────┐   ┌─────────┐
   ▼           │   │ PostgreSQL  │   │ Error   │
┌─────────┐    │   │ Live Query  │   │ Response│
│ Return  │    │   └─────────────┘   └─────────┘
│ Cached  │    ▼
└─────────┘  ┌─────────────────────┐
             │ Lazy Strategy?      │
             └──────────┬──────────┘
                        │
             ┌──────────┴──────────┐
            YES                    NO
             │                     │
             ▼                     ▼
       ┌─────────────┐      ┌─────────────┐
       │ Compute,    │      │ Fallback to │
       │ Cache &     │      │ PostgreSQL  │
       │ Return      │      │ Live Query  │
       └─────────────┘      └─────────────┘
```

### DuckDB Storage Schema

```sql
-- Aggregate definitions table
CREATE TABLE aggregate_definitions (
    id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    source_table VARCHAR NOT NULL,
    aggregate_type VARCHAR NOT NULL,
    column_name VARCHAR,
    group_by_columns VARCHAR,
    filter_condition VARCHAR,
    refresh_strategy VARCHAR NOT NULL,
    refresh_interval_seconds INTEGER,
    last_refreshed TIMESTAMP,
    next_refresh TIMESTAMP,
    is_enabled BOOLEAN DEFAULT TRUE
);

-- Materialized values storage
CREATE TABLE aggregate_values (
    aggregate_id VARCHAR NOT NULL,
    group_key VARCHAR NOT NULL,
    value DOUBLE,
    count BIGINT,
    sum DOUBLE,
    min DOUBLE,
    max DOUBLE,
    avg DOUBLE,
    computed_at TIMESTAMP,
    PRIMARY KEY (aggregate_id, group_key)
);

-- Refresh audit log
CREATE TABLE aggregate_refresh_log (
    id INTEGER PRIMARY KEY,
    aggregate_id VARCHAR NOT NULL,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    status VARCHAR NOT NULL,
    rows_processed BIGINT,
    duration_ms BIGINT,
    error_message VARCHAR
);
```

---

## Extensibility Points

1. **Custom Resolvers**: Add business logic before/after queries
2. **Middleware**: HTTP middleware for logging, metrics
3. **Event Handlers**: React to database changes
4. **Remote Schemas**: Stitch external GraphQL APIs
5. **Actions**: Custom mutations with external handlers
6. **Telemetry Hooks**: Custom spans and metrics via OpenTelemetry
7. **Query Analyzers**: Custom slow query analysis and optimization hints
8. **Analytics Aggregates**: Define custom materialized aggregates with configurable refresh strategies
