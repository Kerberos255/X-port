package xray

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckReleaseUsesStableChannel(t *testing.T) {
	asset, _ := linuxAssetName()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[
{"tag_name":"v27.0.0-rc1","draft":false,"prerelease":true,"published_at":"2026-09-10T22:28:10Z","assets":[{"name":%q,"size":10,"digest":"sha256:00","browser_download_url":"https://example.invalid/rc.zip"}]},
{"tag_name":"v26.9.9","draft":false,"prerelease":false,"published_at":"2026-09-08T22:28:10Z","assets":[{"name":%q,"size":10,"digest":"sha256:00","browser_download_url":"https://example.invalid/stable.zip"}]}
]`, asset, asset)
	}))
	defer srv.Close()
	u := Updater{BinaryPath: "/not-there", ReleasesURL: srv.URL, Client: srv.Client()}
	info, selected, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Latest != "26.9.9" || !info.Available || info.Prerelease {
		t.Fatalf("unexpected info: %+v", info)
	}
	if selected.URL != "https://example.invalid/stable.zip" {
		t.Fatalf("selected prerelease asset: %+v", selected)
	}
}

func TestCheckReleaseFallsBackToLatestStable(t *testing.T) {
	asset, _ := linuxAssetName()
	var listHits, latestHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			listHits++
			parts := make([]string, 0, 10)
			for i := 0; i < 10; i++ {
				parts = append(parts, fmt.Sprintf(`{"tag_name":"v27.0.%d-rc1","draft":false,"prerelease":true,"published_at":"2026-09-10T22:28:10Z","assets":[{"name":%q,"size":10,"digest":"sha256:00","browser_download_url":"https://example.invalid/rc.zip"}]}`, i, asset))
			}
			fmt.Fprintf(w, "[%s]", strings.Join(parts, ","))
		case "/latest":
			latestHits++
			fmt.Fprintf(w, `{"tag_name":"v26.3.27","draft":false,"prerelease":false,"published_at":"2026-03-27T17:51:11Z","assets":[{"name":%q,"size":10,"digest":"sha256:00","browser_download_url":"https://example.invalid/stable.zip"}]}`, asset)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	u := Updater{
		BinaryPath:       "/not-there",
		ReleasesURL:      srv.URL + "/releases",
		LatestReleaseURL: srv.URL + "/latest",
		Client:           srv.Client(),
	}
	info, selected, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Latest != "26.3.27" || !info.Available || info.Prerelease {
		t.Fatalf("unexpected info: %+v", info)
	}
	if selected.URL != "https://example.invalid/stable.zip" {
		t.Fatalf("unexpected selected asset: %+v", selected)
	}
	if listHits != 1 || latestHits != 1 {
		t.Fatalf("unexpected request counts: list=%d latest=%d", listHits, latestHits)
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
