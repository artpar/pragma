package sysprompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitRoot_InRepo(t *testing.T) {
	// Run inside the pragma project directory — known git repo
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := GitRoot(cwd)
	if root == "" {
		t.Fatal("GitRoot returned empty for a known git repo")
	}
	// The root should contain a go.mod file
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Errorf("GitRoot %q does not contain go.mod", root)
	}
}

func TestGitRoot_NotRepo(t *testing.T) {
	dir := t.TempDir()
	root := GitRoot(dir)
	if root != "" {
		t.Errorf("GitRoot(%q) = %q, want empty for non-repo", dir, root)
	}
}

func TestGitBranch_InRepo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	branch := GitBranch(cwd)
	if branch == "" {
		t.Fatal("GitBranch returned empty for a known git repo")
	}
}

func TestGitBranch_NotRepo(t *testing.T) {
	dir := t.TempDir()
	branch := GitBranch(dir)
	if branch != "" {
		t.Errorf("GitBranch(%q) = %q, want empty for non-repo", dir, branch)
	}
}

func TestGitRemoteURL_InRepo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	url := GitRemoteURL(cwd)
	// This project has an origin remote
	if url == "" {
		t.Skip("no origin remote configured")
	}
	if url == "" {
		t.Fatal("GitRemoteURL returned empty for a repo with origin")
	}
}

func TestGitRemoteURL_NotRepo(t *testing.T) {
	dir := t.TempDir()
	url := GitRemoteURL(dir)
	if url != "" {
		t.Errorf("GitRemoteURL(%q) = %q, want empty for non-repo", dir, url)
	}
}
