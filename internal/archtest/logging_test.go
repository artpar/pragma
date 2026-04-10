package archtest

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNoAdHocLogging(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	internalDir := filepath.Join(root, "internal")
	files := scanGoFiles(internalDir)

	forbidden := []string{
		"fmt.Print",
		"fmt.Println",
		"fmt.Printf",
		"log.Print",
		"log.Println",
		"log.Printf",
	}

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		// observe/, tui/, and cli/ are allowed to use direct output
		if strings.Contains(rel, "internal/observe/") ||
			strings.Contains(rel, "internal/tui/") ||
			strings.Contains(rel, "internal/cli/") {
			continue
		}

		locs := findCallExprs(file, forbidden)
		for _, loc := range locs {
			t.Errorf("%s:%d uses direct logging (use observe.EventBus instead)", rel, loc.line)
		}
	}
}
