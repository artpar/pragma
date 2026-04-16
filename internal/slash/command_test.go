package slash

import (
	"context"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs string
		wantOk   bool
	}{
		{"/compact", "compact", "", true},
		{"/compact custom instructions here", "compact", "custom instructions here", true},
		{"/clear", "clear", "", true},
		{"/exit", "exit", "", true},
		{"/help", "help", "", true},
		{"/cost", "cost", "", true},
		{"  /compact  ", "compact", "", true},
		{"/unknown", "unknown", "", true}, // Parse succeeds; Execute checks existence
		{"not a command", "", "", false},
		{"", "", "", false},
		{"/", "", "", false},       // bare slash
		{"/ ", "", "", false},      // slash + space only — TrimSpace reduces to bare slash
		{"hello /world", "", "", false}, // slash not at start
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, args, ok := Parse(tt.input)
			if ok != tt.wantOk {
				t.Errorf("Parse(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if name != tt.wantName {
				t.Errorf("Parse(%q) name = %q, want %q", tt.input, name, tt.wantName)
			}
			if args != tt.wantArgs {
				t.Errorf("Parse(%q) args = %q, want %q", tt.input, args, tt.wantArgs)
			}
		})
	}
}

func TestRegistryExecuteUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "nonexistent", "", Deps{})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error should mention unknown command, got: %v", err)
	}
}

func TestRegistryAliases(t *testing.T) {
	r := NewRegistry()

	// /reset is alias for /clear — need a store to avoid nil panic
	store := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "test", "test", "/tmp"),
	})
	result, err := r.Execute(context.Background(), "reset", "", Deps{Store: store})
	if err != nil {
		t.Fatalf("/reset returned error: %v", err)
	}
	if !result.ClearConversation {
		t.Error("/reset should set ClearConversation=true (alias for /clear)")
	}

	// /quit is alias for /exit
	result, err = r.Execute(context.Background(), "quit", "", Deps{})
	if err != nil {
		t.Fatalf("/quit returned error: %v", err)
	}
	if !result.Quit {
		t.Error("/quit should set Quit=true")
	}

	// /? is alias for /help
	result, err = r.Execute(context.Background(), "?", "", Deps{})
	if err != nil {
		t.Fatalf("/? returned error: %v", err)
	}
	if !strings.Contains(result.DisplayText, "Available commands") {
		t.Error("/? should show help text")
	}
}

func TestRegistryCaseInsensitive(t *testing.T) {
	r := NewRegistry()
	result, err := r.Execute(context.Background(), "HELP", "", Deps{})
	if err != nil {
		t.Fatalf("/HELP returned error: %v", err)
	}
	if !strings.Contains(result.DisplayText, "Available commands") {
		t.Error("/HELP should match /help")
	}
}

func TestRegistryCommands(t *testing.T) {
	r := NewRegistry()
	cmds := r.Commands()
	if len(cmds) < 5 {
		t.Errorf("expected at least 5 built-in commands, got %d", len(cmds))
	}

	// Should be sorted
	for i := 1; i < len(cmds); i++ {
		if cmds[i].Name < cmds[i-1].Name {
			t.Errorf("commands not sorted: %s before %s", cmds[i-1].Name, cmds[i].Name)
		}
	}
}
