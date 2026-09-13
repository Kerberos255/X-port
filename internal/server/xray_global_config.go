package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) getXrayGlobalConfig(w http.ResponseWriter, r *http.Request) {
	raw, err := s.accounts.GlobalXrayConfig()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	var config any
	if err := json.Unmarshal(raw, &config); err != nil {
		writeError(w, 500, "stored global Xray config is invalid")
		return
	}
	writeJSON(w, 200, map[string]any{"config": config})
}

func (s *Server) updateXrayGlobalConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config json.RawMessage `json:"config"`
	}
	if decodeJSON(w, r, &req) != nil {
		return
	}
	if len(req.Config) == 0 {
		writeError(w, 400, "config is required")
		return
	}
	normalized, err := s.accounts.UpdateGlobalXrayConfig(string(req.Config))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	var config any
	if err := json.Unmarshal(normalized, &config); err != nil {
		writeError(w, 500, "normalized global Xray config is invalid")
		return
	}
	writeJSON(w, 200, map[string]any{"config": config, "applied": true})
}
