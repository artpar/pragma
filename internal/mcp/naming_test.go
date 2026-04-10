package mcp

import "testing"

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with spaces", "with_spaces"},
		{"with.dots.and-dashes", "with_dots_and-dashes"},
		{"UPPER", "UPPER"},
		{"special!@#chars", "special_chars"},
		{"double__underscore", "double_underscore"},
		{"triple___under", "triple_under"},
		{"mix of  many   things!", "mix_of_many_things_"},
		{"already_valid-name", "already_valid-name"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildToolName(t *testing.T) {
	tests := []struct {
		server string
		tool   string
		want   string
	}{
		{"github", "create_issue", "mcp__github__create_issue"},
		{"my server", "my tool", "mcp__my_server__my_tool"},
		{"db.prod", "query", "mcp__db_prod__query"},
	}

	for _, tt := range tests {
		t.Run(tt.server+"/"+tt.tool, func(t *testing.T) {
			got := BuildToolName(tt.server, tt.tool)
			if got != tt.want {
				t.Errorf("BuildToolName(%q, %q) = %q, want %q", tt.server, tt.tool, got, tt.want)
			}
		})
	}
}

func TestParseToolName(t *testing.T) {
	tests := []struct {
		input      string
		wantServer string
		wantTool   string
		wantOk     bool
	}{
		{"mcp__github__create_issue", "github", "create_issue", true},
		{"mcp__my_server__my_tool", "my_server", "my_tool", true},
		{"mcp__s__t", "s", "t", true},
		{"mcp__server__tool__with__extras", "server", "tool__with__extras", true},
		{"notmcp__server__tool", "", "", false},
		{"mcp__", "", "", false},
		{"mcp__nodelim", "", "", false},
		{"Bash", "", "", false},
		{"", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			server, tool, ok := ParseToolName(tt.input)
			if ok != tt.wantOk || server != tt.wantServer || tool != tt.wantTool {
				t.Errorf("ParseToolName(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.input, server, tool, ok, tt.wantServer, tt.wantTool, tt.wantOk)
			}
		})
	}
}

func TestBuildParseRoundTrip(t *testing.T) {
	tests := []struct {
		server string
		tool   string
	}{
		{"github", "create_issue"},
		{"slack", "send_message"},
		{"db-prod", "query"},
	}

	for _, tt := range tests {
		t.Run(tt.server+"/"+tt.tool, func(t *testing.T) {
			full := BuildToolName(tt.server, tt.tool)
			server, tool, ok := ParseToolName(full)
			if !ok {
				t.Fatalf("ParseToolName(%q) returned ok=false", full)
			}
			wantServer := NormalizeName(tt.server)
			wantTool := NormalizeName(tt.tool)
			if server != wantServer || tool != wantTool {
				t.Errorf("round-trip failed: got (%q, %q), want (%q, %q)",
					server, tool, wantServer, wantTool)
			}
		})
	}
}

func TestIsMCPTool(t *testing.T) {
	if !IsMCPTool("mcp__server__tool") {
		t.Error("expected mcp__server__tool to be MCP tool")
	}
	if IsMCPTool("Bash") {
		t.Error("expected Bash to not be MCP tool")
	}
	if IsMCPTool("") {
		t.Error("expected empty string to not be MCP tool")
	}
}
