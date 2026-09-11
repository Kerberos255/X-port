package xray

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckRelease(t *testing.T) {
	asset, _ := linuxAssetName()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v26.9.9","draft":false,"prerelease":true,"published_at":"2026-09-08T22:28:10Z","assets":[{"name":%q,"size":10,"digest":"sha256:00","browser_download_url":"https://example.invalid/x.zip"}]}]`, asset)
	}))
	defer srv.Close()
	u := Updater{BinaryPath: "/not-there", ReleasesURL: srv.URL, Client: srv.Client()}
	info, _, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Latest != "26.9.9" || !info.Available || !info.Prerelease {
		t.Fatalf("%+v", info)
	}
}
func TestVerifyDigest(t *testing.T) {
	s := sha256.Sum256([]byte("x"))
	if err := verifyDigest("sha256:"+fmt.Sprintf("%x", s[:]), s[:]); err != nil {
		t.Fatal(err)
	}
	if verifyDigest("sha256:00", s[:]) == nil {
		t.Fatal("expected mismatch")
	}
}
