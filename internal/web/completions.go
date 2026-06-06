package web

import (
	"fmt"
	"net/http"

	"github.com/artpar/pragma/internal/slash"
)

type completionItem struct {
	Label       string `json:"label"`
	Detail      string `json:"detail,omitempty"`
	Replacement string `json:"replacement"`
}

func (s *server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	items := s.completionItems(r.URL.Query().Get("q"))
	if len(items) > 12 {
		items = items[:12]
	}
	page, err := pageRequestFromQuery(r)
	if err != nil {
		writeJSONAPIRequestError(w, err)
		return
	}
	pagedItems, pageMeta := paginateSlice(items, page)
	resources := make([]jsonAPIResource, 0, len(pagedItems))
	for i, item := range pagedItems {
		resources = append(resources, jsonAPIResource{
			Type:       "completions",
			ID:         fmt.Sprintf("%d", pageMeta.Start+i+1),
			Attributes: item,
		})
	}
	writeJSONAPICollection(w, r, resources, pageMeta)
}

func (s *server) completionItems(value string) []completionItem {
	var commands []slash.Command
	if s.cfg.SlashCmds == nil {
		commands = nil
	} else {
		commands = s.cfg.SlashCmds.CommandsWithDeps(s.cfg.SlashDeps)
	}
	workspace := s.cfg.Workspace
	if s.cfg.Store != nil {
		if cwd := s.cfg.Store.Snapshot().CWD; cwd != "" {
			workspace = cwd
		}
	}
	items := slash.CompletionItems(value, commands, workspace)
	out := make([]completionItem, 0, len(items))
	for _, item := range items {
		out = append(out, completionItem{
			Label:       item.Label,
			Detail:      item.Detail,
			Replacement: item.Replacement,
		})
	}
	return out
}
