package xray

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEditableGlobalConfigOnlyExposesAllowedSections(t *testing.T) {
	raw, err := EditableGlobalConfig(`{"log":{"loglevel":"warning"},"dns":{"servers":["1.1.1.1"]},"routing":{"domainStrategy":"AsIs"},"transport":{"tcpSettings":{}}}`)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["dns"] == nil || got["routing"] == nil {
		t.Fatalf("unexpected editable config: %s", raw)
	}
	if _, ok := got["log"]; ok {
		t.Fatal("log must stay hidden")
	}
}

func TestMergeEditableGlobalConfigPreservesHiddenAndRemovesManaged(t *testing.T) {
	base := `{"log":{"loglevel":"warning"},"transport":{"tcpSettings":{}},"dns":{"servers":["old"]},"api":{"tag":"legacy"},"stats":{"legacy":true},"inbounds":[{"port":1}]}`
	merged, normalized, err := MergeEditableGlobalConfig(base, `{"dns":{"servers":["1.1.1.1"]},"routing":{"domainStrategy":"IPIfNonMatch"}}`)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(merged), &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"api", "stats", "inbounds"} {
		if _, ok := got[key]; ok {
			t.Fatalf("managed key %s must be removed: %s", key, merged)
		}
	}
	if got["log"] == nil || got["transport"] == nil || got["dns"] == nil || got["routing"] == nil {
		t.Fatalf("expected preserved/edited sections: %s", merged)
	}
	if strings.Contains(string(normalized), `"log"`) || !strings.Contains(string(normalized), `"dns"`) {
		t.Fatalf("unexpected normalized editable config: %s", normalized)
	}
}

func TestMergeEditableGlobalConfigRejectsManagedAndUnknownSections(t *testing.T) {
	for _, raw := range []string{
		`{"inbounds":[]}`,
		`{"api":{}}`,
		`{"log":{"loglevel":"debug"}}`,
	} {
		if _, _, err := MergeEditableGlobalConfig(`{}`, raw); err == nil {
			t.Fatalf("expected rejection for %s", raw)
		}
	}
}

func TestMergeEditableGlobalConfigRequiresJSONObject(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `"dns"`} {
		if _, _, err := MergeEditableGlobalConfig(`{}`, raw); err == nil {
			t.Fatalf("expected object rejection for %s", raw)
		}
	}
}

func TestApplyWithBaseCommitsRuntimeBaseAfterValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("temporary executable script test")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "xray")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.json")
	m := &Manager{BinaryPath: binary, ConfigPath: configPath, APIPort: 10085, BaseConfigJSON: `{"dns":{"servers":["old"]}}`}
	candidate := `{"dns":{"servers":["1.1.1.1"]}}`
	if err := m.ApplyWithBase(nil, candidate); err != nil {
		t.Fatal(err)
	}
	if m.BaseConfigJSON != candidate {
		t.Fatalf("runtime base not committed: %s", m.BaseConfigJSON)
	}
	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "1.1.1.1") {
		t.Fatalf("candidate base not rendered: %s", b)
	}
}

func TestApplyWithBaseRestoresRuntimeBaseWhenValidationFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("temporary executable script test")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "xray")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("previous"), 0640); err != nil {
		t.Fatal(err)
	}
	previous := `{"dns":{"servers":["old"]}}`
	m := &Manager{BinaryPath: binary, ConfigPath: configPath, APIPort: 10085, BaseConfigJSON: previous}
	if err := m.ApplyWithBase(nil, `{"dns":{"servers":["bad"]}}`); err == nil {
		t.Fatal("expected validation failure")
	}
	if m.BaseConfigJSON != previous {
		t.Fatalf("runtime base changed after failed validation: %s", m.BaseConfigJSON)
	}
	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "previous" {
		t.Fatalf("config file changed before validation succeeded: %q", b)
	}
}
