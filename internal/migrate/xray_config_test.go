package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadGlobalXrayConfigAllowlist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"log":{"loglevel":"warning"},"dns":{"servers":["1.1.1.1"]},"routing":{"domainStrategy":"AsIs"},"outbounds":[{"protocol":"freedom"}],"transport":{},"fakedns":[],"inbounds":[{"port":1}],"api":{"tag":"api"},"stats":{},"metrics":{"listen":"0.0.0.0:11111"},"reverse":{"bridges":[]},"observatory":{"subjectSelector":[""]}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadGlobalXrayConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"log", "dns", "routing", "outbounds", "transport", "fakedns"} {
		if _, ok := cfg[keep]; !ok {
			t.Fatalf("expected %s to be preserved: %s", keep, got)
		}
	}
	for _, drop := range []string{"inbounds", "api", "stats", "metrics", "reverse", "observatory"} {
		if _, ok := cfg[drop]; ok {
			t.Fatalf("expected %s to be removed: %s", drop, got)
		}
	}
}
