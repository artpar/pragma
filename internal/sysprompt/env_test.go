package sysprompt

import (
	"os"
	"strings"
	"testing"
)

func TestDetectEnv_InRepo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	info := DetectEnv(cwd, "claude-sonnet-4-20250514")

	if info.CWD != cwd {
		t.Errorf("CWD = %q, want %q", info.CWD, cwd)
	}
	if info.Platform == "" {
		t.Error("Platform is empty")
	}
	if !info.IsGitRepo {
		t.Error("IsGitRepo = false, expected true for project dir")
	}
	if info.GitBranch == "" {
		t.Error("GitBranch is empty for a git repo")
	}
	if info.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want claude-sonnet-4-20250514", info.Model)
	}
	if info.Date == "" {
		t.Error("Date is empty")
	}
}

func TestDetectEnv_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	info := DetectEnv(dir, "test-model")

	if info.IsGitRepo {
		t.Error("IsGitRepo = true for temp dir")
	}
	if info.GitRoot != "" {
		t.Errorf("GitRoot = %q, want empty", info.GitRoot)
	}
	if info.GitBranch != "" {
		t.Errorf("GitBranch = %q, want empty", info.GitBranch)
	}
}

func TestEnvBlock_Format(t *testing.T) {
	info := EnvInfo{
		CWD:       "/tmp/project",
		Platform:  "darwin",
		Shell:     "/bin/zsh",
		OSVersion: "Darwin 24.4.0",
		IsGitRepo: true,
		GitBranch: "main",
		Model:     "claude-sonnet-4",
		Date:      "2026-04-10",
	}

	block := envBlock(info)

	if block.Cacheable {
		t.Error("envBlock should not be cacheable")
	}

	checks := []string{
		"Working directory: /tmp/project",
		"Is git repo: yes (branch: main)",
		"Platform: darwin",
		"Shell: /bin/zsh",
		"OS: Darwin 24.4.0",
		"Model: claude-sonnet-4",
		"Date: 2026-04-10",
	}
	for _, want := range checks {
		if !strings.Contains(block.Text, want) {
			t.Errorf("envBlock missing %q\ngot: %s", want, block.Text)
		}
	}
}

func TestEnvBlock_NoGit(t *testing.T) {
	info := EnvInfo{
		CWD:       "/tmp/nongit",
		Platform:  "linux",
		IsGitRepo: false,
	}

	block := envBlock(info)
	if !strings.Contains(block.Text, "Is git repo: no") {
		t.Errorf("envBlock should say 'Is git repo: no'\ngot: %s", block.Text)
	}
}
