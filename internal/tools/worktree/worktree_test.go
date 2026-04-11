package worktree

import (
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "my-branch", false, ""},
		{"valid with dots", "v1.2.3", false, ""},
		{"valid with underscore", "feature_branch", false, ""},
		{"valid with slash", "user/feature", false, ""},
		{"valid max length", strings.Repeat("a", 64), false, ""},
		{"empty", "", true, "slug is empty"},
		{"too long", strings.Repeat("a", 65), true, "exceeds"},
		{"absolute path", "/etc/passwd", true, "absolute path"},
		{"dot-dot traversal", "foo/../bar", true, "'..' traversal"},
		{"empty segment", "foo//bar", true, "empty segment"},
		{"invalid chars", "foo bar", true, "invalid characters"},
		{"special chars", "foo@bar", true, "invalid characters"},
		{"just dot-dot", "..", true, "'..' traversal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSlug(tt.slug)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected %q in error, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFlattenSlug(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"user/feature", "user+feature"},
		{"a/b/c", "a+b+c"},
		{"no-slash", "no-slash"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := FlattenSlug(tt.input)
			if got != tt.expect {
				t.Fatalf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}

func TestEnterTool_Name(t *testing.T) {
	tl := &EnterTool{}
	if tl.Name() != "EnterWorktree" {
		t.Fatalf("expected EnterWorktree, got %s", tl.Name())
	}
}

func TestExitTool_Name(t *testing.T) {
	tl := &ExitTool{}
	if tl.Name() != "ExitWorktree" {
		t.Fatalf("expected ExitWorktree, got %s", tl.Name())
	}
}

func TestEnterTool_Flags(t *testing.T) {
	tl := &EnterTool{}
	flags := tl.Flags()
	if flags.ReadOnly {
		t.Fatal("EnterWorktree should not be ReadOnly")
	}
	if flags.Concurrent {
		t.Fatal("EnterWorktree should not be Concurrent")
	}
}

func TestExitTool_Flags(t *testing.T) {
	tl := &ExitTool{}
	flags := tl.Flags()
	if flags.ReadOnly {
		t.Fatal("ExitWorktree should not be ReadOnly")
	}
	if flags.Concurrent {
		t.Fatal("ExitWorktree should not be Concurrent")
	}
}
