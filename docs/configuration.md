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
    Server   ServerConfig   `json:"server"`
    Database DatabaseConfig `json:"database"`
    Auth     AuthConfig     `json:"auth"`
    Events   EventsConfig   `json:"events"`
    Console  ConsoleConfig  `json:"console"`
    Logging  LoggingConfig  `json:"logging"`
}
```

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

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `host` | `GRAPHPOST_DB_HOST` | `localhost` | PostgreSQL host |
| `port` | `GRAPHPOST_DB_PORT` | `5432` | PostgreSQL port |
| `user` | `GRAPHPOST_DB_USER` | `postgres` | Database user |
| `password` | `GRAPHPOST_DB_PASSWORD` | `` | Database password |
| `database` | `GRAPHPOST_DB_NAME` | `postgres` | Database name |
| `sslmode` | `GRAPHPOST_DB_SSLMODE` | `disable` | SSL mode |
| `schema` | `GRAPHPOST_DB_SCHEMA` | `public` | Default schema |
| `max_open_conns` | `GRAPHPOST_DB_MAX_OPEN_CONNS` | `50` | Maximum open connections |
| `max_idle_conns` | `GRAPHPOST_DB_MAX_IDLE_CONNS` | `10` | Maximum idle connections |
| `pool_min_conns` | `GRAPHPOST_DB_POOL_MIN_CONNS` | `5` | Minimum pool connections |
| `pool_max_conns` | `GRAPHPOST_DB_POOL_MAX_CONNS` | `50` | Maximum pool connections |

### Database URL

Alternative to individual parameters:

```bash
GRAPHPOST_DATABASE_URL="postgres://user:password@host:5432/database?sslmode=disable&search_path=public"
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

## Events Configuration

```go
type EventsConfig struct {
    Enabled             bool          `json:"enabled"`
    HTTPPoolSize        int           `json:"http_pool_size"`
    FetchInterval       time.Duration `json:"fetch_interval"`
    RetryLimit          int           `json:"retry_limit"`
    EnableManualTrigger bool          `json:"enable_manual_trigger"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `enabled` | `GRAPHPOST_EVENTS_ENABLED` | `true` | Enable event triggers |
| `http_pool_size` | `GRAPHPOST_EVENTS_HTTP_POOL` | `100` | Webhook HTTP worker pool |
| `fetch_interval` | `GRAPHPOST_EVENTS_FETCH_INTERVAL` | `1s` | Event polling interval |
| `retry_limit` | `GRAPHPOST_EVENTS_RETRY_LIMIT` | `5` | Max retry attempts |
| `enable_manual_trigger` | `GRAPHPOST_EVENTS_MANUAL` | `false` | Allow manual trigger invocation |

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
    Level       string `json:"level"`
    Format      string `json:"format"`
    QueryLog    bool   `json:"query_log"`
    RequestLog  bool   `json:"request_log"`
}
```

| Parameter | Environment Variable | Default | Description |
|-----------|---------------------|---------|-------------|
| `level` | `GRAPHPOST_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `format` | `GRAPHPOST_LOG_FORMAT` | `json` | Log format (json, text) |
| `query_log` | `GRAPHPOST_LOG_QUERIES` | `false` | Log SQL queries |
| `request_log` | `GRAPHPOST_LOG_REQUESTS` | `true` | Log HTTP requests |

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
    "query_log": false,
    "request_log": true
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
  query_log: false
  request_log: true
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

1. **Security**
   - Always set `GRAPHPOST_ADMIN_SECRET`
   - Use `sslmode=require` for database
   - Disable playground in production
   - Set specific CORS origins

2. **Performance**
   - Tune `pool_min_conns` and `pool_max_conns`
   - Set appropriate timeouts
   - Enable connection pooling

3. **Monitoring**
   - Enable request logging
   - Use JSON log format for parsing
   - Monitor pool statistics

4. **High Availability**
   - Use connection pooler (PgBouncer)
   - Configure health check endpoints
   - Set up load balancing
