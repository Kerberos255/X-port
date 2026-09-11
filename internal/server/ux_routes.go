package server

import (
	"net/http"

	"github.com/Kerberos255/X-port/internal/service"
)

func (s *Server) registerUXRoutes(mux *http.ServeMux) {
	mux.Handle("PATCH /api/accounts/{id}/enabled", s.require(http.HandlerFunc(s.setAccountEnabled)))
	mux.Handle("GET /api/accounts/{id}/advanced", s.require(http.HandlerFunc(s.getAccountAdvanced)))
	mux.Handle("PUT /api/accounts/{id}/advanced", s.require(http.HandlerFunc(s.updateAccountAdvanced)))
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
