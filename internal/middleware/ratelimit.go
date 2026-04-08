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
}

var attempts = &attemptStore{data: map[string][]time.Time{}}

func LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		attempts.mu.Lock()
		defer attempts.mu.Unlock()
		windowStart := now.Add(-60 * time.Second)
		recent := attempts.data[ip][:0]
		for _, ts := range attempts.data[ip] {
			if ts.After(windowStart) {
				recent = append(recent, ts)
			}
		}
		if len(recent) >= 10 {
			attempts.data[ip] = recent
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many login attempts"})
			return
		}
		attempts.data[ip] = append(recent, now)
		c.Next()
	}
}
