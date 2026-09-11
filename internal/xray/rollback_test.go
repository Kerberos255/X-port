package xray

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeFakeXray(t *testing.T, path, version string) {
	t.Helper()
	body := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "version" ]; then
  echo "Xray %s (fake)"
  exit 0
fi
if [ "$1" = "run" ] && [ "$2" = "-test" ]; then
  exit 0
fi
exit 1
`, version)
	if err := os.WriteFile(path, []byte(body), 0755); err != nil { t.Fatal(err) }
}

func TestRollbackSwapsCurrentAndPrevious(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "xray")
	prev := bin + ".previous"
	cfg := filepath.Join(dir, "config.json")
	writeFakeXray(t, bin, "26.3.27")
	writeFakeXray(t, prev, "26.3.20")
	if err := os.WriteFile(cfg, []byte(`{}`), 0600); err != nil { t.Fatal(err) }

	u := &Updater{BinaryPath:bin, ConfigPath:cfg}
	info := u.CheckRollback()
	if !info.Available || info.Current != "26.3.27" || info.Previous != "26.3.20" {
		t.Fatalf("unexpected initial rollback info: %+v", info)
	}

	info, err := u.Rollback()
	if err != nil { t.Fatal(err) }
	if info.Current != "26.3.20" || info.Previous != "26.3.27" || !info.Available {
		t.Fatalf("unexpected rollback result: %+v", info)
	}

	info, err = u.Rollback()
	if err != nil { t.Fatal(err) }
	if info.Current != "26.3.27" || info.Previous != "26.3.20" || !info.Available {
		t.Fatalf("rollback was not reversible: %+v", info)
	}
}
