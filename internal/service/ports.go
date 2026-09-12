package service

import (
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"net"
	"strconv"

	"github.com/Kerberos255/X-port/internal/model"
)

func ensurePortFree(port int) error {
	addr := net.JoinHostPort("", strconv.Itoa(port))
	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d is already in use (TCP)", port)
	}
	_ = tcp.Close()
	udp, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("port %d is already in use (UDP)", port)
	}
	_ = udp.Close()
	return nil
}

func randomPortStart(minPort, maxPort int) int {
	count := maxPort - minPort + 1
	if count <= 1 {
		return minPort
	}
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(count)))
	if err != nil {
		return minPort
	}
	return minPort + int(n.Int64())
}

func nextFreePortFrom(accounts []model.Account, minPort, maxPort, reserved, start int) int {
	if minPort < 1 || maxPort < minPort {
		return 0
	}
	if start < minPort || start > maxPort {
		start = minPort
	}
	used := map[int]bool{reserved: true}
	for _, a := range accounts {
		used[a.Port] = true
	}
	count := maxPort - minPort + 1
	for i := 0; i < count; i++ {
		p := minPort + ((start - minPort + i) % count)
		if used[p] {
			continue
		}
		if ensurePortFree(p) == nil {
			return p
		}
	}
	return 0
}

func nextFreePort(accounts []model.Account, minPort, maxPort, reserved int) int {
	return nextFreePortFrom(accounts, minPort, maxPort, reserved, randomPortStart(minPort, maxPort))
}
