package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Kerberos255/X-port/internal/service"
	sysinfo "github.com/Kerberos255/X-port/internal/system"
)

func (s *Server) registerUXRoutes(mux *http.ServeMux) {
	register := func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, s.require(handler)) }
	register("PATCH /api/accounts/{id}/enabled", s.setAccountEnabled)
	register("GET /api/accounts/online", s.accountOnlineConnections)
	register("GET /api/accounts/port-suggestion", s.suggestAccountPort)
	register("GET /api/accounts/{id}/advanced", s.getAccountAdvanced)
	register("PUT /api/accounts/{id}/advanced", s.updateAccountAdvanced)
	register("GET /api/accounts/{id}/expert", s.getAccountExpert)
	register("PUT /api/accounts/{id}/expert", s.updateAccountExpert)
	register("GET /api/xray/config", s.getXrayGlobalConfig)
	register("PUT /api/xray/config", s.updateXrayGlobalConfig)
	register("GET /api/xray/rollback", s.checkXrayRollback)
	register("POST /api/xray/rollback", s.rollbackXray)
}

func (s *Server) setAccountEnabled(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if decodeJSON(w, r, &req) != nil {
		return
	}
	view, err := s.accounts.SetEnabled(id, req.Enabled)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) accountOnlineConnections(w http.ResponseWriter, r *http.Request) {
	views, err := s.accounts.List()
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	ports := make(map[int]struct{}, len(views))
	for _, v := range views {
		ports[v.Port] = struct{}{}
	}
	activity := sysinfo.EstablishedTCPActivityByLocalPort(ports)
	connections := make(map[string]int, len(views))
	peers := make(map[string]int, len(views))
	for _, v := range views {
		a := activity[v.Port]
		id := strconv.FormatInt(v.ID, 10)
		connections[id] = a.Connections
		peers[id] = a.Peers
	}
	writeJSON(w, 200, map[string]any{"connections": connections, "peers": peers})
}

func (s *Server) suggestAccountPort(w http.ResponseWriter, r *http.Request) {
	port, minPort, maxPort, err := s.accounts.SuggestPort()
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{"port": port, "min": minPort, "max": maxPort})
}

func (s *Server) getAccountAdvanced(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	view, err := s.accounts.Advanced(id)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) updateAccountAdvanced(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	var req service.AdvancedConfig
	if decodeJSON(w, r, &req) != nil {
		return
	}
	view, err := s.accounts.UpdateAdvanced(id, req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) getAccountExpert(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	view, err := s.accounts.Expert(id)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) updateAccountExpert(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	var req service.ExpertConfig
	if decodeJSON(w, r, &req) != nil {
		return
	}
	view, err := s.accounts.UpdateExpert(id, req)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, view)
}

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

func (s *Server) checkXrayRollback(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil {
		writeError(w, 503, "updater not configured")
		return
	}
	writeJSON(w, 200, s.updater.CheckRollback())
}

func (s *Server) rollbackXray(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil {
		writeError(w, 503, "updater not configured")
		return
	}
	info, err := s.updater.Rollback()
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, info)
}
