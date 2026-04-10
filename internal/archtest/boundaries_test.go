package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestEventEmissionAtBoundaries(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	// Packages that cross boundaries must emit events
	boundaryPackages := []string{
		filepath.Join(root, "internal", "tool"),
	}

	for _, pkgDir := range boundaryPackages {
		files := scanGoFiles(pkgDir)
		if len(files) == 0 {
			continue
		}

		rel, _ := filepath.Rel(root, pkgDir)
		hasEmit := false

		for _, file := range files {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, file, nil, 0)
			if err != nil {
				continue
			}

			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name == "Emit" {
					hasEmit = true
				}
				return true
			})
		}

		if !hasEmit {
			t.Errorf("boundary package %s has no Emit calls (must emit events at boundaries)", rel)
		}
	}
}

