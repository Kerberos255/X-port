package service

import "fmt"

func (s *Accounts) SuggestPort() (port, minPort, maxPort int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	accounts, err := s.store.Accounts()
	if err != nil {
		return 0, 0, 0, err
	}
	minPort, maxPort, apiPort := s.portDefaults()
	port = nextFreePort(accounts, minPort, maxPort, apiPort)
	if port == 0 {
		return 0, minPort, maxPort, fmt.Errorf("no automatic port available in %d-%d", minPort, maxPort)
	}
	return port, minPort, maxPort, nil
}
