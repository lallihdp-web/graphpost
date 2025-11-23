package config

import (
	"encoding/json"
	"os"
	"time"
)

// Config represents the main configuration for GraphPost
type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Auth     AuthConfig     `json:"auth"`
	Console  ConsoleConfig  `json:"console"`
	Events   EventsConfig   `json:"events"`
	CORS     CORSConfig     `json:"cors"`
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

	// QueryTimeout is the default timeout for queries (0 = no timeout)
	// Default: 0 (no timeout)
	QueryTimeout time.Duration `json:"query_timeout"`

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
	Enabled           bool     `json:"enabled"`
	AdminSecret       string   `json:"admin_secret"`
	JWTSecret         string   `json:"jwt_secret"`
	JWTAlgorithm      string   `json:"jwt_algorithm"`
	JWTClaimsNamespace string  `json:"jwt_claims_namespace"`
	UnauthorizedRole  string   `json:"unauthorized_role"`
	AllowedRoles      []string `json:"allowed_roles"`
	DefaultRole       string   `json:"default_role"`
	WebhookURL        string   `json:"webhook_url"`
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
				QueryTimeout:         0, // No timeout
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
	if queryTimeout := os.Getenv("GRAPHPOST_POOL_QUERY_TIMEOUT"); queryTimeout != "" {
		if d, err := time.ParseDuration(queryTimeout); err == nil {
			config.Database.Pool.QueryTimeout = d
		}
	}
	if lazyConnect := os.Getenv("GRAPHPOST_POOL_LAZY_CONNECT"); lazyConnect == "true" {
		config.Database.Pool.LazyConnect = true
	}
	if simpleProtocol := os.Getenv("GRAPHPOST_POOL_SIMPLE_PROTOCOL"); simpleProtocol == "true" {
		config.Database.Pool.PreferSimpleProtocol = true
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
			QueryTimeout:         0,
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
