package accountcfg

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Kerberos255/X-port/internal/model"
)

type Input struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Credential string `json:"credential"`
	Flow       string `json:"flow"`
	Network    string `json:"network"`
	Security   string `json:"security"`
	ServerName string `json:"serverName"`
	Dest       string `json:"dest"`
	PrivateKey string `json:"privateKey"`
	ShortID    string `json:"shortId"`
	QuotaBytes int64  `json:"quotaBytes"`
	ExpiryTime int64  `json:"expiryTime"`
}

type View struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Credential string `json:"credential,omitempty"`
	Flow       string `json:"flow,omitempty"`
	Network    string `json:"network,omitempty"`
	Security   string `json:"security,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	Dest       string `json:"dest,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	ShortID    string `json:"shortId,omitempty"`
	QuotaBytes int64  `json:"quotaBytes"`
	ExpiryTime int64  `json:"expiryTime"`
	UpBytes    int64  `json:"upBytes"`
	DownBytes  int64  `json:"downBytes"`
	AllTime    int64  `json:"allTimeBytes"`
	Editable   bool   `json:"editable"`
	Tag        string `json:"tag"`
}

type vlessClient struct {
	ID      string `json:"id"`
	Email   string `json:"email,omitempty"`
	Flow    string `json:"flow,omitempty"`
	Level   int    `json:"level,omitempty"`
	TotalGB int64  `json:"totalGB,omitempty"`
	Expiry  int64  `json:"expiryTime,omitempty"`
	Enable  *bool  `json:"enable,omitempty"`
	Comment string `json:"comment,omitempty"`
}

type vlessSettings struct {
	Clients    []vlessClient `json:"clients"`
	Decryption string        `json:"decryption,omitempty"`
	Encryption string        `json:"encryption,omitempty"`
}

type streamSettings struct {
	Network         string          `json:"network,omitempty"`
	Security        string          `json:"security,omitempty"`
	RealitySettings json.RawMessage `json:"realitySettings,omitempty"`
}

type realitySettings struct {
	Show        bool     `json:"show,omitempty"`
	Dest        string   `json:"dest,omitempty"`
	Target      string   `json:"target,omitempty"`
	Xver        int      `json:"xver,omitempty"`
	ServerNames []string `json:"serverNames,omitempty"`
	PrivateKey  string   `json:"privateKey,omitempty"`
	ShortIDs    []string `json:"shortIds,omitempty"`
}

func New(in Input, fallbackPort int) (model.Account, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return model.Account{}, errors.New("name is required")
	}
	if in.Port == 0 {
		in.Port = fallbackPort
	}
	if err := validPort(in.Port); err != nil {
		return model.Account{}, err
	}
	if in.Protocol == "" {
		in.Protocol = "vless"
	}
	if strings.ToLower(in.Protocol) != "vless" {
		return model.Account{}, errors.New("new accounts currently support VLESS; imported protocols remain preserved")
	}
	if in.Credential == "" {
		in.Credential = newUUID()
	}
	if in.Flow == "" {
		in.Flow = "xtls-rprx-vision"
	}
	if in.Network == "" {
		in.Network = "tcp"
	}
	if in.Security == "" {
		in.Security = "reality"
	}
	if in.Security == "reality" {
		if strings.TrimSpace(in.ServerName) == "" {
			return model.Account{}, errors.New("serverName is required for REALITY")
		}
		if strings.TrimSpace(in.Dest) == "" {
			in.Dest = net.JoinHostPort(in.ServerName, "443")
		}
		if in.PrivateKey == "" {
			privateKey, _, err := newX25519Keypair()
			if err != nil {
				return model.Account{}, err
			}
			in.PrivateKey = privateKey
		}
		if in.ShortID == "" {
			in.ShortID = newShortID()
		}
	}
	settings, stream, err := buildVLESS(in)
	if err != nil {
		return model.Account{}, err
	}
	now := time.Now().UnixMilli()
	return model.Account{
		Name: in.Name, Enabled: in.Enabled, Port: in.Port, Protocol: "vless",
		SettingsJSON: settings, StreamSettingsJSON: stream,
		SniffingJSON: `{"enabled":true,"destOverride":["http","tls","quic"]}`,
		Tag:          fmt.Sprintf("xport-%d-%s", in.Port, shortSlug(in.Name)),
		QuotaBytes:   in.QuotaBytes, ExpiryTime: in.ExpiryTime, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func Update(a model.Account, in Input) (model.Account, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return model.Account{}, errors.New("name is required")
	}
	if in.Port == 0 {
		in.Port = a.Port
	}
	if err := validPort(in.Port); err != nil {
		return model.Account{}, err
	}
	oldPort := a.Port
	a.Name, a.Enabled, a.Port, a.QuotaBytes, a.ExpiryTime = in.Name, in.Enabled, in.Port, in.QuotaBytes, in.ExpiryTime
	a.UpdatedAt = time.Now().UnixMilli()
	if strings.EqualFold(a.Protocol, "vless") {
		old, _ := ToView(a)
		if in.Credential == "" {
			in.Credential = old.Credential
		}
		if in.Flow == "" {
			in.Flow = old.Flow
		}
		if in.Network == "" {
			in.Network = old.Network
		}
		if in.Security == "" {
			in.Security = old.Security
		}
		if in.ServerName == "" {
			in.ServerName = old.ServerName
		}
		if in.Dest == "" {
			in.Dest = old.Dest
		}
		if in.PrivateKey == "" {
			in.PrivateKey = old.PrivateKey
		}
		if in.ShortID == "" {
			in.ShortID = old.ShortID
		}
		settings, stream, err := updateVLESSJSON(a.SettingsJSON, a.StreamSettingsJSON, in)
		if err != nil {
			return model.Account{}, err
		}
		a.SettingsJSON, a.StreamSettingsJSON = settings, stream
	}
	if a.Tag == "" || in.Port != oldPort {
		a.Tag = fmt.Sprintf("xport-%d-%s", in.Port, shortSlug(in.Name))
	}
	return a, nil
}

func Clone(a model.Account, name string, port int) (model.Account, error) {
	if err := validPort(port); err != nil {
		return model.Account{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = a.Name + " copy"
	}
	clone := a
	clone.ID = 0
	clone.Name = name
	clone.Port = port
	clone.UpBytes, clone.DownBytes, clone.AllTimeBytes = 0, 0, 0
	clone.Tag = fmt.Sprintf("xport-%d-%s", port, shortSlug(name))
	clone.CreatedAt, clone.UpdatedAt = time.Now().UnixMilli(), time.Now().UnixMilli()
	if strings.EqualFold(clone.Protocol, "vless") {
		var s vlessSettings
		if json.Unmarshal([]byte(clone.SettingsJSON), &s) == nil && len(s.Clients) == 1 {
			s.Clients[0].ID = newUUID()
			s.Clients[0].Email = accountEmail(name, port)
			b, _ := json.Marshal(s)
			clone.SettingsJSON = string(b)
		}
	}
	return clone, nil
}

func ToView(a model.Account) (View, error) {
	v := View{ID: a.ID, Name: a.Name, Enabled: a.Enabled, Port: a.Port, Protocol: a.Protocol, QuotaBytes: a.QuotaBytes, ExpiryTime: a.ExpiryTime, UpBytes: a.UpBytes, DownBytes: a.DownBytes, AllTime: a.AllTimeBytes, Tag: a.Tag}
	if !strings.EqualFold(a.Protocol, "vless") {
		return v, nil
	}
	var s vlessSettings
	if err := json.Unmarshal([]byte(a.SettingsJSON), &s); err != nil || len(s.Clients) != 1 {
		return v, nil
	}
	v.Editable = true
	v.Credential, v.Flow = s.Clients[0].ID, s.Clients[0].Flow
	var st streamSettings
	if json.Unmarshal([]byte(a.StreamSettingsJSON), &st) == nil {
		v.Network, v.Security = st.Network, st.Security
		if st.Security == "reality" && len(st.RealitySettings) > 0 {
			var r realitySettings
			if json.Unmarshal(st.RealitySettings, &r) == nil {
				v.PrivateKey = r.PrivateKey
				v.Dest = r.Target
				if v.Dest == "" {
					v.Dest = r.Dest
				}
				if len(r.ServerNames) > 0 {
					v.ServerName = r.ServerNames[0]
				}
				if len(r.ShortIDs) > 0 {
					v.ShortID = r.ShortIDs[0]
				}
			}
		}
	}
	return v, nil
}

func ShareURI(a model.Account, host string) (string, error) {
	v, err := ToView(a)
	if err != nil {
		return "", err
	}
	if !v.Editable || !strings.EqualFold(v.Protocol, "vless") || v.Credential == "" {
		return "", errors.New("share links currently support one-client VLESS accounts")
	}
	host = normalizeHost(host)
	if host == "" {
		return "", errors.New("host is required")
	}
	u := &url.URL{Scheme: "vless", Host: net.JoinHostPort(host, strconv.Itoa(v.Port)), User: url.User(v.Credential), Fragment: v.Name}
	q := url.Values{}
	if v.Network != "" {
		q.Set("type", v.Network)
	}
	if v.Flow != "" {
		q.Set("flow", v.Flow)
	}
	security := v.Security
	if security == "" {
		security = "none"
	}
	q.Set("security", security)
	if security == "reality" {
		publicKey, err := publicKeyFromPrivate(v.PrivateKey)
		if err != nil {
			return "", fmt.Errorf("derive REALITY public key: %w", err)
		}
		q.Set("pbk", publicKey)
		q.Set("fp", "chrome")
		if v.ServerName != "" {
			q.Set("sni", v.ServerName)
		}
		if v.ShortID != "" {
			q.Set("sid", v.ShortID)
		}
		q.Set("spx", "/")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func updateVLESSJSON(settingsRaw, streamRaw string, in Input) (string, string, error) {
	var settings map[string]any
	if err := json.Unmarshal([]byte(settingsRaw), &settings); err != nil {
		return "", "", err
	}
	clients, ok := settings["clients"].([]any)
	if !ok || len(clients) != 1 {
		return "", "", errors.New("VLESS account must contain exactly one client")
	}
	client, ok := clients[0].(map[string]any)
	if !ok {
		return "", "", errors.New("invalid VLESS client")
	}
	client["id"], client["email"] = in.Credential, accountEmail(in.Name, in.Port)
	if in.Flow == "" {
		delete(client, "flow")
	} else {
		client["flow"] = in.Flow
	}
	settings["clients"] = []any{client}
	if _, ok := settings["decryption"]; !ok {
		settings["decryption"] = "none"
	}

	var stream map[string]any
	if strings.TrimSpace(streamRaw) == "" {
		stream = map[string]any{}
	} else if err := json.Unmarshal([]byte(streamRaw), &stream); err != nil {
		return "", "", err
	}
	stream["network"], stream["security"] = in.Network, in.Security
	if in.Security == "reality" {
		r, _ := stream["realitySettings"].(map[string]any)
		if r == nil {
			r = map[string]any{}
		}
		r["target"] = in.Dest
		delete(r, "dest")
		r["serverNames"] = []string{in.ServerName}
		r["privateKey"] = in.PrivateKey
		r["shortIds"] = []string{in.ShortID}
		stream["realitySettings"] = r
	} else {
		delete(stream, "realitySettings")
	}
	sb, err := json.Marshal(settings)
	if err != nil {
		return "", "", err
	}
	stb, err := json.Marshal(stream)
	if err != nil {
		return "", "", err
	}
	return string(sb), string(stb), nil
}

func buildVLESS(in Input) (string, string, error) {
	if strings.TrimSpace(in.Credential) == "" {
		return "", "", errors.New("credential is required")
	}
	settings := vlessSettings{Clients: []vlessClient{{ID: in.Credential, Email: accountEmail(in.Name, in.Port), Flow: in.Flow}}, Decryption: "none"}
	sb, err := json.Marshal(settings)
	if err != nil {
		return "", "", err
	}
	st := streamSettings{Network: in.Network, Security: in.Security}
	if in.Security == "reality" {
		r := realitySettings{Target: in.Dest, ServerNames: []string{in.ServerName}, PrivateKey: in.PrivateKey, ShortIDs: []string{in.ShortID}}
		rb, _ := json.Marshal(r)
		st.RealitySettings = rb
	}
	stb, err := json.Marshal(st)
	if err != nil {
		return "", "", err
	}
	return string(sb), string(stb), nil
}

func NextPort(accounts []model.Account) int {
	used := map[int]bool{}
	for _, a := range accounts {
		used[a.Port] = true
	}
	for p := 20000; p <= 60000; p++ {
		if !used[p] {
			return p
		}
	}
	return 0
}

func validPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	return nil
}
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func newShortID() string { b := make([]byte, 8); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func newX25519Keypair() (string, string, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(key.Bytes()), base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}
func publicKeyFromPrivate(private string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(private)
	if err != nil {
		return "", err
	}
	key, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}
func normalizeHost(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if h, _, err := net.SplitHostPort(s); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(s, "[]")
}
func shortSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
		if b.Len() >= 12 {
			break
		}
	}
	if b.Len() == 0 {
		return "account"
	}
	return b.String()
}
func accountEmail(name string, port int) string { return fmt.Sprintf("%s-%d", shortSlug(name), port) }
