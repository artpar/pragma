package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestPermissionAuditCompleteness(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	// Scan tool/ package for CheckPerm calls and verify nearby Emit calls
	toolDir := filepath.Join(root, "internal", "tool")
	files := scanGoFiles(toolDir)

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			continue
		}

		// Find functions that call CheckPerm
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok {
				return true
			}

			hasCheckPerm := false
			hasPermissionEmit := false

			ast.Inspect(fn, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				case "CheckPerm":
					hasCheckPerm = true
				case "Emit":
					// Check if Emit argument is a permission event type
					if len(call.Args) > 0 {
						if comp, ok := call.Args[0].(*ast.CompositeLit); ok {
							if sel, ok := comp.Type.(*ast.SelectorExpr); ok {
								name := sel.Sel.Name
								if name == "ToolPermissionChecked" ||
									name == "PermissionDenialEnforced" ||
									name == "ToolPermissionPrompted" ||
									name == "PermissionEscalated" {
									hasPermissionEmit = true
								}
							}
						}
					}
				}
				return true
			})

			if hasCheckPerm && !hasPermissionEmit {
				pos := fset.Position(fn.Pos())
				t.Errorf("%s:%d function %s calls CheckPerm but never emits a permission event (ToolPermissionChecked/PermissionDenialEnforced)",
					rel, pos.Line, fn.Name.Name)
			}
			return true
		})
	}
}
