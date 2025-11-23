# GraphPost

**Instant GraphQL API for PostgreSQL** - A Hasura-compatible GraphQL engine written in Go.

GraphPost automatically introspects your PostgreSQL database and generates a full GraphQL API with queries, mutations, subscriptions, filtering, sorting, pagination, aggregations, and more.

## Features

### Core Features
- **Automatic Schema Generation**: Introspects PostgreSQL schema and generates GraphQL types
- **Full CRUD Operations**: Auto-generated queries and mutations for all tables
- **Relationships**: Automatic relationship detection from foreign keys
- **Real-time Subscriptions**: WebSocket-based subscriptions for live data
- **Aggregations**: Count, sum, avg, min, max, stddev, variance queries
- **Advanced Filtering**: Boolean expressions with `_and`, `_or`, `_not`
- **Sorting & Pagination**: `order_by`, `limit`, `offset`, `distinct_on`

### Hasura Compatibility
- Compatible GraphQL API structure
- Same filtering syntax (`_eq`, `_gt`, `_like`, `_in`, etc.)
- Same mutation patterns (`insert_`, `update_`, `delete_`)
- Same subscription patterns
- Metadata API compatibility

### Authentication & Authorization
- Admin secret authentication
- JWT authentication (HS256, RS256)
- Webhook authentication
- Role-based access control
- Row-level security
- Column-level permissions

### Event System
- Event triggers on INSERT/UPDATE/DELETE
- Webhook delivery with retries
- Event logs and redelivery
- Manual trigger invocation

### Developer Experience
- GraphQL Playground
- Admin Console
- Schema introspection
- Health checks
- Hot reload

## Quick Start

### Using Docker

```bash
docker run -d \
  -e GRAPHPOST_DATABASE_URL=postgres://user:pass@host:5432/dbname \
  -e GRAPHPOST_ADMIN_SECRET=mysecret \
  -p 8080:8080 \
  graphpost/graphpost
```

### Using Binary

```bash
# Download and install
go install github.com/graphpost/graphpost/cmd/graphpost@latest

# Run with database URL
graphpost --database-url postgres://user:pass@localhost:5432/mydb
```

### Using Go

```bash
# Clone the repository
git clone https://github.com/graphpost/graphpost.git
cd graphpost

# Build
go build -o graphpost ./cmd/graphpost

# Run
./graphpost --database-url postgres://localhost/mydb
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GRAPHPOST_DATABASE_URL` | PostgreSQL connection URL | - |
| `GRAPHPOST_HOST` | Server host | `0.0.0.0` |
| `GRAPHPOST_PORT` | Server port | `8080` |
| `GRAPHPOST_ADMIN_SECRET` | Admin secret for authentication | - |
| `GRAPHPOST_JWT_SECRET` | JWT secret for token validation | - |

### Command Line Flags

```bash
graphpost [flags]

Flags:
  --config string        Path to configuration file
  --host string          Server host (default "0.0.0.0")
  --port int             Server port (default 8080)
  --database-url string  PostgreSQL database URL
  --admin-secret string  Admin secret for authentication
  --jwt-secret string    JWT secret for authentication
  --enable-console       Enable admin console (default true)
  --enable-playground    Enable GraphQL Playground (default true)
  --version              Show version
  --help                 Show help
```

### Configuration File

Create a `config.json` file:

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
    "enable_playground": true
  },
  "database": {
    "host": "localhost",
    "port": 5432,
    "user": "postgres",
    "password": "postgres",
    "database": "mydb",
    "schema": "public"
  },
  "auth": {
    "enabled": true,
    "admin_secret": "mysecret",
    "jwt_secret": "myjwtsecret"
  }
}
```

## API Reference

### GraphQL Endpoint

```
POST /v1/graphql
```

### Query Examples

```graphql
# Fetch all users
query {
  users {
    id
    name
    email
    posts {
      title
    }
  }
}

# Fetch with filtering
query {
  users(where: { name: { _like: "%John%" } }) {
    id
    name
  }
}

# Fetch with pagination
query {
  users(limit: 10, offset: 0, order_by: [{ created_at: desc }]) {
    id
    name
  }
}

# Aggregations
query {
  users_aggregate {
    aggregate {
      count
      max { created_at }
    }
  }
}

# Fetch by primary key
query {
  users_by_pk(id: 1) {
    id
    name
    email
  }
}
```

### Mutation Examples

```graphql
# Insert single
mutation {
  insert_users_one(object: { name: "John", email: "john@example.com" }) {
    id
    name
  }
}

# Insert multiple
mutation {
  insert_users(objects: [
    { name: "John", email: "john@example.com" },
    { name: "Jane", email: "jane@example.com" }
  ]) {
    affected_rows
    returning {
      id
      name
    }
  }
}

# Update by primary key
mutation {
  update_users_by_pk(pk_columns: { id: 1 }, _set: { name: "John Doe" }) {
    id
    name
  }
}

# Update multiple
mutation {
  update_users(where: { active: { _eq: false } }, _set: { active: true }) {
    affected_rows
  }
}

# Delete by primary key
mutation {
  delete_users_by_pk(id: 1) {
    id
  }
}

# Delete multiple
mutation {
  delete_users(where: { active: { _eq: false } }) {
    affected_rows
  }
}
```

### Subscription Examples

```graphql
# Subscribe to table changes
subscription {
  users {
    id
    name
  }
}

# Subscribe to single row
subscription {
  users_by_pk(id: 1) {
    id
    name
    status
  }
}
```

## Filtering Operators

| Operator | Description |
|----------|-------------|
| `_eq` | Equal to |
| `_neq` | Not equal to |
| `_gt` | Greater than |
| `_gte` | Greater than or equal |
| `_lt` | Less than |
| `_lte` | Less than or equal |
| `_in` | In array |
| `_nin` | Not in array |
| `_is_null` | Is null |
| `_like` | LIKE pattern |
| `_nlike` | NOT LIKE pattern |
| `_ilike` | Case-insensitive LIKE |
| `_similar` | SIMILAR TO pattern |
| `_regex` | POSIX regex |
| `_contains` | JSONB contains |
| `_has_key` | JSONB has key |

## Authentication

### Admin Secret

```bash
# Set admin secret
export GRAPHPOST_ADMIN_SECRET=mysecret

# Use in requests
curl -X POST http://localhost:8080/v1/graphql \
  -H "X-Hasura-Admin-Secret: mysecret" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ users { id } }"}'
```

### JWT Authentication

```bash
# Set JWT secret
export GRAPHPOST_JWT_SECRET=myjwtsecret

# Use in requests
curl -X POST http://localhost:8080/v1/graphql \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ users { id } }"}'
```

JWT Payload format:
```json
{
  "sub": "user-id",
  "https://hasura.io/jwt/claims": {
    "x-hasura-default-role": "user",
    "x-hasura-allowed-roles": ["user", "admin"],
    "x-hasura-user-id": "user-id"
  }
}
```

## Metadata API

GraphPost supports a Hasura-compatible Metadata API for managing the schema:

```bash
# Export metadata
curl -X POST http://localhost:8080/v1/metadata \
  -H "X-Hasura-Admin-Secret: mysecret" \
  -d '{"type": "export_metadata", "args": {}}'

# Reload metadata
curl -X POST http://localhost:8080/v1/metadata \
  -H "X-Hasura-Admin-Secret: mysecret" \
  -d '{"type": "reload_metadata", "args": {}}'

# Track table
curl -X POST http://localhost:8080/v1/metadata \
  -H "X-Hasura-Admin-Secret: mysecret" \
  -d '{"type": "track_table", "args": {"table": "users", "schema": "public"}}'
```

## Event Triggers

Create event triggers to capture database changes:

```bash
curl -X POST http://localhost:8080/v1/metadata \
  -H "X-Hasura-Admin-Secret: mysecret" \
  -d '{
    "type": "create_event_trigger",
    "args": {
      "name": "user_created",
      "table": "users",
      "schema": "public",
      "operations": ["INSERT"],
      "webhook_url": "http://myapp.com/webhook",
      "retry_config": {
        "num_retries": 3,
        "retry_interval_seconds": 10
      }
    }
  }'
```

## Project Structure

```
graphpost/
├── cmd/
│   └── graphpost/          # CLI entrypoint
├── internal/
│   ├── config/             # Configuration handling
│   ├── database/           # PostgreSQL introspection & connection
│   ├── schema/             # GraphQL schema generation
│   ├── resolver/           # Query/mutation resolution
│   ├── subscription/       # Real-time subscriptions
│   ├── auth/               # Authentication & authorization
│   ├── events/             # Event triggers
│   ├── engine/             # Main engine & HTTP server
│   └── console/            # Admin console
├── pkg/
│   ├── types/              # Shared types
│   └── utils/              # Utilities
├── go.mod
├── go.sum
├── config.sample.json
└── README.md
```

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting PRs.

## License

MIT License - see LICENSE file for details.

## Acknowledgments

- Inspired by [Hasura](https://hasura.io/)
- Built with [graphql-go](https://github.com/graphql-go/graphql)
- PostgreSQL driver by [lib/pq](https://github.com/lib/pq)
