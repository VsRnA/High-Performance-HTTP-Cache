// Package internal содержит внутренние утилиты для библиотеки кэша
package internal

import (
	"hash/fnv"
)

// Hash64 вычисляет 64-битный хеш строки используя FNV-1a алгоритм
// Используется для шардинга и распределенного кэширования
func Hash64(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// Hash32 вычисляет 32-битный хеш строки
func Hash32(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// ShardIndex возвращает индекс шарда для ключа
// shardCount должен быть степенью 2 для эффективности
func ShardIndex(key string, shardCount int) int {
	if shardCount <= 1 {
		return 0
	}
	
	hash := Hash64(key)
	return int(hash) & (shardCount - 1) // Быстрое вычисление остатка для степеней 2
}

// IsPowerOfTwo проверяет является ли число степенью двойки
func IsPowerOfTwo(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}

// NextPowerOfTwo возвращает следующую степень двойки больше или равную n
func NextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	
	// Если уже степень двойки
	if IsPowerOfTwo(n) {
		return n
	}
	
	// Находим следующую степень двойки
	power := 1
	for power < n {
		power <<= 1
	}
	return power
}