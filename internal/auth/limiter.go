package auth

import (
	"sync"
	"time"
)

type loginAttempt struct {
	Count int
	Since time.Time
}

type LoginLimiter struct {
	mu      sync.Mutex
	entries map[string]loginAttempt
	window  time.Duration
	max     int
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{entries: map[string]loginAttempt{}, max: max, window: window}
}

func (l *LoginLimiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	a := l.entries[key]
	if a.Since.IsZero() || now.Sub(a.Since) >= l.window {
		delete(l.entries, key)
		return true
	}
	return a.Count < l.max
}

func (l *LoginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	a := l.entries[key]
	if a.Since.IsZero() || now.Sub(a.Since) >= l.window {
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
