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
	indexHTML, _ := fs.ReadFile(sub, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		cleanPath := path
		if len(cleanPath) > 1 && cleanPath[0] == '/' {
			cleanPath = cleanPath[1:]
		}
		f, err := fs.Stat(sub, cleanPath)
		if err == nil && !f.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html directly to avoid FileServer 301 on non-file paths
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(indexHTML)
	}), nil
}
