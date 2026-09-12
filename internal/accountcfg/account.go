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
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	Port           int    `json:"port"`
	Protocol       string `json:"protocol"`
	Credential     string `json:"credential"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	ServerPassword string `json:"serverPassword"`
	ClientSecurity string `json:"clientSecurity"`
	Method         string `json:"method"`
	Flow           string `json:"flow"`
	Network        string `json:"network"`
	Security       string `json:"security"`
	ServerName     string `json:"serverName"`
	Dest           string `json:"dest"`
	PrivateKey     string `json:"privateKey"`
	ShortID        string `json:"shortId"`
	Host           string `json:"host"`
	Path           string `json:"path"`
	ServiceName    string `json:"serviceName"`
	QuotaBytes     int64  `json:"quotaBytes"`
	ExpiryTime     int64  `json:"expiryTime"`
	MonthlyReset   bool   `json:"monthlyReset"`
}

type View struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabledReason,omitempty"`
	Port           int    `json:"port"`
	Protocol       string `json:"protocol"`
	Credential     string `json:"credential,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	ServerPassword string `json:"serverPassword,omitempty"`
	ClientSecurity string `json:"clientSecurity,omitempty"`
	Method         string `json:"method,omitempty"`
	Flow           string `json:"flow,omitempty"`
	Network        string `json:"network,omitempty"`
	Security       string `json:"security,omitempty"`
	ServerName     string `json:"serverName,omitempty"`
	Dest           string `json:"dest,omitempty"`
	PrivateKey     string `json:"privateKey,omitempty"`
	ShortID        string `json:"shortId,omitempty"`
	Host           string `json:"host,omitempty"`
	Path           string `json:"path,omitempty"`
	ServiceName    string `json:"serviceName,omitempty"`
	QuotaBytes     int64  `json:"quotaBytes"`
	ExpiryTime     int64  `json:"expiryTime"`
	MonthlyReset   bool   `json:"monthlyReset"`
	UpBytes        int64  `json:"upBytes"`
	DownBytes      int64  `json:"downBytes"`
	AllTime        int64  `json:"allTimeBytes"`
	Editable       bool   `json:"editable"`
	Tag            string `json:"tag"`
}

func New(in Input, fallbackPort int) (model.Account, error) {
	normalizeEditableInput(&in)
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
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	if in.Protocol == "" {
		in.Protocol = "vless"
	}
	applyDefaults(&in)
	if err := validateEditableInput(in); err != nil {
		return model.Account{}, err
	}
	settings, stream, err := buildProtocol(in)
	if err != nil {
		return model.Account{}, err
	}
	now := time.Now().UnixMilli()
	a := model.Account{
		Name: in.Name, Enabled: in.Enabled, Port: in.Port, Protocol: in.Protocol,
		SettingsJSON: settings, StreamSettingsJSON: stream,
		SniffingJSON: `{"enabled":true,"destOverride":["http","tls","quic"]}`,
		Tag:          fmt.Sprintf("xport-%d-%s", in.Port, shortSlug(in.Name)),
		QuotaBytes:   in.QuotaBytes, ExpiryTime: in.ExpiryTime, MonthlyReset: in.MonthlyReset,
		CreatedAt: now, UpdatedAt: now,
	}
	if !a.Enabled {
		a.DisabledReason = "manual"
	}
	return a, nil
}

func Update(a model.Account, in Input) (model.Account, error) {
	normalizeEditableInput(&in)
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
	if in.Protocol == "" {
		in.Protocol = a.Protocol
	}
	if !strings.EqualFold(in.Protocol, a.Protocol) {
		return model.Account{}, errors.New("changing an existing account protocol is not supported; clone/create a new account instead")
	}
	oldPort := a.Port
	oldView, _ := ToView(a)
	mergeMissing(&in, oldView)
	normalizeEditableInput(&in)
	if err := validateEditableInput(in); err != nil {
		return model.Account{}, err
	}
	settings, stream, err := updateProtocolJSON(a, in)
	if err != nil {
		return model.Account{}, err
	}
	a.Name, a.Enabled, a.Port = in.Name, in.Enabled, in.Port
	a.QuotaBytes, a.ExpiryTime, a.MonthlyReset = in.QuotaBytes, in.ExpiryTime, in.MonthlyReset
	a.SettingsJSON, a.StreamSettingsJSON = settings, stream
	a.UpdatedAt = time.Now().UnixMilli()
	if a.Enabled {
		a.DisabledReason = ""
	} else if a.DisabledReason == "" || a.DisabledReason == "quota" || a.DisabledReason == "expiry" {
		a.DisabledReason = "manual"
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
	if len(name) > 80 {
		return model.Account{}, errors.New("name is too long")
	}
	clone := a
	clone.ID = 0
	clone.Name = name
	clone.Port = port
	clone.UpBytes, clone.DownBytes, clone.AllTimeBytes = 0, 0, 0
	clone.LastMonthlyReset = ""
	clone.Tag = fmt.Sprintf("xport-%d-%s", port, shortSlug(name))
	clone.CreatedAt, clone.UpdatedAt = time.Now().UnixMilli(), time.Now().UnixMilli()
	if clone.DisabledReason == "quota" {
		clone.Enabled = true
	}
	clone.DisabledReason = ""
	if err := rotateCredential(&clone, name, port); err != nil {
		return model.Account{}, err
	}
	return clone, nil
}

func ToView(a model.Account) (View, error) {
	v := View{
		ID: a.ID, Name: a.Name, Enabled: a.Enabled, DisabledReason: a.DisabledReason,
		Port: a.Port, Protocol: strings.ToLower(a.Protocol), QuotaBytes: a.QuotaBytes,
		ExpiryTime: a.ExpiryTime, MonthlyReset: a.MonthlyReset, UpBytes: a.UpBytes,
		DownBytes: a.DownBytes, AllTime: a.AllTimeBytes, Tag: a.Tag,
	}
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(defaultJSON(a.SettingsJSON)), &settings); err != nil {
		return v, err
	}
	v.Editable = extractProtocolView(&v, settings)
	extractStreamView(&v, a.StreamSettingsJSON)
	return v, nil
}

func PublicView(a model.Account) View {
	v, _ := ToView(a)
	v.Credential, v.Username, v.Password, v.ServerPassword, v.PrivateKey = "", "", "", "", ""
	return v
}

func ShareURI(a model.Account, host string) (string, error) {
	v, err := ToView(a)
	if err != nil {
		return "", err
	}
	if !v.Editable {
		return "", errors.New("this account cannot be represented by the built-in share editor")
	}
	host = normalizeHost(host)
	if host == "" {
		return "", errors.New("host is required")
	}
	switch strings.ToLower(v.Protocol) {
	case "vless":
		if v.Credential == "" {
			return "", errors.New("VLESS UUID is empty")
		}
		u := &url.URL{Scheme: "vless", Host: net.JoinHostPort(host, strconv.Itoa(v.Port)), User: url.User(v.Credential), Fragment: v.Name}
		q, err := commonShareQuery(v)
		if err != nil {
			return "", err
		}
		if v.Flow != "" {
			q.Set("flow", v.Flow)
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	case "trojan":
		if v.Password == "" {
			return "", errors.New("Trojan password is empty")
		}
		u := &url.URL{Scheme: "trojan", Host: net.JoinHostPort(host, strconv.Itoa(v.Port)), User: url.User(v.Password), Fragment: v.Name}
		q, err := commonShareQuery(v)
		if err != nil {
			return "", err
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	case "shadowsocks":
		password := v.Password
		if strings.HasPrefix(v.Method, "2022-") && v.ServerPassword != "" && v.Password != "" {
			password = v.ServerPassword + ":" + v.Password
		} else if password == "" {
			password = v.ServerPassword
		}
		if v.Method == "" || password == "" {
			return "", errors.New("Shadowsocks method/password is empty")
		}
		user := base64.RawURLEncoding.EncodeToString([]byte(v.Method + ":" + password))
		u := &url.URL{Scheme: "ss", Host: net.JoinHostPort(host, strconv.Itoa(v.Port)), User: url.User(user), Fragment: v.Name}
		q, err := commonShareQuery(v)
		if err != nil {
			return "", err
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	case "vmess":
		obj := map[string]any{"v": "2", "ps": v.Name, "add": host, "port": strconv.Itoa(v.Port), "id": v.Credential, "aid": "0", "scy": defaultString(v.ClientSecurity, "auto"), "net": defaultString(v.Network, "tcp"), "type": "none", "host": v.Host, "path": v.Path, "tls": securityForVMess(v.Security), "sni": v.ServerName}
		b, _ := json.Marshal(obj)
		return "vmess://" + base64.StdEncoding.EncodeToString(b), nil
	case "socks":
		u := &url.URL{Scheme: "socks", Host: net.JoinHostPort(host, strconv.Itoa(v.Port)), Fragment: v.Name}
		if v.Username != "" || v.Password != "" {
			u.User = url.UserPassword(v.Username, v.Password)
		}
		return u.String(), nil
	case "http":
		u := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(v.Port)), Fragment: v.Name}
		if v.Username != "" || v.Password != "" {
			u.User = url.UserPassword(v.Username, v.Password)
		}
		return u.String(), nil
	default:
		return "", fmt.Errorf("share links for %s are not supported", v.Protocol)
	}
}

func applyDefaults(in *Input) {
	if in.Network == "" {
		in.Network = "tcp"
	}
	switch in.Protocol {
	case "vless":
		if in.Credential == "" {
			in.Credential = newUUID()
		}
		if in.Security == "" {
			in.Security = "reality"
		}
		if in.Flow == "" && (in.Network == "tcp" || in.Network == "raw") && (in.Security == "tls" || in.Security == "reality") {
			in.Flow = "xtls-rprx-vision"
		}
	case "vmess":
		if in.Credential == "" {
			in.Credential = newUUID()
		}
		if in.ClientSecurity == "" {
			in.ClientSecurity = "auto"
		}
		if in.Security == "" {
			in.Security = "none"
		}
	case "trojan":
		if in.Password == "" {
			in.Password = randomSecret(18)
		}
		if in.Security == "" {
			in.Security = "reality"
		}
	case "shadowsocks":
		if in.Method == "" {
			in.Method = "chacha20-ietf-poly1305"
		}
		if in.ServerPassword == "" {
			in.ServerPassword = randomSecret(24)
		}
		if in.Password == "" {
			in.Password = randomSecret(20)
		}
		if in.Security == "" {
			in.Security = "none"
		}
	case "socks", "http":
		if in.Username == "" {
			in.Username = shortSlug(in.Name)
		}
		if in.Password == "" {
			in.Password = randomSecret(18)
		}
		in.Security = "none"
		in.Network = ""
	}
	if (in.Protocol == "vless" || in.Protocol == "trojan") && in.Security == "reality" {
		if strings.TrimSpace(in.ServerName) != "" && strings.TrimSpace(in.Dest) == "" {
			in.Dest = net.JoinHostPort(in.ServerName, "443")
		}
		if in.PrivateKey == "" {
			privateKey, _, err := newX25519Keypair()
			if err == nil {
				in.PrivateKey = privateKey
			}
		}
		if in.ShortID == "" {
			in.ShortID = newShortID()
		}
	}
}

func buildProtocol(in Input) (string, string, error) {
	settings := map[string]any{}
	email := accountEmail(in.Name, in.Port)
	switch in.Protocol {
	case "vless":
		if in.Credential == "" {
			return "", "", errors.New("VLESS UUID is required")
		}
		settings = map[string]any{"clients": []any{map[string]any{"id": in.Credential, "email": email, "flow": in.Flow}}, "decryption": "none"}
	case "vmess":
		if in.Credential == "" {
			return "", "", errors.New("VMess UUID is required")
		}
		settings = map[string]any{"clients": []any{map[string]any{"id": in.Credential, "email": email, "security": defaultString(in.ClientSecurity, "auto")}}}
	case "trojan":
		if in.Password == "" {
			return "", "", errors.New("Trojan password is required")
		}
		settings = map[string]any{"clients": []any{map[string]any{"password": in.Password, "email": email}}}
	case "shadowsocks":
		if in.Method == "" || in.ServerPassword == "" {
			return "", "", errors.New("Shadowsocks method and server password are required")
		}
		settings = map[string]any{"method": in.Method, "password": in.ServerPassword, "network": "tcp,udp", "clients": []any{map[string]any{"password": in.Password, "email": email}}}
	case "socks":
		settings = map[string]any{"auth": "password", "accounts": []any{map[string]any{"user": in.Username, "pass": in.Password}}, "udp": true, "ip": "127.0.0.1"}
	case "http":
		settings = map[string]any{"accounts": []any{map[string]any{"user": in.Username, "pass": in.Password}}, "allowTransparent": false}
	default:
		return "", "", fmt.Errorf("new %s accounts are not supported by the built-in editor", in.Protocol)
	}
	sb, err := json.Marshal(settings)
	if err != nil {
		return "", "", err
	}
	stream, err := buildStream(in, nil)
	if err != nil {
		return "", "", err
	}
	return string(sb), stream, nil
}

func updateProtocolJSON(a model.Account, in Input) (string, string, error) {
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(defaultJSON(a.SettingsJSON)), &settings); err != nil {
		return "", "", err
	}
	email := accountEmail(in.Name, in.Port)
	switch strings.ToLower(a.Protocol) {
	case "vless", "vmess", "trojan":
		clients, err := oneObject(settings, "clients")
		if err != nil {
			return "", "", err
		}
		switch strings.ToLower(a.Protocol) {
		case "vless":
			clients["id"], clients["email"] = in.Credential, email
			setOrDelete(clients, "flow", in.Flow)
			if _, ok := settings["decryption"]; !ok {
				settings["decryption"] = "none"
			}
		case "vmess":
			clients["id"], clients["email"] = in.Credential, email
			setOrDelete(clients, "security", in.ClientSecurity)
		case "trojan":
			clients["password"], clients["email"] = in.Password, email
		}
		settings["clients"] = []any{clients}
	case "shadowsocks":
		settings["method"] = in.Method
		settings["password"] = in.ServerPassword
		clients, err := oneObjectOptional(settings, "clients")
		if err != nil {
			return "", "", err
		}
		if clients != nil {
			clients["password"], clients["email"] = in.Password, email
			settings["clients"] = []any{clients}
		}
	case "socks", "http":
		accounts, err := oneObject(settings, "accounts")
		if err != nil {
			return "", "", err
		}
		accounts["user"], accounts["pass"] = in.Username, in.Password
		settings["accounts"] = []any{accounts}
	default:
		return "", "", fmt.Errorf("editing %s is not supported by the built-in editor", a.Protocol)
	}
	sb, err := json.Marshal(settings)
	if err != nil {
		return "", "", err
	}
	stream, err := buildStream(in, []byte(defaultJSON(a.StreamSettingsJSON)))
	if err != nil {
		return "", "", err
	}
	return string(sb), stream, nil
}

func buildStream(in Input, existing []byte) (string, error) {
	if in.Protocol == "socks" || in.Protocol == "http" {
		if len(existing) > 0 && json.Valid(existing) {
			return string(existing), nil
		}
		return `{}`, nil
	}
	stream := map[string]any{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &stream)
	}
	stream["network"] = defaultString(in.Network, "tcp")
	stream["security"] = defaultString(in.Security, "none")
	if in.Security == "reality" {
		if in.Protocol != "vless" && in.Protocol != "trojan" {
			return "", errors.New("REALITY is supported by the editor for VLESS and Trojan")
		}
		if strings.TrimSpace(in.ServerName) == "" || strings.TrimSpace(in.PrivateKey) == "" {
			return "", errors.New("REALITY requires serverName and privateKey")
		}
		r := object(stream, "realitySettings")
		r["target"] = in.Dest
		delete(r, "dest")
		r["serverNames"] = []string{in.ServerName}
		r["privateKey"] = in.PrivateKey
		r["shortIds"] = []string{in.ShortID}
		stream["realitySettings"] = r
	} else {
		delete(stream, "realitySettings")
	}
	if in.Security == "tls" && in.ServerName != "" {
		tls := object(stream, "tlsSettings")
		tls["serverName"] = in.ServerName
		stream["tlsSettings"] = tls
	}
	applyTransport(stream, in)
	b, err := json.Marshal(stream)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func applyTransport(stream map[string]any, in Input) {
	switch in.Network {
	case "ws":
		v := object(stream, "wsSettings")
		if in.Path != "" {
			v["path"] = in.Path
		}
		if in.Host != "" {
			headers := object(v, "headers")
			headers["Host"] = in.Host
			v["headers"] = headers
		}
		stream["wsSettings"] = v
	case "grpc":
		v := object(stream, "grpcSettings")
		if in.ServiceName != "" {
			v["serviceName"] = in.ServiceName
		}
		stream["grpcSettings"] = v
	case "httpupgrade":
		v := object(stream, "httpupgradeSettings")
		if in.Path != "" {
			v["path"] = in.Path
		}
		if in.Host != "" {
			v["host"] = in.Host
		}
		stream["httpupgradeSettings"] = v
	case "xhttp":
		v := object(stream, "xhttpSettings")
		if in.Path != "" {
			v["path"] = in.Path
		}
		if in.Host != "" {
			v["host"] = in.Host
		}
		stream["xhttpSettings"] = v
	}
}

func extractProtocolView(v *View, settings map[string]any) bool {
	switch v.Protocol {
	case "vless", "vmess", "trojan":
		if v.Protocol == "vless" {
			decryption := strings.TrimSpace(stringValue(settings["decryption"]))
			if decryption != "" && decryption != "none" {
				return false
			}
		}
		client, err := oneObject(settings, "clients")
		if err != nil {
			return false
		}
		switch v.Protocol {
		case "vless":
			v.Credential = stringValue(client["id"])
			v.Flow = stringValue(client["flow"])
		case "vmess":
			v.Credential = stringValue(client["id"])
			v.ClientSecurity = defaultString(stringValue(client["security"]), "auto")
		case "trojan":
			v.Password = stringValue(client["password"])
		}
		return true
	case "shadowsocks":
		v.Method = stringValue(settings["method"])
		v.ServerPassword = stringValue(settings["password"])
		client, err := oneObjectOptional(settings, "clients")
		if err != nil {
			return false
		}
		if client != nil {
			v.Password = stringValue(client["password"])
		}
		return v.Method != "" && (v.ServerPassword != "" || v.Password != "")
	case "socks", "http":
		account, err := oneObject(settings, "accounts")
		if err != nil {
			return false
		}
		v.Username, v.Password = stringValue(account["user"]), stringValue(account["pass"])
		return true
	default:
		return false
	}
}

func extractStreamView(v *View, raw string) {
	stream := map[string]any{}
	if json.Unmarshal([]byte(defaultJSON(raw)), &stream) != nil {
		return
	}
	v.Network, v.Security = stringValue(stream["network"]), stringValue(stream["security"])
	if v.Network == "" && v.Protocol != "socks" && v.Protocol != "http" {
		v.Network = "tcp"
	}
	if v.Security == "" {
		v.Security = "none"
	}
	if v.Security == "reality" {
		r := object(stream, "realitySettings")
		v.PrivateKey = stringValue(r["privateKey"])
		v.Dest = firstString(r["target"], r["dest"])
		v.ServerName = firstArrayString(r["serverNames"])
		v.ShortID = firstArrayString(r["shortIds"])
	} else if v.Security == "tls" {
		tls := object(stream, "tlsSettings")
		v.ServerName = stringValue(tls["serverName"])
	}
	switch v.Network {
	case "ws":
		x := object(stream, "wsSettings")
		v.Path = stringValue(x["path"])
		v.Host = firstString(x["host"], object(x, "headers")["Host"], object(x, "headers")["host"])
	case "grpc":
		x := object(stream, "grpcSettings")
		v.ServiceName = stringValue(x["serviceName"])
	case "httpupgrade":
		x := object(stream, "httpupgradeSettings")
		v.Path, v.Host = stringValue(x["path"]), stringValue(x["host"])
	case "xhttp":
		x := object(stream, "xhttpSettings")
		v.Path, v.Host = stringValue(x["path"]), stringValue(x["host"])
	}
}

func rotateCredential(a *model.Account, name string, port int) error {
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(defaultJSON(a.SettingsJSON)), &settings); err != nil {
		return err
	}
	email := accountEmail(name, port)
	switch strings.ToLower(a.Protocol) {
	case "vless", "vmess":
		client, err := oneObject(settings, "clients")
		if err != nil {
			return err
		}
		client["id"], client["email"] = newUUID(), email
		settings["clients"] = []any{client}
	case "trojan":
		client, err := oneObject(settings, "clients")
		if err != nil {
			return err
		}
		client["password"], client["email"] = randomSecret(18), email
		settings["clients"] = []any{client}
	case "shadowsocks":
		client, err := oneObjectOptional(settings, "clients")
		if err != nil {
			return err
		}
		if client != nil {
			client["password"], client["email"] = randomSecret(20), email
			settings["clients"] = []any{client}
		} else {
			settings["password"] = randomSecret(24)
		}
	case "socks", "http":
		account, err := oneObject(settings, "accounts")
		if err != nil {
			return err
		}
		account["user"], account["pass"] = shortSlug(name), randomSecret(18)
		settings["accounts"] = []any{account}
	default:
		return fmt.Errorf("clone credential rotation for %s is not supported", a.Protocol)
	}
	b, _ := json.Marshal(settings)
	a.SettingsJSON = string(b)
	return nil
}

func commonShareQuery(v View) (url.Values, error) {
	q := url.Values{}
	if v.Network != "" {
		q.Set("type", v.Network)
	}
	security := defaultString(v.Security, "none")
	q.Set("security", security)
	if v.Host != "" {
		q.Set("host", v.Host)
	}
	if v.Path != "" {
		q.Set("path", v.Path)
	}
	if v.ServiceName != "" {
		q.Set("serviceName", v.ServiceName)
	}
	if security == "reality" {
		publicKey, err := publicKeyFromPrivate(v.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("derive REALITY public key: %w", err)
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
	} else if security == "tls" && v.ServerName != "" {
		q.Set("sni", v.ServerName)
	}
	return q, nil
}

func mergeMissing(in *Input, old View) {
	if in.Credential == "" {
		in.Credential = old.Credential
	}
	if in.Username == "" {
		in.Username = old.Username
	}
	if in.Password == "" {
		in.Password = old.Password
	}
	if in.ServerPassword == "" {
		in.ServerPassword = old.ServerPassword
	}
	if in.ClientSecurity == "" {
		in.ClientSecurity = old.ClientSecurity
	}
	if in.Method == "" {
		in.Method = old.Method
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
	if in.Host == "" {
		in.Host = old.Host
	}
	if in.Path == "" {
		in.Path = old.Path
	}
	if in.ServiceName == "" {
		in.ServiceName = old.ServiceName
	}
}

func oneObject(m map[string]any, key string) (map[string]any, error) {
	v, ok := m[key].([]any)
	if !ok || len(v) != 1 {
		return nil, fmt.Errorf("%s must contain exactly one entry", key)
	}
	o, ok := v[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid %s entry", key)
	}
	return o, nil
}

func oneObjectOptional(m map[string]any, key string) (map[string]any, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return nil, nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid %s list", key)
	}
	if len(a) == 0 {
		return nil, nil
	}
	if len(a) != 1 {
		return nil, fmt.Errorf("%s must contain exactly one entry", key)
	}
	o, ok := a[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid %s entry", key)
	}
	return o, nil
}

func object(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok && v != nil {
		return v
	}
	return map[string]any{}
}

func setOrDelete(m map[string]any, key, value string) {
	if value == "" {
		delete(m, key)
	} else {
		m[key] = value
	}
}

func firstArrayString(v any) string {
	if a, ok := v.([]any); ok && len(a) > 0 {
		return stringValue(a[0])
	}
	if a, ok := v.([]string); ok && len(a) > 0 {
		return a[0]
	}
	if s := stringValue(v); s != "" {
		return strings.Split(s, ",")[0]
	}
	return ""
}

func firstString(values ...any) string {
	for _, v := range values {
		if s := stringValue(v); s != "" {
			return s
		}
	}
	return ""
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func defaultJSON(s string) string {
	if strings.TrimSpace(s) == "" {
		return `{}`
	}
	return s
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func securityForVMess(s string) string {
	if s == "tls" {
		return "tls"
	}
	return ""
}

func normalizeEditableInput(in *Input) {
	in.Name = strings.TrimSpace(in.Name)
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.Network = strings.ToLower(strings.TrimSpace(in.Network))
	in.Security = strings.ToLower(strings.TrimSpace(in.Security))
	in.Method = strings.ToLower(strings.TrimSpace(in.Method))
	in.Credential = strings.TrimSpace(in.Credential)
	in.Username = strings.TrimSpace(in.Username)
	in.ServerName = strings.TrimSpace(in.ServerName)
	in.Dest = strings.TrimSpace(in.Dest)
	in.PrivateKey = strings.TrimSpace(in.PrivateKey)
	in.ShortID = strings.TrimSpace(in.ShortID)
	in.Host = strings.TrimSpace(in.Host)
	in.Path = strings.TrimSpace(in.Path)
	in.ServiceName = strings.TrimSpace(in.ServiceName)
}

func validateEditableInput(in Input) error {
	checks := []struct {
		name, value string
		max         int
	}{
		{"name", in.Name, 80}, {"protocol", in.Protocol, 32}, {"credential", in.Credential, 256},
		{"username", in.Username, 256}, {"password", in.Password, 4096}, {"server password", in.ServerPassword, 4096},
		{"client security", in.ClientSecurity, 64}, {"method", in.Method, 128}, {"flow", in.Flow, 128},
		{"network", in.Network, 32}, {"security", in.Security, 32}, {"server name", in.ServerName, 512},
		{"target", in.Dest, 1024}, {"private key", in.PrivateKey, 256}, {"short id", in.ShortID, 128},
		{"host", in.Host, 512}, {"path", in.Path, 2048}, {"service name", in.ServiceName, 512},
	}
	for _, c := range checks {
		if len(c.value) > c.max {
			return fmt.Errorf("%s is too long", c.name)
		}
		if strings.ContainsRune(c.value, '\x00') || strings.ContainsAny(c.value, "\r\n") {
			return fmt.Errorf("%s contains invalid control characters", c.name)
		}
	}
	if in.Name == "" {
		return errors.New("name is required")
	}
	if in.QuotaBytes < 0 {
		return errors.New("quotaBytes must not be negative")
	}
	if in.ExpiryTime < 0 {
		return errors.New("expiryTime must not be negative")
	}
	switch in.Protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "socks", "http":
	default:
		return fmt.Errorf("protocol %q is not supported by the built-in editor", in.Protocol)
	}
	if in.Protocol != "socks" && in.Protocol != "http" {
		switch in.Network {
		case "tcp", "raw", "ws", "websocket", "grpc", "httpupgrade", "xhttp":
		default:
			return fmt.Errorf("network %q is not supported by the built-in editor", in.Network)
		}
		switch in.Security {
		case "none", "tls":
		case "reality":
			if in.Protocol != "vless" && in.Protocol != "trojan" {
				return errors.New("REALITY is supported only for VLESS and Trojan")
			}
		default:
			return fmt.Errorf("security %q is not supported by the built-in editor", in.Security)
		}
	}
	if in.Security == "reality" {
		switch in.Network {
		case "tcp", "raw", "xhttp", "grpc":
		default:
			return fmt.Errorf("REALITY is not supported with %s transport", in.Network)
		}
	}
	if in.Protocol == "vless" && in.Flow != "" {
		if in.Flow != "xtls-rprx-vision" {
			return fmt.Errorf("unsupported VLESS flow %q", in.Flow)
		}
		if (in.Network != "tcp" && in.Network != "raw") || (in.Security != "tls" && in.Security != "reality") {
			return errors.New("xtls-rprx-vision requires RAW/TCP with TLS or REALITY in the built-in editor")
		}
	}
	if in.Protocol == "vmess" && in.ClientSecurity != "" {
		switch strings.ToLower(in.ClientSecurity) {
		case "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero":
		default:
			return fmt.Errorf("VMess client security %q is not supported", in.ClientSecurity)
		}
	}
	return nil
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

func newShortID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

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

func randomSecret(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
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
