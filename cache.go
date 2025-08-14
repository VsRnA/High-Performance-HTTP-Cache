// Package cache предоставляет высокопроизводительные реализации кэширования
package cache

import "time"

// Cache определяет базовые операции кэша
type Cache interface {
	// Get получает значение по ключу
	Get(key string) ([]byte, bool)
	
	// Set сохраняет значение с TTL по умолчанию
	Set(key string, value []byte) error
	
	// SetWithTTL сохраняет значение с указанным TTL
	SetWithTTL(key string, value []byte, ttl time.Duration) error
	
	// Delete удаляет ключ из кэша
	Delete(key string) bool
	
	// Clear удаляет все ключи из кэша
	Clear()
	
	// Stats возвращает статистику кэша
	Stats() Stats
	
	// Close корректно завершает работу кэша
	Close() error
}

// Stats содержит метрики производительности кэша
type Stats struct {
	Hits      int64   `json:"hits"`       // Количество попаданий в кэш
	Misses    int64   `json:"misses"`     // Количество промахов кэша
	Keys      int64   `json:"keys"`       // Текущее количество ключей
	Evictions int64   `json:"evictions"`  // Количество вытесненных элементов
	HitRate   float64 `json:"hit_rate"`   // Процент попаданий
}

// CalculateHitRate вычисляет процент попаданий
func (s *Stats) CalculateHitRate() {
	total := s.Hits + s.Misses
	if total > 0 {
		s.HitRate = float64(s.Hits) / float64(total) * 100
	}
}

// EvictionPolicy определяет как элементы вытесняются при заполнении кэша
type EvictionPolicy int

const (
	LRU EvictionPolicy = iota // Least Recently Used - наименее недавно использованный
	LFU                       // Least Frequently Used - наименее часто использованный
	FIFO                      // First In, First Out - первый вошел, первый вышел
)

// String возвращает строковое представление политики вытеснения
func (e EvictionPolicy) String() string {
	switch e {
	case LRU:
		return "LRU"
	case LFU:
		return "LFU"
	case FIFO:
		return "FIFO"
	default:
		return "Unknown"
	}
}

// Config содержит конфигурацию кэша
type Config struct {
	MaxSize         int            // Максимальное количество элементов (0 = безлимитно)
	DefaultTTL      time.Duration  // TTL по умолчанию для элементов
	CleanupInterval time.Duration  // Как часто очищать истекшие элементы
	EvictionPolicy  EvictionPolicy // Политика вытеснения при заполнении
}

// DefaultConfig возвращает разумную конфигурацию по умолчанию
func DefaultConfig() Config {
	return Config{
		MaxSize:         1000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		EvictionPolicy:  LRU,
	}
}