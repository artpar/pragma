package toolremote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/permission"
)

type testState struct{ cwd string }

func (s testState) WorkDir() string { return s.cwd }

type alwaysAllow struct{}

func (alwaysAllow) Check(_ context.Context, _, _ string) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionAllow}
}

func newTestToolWithCleanup(t *testing.T, handler http.Handler) *Tool {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Tool{
		HTTPClient:  server.Client(),
		BaseURL:     server.URL,
		TokenSource: func() (string, error) { return "test-token", nil },
		OrgUUID:     func() (string, error) { return "test-org-uuid", nil },
	}
}

func TestRemoteTriggerActions(t *testing.T) {
	tests := []struct {
		name       string
		input      remoteInput
		wantMethod string
		wantPath   string
		wantErr    bool
	}{
		{
			name:       "list",
			input:      remoteInput{Action: "list"},
			wantMethod: "GET",
			wantPath:   "/v1/code/triggers",
		},
		{
			name:       "get",
			input:      remoteInput{Action: "get", TriggerID: "tr-123"},
			wantMethod: "GET",
			wantPath:   "/v1/code/triggers/tr-123",
		},
		{
			name:       "create",
			input:      remoteInput{Action: "create", Body: json.RawMessage(`{"name":"test"}`)},
			wantMethod: "POST",
			wantPath:   "/v1/code/triggers",
		},
		{
			name:       "update",
			input:      remoteInput{Action: "update", TriggerID: "tr-123", Body: json.RawMessage(`{"name":"updated"}`)},
			wantMethod: "POST",
			wantPath:   "/v1/code/triggers/tr-123",
		},
		{
			name:       "run",
			input:      remoteInput{Action: "run", TriggerID: "tr-123"},
			wantMethod: "POST",
			wantPath:   "/v1/code/triggers/tr-123/run",
		},
		{
			name:    "get missing trigger_id",
			input:   remoteInput{Action: "get"},
			wantErr: true,
		},
		{
			name:    "create missing body",
			input:   remoteInput{Action: "create"},
			wantErr: true,
		},
		{
			name:    "unknown action",
			input:   remoteInput{Action: "delete"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path

				// Verify headers
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("missing/wrong Authorization header")
				}
				if r.Header.Get("anthropic-version") != "2023-06-01" {
					t.Errorf("missing/wrong anthropic-version header")
				}
				if r.Header.Get("anthropic-beta") != "ccr-triggers-2026-01-30" {
					t.Errorf("missing/wrong anthropic-beta header")
				}
				if r.Header.Get("x-organization-uuid") != "test-org-uuid" {
					t.Errorf("missing/wrong x-organization-uuid header")
				}

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok":true}`))
			})

			tl := newTestToolWithCleanup(t, handler)
			inputJSON, _ := json.Marshal(tt.input)

			result, err := tl.Invoke(context.Background(), inputJSON, testState{cwd: "/tmp"})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotMethod != tt.wantMethod {
				t.Errorf("method: got %s, want %s", gotMethod, tt.wantMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path: got %s, want %s", gotPath, tt.wantPath)
			}
			if !strings.Contains(result.Content, "HTTP 200") {
				t.Errorf("expected HTTP 200 in result, got: %s", result.Content)
			}
		})
	}
}

func TestRemoteTriggerAuthError(t *testing.T) {
	tl := &Tool{
		HTTPClient:  http.DefaultClient,
		BaseURL:     "http://localhost",
		TokenSource: func() (string, error) { return "", context.Canceled },
		OrgUUID:     func() (string, error) { return "org", nil },
	}

	input, _ := json.Marshal(remoteInput{Action: "list"})
	_, err := tl.Invoke(context.Background(), input, testState{cwd: "/tmp"})
	if err == nil {
		t.Fatal("expected error from token source")
	}
}

func TestRemoteTriggerHTTPError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	})

	tl := newTestToolWithCleanup(t, handler)
	input, _ := json.Marshal(remoteInput{Action: "list"})

	result, err := tl.Invoke(context.Background(), input, testState{cwd: "/tmp"})
	if err != nil {
		t.Fatalf("HTTP errors should be returned as content, not Go errors: %v", err)
	}
	if !strings.Contains(result.Content, "HTTP 500") {
		t.Errorf("expected HTTP 500 in result, got: %s", result.Content)
	}
}
