package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type Session struct {
	Username string
	Expires  time.Time
}

type Sessions struct {
	mu     sync.RWMutex
	values map[string]Session
	ttl    time.Duration
}

func NewSessions(ttl time.Duration) *Sessions {
	return &Sessions{values: map[string]Session{}, ttl: ttl}
}
func (s *Sessions) Create(username string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	s.mu.Lock()
	s.values[token] = Session{Username: username, Expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return token, nil
}
func (s *Sessions) Valid(token string) bool {
	s.mu.RLock()
	v, ok := s.values[token]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(v.Expires) {
		s.Delete(token)
		return false
	}
	return true
}
func (s *Sessions) Delete(token string) { s.mu.Lock(); delete(s.values, token); s.mu.Unlock() }
