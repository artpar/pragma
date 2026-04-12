package toollsp

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/lsp"
	"github.com/artpar/gogent/internal/observe"
)

const (
	gitIgnoreBatchSize = 50
	gitIgnoreTimeout   = 5 * time.Second
)

// filterGitIgnored removes locations with URIs pointing to gitignored files.
// Runs `git check-ignore` in batches of 50 paths with a 5s timeout.
func filterGitIgnored(ctx context.Context, cwd string, uris []string) []string {
	observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "enter")
	defer observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "exit")
	if len(uris) == 0 {
		observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "if: len(uris) == 0")
		observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "return: uris")
		observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "return: uris")
		return uris
	}

	pathSet := make(map[string]bool)
	for _, uri := range uris {
		observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "range uris")
		path := lsp.URIToPath(uri)
		pathSet[path] = false
	}

	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "range pathSet")
		paths = append(paths, p)
	}

	for i := 0; i < len(paths); i += gitIgnoreBatchSize {
		observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "for: i < len(paths)")
		end := i + gitIgnoreBatchSize
		if end > len(paths) {
			observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "if: end > len(paths)")
			end = len(paths)
		}
		batch := paths[i:end]

		checkCtx, cancel := context.WithTimeout(ctx, gitIgnoreTimeout)
		args := append([]string{"check-ignore"}, batch...)
		cmd := exec.CommandContext(checkCtx, "git", args...)
		cmd.Dir = cwd

		out, err := cmd.Output()
		cancel()

		if err != nil {
			observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "if: err != nil")
			continue
		}

		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "range strings.Split(strings.TrimSpace(string(out)), \"\\n\")")
			line = strings.TrimSpace(line)
			if line != "" {
				observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "if: line != \"\"")
				pathSet[line] = true
			}
		}
	}

	// Filter out ignored URIs
	var kept []string
	for _, uri := range uris {
		observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "range uris")
		path := lsp.URIToPath(uri)
		if !pathSet[path] {
			observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "if: !pathSet[path]")
			kept = append(kept, uri)
		}
	}
	observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "return: kept")
	observe.TraceCtx(ctx, "toollsp", "filterGitIgnored", "return: kept")
	return kept
}
