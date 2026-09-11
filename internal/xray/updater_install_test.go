package xray

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdaterInstallsVerifiedBinary(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "xray", Method: zip.Deflate}
	h.SetMode(0755)
	w, _ := zw.CreateHeader(h)
	_, _ = w.Write([]byte("#!/bin/sh\nif [ \"$1\" = version ]; then echo 'Xray 26.9.9'; exit 0; fi\nif [ \"$1\" = run ]; then exit 0; fi\nexit 0\n"))
	_ = zw.Close()
	sum := sha256.Sum256(buf.Bytes())
	assetName, _ := linuxAssetName()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x.zip" {
			w.Write(buf.Bytes())
			return
		}
		fmt.Fprintf(w, `[{"tag_name":"v26.9.9","draft":false,"prerelease":true,"published_at":"2026-09-08T22:28:10Z","assets":[{"name":%q,"size":%d,"digest":"sha256:%x","browser_download_url":%q}]}]`, assetName, buf.Len(), sum, srv.URL+"/x.zip")
	}))
	defer srv.Close()
	dir := t.TempDir()
	bin := filepath.Join(dir, "xray")
	cfg := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfg, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	u := Updater{BinaryPath: bin, ConfigPath: cfg, Client: srv.Client(), ReleasesURL: srv.URL}
	info, err := u.Update(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Current != "26.9.9" || info.Available {
		t.Fatalf("%+v", info)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatal(err)
	}
}
