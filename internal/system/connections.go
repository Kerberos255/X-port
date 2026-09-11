package system

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// EstablishedTCPByLocalPort returns the number of ESTABLISHED TCP sockets whose
// local port is one of ports. Reading /proc/net/tcp{,6} is cheap and avoids
// spawning ss/netstat on every UI refresh. For multiplexed transports such as
// XHTTP, one client may own multiple TCP connections, so this is a connection
// count rather than a device count.
func EstablishedTCPByLocalPort(ports map[int]struct{}) map[int]int {
	out := make(map[int]int, len(ports))
	for p := range ports {
		out[p] = 0
	}
	parseProcTCP("/proc/net/tcp", ports, out)
	parseProcTCP("/proc/net/tcp6", ports, out)
	return out
}

func parseProcTCP(path string, ports map[int]struct{}, out map[int]int) {
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
		local := fields[1]
		i := strings.LastIndexByte(local, ':')
		if i < 0 || i == len(local)-1 {
			continue
		}
		p64, err := strconv.ParseUint(local[i+1:], 16, 16)
		if err != nil {
			continue
		}
		p := int(p64)
		if _, ok := ports[p]; ok {
			out[p]++
		}
	}
}
