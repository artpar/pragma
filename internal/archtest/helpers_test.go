package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type location struct {
	file string
	line int
}

type typeDecl struct {
	name string
	file string
	line int
}

// projectRoot finds the module root by looking for go.mod.
func projectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// scanGoFiles returns all .go files under root, excluding _test.go and vendor.
func scanGoFiles(root string) []string {
	var files []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == ".git" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	return files
}

// parseImports returns all import paths in a Go file.
func parseImports(filePath string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var imports []string
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		imports = append(imports, path)
	}
	return imports, nil
}

// findCallExprs finds function calls matching any of the patterns (package.Func format).
func findCallExprs(filePath string, patterns []string) []location {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return nil
	}

	patternSet := make(map[string]bool, len(patterns))
	for _, p := range patterns {
		patternSet[p] = true
	}

	var locs []location
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if ident, ok := fn.X.(*ast.Ident); ok {
				name = ident.Name + "." + fn.Sel.Name
			}
		case *ast.Ident:
			name = fn.Name
		}
		if patternSet[name] {
			pos := fset.Position(call.Pos())
			locs = append(locs, location{file: filePath, line: pos.Line})
		}
		return true
	})
	return locs
}

// findTypeDecls returns all type declarations in a file.
func findTypeDecls(filePath string) []typeDecl {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return nil
	}

	var decls []typeDecl
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts := spec.(*ast.TypeSpec)
			pos := fset.Position(ts.Pos())
			decls = append(decls, typeDecl{
				name: ts.Name.Name,
				file: filePath,
				line: pos.Line,
			})
		}
	}
	return decls
}
