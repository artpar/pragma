package permission

import "testing"

func TestMatchShellContent(t *testing.T) {
	tests := []struct {
		pattern, command string
		want            bool
	}{
		// Exact match
		{"npm install", "npm install", true},
		{"npm install", "npm install foo", false},

		// Legacy prefix: "cmd:*"
		{"npm:*", "npm install", true},
		{"npm:*", "npm", true},
		{"npm:*", "npx something", false},
		{"git:*", "git push", true},
		{"git:*", "gitk", false},

		// Wildcard: "cmd *"
		{"npm *", "npm install", true},
		{"npm *", "npm install --save foo", true},
		{"npm *", "npm", true}, // trailing space optional
		{"git *", "git push origin main", true},
		{"git *", "git", true},
		{"git *", "gitk", false},

		// Wildcard mid-pattern
		{"docker * build", "docker compose build", true},
		{"docker * build", "docker something build", true},

		// Escaped asterisk
		{`echo \*`, "echo *", true},
		{`echo \*`, "echo foo", false},

		// No match
		{"rm -rf", "ls -la", false},
	}

	for _, tt := range tests {
		got := MatchShellContent(tt.pattern, tt.command)
		if got != tt.want {
			t.Errorf("MatchShellContent(%q, %q) = %v, want %v", tt.pattern, tt.command, got, tt.want)
		}
	}
}

func TestMatchPathContent(t *testing.T) {
	tests := []struct {
		pattern, path, workDir string
		want                   bool
	}{
		{"/src/**/*.go", "/project/src/main.go", "/project", true},
		{"/src/**/*.go", "/project/src/pkg/foo.go", "/project", true},
		{"/src/**/*.go", "/project/README.md", "/project", false},
		{"/.pragma/**", "/project/.pragma/settings.json", "/project", true},
		{"/.pragma/**", "/project/src/main.go", "/project", false},
	}

	for _, tt := range tests {
		got := MatchPathContent(tt.pattern, tt.path, tt.workDir)
		if got != tt.want {
			t.Errorf("MatchPathContent(%q, %q, %q) = %v, want %v", tt.pattern, tt.path, tt.workDir, got, tt.want)
		}
	}
}

func TestMatchDomainContent(t *testing.T) {
	tests := []struct {
		rule, content string
		want          bool
	}{
		{"domain:github.com", "domain:github.com", true},
		{"domain:GitHub.com", "domain:github.com", true}, // case insensitive
		{"domain:github.com", "domain:gitlab.com", false},
		{"domain:example.com", "something", false},
		{"notdomain", "domain:example.com", false},
	}

	for _, tt := range tests {
		got := MatchDomainContent(tt.rule, tt.content)
		if got != tt.want {
			t.Errorf("MatchDomainContent(%q, %q) = %v, want %v", tt.rule, tt.content, got, tt.want)
		}
	}
}
