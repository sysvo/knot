// Package ratelimit provides a small in-process token bucket. It exists to make
// brute-forcing passwords and hammering the post button slow, not to be a
// distributed rate limiter.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter holds one bucket per key.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

// New returns an empty limiter.
func New() *Limiter {
	return &Limiter{buckets: map[string]*bucket{}, now: time.Now}
}

// Allow consumes one token from key's bucket and reports whether the request
// may proceed. rate is tokens per second and burst is the bucket size.
func (l *Limiter) Allow(key string, rate float64, burst int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(burst), last: now}
		l.buckets[key] = b
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = min(float64(burst), b.tokens+elapsed*rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Purge drops buckets that have been idle for longer than idle. It is called
// periodically so that a stream of unique keys cannot grow the map forever.
func (l *Limiter) Purge(idle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := l.now().Add(-idle)
	for k, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// Len reports the number of tracked keys (used by tests).
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
