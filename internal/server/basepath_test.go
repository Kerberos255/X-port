package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMountBasePath(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Path", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	h := MountBasePath(inner, "/secret/")

	req := httptest.NewRequest(http.MethodGet, "http://example.test/secret/api/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || w.Header().Get("X-Seen-Path") != "/api/health" {
		t.Fatalf("prefixed request: code=%d path=%q", w.Code, w.Header().Get("X-Seen-Path"))
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.test/secret", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != "/secret/" {
		t.Fatalf("base redirect: code=%d location=%q", w.Code, w.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.test/styles.css", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || w.Header().Get("X-Seen-Path") != "/styles.css" {
		t.Fatalf("root compatibility: code=%d path=%q", w.Code, w.Header().Get("X-Seen-Path"))
	}
}
