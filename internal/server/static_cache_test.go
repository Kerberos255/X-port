package server

import (
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "testing/fstest"

 "github.com/Kerberos255/X-port/internal/store"
)

func TestStaticEntryDisablesCacheWithoutDisablingAssetCaching(t *testing.T) {
 st, err := store.Open(filepath.Join(t.TempDir(), "cache-test.db"))
 if err != nil { t.Fatal(err) }
 defer st.Close()

 assets := fstest.MapFS{
  "index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>X-port</title>")},
  "status-gauges.js": &fstest.MapFile{Data: []byte("/* cached static asset */")},
 }
 handler := New(st, nil, nil, assets).Handler()

 for _, tc := range []struct{ path string; wantNoStore bool }{
  {"/", true}, {"/index.html", true}, {"/unknown-route", true},
  {"/status-gauges.js?v=0.2.5", false},
 } {
  t.Run(strings.ReplaceAll(tc.path, "/", "_"), func(t *testing.T) {
   rec := httptest.NewRecorder()
   req := httptest.NewRequest(http.MethodGet, "http://panel"+tc.path, nil)
   handler.ServeHTTP(rec,req)
   if rec.Code >= 400 { t.Fatalf("HTTP %d: %s",rec.Code,rec.Body.String()) }
   got := strings.Contains(rec.Header().Get("Cache-Control"),"no-store")
   if got != tc.wantNoStore { t.Fatalf("Cache-Control=%q, want no-store=%t",rec.Header().Get("Cache-Control"),tc.wantNoStore) }
  })
 }
}
