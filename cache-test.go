package cache

import (
	"fmt"
	"testing"
	"time"
)

// TestBasicOperations тестирует базовые операции кэша
func TestBasicOperations(t *testing.T) {
	config := DefaultConfig()
	config.MaxSize = 10
	config.CleanupInterval = 0 // Отключаем фоновую очистку для тестов
	
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	// Тест Set и Get
	key := "test_key"
	value := []byte("test_value")
	
	err := cache.Set(key, value)
	if err != nil {
		t.Fatalf("Ошибка при сохранении: %v", err)
	}
	
	retrievedValue, exists := cache.Get(key)
	if !exists {
		t.Fatal("Ключ не найден после сохранения")
	}
	
	if string(retrievedValue) != string(value) {
		t.Fatalf("Получено неверное значение: ожидали %s, получили %s", value, retrievedValue)
	}
	
	// Тест Delete
	deleted := cache.Delete(key)
	if !deleted {
		t.Fatal("Ключ не был удален")
	}
	
	_, exists = cache.Get(key)
	if exists {
		t.Fatal("Ключ все еще существует после удаления")
	}
}

// TestTTL тестирует функциональность TTL
func TestTTL(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 0 // Отключаем фоновую очистку
	
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	key := "ttl_key"
	value := []byte("ttl_value")
	ttl := 100 * time.Millisecond
	
	// Сохраняем с коротким TTL
	err := cache.SetWithTTL(key, value, ttl)
	if err != nil {
		t.Fatalf("Ошибка при сохранении с TTL: %v", err)
	}
	
	// Сразу должно быть доступно
	_, exists := cache.Get(key)
	if !exists {
		t.Fatal("Ключ не найден сразу после сохранения")
	}
	
	// Ждем истечения TTL
	time.Sleep(150 * time.Millisecond)
	
	// Теперь должно быть недоступно
	_, exists = cache.Get(key)
	if exists {
		t.Fatal("Ключ все еще доступен после истечения TTL")
	}
}

// TestLRUEviction тестирует LRU политику вытеснения
func TestLRUEviction(t *testing.T) {
	config := DefaultConfig()
	config.MaxSize = 3
	config.EvictionPolicy = LRU
	config.CleanupInterval = 0
	
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	// Заполняем кэш до лимита
	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))
	cache.Set("key3", []byte("value3"))
	
	// Обращаемся к key1 чтобы сделать его недавно использованным
	cache.Get("key1")
	
	// Добавляем еще один элемент - должен вытеснить key2 (самый старый)
	cache.Set("key4", []byte("value4"))
	
	// key2 должен быть вытеснен
	_, exists := cache.Get("key2")
	if exists {
		t.Fatal("key2 должен был быть вытеснен по LRU политике")
	}
	
	// key1 и key3 должны остаться
	_, exists = cache.Get("key1")
	if !exists {
		t.Fatal("key1 не должен был быть вытеснен")
	}
	
	_, exists = cache.Get("key3")
	if !exists {
		t.Fatal("key3 не должен был быть вытеснен")
	}
}

// TestLFUEviction тестирует LFU политику вытеснения
func TestLFUEviction(t *testing.T) {
	config := DefaultConfig()
	config.MaxSize = 3
	config.EvictionPolicy = LFU
	config.CleanupInterval = 0
	
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	// Заполняем кэш
	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))
	cache.Set("key3", []byte("value3"))
	
	// Делаем key1 часто используемым
	cache.Get("key1")
	cache.Get("key1")
	cache.Get("key1")
	
	// key2 используем один раз
	cache.Get("key2")
	
	// key3 не используем совсем
	
	// Добавляем новый элемент - должен вытеснить key3 (наименее часто используемый)
	cache.Set("key4", []byte("value4"))
	
	// key3 должен быть вытеснен
	_, exists := cache.Get("key3")
	if exists {
		t.Fatal("key3 должен был быть вытеснен по LFU политике")
	}
	
	// key1 и key2 должны остаться
	_, exists = cache.Get("key1")
	if !exists {
		t.Fatal("key1 не должен был быть вытеснен")
	}
	
	_, exists = cache.Get("key2")
	if !exists {
		t.Fatal("key2 не должен был быть вытеснен")
	}
}

// TestStats тестирует статистику кэша
func TestStats(t *testing.T) {
	config := DefaultConfig()
	config.MaxSize = 2
	config.CleanupInterval = 0
	
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	// Начальная статистика должна быть нулевой
	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Keys != 0 || stats.Evictions != 0 {
		t.Fatal("Начальная статистика должна быть нулевой")
	}
	
	// Добавляем элементы
	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))
	
	stats = cache.Stats()
	if stats.Keys != 2 {
		t.Fatalf("Ожидали 2 ключа, получили %d", stats.Keys)
	}
	
	// Попадание в кэш
	cache.Get("key1")
	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Fatalf("Ожидали 1 попадание, получили %d", stats.Hits)
	}
	
	// Промах кэша
	cache.Get("nonexistent")
	stats = cache.Stats()
	if stats.Misses != 1 {
		t.Fatalf("Ожидали 1 промах, получили %d", stats.Misses)
	}
	
	// Проверяем расчет hit rate
	if stats.HitRate != 50.0 {
		t.Fatalf("Ожидали hit rate 50%%, получили %.2f%%", stats.HitRate)
	}
	
	// Вытеснение (добавляем третий элемент)
	cache.Set("key3", []byte("value3"))
	stats = cache.Stats()
	if stats.Evictions != 1 {
		t.Fatalf("Ожидали 1 вытеснение, получили %d", stats.Evictions)
	}
}

// TestClear тестирует очистку кэша
func TestClear(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 0
	
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	// Добавляем элементы
	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))
	cache.Get("key1") // Создаем попадание для статистики
	
	stats := cache.Stats()
	if stats.Keys != 2 || stats.Hits != 1 {
		t.Fatal("Кэш должен содержать данные перед очисткой")
	}
	
	// Очищаем кэш
	cache.Clear()
	
	// Проверяем что все удалено
	_, exists := cache.Get("key1")
	if exists {
		t.Fatal("key1 должен был быть удален после Clear()")
	}
	
	_, exists = cache.Get("key2")
	if exists {
		t.Fatal("key2 должен был быть удален после Clear()")
	}
	
	// Проверяем что статистика сброшена
	stats = cache.Stats()
	if stats.Keys != 0 || stats.Hits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Fatal("Статистика должна быть сброшена после Clear()")
	}
}

// TestConcurrency тестирует потокобезопасность
func TestConcurrency(t *testing.T) {
	config := DefaultConfig()
	config.MaxSize = 1000
	config.CleanupInterval = 0
	
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	// Запускаем горутины для одновременного доступа
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := []byte(fmt.Sprintf("value_%d_%d", id, j))
				
				cache.Set(key, value)
				cache.Get(key)
			}
			done <- true
		}(i)
	}
	
	// Ждем завершения всех горутин
	for i := 0; i < 10; i++ {
		<-done
	}
	
	stats := cache.Stats()
	if stats.Keys == 0 {
		t.Fatal("Кэш должен содержать элементы после конкурентного доступа")
	}
}

// BenchmarkSet бенчмарк для операции Set
func BenchmarkSet(b *testing.B) {
	config := DefaultConfig()
	config.CleanupInterval = 0
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	value := []byte("benchmark_value")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i)
		cache.Set(key, value)
	}
}

// BenchmarkGet бенчмарк для операции Get
func BenchmarkGet(b *testing.B) {
	config := DefaultConfig()
	config.CleanupInterval = 0
	cache := NewMemoryCache(config)
	defer cache.Close()
	
	// Предварительно заполняем кэш
	value := []byte("benchmark_value")
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key_%d", i)
		cache.Set(key, value)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i%1000)
		cache.Get(key)
	}
}