package migrate

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

type PanelSettings struct {
	BasePath string `json:"basePath,omitempty"`
	CertFile string `json:"certFile,omitempty"`
	KeyFile  string `json:"keyFile,omitempty"`
	Domain   string `json:"domain,omitempty"`
}

func readPanelSettings(db *sql.DB) (PanelSettings, []string, error) {
	exists, err := tableExists(db, "settings")
	if err != nil { return PanelSettings{}, nil, err }
	if !exists { return PanelSettings{}, nil, nil }
	rows, err := db.Query(`SELECT key,value FROM settings WHERE key IN ('webBasePath','webCertFile','webKeyFile','webDomain')`)
	if err != nil { return PanelSettings{}, nil, fmt.Errorf("read x-ui panel TLS settings: %w", err) }
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil { return PanelSettings{}, nil, err }
		values[key] = strings.TrimSpace(value)
	}
	if err := rows.Err(); err != nil { return PanelSettings{}, nil, err }
	out := PanelSettings{
		BasePath: normalizePanelBasePath(values["webBasePath"]),
		CertFile: strings.TrimSpace(values["webCertFile"]),
		KeyFile: strings.TrimSpace(values["webKeyFile"]),
		Domain: strings.TrimSpace(values["webDomain"]),
	}
	warnings := []string{}
	if (out.CertFile == "") != (out.KeyFile == "") {
		warnings = append(warnings, "X-Panel has only one of webCertFile/webKeyFile; panel TLS will not be enabled automatically")
		out.CertFile, out.KeyFile = "", ""
	}
	for _, p := range []struct{name, value string}{{"webCertFile", out.CertFile}, {"webKeyFile", out.KeyFile}} {
		if p.value == "" { continue }
		if !filepath.IsAbs(p.value) {
			warnings = append(warnings, fmt.Sprintf("X-Panel %s is not an absolute path: %q", p.name, p.value))
		}
	}
	return out, warnings, nil
}

func normalizePanelBasePath(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "/" { return "/" }
	v = "/" + strings.Trim(v, "/") + "/"
	if v == "//" { return "/" }
	return v
}
