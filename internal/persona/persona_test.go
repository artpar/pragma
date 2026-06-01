package persona

import (
	"path/filepath"
	"testing"
)

func TestLoadPersonaYAMLs(t *testing.T) {
	for _, id := range []string{"architect", "implementer", "prosecutor"} {
		def, err := LoadDefinitionFile(filepath.Join("..", "..", "personas", id+".yaml"))
		if err != nil {
			t.Fatalf("load %s persona: %v", id, err)
		}
		if def.ID != id {
			t.Fatalf("persona id = %q, want %q", def.ID, id)
		}
		if def.Prompt == "" {
			t.Fatalf("persona %q prompt is empty", id)
		}
	}
}
