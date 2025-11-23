package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Config holds cache configuration
type Config struct {
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

	// CacheableOperations defines which operations can be cached
	// Options: "query", "aggregate", "subscription"
	CacheableOperations []string `json:"cacheable_operations"`

	// ExcludedTables lists tables that should never be cached
	ExcludedTables []string `json:"excluded_tables"`

	// Redis configuration (when backend is "redis")
	Redis RedisConfig `json:"redis"`
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	PoolSize int    `json:"pool_size"`
}

// DefaultConfig returns default cache configuration
func DefaultConfig() Config {
	return Config{
		Enabled:             false,
		Backend:             "memory",
		DefaultTTL:          5 * time.Minute,
		MaxSize:             10000,
		QueryCacheEnabled:   true,
		QueryCacheTTL:       1 * time.Minute,
		SchemaCacheEnabled:  true,
		SchemaCacheTTL:      5 * time.Minute,
		CacheableOperations: []string{"query", "aggregate"},
		ExcludedTables:      []string{},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			Password: "",
			DB:       0,
			PoolSize: 10,
		},
	}
}

// Cache is the main caching interface
type Cache interface {
	// Get retrieves a value from cache
	Get(ctx context.Context, key string) ([]byte, bool)

	// Set stores a value in cache with TTL
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a value from cache
	Delete(ctx context.Context, key string) error

	// Clear removes all values from cache
	Clear(ctx context.Context) error

	// Stats returns cache statistics
	Stats() CacheStats

	// Close closes the cache connection
	Close() error
}

// CacheStats holds cache statistics
type CacheStats struct {
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	HitRate    float64 `json:"hit_rate"`
	Size       int     `json:"size"`
	MaxSize    int     `json:"max_size"`
	Evictions  int64   `json:"evictions"`
	BytesUsed  int64   `json:"bytes_used"`
}

// Manager manages caching operations
type Manager struct {
	config  Config
	cache   Cache
	mu      sync.RWMutex
	enabled bool

	// Stats
	queryHits   int64
	queryMisses int64
	schemaHits  int64
	schemaMisses int64
}

var (
	globalManager *Manager
	managerOnce   sync.Once
)

// Initialize initializes the global cache manager
func Initialize(cfg Config) (*Manager, error) {
	var err error
	managerOnce.Do(func() {
		globalManager, err = NewManager(cfg)
	})
	return globalManager, err
}

// Get returns the global cache manager
func Get() *Manager {
	return globalManager
}

// NewManager creates a new cache manager
func NewManager(cfg Config) (*Manager, error) {
	m := &Manager{
		config:  cfg,
		enabled: cfg.Enabled,
	}

	if !cfg.Enabled {
		return m, nil
	}

	var err error
	switch cfg.Backend {
	case "memory":
		m.cache = NewMemoryCache(cfg.MaxSize, cfg.DefaultTTL)
	case "redis":
		m.cache, err = NewRedisCache(cfg.Redis)
		if err != nil {
			return nil, fmt.Errorf("failed to create redis cache: %w", err)
		}
	default:
		m.cache = NewMemoryCache(cfg.MaxSize, cfg.DefaultTTL)
	}

	return m, nil
}

// QueryCacheKey generates a cache key for a GraphQL query
type QueryCacheKey struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
	Role      string                 `json:"role"`
	UserID    string                 `json:"user_id"`
}

// GenerateKey generates a hash key for the query
func (k QueryCacheKey) GenerateKey() string {
	data, _ := json.Marshal(k)
	hash := sha256.Sum256(data)
	return "gql:" + hex.EncodeToString(hash[:16])
}

// GetQueryResult retrieves a cached query result
func (m *Manager) GetQueryResult(ctx context.Context, key QueryCacheKey) (interface{}, bool) {
	if m == nil || !m.enabled || !m.config.QueryCacheEnabled {
		return nil, false
	}

	cacheKey := key.GenerateKey()
	data, found := m.cache.Get(ctx, cacheKey)
	if !found {
		m.mu.Lock()
		m.queryMisses++
		m.mu.Unlock()
		return nil, false
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}

	m.mu.Lock()
	m.queryHits++
	m.mu.Unlock()
	return result, true
}

// SetQueryResult caches a query result
func (m *Manager) SetQueryResult(ctx context.Context, key QueryCacheKey, result interface{}, ttl time.Duration) error {
	if m == nil || !m.enabled || !m.config.QueryCacheEnabled {
		return nil
	}

	if ttl == 0 {
		ttl = m.config.QueryCacheTTL
	}

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	cacheKey := key.GenerateKey()
	return m.cache.Set(ctx, cacheKey, data, ttl)
}

// InvalidateTable invalidates all cached queries for a table
func (m *Manager) InvalidateTable(ctx context.Context, tableName string) error {
	if m == nil || !m.enabled {
		return nil
	}

	// For memory cache, we can iterate and delete
	// For production, use a more sophisticated approach with tags
	return m.cache.Delete(ctx, "table:"+tableName)
}

// GetSchema retrieves cached database schema
func (m *Manager) GetSchema(ctx context.Context, schemaName string) ([]byte, bool) {
	if m == nil || !m.enabled || !m.config.SchemaCacheEnabled {
		return nil, false
	}

	data, found := m.cache.Get(ctx, "schema:"+schemaName)
	if !found {
		m.mu.Lock()
		m.schemaMisses++
		m.mu.Unlock()
		return nil, false
	}

	m.mu.Lock()
	m.schemaHits++
	m.mu.Unlock()
	return data, true
}

// SetSchema caches database schema
func (m *Manager) SetSchema(ctx context.Context, schemaName string, schema []byte) error {
	if m == nil || !m.enabled || !m.config.SchemaCacheEnabled {
		return nil
	}

	return m.cache.Set(ctx, "schema:"+schemaName, schema, m.config.SchemaCacheTTL)
}

// IsTableCacheable checks if a table should be cached
func (m *Manager) IsTableCacheable(tableName string) bool {
	if m == nil || !m.enabled {
		return false
	}

	for _, excluded := range m.config.ExcludedTables {
		if excluded == tableName {
			return false
		}
	}
	return true
}

// IsOperationCacheable checks if an operation type should be cached
func (m *Manager) IsOperationCacheable(operation string) bool {
	if m == nil || !m.enabled {
		return false
	}

	for _, op := range m.config.CacheableOperations {
		if op == operation {
			return true
		}
	}
	return false
}

// Stats returns cache manager statistics
func (m *Manager) Stats() ManagerStats {
	if m == nil {
		return ManagerStats{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := ManagerStats{
		Enabled:      m.enabled,
		QueryHits:    m.queryHits,
		QueryMisses:  m.queryMisses,
		SchemaHits:   m.schemaHits,
		SchemaMisses: m.schemaMisses,
	}

	if m.queryHits+m.queryMisses > 0 {
		stats.QueryHitRate = float64(m.queryHits) / float64(m.queryHits+m.queryMisses)
	}

	if m.cache != nil {
		stats.CacheStats = m.cache.Stats()
	}

	return stats
}

// ManagerStats holds cache manager statistics
type ManagerStats struct {
	Enabled      bool       `json:"enabled"`
	QueryHits    int64      `json:"query_hits"`
	QueryMisses  int64      `json:"query_misses"`
	QueryHitRate float64    `json:"query_hit_rate"`
	SchemaHits   int64      `json:"schema_hits"`
	SchemaMisses int64      `json:"schema_misses"`
	CacheStats   CacheStats `json:"cache_stats"`
}

// Close closes the cache manager
func (m *Manager) Close() error {
	if m == nil || m.cache == nil {
		return nil
	}
	return m.cache.Close()
}

// Clear clears all cached data
func (m *Manager) Clear(ctx context.Context) error {
	if m == nil || m.cache == nil {
		return nil
	}
	return m.cache.Clear(ctx)
}

// Config returns the cache configuration
func (m *Manager) Config() Config {
	if m == nil {
		return DefaultConfig()
	}
	return m.config
}

// IsEnabled returns whether caching is enabled
func (m *Manager) IsEnabled() bool {
	return m != nil && m.enabled
}
