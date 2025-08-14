package memory

import (
	"sync"
	"sync/atomic"
	"time"

	cache "github.com/VsRnA/High-Performance-HTTP-Cache"
)

// lfuItem представляет элемент в LFU кэше
type lfuItem struct {
	key        string
	value      []byte
	expiresAt  time.Time
	frequency  int64 // Частота использования
	lastAccess time.Time
}

// isExpired проверяет истек ли элемент
func (item *lfuItem) isExpired() bool {
	return !item.expiresAt.IsZero() && time.Now().After(item.expiresAt)
}

// touch увеличивает частоту использования
func (item *lfuItem) touch() {
	atomic.AddInt64(&item.frequency, 1)
	item.lastAccess = time.Now()
}

// LFUCache реализует Least Frequently Used кэш
// Вытесняет элементы которые используются реже всего
type LFUCache struct {
	// Основные данные
	items map[string]*lfuItem
	mu    sync.RWMutex
	
	// Конфигурация
	maxSize    int
	defaultTTL time.Duration
	
	// Управление жизненным циклом
	stopCh chan struct{}
	closed bool
	
	// Статистика
	hits      int64
	misses    int64
	evictions int64
}

// NewLFU создает новый LFU кэш с указанным максимальным размером
func NewLFU(maxSize int) cache.Cache {
	return NewLFUWithTTL(maxSize, 0)
}

// NewLFUWithTTL создает новый LFU кэш с максимальным размером и TTL по умолчанию
func NewLFUWithTTL(maxSize int, defaultTTL time.Duration) cache.Cache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	
	c := &LFUCache{
		items:      make(map[string]*lfuItem, maxSize),
		maxSize:    maxSize,
		defaultTTL: defaultTTL,
		stopCh:     make(chan struct{}),
	}

	if defaultTTL > 0 {
		go c.cleanup()
	}
	
	return c
}

// Get получает значение по ключу
func (c *LFUCache) Get(key string) ([]byte, bool) {
	if key == "" {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	item, exists := c.items[key]
	if !exists {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	if item.isExpired() {
		delete(c.items, key)
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	item.touch()
	atomic.AddInt64(&c.hits, 1)

	value := make([]byte, len(item.value))
	copy(value, item.value)
	return value, true
}

// Set сохраняет значение с TTL по умолчанию
func (c *LFUCache) Set(key string, value []byte) error {
	return c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL сохраняет значение с указанным TTL
func (c *LFUCache) SetWithTTL(key string, value []byte, ttl time.Duration) error {
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
	
	now := time.Now()

	if existingItem, exists := c.items[key]; exists {
		existingItem.value = valueCopy
		existingItem.expiresAt = expiresAt
		existingItem.lastAccess = now
		return nil
	}

	if len(c.items) >= c.maxSize {
		c.evictLFU()
	}

	newItem := &lfuItem{
		key:        key,
		value:      valueCopy,
		expiresAt:  expiresAt,
		frequency:  1, // Начальная частота
		lastAccess: now,
	}
	
	c.items[key] = newItem
	return nil
}

// Delete удаляет ключ из кэша
func (c *LFUCache) Delete(key string) bool {
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
func (c *LFUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.items = make(map[string]*lfuItem)

	atomic.StoreInt64(&c.hits, 0)
	atomic.StoreInt64(&c.misses, 0)
	atomic.StoreInt64(&c.evictions, 0)
}

// Stats возвращает статистику кэша
func (c *LFUCache) Stats() cache.Stats {
	c.mu.RLock()
	keys := int64(len(c.items))
	c.mu.RUnlock()
	
	stats := cache.Stats{
		Hits:      atomic.LoadInt64(&c.hits),
		Misses:    atomic.LoadInt64(&c.misses),
		Keys:      keys,
		Evictions: atomic.LoadInt64(&c.evictions),
	}
	
	stats.CalculateHitRate()
	return stats
}

// Close корректно завершает работу кэша
func (c *LFUCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.closed {
		return nil
	}
	
	c.closed = true
	close(c.stopCh)
	return nil
}

// evictLFU удаляет наименее часто используемый элемент
func (c *LFUCache) evictLFU() {
	if len(c.items) == 0 {
		return
	}
	
	var evictKey string
	var minFrequency int64 = -1
	var oldestTime time.Time

	for key, item := range c.items {
		frequency := atomic.LoadInt64(&item.frequency)
		
		if minFrequency == -1 || 
		   frequency < minFrequency || 
		   (frequency == minFrequency && item.lastAccess.Before(oldestTime)) {
			minFrequency = frequency
			evictKey = key
			oldestTime = item.lastAccess
		}
	}
	
	if evictKey != "" {
		delete(c.items, evictKey)
		atomic.AddInt64(&c.evictions, 1)
	}
}

// cleanup фоновая очистка истекших элементов
func (c *LFUCache) cleanup() {
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
func (c *LFUCache) removeExpired() {
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
	
	if len(expiredKeys) > 0 {
		atomic.AddInt64(&c.evictions, int64(len(expiredKeys)))
	}
}
