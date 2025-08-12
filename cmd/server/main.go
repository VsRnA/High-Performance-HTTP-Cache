package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/VsRnA/High-Performance-HTTP-Cache/internal/cache"
)

func main() {
	cacheEngine := cache.New()
	http.HandleFunc("/cache/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/cache/")
		if key == "" {
			http.Error(w, "Key is required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case "GET":
			value, exists := cacheEngine.Get(key)
			if !exists {
				http.Error(w, "Key not found", http.StatusNotFound)
				return
			}
			fmt.Fprint(w, value)

		case "PUT", "POST":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			ttlStr := r.URL.Query().Get("ttl")
			if ttlStr != "" {
				var ttlSeconds int
				_, err := fmt.Sscanf(ttlStr, "%d", &ttlSeconds)
				if err != nil {
					http.Error(w, "Invalid TTL format", http.StatusBadRequest)
					return
				}
				ttl := time.Duration(ttlSeconds) * time.Second
				cacheEngine.SetWithTTL(key, string(body), ttl)
				fmt.Fprintf(w, "Saved key: %s with TTL: %d seconds", key, ttlSeconds)
			} else {
				cacheEngine.Set(key, string(body))
				fmt.Fprintf(w, "Saved key: %s", key)
			}

		case "DELETE":
			if cacheEngine.Delete(key) {
				fmt.Fprintf(w, "Deleted key: %s", key)
			} else {
				http.Error(w, "Key not found", http.StatusNotFound)
			}

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
	})

	fmt.Println("Starting HTTP Cache server on :8080...")
	fmt.Println("Visit http://localhost:8080 for usage instructions")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}