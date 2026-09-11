package main

import "testing"

func TestManagerCommandRouting(t *testing.T) {
	for _, cmd := range []string{"menu", "status", "start", "stop", "restart", "restart-xray", "logs", "autostart", "settings", "admin", "update", "update-xray", "update-geodata", "backup", "firewall", "repair", "uninstall"} {
		if !isManagerCommand(cmd) { t.Fatalf("expected %q to be a manager command", cmd) }
	}
	for _, cmd := range []string{"serve", "init", "migrate", "render", "xray-check", "xray-update", "version", "unknown"} {
		if isManagerCommand(cmd) { t.Fatalf("did not expect %q to be intercepted by manager", cmd) }
	}
}

func TestValidManagerBasePath(t *testing.T) {
	valid := []string{"/", "/secret/", "/a/b/", "/x-1_2/"}
	invalid := []string{"", "secret/", "/secret", "//", "/../", "/a//b/", "/a b/", "/a?b/", "/a#b/", "/a\\b/"}
	for _, v := range valid { if !validManagerBasePath(v) { t.Errorf("expected valid base path %q", v) } }
	for _, v := range invalid { if validManagerBasePath(v) { t.Errorf("expected invalid base path %q", v) } }
}

func TestPrivateReleaseTokenPrefersEnvironment(t *testing.T) {
	t.Setenv("XPORT_GITHUB_TOKEN", "test-token")
	if got := privateReleaseToken(); got != "test-token" { t.Fatalf("privateReleaseToken() = %q", got) }
}
