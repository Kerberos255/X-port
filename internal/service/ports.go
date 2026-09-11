package service

import (
	"fmt"
	"net"
	"strconv"

	"github.com/Kerberos255/X-port/internal/model"
)

func ensurePortFree(port int) error {
	addr := net.JoinHostPort("", strconv.Itoa(port))
	tcp, err := net.Listen("tcp", addr)
	if err != nil { return fmt.Errorf("port %d is already in use (TCP)", port) }
	_ = tcp.Close()
	udp, err := net.ListenPacket("udp", addr)
	if err != nil { return fmt.Errorf("port %d is already in use (UDP)", port) }
	_ = udp.Close()
	return nil
}

func nextFreePort(accounts []model.Account, minPort, maxPort, reserved int) int {
	used := map[int]bool{reserved:true}
	for _,a := range accounts { used[a.Port]=true }
	for p:=minPort;p<=maxPort;p++ { if used[p]{continue}; if ensurePortFree(p)==nil{return p} }
	return 0
}
