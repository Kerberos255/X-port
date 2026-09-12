package server

import (
	"net/http"
	"strconv"

	"github.com/Kerberos255/X-port/internal/service"
	sysinfo "github.com/Kerberos255/X-port/internal/system"
)

func (s *Server) registerUXRoutes(mux *http.ServeMux) {
	mux.Handle("PATCH /api/accounts/{id}/enabled", s.require(http.HandlerFunc(s.setAccountEnabled)))
	mux.Handle("GET /api/accounts/online", s.require(http.HandlerFunc(s.accountOnlineConnections)))
	mux.Handle("GET /api/accounts/{id}/advanced", s.require(http.HandlerFunc(s.getAccountAdvanced)))
	mux.Handle("PUT /api/accounts/{id}/advanced", s.require(http.HandlerFunc(s.updateAccountAdvanced)))
	mux.Handle("GET /api/accounts/{id}/expert", s.require(http.HandlerFunc(s.getAccountExpert)))
	mux.Handle("PUT /api/accounts/{id}/expert", s.require(http.HandlerFunc(s.updateAccountExpert)))
	mux.Handle("GET /api/xray/rollback", s.require(http.HandlerFunc(s.checkXrayRollback)))
	mux.Handle("POST /api/xray/rollback", s.require(http.HandlerFunc(s.rollbackXray)))
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
