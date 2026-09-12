package server

import "net/http"

func (s *Server) suggestAccountPort(w http.ResponseWriter, r *http.Request) {
	port, minPort, maxPort, err := s.accounts.SuggestPort()
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{"port": port, "min": minPort, "max": maxPort})
}
