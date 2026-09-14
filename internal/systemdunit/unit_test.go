package systemdunit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderCustomPaths(t *testing.T) {
	paths := Paths{
		DataDir:     "/srv/xport-data",
		Prefix:      "/opt/xport",
		XportBinary: "/opt/bin/xport",
	}
	panel, xray, err := Render(paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"EnvironmentFile=-/srv/xport-data/xport.env",
		"ExecStart=/opt/bin/xport serve --data /srv/xport-data --xray-binary /opt/xport/bin/xray --xray-config /srv/xport-data/xray/config.json --xray-service xport-xray.service",
		"ReadWritePaths=/srv/xport-data /opt/xport /opt/bin",
	} {
		if !strings.Contains(panel, want) {
			t.Fatalf("panel unit missing %q\n%s", want, panel)
		}
	}
	for _, want := range []string{
		"Environment=XRAY_LOCATION_ASSET=/opt/xport/bin",
		"ExecStart=/opt/xport/bin/xray run -config /srv/xport-data/xray/config.json",
	} {
		if !strings.Contains(xray, want) {
			t.Fatalf("xray unit missing %q\n%s", want, xray)
		}
	}
}

func TestDetectPathsFromCustomUnits(t *testing.T) {
	want := Paths{DataDir: "/srv/xport-data", Prefix: "/opt/xport", XportBinary: "/opt/bin/xport"}
	panel, xray, err := Render(want)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	panelPath := filepath.Join(dir, PanelUnitName)
	xrayPath := filepath.Join(dir, XrayUnitName)
	if err := os.WriteFile(panelPath, []byte(panel), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xrayPath, []byte(xray), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := DetectPaths(panelPath, xrayPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("DetectPaths() = %#v, want %#v", got, want)
	}
}

func TestDetectPathsUsesXrayUnitWhenPanelMissing(t *testing.T) {
	want := Paths{DataDir: "/srv/xport-data", Prefix: "/opt/xport", XportBinary: DefaultPaths().XportBinary}
	_, xray, err := Render(Paths{DataDir: want.DataDir, Prefix: want.Prefix, XportBinary: "/opt/bin/xport"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	xrayPath := filepath.Join(dir, XrayUnitName)
	if err := os.WriteFile(xrayPath, []byte(xray), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := DetectPaths(filepath.Join(dir, PanelUnitName), xrayPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("DetectPaths() = %#v, want %#v", got, want)
	}
}

func TestWriteMatchesRender(t *testing.T) {
	paths := Paths{DataDir: "/srv/xport-data", Prefix: "/opt/xport", XportBinary: "/opt/bin/xport"}
	panel, xray, err := Render(paths)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Write(dir, paths); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{PanelUnitName: panel, XrayUnitName: xray} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != want {
			t.Fatalf("%s does not match Render output", name)
		}
	}
}

func TestPathsRejectWhitespace(t *testing.T) {
	paths := DefaultPaths()
	paths.DataDir = "/srv/x port"
	if _, _, err := Render(paths); err == nil {
		t.Fatal("expected whitespace path to be rejected")
	}
}
