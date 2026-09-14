package server

import (
	"net/http"
	"strings"

	"github.com/Kerberos255/X-port/internal/panelpath"
)

// MountBasePath keeps an imported X-Panel/3x-ui WebBasePath working as an
// entry URL while retaining the root asset/API endpoints used by the embedded
// dependency-free UI. The base path is a compatibility route, not a security
// boundary; authentication still protects every API operation.
func MountBasePath(next http.Handler, basePath string) http.Handler {
	basePath = panelpath.Normalize(basePath)
	if basePath == "/" {
		return next
	}
	bare := strings.TrimSuffix(basePath, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == bare {
			http.Redirect(w, r, basePath, http.StatusTemporaryRedirect)
			return
		}
		if strings.HasPrefix(r.URL.Path, basePath) {
			clone := r.Clone(r.Context())
			urlCopy := *r.URL
			clone.URL = &urlCopy
			clone.URL.Path = "/" + strings.TrimPrefix(r.URL.Path, basePath)
			if clone.URL.Path == "//" {
				clone.URL.Path = "/"
			}
			next.ServeHTTP(w, clone)
			return
		}
		next.ServeHTTP(w, r)
	})
}
