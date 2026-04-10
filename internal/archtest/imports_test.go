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
		"github.com/artpar/gogent/internal/provider/anthropic",
		"github.com/artpar/gogent/internal/provider/openai",
		"github.com/artpar/gogent/internal/provider/google",
	}

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		// Provider adapters themselves are allowed
		if strings.Contains(rel, "internal/provider/anthropic") ||
			strings.Contains(rel, "internal/provider/openai") ||
			strings.Contains(rel, "internal/provider/google") {
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

func TestPackageDependencyDAG(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	// Define illegal import edges per the DAG in AGENT.md
	// key = package path suffix, value = packages it must NOT import
	illegal := map[string][]string{
		"internal/model/": {
			"internal/provider/",
			"internal/observe/",
			"internal/tool/",
			"internal/app/",
			"internal/permission/",
			"internal/cli/",
			"internal/tui/",
			"internal/query/",
		},
		"internal/permission/": {
			"internal/model/",
			"internal/provider/",
			"internal/observe/",
			"internal/tool/",
			"internal/app/",
		},
		"internal/app/": {
			"internal/tool/",
			"internal/provider/",
			"internal/observe/",
			"internal/cli/",
			"internal/tui/",
			"internal/query/",
		},
		"internal/util/": {
			"internal/model/",
			"internal/provider/",
			"internal/observe/",
			"internal/tool/",
			"internal/app/",
		},
		"internal/sysprompt/": {
			"internal/provider/",
			"internal/tool/",
			"internal/app/",
			"internal/tui/",
			"internal/cli/",
			"internal/query/",
			"internal/session/",
			"internal/permission/",
		},
		"internal/session/": {
			"internal/provider/",
			"internal/observe/",
			"internal/tool/",
			"internal/app/",
			"internal/tui/",
			"internal/cli/",
			"internal/query/",
			"internal/sysprompt/",
			"internal/permission/",
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
					if strings.Contains(imp, forbidden) {
						t.Errorf("DAG violation: %s imports %s (forbidden for %s)", rel, imp, pkgSuffix)
					}
				}
			}
		}
	}
}
