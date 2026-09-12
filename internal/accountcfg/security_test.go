package accountcfg

import (
	"strings"
	"testing"
)

func TestInputValidationBoundsAndEnums(t *testing.T) {
	if _, err := New(Input{Name: strings.Repeat("a", 81), Enabled: true, Port: 20001, Protocol: "vless", ServerName: "example.com"}, 0); err == nil {
		t.Fatal("expected long name to fail")
	}
	if _, err := New(Input{Name: "a", Enabled: true, Port: 20001, Protocol: "vless", Network: "made-up", ServerName: "example.com"}, 0); err == nil {
		t.Fatal("expected unknown network to fail")
	}
	if _, err := New(Input{Name: "a", Enabled: true, Port: 20001, Protocol: "vmess", Security: "reality"}, 0); err == nil {
		t.Fatal("expected VMess REALITY to fail")
	}
	if _, err := New(Input{Name: "a\r\nb", Enabled: true, Port: 20001, Protocol: "vless", ServerName: "example.com"}, 0); err == nil {
		t.Fatal("expected control characters to fail")
	}
}
