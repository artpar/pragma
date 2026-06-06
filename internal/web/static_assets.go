package web

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed static
var staticFiles embed.FS

func (s *server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSONAPIError(w, http.StatusNotFound, "API route not found")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	assetPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if assetPath == "" {
		assetPath = "index.html"
	}
	if s.writeStaticAsset(w, r, assetPath) {
		return
	}
	if s.writeStaticAsset(w, r, "index.html") {
		return
	}
	http.NotFound(w, r)
}

func (s *server) writeStaticAsset(w http.ResponseWriter, r *http.Request, assetPath string) bool {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	data, err := fs.ReadFile(staticRoot, assetPath)
	if err != nil {
		return false
	}
	if contentType := mime.TypeByExtension(path.Ext(assetPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, assetPath, time.Time{}, bytes.NewReader(data))
	return true
}
