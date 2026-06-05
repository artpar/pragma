package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/pragma/internal/hook"
)

func TestEndSessionLifecycleRunsHookWithFreshContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	pragmaDir := filepath.Join(workDir, ".pragma")
	if err := os.MkdirAll(pragmaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(pragmaDir, "session-end-marker")
	scriptPath := filepath.Join(workDir, "session-end.sh")
	script := "#!/bin/sh\nprintf ran > .pragma/session-end-marker\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := struct {
		Hooks map[string][]hook.Entry `json:"hooks"`
	}{
		Hooks: map[string][]hook.Entry{
			string(hook.SessionEnd): {
				{Hooks: []hook.Command{{Type: "command", Command: scriptPath}}},
			},
		},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pragmaDir, "settings.json"), settingsJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deps := &Deps{
		HookMgr:        hook.NewManager(workDir, "test-session", nil),
		SessionStarted: true,
	}

	endSessionLifecycle(ctx, deps)

	if deps.SessionStarted {
		t.Fatal("SessionStarted should be cleared")
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("SessionEnd hook did not run with cancelled caller context: %v", err)
	}
}
