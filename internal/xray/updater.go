package xray

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const releasesURL = "https://api.github.com/repos/XTLS/Xray-core/releases?per_page=10"

type ReleaseAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
}
type Release struct {
	Tag         string         `json:"tag_name"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
}
type UpdateInfo struct {
	Current     string    `json:"current"`
	Latest      string    `json:"latest"`
	Available   bool      `json:"available"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"publishedAt"`
	Asset       string    `json:"asset"`
}

type Updater struct {
	BinaryPath  string
	ConfigPath  string
	Service     string
	Client      *http.Client
	ReleasesURL string
}

func (u *Updater) Check(ctx context.Context) (UpdateInfo, ReleaseAsset, error) {
	rel, asset, err := u.latest(ctx)
	if err != nil {
		return UpdateInfo{}, ReleaseAsset{}, err
	}
	current := binaryVersion(u.BinaryPath)
	latest := strings.TrimPrefix(rel.Tag, "v")
	return UpdateInfo{Current: current, Latest: latest, Available: current == "" || normalizeVersion(current) != normalizeVersion(latest), Prerelease: rel.Prerelease, PublishedAt: rel.PublishedAt, Asset: asset.Name}, asset, nil
}

func (u *Updater) Update(ctx context.Context) (UpdateInfo, error) {
	info, asset, err := u.Check(ctx)
	if err != nil {
		return UpdateInfo{}, err
	}
	if !info.Available {
		return info, nil
	}
	if asset.Size <= 0 || asset.Size > 128<<20 {
		return UpdateInfo{}, fmt.Errorf("unexpected release asset size %d", asset.Size)
	}
	client := u.client()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	req.Header.Set("User-Agent", "X-port/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return UpdateInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UpdateInfo{}, fmt.Errorf("download Xray: HTTP %d", resp.StatusCode)
	}
	tmpDir, err := os.MkdirTemp("", "xport-xray-update-*")
	if err != nil {
		return UpdateInfo{}, err
	}
	defer os.RemoveAll(tmpDir)
	zipPath := filepath.Join(tmpDir, "xray.zip")
	f, err := os.OpenFile(zipPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return UpdateInfo{}, err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 128<<20+1))
	closeErr := f.Close()
	if copyErr != nil {
		return UpdateInfo{}, copyErr
	}
	if closeErr != nil {
		return UpdateInfo{}, closeErr
	}
	if n > 128<<20 {
		return UpdateInfo{}, errors.New("Xray release exceeds download limit")
	}
	if err := verifyDigest(asset.Digest, h.Sum(nil)); err != nil {
		return UpdateInfo{}, err
	}
	candidateDir := filepath.Join(tmpDir, "candidate")
	if err := os.Mkdir(candidateDir, 0700); err != nil {
		return UpdateInfo{}, err
	}
	if err := extractSelected(zipPath, candidateDir); err != nil {
		return UpdateInfo{}, err
	}
	candidate := filepath.Join(candidateDir, "xray")
	if err := os.Chmod(candidate, 0755); err != nil {
		return UpdateInfo{}, err
	}
	if out, err := run(8*time.Second, candidate, "version"); err != nil {
		return UpdateInfo{}, fmt.Errorf("new Xray binary check failed: %v: %s", err, out)
	}
	if u.ConfigPath != "" {
		if _, err := os.Stat(u.ConfigPath); err == nil {
			if out, err := run(10*time.Second, candidate, "run", "-test", "-config", u.ConfigPath); err != nil {
				return UpdateInfo{}, fmt.Errorf("new Xray rejected current config: %v: %s", err, out)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(u.BinaryPath), 0755); err != nil {
		return UpdateInfo{}, err
	}
	backup := u.BinaryPath + ".previous"
	hadOld := false
	if _, err := os.Stat(u.BinaryPath); err == nil {
		hadOld = true
		_ = os.Remove(backup)
		if err := os.Rename(u.BinaryPath, backup); err != nil {
			return UpdateInfo{}, err
		}
	}
	// Update only the core binary here. Keeping geodata outside the core swap
	// makes rollback complete: one failed service restart can restore the exact
	// previous executable without leaving version-skewed auxiliary files.
	installErr := copyFile(candidate, u.BinaryPath, 0755)
	if installErr == nil && u.Service != "" {
		installErr = restartAndVerify(u.Service)
	}
	if installErr != nil {
		_ = os.Remove(u.BinaryPath)
		if hadOld {
			_ = os.Rename(backup, u.BinaryPath)
			if u.Service != "" {
				_ = restartAndVerify(u.Service)
			}
		}
		return UpdateInfo{}, fmt.Errorf("Xray update failed and was rolled back: %w", installErr)
	}
	info.Current = binaryVersion(u.BinaryPath)
	info.Available = false
	return info, nil
}

func (u *Updater) latest(ctx context.Context) (Release, ReleaseAsset, error) {
	endpoint := u.ReleasesURL
	if endpoint == "" {
		endpoint = releasesURL
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "X-port/0.1")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.client().Do(req)
	if err != nil {
		return Release{}, ReleaseAsset{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, ReleaseAsset{}, fmt.Errorf("GitHub releases: HTTP %d", resp.StatusCode)
	}
	var releases []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return Release{}, ReleaseAsset{}, err
	}
	assetName, err := linuxAssetName()
	if err != nil {
		return Release{}, ReleaseAsset{}, err
	}
	for _, rel := range releases {
		if rel.Draft {
			continue
		}
		for _, a := range rel.Assets {
			if a.Name == assetName {
				return rel, a, nil
			}
		}
	}
	return Release{}, ReleaseAsset{}, fmt.Errorf("no %s asset found", assetName)
}
func (u *Updater) client() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}
func linuxAssetName() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "Xray-linux-64.zip", nil
	case "arm64":
		return "Xray-linux-arm64-v8a.zip", nil
	default:
		return "", fmt.Errorf("unsupported architecture %s", runtime.GOARCH)
	}
}
func verifyDigest(digest string, sum []byte) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return errors.New("release asset has no SHA-256 digest")
	}
	want := strings.TrimPrefix(digest, "sha256:")
	if !strings.EqualFold(want, hex.EncodeToString(sum)) {
		return errors.New("Xray release SHA-256 mismatch")
	}
	return nil
}
func extractSelected(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	wanted := map[string]os.FileMode{"xray": 0755, "geoip.dat": 0644, "geosite.dat": 0644}
	found := false
	for _, z := range r.File {
		name := filepath.Base(z.Name)
		mode, ok := wanted[name]
		if !ok {
			continue
		}
		if z.UncompressedSize64 > 80<<20 {
			return fmt.Errorf("archive entry too large: %s", name)
		}
		rc, err := z.Open()
		if err != nil {
			return err
		}
		dst := filepath.Join(dest, name)
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err == nil {
			_, err = io.Copy(f, io.LimitReader(rc, 80<<20+1))
			_ = f.Close()
		}
		_ = rc.Close()
		if err != nil {
			return err
		}
		if name == "xray" {
			found = true
		}
	}
	if !found {
		return errors.New("xray executable missing from release archive")
	}
	return nil
}
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dst)
}
func binaryVersion(path string) string {
	if path == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	line = strings.TrimPrefix(line, "Xray ")
	return strings.Fields(line)[0]
}
func normalizeVersion(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }
