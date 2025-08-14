// Сравнение различных реализаций кэша
package main

import (
	"fmt"
	"strings"
	"time"

	cache "github.com/VsRnA/High-Performance-HTTP-Cache"
	"github.com/VsRnA/High-Performance-HTTP-Cache/memory"
)

func main() {
	fmt.Printf("=== Сравнение реализаций кэша ===\n")
	
	// Тестируем разные стратегии вытеснения
	testEvictionPolicies()
	
	// Тестируем производительность
	fmt.Println("\n" + strings.Repeat("=", 50))
	testPerformance()
}

// testEvictionPolicies демонстрирует различия в политиках вытеснения
func testEvictionPolicies() {
	fmt.Println("1. Сравнение политик вытеснения")
	fmt.Printf("Кэш на 3 элемента, добавляем 5 элементов с разными паттернами доступа\n")
	
	// Создаем кэши
	lruCache := memory.NewLRU(3)
	lfuCache := memory.NewLFU(3)
	defer lruCache.Close()
	defer lfuCache.Close()
	
	// Сценарий: добавляем элементы и создаем разные паттерны доступа
	caches := map[string]cache.Cache{
		"LRU": lruCache,
		"LFU": lfuCache,
	}
	
	for name, c := range caches {
		fmt.Printf("--- %s кэш ---\n", name)
		
		// Добавляем 3 элемента
		c.Set("A", []byte("data A"))
		c.Set("B", []byte("data B"))
		c.Set("C", []byte("data C"))
		
		// Создаем разные паттерны доступа
		// A - используем часто (5 раз)
		for i := 0; i < 5; i++ {
			c.Get("A")
		}
		// B - используем средне (2 раза)
		c.Get("B")
		c.Get("B")
		// C - используем редко (1 раз)
		c.Get("C")
		
		fmt.Printf("После доступов: A(5x), B(2x), C(1x)\n")
		fmt.Printf("Элементов: %d\n", c.Stats().Keys)
		
		// Добавляем новый элемент - кто будет вытеснен?
		c.Set("D", []byte("data D"))
		fmt.Printf("Добавили элемент D\n")
		
		// Проверяем что осталось
		remaining := []string{}
		for _, key := range []string{"A", "B", "C", "D"} {
			if _, exists := c.Get(key); exists {
				remaining = append(remaining, key)
			}
		}
		
		stats := c.Stats()
		fmt.Printf("Осталось: %v\n", remaining)
		fmt.Printf("Вытеснений: %d\n", stats.Evictions)
		fmt.Println()
	}
}

// testPerformance простое сравнение производительности
func testPerformance() {
	fmt.Println("2. Сравнение производительности")
	
	cacheTypes := map[string]func() cache.Cache{
		"Simple": func() cache.Cache { return memory.NewSimple() },
		"LRU":    func() cache.Cache { return memory.NewLRU(10000) },
		"LFU":    func() cache.Cache { return memory.NewLFU(10000) },
	}
	
	operations := 10000
	
	for name, constructor := range cacheTypes {
		fmt.Printf("\n--- %s кэш ---\n", name)
		
		cache := constructor()
		
		// Тест записи
		start := time.Now()
		for i := 0; i < operations; i++ {
			key := fmt.Sprintf("key:%d", i)
			value := []byte(fmt.Sprintf("value:%d", i))
			cache.Set(key, value)
		}
		writeTime := time.Since(start)
		
		// Тест чтения (читаем все что записали)
		start = time.Now()
		hits := 0
		for i := 0; i < operations; i++ {
			key := fmt.Sprintf("key:%d", i)
			if _, exists := cache.Get(key); exists {
				hits++
			}
		}
		readTime := time.Since(start)
		
		// Тест смешанного доступа (70% чтения, 30% записи)
		start = time.Now()
		for i := 0; i < operations; i++ {
			if i%10 < 7 { // 70% чтения
				key := fmt.Sprintf("key:%d", i%1000) // Читаем из первой 1000
				cache.Get(key)
			} else { // 30% записи
				key := fmt.Sprintf("new_key:%d", i)
				cache.Set(key, []byte("new value"))
			}
		}
		mixedTime := time.Since(start)
		
		stats := cache.Stats()
		
		fmt.Printf("Запись %d элементов: %v (%.0f ops/sec)\n", 
			operations, writeTime, float64(operations)/writeTime.Seconds())
		fmt.Printf("Чтение %d элементов: %v (%.0f ops/sec)\n", 
			operations, readTime, float64(operations)/readTime.Seconds())
		fmt.Printf("Смешанный доступ: %v (%.0f ops/sec)\n", 
			mixedTime, float64(operations)/mixedTime.Seconds())
		fmt.Printf("Финальная статистика: %d ключей, %.1f%% hit rate\n", 
			stats.Keys, stats.HitRate)
		
		cache.Close()
	}
}

// demonstrateTTL показывает работу с TTL в разных кэшах
func demonstrateTTL() {
	fmt.Println("\n3. Демонстрация TTL")
	
	// Создаем кэши с TTL по умолчанию
	caches := map[string]cache.Cache{
		"Simple": memory.NewSimpleWithTTL(2 * time.Second),
		"LRU":    memory.NewLRUWithTTL(100, 2*time.Second),
		"LFU":    memory.NewLFUWithTTL(100, 2*time.Second),
	}
	
	defer func() {
		for _, c := range caches {
			c.Close()
		}
	}()
	
	// Добавляем данные во все кэши
	for name, c := range caches {
		c.Set("temp_data", []byte("will expire"))
		c.SetWithTTL("custom_ttl", []byte("custom expiration"), 1*time.Second)
		
		fmt.Printf("%s: добавили данные с TTL\n", name)
	}
	
	// Ждем истечения custom_ttl
	time.Sleep(1500 * time.Millisecond)
	
	fmt.Println("\nПосле 1.5 секунд:")
	for name, c := range caches {
		_, exists1 := c.Get("temp_data")
		_, exists2 := c.Get("custom_ttl")
		fmt.Printf("%s: temp_data=%v, custom_ttl=%v\n", name, exists1, exists2)
	}
	
	// Ждем истечения остальных
	time.Sleep(1 * time.Second)
	
	fmt.Println("\nПосле 2.5 секунд:")
	for name, c := range caches {
		_, exists1 := c.Get("temp_data")
		_, exists2 := c.Get("custom_ttl")
		fmt.Printf("%s: temp_data=%v, custom_ttl=%v\n", name, exists1, exists2)
	}
}