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
func (s *Sessions) Username(token string) (string, bool) {
	s.mu.RLock()
	v, ok := s.values[token]
	s.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(v.Expires) {
		s.Delete(token)
		return "", false
	}
	return v.Username, true
}
func (s *Sessions) RenameUser(oldUsername, newUsername string) {
	if oldUsername == "" || newUsername == "" || oldUsername == newUsername {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, session := range s.values {
		if session.Username == oldUsername {
			session.Username = newUsername
			s.values[token] = session
		}
	}
}
func (s *Sessions) Clear() {
	s.mu.Lock()
	s.values = map[string]Session{}
	s.mu.Unlock()
}
func (s *Sessions) Delete(token string) { s.mu.Lock(); delete(s.values, token); s.mu.Unlock() }
