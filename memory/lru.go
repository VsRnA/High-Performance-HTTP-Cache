// Package memory предоставляет высокопроизводительные in-memory реализации кэша
package memory

import (
	"sync"
	"sync/atomic"
	"time"

	cache "github.com/VsRnA/High-Performance-HTTP-Cache"
)

// lruItem представляет элемент в LRU кэше
type lruItem struct {
	key        string
	value      []byte
	expiresAt  time.Time
	prev, next *lruItem
}

// isExpired проверяет истек ли элемент
func (item *lruItem) isExpired() bool {
	return !item.expiresAt.IsZero() && time.Now().After(item.expiresAt)
}

// LRUCache реализует Least Recently Used кэш
type LRUCache struct {
	// Основные данные
	items    map[string]*lruItem
	head     *lruItem // Самый недавно использованный
	tail     *lruItem // Самый давно использованный
	mu       sync.RWMutex
	
	// Конфигурация
	maxSize    int
	defaultTTL time.Duration
	
	// Управление жизненным циклом
	stopCh chan struct{}
	closed bool
	
	// Статистика (atomic для производительности)
	hits      int64
	misses    int64
	evictions int64
}

// NewLRU создает новый LRU кэш с указанным максимальным размером
func NewLRU(maxSize int) cache.Cache {
	return NewLRUWithTTL(maxSize, 0)
}

// NewLRUWithTTL создает новый LRU кэш с максимальным размером и TTL по умолчанию
func NewLRUWithTTL(maxSize int, defaultTTL time.Duration) cache.Cache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	
	c := &LRUCache{
		items:      make(map[string]*lruItem, maxSize),
		maxSize:    maxSize,
		defaultTTL: defaultTTL,
		stopCh:     make(chan struct{}),
	}

	c.head = &lruItem{}
	c.tail = &lruItem{}
	c.head.next = c.tail
	c.tail.prev = c.head
	
	if defaultTTL > 0 {
		go c.cleanup()
	}
	
	return c
}

// Get получает значение по ключу
func (c *LRUCache) Get(key string) ([]byte, bool) {
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
		c.removeItem(item)
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	c.moveToHead(item)
	
	atomic.AddInt64(&c.hits, 1)

	value := make([]byte, len(item.value))
	copy(value, item.value)
	return value, true
}

// Set сохраняет значение с TTL по умолчанию
func (c *LRUCache) Set(key string, value []byte) error {
	return c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL сохраняет значение с указанным TTL
func (c *LRUCache) SetWithTTL(key string, value []byte, ttl time.Duration) error {
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

	if existingItem, exists := c.items[key]; exists {
		existingItem.value = valueCopy
		existingItem.expiresAt = expiresAt
		c.moveToHead(existingItem)
		return nil
	}

	newItem := &lruItem{
		key:       key,
		value:     valueCopy,
		expiresAt: expiresAt,
	}

	if len(c.items) >= c.maxSize {
		c.evictTail()
	}

	c.items[key] = newItem
	c.addToHead(newItem)
	
	return nil
}

// Delete удаляет ключ из кэша
func (c *LRUCache) Delete(key string) bool {
	if key == "" {
		return false
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	item, exists := c.items[key]
	if !exists {
		return false
	}
	
	c.removeItem(item)
	return true
}

// Clear очищает весь кэш
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.items = make(map[string]*lruItem)
	c.head.next = c.tail
	c.tail.prev = c.head

	atomic.StoreInt64(&c.hits, 0)
	atomic.StoreInt64(&c.misses, 0)
	atomic.StoreInt64(&c.evictions, 0)
}

func (c *LRUCache) Stats() cache.Stats {
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

func (c *LRUCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.closed {
		return nil
	}
	
	c.closed = true
	close(c.stopCh)
	return nil
}

// Приватные методы для управления двусвязным списком

// addToHead добавляет элемент в начало списка
func (c *LRUCache) addToHead(item *lruItem) {
	item.prev = c.head
	item.next = c.head.next
	c.head.next.prev = item
	c.head.next = item
}

// removeFromList удаляет элемент из списка
func (c *LRUCache) removeFromList(item *lruItem) {
	item.prev.next = item.next
	item.next.prev = item.prev
}

// moveToHead перемещает элемент в начало списка
func (c *LRUCache) moveToHead(item *lruItem) {
	c.removeFromList(item)
	c.addToHead(item)
}

// evictTail удаляет последний элемент (LRU)
func (c *LRUCache) evictTail() {
	lastItem := c.tail.prev
	if lastItem != c.head {
		c.removeItem(lastItem)
		atomic.AddInt64(&c.evictions, 1)
	}
}

// removeItem полностью удаляет элемент из кэша
func (c *LRUCache) removeItem(item *lruItem) {
	delete(c.items, item.key)
	c.removeFromList(item)
}

// cleanup фоновая очистка истекших элементов
func (c *LRUCache) cleanup() {
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
func (c *LRUCache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	var expiredKeys []string

	for key, item := range c.items {
		if item.isExpired() {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		if item, exists := c.items[key]; exists {
			c.removeItem(item)
		}
	}
	
	if len(expiredKeys) > 0 {
		atomic.AddInt64(&c.evictions, int64(len(expiredKeys)))
	}
}
