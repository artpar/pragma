package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

func (s *server) handlePermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/permission/")
	raw, err := decodeJSONAPIAttributes(r, "permission-responses")
	if err != nil {
		writeJSONAPIRequestError(w, err)
		return
	}
	var decisionValue string
	if err := json.Unmarshal(raw["decision"], &decisionValue); err != nil {
		writeJSONAPIError(w, http.StatusBadRequest, "decision is required")
		return
	}
	scope := permission.RememberNone
	if rawScope, ok := raw["scope"]; ok {
		var scopeValue string
		if err := json.Unmarshal(rawScope, &scopeValue); err != nil {
			writeJSONAPIError(w, http.StatusBadRequest, "scope must be a string")
			return
		}
		scope = permission.RememberScope(scopeValue)
		if scope != permission.RememberNone && scope != permission.RememberSession && scope != permission.RememberPersistent {
			writeJSONAPIError(w, http.StatusBadRequest, "scope must be none, session, or persistent")
			return
		}
	}
	if rawRemember, ok := raw["remember"]; ok {
		var remember bool
		_ = json.Unmarshal(rawRemember, &remember)
		if remember {
			scope = permission.RememberSession
		}
	}
	decision := permission.Decision(decisionValue)
	if decision != permission.DecisionAllow && decision != permission.DecisionDeny {
		writeJSONAPIError(w, http.StatusBadRequest, "decision must be allow or deny")
		return
	}
	submitted := map[string]interface{}{"id": id, "body": raw}
	if !s.cfg.Bridge.resolvePermission(id, permissionResponse{Decision: decision, Scope: scope}) {
		writeJSONAPIError(w, http.StatusNotFound, "permission request not found")
		return
	}
	s.hub.publish("permission_response", submitted)
	writeJSONAPIOK(w, r, "permission-responses", id, map[string]interface{}{"accepted": true, "request_id": id})
}

func (s *server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/ask/")
	raw, err := decodeJSONAPIAttributes(r, "ask-responses")
	if err != nil {
		writeJSONAPIRequestError(w, err)
		return
	}
	var answers map[string]string
	if err := json.Unmarshal(raw["answers"], &answers); err != nil {
		writeJSONAPIError(w, http.StatusBadRequest, "answers are required")
		return
	}
	submitted := map[string]interface{}{"id": id, "body": raw}
	if !s.cfg.Bridge.resolveAsk(id, tool.AskResponse{Answers: answers}) {
		writeJSONAPIError(w, http.StatusNotFound, "ask request not found")
		return
	}
	s.hub.publish("ask_response", submitted)
	writeJSONAPIOK(w, r, "ask-responses", id, map[string]interface{}{"accepted": true, "request_id": id})
}
