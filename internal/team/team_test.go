package team

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/observe"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Team", "my-team"},
		{"Hello World!", "hello-world"},
		{"UPPERCASE", "uppercase"},
		{"already-good", "already-good"},
		{"  spaces  ", "spaces"},
		{"multiple---hyphens", "multiple-hyphens"},
		{"special@chars#here", "special-chars-here"},
		{"", "unnamed"},
		{"---", "unnamed"},
		{"café", "caf"},
		{"123numbers", "123numbers"},
		{"a", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatAgentID(t *testing.T) {
	tests := []struct {
		name     string
		teamName string
		want     string
	}{
		{"team-lead", "My Team", "team-lead@my-team"},
		{"worker", "test", "worker@test"},
		{"agent", "Multi Word Name", "agent@multi-word-name"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"@"+tt.teamName, func(t *testing.T) {
			got := FormatAgentID(tt.name, tt.teamName)
			if got != tt.want {
				t.Errorf("FormatAgentID(%q, %q) = %q, want %q", tt.name, tt.teamName, got, tt.want)
			}
		})
	}
}

func TestGenerateWordSlug(t *testing.T) {
	slug := GenerateWordSlug()
	if slug == "" {
		t.Fatal("GenerateWordSlug() returned empty string")
	}

	parts := strings.Split(slug, "-")
	if len(parts) != 3 {
		t.Errorf("GenerateWordSlug() = %q, want 3 hyphen-separated words, got %d parts", slug, len(parts))
	}

	// Two calls should differ (crypto/rand makes collision astronomically unlikely)
	slug2 := GenerateWordSlug()
	if slug == slug2 {
		t.Errorf("GenerateWordSlug() returned same slug twice: %q", slug)
	}
}

func TestTeamFileRoundTrip(t *testing.T) {
	active := true
	original := TeamFile{
		Name:          "test-team",
		Description:   "A test team",
		CreatedAt:     1700000000000,
		LeadAgentID:   "team-lead@test-team",
		LeadSessionID: "session-123",
		Members: []TeamMember{
			{
				AgentID:       "team-lead@test-team",
				Name:          "team-lead",
				AgentType:     "researcher",
				Model:         "claude-opus-4-6",
				JoinedAt:      1700000000000,
				CWD:           "/home/user/project",
				WorktreePath:  "/tmp/worktree",
				IsActive:      &active,
				Subscriptions: []string{"task-complete", "error"},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var roundTripped TeamFile
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Re-marshal and compare bytes for stability
	data2, err := json.Marshal(roundTripped)
	if err != nil {
		t.Fatalf("Marshal roundtripped: %v", err)
	}
	if string(data) != string(data2) {
		t.Errorf("JSON round-trip not stable:\n  got:  %s\n  want: %s", data2, data)
	}
}

func TestWriteAndReadTeamFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	tf := &TeamFile{
		Name:        "write-test",
		CreatedAt:   1700000000000,
		LeadAgentID: "team-lead@write-test",
		Members: []TeamMember{
			{
				AgentID:       "team-lead@write-test",
				Name:          "team-lead",
				JoinedAt:      1700000000000,
				CWD:           "/tmp",
				Subscriptions: []string{},
			},
		},
	}

	if err := WriteTeamFile("write-test", tf); err != nil {
		t.Fatalf("WriteTeamFile: %v", err)
	}

	// Verify file exists
	path := TeamFilePath("write-test")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("team file not created at %s: %v", path, err)
	}

	// Read back
	got, err := ReadTeamFile("write-test")
	if err != nil {
		t.Fatalf("ReadTeamFile: %v", err)
	}
	if got == nil {
		t.Fatal("ReadTeamFile returned nil")
	}
	if got.Name != tf.Name {
		t.Errorf("Name = %q, want %q", got.Name, tf.Name)
	}
	if got.LeadAgentID != tf.LeadAgentID {
		t.Errorf("LeadAgentID = %q, want %q", got.LeadAgentID, tf.LeadAgentID)
	}
	if len(got.Members) != 1 {
		t.Errorf("Members count = %d, want 1", len(got.Members))
	}
}

func TestReadTeamFile_NotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	tf, err := ReadTeamFile("nonexistent-team")
	if err != nil {
		t.Fatalf("ReadTeamFile: unexpected error: %v", err)
	}
	if tf != nil {
		t.Errorf("ReadTeamFile for nonexistent team returned non-nil: %+v", tf)
	}
}

func TestTeamExists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if TeamExists("noexist") {
		t.Error("TeamExists returned true for nonexistent team")
	}

	tf := &TeamFile{Name: "exists-test", CreatedAt: 1, LeadAgentID: "lead@exists-test", Members: []TeamMember{}}
	if err := WriteTeamFile("exists-test", tf); err != nil {
		t.Fatalf("WriteTeamFile: %v", err)
	}
	if !TeamExists("exists-test") {
		t.Error("TeamExists returned false for existing team")
	}
}

func TestTeamDir_TasksDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	teamDir := TeamDir("My Team")
	if !strings.Contains(teamDir, filepath.Join(".pragma", "teams", "my-team")) {
		t.Errorf("TeamDir = %q, expected to contain teams/my-team", teamDir)
	}

	tasksDir := TasksDir("My Team")
	if !strings.Contains(tasksDir, filepath.Join(".pragma", "tasks", "my-team")) {
		t.Errorf("TasksDir = %q, expected to contain tasks/my-team", tasksDir)
	}
}

func TestCleanupTeamDirectories(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create team file and tasks dir
	tf := &TeamFile{Name: "cleanup-test", CreatedAt: 1, LeadAgentID: "lead@cleanup-test", Members: []TeamMember{}}
	if err := WriteTeamFile("cleanup-test", tf); err != nil {
		t.Fatalf("WriteTeamFile: %v", err)
	}
	tasksDir := TasksDir("cleanup-test")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("MkdirAll tasks: %v", err)
	}

	// Verify dirs exist
	if _, err := os.Stat(TeamDir("cleanup-test")); err != nil {
		t.Fatalf("team dir not created: %v", err)
	}

	bus := observe.NewEventBus(100)

	if err := CleanupTeamDirectories("cleanup-test", bus); err != nil {
		t.Fatalf("CleanupTeamDirectories: %v", err)
	}

	// Verify dirs removed
	if _, err := os.Stat(TeamDir("cleanup-test")); !os.IsNotExist(err) {
		t.Error("team dir still exists after cleanup")
	}
	if _, err := os.Stat(tasksDir); !os.IsNotExist(err) {
		t.Error("tasks dir still exists after cleanup")
	}
}
