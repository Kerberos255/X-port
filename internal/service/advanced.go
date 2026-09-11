package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kerberos255/X-port/internal/model"
)

type AdvancedConfig struct {
	Protocol            string `json:"protocol"`
	Network             string `json:"network"`
	Security            string `json:"security"`
	FallbacksJSON       string `json:"fallbacksJson"`
	AcceptProxyProtocol bool   `json:"acceptProxyProtocol"`
	HTTPHeaderJSON      string `json:"httpHeaderJson"`
	SupportsFallbacks   bool   `json:"supportsFallbacks"`
	SupportsHTTPHeader  bool   `json:"supportsHttpHeader"`
}

func (s *Accounts) Advanced(id int64) (AdvancedConfig, error) {
	a, err := s.raw(id)
	if err != nil {
		return AdvancedConfig{}, err
	}
	return advancedView(a)
}

func (s *Accounts) UpdateAdvanced(id int64, in AdvancedConfig) (AdvancedConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, err := s.store.Accounts()
	if err != nil {
		return AdvancedConfig{}, err
	}
	idx := indexID(old, id)
	if idx < 0 {
		return AdvancedConfig{}, errors.New("account not found")
	}
	next := copyAccounts(old)
	a := &next[idx]

	settings := map[string]any{}
	if err := json.Unmarshal([]byte(defaultAdvancedJSON(a.SettingsJSON)), &settings); err != nil {
		return AdvancedConfig{}, fmt.Errorf("invalid account settings JSON: %w", err)
	}
	stream := map[string]any{}
	if err := json.Unmarshal([]byte(defaultAdvancedJSON(a.StreamSettingsJSON)), &stream); err != nil {
		return AdvancedConfig{}, fmt.Errorf("invalid stream settings JSON: %w", err)
	}

	protocol := strings.ToLower(strings.TrimSpace(a.Protocol))
	network := strings.ToLower(strings.TrimSpace(advancedString(stream["network"])))
	if network == "" && protocol != "socks" && protocol != "http" {
		network = "tcp"
	}
	security := strings.ToLower(strings.TrimSpace(advancedString(stream["security"])))
	if security == "" {
		security = "none"
	}

	fallbacks, err := parseFallbacks(in.FallbacksJSON)
	if err != nil {
		return AdvancedConfig{}, err
	}
	if len(fallbacks) > 0 {
		if protocol != "vless" && protocol != "trojan" {
			return AdvancedConfig{}, errors.New("fallbacks are only supported for VLESS or Trojan")
		}
		if network != "tcp" && network != "raw" {
			return AdvancedConfig{}, errors.New("fallbacks require TCP/RAW transport")
		}
		if security != "tls" {
			return AdvancedConfig{}, errors.New("fallbacks require TLS transport security")
		}
		settings["fallbacks"] = fallbacks
	} else {
		delete(settings, "fallbacks")
	}

	// Normalize PROXY protocol to streamSettings.sockopt, which is supported by
	// current Xray transports. Remove transport-local copies to avoid conflicting
	// values inherited from older panel schemas.
	for _, key := range []string{"tcpSettings", "rawSettings", "wsSettings", "grpcSettings", "httpupgradeSettings", "xhttpSettings"} {
		if obj, ok := stream[key].(map[string]any); ok {
			delete(obj, "acceptProxyProtocol")
		}
	}
	sockopt, _ := stream["sockopt"].(map[string]any)
	if sockopt == nil {
		sockopt = map[string]any{}
	}
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

	header, hasHeader, err := parseHTTPHeader(in.HTTPHeaderJSON)
	if err != nil {
		return AdvancedConfig{}, err
	}
	if hasHeader && network != "tcp" && network != "raw" {
		return AdvancedConfig{}, errors.New("HTTP camouflage header requires TCP/RAW transport")
	}
	if network == "tcp" || network == "raw" {
		key := "tcpSettings"
		if network == "raw" {
			key = "rawSettings"
		} else if _, ok := stream["rawSettings"]; ok {
			key = "rawSettings"
		}
		transport, _ := stream[key].(map[string]any)
		if transport == nil {
			transport = map[string]any{}
		}
		if hasHeader {
			transport["header"] = header
			stream[key] = transport
		} else {
			delete(transport, "header")
			if len(transport) == 0 {
				delete(stream, key)
			} else {
				stream[key] = transport
			}
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return AdvancedConfig{}, err
	}
	streamJSON, err := json.Marshal(stream)
	if err != nil {
		return AdvancedConfig{}, err
	}
	a.SettingsJSON = string(settingsJSON)
	a.StreamSettingsJSON = string(streamJSON)
	a.UpdatedAt = time.Now().UnixMilli()

	if err := s.applyAndPersist(old, next); err != nil {
		return AdvancedConfig{}, err
	}
	saved, err := s.raw(id)
	if err != nil {
		return AdvancedConfig{}, err
	}
	return advancedView(saved)
}

func advancedView(a model.Account) (AdvancedConfig, error) {
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(defaultAdvancedJSON(a.SettingsJSON)), &settings); err != nil {
		return AdvancedConfig{}, err
	}
	stream := map[string]any{}
	if err := json.Unmarshal([]byte(defaultAdvancedJSON(a.StreamSettingsJSON)), &stream); err != nil {
		return AdvancedConfig{}, err
	}
	protocol := strings.ToLower(strings.TrimSpace(a.Protocol))
	network := strings.ToLower(strings.TrimSpace(advancedString(stream["network"])))
	if network == "" && protocol != "socks" && protocol != "http" {
		network = "tcp"
	}
	security := strings.ToLower(strings.TrimSpace(advancedString(stream["security"])))
	if security == "" {
		security = "none"
	}

	fallbacksJSON := "[]"
	if v, ok := settings["fallbacks"]; ok {
		if b, err := json.MarshalIndent(v, "", "  "); err == nil {
			fallbacksJSON = string(b)
		}
	}
	acceptProxy := false
	if sockopt, ok := stream["sockopt"].(map[string]any); ok {
		acceptProxy = advancedBool(sockopt["acceptProxyProtocol"])
	}
	if !acceptProxy {
		for _, key := range []string{"tcpSettings", "rawSettings", "wsSettings", "grpcSettings", "httpupgradeSettings", "xhttpSettings"} {
			if obj, ok := stream[key].(map[string]any); ok && advancedBool(obj["acceptProxyProtocol"]) {
				acceptProxy = true
				break
			}
		}
	}

	headerJSON := ""
	if network == "tcp" || network == "raw" {
		for _, key := range []string{"rawSettings", "tcpSettings"} {
			if obj, ok := stream[key].(map[string]any); ok {
				if header, ok := obj["header"]; ok {
					if b, err := json.MarshalIndent(header, "", "  "); err == nil {
						headerJSON = string(b)
					}
					break
				}
			}
		}
	}

	return AdvancedConfig{
		Protocol: protocol, Network: network, Security: security,
		FallbacksJSON: fallbacksJSON, AcceptProxyProtocol: acceptProxy,
		HTTPHeaderJSON: headerJSON,
		SupportsFallbacks: (protocol == "vless" || protocol == "trojan") && (network == "tcp" || network == "raw"),
		SupportsHTTPHeader: network == "tcp" || network == "raw",
	}, nil
}

func parseFallbacks(raw string) ([]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}
	var v []any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("fallbacks must be a JSON array: %w", err)
	}
	return v, nil
}

func parseHTTPHeader(raw string) (map[string]any, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return nil, false, nil
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false, fmt.Errorf("HTTP camouflage header must be a JSON object: %w", err)
	}
	if len(v) == 0 {
		return nil, false, nil
	}
	typ := strings.ToLower(strings.TrimSpace(advancedString(v["type"])))
	if typ != "none" && typ != "http" {
		return nil, false, errors.New("HTTP camouflage header type must be none or http")
	}
	return v, true, nil
}

func defaultAdvancedJSON(v string) string {
	if strings.TrimSpace(v) == "" {
		return `{}`
	}
	return v
}

func advancedString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func advancedBool(v any) bool {
	b, _ := v.(bool)
	return b
}
