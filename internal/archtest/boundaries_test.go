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

	// Each boundary package must emit specific event types
	type boundaryCheck struct {
		dir            string
		requiredEvents []string
	}

	checks := []boundaryCheck{
		{
			dir: filepath.Join(root, "internal", "tool"),
			requiredEvents: []string{
				"ToolCallReceived",
				"ToolPermissionChecked",
				"ToolExecutionStarted",
				"ToolBatchStarted",
				"ToolBatchCompleted",
			},
		},
		{
			dir: filepath.Join(root, "internal", "provider", "anthropic"),
			requiredEvents: []string{
				"APIRequestStarted",
				"APIRequestCompleted",
			},
		},
	}

	for _, check := range checks {
		files := scanGoFiles(check.dir)
		if len(files) == 0 {
			continue
		}

		rel, _ := filepath.Rel(root, check.dir)
		emittedEvents := make(map[string]bool)

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
				if sel.Sel.Name != "Emit" || len(call.Args) == 0 {
					return true
				}
				// Check composite literal argument for event type name
				if comp, ok := call.Args[0].(*ast.CompositeLit); ok {
					if typeSel, ok := comp.Type.(*ast.SelectorExpr); ok {
						emittedEvents[typeSel.Sel.Name] = true
					}
				}
				return true
			})
		}

		for _, required := range check.requiredEvents {
			if !emittedEvents[required] {
				t.Errorf("boundary package %s must emit %s event but does not", rel, required)
			}
		}
	}
}
