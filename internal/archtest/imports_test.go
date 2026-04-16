package archtest

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNoDirectProviderImports(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	internalDir := filepath.Join(root, "internal")
	files := scanGoFiles(internalDir)

	forbidden := []string{
		"github.com/artpar/pragma/internal/provider/anthropic",
		"github.com/artpar/pragma/internal/provider/openai",
		"github.com/artpar/pragma/internal/provider/google",
		"github.com/artpar/pragma/internal/provider/groq",
	}

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		// Provider adapters and cli (wiring layer) are allowed
		if strings.Contains(rel, "internal/provider/anthropic") ||
			strings.Contains(rel, "internal/provider/openai") ||
			strings.Contains(rel, "internal/provider/google") ||
			strings.Contains(rel, "internal/provider/groq") ||
			strings.Contains(rel, "internal/cli/") {
			continue
		}

		imports, err := parseImports(file)
		if err != nil {
			continue
		}

		for _, imp := range imports {
			for _, f := range forbidden {
				if imp == f {
					t.Errorf("%s imports %s directly (only provider adapters and cli may do this)", rel, imp)
				}
			}
		}
	}
}

// importsPackage checks if importPath refers to pkg or a subpackage of pkg.
// pkg must not have a trailing slash (e.g., "internal/observe").
func importsPackage(importPath, pkg string) bool {
	return strings.HasSuffix(importPath, pkg) || strings.Contains(importPath, pkg+"/")
}

func TestPackageDependencyDAG(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	// Define illegal import edges per the DAG in AGENT.md
	// key = package path suffix (with trailing slash for file matching)
	// value = packages it must NOT import (no trailing slash — matched via importsPackage)
	illegal := map[string][]string{
		"internal/model/": {
			"internal/provider",
			"internal/observe",
			"internal/tool",
			"internal/app",
			"internal/permission",
			"internal/cli",
			"internal/tui",
			"internal/query",
		},
		"internal/permission/": {
			// Allowed: internal/observe (EventBus), internal/config (rule loading)
			"internal/model",
			"internal/provider",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
		},
		"internal/app/": {
			"internal/tool",
			"internal/provider",
			"internal/observe",
			"internal/cli",
			"internal/tui",
			"internal/query",
		},
		"internal/util/": {
			"internal/model",
			"internal/provider",
			"internal/observe",
			"internal/tool",
			"internal/app",
		},
		"internal/sysprompt/": {
			"internal/provider",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/session",
			"internal/permission",
		},
		"internal/session/": {
			"internal/provider",
			"internal/observe",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/sysprompt",
			"internal/permission",
		},
		"internal/tui/": {
			"internal/provider",
			"internal/sysprompt",
			"internal/cli",
			"internal/session",
		},
		"internal/mcp/": {
			"internal/provider",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/session",
			"internal/sysprompt",
		},
		"internal/lsp/": {
			"internal/provider",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/session",
			"internal/sysprompt",
			"internal/tool",
			"internal/permission",
			"internal/mcp",
		},
		"internal/compact/": {
			"internal/tui",
			"internal/cli",
			"internal/session",
			"internal/tool",
			"internal/query",
			"internal/permission",
			"internal/slash",
			"internal/mcp",
			"internal/sysprompt",
			"internal/app",
		},
		"internal/slash/": {
			"internal/tui",
			"internal/cli",
			"internal/tool",
			"internal/query",
			"internal/permission",
			"internal/provider",
			"internal/mcp",
			"internal/sysprompt",
		},
		"internal/query/": {
			"internal/tui",
			"internal/cli",
			"internal/session",
			"internal/sysprompt",
			"internal/mcp",
			"internal/permission",
		},
		"internal/task/": {
			"internal/model",
			"internal/provider",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/session",
		},
		"internal/config/": {
			"internal/model",
			"internal/provider",
			"internal/observe",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
		},
		"internal/hook/": {
			"internal/model",
			"internal/provider",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/session",
			"internal/sysprompt",
			"internal/permission",
			"internal/slash",
			"internal/mcp",
		},
		"internal/buildinfo/": {
			"internal/model",
			"internal/provider",
			"internal/observe",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/config",
		},
		"internal/cron/": {
			"internal/model",
			"internal/provider",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/session",
			"internal/sysprompt",
			"internal/permission",
			"internal/mcp",
		},
		"internal/skill/": {
			"internal/provider",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/session",
			"internal/sysprompt",
			"internal/permission",
			"internal/mcp",
		},
		"internal/team/": {
			"internal/provider",
			"internal/tool",
			"internal/app",
			"internal/tui",
			"internal/cli",
			"internal/query",
			"internal/session",
			"internal/sysprompt",
			"internal/permission",
			"internal/mcp",
			"internal/model",
		},
	}

	internalDir := filepath.Join(root, "internal")
	files := scanGoFiles(internalDir)

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		imports, err := parseImports(file)
		if err != nil {
			continue
		}

		for pkgSuffix, forbiddenPkgs := range illegal {
			if !strings.Contains(rel, pkgSuffix) {
				continue
			}
			for _, imp := range imports {
				for _, forbidden := range forbiddenPkgs {
					if importsPackage(imp, forbidden) {
						t.Errorf("DAG violation: %s imports %s (forbidden for %s)", rel, imp, pkgSuffix)
					}
				}
			}
		}
	}
}
