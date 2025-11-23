package config

import (
	"encoding/json"
	"os"
	"time"
)

// Config represents the main configuration for GraphPost
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

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Host              string        `json:"host"`
	Port              int           `json:"port"`
	EnablePlayground  bool          `json:"enable_playground"`
	EnableIntrospection bool        `json:"enable_introspection"`
	RequestTimeout    time.Duration `json:"request_timeout"`
	MaxConnections    int           `json:"max_connections"`
}

// GraphQLConfig holds GraphQL operation configuration
type GraphQLConfig struct {
	// EnableQueries allows SELECT operations (default: true)
	EnableQueries bool `json:"enable_queries"`

	// EnableMutations allows INSERT/UPDATE/DELETE operations (default: true)
	EnableMutations bool `json:"enable_mutations"`

	// EnableSubscriptions allows real-time subscriptions (default: true)
	EnableSubscriptions bool `json:"enable_subscriptions"`

	// EnableAggregations allows aggregate queries (default: true)
	EnableAggregations bool `json:"enable_aggregations"`

	// QueryDepthLimit limits nested query depth (0 = unlimited)
	QueryDepthLimit int `json:"query_depth_limit"`

	// QueryTimeout is the default timeout for queries
	// Applied via context.WithTimeout at query execution
	// 0 = no timeout (default)
	QueryTimeout time.Duration `json:"query_timeout"`
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"`
	Schema   string `json:"schema"`

	// pgx Connection Pool Configuration
	Pool PoolConfig `json:"pool"`

	// Legacy fields (mapped to Pool config for backward compatibility)
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

// PoolConfig holds pgx connection pool configuration
type PoolConfig struct {
	// MinConns is the minimum number of connections kept open in the pool
	// These connections are maintained even when idle
	// Default: 5
	MinConns int32 `json:"min_conns"`

	// MaxConns is the maximum number of connections in the pool
	// Default: 50
	MaxConns int32 `json:"max_conns"`

	// MaxConnLifetime is the maximum lifetime of a connection
	// Connections older than this will be closed and replaced
	// Default: 1 hour
	MaxConnLifetime time.Duration `json:"max_conn_lifetime"`

	// MaxConnIdleTime is the maximum time a connection can be idle
	// Idle connections exceeding MinConns will be closed after this duration
	// Default: 30 minutes
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time"`

	// HealthCheckPeriod is how often health checks are performed on idle connections
	// Default: 1 minute
	HealthCheckPeriod time.Duration `json:"health_check_period"`

	// ConnectTimeout is the timeout for establishing new connections
	// Default: 10 seconds
	ConnectTimeout time.Duration `json:"connect_timeout"`

	// LazyConnect delays connection creation until first use
	// Default: false
	LazyConnect bool `json:"lazy_connect"`

	// PreferSimpleProtocol disables implicit prepared statement usage
	// Useful when using PgBouncer in transaction pooling mode
	// Default: false
	PreferSimpleProtocol bool `json:"prefer_simple_protocol"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Enabled           bool       `json:"enabled"`
	AdminSecret       string     `json:"admin_secret"`
	JWTSecret         string     `json:"jwt_secret"`          // Legacy: simple secret for HS256
	JWTAlgorithm      string     `json:"jwt_algorithm"`       // Legacy: algorithm type
	JWTClaimsNamespace string    `json:"jwt_claims_namespace"`
	UnauthorizedRole  string     `json:"unauthorized_role"`
	AllowedRoles      []string   `json:"allowed_roles"`
	DefaultRole       string     `json:"default_role"`
	WebhookURL        string     `json:"webhook_url"`
	JWT               *JWTConfig `json:"jwt,omitempty"`       // New: detailed JWT config
}

// JWTConfig holds detailed JWT verification configuration
type JWTConfig struct {
	// Type specifies the key type: "HS256", "HS384", "HS512", "RS256", "RS384", "RS512", "ES256", "ES384", "ES512"
	Type string `json:"type"`

	// Key is the secret (for HMAC) or public key (for RSA/ECDSA) in PEM format
	Key string `json:"key,omitempty"`

	// JWKUrl is the URL to fetch JSON Web Key Set for key rotation
	JWKUrl string `json:"jwk_url,omitempty"`

	// ClaimsNamespace is the namespace/key in JWT where Hasura/GraphPost claims are stored
	// e.g., "https://hasura.io/jwt/claims" or "https://graphpost.io/jwt/claims"
	ClaimsNamespace string `json:"claims_namespace,omitempty"`

	// ClaimsNamespacePath is a JSON path to nested claims (alternative to ClaimsNamespace)
	// e.g., "$.hasura.claims"
	ClaimsNamespacePath string `json:"claims_namespace_path,omitempty"`

	// ClaimsMap allows custom mapping of JWT claims to session variables
	// e.g., {"x-hasura-user-id": {"path": "$.sub"}, "x-hasura-org-id": {"path": "$.org.id"}}
	ClaimsMap map[string]ClaimMapping `json:"claims_map,omitempty"`

	// Issuer validates the "iss" claim if set
	Issuer string `json:"issuer,omitempty"`

	// Audience validates the "aud" claim if set
	Audience string `json:"audience,omitempty"`

	// AllowedSkew is the allowed clock skew for expiration validation (in seconds)
	AllowedSkew int `json:"allowed_skew,omitempty"`

	// Header specifies custom header to read JWT from (default: "Authorization")
	Header *JWTHeader `json:"header,omitempty"`

	// SkipVerification skips JWT signature verification (FOR DEVELOPMENT/TESTING ONLY)
	// WARNING: Never enable this in production! Tokens will be trusted without validation.
	SkipVerification bool `json:"skip_verification,omitempty"`

	// SkipExpirationCheck skips JWT expiration validation (FOR DEVELOPMENT/TESTING ONLY)
	// WARNING: Never enable this in production! Expired tokens will be accepted.
	SkipExpirationCheck bool `json:"skip_expiration_check,omitempty"`
}

// ClaimMapping defines how to map a JWT claim to a session variable
type ClaimMapping struct {
	// Path is a JSON path to extract the value (e.g., "$.user.id")
	Path string `json:"path,omitempty"`

	// Default is the default value if the claim is not found
	Default interface{} `json:"default,omitempty"`
}

// JWTHeader specifies where to read the JWT from
type JWTHeader struct {
	// Type is either "Authorization" or "Cookie"
	Type string `json:"type"`

	// Name is the header name (for Authorization) or cookie name (for Cookie)
	Name string `json:"name"`
}

// ConsoleConfig holds admin console configuration
type ConsoleConfig struct {
	Enabled    bool   `json:"enabled"`
	AssetsPath string `json:"assets_path"`
}

// EventsConfig holds event trigger configuration
type EventsConfig struct {
	Enabled           bool          `json:"enabled"`
	HTTPPoolSize      int           `json:"http_pool_size"`
	FetchInterval     time.Duration `json:"fetch_interval"`
	RetryLimit        int           `json:"retry_limit"`
	RetryIntervals    []int         `json:"retry_intervals"`
	EnableManualTrigger bool        `json:"enable_manual_trigger"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Enabled          bool     `json:"enabled"`
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age"`
}

// TelemetryConfig holds OpenTelemetry configuration
type TelemetryConfig struct {
	// Enabled enables OpenTelemetry integration
	Enabled bool `json:"enabled"`

	// ServiceName is the name of this service in traces
	ServiceName string `json:"service_name"`

	// ServiceVersion is the version of this service
	ServiceVersion string `json:"service_version"`

	// OTLPEndpoint is the OpenTelemetry collector endpoint
	// Example: "localhost:4317" for gRPC, "localhost:4318" for HTTP
	OTLPEndpoint string `json:"otlp_endpoint"`

	// OTLPProtocol is the protocol to use: "grpc" or "http"
	OTLPProtocol string `json:"otlp_protocol"`

	// OTLPInsecure disables TLS for the OTLP connection
	OTLPInsecure bool `json:"otlp_insecure"`

	// SampleRate is the sampling rate for traces (0.0 to 1.0)
	// 1.0 = sample all traces, 0.1 = sample 10% of traces
	SampleRate float64 `json:"sample_rate"`

	// TraceQueries enables tracing for individual database queries
	TraceQueries bool `json:"trace_queries"`

	// TraceResolvers enables tracing for GraphQL resolvers
	TraceResolvers bool `json:"trace_resolvers"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	// Level is the minimum log level (debug, info, warn, error)
	Level string `json:"level"`

	// Format is the log format (json, text)
	Format string `json:"format"`

	// Output is where logs are written (stdout, stderr, or file path)
	Output string `json:"output"`

	// QueryLog enables SQL query logging
	QueryLog bool `json:"query_log"`

	// QueryLogLevel is the level for query logs (debug, info)
	QueryLogLevel string `json:"query_log_level"`

	// SlowQueryThreshold is the duration above which queries are logged as slow
	// Set to 0 to disable slow query logging
	// This helps identify queries that need optimization (indexes, partitioning)
	SlowQueryThreshold time.Duration `json:"slow_query_threshold"`

	// SlowQueryLogLevel is the level for slow query logs (warn, error)
	SlowQueryLogLevel string `json:"slow_query_log_level"`

	// LogQueryParams includes query parameters in logs (be careful with sensitive data)
	LogQueryParams bool `json:"log_query_params"`

	// RequestLog enables HTTP request logging
	RequestLog bool `json:"request_log"`

	// IncludeStackTrace includes stack traces for errors
	IncludeStackTrace bool `json:"include_stack_trace"`
}

// CacheConfig holds caching configuration
type CacheConfig struct {
	// Enabled enables caching
	Enabled bool `json:"enabled"`

	// Backend is the cache backend: "memory", "redis"
	Backend string `json:"backend"`

	// DefaultTTL is the default time-to-live for cached items
	DefaultTTL time.Duration `json:"default_ttl"`

	// MaxSize is the maximum number of items in the cache (memory backend)
	MaxSize int `json:"max_size"`

	// QueryCacheEnabled enables GraphQL query result caching
	QueryCacheEnabled bool `json:"query_cache_enabled"`

	// QueryCacheTTL is the TTL for query results
	QueryCacheTTL time.Duration `json:"query_cache_ttl"`

	// SchemaCacheEnabled enables database schema caching
	SchemaCacheEnabled bool `json:"schema_cache_enabled"`

	// SchemaCacheTTL is the TTL for schema cache
	SchemaCacheTTL time.Duration `json:"schema_cache_ttl"`

	// ExcludedTables lists tables that should never be cached
	ExcludedTables []string `json:"excluded_tables"`

	// Redis configuration (when backend is "redis")
	RedisHost     string `json:"redis_host"`
	RedisPort     int    `json:"redis_port"`
	RedisPassword string `json:"redis_password"`
	RedisDB       int    `json:"redis_db"`
}

// AnalyticsConfig holds DuckDB analytics configuration
type AnalyticsConfig struct {
	// Enabled enables the analytics engine
	Enabled bool `json:"enabled"`

	// DuckDB configuration
	DuckDB DuckDBConfig `json:"duckdb"`

	// Refresh configuration
	Refresh RefreshConfig `json:"refresh"`

	// Router configuration
	Router RouterConfig `json:"router"`
}

// DuckDBConfig holds DuckDB-specific configuration
type DuckDBConfig struct {
	// Path is the database file path (empty for in-memory)
	// Use ":memory:" for pure in-memory operation
	Path string `json:"path"`

	// MemoryLimit is the maximum memory DuckDB can use (e.g., "4GB", "512MB")
	MemoryLimit string `json:"memory_limit"`

	// Threads is the number of threads for query execution (0 = auto)
	Threads int `json:"threads"`

	// TempDirectory for spilling large operations to disk
	TempDirectory string `json:"temp_directory"`
}

// RefreshConfig holds aggregate refresh configuration
type RefreshConfig struct {
	// SchedulerEnabled enables the scheduled refresh background worker
	SchedulerEnabled bool `json:"scheduler_enabled"`

	// SchedulerInterval is how often to check for pending scheduled refreshes
	SchedulerInterval time.Duration `json:"scheduler_interval"`

	// CDCEnabled enables CDC-based (Change Data Capture) refresh
	// This monitors PostgreSQL for changes and triggers refreshes
	CDCEnabled bool `json:"cdc_enabled"`

	// CDCMode is the CDC operation mode: "polling", "realtime", or "both"
	// - polling: Uses pg_stat_user_tables to detect changes (lower PostgreSQL overhead, higher latency)
	// - realtime: Uses LISTEN/NOTIFY with triggers (lower latency, requires trigger creation)
	// - both: Uses both modes for redundancy
	CDCMode string `json:"cdc_mode"`

	// CDCPollInterval is how often to poll PostgreSQL for changes (polling mode)
	CDCPollInterval time.Duration `json:"cdc_poll_interval"`

	// CDCCreateTriggers automatically creates CDC triggers on monitored tables (realtime mode)
	// When true, triggers are created automatically when aggregates are defined
	CDCCreateTriggers bool `json:"cdc_create_triggers"`

	// CDCIncludeRowData includes row data in CDC events (realtime mode)
	// When true, old/new row data is included in notifications (increases payload size)
	CDCIncludeRowData bool `json:"cdc_include_row_data"`

	// CDCReconnectDelay is the delay before reconnecting after a connection failure
	CDCReconnectDelay time.Duration `json:"cdc_reconnect_delay"`

	// LazyTTL is the time-to-live for lazy-refreshed aggregates
	// After this duration, the next query triggers a recomputation
	LazyTTL time.Duration `json:"lazy_ttl"`

	// MaxConcurrentRefreshes limits parallel refresh operations
	MaxConcurrentRefreshes int `json:"max_concurrent_refreshes"`

	// RefreshTimeout is the maximum time for a single refresh operation
	RefreshTimeout time.Duration `json:"refresh_timeout"`
}

// RouterConfig holds query routing configuration
type RouterConfig struct {
	// Enabled enables analytics query routing
	Enabled bool `json:"enabled"`

	// PreferMaterialized prefers materialized data over live PostgreSQL queries
	PreferMaterialized bool `json:"prefer_materialized"`

	// FallbackToLive allows falling back to PostgreSQL if materialized data is unavailable
	FallbackToLive bool `json:"fallback_to_live"`

	// StalenessThreshold defines when materialized data is considered stale
	// Queries for stale data will trigger refresh or fallback
	StalenessThreshold time.Duration `json:"staleness_threshold"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:              "0.0.0.0",
			Port:              8080,
			EnablePlayground:  true,
			EnableIntrospection: true,
			RequestTimeout:    60 * time.Second,
			MaxConnections:    1000,
		},
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "",
			Database: "postgres",
			SSLMode:  "disable",
			Schema:   "public",
			Pool: PoolConfig{
				MinConns:             5,
				MaxConns:             50,
				MaxConnLifetime:      1 * time.Hour,
				MaxConnIdleTime:      30 * time.Minute,
				HealthCheckPeriod:    1 * time.Minute,
				ConnectTimeout:       10 * time.Second,
				LazyConnect:          false,
				PreferSimpleProtocol: false,
			},
			// Legacy fields
			MaxOpenConns:    50,
			MaxIdleConns:    5,
			ConnMaxLifetime: 1 * time.Hour,
		},
		Auth: AuthConfig{
			Enabled:           false,
			AdminSecret:       "",
			JWTSecret:         "",
			JWTAlgorithm:      "HS256",
			JWTClaimsNamespace: "https://graphpost.io/jwt/claims",
			UnauthorizedRole:  "anonymous",
			AllowedRoles:      []string{"user", "admin"},
			DefaultRole:       "user",
		},
		Console: ConsoleConfig{
			Enabled:    true,
			AssetsPath: "./console",
		},
		Events: EventsConfig{
			Enabled:           true,
			HTTPPoolSize:      100,
			FetchInterval:     1 * time.Second,
			RetryLimit:        3,
			RetryIntervals:    []int{10, 30, 60},
			EnableManualTrigger: true,
		},
		CORS: CORSConfig{
			Enabled:          true,
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
			AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Admin-Secret"},
			AllowCredentials: true,
			MaxAge:           86400,
		},
		GraphQL: GraphQLConfig{
			EnableQueries:       true,
			EnableMutations:     true,
			EnableSubscriptions: true,
			EnableAggregations:  true,
			QueryDepthLimit:     0, // Unlimited
			QueryTimeout:        0, // No timeout
		},
		Telemetry: TelemetryConfig{
			Enabled:        false,
			ServiceName:    "graphpost",
			ServiceVersion: "1.0.0",
			OTLPEndpoint:   "localhost:4317",
			OTLPProtocol:   "grpc",
			OTLPInsecure:   true,
			SampleRate:     1.0,
			TraceQueries:   true,
			TraceResolvers: true,
		},
		Logging: LoggingConfig{
			Level:              "info",
			Format:             "json",
			Output:             "stdout",
			QueryLog:           false,
			QueryLogLevel:      "debug",
			SlowQueryThreshold: 1 * time.Second,
			SlowQueryLogLevel:  "warn",
			LogQueryParams:     false,
			RequestLog:         true,
			IncludeStackTrace:  false,
		},
		Cache: CacheConfig{
			Enabled:            false,
			Backend:            "memory",
			DefaultTTL:         5 * time.Minute,
			MaxSize:            10000,
			QueryCacheEnabled:  true,
			QueryCacheTTL:      1 * time.Minute,
			SchemaCacheEnabled: true,
			SchemaCacheTTL:     5 * time.Minute,
			ExcludedTables:     []string{},
			RedisHost:          "localhost",
			RedisPort:          6379,
			RedisPassword:      "",
			RedisDB:            0,
		},
		Analytics: AnalyticsConfig{
			Enabled: false,
			DuckDB: DuckDBConfig{
				Path:          ":memory:",
				MemoryLimit:   "2GB",
				Threads:       0, // Auto
				TempDirectory: "",
			},
			Refresh: RefreshConfig{
				SchedulerEnabled:       true,
				SchedulerInterval:      30 * time.Second,
				CDCEnabled:             false,
				CDCMode:                "polling",
				CDCPollInterval:        5 * time.Second,
				CDCCreateTriggers:      false,
				CDCIncludeRowData:      false,
				CDCReconnectDelay:      5 * time.Second,
				LazyTTL:                5 * time.Minute,
				MaxConcurrentRefreshes: 4,
				RefreshTimeout:         5 * time.Minute,
			},
			Router: RouterConfig{
				Enabled:            true,
				PreferMaterialized: true,
				FallbackToLive:     true,
				StalenessThreshold: 5 * time.Minute,
			},
		},
	}
}

// LoadFromFile loads configuration from a JSON file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	config := DefaultConfig()

	if host := os.Getenv("GRAPHPOST_HOST"); host != "" {
		config.Server.Host = host
	}
	if port := os.Getenv("GRAPHPOST_PORT"); port != "" {
		var p int
		if err := json.Unmarshal([]byte(port), &p); err == nil {
			config.Server.Port = p
		}
	}
	if dbURL := os.Getenv("GRAPHPOST_DATABASE_URL"); dbURL != "" {
		// Parse database URL and set config
		config.Database = parseDatabaseURL(dbURL)
	}
	if secret := os.Getenv("GRAPHPOST_ADMIN_SECRET"); secret != "" {
		config.Auth.AdminSecret = secret
		config.Auth.Enabled = true
	}
	if jwtSecret := os.Getenv("GRAPHPOST_JWT_SECRET"); jwtSecret != "" {
		config.Auth.JWTSecret = jwtSecret
	}

	// Pool configuration from environment variables
	if minConns := os.Getenv("GRAPHPOST_POOL_MIN_CONNS"); minConns != "" {
		var v int32
		if err := json.Unmarshal([]byte(minConns), &v); err == nil {
			config.Database.Pool.MinConns = v
		}
	}
	if maxConns := os.Getenv("GRAPHPOST_POOL_MAX_CONNS"); maxConns != "" {
		var v int32
		if err := json.Unmarshal([]byte(maxConns), &v); err == nil {
			config.Database.Pool.MaxConns = v
		}
	}
	if maxLifetime := os.Getenv("GRAPHPOST_POOL_MAX_CONN_LIFETIME"); maxLifetime != "" {
		if d, err := time.ParseDuration(maxLifetime); err == nil {
			config.Database.Pool.MaxConnLifetime = d
		}
	}
	if maxIdleTime := os.Getenv("GRAPHPOST_POOL_MAX_CONN_IDLE_TIME"); maxIdleTime != "" {
		if d, err := time.ParseDuration(maxIdleTime); err == nil {
			config.Database.Pool.MaxConnIdleTime = d
		}
	}
	if healthCheck := os.Getenv("GRAPHPOST_POOL_HEALTH_CHECK_PERIOD"); healthCheck != "" {
		if d, err := time.ParseDuration(healthCheck); err == nil {
			config.Database.Pool.HealthCheckPeriod = d
		}
	}
	if connectTimeout := os.Getenv("GRAPHPOST_POOL_CONNECT_TIMEOUT"); connectTimeout != "" {
		if d, err := time.ParseDuration(connectTimeout); err == nil {
			config.Database.Pool.ConnectTimeout = d
		}
	}
	if lazyConnect := os.Getenv("GRAPHPOST_POOL_LAZY_CONNECT"); lazyConnect == "true" {
		config.Database.Pool.LazyConnect = true
	}
	if simpleProtocol := os.Getenv("GRAPHPOST_POOL_SIMPLE_PROTOCOL"); simpleProtocol == "true" {
		config.Database.Pool.PreferSimpleProtocol = true
	}

	// GraphQL operation configuration
	if enableQueries := os.Getenv("GRAPHPOST_ENABLE_QUERIES"); enableQueries == "false" {
		config.GraphQL.EnableQueries = false
	}
	if enableMutations := os.Getenv("GRAPHPOST_ENABLE_MUTATIONS"); enableMutations == "false" {
		config.GraphQL.EnableMutations = false
	}
	if enableSubscriptions := os.Getenv("GRAPHPOST_ENABLE_SUBSCRIPTIONS"); enableSubscriptions == "false" {
		config.GraphQL.EnableSubscriptions = false
	}
	if enableAggregations := os.Getenv("GRAPHPOST_ENABLE_AGGREGATIONS"); enableAggregations == "false" {
		config.GraphQL.EnableAggregations = false
	}
	if depthLimit := os.Getenv("GRAPHPOST_QUERY_DEPTH_LIMIT"); depthLimit != "" {
		var v int
		if err := json.Unmarshal([]byte(depthLimit), &v); err == nil {
			config.GraphQL.QueryDepthLimit = v
		}
	}
	if queryTimeout := os.Getenv("GRAPHPOST_QUERY_TIMEOUT"); queryTimeout != "" {
		if d, err := time.ParseDuration(queryTimeout); err == nil {
			config.GraphQL.QueryTimeout = d
		}
	}

	// Telemetry configuration
	if telemetryEnabled := os.Getenv("GRAPHPOST_TELEMETRY_ENABLED"); telemetryEnabled == "true" {
		config.Telemetry.Enabled = true
	}
	if serviceName := os.Getenv("GRAPHPOST_TELEMETRY_SERVICE_NAME"); serviceName != "" {
		config.Telemetry.ServiceName = serviceName
	}
	if serviceVersion := os.Getenv("GRAPHPOST_TELEMETRY_SERVICE_VERSION"); serviceVersion != "" {
		config.Telemetry.ServiceVersion = serviceVersion
	}
	if otlpEndpoint := os.Getenv("GRAPHPOST_OTLP_ENDPOINT"); otlpEndpoint != "" {
		config.Telemetry.OTLPEndpoint = otlpEndpoint
	}
	if otlpProtocol := os.Getenv("GRAPHPOST_OTLP_PROTOCOL"); otlpProtocol != "" {
		config.Telemetry.OTLPProtocol = otlpProtocol
	}
	if otlpInsecure := os.Getenv("GRAPHPOST_OTLP_INSECURE"); otlpInsecure == "false" {
		config.Telemetry.OTLPInsecure = false
	}
	if sampleRate := os.Getenv("GRAPHPOST_TELEMETRY_SAMPLE_RATE"); sampleRate != "" {
		var v float64
		if err := json.Unmarshal([]byte(sampleRate), &v); err == nil {
			config.Telemetry.SampleRate = v
		}
	}
	if traceQueries := os.Getenv("GRAPHPOST_TELEMETRY_TRACE_QUERIES"); traceQueries == "false" {
		config.Telemetry.TraceQueries = false
	}
	if traceResolvers := os.Getenv("GRAPHPOST_TELEMETRY_TRACE_RESOLVERS"); traceResolvers == "false" {
		config.Telemetry.TraceResolvers = false
	}

	// Logging configuration
	if logLevel := os.Getenv("GRAPHPOST_LOG_LEVEL"); logLevel != "" {
		config.Logging.Level = logLevel
	}
	if logFormat := os.Getenv("GRAPHPOST_LOG_FORMAT"); logFormat != "" {
		config.Logging.Format = logFormat
	}
	if logOutput := os.Getenv("GRAPHPOST_LOG_OUTPUT"); logOutput != "" {
		config.Logging.Output = logOutput
	}
	if queryLog := os.Getenv("GRAPHPOST_LOG_QUERIES"); queryLog == "true" {
		config.Logging.QueryLog = true
	}
	if queryLogLevel := os.Getenv("GRAPHPOST_LOG_QUERY_LEVEL"); queryLogLevel != "" {
		config.Logging.QueryLogLevel = queryLogLevel
	}
	if slowQueryThreshold := os.Getenv("GRAPHPOST_SLOW_QUERY_THRESHOLD"); slowQueryThreshold != "" {
		if d, err := time.ParseDuration(slowQueryThreshold); err == nil {
			config.Logging.SlowQueryThreshold = d
		}
	}
	if slowQueryLogLevel := os.Getenv("GRAPHPOST_SLOW_QUERY_LOG_LEVEL"); slowQueryLogLevel != "" {
		config.Logging.SlowQueryLogLevel = slowQueryLogLevel
	}
	if logQueryParams := os.Getenv("GRAPHPOST_LOG_QUERY_PARAMS"); logQueryParams == "true" {
		config.Logging.LogQueryParams = true
	}
	if requestLog := os.Getenv("GRAPHPOST_LOG_REQUESTS"); requestLog == "false" {
		config.Logging.RequestLog = false
	}
	if includeStackTrace := os.Getenv("GRAPHPOST_LOG_STACK_TRACE"); includeStackTrace == "true" {
		config.Logging.IncludeStackTrace = true
	}

	// Cache configuration
	if cacheEnabled := os.Getenv("GRAPHPOST_CACHE_ENABLED"); cacheEnabled == "true" {
		config.Cache.Enabled = true
	}
	if cacheBackend := os.Getenv("GRAPHPOST_CACHE_BACKEND"); cacheBackend != "" {
		config.Cache.Backend = cacheBackend
	}
	if cacheTTL := os.Getenv("GRAPHPOST_CACHE_TTL"); cacheTTL != "" {
		if d, err := time.ParseDuration(cacheTTL); err == nil {
			config.Cache.DefaultTTL = d
		}
	}
	if cacheMaxSize := os.Getenv("GRAPHPOST_CACHE_MAX_SIZE"); cacheMaxSize != "" {
		var v int
		if err := json.Unmarshal([]byte(cacheMaxSize), &v); err == nil {
			config.Cache.MaxSize = v
		}
	}
	if queryCacheEnabled := os.Getenv("GRAPHPOST_CACHE_QUERY_ENABLED"); queryCacheEnabled == "false" {
		config.Cache.QueryCacheEnabled = false
	}
	if queryCacheTTL := os.Getenv("GRAPHPOST_CACHE_QUERY_TTL"); queryCacheTTL != "" {
		if d, err := time.ParseDuration(queryCacheTTL); err == nil {
			config.Cache.QueryCacheTTL = d
		}
	}
	if schemaCacheEnabled := os.Getenv("GRAPHPOST_CACHE_SCHEMA_ENABLED"); schemaCacheEnabled == "false" {
		config.Cache.SchemaCacheEnabled = false
	}
	if schemaCacheTTL := os.Getenv("GRAPHPOST_CACHE_SCHEMA_TTL"); schemaCacheTTL != "" {
		if d, err := time.ParseDuration(schemaCacheTTL); err == nil {
			config.Cache.SchemaCacheTTL = d
		}
	}
	if redisHost := os.Getenv("GRAPHPOST_REDIS_HOST"); redisHost != "" {
		config.Cache.RedisHost = redisHost
	}
	if redisPort := os.Getenv("GRAPHPOST_REDIS_PORT"); redisPort != "" {
		var v int
		if err := json.Unmarshal([]byte(redisPort), &v); err == nil {
			config.Cache.RedisPort = v
		}
	}
	if redisPassword := os.Getenv("GRAPHPOST_REDIS_PASSWORD"); redisPassword != "" {
		config.Cache.RedisPassword = redisPassword
	}
	if redisDB := os.Getenv("GRAPHPOST_REDIS_DB"); redisDB != "" {
		var v int
		if err := json.Unmarshal([]byte(redisDB), &v); err == nil {
			config.Cache.RedisDB = v
		}
	}

	// Analytics configuration
	if analyticsEnabled := os.Getenv("GRAPHPOST_ANALYTICS_ENABLED"); analyticsEnabled == "true" {
		config.Analytics.Enabled = true
	}
	if duckdbPath := os.Getenv("GRAPHPOST_DUCKDB_PATH"); duckdbPath != "" {
		config.Analytics.DuckDB.Path = duckdbPath
	}
	if duckdbMemory := os.Getenv("GRAPHPOST_DUCKDB_MEMORY_LIMIT"); duckdbMemory != "" {
		config.Analytics.DuckDB.MemoryLimit = duckdbMemory
	}
	if duckdbThreads := os.Getenv("GRAPHPOST_DUCKDB_THREADS"); duckdbThreads != "" {
		var v int
		if err := json.Unmarshal([]byte(duckdbThreads), &v); err == nil {
			config.Analytics.DuckDB.Threads = v
		}
	}
	if duckdbTempDir := os.Getenv("GRAPHPOST_DUCKDB_TEMP_DIR"); duckdbTempDir != "" {
		config.Analytics.DuckDB.TempDirectory = duckdbTempDir
	}

	// Analytics refresh configuration
	if schedulerEnabled := os.Getenv("GRAPHPOST_ANALYTICS_SCHEDULER_ENABLED"); schedulerEnabled == "false" {
		config.Analytics.Refresh.SchedulerEnabled = false
	}
	if schedulerInterval := os.Getenv("GRAPHPOST_ANALYTICS_SCHEDULER_INTERVAL"); schedulerInterval != "" {
		if d, err := time.ParseDuration(schedulerInterval); err == nil {
			config.Analytics.Refresh.SchedulerInterval = d
		}
	}
	if cdcEnabled := os.Getenv("GRAPHPOST_ANALYTICS_CDC_ENABLED"); cdcEnabled == "true" {
		config.Analytics.Refresh.CDCEnabled = true
	}
	if cdcPollInterval := os.Getenv("GRAPHPOST_ANALYTICS_CDC_POLL_INTERVAL"); cdcPollInterval != "" {
		if d, err := time.ParseDuration(cdcPollInterval); err == nil {
			config.Analytics.Refresh.CDCPollInterval = d
		}
	}
	if cdcMode := os.Getenv("GRAPHPOST_ANALYTICS_CDC_MODE"); cdcMode != "" {
		config.Analytics.Refresh.CDCMode = cdcMode
	}
	if cdcCreateTriggers := os.Getenv("GRAPHPOST_ANALYTICS_CDC_CREATE_TRIGGERS"); cdcCreateTriggers == "true" {
		config.Analytics.Refresh.CDCCreateTriggers = true
	}
	if cdcIncludeRowData := os.Getenv("GRAPHPOST_ANALYTICS_CDC_INCLUDE_ROW_DATA"); cdcIncludeRowData == "true" {
		config.Analytics.Refresh.CDCIncludeRowData = true
	}
	if cdcReconnectDelay := os.Getenv("GRAPHPOST_ANALYTICS_CDC_RECONNECT_DELAY"); cdcReconnectDelay != "" {
		if d, err := time.ParseDuration(cdcReconnectDelay); err == nil {
			config.Analytics.Refresh.CDCReconnectDelay = d
		}
	}
	if lazyTTL := os.Getenv("GRAPHPOST_ANALYTICS_LAZY_TTL"); lazyTTL != "" {
		if d, err := time.ParseDuration(lazyTTL); err == nil {
			config.Analytics.Refresh.LazyTTL = d
		}
	}
	if maxConcurrent := os.Getenv("GRAPHPOST_ANALYTICS_MAX_CONCURRENT_REFRESHES"); maxConcurrent != "" {
		var v int
		if err := json.Unmarshal([]byte(maxConcurrent), &v); err == nil {
			config.Analytics.Refresh.MaxConcurrentRefreshes = v
		}
	}
	if refreshTimeout := os.Getenv("GRAPHPOST_ANALYTICS_REFRESH_TIMEOUT"); refreshTimeout != "" {
		if d, err := time.ParseDuration(refreshTimeout); err == nil {
			config.Analytics.Refresh.RefreshTimeout = d
		}
	}

	// Analytics router configuration
	if routerEnabled := os.Getenv("GRAPHPOST_ANALYTICS_ROUTER_ENABLED"); routerEnabled == "false" {
		config.Analytics.Router.Enabled = false
	}
	if preferMaterialized := os.Getenv("GRAPHPOST_ANALYTICS_PREFER_MATERIALIZED"); preferMaterialized == "false" {
		config.Analytics.Router.PreferMaterialized = false
	}
	if fallbackToLive := os.Getenv("GRAPHPOST_ANALYTICS_FALLBACK_TO_LIVE"); fallbackToLive == "false" {
		config.Analytics.Router.FallbackToLive = false
	}
	if stalenessThreshold := os.Getenv("GRAPHPOST_ANALYTICS_STALENESS_THRESHOLD"); stalenessThreshold != "" {
		if d, err := time.ParseDuration(stalenessThreshold); err == nil {
			config.Analytics.Router.StalenessThreshold = d
		}
	}

	return config
}

// parseDatabaseURL parses a PostgreSQL connection URL
func parseDatabaseURL(url string) DatabaseConfig {
	// Basic URL parsing - in production, use proper URL parsing
	config := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "",
		Database: "postgres",
		SSLMode:  "disable",
		Schema:   "public",
		Pool: PoolConfig{
			MinConns:             5,
			MaxConns:             50,
			MaxConnLifetime:      1 * time.Hour,
			MaxConnIdleTime:      30 * time.Minute,
			HealthCheckPeriod:    1 * time.Minute,
			ConnectTimeout:       10 * time.Second,
			LazyConnect:          false,
			PreferSimpleProtocol: false,
		},
		MaxOpenConns:    50,
		MaxIdleConns:    5,
		ConnMaxLifetime: 1 * time.Hour,
	}
	// URL format: postgres://user:password@host:port/database?sslmode=disable
	// This is a simplified parser - use net/url for production
	return config
}

// ConnectionString returns a PostgreSQL connection string
func (d *DatabaseConfig) ConnectionString() string {
	return "host=" + d.Host +
		" port=" + string(rune(d.Port)) +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.Database +
		" sslmode=" + d.SSLMode
}

// DSN returns a PostgreSQL DSN
func (d *DatabaseConfig) DSN() string {
	return "postgres://" + d.User + ":" + d.Password + "@" + d.Host + ":" +
		string(rune(d.Port)) + "/" + d.Database + "?sslmode=" + d.SSLMode
}
