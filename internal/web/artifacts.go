package web

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	path := filepath.Clean(r.URL.Query().Get("path"))
	if path == "." || path == "" {
		writeJSONAPIError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !s.sessionOwnsArtifact(path) {
		writeJSONAPIError(w, http.StatusForbidden, "artifact path is not recorded in this session")
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		writeJSONAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSONAPIResource(w, r, "artifacts", stableID(path), map[string]interface{}{
		"path":    path,
		"content": string(content),
	})
}

func (s *server) sessionOwnsArtifact(path string) bool {
	if s.cfg.Store == nil {
		return false
	}
	for _, artifact := range s.cfg.Store.Snapshot().OrchestrationArtifacts {
		if filepath.Clean(artifact.Path) == path {
			return true
		}
	}
	return false
}
