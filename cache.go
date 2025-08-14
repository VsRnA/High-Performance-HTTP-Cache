// Package cache предоставляет универсальные интерфейсы кэширования для Go приложений.
//
// Библиотека поддерживает множество реализаций:
//   - In-memory кэши: memory.NewLRU(), memory.NewLFU(), memory.NewSimple()
//   - Redis адаптер: redis.New()
//   - Распределенные кэши: distributed.NewConsistent()
package cache

import (
	"errors"
	"time"
)

// Cache определяет универсальный интерфейс для всех реализаций кэша
type Cache interface {
	// Get получает значение по ключу
	Get(key string) ([]byte, bool)
	
	// Set сохраняет значение в кэше
	Set(key string, value []byte) error
	
	// SetWithTTL сохраняет значение с указанным временем жизни
	SetWithTTL(key string, value []byte, ttl time.Duration) error
	
	// Delete удаляет ключ из кэша
	Delete(key string) bool
	
	// Clear очищает весь кэш
	Clear()
	
	// Stats возвращает статистику кэша
	Stats() Stats
	
	// Close корректно завершает работу кэша
	Close() error
}

// Stats содержит метрики производительности кэша
type Stats struct {
	Hits      int64   `json:"hits"`       // Успешные обращения
	Misses    int64   `json:"misses"`     // Промахи
	Keys      int64   `json:"keys"`       // Количество ключей
	Evictions int64   `json:"evictions"`  // Вытеснения
	HitRate   float64 `json:"hit_rate"`   // Процент попаданий
}

// CalculateHitRate вычисляет процент попаданий
func (s *Stats) CalculateHitRate() {
	total := s.Hits + s.Misses
	if total > 0 {
		s.HitRate = float64(s.Hits) / float64(total) * 100
	}
}

// EvictionPolicy определяет политику вытеснения элементов
type EvictionPolicy int

const (
	LRU EvictionPolicy = iota // Least Recently Used
	LFU                       // Least Frequently Used  
	FIFO                      // First In, First Out
)

// String возвращает строковое представление политики
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

// Общие ошибки для всех реализаций кэша
var (
	ErrKeyEmpty      = errors.New("ключ не может быть пустым")
	ErrValueTooLarge = errors.New("значение слишком большое")
	ErrCacheClosed   = errors.New("кэш закрыт")
	ErrCacheFull     = errors.New("кэш переполнен")
)