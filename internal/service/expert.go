package service

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kerberos255/X-port/internal/model"
)

// ExpertConfig exposes low-frequency settings using the names and nesting used
// by current Xray-core while preserving every unexposed field in the stored raw
// JSON. PublicKey is derived from RealityPrivateKey and is read-only.
type ExpertConfig struct {
	Protocol string `json:"protocol"`
	Method   string `json:"method"`
	Security string `json:"security"`

	FallbacksJSON       string `json:"fallbacksJson"`
	AcceptProxyProtocol bool   `json:"acceptProxyProtocol"`
	HTTPHeaderJSON      string `json:"httpHeaderJson"`

	RealityTarget       string   `json:"realityTarget"`
	RealityServerNames  []string `json:"realityServerNames"`
	RealityPrivateKey   string   `json:"realityPrivateKey"`
	RealityPublicKey    string   `json:"realityPublicKey"`
	RealityShortIDs     []string `json:"realityShortIds"`
	RealityXver         uint64   `json:"realityXver"`
	RealityMinClientVer string   `json:"realityMinClientVer"`
	RealityMaxClientVer string   `json:"realityMaxClientVer"`
	RealityMaxTimeDiff  uint64   `json:"realityMaxTimeDiff"`
	RealityShow         bool     `json:"realityShow"`
	RealityMLDSA65Seed  string   `json:"realityMldsa65Seed"`

	TLSALPN             []string `json:"tlsAlpn"`
	TLSMinVersion       string   `json:"tlsMinVersion"`
	TLSMaxVersion       string   `json:"tlsMaxVersion"`
	TLSCipherSuites     string   `json:"tlsCipherSuites"`
	TLSRejectUnknownSNI bool     `json:"tlsRejectUnknownSni"`
	TLSCertificatesJSON string   `json:"tlsCertificatesJson"`

	XHTTPMode                 string `json:"xhttpMode"`
	XHTTPXPaddingBytes        string `json:"xhttpXPaddingBytes"`
	XHTTPNoSSEHeader          bool   `json:"xhttpNoSseHeader"`
	XHTTPScMaxBufferedPosts   int64  `json:"xhttpScMaxBufferedPosts"`
	XHTTPScMaxEachPostBytes   string `json:"xhttpScMaxEachPostBytes"`
	XHTTPScStreamUpServerSecs string `json:"xhttpScStreamUpServerSecs"`
	XHTTPUplinkHTTPMethod     string `json:"xhttpUplinkHttpMethod"`

	SupportsFallbacks  bool `json:"supportsFallbacks"`
	SupportsHTTPHeader bool `json:"supportsHttpHeader"`
	SupportsReality    bool `json:"supportsReality"`
	SupportsTLS        bool `json:"supportsTls"`
	SupportsXHTTP      bool `json:"supportsXhttp"`
}

func (s *Accounts) Expert(id int64) (ExpertConfig, error) {
	a, err := s.raw(id)
	if err != nil {
		return ExpertConfig{}, err
	}
	return expertView(a)
}

func (s *Accounts) UpdateExpert(id int64, in ExpertConfig) (ExpertConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, err := s.store.Accounts()
	if err != nil {
		return ExpertConfig{}, err
	}
	idx := indexID(old, id)
	if idx < 0 {
		return ExpertConfig{}, errors.New("account not found")
	}
	next := copyAccounts(old)
	a := &next[idx]
	settings, stream, err := expertMaps(*a)
	if err != nil {
		return ExpertConfig{}, err
	}
	protocol := strings.ToLower(strings.TrimSpace(a.Protocol))
	method := expertMethod(stream, protocol)
	security := strings.ToLower(strings.TrimSpace(expertString(stream["security"])))
	if security == "" {
		security = "none"
	}

	fallbacks, err := parseExpertArray(in.FallbacksJSON, "fallbacks")
	if err != nil {
		return ExpertConfig{}, err
	}
	if len(fallbacks) > 0 {
		if protocol != "vless" && protocol != "trojan" {
			return ExpertConfig{}, errors.New("fallbacks are supported only for VLESS or Trojan")
		}
		if method != "raw" || security != "tls" {
			return ExpertConfig{}, errors.New("fallbacks require RAW/TCP transport with TLS")
		}
		settings["fallbacks"] = fallbacks
	} else {
		delete(settings, "fallbacks")
	}

	// Xray accepts acceptProxyProtocol in sockopt for the listener. Older panels
	// may keep transport-local copies; remove those only for the active transport
	// so one authoritative value remains without disturbing unrelated raw JSON.
	for _, key := range expertActiveTransportKeys(method, stream) {
		if obj, ok := stream[key].(map[string]any); ok {
			delete(obj, "acceptProxyProtocol")
		}
	}
	sockopt := expertObject(stream, "sockopt")
	if in.AcceptProxyProtocol {
		sockopt["acceptProxyProtocol"] = true
		stream["sockopt"] = sockopt
	} else {
		delete(sockopt, "acceptProxyProtocol")
		if len(sockopt) == 0 {
			delete(stream, "sockopt")
		} else {
			stream["sockopt"] = sockopt
		}
	}

	header, hasHeader, err := parseExpertObject(in.HTTPHeaderJSON, "HTTP camouflage header")
	if err != nil {
		return ExpertConfig{}, err
	}
	if hasHeader && method != "raw" {
		return ExpertConfig{}, errors.New("HTTP camouflage header requires RAW/TCP transport")
	}
	if method == "raw" {
		key := expertRawKey(stream)
		raw := expertObject(stream, key)
		if hasHeader {
			typ := strings.ToLower(strings.TrimSpace(expertString(header["type"])))
			if typ != "none" && typ != "http" {
				return ExpertConfig{}, errors.New("HTTP camouflage header type must be none or http")
			}
			raw["header"] = header
		} else {
			delete(raw, "header")
		}
		if len(raw) == 0 {
			delete(stream, key)
		} else {
			stream[key] = raw
		}
	}

	if security == "reality" {
		r := expertObject(stream, "realitySettings")
		if strings.TrimSpace(in.RealityTarget) != "" {
			r["target"] = strings.TrimSpace(in.RealityTarget)
			delete(r, "dest") // target is the current Xray spelling; dest remains an alias.
		}
		if len(in.RealityServerNames) > 0 {
			r["serverNames"] = cleanStrings(in.RealityServerNames)
		}
		if strings.TrimSpace(in.RealityPrivateKey) != "" {
			if _, err := deriveRealityPublicKey(in.RealityPrivateKey); err != nil {
				return ExpertConfig{}, err
			}
			r["privateKey"] = strings.TrimSpace(in.RealityPrivateKey)
		}
		if len(in.RealityShortIDs) > 0 {
			r["shortIds"] = cleanStrings(in.RealityShortIDs)
		}
		r["xver"] = in.RealityXver
		setExpertString(r, "minClientVer", in.RealityMinClientVer)
		setExpertString(r, "maxClientVer", in.RealityMaxClientVer)
		if in.RealityMaxTimeDiff > 0 { r["maxTimeDiff"] = in.RealityMaxTimeDiff } else { delete(r, "maxTimeDiff") }
		if in.RealityShow { r["show"] = true } else { delete(r, "show") }
		setExpertString(r, "mldsa65Seed", in.RealityMLDSA65Seed)
		stream["realitySettings"] = r
	}

	if security == "tls" {
		tls := expertObject(stream, "tlsSettings")
		if len(in.TLSALPN) > 0 { tls["alpn"] = cleanStrings(in.TLSALPN) } else { delete(tls, "alpn") }
		setExpertString(tls, "minVersion", in.TLSMinVersion)
		setExpertString(tls, "maxVersion", in.TLSMaxVersion)
		setExpertString(tls, "cipherSuites", in.TLSCipherSuites)
		if in.TLSRejectUnknownSNI { tls["rejectUnknownSni"] = true } else { delete(tls, "rejectUnknownSni") }
		certs, err := parseExpertArray(in.TLSCertificatesJSON, "TLS certificates")
		if err != nil { return ExpertConfig{}, err }
		if len(certs) > 0 { tls["certificates"] = certs } else { delete(tls, "certificates") }
		stream["tlsSettings"] = tls
	}

	if method == "xhttp" {
		key := expertXHTTPKey(stream)
		x := expertObject(stream, key)
		mode := strings.TrimSpace(in.XHTTPMode)
		switch mode {
		case "", "auto", "packet-up", "stream-up", "stream-one":
		default:
			return ExpertConfig{}, fmt.Errorf("unsupported XHTTP mode %q", mode)
		}
		setExpertString(x, "mode", mode)
		setExpertString(x, "xPaddingBytes", in.XHTTPXPaddingBytes)
		if in.XHTTPNoSSEHeader { x["noSSEHeader"] = true } else { delete(x, "noSSEHeader") }
		if in.XHTTPScMaxBufferedPosts > 0 { x["scMaxBufferedPosts"] = in.XHTTPScMaxBufferedPosts } else { delete(x, "scMaxBufferedPosts") }
		setExpertString(x, "scMaxEachPostBytes", in.XHTTPScMaxEachPostBytes)
		setExpertString(x, "scStreamUpServerSecs", in.XHTTPScStreamUpServerSecs)
		setExpertString(x, "uplinkHTTPMethod", strings.ToUpper(strings.TrimSpace(in.XHTTPUplinkHTTPMethod)))
		stream[key] = x
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil { return ExpertConfig{}, err }
	streamJSON, err := json.Marshal(stream)
	if err != nil { return ExpertConfig{}, err }
	a.SettingsJSON = string(settingsJSON)
	a.StreamSettingsJSON = string(streamJSON)
	a.UpdatedAt = time.Now().UnixMilli()
	if err := s.applyAndPersist(old, next); err != nil {
		return ExpertConfig{}, err
	}
	saved, err := s.raw(id)
	if err != nil { return ExpertConfig{}, err }
	return expertView(saved)
}

func expertView(a model.Account) (ExpertConfig, error) {
	settings, stream, err := expertMaps(a)
	if err != nil { return ExpertConfig{}, err }
	protocol := strings.ToLower(strings.TrimSpace(a.Protocol))
	method := expertMethod(stream, protocol)
	security := strings.ToLower(strings.TrimSpace(expertString(stream["security"])))
	if security == "" { security = "none" }
	out := ExpertConfig{Protocol: protocol, Method: method, Security: security}
	out.SupportsFallbacks = (protocol == "vless" || protocol == "trojan") && method == "raw" && security == "tls"
	out.SupportsHTTPHeader = method == "raw"
	out.SupportsReality = security == "reality"
	out.SupportsTLS = security == "tls"
	out.SupportsXHTTP = method == "xhttp"

	if v, ok := settings["fallbacks"]; ok {
		out.FallbacksJSON = indentJSON(v, "[]")
	} else { out.FallbacksJSON = "[]" }

	if sockopt, ok := stream["sockopt"].(map[string]any); ok {
		out.AcceptProxyProtocol = expertBool(sockopt["acceptProxyProtocol"])
	}
	if !out.AcceptProxyProtocol {
		for _, key := range expertActiveTransportKeys(method, stream) {
			if obj, ok := stream[key].(map[string]any); ok && expertBool(obj["acceptProxyProtocol"]) {
				out.AcceptProxyProtocol = true
				break
			}
		}
	}
	if method == "raw" {
		raw := expertObject(stream, expertRawKey(stream))
		if header, ok := raw["header"]; ok { out.HTTPHeaderJSON = indentJSON(header, "") }
	}

	if out.SupportsReality {
		r := expertObject(stream, "realitySettings")
		out.RealityTarget = expertFirstString(r["target"], r["dest"])
		out.RealityServerNames = expertStringSlice(r["serverNames"])
		out.RealityPrivateKey = expertString(r["privateKey"])
		out.RealityPublicKey, _ = deriveRealityPublicKey(out.RealityPrivateKey)
		out.RealityShortIDs = expertStringSlice(r["shortIds"])
		out.RealityXver = expertUint(r["xver"])
		out.RealityMinClientVer = expertString(r["minClientVer"])
		out.RealityMaxClientVer = expertString(r["maxClientVer"])
		out.RealityMaxTimeDiff = expertUint(r["maxTimeDiff"])
		out.RealityShow = expertBool(r["show"])
		out.RealityMLDSA65Seed = expertString(r["mldsa65Seed"])
	}
	if out.SupportsTLS {
		tls := expertObject(stream, "tlsSettings")
		out.TLSALPN = expertStringSlice(tls["alpn"])
		out.TLSMinVersion = expertString(tls["minVersion"])
		out.TLSMaxVersion = expertString(tls["maxVersion"])
		out.TLSCipherSuites = expertString(tls["cipherSuites"])
		out.TLSRejectUnknownSNI = expertBool(tls["rejectUnknownSni"])
		if certs, ok := tls["certificates"]; ok { out.TLSCertificatesJSON = indentJSON(certs, "[]") } else { out.TLSCertificatesJSON = "[]" }
	}
	if out.SupportsXHTTP {
		x := expertObject(stream, expertXHTTPKey(stream))
		out.XHTTPMode = expertString(x["mode"])
		out.XHTTPXPaddingBytes = expertScalarString(x["xPaddingBytes"])
		out.XHTTPNoSSEHeader = expertBool(x["noSSEHeader"])
		out.XHTTPScMaxBufferedPosts = int64(expertUint(x["scMaxBufferedPosts"]))
		out.XHTTPScMaxEachPostBytes = expertScalarString(x["scMaxEachPostBytes"])
		out.XHTTPScStreamUpServerSecs = expertScalarString(x["scStreamUpServerSecs"])
		out.XHTTPUplinkHTTPMethod = expertString(x["uplinkHTTPMethod"])
	}
	return out, nil
}

func expertMaps(a model.Account) (map[string]any, map[string]any, error) {
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(defaultAdvancedJSON(a.SettingsJSON)), &settings); err != nil {
		return nil, nil, fmt.Errorf("invalid account settings JSON: %w", err)
	}
	stream := map[string]any{}
	if err := json.Unmarshal([]byte(defaultAdvancedJSON(a.StreamSettingsJSON)), &stream); err != nil {
		return nil, nil, fmt.Errorf("invalid stream settings JSON: %w", err)
	}
	return settings, stream, nil
}

func expertMethod(stream map[string]any, protocol string) string {
	m := strings.ToLower(strings.TrimSpace(expertFirstString(stream["method"], stream["network"])))
	if m == "" && protocol != "socks" && protocol != "http" { m = "raw" }
	switch m { case "tcp": return "raw"; case "ws": return "websocket"; case "splithttp": return "xhttp"; case "mkcp": return "kcp" }
	return m
}

func expertActiveTransportKeys(method string, stream map[string]any) []string {
	switch method {
	case "raw": return []string{expertRawKey(stream)}
	case "xhttp": return []string{expertXHTTPKey(stream)}
	case "websocket": return []string{"wsSettings"}
	case "httpupgrade": return []string{"httpupgradeSettings"}
	case "grpc": return []string{"grpcSettings"}
	default: return nil
	}
}
func expertRawKey(stream map[string]any) string { if _, ok := stream["rawSettings"]; ok { return "rawSettings" }; if _, ok := stream["tcpSettings"]; ok { return "tcpSettings" }; return "rawSettings" }
func expertXHTTPKey(stream map[string]any) string { if _, ok := stream["xhttpSettings"]; ok { return "xhttpSettings" }; if _, ok := stream["splithttpSettings"]; ok { return "splithttpSettings" }; return "xhttpSettings" }
func expertObject(m map[string]any, key string) map[string]any { if v, ok := m[key].(map[string]any); ok && v != nil { return v }; return map[string]any{} }
func expertString(v any) string { s, _ := v.(string); return s }
func expertFirstString(vs ...any) string { for _, v := range vs { if s := expertString(v); s != "" { return s } }; return "" }
func expertBool(v any) bool { b, _ := v.(bool); return b }
func expertUint(v any) uint64 { switch n := v.(type) { case float64: if n > 0 { return uint64(n) }; case json.Number: u, _ := n.Int64(); if u > 0 { return uint64(u) } }; return 0 }
func expertScalarString(v any) string { if v == nil { return "" }; if s, ok := v.(string); ok { return s }; b, _ := json.Marshal(v); return strings.TrimSpace(string(b)) }
func expertStringSlice(v any) []string { a, ok := v.([]any); if !ok { if s, ok := v.([]string); ok { return cleanStrings(s) }; return nil }; out := make([]string,0,len(a)); for _, x := range a { if s, ok := x.(string); ok && strings.TrimSpace(s)!="" { out=append(out,strings.TrimSpace(s)) } }; return out }
func cleanStrings(in []string) []string { out:=make([]string,0,len(in)); for _,s:=range in { s=strings.TrimSpace(s); if s!="" { out=append(out,s) } }; return out }
func setExpertString(m map[string]any, key, value string) { value=strings.TrimSpace(value); if value=="" { delete(m,key) } else { m[key]=value } }
func indentJSON(v any, fallback string) string { b, err := json.MarshalIndent(v,"","  "); if err != nil { return fallback }; return string(b) }
func parseExpertArray(raw, label string) ([]any,error) { raw=strings.TrimSpace(raw); if raw==""||raw=="null"||raw=="[]" { return nil,nil }; var v []any; if err:=json.Unmarshal([]byte(raw),&v);err!=nil{return nil,fmt.Errorf("%s must be a JSON array: %w",label,err)}; return v,nil }
func parseExpertObject(raw, label string)(map[string]any,bool,error){raw=strings.TrimSpace(raw);if raw==""||raw=="null"||raw=="{}"{return nil,false,nil};var v map[string]any;if err:=json.Unmarshal([]byte(raw),&v);err!=nil{return nil,false,fmt.Errorf("%s must be a JSON object: %w",label,err)};return v,len(v)>0,nil}
func deriveRealityPublicKey(privateKey string)(string,error){privateKey=strings.TrimSpace(privateKey);if privateKey==""{return "",nil};raw,err:=base64.RawURLEncoding.DecodeString(privateKey);if err!=nil||len(raw)!=32{return "",errors.New("invalid REALITY private key")};pk,err:=ecdh.X25519().NewPrivateKey(raw);if err!=nil{return "",errors.New("invalid REALITY private key")};return base64.RawURLEncoding.EncodeToString(pk.PublicKey().Bytes()),nil}
