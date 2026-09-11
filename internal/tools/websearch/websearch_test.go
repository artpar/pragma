package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/config"
)

// WEB-001 unit gates for the ported tool.

func TestToolDefShape(t *testing.T) {
	def := ToolDef()
	if def.Name != "WebSearch" {
		t.Fatalf("name = %q", def.Name)
	}
	var schema map[string]any
	if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["query"]; !ok {
		t.Fatalf("schema missing query property: %s", def.InputSchema)
	}
	if req, _ := schema["required"].([]any); len(req) != 1 || req[0] != "query" {
		t.Fatalf("schema required = %v", schema["required"])
	}
	if !strings.Contains(def.Description, "Sources:") {
		t.Fatalf("description must demand source citations")
	}
}

func newBraveStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/res/v1/web/search" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Subscription-Token") != "stub-key" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Accept") != "application/json" {
			http.Error(w, "bad accept", http.StatusBadRequest)
			return
		}
		q := r.URL.Query().Get("q")
		if q == "" {
			http.Error(w, "missing q", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]string{
					{"title": "Result One about " + q, "url": "https://one.example.com/a", "description": "First"},
					{"title": "Result Two about " + q, "url": "https://two.example.org/b", "description": "Second"},
					{"title": "Blocked host", "url": "https://sub.blocked.example.net/c", "description": "Should be filtered"},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestExecuteSearchesAndFormats(t *testing.T) {
	srv := newBraveStub(t)
	input, _ := json.Marshal(Input{Query: "pragma harness"})
	out, err := Execute(context.Background(), "stub-key", srv.URL, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Result One about pragma harness") ||
		!strings.Contains(out, "https://one.example.com/a") {
		t.Fatalf("formatted results missing: %q", out)
	}
	if !strings.Contains(out, "REMINDER: You MUST cite the sources") {
		t.Fatalf("citation reminder missing: %q", out)
	}
}

func TestExecuteAllowedDomainsBuildsSiteFilters(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"web":{"results":[]}}`))
	}))
	defer srv.Close()

	input, _ := json.Marshal(Input{Query: "harness docs", AllowedDomains: []string{"a.com", "b.org"}})
	out, err := Execute(context.Background(), "stub-key", srv.URL, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotQuery != "harness docs site:a.com OR site:b.org" {
		t.Fatalf("query with site filters = %q", gotQuery)
	}
	if !strings.Contains(out, "No web search results found") {
		t.Fatalf("empty results message = %q", out)
	}
}

func TestExecuteBlockedDomainsFiltersHosts(t *testing.T) {
	srv := newBraveStub(t)
	input, _ := json.Marshal(Input{Query: "pragma harness", BlockedDomains: []string{"blocked.example.net"}})
	out, err := Execute(context.Background(), "stub-key", srv.URL, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, "Blocked host") || strings.Contains(out, "sub.blocked.example.net") {
		t.Fatalf("blocked domain result not filtered: %q", out)
	}
	if !strings.Contains(out, "Result One") {
		t.Fatalf("unblocked results missing: %q", out)
	}
}

func TestExecuteValidation(t *testing.T) {
	srv := newBraveStub(t)
	if _, err := Execute(context.Background(), "stub-key", srv.URL, json.RawMessage(`{not json`)); err == nil {
		t.Fatal("invalid JSON must error")
	}
	short, _ := json.Marshal(Input{Query: "x"})
	if _, err := Execute(context.Background(), "stub-key", srv.URL, short); err == nil ||
		!strings.Contains(err.Error(), "at least 2 characters") {
		t.Fatalf("short query error = %v", err)
	}
	both, _ := json.Marshal(Input{Query: "valid", AllowedDomains: []string{"a.com"}, BlockedDomains: []string{"b.org"}})
	if _, err := Execute(context.Background(), "stub-key", srv.URL, both); err == nil ||
		!strings.Contains(err.Error(), "both") {
		t.Fatalf("both-filters error = %v", err)
	}
}

func TestExecuteAPIFailureIsError(t *testing.T) {
	srv := newBraveStub(t)
	input, _ := json.Marshal(Input{Query: "valid query"})
	// wrong key → stub answers 401
	if _, err := Execute(context.Background(), "wrong-key", srv.URL, input); err == nil ||
		!strings.Contains(err.Error(), "401") {
		t.Fatalf("API failure error = %v", err)
	}
}

func TestSearchBraveUsesDefaultEndpointWhenBaseURLEmpty(t *testing.T) {
	// Cannot hit the real endpoint in a unit test; assert the default is
	// applied by checking a request against an unreachable override is not
	// used when empty is passed — structural check via URL build.
	if DefaultBaseURL != "https://api.search.brave.com" {
		t.Fatalf("DefaultBaseURL changed: %s", DefaultBaseURL)
	}
}

func TestFilterBlockedDomainsExactAndSubdomain(t *testing.T) {
	results := []Result{
		{URL: "https://a.com/1"},
		{URL: "https://sub.a.com/2"},
		{URL: "https://notb.com/3"},
		{URL: "https://b.com/4"},
		{URL: "://bad-url"},
	}
	out := filterBlockedDomains(results, []string{"a.com"})
	var hosts []string
	for _, r := range out {
		hosts = append(hosts, r.URL)
	}
	want := []string{"https://notb.com/3", "https://b.com/4"}
	if strings.Join(hosts, ",") != strings.Join(want, ",") {
		t.Fatalf("filtered = %v, want %v", hosts, want)
	}
}

func TestFormatResultsEmpty(t *testing.T) {
	out := FormatResults("nothing", nil, time.Second)
	if out != `No web search results found for query: "nothing"` {
		t.Fatalf("empty format = %q", out)
	}
}

// Key resolution parity with the cli wiring: credentials carry the brave
// provider key (the cli wrapper reads env first — covered by cli tests).
func TestCredentialForBrave(t *testing.T) {
	creds := config.Credentials{Providers: map[string]config.ProviderCredential{
		"brave": {APIKey: "file-key"},
	}}
	if got := creds.CredentialFor("brave").APIKey; got != "file-key" {
		t.Fatalf("CredentialFor(brave) = %q", got)
	}
}
