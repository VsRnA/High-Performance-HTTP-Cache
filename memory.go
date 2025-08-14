package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

// Item представляет элемент кэша
type Item struct {
	Value       []byte    // Значение
	ExpiresAt   time.Time // Время истечения
	LastAccess  time.Time // Время последнего доступа
	AccessCount int64     // Количество обращений
	CreatedAt   time.Time // Время создания
}

// IsExpired проверяет истек ли срок жизни элемента
func (item *Item) IsExpired() bool {
	return time.Now().After(item.ExpiresAt)
}

// Touch обновляет время последнего доступа и увеличивает счетчик обращений
func (item *Item) Touch() {
	item.LastAccess = time.Now()
	atomic.AddInt64(&item.AccessCount, 1)
}

// MemoryCache - реализация кэша в памяти
type MemoryCache struct {
	data   map[string]*Item // Данные кэша
	mu     sync.RWMutex     // Мьютекс для безопасного доступа
	config Config           // Конфигурация кэша
	
	// Корректное завершение работы
  stopCh chan struct{}
  once   sync.Once
	
	// Статистика (используем atomic для потокобезопасности)
	hits      int64 // Попадания
	misses    int64 // Промахи
	evictions int64 // Вытеснения
}

// NewMemoryCache создает новый кэш в памяти с заданной конфигурацией
func NewMemoryCache(config Config) Cache {
	cache := &MemoryCache{
		data:   make(map[string]*Item),
		config: config,
		stopCh: make(chan struct{}),
	}
	
	// Запускаем фоновую очистку если установлен интервал
	if config.CleanupInterval > 0 {
		go cache.cleanup()
	}
	
	return cache
}

// Get получает значение по ключу
func (c *MemoryCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	item, exists := c.data[key]
	c.mu.RUnlock()

	if !exists {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	// Проверяем истечение (паттерн double-checked locking)
	if item.IsExpired() {
		c.mu.Lock()
		item, exists = c.data[key]
		if exists && item.IsExpired() {
			delete(c.data, key)
			exists = false
		}
		c.mu.Unlock()
	}

	if !exists {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	// Обновляем информацию о доступе для политик вытеснения
	c.mu.Lock()
	if item, exists := c.data[key]; exists && !item.IsExpired() {
		item.Touch()
		atomic.AddInt64(&c.hits, 1)
		value := make([]byte, len(item.Value))
		copy(value, item.Value)
		c.mu.Unlock()
		return value, true
	}
	c.mu.Unlock()
	
	atomic.AddInt64(&c.misses, 1)
	return nil, false
}

// Set сохраняет значение с TTL по умолчанию
func (c *MemoryCache) Set(key string, value []byte) error {
	return c.SetWithTTL(key, value, c.config.DefaultTTL)
}

// SetWithTTL сохраняет значение с указанным TTL
func (c *MemoryCache) SetWithTTL(key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	var expiresAt time.Time
	
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	} else if c.config.DefaultTTL > 0 {
		expiresAt = now.Add(c.config.DefaultTTL)
	} else {
		// Без TTL - устанавливаем далекую дату
		expiresAt = now.Add(100 * 365 * 24 * time.Hour)
	}

	// Проверяем нужно ли вытеснить элементы
	if c.config.MaxSize > 0 && len(c.data) >= c.config.MaxSize {
		_, exists := c.data[key]
		if !exists { // Вытесняем только при добавлении нового ключа
			c.evict()
		}
	}
	
	// Создаем копию значения чтобы избежать внешних изменений
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	
	c.data[key] = &Item{
		Value:       valueCopy,
		ExpiresAt:   expiresAt,
		LastAccess:  now,
		AccessCount: 1,
		CreatedAt:   now,
	}
	
	return nil
}

// Delete удаляет ключ из кэша
func (c *MemoryCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	_, exists := c.data[key]
	if exists {
		delete(c.data, key)
	}
	return exists
}

// Clear удаляет все ключи из кэша
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.data = make(map[string]*Item)
	
	// Сбрасываем статистику
	atomic.StoreInt64(&c.hits, 0)
	atomic.StoreInt64(&c.misses, 0)
	atomic.StoreInt64(&c.evictions, 0)
}

// Stats возвращает статистику кэша
func (c *MemoryCache) Stats() Stats {
	c.mu.RLock()
	keys := int64(len(c.data))
	c.mu.RUnlock()
	
	stats := Stats{
		Hits:      atomic.LoadInt64(&c.hits),
		Misses:    atomic.LoadInt64(&c.misses),
		Keys:      keys,
		Evictions: atomic.LoadInt64(&c.evictions),
	}
	
	stats.CalculateHitRate()
	return stats
}

// Close корректно завершает работу кэша
func (c *MemoryCache) Close() error {
  c.once.Do(func() { close(c.stopCh) })
  return nil
}

// cleanup выполняет периодическую очистку истекших элементов
func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(c.config.CleanupInterval)
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
func (c *MemoryCache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	expired := 0
	for key, item := range c.data {
		if item.IsExpired() {
			delete(c.data, key)
			expired++
		}
	}
	
	if expired > 0 {
		atomic.AddInt64(&c.evictions, int64(expired))
	}
}

// evict удаляет элементы в соответствии с настроенной политикой вытеснения
func (c *MemoryCache) evict() {
	if len(c.data) == 0 {
		return
	}
	
	switch c.config.EvictionPolicy {
	case LRU:
		c.evictLRU()
	case LFU:
		c.evictLFU()
	case FIFO:
		c.evictFIFO()
	default:
		c.evictLRU() // По умолчанию LRU
	}
}

// evictLRU удаляет наименее недавно использованный элемент
func (c *MemoryCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	
	for key, item := range c.data {
		if first || item.LastAccess.Before(oldestTime) {
			oldestTime = item.LastAccess
			oldestKey = key
			first = false
		}
	}
	
	if oldestKey != "" {
		delete(c.data, oldestKey)
		atomic.AddInt64(&c.evictions, 1)
	}
}

// evictLFU удаляет наименее часто использованный элемент
func (c *MemoryCache) evictLFU() {
	var evictKey string
	var minAccess int64 = -1
	
	for key, item := range c.data {
		accessCount := atomic.LoadInt64(&item.AccessCount)
		if minAccess == -1 || accessCount < minAccess {
			minAccess = accessCount
			evictKey = key
		}
	}
	
	if evictKey != "" {
		delete(c.data, evictKey)
		atomic.AddInt64(&c.evictions, 1)
	}
}

// evictFIFO удаляет первый добавленный элемент (самый старый по времени создания)
func (c *MemoryCache) evictFIFO() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	
	for key, item := range c.data {
		if first || item.CreatedAt.Before(oldestTime) {
			oldestTime = item.CreatedAt
			oldestKey = key
			first = false
		}
	}
	
	if oldestKey != "" {
		delete(c.data, oldestKey)
		atomic.AddInt64(&c.evictions, 1)
	}
}