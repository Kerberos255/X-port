package service

import (
	"net"
	"testing"
)

func TestRandomPortStartStaysWithinRange(t *testing.T) {
	for i := 0; i < 128; i++ {
		got := randomPortStart(20000, 60000)
		if got < 20000 || got > 60000 {
			t.Fatalf("random start %d outside configured range", got)
		}
	}
}

func TestNextFreePortFromHonorsReservedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	if got := nextFreePortFrom(nil, port, port, 0, port); got != port {
		t.Fatalf("expected the only free port %d, got %d", port, got)
	}
	if got := nextFreePortFrom(nil, port, port, port, port); got != 0 {
		t.Fatalf("reserved port %d must not be selected, got %d", port, got)
	}
}
