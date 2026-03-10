package ratelimit

import (
	"sync"
	"time"
)

type entry struct {
	count int
	until time.Time
}

// Limiter in-memory rate limiter. Key -> count within window.
type Limiter struct {
	mu       sync.Mutex
	entries  map[string]*entry
	limit    int
	window   time.Duration
	cleanup  time.Time
}

// New creates a limiter: max `limit` requests per `window` per key.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		entries: make(map[string]*entry),
		limit:   limit,
		window:  window,
	}
}

// Allow returns true if the key is under the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.cleanup.Before(now) {
		l.cleanup = now.Add(l.window)
		for k, e := range l.entries {
			if e.until.Before(now) {
				delete(l.entries, k)
			}
		}
	}
	e, ok := l.entries[key]
	if !ok || e.until.Before(now) {
		l.entries[key] = &entry{count: 1, until: now.Add(l.window)}
		return true
	}
	if e.count >= l.limit {
		return false
	}
	e.count++
	return true
}

// RetryAfter returns seconds until the key can try again, or 0 if allowed now.
func (l *Limiter) RetryAfter(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || e.until.Before(time.Now()) {
		return 0
	}
	if e.count < l.limit {
		return 0
	}
	return int(time.Until(e.until).Seconds())
}
