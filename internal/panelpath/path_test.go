package panelpath

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"":              "/",
		"/":             "/",
		" secret ":      "/secret/",
		"/secret":       "/secret/",
		"secret/":       "/secret/",
		"///a/b///":     "/a/b/",
		"  /a/b/  ":     "/a/b/",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValid(t *testing.T) {
	valid := []string{"/", "/secret/", "/a/b/", "/A-1_b/"}
	for _, v := range valid {
		if !Valid(v) {
			t.Fatalf("Valid(%q) = false", v)
		}
	}
	invalid := []string{"", "secret", "/secret", "secret/", "//", "/a//b/", "/./", "/../", "/a/../b/", "/a b/", "/a?b/", "/a#b/", "/a\\b/", "/a\tb/", "/a\nb/"}
	for _, v := range invalid {
		if Valid(v) {
			t.Fatalf("Valid(%q) = true", v)
		}
	}
}
