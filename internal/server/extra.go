package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"rsc.io/qr"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/backup"
	"github.com/Kerberos255/X-port/internal/buildinfo"
	"github.com/Kerberos255/X-port/internal/ops"
	"github.com/Kerberos255/X-port/internal/selfupdate"
)

func (s *Server) registerExtraRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/accounts/export", s.require(http.HandlerFunc(s.exportAccounts)))
	mux.Handle("GET /api/xray/geodata", s.require(http.HandlerFunc(s.checkGeodata)))
	mux.Handle("POST /api/xray/geodata", s.require(http.HandlerFunc(s.updateGeodata)))
	mux.Handle("POST /api/backups/import", s.require(http.HandlerFunc(s.importBackup)))
	mux.Handle("POST /api/xport/restart", s.require(http.HandlerFunc(s.restartPanel)))
	mux.Handle("GET /api/xport/update", s.require(http.HandlerFunc(s.checkPanelUpdate)))
	mux.Handle("POST /api/xport/update", s.require(http.HandlerFunc(s.updatePanel)))
}

func (s *Server) exportAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.store.Accounts()
	if err != nil { writeError(w, 500, "database error"); return }
	host := shareHost(r)
	if strings.TrimSpace(host) == "" { writeError(w, 400, "host is required"); return }
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	links, warnings := &strings.Builder{}, &strings.Builder{}
	exported := 0
	for _, a := range accounts {
		uri, err := accountcfg.ShareURI(a, host)
		if err != nil { fmt.Fprintf(warnings, "%s (:%d): %v\n", a.Name, a.Port, err); continue }
		exported++
		fmt.Fprintln(links, uri)
		code, err := qr.Encode(uri, qr.M)
		if err != nil { fmt.Fprintf(warnings, "%s (:%d): QR generation failed: %v\n", a.Name, a.Port, err); continue }
		code.Scale = 6
		name := fmt.Sprintf("qr/%02d-%s.png", exported, safeExportName(a.Name))
		entry, err := zw.Create(name); if err != nil { _ = zw.Close(); writeError(w, 500, err.Error()); return }
		if _, err := entry.Write(code.PNG()); err != nil { _ = zw.Close(); writeError(w, 500, err.Error()); return }
	}
	entry, err := zw.Create("links.txt"); if err != nil { writeError(w, 500, err.Error()); return }
	if _, err := io.WriteString(entry, links.String()); err != nil { writeError(w, 500, err.Error()); return }
	if warnings.Len() > 0 {
		entry, err = zw.Create("warnings.txt"); if err != nil { writeError(w, 500, err.Error()); return }
		if _, err := io.WriteString(entry, warnings.String()); err != nil { writeError(w, 500, err.Error()); return }
	}
	if err := zw.Close(); err != nil { writeError(w, 500, err.Error()); return }
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="xport-accounts.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Xport-Exported", fmt.Sprintf("%d", exported))
	w.WriteHeader(http.StatusOK); _, _ = w.Write(buf.Bytes())
}

var exportNameRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
func safeExportName(name string) string { name = strings.Trim(exportNameRE.ReplaceAllString(strings.TrimSpace(name), "-"), "-._"); if name == "" { return "account" }; if len(name) > 64 { name = name[:64] }; return name }

func (s *Server) checkGeodata(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil { writeError(w, 503, "updater not configured"); return }
	ctx, cancel := contextWithTimeout(r, 12*time.Second); defer cancel()
	info, _, err := s.updater.CheckGeodata(ctx); if err != nil { writeError(w, 502, err.Error()); return }
	writeJSON(w, 200, info)
}
func (s *Server) updateGeodata(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil { writeError(w, 503, "updater not configured"); return }
	ctx, cancel := contextWithTimeout(r, 2*time.Minute); defer cancel()
	info, err := s.updater.UpdateGeodata(ctx); if err != nil { writeError(w, 502, err.Error()); return }
	writeJSON(w, 200, info)
}

func (s *Server) importBackup(w http.ResponseWriter, r *http.Request) {
	dir := s.backupDir(); if dir == "" { writeError(w, 503, "backup directory is not configured"); return }
	if err := os.MkdirAll(dir, 0750); err != nil { writeError(w, 500, err.Error()); return }
	name := "xport-import-" + time.Now().Format("20060102-150405.000") + ".json.gz"
	path := filepath.Join(dir, name); tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600); if err != nil { writeError(w, 500, err.Error()); return }
	n, copyErr := io.Copy(f, io.LimitReader(http.MaxBytesReader(w, r.Body, 64<<20+1), 64<<20+1)); closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n > 64<<20 {
		_ = os.Remove(tmp)
		if n > 64<<20 { writeError(w, http.StatusRequestEntityTooLarge, "backup exceeds 64 MiB limit"); return }
		if copyErr != nil { writeError(w, 400, copyErr.Error()); return }; writeError(w, 500, closeErr.Error()); return
	}
	if err := os.Rename(tmp, path); err != nil { _ = os.Remove(tmp); writeError(w, 500, err.Error()); return }
	if _, err := backup.Load(dir, name); err != nil { _ = os.Remove(path); writeError(w, 400, "invalid X-port backup: "+err.Error()); return }
	st, err := os.Stat(path); if err != nil { writeError(w, 500, err.Error()); return }
	writeJSON(w, 201, backup.Info{Name: name, Size: st.Size(), CreatedAt: st.ModTime().UnixMilli()})
}

func (s *Server) restartPanel(w http.ResponseWriter, r *http.Request) {
	if s.panelService == "" { writeError(w, 503, "panel service is not configured"); return }
	if err := ops.ScheduleRestart(s.panelService, time.Second); err != nil { writeError(w, 502, err.Error()); return }
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "scheduled": true})
}

func panelSelfUpdater() (*selfupdate.Updater, error) {
	exe, err := os.Executable(); if err != nil { return nil, err }
	return &selfupdate.Updater{CurrentVersion: buildinfo.Current, BinaryPath: exe, Repo: "Kerberos255/X-port", Token: os.Getenv("XPORT_GITHUB_TOKEN")}, nil
}
func (s *Server) checkPanelUpdate(w http.ResponseWriter, r *http.Request) {
	u, err := panelSelfUpdater(); if err != nil { writeError(w, 500, err.Error()); return }
	ctx, cancel := contextWithTimeout(r, 12*time.Second); defer cancel()
	info, _, err := u.Check(ctx); if err != nil { writeError(w, 502, err.Error()); return }
	writeJSON(w, 200, info)
}
func (s *Server) updatePanel(w http.ResponseWriter, r *http.Request) {
	if s.panelService == "" { writeError(w, 503, "panel service is not configured"); return }
	u, err := panelSelfUpdater(); if err != nil { writeError(w, 500, err.Error()); return }
	ctx, cancel := contextWithTimeout(r, 2*time.Minute); defer cancel()
	info, previous, err := u.Stage(ctx); if err != nil { writeError(w, 502, err.Error()); return }
	if previous == "" { writeJSON(w, 200, info); return }
	binary := strings.TrimSuffix(previous, ".previous")
	if err := ops.ScheduleVerifiedBinaryRestart(s.panelService, binary, previous, time.Second); err != nil {
		_ = u.Restore(previous)
		writeError(w, 502, "could not schedule verified X-port restart: "+err.Error()); return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"update":info,"scheduled":true})
}
