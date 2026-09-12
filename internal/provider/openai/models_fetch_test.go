package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchModelsListsOpenAICompatibleCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("request path = %q, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization header = %q, want Bearer test-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"whisper-1","object":"model"},{"id":"gpt-4o","object":"model"}]}`)
	}))
	defer server.Close()

	models, err := FetchModels(context.Background(), "test-key", server.URL)
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	want := []string{"gpt-4o", "whisper-1"}
	if len(models) != len(want) {
		t.Fatalf("models = %v, want %v", models, want)
	}
	for i, id := range want {
		if models[i] != id {
			t.Errorf("models[%d] = %q, want %q", i, models[i], id)
		}
	}
}

func TestFetchModelsRequiresBaseURL(t *testing.T) {
	if _, err := FetchModels(context.Background(), "key", ""); err == nil {
		t.Fatal("expected error when base URL is empty")
	}
}
