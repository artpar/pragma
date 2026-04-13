package team

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

const (
	// TeamLeadName is the constant name for the team leader member.
	TeamLeadName = "team-lead"
	teamsSubdir  = "teams"
	tasksSubdir  = "tasks"
)

// TeamFile is the on-disk representation of a multi-agent team.
type TeamFile struct {
	Name          string       `json:"name"`
	Description   string       `json:"description,omitempty"`
	CreatedAt     int64        `json:"createdAt"`
	LeadAgentID   string       `json:"leadAgentId"`
	LeadSessionID string       `json:"leadSessionId,omitempty"`
	Members       []TeamMember `json:"members"`
}

// TeamMember represents a single member of a team.
type TeamMember struct {
	AgentID       string   `json:"agentId"`
	Name          string   `json:"name"`
	AgentType     string   `json:"agentType,omitempty"`
	Model         string   `json:"model,omitempty"`
	JoinedAt      int64    `json:"joinedAt"`
	CWD           string   `json:"cwd"`
	WorktreePath  string   `json:"worktreePath,omitempty"`
	IsActive      *bool    `json:"isActive,omitempty"`
	Subscriptions []string `json:"subscriptions"`
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// FormatAgentID creates a deterministic agent ID: "team-lead@my-team".
func FormatAgentID(name, teamName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: name + \"@\" + SanitizeName(teamName)")
	return name + "@" + SanitizeName(teamName)
}

// SanitizeName converts a string to a lowercase, hyphen-separated slug.
func SanitizeName(s string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lower := strings.ToLower(strings.TrimSpace(s))
	slug := nonAlphanumeric.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		observe.GlobalTrace("if: slug == \"\"")
		observe.GlobalTrace("return: \"unnamed\"")
		return "unnamed"
	}
	observe.GlobalTrace("return: slug")
	return slug
}

// gogentHome returns the gogent home dir, falling back to ~/.gogent on error.
func gogentHome() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	home, err := config.GogentHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")

		h, _ := os.UserHomeDir()
		observe.GlobalTrace("return: filepath.Join(h, \".gogent\")")
		return filepath.Join(h, ".gogent")
	}
	observe.GlobalTrace("return: home")
	return home
}

// TeamDir returns the directory path for a team: ~/.gogent/teams/{sanitized}/
func TeamDir(teamName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: filepath.Join(gogentHome(), teamsSubdir, SanitizeName(teamName))")
	return filepath.Join(gogentHome(), teamsSubdir, SanitizeName(teamName))
}

// TasksDir returns the tasks directory for a team: ~/.gogent/tasks/{sanitized}/
func TasksDir(teamName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: filepath.Join(gogentHome(), tasksSubdir, SanitizeName(teamName))")
	return filepath.Join(gogentHome(), tasksSubdir, SanitizeName(teamName))
}

// TeamFilePath returns the config file path for a team.
func TeamFilePath(teamName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: filepath.Join(TeamDir(teamName), \"config.json\")")
	return filepath.Join(TeamDir(teamName), "config.json")
}

// ReadTeamFile reads and parses a team's config.json. Returns nil, nil if not found.
func ReadTeamFile(teamName string) (*TeamFile, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(TeamFilePath(teamName))
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		if os.IsNotExist(err) {
			observe.GlobalTrace("if: os.IsNotExist(err)")
			observe.GlobalTrace("return: nil, nil")
			return nil, nil
		}
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read team file: %w\", err)")
		return nil, fmt.Errorf("read team file: %w", err)
	}
	var tf TeamFile
	if err := json.Unmarshal(data, &tf); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"parse team file: %w\", err)")
		return nil, fmt.Errorf("parse team file: %w", err)
	}
	observe.GlobalTrace("return: &tf, nil")
	return &tf, nil
}

// WriteTeamFile atomically writes a team config file (temp + rename).
// Issue #22239: prevents race conditions from concurrent writes.
func WriteTeamFile(teamName string, tf *TeamFile) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"marshal team file: %w\", err)")
		return fmt.Errorf("marshal team file: %w", err)
	}
	path := TeamFilePath(teamName)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"create team dir: %w\", err)")
		return fmt.Errorf("create team dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"write temp team file: %w\", err)")
		return fmt.Errorf("write temp team file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		observe.GlobalTrace("if: err != nil")
		os.Remove(tmp)
		observe.GlobalTrace("return: fmt.Errorf(\"rename team file: %w\", err)")
		return fmt.Errorf("rename team file: %w", err)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// CleanupTeamDirectories removes team directory, tasks directory, and any git worktrees.
func CleanupTeamDirectories(teamName string, bus *observe.EventBus) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	tf, _ := ReadTeamFile(teamName)
	if tf != nil {
		observe.GlobalTrace("if: tf != nil")
		for _, m := range tf.Members {
			observe.GlobalTrace("range tf.Members")
			if m.WorktreePath != "" {
				observe.GlobalTrace("if: m.WorktreePath != \"\"")

				cmd := exec.Command("git", "worktree", "remove", "--force", m.WorktreePath)
				if m.CWD != "" {
					observe.GlobalTrace("if: m.CWD != \"\"")
					cmd.Dir = m.CWD
				}
				_ = cmd.Run()
			}
		}
	}

	teamDir := TeamDir(teamName)
	if err := os.RemoveAll(teamDir); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"remove team dir %s: %w\", teamDir, err)")
		return fmt.Errorf("remove team dir %s: %w", teamDir, err)
	}

	tasksDir := TasksDir(teamName)
	if err := os.RemoveAll(tasksDir); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"remove tasks dir %s: %w\", tasksDir, err)")
		return fmt.Errorf("remove tasks dir %s: %w", tasksDir, err)
	}

	if bus != nil {
		observe.GlobalTrace("if: bus != nil")
		bus.Trace("team", "CleanupTeamDirectories", fmt.Sprintf("cleaned up team=%s", teamName))
	}
	observe.GlobalTrace("return: nil")

	return nil
}

// TeamExists checks if a team config file already exists on disk.
func TeamExists(teamName string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	_, err := os.Stat(TeamFilePath(teamName))
	observe.GlobalTrace("return: err == nil")
	return err == nil
}
