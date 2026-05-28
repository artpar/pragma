package rawcapture

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransportCapturesRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization header = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"stream":false,"x":1}` {
			t.Fatalf("request body = %q", string(body))
		}
		w.Header().Set("X-Test", "ok")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	client := &http.Client{Transport: NewTransport(dir, http.DefaultTransport)}
	req, err := http.NewRequest("POST", server.URL+"/chat/completions", strings.NewReader(`{"stream":false,"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(respBody) != `{"ok":true}` {
		t.Fatalf("response body = %q", string(respBody))
	}

	capture := onlyCaptureDir(t, dir)
	if got := readFile(t, filepath.Join(capture, "request.json")); got != `{"stream":false,"x":1}` {
		t.Fatalf("captured request = %q", got)
	}
	if got := readFile(t, filepath.Join(capture, "response.raw")); got != `{"ok":true}` {
		t.Fatalf("captured response = %q", got)
	}

	var headers map[string][]string
	readJSON(t, filepath.Join(capture, "request.headers.json"), &headers)
	if got := headers["Authorization"]; len(got) != 1 || got[0] != "<redacted>" {
		t.Fatalf("captured Authorization = %#v", got)
	}

	var meta responseMeta
	readJSON(t, filepath.Join(capture, "response.meta.json"), &meta)
	if meta.StatusCode != 200 || meta.ResponseBytes != int64(len(`{"ok":true}`)) || meta.ResponseSHA256 == "" {
		t.Fatalf("response meta = %+v", meta)
	}
}

func TestTransportCapturesStreamingBodyAsRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: one\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	dir := t.TempDir()
	client := &http.Client{Transport: NewTransport(dir, http.DefaultTransport)}
	resp, err := client.Post(server.URL, "application/json", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	capture := onlyCaptureDir(t, dir)
	if got := readFile(t, filepath.Join(capture, "response.raw")); got != string(body) {
		t.Fatalf("stream capture = %q, want %q", got, string(body))
	}
	var reqMeta requestMeta
	readJSON(t, filepath.Join(capture, "request.meta.json"), &reqMeta)
	if reqMeta.Stream == nil || !*reqMeta.Stream {
		t.Fatalf("request stream metadata = %+v", reqMeta.Stream)
	}
}

func onlyCaptureDir(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("capture entries = %d, want 1", len(entries))
	}
	return filepath.Join(dir, entries[0].Name())
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
