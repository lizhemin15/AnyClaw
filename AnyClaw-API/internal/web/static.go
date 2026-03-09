package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the filesystem for web static files (SPA).
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// SPAHandler serves static files and falls back to index.html for SPA routing.
func SPAHandler() (http.Handler, error) {
	sub, err := FS()
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		f, err := fs.Stat(sub, path[1:])
		if err == nil && !f.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback
		r.URL.Path = "/index.html"
		r.URL.RawPath = "/index.html"
		fileServer.ServeHTTP(w, r)
	}), nil
}
