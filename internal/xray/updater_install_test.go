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

func xrayReleaseArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries := []struct {
		name string
		mode os.FileMode
		body string
	}{
		{"xray", 0755, "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'Xray 26.9.9'; exit 0; fi\nif [ \"$1\" = run ]; then exit 0; fi\nexit 0\n"},
		{"geoip.dat", 0644, "geoip-test-data"},
		{"geosite.dat", 0644, "geosite-test-data"},
	}
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func releaseServer(t *testing.T, archive []byte) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(archive)
	assetName, err := linuxAssetName()
	if err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x.zip" {
			_, _ = w.Write(archive)
			return
		}
		fmt.Fprintf(w, `[{"tag_name":"v26.9.9","draft":false,"prerelease":false,"published_at":"2026-09-08T22:28:10Z","assets":[{"name":%q,"size":%d,"digest":"sha256:%x","browser_download_url":%q}]}]`, assetName, len(archive), sum, srv.URL+"/x.zip")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdaterInstallsVerifiedBinaryAndBootstrapsGeodata(t *testing.T) {
	archive := xrayReleaseArchive(t)
	srv := releaseServer(t, archive)
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
	for _, name := range []string{"xray", "geoip.dat", "geosite.dat"} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
		if st.Size() == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}

func TestUpdaterPreservesExistingGeodata(t *testing.T) {
	archive := xrayReleaseArchive(t)
	srv := releaseServer(t, archive)
	dir := t.TempDir()
	bin := filepath.Join(dir, "xray")
	cfg := filepath.Join(dir, "config.json")
	oldGeoIP := []byte("keep-old-geoip")
	oldGeoSite := []byte("keep-old-geosite")
	if err := os.WriteFile(cfg, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geoip.dat"), oldGeoIP, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geosite.dat"), oldGeoSite, 0644); err != nil {
		t.Fatal(err)
	}
	u := Updater{BinaryPath: bin, ConfigPath: cfg, Client: srv.Client(), ReleasesURL: srv.URL}
	if _, err := u.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	gotGeoIP, err := os.ReadFile(filepath.Join(dir, "geoip.dat"))
	if err != nil {
		t.Fatal(err)
	}
	gotGeoSite, err := os.ReadFile(filepath.Join(dir, "geosite.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotGeoIP, oldGeoIP) || !bytes.Equal(gotGeoSite, oldGeoSite) {
		t.Fatalf("existing GeoData was overwritten")
	}
}
