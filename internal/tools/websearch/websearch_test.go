package websearch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type staticState struct{ dir string }

func (s staticState) WorkDir() string { return s.dir }

func TestInvoke_ShortQuery(t *testing.T) {
	tl := &Tool{}
	input, _ := json.Marshal(WebSearchInput{Query: "a"})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "at least 2 characters") {
		t.Errorf("Content = %q, want short query error", result.Content)
	}
}

func TestInvoke_ConflictingFilters(t *testing.T) {
	tl := &Tool{}
	input, _ := json.Marshal(WebSearchInput{
		Query:          "test query",
		AllowedDomains: []string{"example.com"},
		BlockedDomains: []string{"bad.com"},
	})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Cannot specify both") {
		t.Errorf("Content = %q, want conflicting filters error", result.Content)
	}
}

func TestInvoke_MissingAPIKey(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "")
	tl := &Tool{}
	input, _ := json.Marshal(WebSearchInput{Query: "test query"})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "BRAVE_SEARCH_API_KEY") {
		t.Errorf("Content = %q, want API key error", result.Content)
	}
}

func TestFilterBlockedDomains(t *testing.T) {
	results := []braveResult{
		{Title: "Keep", URL: "https://example.com/page", Description: "keep this"},
		{Title: "Block exact", URL: "https://bad.com/page", Description: "block this"},
		{Title: "Block subdomain", URL: "https://sub.bad.com/page", Description: "block this too"},
		{Title: "Keep similar", URL: "https://notbad.com/page", Description: "keep this"},
		{Title: "Invalid URL", URL: "://broken", Description: "dropped"},
	}

	tests := []struct {
		name    string
		blocked []string
		want    int
	}{
		{"block exact", []string{"bad.com"}, 2},      // keeps example.com, notbad.com
		{"block all", []string{"bad.com", "example.com", "notbad.com"}, 0},
		{"block none", []string{"other.com"}, 4}, // keeps example.com, bad.com, sub.bad.com, notbad.com (invalid URL dropped by url.Parse — but "://broken" parses without error)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterBlockedDomains(results, tt.blocked)
			if len(got) != tt.want {
				names := make([]string, len(got))
				for i, r := range got {
					names[i] = r.Title
				}
				t.Errorf("filterBlockedDomains blocked=%v: got %d results %v, want %d",
					tt.blocked, len(got), names, tt.want)
			}
		})
	}
}

func TestFormatResults_Empty(t *testing.T) {
	result := formatResults("test query", nil, time.Second)
	if !strings.Contains(result, "No web search results found") {
		t.Errorf("formatResults(nil) = %q, want 'No web search results found'", result)
	}
}

func TestFormatResults_WithResults(t *testing.T) {
	results := []braveResult{
		{Title: "First Result", URL: "https://example.com", Description: "Description 1"},
		{Title: "Second Result", URL: "https://other.com", Description: ""},
	}
	output := formatResults("test", results, 500*time.Millisecond)

	if !strings.Contains(output, "1. First Result") {
		t.Error("missing numbered first result")
	}
	if !strings.Contains(output, "https://example.com") {
		t.Error("missing URL")
	}
	if !strings.Contains(output, "Description 1") {
		t.Error("missing description")
	}
	if !strings.Contains(output, "2. Second Result") {
		t.Error("missing numbered second result")
	}
	if !strings.Contains(output, "REMINDER") {
		t.Error("missing citation reminder")
	}
}
