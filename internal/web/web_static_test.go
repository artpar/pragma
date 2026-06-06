package web

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleStaticServesEmbeddedIndex(t *testing.T) {
	srv := newServer(Config{Bridge: NewBridge()})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleStatic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" && ct != "text/html" {
		t.Fatalf("content type: got %q, want html", ct)
	}
	if body := rec.Body.String(); body == "" || !strings.Contains(body, "Pragma Workbench") {
		t.Fatalf("body does not look like embedded app index: %q", body)
	}
}

func TestHandleStaticFallsBackForClientRoutes(t *testing.T) {
	srv := newServer(Config{Bridge: NewBridge()})
	req := httptest.NewRequest(http.MethodGet, "/sessions/session-1", nil)
	rec := httptest.NewRecorder()

	srv.handleStatic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Pragma Workbench") {
		t.Fatalf("fallback body does not look like app index: %q", body)
	}
}

func TestHandleStaticDoesNotShadowAPIRoutes(t *testing.T) {
	srv := newServer(Config{Bridge: NewBridge()})
	req := httptest.NewRequest(http.MethodGet, "/api/not-found", nil)
	rec := httptest.NewRecorder()

	srv.handleStatic(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != jsonAPIMediaType {
		t.Fatalf("content type: got %q, want %q", ct, jsonAPIMediaType)
	}
}

func TestConfigListenAddrDefaultsToRandomLocalhost(t *testing.T) {
	if got := (Config{}).listenAddr(); got != "127.0.0.1:0" {
		t.Fatalf("listen addr: got %q, want random localhost", got)
	}
}

func TestConfigListenAddrUsesExplicitWebAddr(t *testing.T) {
	cfg := Config{WebAddr: "127.0.0.1:4817"}
	if got := cfg.listenAddr(); got != "127.0.0.1:4817" {
		t.Fatalf("listen addr: got %q, want explicit address", got)
	}
}

func TestRunUsesExplicitWebAddr(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()

	err = Run(context.Background(), Config{Bridge: NewBridge(), WebAddr: ln.Addr().String()})
	if err == nil {
		t.Fatal("Run succeeded on an occupied explicit web address")
	}
}
