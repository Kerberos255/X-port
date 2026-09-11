package auth

import "time"

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
