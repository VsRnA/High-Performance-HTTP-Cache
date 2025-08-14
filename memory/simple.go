package memory

import (
	"sync"
	"sync/atomic"
	"time"

	cache "github.com/VsRnA/High-Performance-HTTP-Cache"
)

// simpleItem представляет элемент в простом кэше
type simpleItem struct {
	value     []byte
	expiresAt time.Time
}

// isExpired проверяет истек ли элемент
func (item *simpleItem) isExpired() bool {
	return !item.expiresAt.IsZero() && time.Now().After(item.expiresAt)
}

// SimpleCache - простейшая реализация кэша без политик вытеснения
// Подходит когда размер кэша контролируется приложением или когда нужна максимальная производительность
type SimpleCache struct {
	// Основные данные
	items map[string]*simpleItem
	mu    sync.RWMutex
	
	// Конфигурация
	defaultTTL time.Duration
	
	// Управление жизненным циклом
	stopCh chan struct{}
	closed bool
	
	// Статистика
	hits   int64
	misses int64
}

// NewSimple создает новый простой кэш без ограничений размера
func NewSimple() cache.Cache {
	return NewSimpleWithTTL(0)
}

// NewSimpleWithTTL создает новый простой кэш с TTL по умолчанию
func NewSimpleWithTTL(defaultTTL time.Duration) cache.Cache {
	c := &SimpleCache{
		items:      make(map[string]*simpleItem),
		defaultTTL: defaultTTL,
		stopCh:     make(chan struct{}),
	}

	if defaultTTL > 0 {
		go c.cleanup()
	}
	
	return c
}

// Get получает значение по ключу
func (c *SimpleCache) Get(key string) ([]byte, bool) {
	if key == "" {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}
	
	c.mu.RLock()
	item, exists := c.items[key]
	c.mu.RUnlock()
	
	if !exists {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	if item.isExpired() {
		c.mu.Lock()
		if item, exists := c.items[key]; exists && item.isExpired() {
			delete(c.items, key)
			exists = false
		}
		c.mu.Unlock()
		
		if !exists {
			atomic.AddInt64(&c.misses, 1)
			return nil, false
		}
	}
	
	atomic.AddInt64(&c.hits, 1)

	value := make([]byte, len(item.value))
	copy(value, item.value)
	return value, true
}

// Set сохраняет значение с TTL по умолчанию
func (c *SimpleCache) Set(key string, value []byte) error {
	return c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL сохраняет значение с указанным TTL
func (c *SimpleCache) SetWithTTL(key string, value []byte, ttl time.Duration) error {
	if key == "" {
		return cache.ErrKeyEmpty
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.closed {
		return cache.ErrCacheClosed
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	} else if c.defaultTTL > 0 {
		expiresAt = time.Now().Add(c.defaultTTL)
	}

	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	c.items[key] = &simpleItem{
		value:     valueCopy,
		expiresAt: expiresAt,
	}
	
	return nil
}

// Delete удаляет ключ из кэша
func (c *SimpleCache) Delete(key string) bool {
	if key == "" {
		return false
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	_, exists := c.items[key]
	if exists {
		delete(c.items, key)
		return true
	}
	
	return false
}

// Clear очищает весь кэш
func (c *SimpleCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.items = make(map[string]*simpleItem)

	atomic.StoreInt64(&c.hits, 0)
	atomic.StoreInt64(&c.misses, 0)
}

// Stats возвращает статистику кэша
func (c *SimpleCache) Stats() cache.Stats {
	c.mu.RLock()
	keys := int64(len(c.items))
	c.mu.RUnlock()
	
	stats := cache.Stats{
		Hits:      atomic.LoadInt64(&c.hits),
		Misses:    atomic.LoadInt64(&c.misses),
		Keys:      keys,
		Evictions: 0, // Простой кэш не делает eviction
	}
	
	stats.CalculateHitRate()
	return stats
}

// Close корректно завершает работу кэша
func (c *SimpleCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.closed {
		return nil
	}
	
	c.closed = true
	close(c.stopCh)
	return nil
}

// cleanup фоновая очистка истекших элементов
func (c *SimpleCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			c.removeExpired()
		case <-c.stopCh:
			return
		}
	}
}

// removeExpired удаляет все истекшие элементы
func (c *SimpleCache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	var expiredKeys []string

	for key, item := range c.items {
		if item.isExpired() {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(c.items, key)
	}
}
