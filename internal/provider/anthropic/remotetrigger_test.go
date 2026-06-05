package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/remote"
)

func newTestRemoteTriggerClient(t *testing.T, handler http.Handler) *RemoteTriggerClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewRemoteTriggerClient(
		server.Client(),
		server.URL,
		func() (string, error) { return "test-token", nil },
		func() (string, error) { return "test-org-uuid", nil },
	)
}

func TestRemoteTriggerClientActions(t *testing.T) {
	tests := []struct {
		name       string
		req        remote.Request
		wantMethod string
		wantPath   string
	}{
		{
			name:       "list",
			req:        remote.Request{Action: remote.ActionList},
			wantMethod: "GET",
			wantPath:   "/v1/code/triggers",
		},
		{
			name:       "get",
			req:        remote.Request{Action: remote.ActionGet, TriggerID: "tr-123"},
			wantMethod: "GET",
			wantPath:   "/v1/code/triggers/tr-123",
		},
		{
			name:       "create",
			req:        remote.Request{Action: remote.ActionCreate, Body: json.RawMessage(`{"name":"test"}`)},
			wantMethod: "POST",
			wantPath:   "/v1/code/triggers",
		},
		{
			name:       "update",
			req:        remote.Request{Action: remote.ActionUpdate, TriggerID: "tr-123", Body: json.RawMessage(`{"name":"updated"}`)},
			wantMethod: "POST",
			wantPath:   "/v1/code/triggers/tr-123",
		},
		{
			name:       "run",
			req:        remote.Request{Action: remote.ActionRun, TriggerID: "tr-123"},
			wantMethod: "POST",
			wantPath:   "/v1/code/triggers/tr-123/run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path

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

			client := newTestRemoteTriggerClient(t, handler)
			resp, err := client.Execute(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMethod != tt.wantMethod {
				t.Errorf("method: got %s, want %s", gotMethod, tt.wantMethod)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path: got %s, want %s", gotPath, tt.wantPath)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
			}
			if !strings.Contains(string(resp.Body), `"ok":true`) {
				t.Errorf("expected response body, got %s", resp.Body)
			}
		})
	}
}

func TestRemoteTriggerClientAuthError(t *testing.T) {
	client := NewRemoteTriggerClient(http.DefaultClient, "http://localhost",
		func() (string, error) { return "", context.Canceled },
		func() (string, error) { return "org", nil },
	)

	_, err := client.Execute(context.Background(), remote.Request{Action: remote.ActionList})
	if err == nil {
		t.Fatal("expected error from token source")
	}
}

func TestRemoteTriggerClientHTTPError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	})

	client := newTestRemoteTriggerClient(t, handler)
	resp, err := client.Execute(context.Background(), remote.Request{Action: remote.ActionList})
	if err != nil {
		t.Fatalf("HTTP errors should be returned as responses, not Go errors: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}
