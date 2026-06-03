package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPersonaYAMLs(t *testing.T) {
	assertPersonaDir(t, filepath.Join("..", "..", "personas"))
	assertPersonaDir(t, filepath.Join("..", "..", "personas-research-v2"))
}

func assertPersonaDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read persona dir %s: %v", dir, err)
	}
	found := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		found++
		path := filepath.Join(dir, entry.Name())
		id := strings.TrimSuffix(entry.Name(), ".yaml")
		def, err := LoadDefinitionFile(path)
		if err != nil {
			t.Fatalf("load %s persona: %v", path, err)
		}
		if def.ID != id {
			t.Fatalf("persona %s id = %q, want %q", path, def.ID, id)
		}
		if def.Prompt == "" {
			t.Fatalf("persona %s prompt is empty", path)
		}
	}
	if found == 0 {
		t.Fatalf("persona dir %s has no YAML files", dir)
	}
}
