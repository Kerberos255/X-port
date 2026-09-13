package xray

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Kerberos255/X-port/internal/model"
)

const maxEditableGlobalConfigBytes = 2 << 20

var editableGlobalXraySections = map[string]struct{}{
	"routing": {},
	"dns":      {},
	"outbounds": {},
	"policy":   {},
}

var managedGlobalXraySections = map[string]struct{}{
	"api":      {},
	"stats":    {},
	"inbounds": {},
}

// EditableGlobalConfig returns only the global Xray sections exposed by X-port.
// Other preserved migration sections remain stored but are intentionally hidden.
func EditableGlobalConfig(baseJSON string) ([]byte, error) {
	base, err := parseGlobalConfigObject(baseJSON, "stored global Xray config")
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(editableGlobalXraySections))
	for key := range editableGlobalXraySections {
		if value, ok := base[key]; ok {
			out[key] = value
		}
	}
	return json.MarshalIndent(out, "", "  ")
}

// MergeEditableGlobalConfig replaces only routing/dns/outbounds/policy while
// preserving safe hidden migration sections such as log/transport/fakedns.
// X-port-managed api/stats/inbounds are always removed from the persisted base.
func MergeEditableGlobalConfig(baseJSON, editableJSON string) (string, []byte, error) {
	base, err := parseGlobalConfigObject(baseJSON, "stored global Xray config")
	if err != nil {
		return "", nil, err
	}
	editable, err := parseGlobalConfigObject(editableJSON, "global Xray advanced config")
	if err != nil {
		return "", nil, err
	}
	for key := range editable {
		if _, ok := editableGlobalXraySections[key]; !ok {
			if _, managed := managedGlobalXraySections[key]; managed {
				return "", nil, fmt.Errorf("%s is managed by X-port and cannot be overridden here", key)
			}
			return "", nil, fmt.Errorf("section %q is not editable here; allowed sections are routing, dns, outbounds, policy", key)
		}
	}
	for key := range editableGlobalXraySections {
		delete(base, key)
	}
	for key := range managedGlobalXraySections {
		delete(base, key)
	}
	for key, value := range editable {
		base[key] = value
	}
	merged, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		return "", nil, err
	}
	normalized, err := json.MarshalIndent(editable, "", "  ")
	if err != nil {
		return "", nil, err
	}
	return string(merged), normalized, nil
}

func parseGlobalConfigObject(raw, label string) (map[string]json.RawMessage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]json.RawMessage{}, nil
	}
	if len(raw) > maxEditableGlobalConfigBytes {
		return nil, fmt.Errorf("%s is too large", label)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", label, err)
	}
	if out == nil {
		return nil, errors.New(label + " must be a JSON object")
	}
	return out, nil
}

// ApplyWithBase applies accounts against a candidate base configuration. The
// Manager runtime value changes only after Xray validation/restart succeeds.
func (m *Manager) ApplyWithBase(accounts []model.Account, baseJSON string) error {
	previous := m.BaseConfigJSON
	m.BaseConfigJSON = baseJSON
	if err := m.Apply(accounts); err != nil {
		m.BaseConfigJSON = previous
		return err
	}
	return nil
}
