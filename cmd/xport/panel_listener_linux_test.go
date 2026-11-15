package main

import (
	"net"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPanelListenerSocketProtocolIsTCP(t *testing.T) {
	// A forced Go default must not turn the panel's listening socket into MPTCP.
	t.Setenv("GODEBUG", "multipathtcp=2")
	ln, err := panelListener("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	tcpListener, ok := ln.(*net.TCPListener)
	if !ok {
		t.Fatalf("expected TCP listener, got %T", ln)
	}
	raw, err := tcpListener.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var protocol int
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		protocol, socketErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PROTOCOL)
	}); err != nil {
		t.Fatal(err)
	}
	if socketErr != nil {
		t.Fatal(socketErr)
	}
	if protocol != unix.IPPROTO_TCP {
		t.Fatalf("panel listening socket protocol = %d, want TCP (%d)", protocol, unix.IPPROTO_TCP)
	}
}
