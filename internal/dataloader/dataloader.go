package dataloader

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DataLoader batches and caches database queries to prevent N+1 problems
type DataLoader struct {
	// batchFunc is called with batched keys
	batchFunc func(ctx context.Context, keys []interface{}) ([]interface{}, error)

	// Cache stores results
	cache map[interface{}]interface{}

	// Batching
	batch      []interface{}
	batchMu    sync.Mutex
	batchTimer *time.Timer
	wait       time.Duration
	maxBatch   int

	// Results
	results map[interface{}]chan result
	mu      sync.RWMutex
}

type result struct {
	data interface{}
	err  error
}

// NewDataLoader creates a new DataLoader
func NewDataLoader(batchFunc func(context.Context, []interface{}) ([]interface{}, error)) *DataLoader {
	return &DataLoader{
		batchFunc: batchFunc,
		cache:     make(map[interface{}]interface{}),
		results:   make(map[interface{}]chan result),
		wait:      1 * time.Millisecond, // Small delay to batch requests
		maxBatch:  100,
	}
}

// Load loads a value by key, batching requests
func (d *DataLoader) Load(ctx context.Context, key interface{}) (interface{}, error) {
	// Check cache first
	d.mu.RLock()
	if val, ok := d.cache[key]; ok {
		d.mu.RUnlock()
		return val, nil
	}
	d.mu.RUnlock()

	// Create result channel for this key
	d.mu.Lock()
	resChan, exists := d.results[key]
	if !exists {
		resChan = make(chan result, 1)
		d.results[key] = resChan

		// Add to batch
		d.batchMu.Lock()
		d.batch = append(d.batch, key)
		batchLen := len(d.batch)

		// Start timer if first item
		if batchLen == 1 {
			d.batchTimer = time.AfterFunc(d.wait, func() {
				d.executeBatch(ctx)
			})
		}

		// Execute immediately if batch is full
		if batchLen >= d.maxBatch {
			if d.batchTimer != nil {
				d.batchTimer.Stop()
			}
			d.batchMu.Unlock()
			d.executeBatch(ctx)
		} else {
			d.batchMu.Unlock()
		}
	}
	d.mu.Unlock()

	// Wait for result
	res := <-resChan
	return res.data, res.err
}

// executeBatch executes batched queries
func (d *DataLoader) executeBatch(ctx context.Context) {
	d.batchMu.Lock()
	if len(d.batch) == 0 {
		d.batchMu.Unlock()
		return
	}

	keys := d.batch
	d.batch = nil
	d.batchMu.Unlock()

	// Execute batch function
	results, err := d.batchFunc(ctx, keys)

	d.mu.Lock()
	defer d.mu.Unlock()

	if err != nil {
		// Send error to all waiting requests
		for _, key := range keys {
			if ch, ok := d.results[key]; ok {
				ch <- result{err: err}
				close(ch)
				delete(d.results, key)
			}
		}
		return
	}

	// Distribute results
	for i, key := range keys {
		var data interface{}
		if i < len(results) {
			data = results[i]
			// Cache result
			d.cache[key] = data
		}

		if ch, ok := d.results[key]; ok {
			ch <- result{data: data}
			close(ch)
			delete(d.results, key)
		}
	}
}

// Clear clears the cache
func (d *DataLoader) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[interface{}]interface{})
}

// ClearKey clears a specific key from cache
func (d *DataLoader) ClearKey(key interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.cache, key)
}

// RelationshipLoader creates a DataLoader for relationship queries
type RelationshipLoader struct {
	resolver   RelationshipResolver
	tableName  string
	foreignKey string
	loaders    map[string]*DataLoader // One loader per table+foreignKey combination
	mu         sync.RWMutex
}

// RelationshipResolver resolves batched relationship queries
type RelationshipResolver interface {
	ResolveBatch(ctx context.Context, tableName string, foreignKey string, keys []interface{}) (map[interface{}][]map[string]interface{}, error)
}

// NewRelationshipLoader creates a relationship loader
func NewRelationshipLoader(resolver RelationshipResolver) *RelationshipLoader {
	return &RelationshipLoader{
		resolver: resolver,
		loaders:  make(map[string]*DataLoader),
	}
}

// Load loads relationships for a foreign key value
func (rl *RelationshipLoader) Load(ctx context.Context, tableName, foreignKey string, keyValue interface{}) ([]map[string]interface{}, error) {
	loaderKey := fmt.Sprintf("%s:%s", tableName, foreignKey)

	rl.mu.RLock()
	loader, exists := rl.loaders[loaderKey]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if loader, exists = rl.loaders[loaderKey]; !exists {
			loader = NewDataLoader(func(ctx context.Context, keys []interface{}) ([]interface{}, error) {
				// Batch resolve relationships
				resultMap, err := rl.resolver.ResolveBatch(ctx, tableName, foreignKey, keys)
				if err != nil {
					return nil, err
				}

				// Convert map to slice in same order as keys
				results := make([]interface{}, len(keys))
				for i, key := range keys {
					if rows, ok := resultMap[key]; ok {
						results[i] = rows
					} else {
						results[i] = []map[string]interface{}{}
					}
				}
				return results, nil
			})
			rl.loaders[loaderKey] = loader
		}
		rl.mu.Unlock()
	}

	result, err := loader.Load(ctx, keyValue)
	if err != nil {
		return nil, err
	}

	if rows, ok := result.([]map[string]interface{}); ok {
		return rows, nil
	}

	return []map[string]interface{}{}, nil
}

// Clear clears all loaders
func (rl *RelationshipLoader) Clear() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for _, loader := range rl.loaders {
		loader.Clear()
	}
}
