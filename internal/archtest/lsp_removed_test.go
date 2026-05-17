package archtest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyLSPPackagesStayRemoved(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("project root not found")
	}

	for _, dir := range []string{
		filepath.Join(root, "internal", "lsp"),
		filepath.Join(root, "internal", "tools", "lsp"),
	} {
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy LSP package path exists: %s", dir)
		}
	}

	forbiddenImports := map[string]bool{
		"github.com/artpar/pragma/internal/" + "lsp":       true,
		"github.com/artpar/pragma/internal/tools/" + "lsp": true,
	}
	internalDir := filepath.Join(root, "internal")
	if err := filepath.WalkDir(internalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		imports, err := parseImports(path)
		if err != nil {
			return err
		}
		for _, importPath := range imports {
			if forbiddenImports[importPath] {
				t.Fatalf("legacy LSP import %s found in %s", importPath, path)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
