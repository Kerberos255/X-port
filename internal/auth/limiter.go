package auth

import (
	"sync"
	"time"
)

const defaultLimiterCapacity = 8192

type loginAttempt struct {
	Count int
	Since time.Time
}

type LoginLimiter struct {
	mu          sync.Mutex
	entries     map[string]loginAttempt
	window      time.Duration
	max         int
	capacity    int
	lastCleanup time.Time
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return newLoginLimiter(max, window, defaultLimiterCapacity)
}

func newLoginLimiter(max int, window time.Duration, capacity int) *LoginLimiter {
	if max < 1 {
		max = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	if capacity < 1 {
		capacity = 1
	}
	return &LoginLimiter{
		entries:  map[string]loginAttempt{},
		max:      max,
		window:   window,
		capacity: capacity,
	}
}

func (l *LoginLimiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.cleanupLocked(now, false)
	a, ok := l.entries[key]
	if !ok {
		return true
	}
	if now.Sub(a.Since) >= l.window {
		delete(l.entries, key)
		return true
	}
	return a.Count < l.max
}

func (l *LoginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.cleanupLocked(now, false)
	a, ok := l.entries[key]
	if !ok || now.Sub(a.Since) >= l.window {
		l.makeRoomLocked(now, key)
		a = loginAttempt{Since: now}
	}
	a.Count++
	l.entries[key] = a
}

func (l *LoginLimiter) Success(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func (l *LoginLimiter) cleanupLocked(now time.Time, force bool) {
	interval := time.Minute
	if l.window < interval {
		interval = l.window
	}
	if !force && !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < interval {
		return
	}
	for key, a := range l.entries {
		if a.Since.IsZero() || now.Sub(a.Since) >= l.window {
			delete(l.entries, key)
		}
	}
	l.lastCleanup = now
}

func (l *LoginLimiter) makeRoomLocked(now time.Time, key string) {
	if _, exists := l.entries[key]; exists {
		return
	}
	if len(l.entries) < l.capacity {
		return
	}
	l.cleanupLocked(now, true)
	if len(l.entries) < l.capacity {
		return
	}
	// Keep memory bounded even under distributed username/IP churn. Evicting the
	// oldest bucket is preferable to letting an attacker grow the map without
	// bound; the independent per-IP limiter still provides a second throttle.
	oldestKey := ""
	oldestSince := now
	for k, a := range l.entries {
		if oldestKey == "" || a.Since.Before(oldestSince) {
			oldestKey = k
			oldestSince = a.Since
		}
	}
	if oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}
