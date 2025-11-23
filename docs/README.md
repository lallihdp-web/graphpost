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
│   ├── resolver/
│   │   └── resolver.go          # GraphQL query resolution
│   ├── schema/
│   │   └── generator.go         # GraphQL schema generation
│   └── subscription/
│       └── manager.go           # Real-time subscriptions
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

## Technology Stack

| Component | Technology | Purpose |
|-----------|------------|---------|
| Language | Go 1.21+ | High performance, concurrency |
| Database Driver | pgx v5 | PostgreSQL with connection pooling |
| GraphQL | graphql-go | GraphQL implementation |
| WebSocket | gorilla/websocket | Real-time subscriptions |
| HTTP | net/http | API server |

## Key Features

- **Instant GraphQL API**: Auto-generates GraphQL schema from PostgreSQL
- **Real-time Subscriptions**: Live queries via WebSocket
- **Authentication**: Admin secret, JWT, and webhook auth
- **Event Triggers**: Webhook notifications on data changes
- **Connection Pooling**: Efficient database connection management
- **Schema Introspection**: Dynamic schema updates
- **Hasura Compatible**: Drop-in replacement for many use cases

## Version History

| Version | Changes |
|---------|---------|
| 1.0.0 | Initial release with full GraphQL support |

## License

MIT License
