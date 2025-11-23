# GraphPost Technical Documentation

GraphPost is a high-performance GraphQL server for PostgreSQL databases, providing instant GraphQL APIs with real-time subscriptions, authentication, and event triggers. It is designed to be Hasura-compatible while offering enhanced performance through the pgx PostgreSQL driver.

## Table of Contents

1. [Architecture Overview](./architecture.md)
2. [Database Connection & Pooling](./database.md)
3. [GraphQL Schema Generation](./schema.md)
4. [Query Resolution](./resolver.md)
5. [Authentication & Authorization](./authentication.md)
6. [Real-time Subscriptions](./subscriptions.md)
7. [Event Triggers](./events.md)
8. [Configuration Reference](./configuration.md)
9. [API Reference](./api.md)
10. [Performance & Benchmarks](./benchmarks.md)
11. [Observability (Telemetry & Logging)](#observability)

## Quick Start

```bash
# Using environment variable
export GRAPHPOST_DATABASE_URL="postgres://user:password@localhost:5432/mydb"
./graphpost

# Using command line flags
./graphpost --database-url "postgres://user:password@localhost:5432/mydb" --port 8080

# Using config file
./graphpost --config config.json
```

## Project Structure

```
graphpost/
├── cmd/
│   └── graphpost/
│       └── main.go              # Application entry point
├── internal/
│   ├── auth/
│   │   └── auth.go              # Authentication & authorization
│   ├── config/
│   │   └── config.go            # Configuration management
│   ├── console/
│   │   └── console.go           # Admin console UI
│   ├── database/
│   │   ├── connection.go        # PostgreSQL connection (pgx)
│   │   └── introspector.go      # Database schema introspection
│   ├── engine/
│   │   ├── engine.go            # Main GraphQL engine
│   │   └── server.go            # HTTP server
│   ├── events/
│   │   └── triggers.go          # Event trigger management
│   ├── logging/
│   │   └── logging.go           # Query logging & slow query detection
│   ├── resolver/
│   │   └── resolver.go          # GraphQL query resolution
│   ├── schema/
│   │   └── generator.go         # GraphQL schema generation
│   ├── subscription/
│   │   └── manager.go           # Real-time subscriptions
│   ├── telemetry/
│   │   └── telemetry.go         # OpenTelemetry integration
│   ├── cache/
│   │   ├── cache.go             # Cache manager
│   │   ├── memory.go            # In-memory LRU cache
│   │   └── redis.go             # Redis cache backend
│   └── analytics/
│       ├── duckdb.go            # DuckDB connection & queries
│       ├── materialized.go      # Materialized aggregate store
│       ├── refresh.go           # Refresh strategy implementations
│       └── router.go            # Query routing logic
├── benchmarks/
│   ├── benchmark_test.go        # Performance benchmarks
│   └── comparison.md            # Hasura comparison
└── docs/
    └── *.md                     # Documentation files
```

## Core Components

### 1. Engine (`internal/engine/`)
The central orchestrator that initializes and coordinates all components.

### 2. Database (`internal/database/`)
Handles PostgreSQL connectivity using pgx with connection pooling and schema introspection.

### 3. Schema (`internal/schema/`)
Generates GraphQL schema dynamically from database structure.

### 4. Resolver (`internal/resolver/`)
Translates GraphQL queries into SQL and executes them.

### 5. Auth (`internal/auth/`)
Manages authentication via admin secret, JWT tokens, and webhooks.

### 6. Subscription (`internal/subscription/`)
Provides real-time data using PostgreSQL LISTEN/NOTIFY.

### 7. Events (`internal/events/`)
Manages event triggers with webhook delivery.

### 8. Telemetry (`internal/telemetry/`)
OpenTelemetry integration for distributed tracing and metrics. Supports OTLP export to Jaeger, Grafana Tempo, Datadog, and other collectors.

### 9. Logging (`internal/logging/`)
Structured logging with query logging and slow query detection. Provides optimization suggestions for database performance tuning.

### 10. Cache (`internal/cache/`)
Multi-tier caching with in-memory LRU and Redis backends. Supports query result caching and schema caching.

### 11. Analytics (`internal/analytics/`)
DuckDB-powered analytics engine for high-performance aggregate queries using materialized aggregates with configurable refresh strategies (Scheduled, On-Demand, CDC, Lazy).

## Technology Stack

| Component | Technology | Purpose |
|-----------|------------|---------|
| Language | Go 1.21+ | High performance, concurrency |
| Database Driver | pgx v5 | PostgreSQL with connection pooling |
| GraphQL | graphql-go | GraphQL implementation |
| WebSocket | gorilla/websocket | Real-time subscriptions |
| HTTP | net/http | API server |
| Telemetry | OpenTelemetry | Distributed tracing & metrics |
| Analytics | DuckDB | OLAP queries & materialized aggregates |
| Cache | Redis / Memory | Query result & schema caching |

## Key Features

- **Instant GraphQL API**: Auto-generates GraphQL schema from PostgreSQL
- **Real-time Subscriptions**: Live queries via WebSocket
- **Authentication**: Admin secret, JWT, and webhook auth
- **Event Triggers**: Webhook notifications on data changes
- **Connection Pooling**: Efficient database connection management (pgx)
- **Schema Introspection**: Dynamic schema updates
- **Hasura Compatible**: Drop-in replacement for many use cases
- **OpenTelemetry**: Distributed tracing with Jaeger, Tempo, Datadog support
- **Query Logging**: SQL query logging with slow query detection
- **Database Optimization**: Automatic suggestions for indexes and partitioning
- **Multi-tier Caching**: In-memory LRU and Redis cache backends
- **DuckDB Analytics**: High-performance OLAP queries with materialized aggregates
- **Configurable Refresh Strategies**: Scheduled, On-Demand, CDC, and Lazy refresh

## Observability

### OpenTelemetry Integration

```bash
# Enable telemetry
GRAPHPOST_TELEMETRY_ENABLED=true
GRAPHPOST_OTLP_ENDPOINT=localhost:4317
GRAPHPOST_TELEMETRY_SAMPLE_RATE=1.0
```

Supported backends:
- Jaeger
- Grafana Tempo
- Datadog
- Any OTLP-compatible collector

### Query Logging

```bash
# Enable query logging for database optimization
GRAPHPOST_LOG_QUERIES=true
GRAPHPOST_SLOW_QUERY_THRESHOLD=500ms
```

Slow query logs include:
- Query duration
- SQL statement
- Optimization suggestions (indexes, partitioning)
- EXPLAIN ANALYZE hints

## Analytics (DuckDB)

GraphPost includes a DuckDB-powered analytics engine for high-performance aggregate queries.

### Enable Analytics

```bash
GRAPHPOST_ANALYTICS_ENABLED=true
GRAPHPOST_DUCKDB_PATH=:memory:
GRAPHPOST_DUCKDB_MEMORY_LIMIT=2GB
```

### Refresh Strategies

| Strategy | Environment Variable | Description |
|----------|---------------------|-------------|
| Scheduled | `GRAPHPOST_ANALYTICS_SCHEDULER_ENABLED=true` | Cron-based periodic refresh |
| On-Demand | API call | Manual refresh via API |
| CDC (Polling) | `GRAPHPOST_ANALYTICS_CDC_MODE=polling` | Polls pg_stat for changes (~5s latency) |
| CDC (Realtime) | `GRAPHPOST_ANALYTICS_CDC_MODE=realtime` | LISTEN/NOTIFY triggers (<1s latency) |
| CDC (Both) | `GRAPHPOST_ANALYTICS_CDC_MODE=both` | Redundant: realtime + polling fallback |
| Lazy | `GRAPHPOST_ANALYTICS_LAZY_TTL=5m` | Compute on cache miss |

### Performance Benefits

| Query Type | PostgreSQL | DuckDB Analytics | Speedup |
|------------|------------|------------------|---------|
| COUNT(*) GROUP BY | 2.5s | 15ms | ~166x |
| SUM(amount) GROUP BY | 3.2s | 22ms | ~145x |
| Complex aggregates | 8.5s | 85ms | ~100x |

*Benchmarks based on 10M row dataset*

## Version History

| Version | Changes |
|---------|---------|
| 1.3.1 | CDC mode selection: polling, realtime (LISTEN/NOTIFY), or both |
| 1.3.0 | DuckDB analytics engine with materialized aggregates and configurable refresh strategies |
| 1.2.1 | Multi-tier caching (in-memory LRU + Redis) |
| 1.2.0 | OpenTelemetry integration, query logging with slow query detection |
| 1.1.0 | GraphQL operation enable/disable, pgx connection pool configuration |
| 1.0.0 | Initial release with full GraphQL support |

## License

MIT License
