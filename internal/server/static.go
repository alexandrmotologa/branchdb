package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var embeddedDist embed.FS

func (s *Server) handleStaticAssets(w http.ResponseWriter, r *http.Request) {
	// Try embedded static assets from web/dist
	sub, err := fs.Sub(embeddedDist, "dist")
	if err == nil {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, statErr := fs.Stat(sub, path); statErr == nil {
			http.FileServer(http.FS(sub)).ServeHTTP(w, r)
			return
		}
		// If single-page app route, fallback to index.html
		if indexData, indexErr := fs.ReadFile(sub, "index.html"); indexErr == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(indexData)
			return
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>BranchDB Studio</title></head><body><h1>BranchDB Studio</h1><p>Active</p></body></html>"))
}
