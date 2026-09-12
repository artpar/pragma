package google

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchModelsKeepsChatCapableModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models") {
			t.Errorf("request path = %q, want suffix /models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[
			{"name":"models/gemini-2.5-pro","supportedGenerationMethods":["generateContent","countTokens"]},
			{"name":"models/gemini-2.5-flash","supportedGenerationMethods":["generateContent"]},
			{"name":"models/text-embedding-004","supportedGenerationMethods":["embedContent","countTokens"]},
			{"name":"models/imagen-3.0-generate-002","supportedGenerationMethods":["predict"]}
		]}`)
	}))
	defer server.Close()

	models, err := FetchModels(context.Background(), "test-key", server.URL)
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	want := []string{"gemini-2.5-flash", "gemini-2.5-pro"}
	if len(models) != len(want) {
		t.Fatalf("models = %v, want %v", models, want)
	}
	for i, id := range want {
		if models[i] != id {
			t.Errorf("models[%d] = %q, want %q", i, models[i], id)
		}
	}
	for _, id := range models {
		if strings.Contains(id, "models/") {
			t.Errorf("model id %q retains the models/ resource prefix", id)
		}
	}
}

func TestFetchModelsPropagatesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"API key not valid"}}`)
	}))
	defer server.Close()

	if _, err := FetchModels(context.Background(), "bad-key", server.URL); err == nil {
		t.Fatal("expected error for 403 response")
	}
}
