// Package ratelimit is a small in-process fixed-window limiter for auth
// endpoints. State is per replica: a multi-instance deployment needs a
// shared limiter, but a single process still blocks password guessing and
// reset-mail floods.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	count int
	reset time.Time
}

// Limiter caps attempts per key inside a fixed window. Max <= 0 disables it.
type Limiter struct {
	mu        sync.Mutex
	max       int
	window    time.Duration
	buckets   map[string]*bucket
	lastSweep time.Time
}

func New(max int, window time.Duration) *Limiter {
	return &Limiter{
		max:       max,
		window:    window,
		buckets:   map[string]*bucket{},
		lastSweep: time.Now(),
	}
}

// Allow checks every key first, then consumes one attempt from each. A
// rejection consumes nothing, so a limited caller cannot extend another
// key's window by retrying.
func (l *Limiter) Allow(keys ...string) (bool, time.Duration) {
	if l == nil || l.max <= 0 || l.window <= 0 || len(keys) == 0 {
		return true, 0
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(now)

	var retry time.Duration
	for _, key := range keys {
		b := l.buckets[key]
		if b == nil || now.After(b.reset) {
			continue
		}
		if b.count >= l.max {
			if d := b.reset.Sub(now); d > retry {
				retry = d
			}
		}
	}
	if retry > 0 {
		return false, retry
	}
	for _, key := range keys {
		b := l.buckets[key]
		if b == nil || now.After(b.reset) {
			b = &bucket{reset: now.Add(l.window)}
			l.buckets[key] = b
		}
		b.count++
	}
	return true, 0
}

func (l *Limiter) sweepLocked(now time.Time) {
	// Bound memory even under key churn (rotating emails/IPs): sweep once
	// per window, and hard-clear if a single window holds an absurd number
	// of keys. Clearing fails open; the alternative is unbounded growth.
	if now.Sub(l.lastSweep) < l.window && len(l.buckets) < 10000 {
		return
	}
	for key, b := range l.buckets {
		if now.After(b.reset) {
			delete(l.buckets, key)
		}
	}
	l.lastSweep = now
	if len(l.buckets) >= 10000 {
		l.buckets = map[string]*bucket{}
	}
}
