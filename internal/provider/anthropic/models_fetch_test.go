package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchModelsPagesThroughResults(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/models" {
			t.Errorf("request path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key header = %q, want test-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "" {
			fmt.Fprint(w, `{"data":[{"id":"claude-b","type":"model","display_name":"B","created_at":"2025-01-01T00:00:00Z"}],"has_more":true,"last_id":"claude-b","first_id":"claude-b"}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"claude-a","type":"model","display_name":"A","created_at":"2025-01-01T00:00:00Z"}],"has_more":false,"last_id":"claude-a","first_id":"claude-a"}`)
	}))
	defer server.Close()

	models, err := FetchModels(context.Background(), "test-key", server.URL)
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 paginated requests, got %d", requests)
	}
	want := []string{"claude-a", "claude-b"}
	if len(models) != len(want) {
		t.Fatalf("models = %v, want %v", models, want)
	}
	for i, id := range want {
		if models[i] != id {
			t.Errorf("models[%d] = %q, want %q", i, models[i], id)
		}
	}
}

func TestFetchModelsPropagatesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer server.Close()

	if _, err := FetchModels(context.Background(), "bad-key", server.URL); err == nil {
		t.Fatal("expected error for 401 response")
	}
}
