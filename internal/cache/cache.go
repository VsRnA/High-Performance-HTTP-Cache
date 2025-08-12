package cache

import (
	"sync"
	"time"
)

type Cache interface {
	Get(key string) (string, bool)
	Set(key string, value string)
	SetWithTTL(key string, value string, ttl time.Duration)
	Delete(key string) bool
}

type CacheItem struct {
	Value     string
	ExpiresAt time.Time
}

func (item *CacheItem) IsExpired() bool {
	return time.Now().After(item.ExpiresAt)
}

type SimpleCache struct {
	data map[string]*CacheItem
	mu   sync.RWMutex
}

func New() Cache {
	c := &SimpleCache{
		data: make(map[string]*CacheItem),
	}

	go c.cleanup()
	
	return c
}

func (c *SimpleCache) Get(key string) (string, bool) {
	c.mu.RLock()
	item, exists := c.data[key]
	c.mu.RUnlock()

	if !exists {
		return "", false
	}

	if item.IsExpired() {
		c.mu.Lock()
		item, exists = c.data[key]
		if exists && item.IsExpired() {
			delete(c.data, key)
			exists = false
		}
		c.mu.Unlock()
	}

	if !exists || item.IsExpired() {
		return "", false
	}

	return item.Value, true
}

func (c *SimpleCache) Set(key string, value string) {
	c.SetWithTTL(key, value, 0)
}

func (c *SimpleCache) SetWithTTL(key string, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	} else {
		expiresAt = time.Now().Add(100 * 365 * 24 * time.Hour)
	}
	
	c.data[key] = &CacheItem{
		Value:     value,
		ExpiresAt: expiresAt,
	}
}

func (c *SimpleCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	_, exists := c.data[key]
	if exists {
		delete(c.data, key)
	}
	return exists
}

func (c *SimpleCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		c.removeExpired()
	}
}

func (c *SimpleCache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for key, item := range c.data {
		if item.IsExpired() {
			delete(c.data, key)
		}
	}
}