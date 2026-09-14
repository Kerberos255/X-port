package panelpath

import "strings"

// Normalize returns the canonical panel base path form used by the CLI,
// migration code and HTTP compatibility mount. Root is "/"; non-root paths
// always have exactly one leading and trailing slash.
func Normalize(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "/" {
		return "/"
	}
	return "/" + strings.Trim(v, "/") + "/"
}

// Valid reports whether v is a canonical, safe panel base path.
func Valid(v string) bool {
	if v == "/" {
		return true
	}
	if !strings.HasPrefix(v, "/") || !strings.HasSuffix(v, "/") {
		return false
	}
	if strings.ContainsAny(v, "?#\\\t\r\n ") {
		return false
	}
	for _, part := range strings.Split(strings.Trim(v, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
