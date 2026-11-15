package main

import (
	"context"
	"net"
)

// panelListenConfig opts out of Go's default MPTCP listener behavior.
// This is an attack-surface reduction, not a substitute for kernel updates.
func panelListenConfig() net.ListenConfig {
	var lc net.ListenConfig
	lc.SetMultipathTCP(false)
	return lc
}

func panelListener(address string) (net.Listener, error) {
	lc := panelListenConfig()
	return lc.Listen(context.Background(), "tcp", address)
}
