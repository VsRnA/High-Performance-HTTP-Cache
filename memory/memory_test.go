package memory

import (
	"fmt"
	"sync"
	"testing"
	"time"

	cache "github.com/VsRnA/High-Performance-HTTP-Cache"
)

// Импортируем ошибки для удобства
var (
	ErrKeyEmpty    = cache.ErrKeyEmpty
	ErrCacheClosed = cache.ErrCacheClosed
)

// TestAllImplementations тестирует все реализации на одном наборе тестов
func TestAllImplementations(t *testing.T) {
	implementations := map[string]func() cache.Cache{
		"Simple": func() cache.Cache { return NewSimpleWithTTL(1 * time.Minute) }, // Добавим TTL для тестирования
		"LRU":    func() cache.Cache { return NewLRU(100) },
		"LFU":    func() cache.Cache { return NewLFU(100) },
	}

	for name, constructor := range implementations {
		t.Run(name, func(t *testing.T) {
			cache := constructor()
			defer cache.Close()
			
			testBasicOperations(t, cache)
			testTTL(t, cache)
			testStats(t, cache)
		})
	}
}

// testBasicOperations проверяет базовые операции кэша
func testBasicOperations(t *testing.T, cache cache.Cache) {
	// Тест Set/Get
	key := "test_key"
	value := []byte("test_value")

	err := cache.Set(key, value)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrievedValue, exists := cache.Get(key)
	if !exists {
		t.Fatal("Key not found after Set")
	}

	if string(retrievedValue) != string(value) {
		t.Fatalf("Wrong value: expected %s, got %s", value, retrievedValue)
	}

	// Тест Delete
	deleted := cache.Delete(key)
	if !deleted {
		t.Fatal("Delete should return true for existing key")
	}

	_, exists = cache.Get(key)
	if exists {
		t.Fatal("Key should not exist after Delete")
	}

	// Тест повторного Delete
	deleted = cache.Delete(key)
	if deleted {
		t.Fatal("Delete should return false for non-existing key")
	}
}

// testTTL проверяет функциональность TTL
func testTTL(t *testing.T, cache cache.Cache) {
	key := "ttl_key"
	value := []byte("ttl_value")
	ttl := 100 * time.Millisecond

	// Сохраняем с TTL
	err := cache.SetWithTTL(key, value, ttl)
	if err != nil {
		t.Fatalf("SetWithTTL failed: %v", err)
	}

	// Сразу должно быть доступно
	_, exists := cache.Get(key)
	if !exists {
		t.Fatal("Key should exist immediately after SetWithTTL")
	}

	// Ждем истечения TTL
	time.Sleep(150 * time.Millisecond)

	// Проверяем истечение - может потребоваться несколько попыток для Simple кэша
	// так как у него нет фоновой очистки, только lazy expiration
	maxAttempts := 3
	expired := false
	for i := 0; i < maxAttempts; i++ {
		if _, exists := cache.Get(key); !exists {
			expired = true
			break
		}
		time.Sleep(50 * time.Millisecond) // Небольшая задержка между попытками
	}

	if !expired {
		t.Fatal("Key should expire after TTL")
	}
}

// testStats проверяет корректность статистики
func testStats(t *testing.T, cache cache.Cache) {
	// Очищаем кэш перед тестом статистики
	cache.Clear()
	
	// Начальная статистика
	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Keys != 0 {
		t.Fatalf("Initial stats should be zero, got: hits=%d, misses=%d, keys=%d", 
			stats.Hits, stats.Misses, stats.Keys)
	}

	// Добавляем элементы
	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))

	stats = cache.Stats()
	if stats.Keys != 2 {
		t.Fatalf("Expected 2 keys, got %d", stats.Keys)
	}

	// Hit
	cache.Get("key1")
	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Fatalf("Expected 1 hit, got %d", stats.Hits)
	}

	// Miss
	cache.Get("nonexistent")
	stats = cache.Stats()
	if stats.Misses != 1 {
		t.Fatalf("Expected 1 miss, got %d", stats.Misses)
	}

	// Hit rate
	if stats.HitRate != 50.0 {
		t.Fatalf("Expected 50%% hit rate, got %.2f%%", stats.HitRate)
	}

	// Clear
	cache.Clear()
	stats = cache.Stats()
	if stats.Keys != 0 || stats.Hits != 0 || stats.Misses != 0 {
		t.Fatal("Stats should be reset after Clear")
	}
}

// TestLRUEviction специально тестирует LRU политику
func TestLRUEviction(t *testing.T) {
	cache := NewLRU(3)
	defer cache.Close()

	// Заполняем до лимита
	cache.Set("A", []byte("valueA"))
	cache.Set("B", []byte("valueB"))
	cache.Set("C", []byte("valueC"))

	// Делаем A недавно использованным
	cache.Get("A")

	// Добавляем D - должен вытеснить B (самый старый неиспользованный)
	cache.Set("D", []byte("valueD"))

	// Проверяем что осталось
	_, existsA := cache.Get("A")
	_, existsB := cache.Get("B")
	_, existsC := cache.Get("C")
	_, existsD := cache.Get("D")

	if !existsA {
		t.Error("A should still exist (recently used)")
	}
	if existsB {
		t.Error("B should be evicted (LRU)")
	}
	if !existsC {
		t.Error("C should still exist")
	}
	if !existsD {
		t.Error("D should exist (just added)")
	}

	stats := cache.Stats()
	if stats.Evictions == 0 {
		t.Error("Should have evictions")
	}
}

// TestLFUEviction специально тестирует LFU политику
func TestLFUEviction(t *testing.T) {
	cache := NewLFU(3)
	defer cache.Close()

	// Заполняем кэш
	cache.Set("A", []byte("valueA"))
	cache.Set("B", []byte("valueB"))
	cache.Set("C", []byte("valueC"))

	// Создаем разные частоты использования
	// A - 3 раза
	cache.Get("A")
	cache.Get("A")
	cache.Get("A")

	// B - 1 раз
	cache.Get("B")

	// C - не используем

	// Добавляем D - должен вытеснить C (наименее часто используемый)
	cache.Set("D", []byte("valueD"))

	// Проверяем результат
	_, existsA := cache.Get("A")
	_, existsB := cache.Get("B")
	_, existsC := cache.Get("C")
	_, existsD := cache.Get("D")

	if !existsA {
		t.Error("A should still exist (most frequently used)")
	}
	if !existsB {
		t.Error("B should still exist (used once)")
	}
	if existsC {
		t.Error("C should be evicted (never used after creation)")
	}
	if !existsD {
		t.Error("D should exist (just added)")
	}
}

// TestConcurrency проверяет потокобезопасность
func TestConcurrency(t *testing.T) {
	implementations := map[string]func() cache.Cache{
		"Simple": func() cache.Cache { return NewSimple() },
		"LRU":    func() cache.Cache { return NewLRU(1000) },
		"LFU":    func() cache.Cache { return NewLFU(1000) },
	}

	for name, constructor := range implementations {
		t.Run(name, func(t *testing.T) {
			cache := constructor()
			defer cache.Close()

			var wg sync.WaitGroup
			goroutines := 10
			operations := 100

			// Запускаем горутины для одновременной записи/чтения
			for i := 0; i < goroutines; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for j := 0; j < operations; j++ {
						key := fmt.Sprintf("key_%d_%d", id, j)
						value := []byte(fmt.Sprintf("value_%d_%d", id, j))

						// Пишем
						cache.Set(key, value)

						// Читаем
						cache.Get(key)

						// Иногда удаляем
						if j%10 == 0 {
							cache.Delete(key)
						}
					}
				}(i)
			}

			wg.Wait()

			// Проверяем что кэш остался в консистентном состоянии
			stats := cache.Stats()
			if stats.Keys < 0 {
				t.Error("Negative key count indicates race condition")
			}
		})
	}
}

// TestErrorCases проверяет обработку ошибок
func TestErrorCases(t *testing.T) {
	cache := NewSimple()
	defer cache.Close()

	// Пустой ключ
	err := cache.Set("", []byte("value"))
	if err != ErrKeyEmpty {
		t.Errorf("Expected ErrKeyEmpty, got %v", err)
	}

	_, exists := cache.Get("")
	if exists {
		t.Error("Empty key should return false")
	}

	deleted := cache.Delete("")
	if deleted {
		t.Error("Delete empty key should return false")
	}

	// Закрытый кэш
	cache.Close()
	err = cache.Set("key", []byte("value"))
	if err != ErrCacheClosed {
		t.Errorf("Expected ErrCacheClosed, got %v", err)
	}
}

// TestDataSafety проверяет что возвращаемые данные безопасны от модификации
func TestDataSafety(t *testing.T) {
	cache := NewSimple()
	defer cache.Close()

	original := []byte("original data")
	cache.Set("safety", original)

	// Модифицируем исходные данные
	original[0] = 'X'

	// Получаем из кэша
	cached, exists := cache.Get("safety")
	if !exists {
		t.Fatal("Key should exist")
	}

	// Данные в кэше не должны измениться
	if string(cached) != "original data" {
		t.Error("Cache data was modified externally")
	}

	// Модифицируем полученные данные
	cached[0] = 'Y'

	// Снова получаем из кэша
	cached2, _ := cache.Get("safety")
	if string(cached2) != "original data" {
		t.Error("Cache data was modified through returned slice")
	}
}

// Бенчмарки для сравнения производительности

func BenchmarkSimpleSet(b *testing.B) {
	cache := NewSimple()
	defer cache.Close()
	benchmarkSet(b, cache)
}

func BenchmarkLRUSet(b *testing.B) {
	cache := NewLRU(b.N)
	defer cache.Close()
	benchmarkSet(b, cache)
}

func BenchmarkLFUSet(b *testing.B) {
	cache := NewLFU(b.N)
	defer cache.Close()
	benchmarkSet(b, cache)
}

func benchmarkSet(b *testing.B, cache cache.Cache) {
	value := []byte("benchmark value")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key%d", i)
		cache.Set(key, value)
	}
}

func BenchmarkSimpleGet(b *testing.B) {
	cache := NewSimple()
	defer cache.Close()
	benchmarkGet(b, cache)
}

func BenchmarkLRUGet(b *testing.B) {
	cache := NewLRU(b.N)
	defer cache.Close()
	benchmarkGet(b, cache)
}

func BenchmarkLFUGet(b *testing.B) {
	cache := NewLFU(b.N)
	defer cache.Close()
	benchmarkGet(b, cache)
}

func benchmarkGet(b *testing.B, cache cache.Cache) {
	// Предварительно заполняем кэш
	value := []byte("benchmark value")
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		cache.Set(key, value)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key%d", i%1000)
		cache.Get(key)
	}
}

// BenchmarkConcurrent тестирует производительность в многопоточном режиме
func BenchmarkConcurrentAccess(b *testing.B) {
	implementations := map[string]func() cache.Cache{
		"Simple": func() cache.Cache { return NewSimple() },
		"LRU":    func() cache.Cache { return NewLRU(10000) },
		"LFU":    func() cache.Cache { return NewLFU(10000) },
	}

	for name, constructor := range implementations {
		b.Run(name, func(b *testing.B) {
			cache := constructor()
			defer cache.Close()

			// Предварительно заполняем
			for i := 0; i < 1000; i++ {
				cache.Set(fmt.Sprintf("key%d", i), []byte("value"))
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					if i%2 == 0 {
						cache.Get(fmt.Sprintf("key%d", i%1000))
					} else {
						cache.Set(fmt.Sprintf("newkey%d", i), []byte("newvalue"))
					}
					i++
				}
			})
		})
	}
}