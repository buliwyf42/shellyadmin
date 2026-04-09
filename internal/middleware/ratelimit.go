package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type attemptStore struct {
	mu   sync.Mutex
	data map[string][]time.Time
	hits int
}

var attempts = &attemptStore{data: map[string][]time.Time{}}
var apiAttempts = &attemptStore{data: map[string][]time.Time{}}

const (
	loginWindow         = 60 * time.Second
	loginMaxAttempts    = 10
	apiWindow           = 60 * time.Second
	apiMaxRequests      = 300
	rateLimiterMaxKeys  = 4096
	rateLimiterEvictAge = 10 * time.Minute
	rateLimiterGCEveryN = 128
)

func LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !allowRequest(attempts, c.ClientIP(), loginWindow, loginMaxAttempts) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many login attempts"})
			return
		}
		c.Next()
	}
}

func APIRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !allowRequest(apiAttempts, c.ClientIP(), apiWindow, apiMaxRequests) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many api requests"})
			return
		}
		c.Next()
	}
}

func allowRequest(store *attemptStore, key string, window time.Duration, max int) bool {
	now := time.Now()
	windowStart := now.Add(-window)

	store.mu.Lock()
	defer store.mu.Unlock()

	store.hits++
	gcNeeded := store.hits%rateLimiterGCEveryN == 0
	recent := store.data[key][:0]
	for _, ts := range store.data[key] {
		if ts.After(windowStart) {
			recent = append(recent, ts)
		}
	}
	if len(recent) >= max {
		store.data[key] = recent
		if gcNeeded {
			compactStore(store, now)
		}
		return false
	}
	store.data[key] = append(recent, now)
	if gcNeeded || len(store.data) > rateLimiterMaxKeys {
		compactStore(store, now)
	}
	return true
}

func compactStore(store *attemptStore, now time.Time) {
	cutoff := now.Add(-rateLimiterEvictAge)
	for key, entries := range store.data {
		keep := entries[:0]
		for _, ts := range entries {
			if ts.After(cutoff) {
				keep = append(keep, ts)
			}
		}
		if len(keep) == 0 {
			delete(store.data, key)
			continue
		}
		store.data[key] = keep
	}
}
