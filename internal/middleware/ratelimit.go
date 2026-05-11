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
var mcpAttempts = &attemptStore{data: map[string][]time.Time{}}

const (
	loginWindow         = 60 * time.Second
	loginMaxAttempts    = 10
	apiWindow           = 60 * time.Second
	apiMaxRequests      = 300
	mcpWindow           = 60 * time.Second
	mcpMaxRequests      = 300
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

// MCPRateLimitMiddleware wraps a net/http.Handler with the same
// token-bucket logic as APIRateLimit but a separate counter store, so
// MCP traffic and SPA-API traffic do not exhaust each other's budget.
// Used by the MCP listener (cmd/shellyctl/main.go wires this) which
// runs on a standalone *http.Server outside the gin router. S8 from
// the consolidated review — without this a stolen MCP token has
// unbounded request rate even though the SPA-API is rate-limited.
func MCPRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIPFromRequest(r)
		if !allowRequest(mcpAttempts, ip, mcpWindow, mcpMaxRequests) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"too many mcp requests"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIPFromRequest returns the request's peer address. MCP runs on
// its own listener that is not behind the gin TrustedProxies machinery,
// so we read the raw RemoteAddr — X-Forwarded-For from the MCP client
// is intentionally ignored, otherwise any client could spoof the
// rate-limit key.
func clientIPFromRequest(r *http.Request) string {
	host := r.RemoteAddr
	if i := lastIndexByte(host, ':'); i >= 0 {
		return host[:i]
	}
	return host
}

// lastIndexByte avoids importing strings just for this one call.
func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
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
