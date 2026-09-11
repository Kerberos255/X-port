package xray

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type GeodataInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Asset     string `json:"asset"`
}

func (u *Updater) CheckGeodata(ctx context.Context) (GeodataInfo, ReleaseAsset, error) {
	rel, asset, err := u.latest(ctx)
	if err != nil {
		return GeodataInfo{}, ReleaseAsset{}, err
	}
	latest := strings.TrimSpace(rel.Tag)
	current := ""
	if b, err := os.ReadFile(u.geodataMarker()); err == nil {
		current = strings.TrimSpace(string(b))
	}
	return GeodataInfo{Current: current, Latest: latest, Available: current != latest, Asset: asset.Name}, asset, nil
}

func (u *Updater) UpdateGeodata(ctx context.Context) (GeodataInfo, error) {
	info, asset, err := u.CheckGeodata(ctx)
	if err != nil {
		return GeodataInfo{}, err
	}
	if !info.Available {
		return info, nil
	}
	if asset.Size <= 0 || asset.Size > 128<<20 {
		return GeodataInfo{}, fmt.Errorf("unexpected release asset size %d", asset.Size)
	}
	tmpDir, err := os.MkdirTemp("", "xport-geodata-update-*")
	if err != nil {
		return GeodataInfo{}, err
	}
	defer os.RemoveAll(tmpDir)
	zipPath := filepath.Join(tmpDir, "xray.zip")
	if err := u.downloadAsset(ctx, asset, zipPath); err != nil {
		return GeodataInfo{}, err
	}
	candidateDir := filepath.Join(tmpDir, "candidate")
	if err := os.Mkdir(candidateDir, 0700); err != nil {
		return GeodataInfo{}, err
	}
	if err := extractSelected(zipPath, candidateDir); err != nil {
		return GeodataInfo{}, err
	}
	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		if st, err := os.Stat(filepath.Join(candidateDir, name)); err != nil || st.Size() == 0 {
			return GeodataInfo{}, fmt.Errorf("%s missing from Xray release", name)
		}
	}

	assetDir := filepath.Dir(u.BinaryPath)
	if err := os.MkdirAll(assetDir, 0755); err != nil {
		return GeodataInfo{}, err
	}
	type oldFile struct {
		name   string
		hadOld bool
	}
	old := make([]oldFile, 0, 2)
	rollback := func() {
		for _, f := range old {
			dst := filepath.Join(assetDir, f.name)
			bak := dst + ".previous"
			_ = os.Remove(dst)
			if f.hadOld {
				_ = os.Rename(bak, dst)
			}
		}
		if u.Service != "" {
			_ = restartAndVerify(u.Service)
		}
	}
	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		dst := filepath.Join(assetDir, name)
		bak := dst + ".previous"
		hadOld := false
		if _, err := os.Stat(dst); err == nil {
			hadOld = true
			_ = os.Remove(bak)
			if err := os.Rename(dst, bak); err != nil {
				rollback()
				return GeodataInfo{}, err
			}
		}
		old = append(old, oldFile{name: name, hadOld: hadOld})
		if err := copyFile(filepath.Join(candidateDir, name), dst, 0644); err != nil {
			rollback()
			return GeodataInfo{}, err
		}
	}
	if u.Service != "" {
		if err := restartAndVerify(u.Service); err != nil {
			rollback()
			return GeodataInfo{}, fmt.Errorf("GeoData update failed and was rolled back: %w", err)
		}
	}
	markerTmp := u.geodataMarker() + ".new"
	if err := os.WriteFile(markerTmp, []byte(info.Latest+"\n"), 0644); err != nil {
		rollback()
		return GeodataInfo{}, err
	}
	if err := os.Rename(markerTmp, u.geodataMarker()); err != nil {
		rollback()
		return GeodataInfo{}, err
	}
	for _, f := range old {
		_ = os.Remove(filepath.Join(assetDir, f.name) + ".previous")
	}
	info.Current = info.Latest
	info.Available = false
	return info, nil
}

func (u *Updater) geodataMarker() string {
	return filepath.Join(filepath.Dir(u.BinaryPath), ".xport-geodata-version")
}

func (u *Updater) downloadAsset(ctx context.Context, asset ReleaseAsset, dst string) error {
	client := u.client()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	req.Header.Set("User-Agent", "X-port/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download Xray asset: HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 128<<20+1))
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > 128<<20 {
		return errors.New("Xray release exceeds download limit")
	}
	if err := verifyDigest(asset.Digest, h.Sum(nil)); err != nil {
		return err
	}
	return nil
}

var _ = time.Second
