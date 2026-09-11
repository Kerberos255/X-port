package accountcfg

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Kerberos255/X-port/internal/model"
)

func TestNewCloneShareReality(t *testing.T) {
	a, err := New(Input{Name: "Alpha", Enabled: true, Port: 21001, Protocol: "vless", ServerName: "www.example.com", Dest: "www.example.com:443"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := ToView(a)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Editable || v.Credential == "" || v.PrivateKey == "" || v.ShortID == "" {
		t.Fatalf("incomplete view: %+v", v)
	}
	uri, err := ShareURI(a, "proxy.example.net")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "vless" || !strings.Contains(u.Host, ":21001") || u.Query().Get("pbk") == "" {
		t.Fatalf("bad uri: %s", uri)
	}
	clone, err := Clone(a, "Beta", 21002)
	if err != nil {
		t.Fatal(err)
	}
	cv, _ := ToView(clone)
	if cv.Credential == v.Credential || clone.Port != 21002 || clone.ID != 0 {
		t.Fatalf("clone not isolated: %+v", cv)
	}
}

func TestNextPort(t *testing.T) {
	a, _ := New(Input{Name: "A", Enabled: true, Port: 20000, ServerName: "a.example"}, 0)
	if got := NextPort([]model.Account{a}); got != 20001 {
		t.Fatalf("got %d", got)
	}
}
