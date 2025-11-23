# Configuration Reference

## Configuration Methods

GraphPost can be configured through:
1. Configuration file (JSON/YAML)
2. Command-line flags
3. Environment variables

**Priority**: Environment Variables > CLI Flags > Config File > Defaults

## Full Configuration Structure

```go
// internal/config/config.go

type Config struct {
    Server    ServerConfig    `json:"server"`
    Database  DatabaseConfig  `json:"database"`
    Auth      AuthConfig      `json:"auth"`
    Console   ConsoleConfig   `json:"console"`
    Events    EventsConfig    `json:"events"`
    CORS      CORSConfig      `json:"cors"`
    GraphQL   GraphQLConfig   `json:"graphql"`
    Telemetry TelemetryConfig `json:"telemetry"`
    Logging   LoggingConfig   `json:"logging"`
    Cache     CacheConfig     `json:"cache"`
    Analytics AnalyticsConfig `json:"analytics"`
}
```

## Quick Start Environment Variables

```bash
# Essential configuration
export GRAPHPOST_DATABASE_URL="postgres://user:pass@localhost:5432/mydb"
export GRAPHPOST_ADMIN_SECRET="your-admin-secret"
export GRAPHPOST_PORT=8080

# Enable query logging for optimization
export GRAPHPOST_LOG_QUERIES=true
export GRAPHPOST_SLOW_QUERY_THRESHOLD=500ms

# Enable OpenTelemetry
export GRAPHPOST_TELEMETRY_ENABLED=true
export GRAPHPOST_OTLP_ENDPOINT=localhost:4317
```

---

## Server Configuration

```go
type ServerConfig struct {
    Host             string `json:"host"`
    Port             int    `json:"port"`
    EnablePlayground bool   `json:"enable_playground"`
    EnableIntrospection bool `json:"enable_introspection"`
    CORSOrigins      []string `json:"cors_origins"`
    ReadTimeout      int    `json:"read_timeout_seconds"`
    WriteTimeout     int    `json:"write_timeout_seconds"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `host` | `GRAPHPOST_HOST` | `0.0.0.0` | Server bind address |
| `port` | `GRAPHPOST_PORT` | `8080` | Server port |
| `enable_playground` | `GRAPHPOST_ENABLE_PLAYGROUND` | `true` | GraphQL Playground UI |
| `enable_introspection` | `GRAPHPOST_ENABLE_INTROSPECTION` | `true` | GraphQL introspection |
| `cors_origins` | `GRAPHPOST_CORS_ORIGINS` | `["*"]` | CORS allowed origins |
| `read_timeout_seconds` | `GRAPHPOST_READ_TIMEOUT` | `30` | HTTP read timeout |
| `write_timeout_seconds` | `GRAPHPOST_WRITE_TIMEOUT` | `30` | HTTP write timeout |

## Database Configuration

```go
type DatabaseConfig struct {
    Host     string     `json:"host"`
    Port     int        `json:"port"`
    User     string     `json:"user"`
    Password string     `json:"password"`
    Database string     `json:"database"`
    SSLMode  string     `json:"ssl_mode"`
    Schema   string     `json:"schema"`
    Pool     PoolConfig `json:"pool"`
}

type PoolConfig struct {
    MinConns             int32         `json:"min_conns"`
    MaxConns             int32         `json:"max_conns"`
    MaxConnLifetime      time.Duration `json:"max_conn_lifetime"`
    MaxConnIdleTime      time.Duration `json:"max_conn_idle_time"`
    HealthCheckPeriod    time.Duration `json:"health_check_period"`
    ConnectTimeout       time.Duration `json:"connect_timeout"`
    QueryTimeout         time.Duration `json:"query_timeout"`
    LazyConnect          bool          `json:"lazy_connect"`
    PreferSimpleProtocol bool          `json:"prefer_simple_protocol"`
}
```

### Basic Database Parameters

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `host` | `GRAPHPOST_DB_HOST` | `localhost` | PostgreSQL host |
| `port` | `GRAPHPOST_DB_PORT` | `5432` | PostgreSQL port |
| `user` | `GRAPHPOST_DB_USER` | `postgres` | Database user |
| `password` | `GRAPHPOST_DB_PASSWORD` | `` | Database password |
| `database` | `GRAPHPOST_DB_NAME` | `postgres` | Database name |
| `ssl_mode` | `GRAPHPOST_DB_SSLMODE` | `disable` | SSL mode (disable, require, verify-ca, verify-full) |
| `schema` | `GRAPHPOST_DB_SCHEMA` | `public` | Default schema |

### pgx Connection Pool Configuration

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `pool.min_conns` | `GRAPHPOST_POOL_MIN_CONNS` | `5` | Minimum connections kept open (even when idle) |
| `pool.max_conns` | `GRAPHPOST_POOL_MAX_CONNS` | `50` | Maximum connections in pool |
| `pool.max_conn_lifetime` | `GRAPHPOST_POOL_MAX_CONN_LIFETIME` | `1h` | Max lifetime before connection is closed |
| `pool.max_conn_idle_time` | `GRAPHPOST_POOL_MAX_CONN_IDLE_TIME` | `30m` | Max idle time before excess connections closed |
| `pool.health_check_period` | `GRAPHPOST_POOL_HEALTH_CHECK_PERIOD` | `1m` | How often to health check idle connections |
| `pool.connect_timeout` | `GRAPHPOST_POOL_CONNECT_TIMEOUT` | `10s` | Timeout for new connections |
| `pool.query_timeout` | `GRAPHPOST_POOL_QUERY_TIMEOUT` | `0` | Default query timeout (0 = no timeout) |
| `pool.lazy_connect` | `GRAPHPOST_POOL_LAZY_CONNECT` | `false` | Delay connection until first use |
| `pool.prefer_simple_protocol` | `GRAPHPOST_POOL_SIMPLE_PROTOCOL` | `false` | Use simple protocol (for PgBouncer) |

### Database URL

Alternative to individual parameters:

```bash
GRAPHPOST_DATABASE_URL="postgres://user:password@host:5432/database?sslmode=disable&search_path=public"
```

### Pool Tuning Guide

**For high-throughput applications:**
```bash
GRAPHPOST_POOL_MIN_CONNS=10
GRAPHPOST_POOL_MAX_CONNS=100
GRAPHPOST_POOL_MAX_CONN_LIFETIME=30m
```

**For PgBouncer transaction pooling:**
```bash
GRAPHPOST_POOL_SIMPLE_PROTOCOL=true
GRAPHPOST_POOL_MIN_CONNS=2
GRAPHPOST_POOL_MAX_CONNS=20
```

**For development/low-traffic:**
```bash
GRAPHPOST_POOL_MIN_CONNS=1
GRAPHPOST_POOL_MAX_CONNS=10
GRAPHPOST_POOL_LAZY_CONNECT=true
```

## Authentication Configuration

```go
type AuthConfig struct {
    Enabled            bool     `json:"enabled"`
    AdminSecret        string   `json:"admin_secret"`
    JWTSecret          string   `json:"jwt_secret"`
    JWTPublicKey       string   `json:"jwt_public_key"`
    JWTAlgorithm       string   `json:"jwt_algorithm"`
    JWTClaimsNamespace string   `json:"jwt_claims_namespace"`
    WebhookURL         string   `json:"webhook_url"`
    AllowedHeaders     []string `json:"allowed_headers"`
    AnonymousRole      string   `json:"anonymous_role"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enabled` | `GRAPHPOST_AUTH_ENABLED` | `false` | Enable authentication |
| `admin_secret` | `GRAPHPOST_ADMIN_SECRET` | `` | Admin secret key |
| `jwt_secret` | `GRAPHPOST_JWT_SECRET` | `` | JWT signing secret (HS256) |
| `jwt_public_key` | `GRAPHPOST_JWT_PUBLIC_KEY` | `` | JWT public key (RS256) |
| `jwt_algorithm` | `GRAPHPOST_JWT_ALGORITHM` | `HS256` | JWT algorithm |
| `jwt_claims_namespace` | `GRAPHPOST_JWT_CLAIMS_NS` | `https://graphpost.io/jwt/claims` | JWT claims namespace |
| `webhook_url` | `GRAPHPOST_AUTH_WEBHOOK_URL` | `` | Auth webhook URL |
| `anonymous_role` | `GRAPHPOST_ANONYMOUS_ROLE` | `` | Role for unauthenticated requests |

## GraphQL Configuration

```go
type GraphQLConfig struct {
    EnableQueries       bool          `json:"enable_queries"`
    EnableMutations     bool          `json:"enable_mutations"`
    EnableSubscriptions bool          `json:"enable_subscriptions"`
    EnableAggregations  bool          `json:"enable_aggregations"`
    QueryDepthLimit     int           `json:"query_depth_limit"`
    QueryTimeout        time.Duration `json:"query_timeout"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enable_queries` | `GRAPHPOST_ENABLE_QUERIES` | `true` | Allow SELECT operations |
| `enable_mutations` | `GRAPHPOST_ENABLE_MUTATIONS` | `true` | Allow INSERT/UPDATE/DELETE |
| `enable_subscriptions` | `GRAPHPOST_ENABLE_SUBSCRIPTIONS` | `true` | Allow real-time subscriptions |
| `enable_aggregations` | `GRAPHPOST_ENABLE_AGGREGATIONS` | `true` | Allow aggregate queries |
| `query_depth_limit` | `GRAPHPOST_QUERY_DEPTH_LIMIT` | `0` | Max query nesting depth (0 = unlimited) |
| `query_timeout` | `GRAPHPOST_QUERY_TIMEOUT` | `0` | Query timeout via context (0 = no timeout) |

### Read-Only Mode (GET Only)

To run GraphPost in read-only mode for analytics/reporting:

```bash
# Disable all write operations
GRAPHPOST_ENABLE_MUTATIONS=false

# Optionally disable subscriptions
GRAPHPOST_ENABLE_SUBSCRIPTIONS=false
```

### Query Security

```bash
# Limit query depth to prevent deep nested attacks
GRAPHPOST_QUERY_DEPTH_LIMIT=10

# Set query timeout to prevent long-running queries
GRAPHPOST_QUERY_TIMEOUT=30s
```

---

## CORS Configuration

```go
type CORSConfig struct {
    Enabled          bool     `json:"enabled"`
    AllowedOrigins   []string `json:"allowed_origins"`
    AllowedMethods   []string `json:"allowed_methods"`
    AllowedHeaders   []string `json:"allowed_headers"`
    AllowCredentials bool     `json:"allow_credentials"`
    MaxAge           int      `json:"max_age"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enabled` | `GRAPHPOST_CORS_ENABLED` | `true` | Enable CORS |
| `allowed_origins` | `GRAPHPOST_CORS_ORIGINS` | `["*"]` | Allowed origins |
| `allowed_methods` | `GRAPHPOST_CORS_METHODS` | `["GET","POST","OPTIONS"]` | Allowed HTTP methods |
| `allowed_headers` | `GRAPHPOST_CORS_HEADERS` | `["Content-Type","Authorization","X-Admin-Secret"]` | Allowed headers |
| `allow_credentials` | `GRAPHPOST_CORS_CREDENTIALS` | `true` | Allow credentials |
| `max_age` | `GRAPHPOST_CORS_MAX_AGE` | `86400` | Preflight cache duration (seconds) |

### Production CORS Settings

```bash
# Restrict to specific origins
GRAPHPOST_CORS_ORIGINS="https://app.example.com,https://admin.example.com"

# Disable credentials for public API
GRAPHPOST_CORS_CREDENTIALS=false
```

---

## Events Configuration

```go
type EventsConfig struct {
    Enabled             bool          `json:"enabled"`
    HTTPPoolSize        int           `json:"http_pool_size"`
    FetchInterval       time.Duration `json:"fetch_interval"`
    RetryLimit          int           `json:"retry_limit"`
    RetryIntervals      []int         `json:"retry_intervals"`
    EnableManualTrigger bool          `json:"enable_manual_trigger"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enabled` | `GRAPHPOST_EVENTS_ENABLED` | `true` | Enable event triggers |
| `http_pool_size` | `GRAPHPOST_EVENTS_HTTP_POOL` | `100` | Webhook HTTP worker pool |
| `fetch_interval` | `GRAPHPOST_EVENTS_FETCH_INTERVAL` | `1s` | Event polling interval |
| `retry_limit` | `GRAPHPOST_EVENTS_RETRY_LIMIT` | `3` | Max retry attempts |
| `retry_intervals` | - | `[10,30,60]` | Retry intervals in seconds |
| `enable_manual_trigger` | `GRAPHPOST_EVENTS_MANUAL` | `true` | Allow manual trigger invocation |

---

## Console Configuration

```go
type ConsoleConfig struct {
    Enabled       bool   `json:"enabled"`
    AssetsPath    string `json:"assets_path"`
    AdminEndpoint string `json:"admin_endpoint"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enabled` | `GRAPHPOST_CONSOLE_ENABLED` | `true` | Enable admin console |
| `assets_path` | `GRAPHPOST_CONSOLE_ASSETS` | `./console` | Console assets path |
| `admin_endpoint` | `GRAPHPOST_CONSOLE_ENDPOINT` | `/console` | Console URL path |

## Logging Configuration

```go
type LoggingConfig struct {
    Level              string        `json:"level"`
    Format             string        `json:"format"`
    Output             string        `json:"output"`
    QueryLog           bool          `json:"query_log"`
    QueryLogLevel      string        `json:"query_log_level"`
    SlowQueryThreshold time.Duration `json:"slow_query_threshold"`
    SlowQueryLogLevel  string        `json:"slow_query_log_level"`
    LogQueryParams     bool          `json:"log_query_params"`
    RequestLog         bool          `json:"request_log"`
    IncludeStackTrace  bool          `json:"include_stack_trace"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `level` | `GRAPHPOST_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `format` | `GRAPHPOST_LOG_FORMAT` | `json` | Log format (json, text) |
| `output` | `GRAPHPOST_LOG_OUTPUT` | `stdout` | Log output (stdout, stderr, file path) |
| `query_log` | `GRAPHPOST_LOG_QUERIES` | `false` | Log SQL queries |
| `query_log_level` | `GRAPHPOST_LOG_QUERY_LEVEL` | `debug` | Level for query logs |
| `slow_query_threshold` | `GRAPHPOST_SLOW_QUERY_THRESHOLD` | `1s` | Duration above which queries are logged as slow |
| `slow_query_log_level` | `GRAPHPOST_SLOW_QUERY_LOG_LEVEL` | `warn` | Level for slow query logs |
| `log_query_params` | `GRAPHPOST_LOG_QUERY_PARAMS` | `false` | Include query parameters in logs |
| `request_log` | `GRAPHPOST_LOG_REQUESTS` | `true` | Log HTTP requests |
| `include_stack_trace` | `GRAPHPOST_LOG_STACK_TRACE` | `false` | Include stack traces for errors |

### Query Logging for Database Optimization

Enable query logging to identify slow queries that need optimization:

```bash
# Enable query logging
GRAPHPOST_LOG_QUERIES=true

# Set slow query threshold (queries longer than this are flagged)
GRAPHPOST_SLOW_QUERY_THRESHOLD=500ms

# Log slow queries as warnings
GRAPHPOST_SLOW_QUERY_LOG_LEVEL=warn
```

**Sample slow query log output:**
```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "warn",
  "message": "Slow query detected",
  "fields": {
    "query": "SELECT * FROM users WHERE status = $1 ORDER BY created_at",
    "duration": "1.234s",
    "duration_ms": 1234,
    "table": "users",
    "operation": "SELECT",
    "slow_query": true,
    "threshold": "1s"
  }
}
```

**Optimization suggestions provided:**
- Missing indexes detection
- Full table scan warnings
- Recommendations for partitioning large tables
- EXPLAIN ANALYZE hints

## Telemetry Configuration (OpenTelemetry)

```go
type TelemetryConfig struct {
    Enabled        bool    `json:"enabled"`
    ServiceName    string  `json:"service_name"`
    ServiceVersion string  `json:"service_version"`
    OTLPEndpoint   string  `json:"otlp_endpoint"`
    OTLPProtocol   string  `json:"otlp_protocol"`
    OTLPInsecure   bool    `json:"otlp_insecure"`
    SampleRate     float64 `json:"sample_rate"`
    TraceQueries   bool    `json:"trace_queries"`
    TraceResolvers bool    `json:"trace_resolvers"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enabled` | `GRAPHPOST_TELEMETRY_ENABLED` | `false` | Enable OpenTelemetry integration |
| `service_name` | `GRAPHPOST_TELEMETRY_SERVICE_NAME` | `graphpost` | Service name in traces |
| `service_version` | `GRAPHPOST_TELEMETRY_SERVICE_VERSION` | `1.0.0` | Service version |
| `otlp_endpoint` | `GRAPHPOST_OTLP_ENDPOINT` | `localhost:4317` | OTLP collector endpoint |
| `otlp_protocol` | `GRAPHPOST_OTLP_PROTOCOL` | `grpc` | Protocol: grpc or http |
| `otlp_insecure` | `GRAPHPOST_OTLP_INSECURE` | `true` | Disable TLS for OTLP |
| `sample_rate` | `GRAPHPOST_TELEMETRY_SAMPLE_RATE` | `1.0` | Trace sampling rate (0.0-1.0) |
| `trace_queries` | `GRAPHPOST_TELEMETRY_TRACE_QUERIES` | `true` | Trace individual DB queries |
| `trace_resolvers` | `GRAPHPOST_TELEMETRY_TRACE_RESOLVERS` | `true` | Trace GraphQL resolvers |

### OpenTelemetry Setup

**With Jaeger:**
```bash
# Start Jaeger with OTLP support
docker run -d --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4317:4317 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest

# Configure GraphPost
GRAPHPOST_TELEMETRY_ENABLED=true
GRAPHPOST_OTLP_ENDPOINT=localhost:4317
```

**With Grafana Tempo:**
```bash
GRAPHPOST_TELEMETRY_ENABLED=true
GRAPHPOST_OTLP_ENDPOINT=tempo:4317
GRAPHPOST_OTLP_PROTOCOL=grpc
```

**With Datadog:**
```bash
GRAPHPOST_TELEMETRY_ENABLED=true
GRAPHPOST_OTLP_ENDPOINT=localhost:4317
GRAPHPOST_TELEMETRY_SERVICE_NAME=my-graphql-api
```

### Trace Sampling

For high-traffic production environments:
```bash
# Sample 10% of traces
GRAPHPOST_TELEMETRY_SAMPLE_RATE=0.1

# Sample all traces (development)
GRAPHPOST_TELEMETRY_SAMPLE_RATE=1.0
```

---

## Cache Configuration

```go
type CacheConfig struct {
    Enabled            bool          `json:"enabled"`
    Backend            string        `json:"backend"`
    DefaultTTL         time.Duration `json:"default_ttl"`
    MaxSize            int           `json:"max_size"`
    QueryCacheEnabled  bool          `json:"query_cache_enabled"`
    QueryCacheTTL      time.Duration `json:"query_cache_ttl"`
    SchemaCacheEnabled bool          `json:"schema_cache_enabled"`
    SchemaCacheTTL     time.Duration `json:"schema_cache_ttl"`
    ExcludedTables     []string      `json:"excluded_tables"`
    RedisHost          string        `json:"redis_host"`
    RedisPort          int           `json:"redis_port"`
    RedisPassword      string        `json:"redis_password"`
    RedisDB            int           `json:"redis_db"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enabled` | `GRAPHPOST_CACHE_ENABLED` | `false` | Enable caching |
| `backend` | `GRAPHPOST_CACHE_BACKEND` | `memory` | Cache backend: memory, redis |
| `default_ttl` | `GRAPHPOST_CACHE_TTL` | `5m` | Default TTL for cached items |
| `max_size` | `GRAPHPOST_CACHE_MAX_SIZE` | `10000` | Max items in memory cache |
| `query_cache_enabled` | `GRAPHPOST_CACHE_QUERY_ENABLED` | `true` | Enable query result caching |
| `query_cache_ttl` | `GRAPHPOST_CACHE_QUERY_TTL` | `1m` | TTL for query results |
| `schema_cache_enabled` | `GRAPHPOST_CACHE_SCHEMA_ENABLED` | `true` | Enable schema caching |
| `schema_cache_ttl` | `GRAPHPOST_CACHE_SCHEMA_TTL` | `5m` | TTL for schema cache |
| `redis_host` | `GRAPHPOST_REDIS_HOST` | `localhost` | Redis host |
| `redis_port` | `GRAPHPOST_REDIS_PORT` | `6379` | Redis port |
| `redis_password` | `GRAPHPOST_REDIS_PASSWORD` | `` | Redis password |
| `redis_db` | `GRAPHPOST_REDIS_DB` | `0` | Redis database number |

### In-Memory Cache (Default)

```bash
# Enable caching with in-memory backend
GRAPHPOST_CACHE_ENABLED=true
GRAPHPOST_CACHE_BACKEND=memory
GRAPHPOST_CACHE_MAX_SIZE=10000
GRAPHPOST_CACHE_TTL=5m
```

### Redis Cache (Distributed)

```bash
# Enable caching with Redis backend
GRAPHPOST_CACHE_ENABLED=true
GRAPHPOST_CACHE_BACKEND=redis
GRAPHPOST_REDIS_HOST=redis.example.com
GRAPHPOST_REDIS_PORT=6379
GRAPHPOST_REDIS_PASSWORD=secret
```

### Query Result Caching

```bash
# Cache GraphQL query results for 2 minutes
GRAPHPOST_CACHE_ENABLED=true
GRAPHPOST_CACHE_QUERY_ENABLED=true
GRAPHPOST_CACHE_QUERY_TTL=2m
```

### Cache Statistics

Cache statistics are available via the `/health` endpoint:
```json
{
  "cache": {
    "enabled": true,
    "query_hits": 1250,
    "query_misses": 150,
    "query_hit_rate": 0.89,
    "cache_stats": {
      "hits": 1400,
      "misses": 200,
      "hit_rate": 0.875,
      "size": 5000,
      "max_size": 10000,
      "evictions": 50,
      "bytes_used": 2500000
    }
  }
}
```

---

## Analytics Configuration (DuckDB)

GraphPost includes a DuckDB-powered analytics engine for high-performance aggregate queries using materialized aggregates with configurable refresh strategies.

```go
type AnalyticsConfig struct {
    Enabled bool          `json:"enabled"`
    DuckDB  DuckDBConfig  `json:"duckdb"`
    Refresh RefreshConfig `json:"refresh"`
    Router  RouterConfig  `json:"router"`
}

type DuckDBConfig struct {
    Path          string `json:"path"`
    MemoryLimit   string `json:"memory_limit"`
    Threads       int    `json:"threads"`
    TempDirectory string `json:"temp_directory"`
}

type RefreshConfig struct {
    SchedulerEnabled       bool          `json:"scheduler_enabled"`
    SchedulerInterval      time.Duration `json:"scheduler_interval"`
    CDCEnabled             bool          `json:"cdc_enabled"`
    CDCPollInterval        time.Duration `json:"cdc_poll_interval"`
    LazyTTL                time.Duration `json:"lazy_ttl"`
    MaxConcurrentRefreshes int           `json:"max_concurrent_refreshes"`
    RefreshTimeout         time.Duration `json:"refresh_timeout"`
}

type RouterConfig struct {
    Enabled            bool          `json:"enabled"`
    PreferMaterialized bool          `json:"prefer_materialized"`
    FallbackToLive     bool          `json:"fallback_to_live"`
    StalenessThreshold time.Duration `json:"staleness_threshold"`
}
```

### DuckDB Configuration

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enabled` | `GRAPHPOST_ANALYTICS_ENABLED` | `false` | Enable analytics engine |
| `duckdb.path` | `GRAPHPOST_DUCKDB_PATH` | `:memory:` | Database path (`:memory:` for in-memory) |
| `duckdb.memory_limit` | `GRAPHPOST_DUCKDB_MEMORY_LIMIT` | `2GB` | Max memory for DuckDB |
| `duckdb.threads` | `GRAPHPOST_DUCKDB_THREADS` | `0` | Query threads (0 = auto) |
| `duckdb.temp_directory` | `GRAPHPOST_DUCKDB_TEMP_DIR` | `` | Temp directory for large operations |

### Refresh Strategy Configuration

GraphPost supports four refresh strategies for materialized aggregates:

| Strategy | Description | Use Case |
|----------|-------------|----------|
| **Scheduled** | Cron-based periodic refresh | Regular reporting, dashboards |
| **On-Demand** | Manual API-triggered refresh | Ad-hoc analytics, user-triggered reports |
| **CDC** | Change Data Capture - refreshes on source data changes | Near real-time analytics |
| **Lazy** | Computes on cache miss, caches result | Infrequent queries, low-latency reads |

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `refresh.scheduler_enabled` | `GRAPHPOST_ANALYTICS_SCHEDULER_ENABLED` | `true` | Enable scheduled refresh worker |
| `refresh.scheduler_interval` | `GRAPHPOST_ANALYTICS_SCHEDULER_INTERVAL` | `30s` | How often to check for pending refreshes |
| `refresh.cdc_enabled` | `GRAPHPOST_ANALYTICS_CDC_ENABLED` | `false` | Enable CDC-based refresh |
| `refresh.cdc_poll_interval` | `GRAPHPOST_ANALYTICS_CDC_POLL_INTERVAL` | `5s` | CDC polling interval |
| `refresh.lazy_ttl` | `GRAPHPOST_ANALYTICS_LAZY_TTL` | `5m` | TTL for lazy-refreshed aggregates |
| `refresh.max_concurrent_refreshes` | `GRAPHPOST_ANALYTICS_MAX_CONCURRENT_REFRESHES` | `4` | Max parallel refresh operations |
| `refresh.refresh_timeout` | `GRAPHPOST_ANALYTICS_REFRESH_TIMEOUT` | `5m` | Timeout for single refresh operation |

### Router Configuration

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `router.enabled` | `GRAPHPOST_ANALYTICS_ROUTER_ENABLED` | `true` | Enable analytics query routing |
| `router.prefer_materialized` | `GRAPHPOST_ANALYTICS_PREFER_MATERIALIZED` | `true` | Prefer materialized over live queries |
| `router.fallback_to_live` | `GRAPHPOST_ANALYTICS_FALLBACK_TO_LIVE` | `true` | Fall back to PostgreSQL if unavailable |
| `router.staleness_threshold` | `GRAPHPOST_ANALYTICS_STALENESS_THRESHOLD` | `5m` | When data is considered stale |

### Analytics Setup Examples

**Basic In-Memory Analytics:**
```bash
GRAPHPOST_ANALYTICS_ENABLED=true
GRAPHPOST_DUCKDB_PATH=:memory:
GRAPHPOST_DUCKDB_MEMORY_LIMIT=2GB
```

**Persistent Analytics with Scheduled Refresh:**
```bash
GRAPHPOST_ANALYTICS_ENABLED=true
GRAPHPOST_DUCKDB_PATH=/data/analytics.duckdb
GRAPHPOST_DUCKDB_MEMORY_LIMIT=4GB
GRAPHPOST_ANALYTICS_SCHEDULER_ENABLED=true
GRAPHPOST_ANALYTICS_SCHEDULER_INTERVAL=1m
```

**Real-Time Analytics with CDC:**
```bash
GRAPHPOST_ANALYTICS_ENABLED=true
GRAPHPOST_ANALYTICS_CDC_ENABLED=true
GRAPHPOST_ANALYTICS_CDC_POLL_INTERVAL=5s
GRAPHPOST_ANALYTICS_PREFER_MATERIALIZED=true
```

**Lazy Analytics for Infrequent Queries:**
```bash
GRAPHPOST_ANALYTICS_ENABLED=true
GRAPHPOST_ANALYTICS_SCHEDULER_ENABLED=false
GRAPHPOST_ANALYTICS_LAZY_TTL=10m
GRAPHPOST_ANALYTICS_FALLBACK_TO_LIVE=true
```

### Materialized Aggregate Definition

Aggregates can be defined via API or configuration:

```json
{
  "name": "orders_by_status",
  "source_table": "orders",
  "aggregate_type": "count",
  "group_by_columns": ["status", "region"],
  "filter_condition": "created_at > NOW() - INTERVAL '30 days'",
  "refresh_strategy": "scheduled",
  "refresh_interval": "5m"
}
```

**Supported Aggregate Types:**
- `count` - Count rows
- `count_distinct` - Count distinct values
- `sum` - Sum numeric column
- `avg` - Average numeric column
- `min` - Minimum value
- `max` - Maximum value

### Performance Benefits

| Query Type | PostgreSQL | DuckDB Analytics | Speedup |
|------------|------------|------------------|---------|
| COUNT(*) GROUP BY | 2.5s | 15ms | ~166x |
| SUM(amount) GROUP BY | 3.2s | 22ms | ~145x |
| Complex aggregates | 8.5s | 85ms | ~100x |

*Benchmarks based on 10M row dataset*

---

## Configuration File Examples

### JSON Configuration

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
    "enable_playground": true,
    "cors_origins": ["http://localhost:3000"]
  },
  "database": {
    "host": "localhost",
    "port": 5432,
    "user": "postgres",
    "password": "secret",
    "database": "myapp",
    "sslmode": "disable",
    "schema": "public",
    "pool_min_conns": 5,
    "pool_max_conns": 50
  },
  "auth": {
    "enabled": true,
    "admin_secret": "my-admin-secret",
    "jwt_secret": "my-jwt-secret",
    "jwt_claims_namespace": "https://graphpost.io/jwt/claims"
  },
  "events": {
    "enabled": true,
    "http_pool_size": 100,
    "retry_limit": 5
  },
  "console": {
    "enabled": true
  },
  "logging": {
    "level": "info",
    "format": "json",
    "output": "stdout",
    "query_log": true,
    "query_log_level": "debug",
    "slow_query_threshold": "1s",
    "slow_query_log_level": "warn",
    "log_query_params": false,
    "request_log": true
  },
  "telemetry": {
    "enabled": true,
    "service_name": "my-graphql-api",
    "service_version": "1.0.0",
    "otlp_endpoint": "localhost:4317",
    "otlp_protocol": "grpc",
    "otlp_insecure": true,
    "sample_rate": 1.0,
    "trace_queries": true,
    "trace_resolvers": true
  }
}
```

### YAML Configuration

```yaml
server:
  host: 0.0.0.0
  port: 8080
  enable_playground: true
  cors_origins:
    - http://localhost:3000

database:
  host: localhost
  port: 5432
  user: postgres
  password: secret
  database: myapp
  sslmode: disable
  schema: public
  pool_min_conns: 5
  pool_max_conns: 50

auth:
  enabled: true
  admin_secret: my-admin-secret
  jwt_secret: my-jwt-secret
  jwt_claims_namespace: https://graphpost.io/jwt/claims

events:
  enabled: true
  http_pool_size: 100
  retry_limit: 5

console:
  enabled: true

logging:
  level: info
  format: json
  output: stdout
  query_log: true
  query_log_level: debug
  slow_query_threshold: 1s
  slow_query_log_level: warn
  log_query_params: false
  request_log: true

telemetry:
  enabled: true
  service_name: my-graphql-api
  service_version: 1.0.0
  otlp_endpoint: localhost:4317
  otlp_protocol: grpc
  otlp_insecure: true
  sample_rate: 1.0
  trace_queries: true
  trace_resolvers: true
```

## Command-Line Usage

```bash
# Basic usage
graphpost --database-url "postgres://user:pass@localhost:5432/mydb"

# Full options
graphpost \
  --config /path/to/config.json \
  --host 0.0.0.0 \
  --port 8080 \
  --database-url "postgres://user:pass@localhost:5432/mydb" \
  --admin-secret "my-secret" \
  --jwt-secret "jwt-secret" \
  --enable-console \
  --enable-playground

# Show help
graphpost --help

# Show version
graphpost --version
```

## Docker Configuration

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o graphpost ./cmd/graphpost

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/graphpost .
EXPOSE 8080
CMD ["./graphpost"]
```

### Docker Compose

```yaml
version: '3.8'

services:
  graphpost:
    image: graphpost:latest
    ports:
      - "8080:8080"
    environment:
      GRAPHPOST_DATABASE_URL: postgres://postgres:password@db:5432/myapp
      GRAPHPOST_ADMIN_SECRET: my-admin-secret
      GRAPHPOST_JWT_SECRET: my-jwt-secret
    depends_on:
      - db

  db:
    image: postgres:15
    environment:
      POSTGRES_DB: myapp
      POSTGRES_PASSWORD: password
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

## Production Recommendations

### 1. Security

```bash
# Always set admin secret
GRAPHPOST_ADMIN_SECRET="strong-random-secret"

# Use SSL for database
GRAPHPOST_DB_SSLMODE=require

# Disable playground in production
GRAPHPOST_ENABLE_PLAYGROUND=false

# Set specific CORS origins
GRAPHPOST_CORS_ORIGINS="https://app.example.com"

# Limit query depth
GRAPHPOST_QUERY_DEPTH_LIMIT=10
```

### 2. Performance

```bash
# Tune connection pool
GRAPHPOST_POOL_MIN_CONNS=10
GRAPHPOST_POOL_MAX_CONNS=100
GRAPHPOST_POOL_MAX_CONN_LIFETIME=30m

# Set query timeout
GRAPHPOST_QUERY_TIMEOUT=30s
```

### 3. Monitoring & Observability

```bash
# Enable OpenTelemetry
GRAPHPOST_TELEMETRY_ENABLED=true
GRAPHPOST_OTLP_ENDPOINT=otel-collector:4317
GRAPHPOST_TELEMETRY_SAMPLE_RATE=0.1  # 10% sampling in production

# Enable query logging for optimization
GRAPHPOST_LOG_QUERIES=true
GRAPHPOST_SLOW_QUERY_THRESHOLD=500ms
GRAPHPOST_LOG_FORMAT=json
```

### 4. High Availability

```bash
# Use PgBouncer
GRAPHPOST_POOL_SIMPLE_PROTOCOL=true

# Health check configuration
GRAPHPOST_POOL_HEALTH_CHECK_PERIOD=30s
```

---

## Complete Environment Variable Reference

| Category | Variable | Default | Description |
|----------|----------|---------|-------------|
| **Server** | `GRAPHPOST_HOST` | `0.0.0.0` | Bind address |
| | `GRAPHPOST_PORT` | `8080` | Port |
| | `GRAPHPOST_ENABLE_PLAYGROUND` | `true` | GraphQL Playground |
| **Database** | `GRAPHPOST_DATABASE_URL` | - | Connection URL |
| | `GRAPHPOST_DB_HOST` | `localhost` | Host |
| | `GRAPHPOST_DB_PORT` | `5432` | Port |
| | `GRAPHPOST_DB_USER` | `postgres` | User |
| | `GRAPHPOST_DB_PASSWORD` | - | Password |
| | `GRAPHPOST_DB_NAME` | `postgres` | Database |
| | `GRAPHPOST_DB_SSLMODE` | `disable` | SSL mode |
| **Pool** | `GRAPHPOST_POOL_MIN_CONNS` | `5` | Min connections |
| | `GRAPHPOST_POOL_MAX_CONNS` | `50` | Max connections |
| | `GRAPHPOST_POOL_MAX_CONN_LIFETIME` | `1h` | Connection lifetime |
| | `GRAPHPOST_POOL_SIMPLE_PROTOCOL` | `false` | PgBouncer mode |
| **Auth** | `GRAPHPOST_ADMIN_SECRET` | - | Admin secret |
| | `GRAPHPOST_JWT_SECRET` | - | JWT secret |
| | `GRAPHPOST_JWT_CLAIMS_NS` | `https://graphpost.io/jwt/claims` | Claims namespace |
| **GraphQL** | `GRAPHPOST_ENABLE_QUERIES` | `true` | Enable queries |
| | `GRAPHPOST_ENABLE_MUTATIONS` | `true` | Enable mutations |
| | `GRAPHPOST_ENABLE_SUBSCRIPTIONS` | `true` | Enable subscriptions |
| | `GRAPHPOST_QUERY_DEPTH_LIMIT` | `0` | Max depth |
| | `GRAPHPOST_QUERY_TIMEOUT` | `0` | Query timeout |
| **Telemetry** | `GRAPHPOST_TELEMETRY_ENABLED` | `false` | Enable OTEL |
| | `GRAPHPOST_OTLP_ENDPOINT` | `localhost:4317` | Collector endpoint |
| | `GRAPHPOST_OTLP_PROTOCOL` | `grpc` | Protocol |
| | `GRAPHPOST_TELEMETRY_SAMPLE_RATE` | `1.0` | Sample rate |
| | `GRAPHPOST_TELEMETRY_TRACE_QUERIES` | `true` | Trace queries |
| **Logging** | `GRAPHPOST_LOG_LEVEL` | `info` | Log level |
| | `GRAPHPOST_LOG_FORMAT` | `json` | Log format |
| | `GRAPHPOST_LOG_QUERIES` | `false` | Query logging |
| | `GRAPHPOST_SLOW_QUERY_THRESHOLD` | `1s` | Slow query threshold |
| | `GRAPHPOST_LOG_QUERY_PARAMS` | `false` | Log parameters |
| **Cache** | `GRAPHPOST_CACHE_ENABLED` | `false` | Enable caching |
| | `GRAPHPOST_CACHE_BACKEND` | `memory` | Backend (memory/redis) |
| | `GRAPHPOST_CACHE_TTL` | `5m` | Default TTL |
| | `GRAPHPOST_CACHE_MAX_SIZE` | `10000` | Max cache items |
| | `GRAPHPOST_REDIS_HOST` | `localhost` | Redis host |
| | `GRAPHPOST_REDIS_PORT` | `6379` | Redis port |
| **Analytics** | `GRAPHPOST_ANALYTICS_ENABLED` | `false` | Enable DuckDB analytics |
| | `GRAPHPOST_DUCKDB_PATH` | `:memory:` | Database path |
| | `GRAPHPOST_DUCKDB_MEMORY_LIMIT` | `2GB` | Memory limit |
| | `GRAPHPOST_DUCKDB_THREADS` | `0` | Query threads |
| | `GRAPHPOST_ANALYTICS_SCHEDULER_ENABLED` | `true` | Enable scheduler |
| | `GRAPHPOST_ANALYTICS_CDC_ENABLED` | `false` | Enable CDC refresh |
| | `GRAPHPOST_ANALYTICS_LAZY_TTL` | `5m` | Lazy refresh TTL |
| | `GRAPHPOST_ANALYTICS_PREFER_MATERIALIZED` | `true` | Prefer materialized |
| | `GRAPHPOST_ANALYTICS_FALLBACK_TO_LIVE` | `true` | Fallback to PostgreSQL |
