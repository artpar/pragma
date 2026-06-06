package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/session"
)

func (s *server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	store := s.cfg.SessionStore
	if store == nil {
		var err error
		store, err = session.NewStore()
		if err != nil {
			writeJSONAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	summaries, err := store.List()
	if err != nil {
		writeJSONAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if summaries == nil {
		summaries = []session.SessionSummary{}
	}
	rows, err := webSessionSummaries(summaries, s.cfg.Workspace)
	if err != nil {
		writeJSONAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	page, err := pageRequestFromQuery(r)
	if err != nil {
		writeJSONAPIRequestError(w, err)
		return
	}
	pagedRows, pageMeta := paginateSlice(rows, page)
	resources := make([]jsonAPIResource, 0, len(pagedRows))
	for _, row := range pagedRows {
		id, _ := row["id"].(string)
		resources = append(resources, jsonAPIResource{
			Type:       "sessions",
			ID:         id,
			Attributes: jsonAPIAttributes(row, "id"),
		})
	}
	writeJSONAPICollection(w, r, resources, pageMeta)
}

func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeJSONAPIError(w, http.StatusBadRequest, "session id is required")
		return
	}
	store := s.cfg.SessionStore
	if store == nil {
		var err error
		store, err = session.NewStore()
		if err != nil {
			writeJSONAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	sess, err := store.Load(sessionID)
	if err != nil {
		writeJSONAPIError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSONAPIResource(w, r, "sessions", sessionID, sess)
}

func webSessionSummaries(summaries []session.SessionSummary, workspace string) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(summaries))
	cwd := filepath.Clean(strings.TrimSpace(workspace))
	for _, summary := range summaries {
		var row map[string]interface{}
		data, err := json.Marshal(summary)
		if err != nil {
			return nil, fmt.Errorf("marshal session summary %q: %w", summary.ID, err)
		}
		if err := json.Unmarshal(data, &row); err != nil {
			return nil, fmt.Errorf("project session summary %q: %w", summary.ID, err)
		}
		if cwd != "" && filepath.Clean(summary.WorkDir) == cwd {
			row["in_current_work_dir"] = true
		} else {
			row["in_current_work_dir"] = false
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	raw, err := decodeJSONAPIAttributes(r, "session-resumes")
	if err != nil {
		writeJSONAPIRequestError(w, err)
		return
	}
	var sessionID string
	if err := json.Unmarshal(raw["session_id"], &sessionID); err != nil {
		writeJSONAPIError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	if err := s.resume(sessionID, raw); err != nil {
		if errors.Is(err, errRuntimeBusy) {
			writeJSONAPIError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSONAPIOK(w, r, "session-resumes", sessionID, map[string]interface{}{"accepted": true, "session_id": sessionID})
}

func (s *server) resume(sessionID string, raw map[string]json.RawMessage) error {
	if s.cfg.Resume == nil {
		return errors.New("session resume is not available")
	}
	endTransition, err := s.beginRuntimeTransition(nil)
	if err != nil {
		return err
	}
	defer endTransition()

	s.hub.publish("resume_requested", raw)
	if err := s.cfg.Resume(sessionID); err != nil {
		return err
	}
	store := s.cfg.SessionStore
	if store == nil {
		store, err = session.NewStore()
		if err != nil {
			return err
		}
	}
	sess, err := store.Load(sessionID)
	if err != nil {
		return err
	}
	s.hub.seed(sessionWebEvents(sess.WebEvents))
	s.hub.publish("session_resumed", sess)
	return nil
}
