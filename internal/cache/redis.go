package cache

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is a Redis-backed cache implementation
type RedisCache struct {
	client *redis.Client
	config RedisConfig

	// Stats (approximate for Redis)
	hits   int64
	misses int64
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(cfg RedisConfig) (*RedisCache, error) {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 6379
	}
	if cfg.PoolSize == 0 {
		cfg.PoolSize = 10
	}

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
		config: cfg,
	}, nil
}

// Get retrieves a value from cache
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool) {
	val, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}
	if err != nil {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	return val, true
}

// Set stores a value in cache with TTL
func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Delete removes a value from cache
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// Clear removes all values from cache (with graphpost prefix)
func (c *RedisCache) Clear(ctx context.Context) error {
	// Use SCAN to find keys with our prefix and delete them
	var cursor uint64
	prefix := "gql:*"

	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, prefix, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		if cursor == 0 {
			break
		}
	}

	// Also clear schema cache
	prefix = "schema:*"
	cursor = 0
	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, prefix, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		if cursor == 0 {
			break
		}
	}

	return nil
}

// Stats returns cache statistics
func (c *RedisCache) Stats() CacheStats {
	ctx := context.Background()

	hits := atomic.LoadInt64(&c.hits)
	misses := atomic.LoadInt64(&c.misses)
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	stats := CacheStats{
		Hits:    hits,
		Misses:  misses,
		HitRate: hitRate,
	}

	// Get Redis info
	info, err := c.client.Info(ctx, "memory").Result()
	if err == nil {
		// Parse memory usage from info string
		// This is a simplified version - production code should parse properly
		_ = info
	}

	// Get approximate key count
	dbSize, err := c.client.DBSize(ctx).Result()
	if err == nil {
		stats.Size = int(dbSize)
	}

	return stats
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// InvalidatePattern deletes all keys matching a pattern
func (c *RedisCache) InvalidatePattern(ctx context.Context, pattern string) error {
	var cursor uint64

	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		if cursor == 0 {
			break
		}
	}

	return nil
}

// SetWithTags stores a value with associated tags for later invalidation
func (c *RedisCache) SetWithTags(ctx context.Context, key string, value []byte, ttl time.Duration, tags []string) error {
	pipe := c.client.Pipeline()

	// Set the main value
	pipe.Set(ctx, key, value, ttl)

	// Add key to tag sets
	for _, tag := range tags {
		tagKey := "tag:" + tag
		pipe.SAdd(ctx, tagKey, key)
		pipe.Expire(ctx, tagKey, ttl+time.Hour) // Tags expire slightly after values
	}

	_, err := pipe.Exec(ctx)
	return err
}

// InvalidateTag deletes all keys with a specific tag
func (c *RedisCache) InvalidateTag(ctx context.Context, tag string) error {
	tagKey := "tag:" + tag

	// Get all keys with this tag
	keys, err := c.client.SMembers(ctx, tagKey).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		// Delete all tagged keys
		if err := c.client.Del(ctx, keys...).Err(); err != nil {
			return err
		}
	}

	// Delete the tag set itself
	return c.client.Del(ctx, tagKey).Err()
}

// Ping checks if Redis is responsive
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
