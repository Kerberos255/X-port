package selfupdate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStageSelectsStableReleaseAndKeepsRollbackBinary(t *testing.T) {
	candidate := []byte("#!/bin/sh\nif [ \"$1\" = version ]; then echo '0.2.0'; exit 0; fi\nexit 0\n")
	sum := sha256.Sum256(candidate)
	name, _ := assetName()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/asset" { _, _ = w.Write(candidate); return }
		fmt.Fprintf(w, `[
{"tag_name":"v0.3.0-rc1","draft":false,"prerelease":true,"assets":[{"name":%q,"url":%q,"size":%d,"digest":"sha256:%x"}]},
{"tag_name":"v0.2.0","draft":false,"prerelease":false,"assets":[{"name":%q,"url":%q,"size":%d,"digest":"sha256:%x"}]}
]`, name, srv.URL+"/asset", len(candidate), sum, name, srv.URL+"/asset", len(candidate), sum)
	}))
	defer srv.Close()

	dir := t.TempDir()
	binary := filepath.Join(dir, "xport")
	old := []byte("#!/bin/sh\nif [ \"$1\" = version ]; then echo '0.1.0'; exit 0; fi\nexit 0\n")
	if err := os.WriteFile(binary, old, 0755); err != nil { t.Fatal(err) }
	u := Updater{CurrentVersion:"0.1.0", BinaryPath:binary, ReleasesURL:srv.URL, Client:srv.Client()}
	info, previous, err := u.Stage(context.Background())
	if err != nil { t.Fatal(err) }
	if info.Current != "0.2.0" || info.Latest != "0.2.0" || info.Available { t.Fatalf("unexpected info: %+v", info) }
	if previous != binary+".previous" { t.Fatalf("backup = %q", previous) }
	oldOnDisk, err := os.ReadFile(previous); if err != nil { t.Fatal(err) }
	if string(oldOnDisk) != string(old) { t.Fatal("rollback binary was not preserved") }
	v, err := executableVersion(binary); if err != nil { t.Fatal(err) }
	if v != "0.2.0" { t.Fatalf("installed version = %q", v) }
	if err := u.Restore(previous); err != nil { t.Fatal(err) }
	v, err = executableVersion(binary); if err != nil { t.Fatal(err) }
	if v != "0.1.0" { t.Fatalf("restored version = %q", v) }
}
