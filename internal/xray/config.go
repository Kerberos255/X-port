package xray

import (
	"encoding/json"
	"fmt"

	"github.com/Kerberos255/X-port/internal/model"
)

type inbound struct {
	Listen         string          `json:"listen,omitempty"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings,omitempty"`
	Tag            string          `json:"tag"`
	Sniffing       json.RawMessage `json:"sniffing,omitempty"`
}

func BuildConfig(accounts []model.Account, apiPort int) ([]byte, error) {
	inbounds := make([]inbound, 0, len(accounts)+1)
	seenPorts := map[int]bool{}
	seenTags := map[string]bool{}
	for _, a := range accounts {
		if !a.Enabled {
			continue
		}
		if a.Port < 1 || a.Port > 65535 {
			return nil, fmt.Errorf("account %q has invalid port %d", a.Name, a.Port)
		}
		if seenPorts[a.Port] {
			return nil, fmt.Errorf("duplicate port %d", a.Port)
		}
		if a.Tag == "" {
			return nil, fmt.Errorf("account %q has empty tag", a.Name)
		}
		if seenTags[a.Tag] {
			return nil, fmt.Errorf("duplicate tag %q", a.Tag)
		}
		seenPorts[a.Port], seenTags[a.Tag] = true, true
		if !json.Valid([]byte(a.SettingsJSON)) {
			return nil, fmt.Errorf("account %q has invalid settings JSON", a.Name)
		}
		if !json.Valid([]byte(a.StreamSettingsJSON)) {
			return nil, fmt.Errorf("account %q has invalid stream settings JSON", a.Name)
		}
		if !json.Valid([]byte(a.SniffingJSON)) {
			return nil, fmt.Errorf("account %q has invalid sniffing JSON", a.Name)
		}
		inbounds = append(inbounds, inbound{Listen: a.Listen, Port: a.Port, Protocol: a.Protocol, Settings: json.RawMessage(a.SettingsJSON), StreamSettings: json.RawMessage(a.StreamSettingsJSON), Tag: a.Tag, Sniffing: json.RawMessage(a.SniffingJSON)})
	}
	if apiPort <= 0 {
		apiPort = 10085
	}
	if seenPorts[apiPort] {
		return nil, fmt.Errorf("Xray API port %d conflicts with an account", apiPort)
	}
	inbounds = append(inbounds, inbound{Listen: "127.0.0.1", Port: apiPort, Protocol: "dokodemo-door", Settings: json.RawMessage(`{"address":"127.0.0.1"}`), Tag: "api"})
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"api":       map[string]any{"tag": "api", "services": []string{"HandlerService", "LoggerService", "StatsService"}},
		"inbounds":  inbounds,
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct"}, map[string]any{"protocol": "blackhole", "tag": "blocked"}},
		"policy": map[string]any{
			"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true}},
			"system": map[string]any{"statsInboundUplink": true, "statsInboundDownlink": true, "statsOutboundUplink": true, "statsOutboundDownlink": true},
		},
		"routing": map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}},
		"stats":   map[string]any{},
	}
	return json.MarshalIndent(cfg, "", "  ")
}
