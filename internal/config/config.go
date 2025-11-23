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
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	User            string        `json:"user"`
	Password        string        `json:"password"`
	Database        string        `json:"database"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	Schema          string        `json:"schema"`
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
			Host:            "localhost",
			Port:            5432,
			User:            "postgres",
			Password:        "",
			Database:        "postgres",
			SSLMode:         "disable",
			MaxOpenConns:    50,
			MaxIdleConns:    10,
			ConnMaxLifetime: 30 * time.Minute,
			Schema:          "public",
		},
		Auth: AuthConfig{
			Enabled:           false,
			AdminSecret:       "",
			JWTSecret:         "",
			JWTAlgorithm:      "HS256",
			JWTClaimsNamespace: "https://hasura.io/jwt/claims",
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
			AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Hasura-Admin-Secret"},
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

	return config
}

// parseDatabaseURL parses a PostgreSQL connection URL
func parseDatabaseURL(url string) DatabaseConfig {
	// Basic URL parsing - in production, use proper URL parsing
	config := DatabaseConfig{
		Host:            "localhost",
		Port:            5432,
		User:            "postgres",
		Password:        "",
		Database:        "postgres",
		SSLMode:         "disable",
		MaxOpenConns:    50,
		MaxIdleConns:    10,
		ConnMaxLifetime: 30 * time.Minute,
		Schema:          "public",
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
