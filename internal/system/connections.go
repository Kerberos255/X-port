package system

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// TCPActivity describes current established TCP activity for one local port.
// Peers is the number of unique remote IP addresses, which is a better
// approximation of active clients than raw socket count. It is still not an
// exact device count: clients behind the same NAT may share an IP, one device
// can use more than one IP, and UDP traffic is not represented here.
type TCPActivity struct {
	Connections int
	Peers       int
}

// EstablishedTCPActivityByLocalPort returns ESTABLISHED TCP socket counts and
// unique remote-IP counts for the requested local ports. Reading
// /proc/net/tcp{,6} is cheap and avoids spawning ss/netstat on every UI refresh.
func EstablishedTCPActivityByLocalPort(ports map[int]struct{}) map[int]TCPActivity {
	connections := make(map[int]int, len(ports))
	peers := make(map[int]map[string]struct{}, len(ports))
	for p := range ports {
		connections[p] = 0
		peers[p] = make(map[string]struct{})
	}
	parseProcTCPActivity("/proc/net/tcp", "4", ports, connections, peers)
	parseProcTCPActivity("/proc/net/tcp6", "6", ports, connections, peers)
	out := make(map[int]TCPActivity, len(ports))
	for p := range ports {
		out[p] = TCPActivity{Connections: connections[p], Peers: len(peers[p])}
	}
	return out
}

// EstablishedTCPByLocalPort preserves the original connection-count helper for
// callers that explicitly need raw ESTABLISHED socket counts.
func EstablishedTCPByLocalPort(ports map[int]struct{}) map[int]int {
	activity := EstablishedTCPActivityByLocalPort(ports)
	out := make(map[int]int, len(activity))
	for p, a := range activity {
		out[p] = a.Connections
	}
	return out
}

func parseProcTCPActivity(path, family string, ports map[int]struct{}, connections map[int]int, peers map[int]map[string]struct{}) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	first := true
	for s.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(s.Text())
		if len(fields) < 4 || fields[3] != "01" { // 01 = TCP_ESTABLISHED
			continue
		}
		localPort, ok := procPort(fields[1])
		if !ok {
			continue
		}
		if _, wanted := ports[localPort]; !wanted {
			continue
		}
		connections[localPort]++
		if remoteAddr, ok := procAddress(fields[2]); ok {
			peers[localPort][family+":"+strings.ToLower(remoteAddr)] = struct{}{}
		}
	}
}

// parseProcTCP is kept for the focused parser test and raw-count callers.
func parseProcTCP(path string, ports map[int]struct{}, out map[int]int) {
	peers := make(map[int]map[string]struct{}, len(ports))
	for p := range ports {
		peers[p] = make(map[string]struct{})
	}
	parseProcTCPActivity(path, "test", ports, out, peers)
}

func procPort(endpoint string) (int, bool) {
	i := strings.LastIndexByte(endpoint, ':')
	if i < 0 || i == len(endpoint)-1 {
		return 0, false
	}
	p64, err := strconv.ParseUint(endpoint[i+1:], 16, 16)
	if err != nil {
		return 0, false
	}
	return int(p64), true
}

func procAddress(endpoint string) (string, bool) {
	i := strings.LastIndexByte(endpoint, ':')
	if i <= 0 {
		return "", false
	}
	addr := endpoint[:i]
	if strings.Trim(addr, "0") == "" {
		return "", false
	}
	return addr, true
}
