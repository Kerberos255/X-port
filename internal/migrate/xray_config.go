package migrate

import (
	"encoding/json"
	"fmt"
	"os"
)

var preservedGlobalXraySections = map[string]struct{}{
	"log": {}, "dns": {}, "routing": {}, "policy": {}, "outbounds": {},
	"transport": {}, "fakedns": {},
}

// ReadGlobalXrayConfig preserves only global sections that X-port can safely
// carry forward. Listener/API/metrics/reverse/observatory sections are rebuilt
// or intentionally omitted so a legacy config cannot silently introduce a new
// management listener or unrelated active behavior during migration.
func ReadGlobalXrayConfig(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(b) > 16<<20 {
		return "", fmt.Errorf("Xray config is unexpectedly large")
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		return "", fmt.Errorf("parse Xray config: %w", err)
	}
	out := make(map[string]any, len(preservedGlobalXraySections))
	for key := range preservedGlobalXraySections {
		if value, ok := cfg[key]; ok {
			out[key] = value
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
