package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the filesystem for web static files (SPA).
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// SPAHTMLMiddleware returns a middleware that serves index.html for GET requests to SPA routes
// when the client accepts HTML (browser navigation/refresh). This prevents /instances/6 etc.
// from returning API JSON instead of the SPA.
func SPAHTMLMiddleware() (func(http.Handler) http.Handler, error) {
	sub, err := FS()
	if err != nil {
		return nil, err
	}
	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}
			path := r.URL.Path
			accept := r.Header.Get("Accept")
			if !strings.Contains(accept, "text/html") {
				next.ServeHTTP(w, r)
				return
			}
			// SPA routes that overlap with API: /instances/X (exact, no /ws, /messages, etc.)
			if strings.HasPrefix(path, "/instances/") {
				rest := strings.TrimPrefix(path, "/instances/")
				if rest != "" && !strings.Contains(rest, "/") {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.WriteHeader(http.StatusOK)
					w.Write(indexHTML)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}, nil
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
