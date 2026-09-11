package selfupdate

import (
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

const defaultRepo = "Kerberos255/X-port"

type Asset struct {
	Name   string `json:"name"`
	APIURL string `json:"url"`
	WebURL string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Info struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Asset     string `json:"asset"`
	AuthMode  string `json:"authMode"`
}

type Updater struct {
	CurrentVersion string
	BinaryPath     string
	Repo           string
	Token          string
	Client         *http.Client
	ReleasesURL    string
}

func (u *Updater) Check(ctx context.Context) (Info, Asset, error) {
	rel, asset, err := u.latest(ctx)
	if err != nil { return Info{}, Asset{}, err }
	current := normalize(u.CurrentVersion)
	latest := normalize(rel.Tag)
	mode := "public"
	if strings.TrimSpace(u.Token) != "" { mode = "token" }
	return Info{Current: current, Latest: latest, Available: current != latest, Asset: asset.Name, AuthMode: mode}, asset, nil
}

// Stage verifies and atomically replaces the current executable, keeping the
// previous binary at the returned backup path. The caller must schedule an
// out-of-process verified service restart, then remove or restore that backup.
func (u *Updater) Stage(ctx context.Context) (Info, string, error) {
	info, asset, err := u.Check(ctx)
	if err != nil { return Info{}, "", err }
	if !info.Available { return info, "", nil }
	if asset.Size <= 0 || asset.Size > 64<<20 { return Info{}, "", fmt.Errorf("unexpected X-port asset size %d", asset.Size) }
	if strings.TrimSpace(u.BinaryPath) == "" { return Info{}, "", errors.New("X-port binary path is not configured") }

	tmpDir, err := os.MkdirTemp("", "xport-self-update-*")
	if err != nil { return Info{}, "", err }
	defer os.RemoveAll(tmpDir)
	candidate := filepath.Join(tmpDir, "xport")
	if err := u.download(ctx, asset, candidate); err != nil { return Info{}, "", err }
	if err := os.Chmod(candidate, 0755); err != nil { return Info{}, "", err }
	candidateVersion, err := executableVersion(candidate)
	if err != nil { return Info{}, "", fmt.Errorf("candidate X-port binary check failed: %w", err) }
	if normalize(candidateVersion) != info.Latest { return Info{}, "", fmt.Errorf("candidate version %q does not match release %q", candidateVersion, info.Latest) }

	binary := u.BinaryPath
	if resolved, err := filepath.EvalSymlinks(binary); err == nil { binary = resolved }
	if err := os.MkdirAll(filepath.Dir(binary), 0755); err != nil { return Info{}, "", err }
	backup := binary + ".previous"
	if _, err := os.Stat(binary); err != nil { return Info{}, "", fmt.Errorf("current X-port binary: %w", err) }
	_ = os.Remove(backup)
	if err := os.Rename(binary, backup); err != nil { return Info{}, "", err }
	if err := copyBinary(candidate, binary); err != nil {
		_ = os.Rename(backup, binary)
		return Info{}, "", err
	}
	info.Current = candidateVersion
	info.Available = false
	return info, backup, nil
}

func (u *Updater) Restore(backup string) error {
	if backup == "" { return errors.New("backup path is empty") }
	binary := u.BinaryPath
	if resolved, err := filepath.EvalSymlinks(binary); err == nil { binary = resolved }
	if _, err := os.Stat(backup); err != nil { return err }
	_ = os.Remove(binary)
	return os.Rename(backup, binary)
}

func (u *Updater) latest(ctx context.Context) (release, Asset, error) {
	endpoint := u.ReleasesURL
	if endpoint == "" {
		repo := strings.TrimSpace(u.Repo); if repo == "" { repo = defaultRepo }
		endpoint = "https://api.github.com/repos/" + repo + "/releases?per_page=10"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	u.headers(req, false)
	resp, err := u.client().Do(req)
	if err != nil { return release{}, Asset{}, err }
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && strings.TrimSpace(u.Token) == "" {
		return release{}, Asset{}, errors.New("X-port release unavailable; private repository requires XPORT_GITHUB_TOKEN")
	}
	if resp.StatusCode != http.StatusOK { return release{}, Asset{}, fmt.Errorf("X-port releases: HTTP %d", resp.StatusCode) }
	var releases []release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil { return release{}, Asset{}, err }
	name, err := assetName(); if err != nil { return release{}, Asset{}, err }
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease { continue }
		for _, a := range rel.Assets { if a.Name == name { return rel, a, nil } }
	}
	return release{}, Asset{}, fmt.Errorf("no stable %s release asset found", name)
}

func (u *Updater) download(ctx context.Context, asset Asset, dst string) error {
	url := asset.APIURL
	apiMode := url != ""
	if url == "" { url = asset.WebURL }
	if url == "" { return errors.New("release asset has no download URL") }
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	u.headers(req, apiMode)
	resp, err := u.client().Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return fmt.Errorf("download X-port: HTTP %d", resp.StatusCode) }
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil { return err }
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 64<<20+1))
	closeErr := f.Close()
	if copyErr != nil { return copyErr }; if closeErr != nil { return closeErr }
	if n > 64<<20 { return errors.New("X-port release exceeds download limit") }
	if !strings.HasPrefix(asset.Digest, "sha256:") { return errors.New("X-port release asset has no SHA-256 digest") }
	want := strings.TrimPrefix(asset.Digest, "sha256:")
	if !strings.EqualFold(want, hex.EncodeToString(h.Sum(nil))) { return errors.New("X-port release SHA-256 mismatch") }
	return nil
}

func (u *Updater) headers(req *http.Request, binary bool) {
	req.Header.Set("User-Agent", "X-port/0.1")
	if binary { req.Header.Set("Accept", "application/octet-stream") } else { req.Header.Set("Accept", "application/vnd.github+json") }
	if token := strings.TrimSpace(u.Token); token != "" { req.Header.Set("Authorization", "Bearer "+token) }
}
func (u *Updater) client() *http.Client { if u.Client != nil { return u.Client }; return &http.Client{Timeout:30*time.Second} }
func assetName()(string,error){switch runtime.GOARCH{case "amd64":return "xport-linux-amd64",nil;case "arm64":return "xport-linux-arm64",nil;default:return "",fmt.Errorf("unsupported architecture %s",runtime.GOARCH)}}
func executableVersion(path string)(string,error){ctx,c:=context.WithTimeout(context.Background(),5*time.Second);defer c();out,err:=exec.CommandContext(ctx,path,"version").CombinedOutput();if ctx.Err()!=nil{return "",ctx.Err()};if err!=nil{return "",fmt.Errorf("%w: %s",err,strings.TrimSpace(string(out)))};v:=strings.TrimSpace(strings.SplitN(string(out),"\n",2)[0]);if v==""{return "",errors.New("empty version output")};return v,nil}
func normalize(v string)string{return strings.TrimPrefix(strings.TrimSpace(v),"v")}
func copyBinary(src,dst string)error{in,err:=os.Open(src);if err!=nil{return err};defer in.Close();tmp:=dst+".new";out,err:=os.OpenFile(tmp,os.O_CREATE|os.O_WRONLY|os.O_TRUNC,0755);if err!=nil{return err};_,err=io.Copy(out,in);closeErr:=out.Close();if err!=nil{_ = os.Remove(tmp);return err};if closeErr!=nil{_ = os.Remove(tmp);return closeErr};return os.Rename(tmp,dst)}
