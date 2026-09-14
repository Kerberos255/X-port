package main

import "testing"

func TestDispatchManagerPassesThroughLowLevelCommands(t *testing.T) {
	for _, cmd := range []string{"serve", "init", "migrate", "render", "xray-check", "xray-update", "version", "unknown"} {
		handled, code := dispatchManager([]string{cmd})
		if handled || code != 0 {
			t.Fatalf("dispatchManager(%q) = handled %t, code %d; want false, 0", cmd, handled, code)
		}
	}
}

func TestManagerCommandClassification(t *testing.T) {
	for _, cmd := range []string{"menu", "status", "start", "stop", "restart", "restart-xray", "logs", "autostart", "settings", "admin", "update", "update-xray", "update-geodata", "backup", "firewall", "repair", "uninstall"} {
		if !isManagerCommand(cmd) {
			t.Fatalf("isManagerCommand(%q) = false", cmd)
		}
	}
}
