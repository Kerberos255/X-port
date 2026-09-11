package accountcfg

import "github.com/Kerberos255/X-port/internal/model"

// NextPort is the deterministic account-model helper retained for callers and
// tests. The runtime service performs the stronger DB + TCP/UDP availability
// check before it actually assigns a port.
func NextPort(accounts []model.Account) int {
	used := make(map[int]bool, len(accounts))
	for _, a := range accounts { used[a.Port] = true }
	for p := 20000; p <= 60000; p++ { if !used[p] { return p } }
	return 0
}
