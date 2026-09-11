package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProcTCP(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "tcp")
	data := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000:1F90 0100007F:C350 01 00000000:00000000 00:00000000 00000000 0 0 1\n" +
		"   1: 00000000:1F90 0100007F:C351 01 00000000:00000000 00:00000000 00000000 0 0 2\n" +
		"   2: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 3\n" +
		"   3: 00000000:2382 0100007F:C352 01 00000000:00000000 00:00000000 00000000 0 0 4\n"
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	ports := map[int]struct{}{8080: {}, 9090: {}}
	out := map[int]int{8080: 0, 9090: 0}
	parseProcTCP(p, ports, out)
	if out[8080] != 2 || out[9090] != 1 {
		t.Fatalf("unexpected counts: %#v", out)
	}
}
