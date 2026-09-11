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

func TestGeodataUpdateInstallsVerifiedPair(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"xray":       "#!/bin/sh\necho 'Xray 26.9.9'\n",
		"geoip.dat":   "geoip-test-data",
		"geosite.dat": "geosite-test-data",
	}
	for name, data := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if name == "xray" { h.SetMode(0755) } else { h.SetMode(0644) }
		w, err := zw.CreateHeader(h)
		if err != nil { t.Fatal(err) }
		if _, err := w.Write([]byte(data)); err != nil { t.Fatal(err) }
	}
	if err := zw.Close(); err != nil { t.Fatal(err) }
	sum := sha256.Sum256(buf.Bytes())
	assetName, _ := linuxAssetName()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x.zip" { _, _ = w.Write(buf.Bytes()); return }
		fmt.Fprintf(w, `[{"tag_name":"v26.9.9","draft":false,"prerelease":false,"published_at":"2026-09-08T22:28:10Z","assets":[{"name":%q,"size":%d,"digest":"sha256:%x","browser_download_url":%q}]}]`, assetName, buf.Len(), sum, srv.URL+"/x.zip")
	}))
	defer srv.Close()

	dir := t.TempDir()
	u := Updater{BinaryPath: filepath.Join(dir, "xray"), Client: srv.Client(), ReleasesURL: srv.URL}
	info, err := u.UpdateGeodata(context.Background())
	if err != nil { t.Fatal(err) }
	if info.Current != "v26.9.9" || info.Available { t.Fatalf("unexpected info: %+v", info) }
	for name, want := range map[string]string{"geoip.dat":"geoip-test-data", "geosite.dat":"geosite-test-data"} {
		b, err := os.ReadFile(filepath.Join(dir, name)); if err != nil { t.Fatal(err) }
		if string(b) != want { t.Fatalf("%s = %q, want %q", name, b, want) }
	}
	marker, err := os.ReadFile(filepath.Join(dir, ".xport-geodata-version")); if err != nil { t.Fatal(err) }
	if string(marker) != "v26.9.9\n" { t.Fatalf("marker = %q", marker) }
	checked, _, err := u.CheckGeodata(context.Background()); if err != nil { t.Fatal(err) }
	if checked.Available { t.Fatalf("expected current GeoData, got %+v", checked) }
}
