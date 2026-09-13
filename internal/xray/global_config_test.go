package xray

import (
	"encoding/json"
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
