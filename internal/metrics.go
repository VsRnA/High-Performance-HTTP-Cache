package internal

import (
	"sync/atomic"
	"time"
)

// Metrics содержит детальные метрики для кэша
type Metrics struct {
	// Основные счетчики
	hits      int64
	misses    int64
	sets      int64
	deletes   int64
	evictions int64
	
	// Временные метрики
	totalSetTime    int64 // В наносекундах
	totalGetTime    int64 // В наносекундах
	totalDeleteTime int64 // В наносекундах
	
	// Размеры
	keyCount    int64
	memoryUsage int64
	
	// Время запуска
	startTime time.Time
}

// NewMetrics создает новый экземпляр метрик
func NewMetrics() *Metrics {
	return &Metrics{
		startTime: time.Now(),
	}
}

// RecordHit записывает попадание в кэш
func (m *Metrics) RecordHit() {
	atomic.AddInt64(&m.hits, 1)
}

// RecordMiss записывает промах кэша
func (m *Metrics) RecordMiss() {
	atomic.AddInt64(&m.misses, 1)
}

// RecordSet записывает операцию записи с временем выполнения
func (m *Metrics) RecordSet(duration time.Duration) {
	atomic.AddInt64(&m.sets, 1)
	atomic.AddInt64(&m.totalSetTime, int64(duration))
}

// RecordGet записывает операцию чтения с временем выполнения
func (m *Metrics) RecordGet(duration time.Duration) {
	atomic.AddInt64(&m.totalGetTime, int64(duration))
}

// RecordDelete записывает операцию удаления
func (m *Metrics) RecordDelete(duration time.Duration) {
	atomic.AddInt64(&m.deletes, 1)
	atomic.AddInt64(&m.totalDeleteTime, int64(duration))
}

// RecordEviction записывает вытеснение элемента
func (m *Metrics) RecordEviction() {
	atomic.AddInt64(&m.evictions, 1)
}

// SetKeyCount обновляет количество ключей
func (m *Metrics) SetKeyCount(count int64) {
	atomic.StoreInt64(&m.keyCount, count)
}

// SetMemoryUsage обновляет использование памяти
func (m *Metrics) SetMemoryUsage(bytes int64) {
	atomic.StoreInt64(&m.memoryUsage, bytes)
}

// Snapshot возвращает моментальный снимок всех метрик
type Snapshot struct {
	Hits      int64         `json:"hits"`
	Misses    int64         `json:"misses"`
	Sets      int64         `json:"sets"`
	Deletes   int64         `json:"deletes"`
	Evictions int64         `json:"evictions"`
	KeyCount  int64         `json:"key_count"`
	Memory    int64         `json:"memory_usage"`
	HitRate   float64       `json:"hit_rate"`
	Uptime    time.Duration `json:"uptime"`
	
	// Средние времена выполнения
	AvgSetTime    time.Duration `json:"avg_set_time"`
	AvgGetTime    time.Duration `json:"avg_get_time"`
	AvgDeleteTime time.Duration `json:"avg_delete_time"`
	
	// Операции в секунду
	SetsPerSec    float64 `json:"sets_per_sec"`
	GetsPerSec    float64 `json:"gets_per_sec"`
	DeletesPerSec float64 `json:"deletes_per_sec"`
}

// GetSnapshot возвращает снимок текущих метрик
func (m *Metrics) GetSnapshot() Snapshot {
	hits := atomic.LoadInt64(&m.hits)
	misses := atomic.LoadInt64(&m.misses)
	sets := atomic.LoadInt64(&m.sets)
	deletes := atomic.LoadInt64(&m.deletes)
	evictions := atomic.LoadInt64(&m.evictions)
	keyCount := atomic.LoadInt64(&m.keyCount)
	memory := atomic.LoadInt64(&m.memoryUsage)
	
	totalSetTime := atomic.LoadInt64(&m.totalSetTime)
	totalGetTime := atomic.LoadInt64(&m.totalGetTime)
	totalDeleteTime := atomic.LoadInt64(&m.totalDeleteTime)
	
	uptime := time.Since(m.startTime)
	uptimeSeconds := uptime.Seconds()
	
	snapshot := Snapshot{
		Hits:      hits,
		Misses:    misses,
		Sets:      sets,
		Deletes:   deletes,
		Evictions: evictions,
		KeyCount:  keyCount,
		Memory:    memory,
		Uptime:    uptime,
	}
	
	// Вычисляем hit rate
	total := hits + misses
	if total > 0 {
		snapshot.HitRate = float64(hits) / float64(total) * 100
	}
	
	// Вычисляем средние времена
	if sets > 0 {
		snapshot.AvgSetTime = time.Duration(totalSetTime / sets)
	}
	
	totalGets := hits + misses
	if totalGets > 0 {
		snapshot.AvgGetTime = time.Duration(totalGetTime / totalGets)
	}
	
	if deletes > 0 {
		snapshot.AvgDeleteTime = time.Duration(totalDeleteTime / deletes)
	}
	
	// Вычисляем операции в секунду
	if uptimeSeconds > 0 {
		snapshot.SetsPerSec = float64(sets) / uptimeSeconds
		snapshot.GetsPerSec = float64(totalGets) / uptimeSeconds
		snapshot.DeletesPerSec = float64(deletes) / uptimeSeconds
	}
	
	return snapshot
}

// Reset сбрасывает все метрики
func (m *Metrics) Reset() {
	atomic.StoreInt64(&m.hits, 0)
	atomic.StoreInt64(&m.misses, 0)
	atomic.StoreInt64(&m.sets, 0)
	atomic.StoreInt64(&m.deletes, 0)
	atomic.StoreInt64(&m.evictions, 0)
	atomic.StoreInt64(&m.totalSetTime, 0)
	atomic.StoreInt64(&m.totalGetTime, 0)
	atomic.StoreInt64(&m.totalDeleteTime, 0)
	atomic.StoreInt64(&m.keyCount, 0)
	atomic.StoreInt64(&m.memoryUsage, 0)
	m.startTime = time.Now()
}

// Timer помогает измерять время выполнения операций
type Timer struct {
	start time.Time
}

// NewTimer создает новый таймер
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// Duration возвращает прошедшее время
func (t *Timer) Duration() time.Duration {
	return time.Since(t.start)
}

// EstimateMemory приблизительно оценивает использование памяти для ключа и значения
func EstimateMemory(key string, value []byte) int64 {
	// Размер ключа + размер значения + накладные расходы
	overhead := int64(64) // Приблизительные накладные расходы структуры
	return int64(len(key)) + int64(len(value)) + overhead
}