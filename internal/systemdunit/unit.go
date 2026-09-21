package systemdunit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	PanelUnitName = "xport.service"
	XrayUnitName  = "xport-xray.service"
)

type Paths struct {
	DataDir     string
	Prefix      string
	XportBinary string
}

func DefaultPaths() Paths {
	return Paths{
		DataDir:     "/etc/x-port",
		Prefix:      "/usr/local/x-port",
		XportBinary: "/usr/local/bin/xport",
	}
}

func (p Paths) XrayBinary() string { return filepath.Join(p.Prefix, "bin", "xray") }
func (p Paths) XrayConfig() string { return filepath.Join(p.DataDir, "xray", "config.json") }

func (p Paths) Validate() error {
	for name, value := range map[string]string{
		"data directory": p.DataDir,
		"prefix":         p.Prefix,
		"X-port binary":  p.XportBinary,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
		if !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be absolute: %s", name, value)
		}
		if strings.ContainsAny(value, " \t\r\n") {
			return fmt.Errorf("%s cannot contain whitespace: %s", name, value)
		}
	}
	return nil
}

func Render(paths Paths) (panel, xray string, err error) {
	if err := paths.Validate(); err != nil {
		return "", "", err
	}
	dataDir := filepath.Clean(paths.DataDir)
	prefix := filepath.Clean(paths.Prefix)
	xportBinary := filepath.Clean(paths.XportBinary)
	xportBinDir := filepath.Dir(xportBinary)
	xrayBinary := filepath.Join(prefix, "bin", "xray")
	xrayConfig := filepath.Join(dataDir, "xray", "config.json")

	panel = fmt.Sprintf(`[Unit]
Description=X-port Control Panel
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
EnvironmentFile=-%s/xport.env
ExecStart=%s serve --data %s --xray-binary %s --xray-config %s --xray-service %s
Restart=on-failure
RestartSec=3
Nice=5
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectHome=read-only
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictRealtime=true
LockPersonality=true
SystemCallArchitectures=native
ProtectSystem=strict
ReadWritePaths=%s %s %s
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
[Install]
WantedBy=multi-user.target
`, dataDir, xportBinary, dataDir, xrayBinary, xrayConfig, XrayUnitName, dataDir, prefix, xportBinDir)

	xray = fmt.Sprintf(`[Unit]
Description=X-port Xray Core
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
Environment=XRAY_LOCATION_ASSET=%s/bin
ExecStart=%s run -config %s
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectHome=read-only
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictRealtime=true
LockPersonality=true
SystemCallArchitectures=native
ProtectSystem=full
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
[Install]
WantedBy=multi-user.target
`, prefix, xrayBinary, xrayConfig)
	return panel, xray, nil
}

func Write(unitDir string, paths Paths) error {
	panel, xray, err := Render(paths)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(unitDir) {
		return errors.New("systemd unit directory must be absolute")
	}
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(unitDir, PanelUnitName), []byte(panel), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(unitDir, XrayUnitName), []byte(xray), 0644)
}

// DetectPaths keeps repair from silently replacing a custom installation with
// default paths. Existing unit files override the defaults when they contain
// enough information; missing files are tolerated so repair still works after
// one or both units were removed.
func DetectPaths(panelPath, xrayPath string) (Paths, error) {
	paths := DefaultPaths()
	if b, err := os.ReadFile(panelPath); err == nil {
		detectPanel(string(b), &paths)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Paths{}, err
	}
	if b, err := os.ReadFile(xrayPath); err == nil {
		detectXray(string(b), &paths)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Paths{}, err
	}
	return paths, paths.Validate()
}

func detectPanel(unit string, paths *Paths) {
	for _, raw := range strings.Split(unit, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "EnvironmentFile=-"):
			value := strings.TrimPrefix(line, "EnvironmentFile=-")
			if filepath.Base(value) == "xport.env" {
				paths.DataDir = filepath.Dir(value)
			}
		case strings.HasPrefix(line, "ExecStart="):
			fields := strings.Fields(strings.TrimPrefix(line, "ExecStart="))
			if len(fields) == 0 {
				continue
			}
			paths.XportBinary = fields[0]
			for i := 1; i+1 < len(fields); i++ {
				switch fields[i] {
				case "--data":
					paths.DataDir = fields[i+1]
				case "--xray-binary":
					paths.Prefix = prefixFromXrayBinary(fields[i+1], paths.Prefix)
				}
			}
		}
	}
}

func detectXray(unit string, paths *Paths) {
	for _, raw := range strings.Split(unit, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "Environment=XRAY_LOCATION_ASSET="):
			assetDir := strings.TrimPrefix(line, "Environment=XRAY_LOCATION_ASSET=")
			if filepath.Base(assetDir) == "bin" {
				paths.Prefix = filepath.Dir(assetDir)
			}
		case strings.HasPrefix(line, "ExecStart="):
			fields := strings.Fields(strings.TrimPrefix(line, "ExecStart="))
			if len(fields) > 0 {
				paths.Prefix = prefixFromXrayBinary(fields[0], paths.Prefix)
			}
			for i := 1; i+1 < len(fields); i++ {
				if fields[i] == "-config" {
					config := fields[i+1]
					if filepath.Base(config) == "config.json" && filepath.Base(filepath.Dir(config)) == "xray" {
						paths.DataDir = filepath.Dir(filepath.Dir(config))
					}
				}
			}
		}
	}
}

func prefixFromXrayBinary(binary, fallback string) string {
	if filepath.Base(binary) != "xray" || filepath.Base(filepath.Dir(binary)) != "bin" {
		return fallback
	}
	return filepath.Dir(filepath.Dir(binary))
}
