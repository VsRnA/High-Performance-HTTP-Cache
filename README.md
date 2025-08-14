# High-Performance-HTTP-Cache

Высокопроизводительная библиотека кэширования для Go с поддержкой различных стратегий eviction и TTL.


## 🚀 Особенности

- **Множественные реализации**: Simple, LRU, LFU кэши
- **TTL поддержка**: Автоматическое истечение элементов
- **Потокобезопасность**: Все операции thread-safe
- **Высокая производительность**: Оптимизированные структуры данных
- **Детальная статистика**: Метрики попаданий, промахов, eviction
- **Без внешних зависимостей**: Только стандартная библиотека Go

## 📦 Установка

```bash
go get github.com/VsRnA/High-Performance-HTTP-Cache
```

## 🎯 Быстрый старт

```go
package main

import (
    "fmt"
    "time"
    
    "github.com/VsRnA/High-Performance-HTTP-Cache/memory"
)

func main() {
    // Создаем LRU кэш на 1000 элементов
    cache := memory.NewLRU(1000)
    defer cache.Close()
    
    // Сохраняем данные
    cache.Set("user:123", []byte(`{"name": "Alice", "email": "alice@example.com"}`))
    
    // Получаем данные
    if data, exists := cache.Get("user:123"); exists {
        fmt.Printf("User data: %s\n", string(data))
    }
    
    // Сохраняем с TTL
    cache.SetWithTTL("session:abc", []byte("session_data"), 5*time.Minute)
    
    // Статистика
    stats := cache.Stats()
    fmt.Printf("Hit rate: %.2f%%\n", stats.HitRate)
}
```

## 📚 Доступные реализации

### Simple Cache
Базовый кэш без политик вытеснения. Максимальная производительность.

```go
cache := memory.NewSimple()
cache := memory.NewSimpleWithTTL(10 * time.Minute) // С TTL по умолчанию
```

**Использовать когда:**
- Размер кэша контролируется приложением
- Нужна максимальная производительность
- Нет ограничений по памяти

### LRU Cache (Least Recently Used)
Вытесняет наименее недавно использованные элементы.

```go
cache := memory.NewLRU(1000)                                    // 1000 элементов
cache := memory.NewLRUWithTTL(1000, 30 * time.Minute)         // С TTL
```

**Использовать когда:**
- Есть temporal locality (недавние данные используются чаще)
- Нужен контроль размера кэша
- Классический выбор для большинства случаев

### LFU Cache (Least Frequently Used)
Вытесняет наименее часто используемые элементы.

```go
cache := memory.NewLFU(1000)
cache := memory.NewLFUWithTTL(1000, 1 * time.Hour)
```

**Использовать когда:**
- Есть популярные данные которые используются постоянно
- Частота важнее времени доступа
- Долгосрочное кэширование

### Основные операции

```go
// Сохранение
err := cache.Set(key, value)
err := cache.SetWithTTL(key, value, ttl)

// Получение
value, exists := cache.Get(key)

// Удаление
deleted := cache.Delete(key)

// Очистка
cache.Clear()

// Статистика
stats := cache.Stats()

// Размер
size := cache.Size() // Только для размерных интерфейсов

// Закрытие
cache.Close()
```

### Статистика

```go
stats := cache.Stats()
fmt.Printf("Попадания: %d\n", stats.Hits)
fmt.Printf("Промахи: %d\n", stats.Misses) 
fmt.Printf("Процент попаданий: %.2f%%\n", stats.HitRate)
fmt.Printf("Элементов: %d\n", stats.Keys)
fmt.Printf("Вытеснений: %d\n", stats.Evictions)
```

## 📊 Сравнение производительности

| Реализация | Set ops/sec | Get ops/sec | Смешанный доступ | Память |
|------------|-------------|-------------|------------------|--------|
| Simple     | 2.0M        | 4.7M        | 3.2M            | Низкое |
| LRU        | 2.3M        | 4.7M        | 4.0M            | Среднее |
| LFU        | 2.0M        | 4.9M        | 34K*            | Среднее |

*LFU медленнее в смешанном режиме из-за поиска минимальной частоты

## 🎮 Примеры использования

### Кэширование пользователей

```go
type UserCache struct {
    cache cache.Cache
}

func NewUserCache() *UserCache {
    return &UserCache{
        cache: memory.NewLRUWithTTL(10000, 1*time.Hour),
    }
}

func (uc *UserCache) GetUser(id string) (*User, error) {
    // Проверяем кэш
    if data, exists := uc.cache.Get("user:" + id); exists {
        user := &User{}
        json.Unmarshal(data, user)
        return user, nil
    }
    
    // Загружаем из БД
    user, err := loadUserFromDB(id)
    if err != nil {
        return nil, err
    }
    
    // Сохраняем в кэш
    data, _ := json.Marshal(user)
    uc.cache.Set("user:"+id, data)
    
    return user, nil
}
```

### Кэширование API ответов

```go
func cacheMiddleware(cache cache.Cache, ttl time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.Request.URL.Path + "?" + c.Request.URL.RawQuery
        
        // Проверяем кэш
        if data, exists := cache.Get(key); exists {
            c.Data(200, "application/json", data)
            return
        }
        
        // Перехватываем ответ
        w := &responseWriter{ResponseWriter: c.Writer}
        c.Writer = w
        c.Next()
        
        // Сохраняем в кэш если статус 200
        if c.Writer.Status() == 200 {
            cache.SetWithTTL(key, w.body, ttl)
        }
    }
}
```

### Сессионный кэш

```go
type SessionManager struct {
    cache cache.Cache
}

func NewSessionManager() *SessionManager {
    return &SessionManager{
        cache: memory.NewSimpleWithTTL(30 * time.Minute),
    }
}

func (sm *SessionManager) CreateSession(userID string) string {
    sessionID := generateSessionID()
    sessionData := map[string]interface{}{
        "user_id": userID,
        "created": time.Now(),
    }
    
    data, _ := json.Marshal(sessionData)
    sm.cache.Set(sessionID, data)
    
    return sessionID
}

func (sm *SessionManager) GetSession(sessionID string) map[string]interface{} {
    if data, exists := sm.cache.Get(sessionID); exists {
        var session map[string]interface{}
        json.Unmarshal(data, &session)
        return session
    }
    return nil
}
```

## 🧪 Тестирование

```bash
go test -v ./memory
```

## 📈 Мониторинг

```go
// Периодический вывод статистики
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    for range ticker.C {
        stats := cache.Stats()
        log.Printf("Cache stats: hits=%d, misses=%d, hit_rate=%.2f%%", 
            stats.Hits, stats.Misses, stats.HitRate)
    }
}()
```

## ⚙️ Конфигурация

### Выбор размера кэша

```go
// Для небольших приложений
cache := memory.NewLRU(100)

// Для средних приложений  
cache := memory.NewLRU(10000)

// Для высоконагруженных систем
cache := memory.NewLRU(100000)
```

### TTL стратегии

```go
// Короткий TTL для часто изменяющихся данных
cache.SetWithTTL("prices", data, 1*time.Minute)

// Средний TTL для пользовательских данных
cache.SetWithTTL("user:123", data, 1*time.Hour)

// Длинный TTL для статических данных
cache.SetWithTTL("config", data, 24*time.Hour)
```


## 👥 Авторы

- **Васенин Роман**  - [VsRnA](https://github.com/VsRnA)
