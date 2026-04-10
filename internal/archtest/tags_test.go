package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolInputStructTags(t *testing.T) {
	root := projectRoot()
	if root == "" {
		t.Fatal("could not find project root")
	}

	toolsDir := filepath.Join(root, "internal", "tools")
	files := scanGoFiles(toolsDir)

	if len(files) == 0 {
		t.Skip("no tool implementations yet")
	}

	for _, file := range files {
		rel, _ := filepath.Rel(root, file)

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			continue
		}

		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts := spec.(*ast.TypeSpec)
				// Look for Input structs
				if !strings.HasSuffix(ts.Name.Name, "Input") {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					if field.Tag == nil {
						for _, name := range field.Names {
							pos := fset.Position(field.Pos())
							t.Errorf("%s:%d field %s in %s has no struct tag (need json+desc)",
								rel, pos.Line, name.Name, ts.Name.Name)
						}
						continue
					}
					tag := field.Tag.Value
					if !strings.Contains(tag, `json:`) {
						for _, name := range field.Names {
							pos := fset.Position(field.Pos())
							t.Errorf("%s:%d field %s in %s missing json tag",
								rel, pos.Line, name.Name, ts.Name.Name)
						}
					}
					if !strings.Contains(tag, `desc:`) {
						for _, name := range field.Names {
							pos := fset.Position(field.Pos())
							t.Errorf("%s:%d field %s in %s missing desc tag",
								rel, pos.Line, name.Name, ts.Name.Name)
						}
					}
				}
			}
		}
	}
}
