package archtest

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNoDuplicateContentTypes(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	internalDir := filepath.Join(root, "internal")
	files := scanGoFiles(internalDir)

	variants := []string{"TextPart", "ImagePart", "ToolCallPart", "ToolResultPart", "ThinkingPart"}

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		// content.go in model/ is the canonical source
		if strings.HasSuffix(rel, "internal/model/content.go") {
			continue
		}

		decls := findTypeDecls(file)
		for _, d := range decls {
			for _, v := range variants {
				if d.name == v {
					t.Errorf("%s:%d defines %s (should only be in internal/model/content.go)", rel, d.line, v)
				}
			}
		}
	}
}

func TestNoDuplicateMessageTypes(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	internalDir := filepath.Join(root, "internal")
	files := scanGoFiles(internalDir)

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		if strings.HasSuffix(rel, "internal/model/message.go") {
			continue
		}

		// TUI may have its own Message type (tea.Msg)
		if strings.Contains(rel, "internal/tui/") {
			continue
		}

		decls := findTypeDecls(file)
		for _, d := range decls {
			if d.name == "Message" {
				t.Errorf("%s:%d defines Message (should only be in internal/model/message.go)", rel, d.line)
			}
		}
	}
}

func TestNoDuplicateToolInterface(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	internalDir := filepath.Join(root, "internal")
	files := scanGoFiles(internalDir)

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		if strings.HasSuffix(rel, "internal/tool/tool.go") {
			continue
		}

		decls := findTypeDecls(file)
		for _, d := range decls {
			if d.name == "Descriptor" {
				t.Errorf("%s:%d defines Descriptor (should only be in internal/tool/tool.go)", rel, d.line)
			}
		}
	}
}
