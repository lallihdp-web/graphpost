package cache

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryCache is an in-memory LRU cache with TTL support
type MemoryCache struct {
	mu         sync.RWMutex
	items      map[string]*cacheItem
	lruList    *list.List
	maxSize    int
	defaultTTL time.Duration

	// Stats
	hits      int64
	misses    int64
	evictions int64
	bytesUsed int64
}

type cacheItem struct {
	key       string
	value     []byte
	expiresAt time.Time
	element   *list.Element
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(maxSize int, defaultTTL time.Duration) *MemoryCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	if defaultTTL <= 0 {
		defaultTTL = 5 * time.Minute
	}

	c := &MemoryCache{
		items:      make(map[string]*cacheItem),
		lruList:    list.New(),
		maxSize:    maxSize,
		defaultTTL: defaultTTL,
	}

	// Start cleanup goroutine
	go c.cleanupLoop()

	return c
}

// Get retrieves a value from cache
func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.items[key]
	if !found {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	// Check if expired
	if time.Now().After(item.expiresAt) {
		c.removeItem(item)
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	// Move to front of LRU list
	c.lruList.MoveToFront(item.element)
	atomic.AddInt64(&c.hits, 1)

	// Return a copy to prevent mutation
	result := make([]byte, len(item.value))
	copy(result, item.value)
	return result, true
}

// Set stores a value in cache with TTL
func (c *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	// Check if key already exists
	if item, found := c.items[key]; found {
		// Update existing item
		atomic.AddInt64(&c.bytesUsed, int64(len(value)-len(item.value)))
		item.value = value
		item.expiresAt = time.Now().Add(ttl)
		c.lruList.MoveToFront(item.element)
		return nil
	}

	// Evict items if at capacity
	for len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	// Create new item
	item := &cacheItem{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	item.element = c.lruList.PushFront(item)
	c.items[key] = item
	atomic.AddInt64(&c.bytesUsed, int64(len(value)))

	return nil
}

// Delete removes a value from cache
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, found := c.items[key]; found {
		c.removeItem(item)
	}
	return nil
}

// Clear removes all values from cache
func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
	c.lruList = list.New()
	atomic.StoreInt64(&c.bytesUsed, 0)
	return nil
}

// Stats returns cache statistics
func (c *MemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hits := atomic.LoadInt64(&c.hits)
	misses := atomic.LoadInt64(&c.misses)
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
		Hits:      hits,
		Misses:    misses,
		HitRate:   hitRate,
		Size:      len(c.items),
		MaxSize:   c.maxSize,
		Evictions: atomic.LoadInt64(&c.evictions),
		BytesUsed: atomic.LoadInt64(&c.bytesUsed),
	}
}

// Close closes the cache (no-op for memory cache)
func (c *MemoryCache) Close() error {
	return nil
}

// removeItem removes an item from the cache (must be called with lock held)
func (c *MemoryCache) removeItem(item *cacheItem) {
	c.lruList.Remove(item.element)
	delete(c.items, item.key)
	atomic.AddInt64(&c.bytesUsed, -int64(len(item.value)))
}

// evictOldest removes the oldest item from the cache (must be called with lock held)
func (c *MemoryCache) evictOldest() {
	elem := c.lruList.Back()
	if elem != nil {
		item := elem.Value.(*cacheItem)
		c.removeItem(item)
		atomic.AddInt64(&c.evictions, 1)
	}
}

// cleanupLoop periodically removes expired items
func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes all expired items
func (c *MemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for _, item := range c.items {
		if now.After(item.expiresAt) {
			c.removeItem(item)
		}
	}
}

// Size returns the current number of items in cache
func (c *MemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Keys returns all keys in the cache (for debugging)
func (c *MemoryCache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, k)
	}
	return keys
}
