package auth

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Limiter is a fixed-window in-memory rate limiter keyed by an arbitrary
// string (typically client IP). It is intentionally per-process: adequate for
// a single-node deployment and replaced by a shared store when the API scales
// horizontally.
type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	buckets map[string]*windowBucket
}

type windowBucket struct {
	count int
	reset time.Time
}

// NewLimiter creates a limiter allowing `limit` events per `window` per key.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		now:     time.Now,
		buckets: make(map[string]*windowBucket),
	}
}

// Allow records an event for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok || now.After(b.reset) {
		l.buckets[key] = &windowBucket{count: 1, reset: now.Add(l.window)}
		l.prune(now)
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

// Reset clears the bucket for key (e.g. after a successful login).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// prune drops expired buckets; called under lock when a new bucket is created.
func (l *Limiter) prune(now time.Time) {
	if len(l.buckets) < 1024 {
		return
	}
	for k, b := range l.buckets {
		if now.After(b.reset) {
			delete(l.buckets, k)
		}
	}
}

// rateLimit returns middleware that rejects requests over the limit with 429.
func rateLimit(l *Limiter, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(keyFn(c)) {
			httpx.Error(c, httpx.RateLimited("Too many requests. Please try again later."))
			c.Abort()
			return
		}
		c.Next()
	}
}
