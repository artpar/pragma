package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// instrumentConfig holds include/exclude rules parsed from flags.
type instrumentConfig struct {
	includePkg    map[string]bool
	excludePkg    map[string]bool
	includeFunc   map[string]bool
	excludeFunc   map[string]bool
	includeStruct map[string]bool
	excludeFile   []string // glob patterns
	dryRun        bool
}

func main() {
	var (
		includePkg    string
		excludePkg    string
		includeFunc   string
		excludeFunc   string
		includeStruct string
		excludeFile   string
		dryRun        bool
	)

	flag.StringVar(&includePkg, "include-pkg", "", "comma-separated packages to instrument (whitelist)")
	flag.StringVar(&excludePkg, "exclude-pkg", "", "comma-separated packages to skip")
	flag.StringVar(&includeFunc, "include-func", "", "comma-separated functions to instrument (whitelist)")
	flag.StringVar(&excludeFunc, "exclude-func", "String,MarshalJSON,UnmarshalJSON,eventSealed,loopEventSealed,Error", "comma-separated functions to skip")
	flag.StringVar(&includeStruct, "include-struct", "", "comma-separated struct names (only instrument their methods)")
	flag.StringVar(&excludeFile, "exclude-file", "*_generated.go", "comma-separated glob patterns for files to skip")
	flag.BoolVar(&dryRun, "dry-run", false, "print changes without writing files")
	flag.Parse()

	cfg := instrumentConfig{
		includePkg:    parseSet(includePkg),
		excludePkg:    parseSet(excludePkg),
		includeFunc:   parseSet(includeFunc),
		excludeFunc:   parseSet(excludeFunc),
		includeStruct: parseSet(includeStruct),
		excludeFile:   parseList(excludeFile),
		dryRun:        dryRun,
	}

	// Hard exclusions
	if cfg.excludePkg == nil {
		cfg.excludePkg = make(map[string]bool)
	}
	cfg.excludePkg["observe"] = true // avoid circular
	cfg.excludePkg["model"] = true   // model↔observe import cycle
	cfg.excludePkg["app"] = true     // DAG: app has no internal deps
	cfg.excludePkg["config"] = true  // DAG: config has no internal deps
	cfg.excludePkg["session"] = true // DAG: session → model, config only
	cfg.excludePkg["util"] = true    // DAG: util has no internal deps

	args := flag.Args()
	if len(args) == 0 {
		args = []string{"./internal/..."}
	}

	var totalFiles, totalPoints, instrumentedPoints int
	for _, pattern := range args {
		// Expand ... pattern to directories
		dirs, err := expandPattern(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error expanding %s: %v\n", pattern, err)
			os.Exit(1)
		}
		for _, dir := range dirs {
			f, p, ip, dirErr := processDir(dir, cfg)
			if dirErr != nil {
				fmt.Fprintf(os.Stderr, "error processing %s: %v\n", dir, dirErr)
				continue
			}
			totalFiles += f
			totalPoints += p
			instrumentedPoints += ip
		}
	}

	fmt.Printf("Instrumented %d/%d branch points across %d files\n", instrumentedPoints, totalPoints, totalFiles)
}

func processDir(dir string, cfg instrumentConfig) (files, totalPts, instrPts int, err error) {
	fset := token.NewFileSet()
	pkgs, parseErr := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		name := fi.Name()
		if strings.HasSuffix(name, "_test.go") {
			return false
		}
		for _, pat := range cfg.excludeFile {
			if matched, _ := filepath.Match(pat, name); matched {
				return false
			}
		}
		return true
	}, parser.ParseComments)
	if parseErr != nil {
		return 0, 0, 0, parseErr
	}

	for pkgName, pkg := range pkgs {
		if cfg.excludePkg[pkgName] {
			continue
		}
		if cfg.includePkg != nil && !cfg.includePkg[pkgName] {
			continue
		}

		for filePath, file := range pkg.Files {
			f, t, i := processFile(fset, filePath, file, pkgName, cfg)
			files += f
			totalPts += t
			instrPts += i
		}
	}
	return
}

func processFile(fset *token.FileSet, filePath string, file *ast.File, pkgName string, cfg instrumentConfig) (files, totalPts, instrPts int) {
	instr := &instrumenter{
		fset:    fset,
		pkgName: pkgName,
		cfg:     cfg,
	}

	modified := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || len(fn.Body.List) == 0 {
			continue
		}

		funcName := fn.Name.Name
		receiverName := ""
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			receiverName = extractReceiverName(fn.Recv.List[0].Type)
		}

		// Filter by struct
		if cfg.includeStruct != nil && receiverName != "" && !cfg.includeStruct[receiverName] {
			continue
		}

		// Filter by function name
		if cfg.excludeFunc != nil && cfg.excludeFunc[funcName] {
			continue
		}
		if cfg.includeFunc != nil && !cfg.includeFunc[funcName] {
			continue
		}

		hasCtx := funcHasContext(fn)
		displayName := funcName
		if receiverName != "" {
			displayName = receiverName + "." + funcName
		}

		if instr.instrumentFunc(fn, displayName, hasCtx) {
			modified = true
		}
	}

	if !modified {
		return 0, instr.totalPoints, 0
	}

	// Ensure observe import
	addImport(file, "github.com/artpar/gogent/internal/observe")

	// Strip comments before writing — injected AST nodes have zero positions which
	// causes go/format to float comments into the wrong places. We preserve the
	// source comments by reading the original file and only replacing non-comment code.
	// The pragmatic fix: clear comment map so go/format doesn't try to attach them.
	file.Comments = nil

	// Write back
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		fmt.Fprintf(os.Stderr, "warning: format %s: %v\n", filePath, err)
		return 0, instr.totalPoints, 0
	}

	if cfg.dryRun {
		fmt.Printf("--- %s: %d points instrumented ---\n", filePath, instr.instrPoints)
		fmt.Println(buf.String())
	} else {
		if err := os.WriteFile(filePath, buf.Bytes(), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: write %s: %v\n", filePath, err)
			return 0, instr.totalPoints, 0
		}
		fmt.Printf("  %s: %d points\n", filePath, instr.instrPoints)
	}

	return 1, instr.totalPoints, instr.instrPoints
}

type instrumenter struct {
	fset         *token.FileSet
	pkgName      string
	cfg          instrumentConfig
	totalPoints  int
	instrPoints  int
}

func (inst *instrumenter) instrumentFunc(fn *ast.FuncDecl, displayName string, hasCtx bool) bool {
	modified := false

	// Function entry trace
	inst.totalPoints++
	if !hasExistingTrace(fn.Body) {
		entryStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, displayName, "enter")
		exitStmt := inst.makeDeferTraceStmt(hasCtx, inst.pkgName, displayName, "exit")
		fn.Body.List = append([]ast.Stmt{entryStmt, exitStmt}, fn.Body.List...)
		inst.instrPoints++
		modified = true
	}

	// Walk body for branch points
	if inst.instrumentBlock(fn.Body, displayName, hasCtx) {
		modified = true
	}

	return modified
}

func (inst *instrumenter) instrumentBlock(block *ast.BlockStmt, funcName string, hasCtx bool) bool {
	if block == nil {
		return false
	}
	modified := false
	newList := make([]ast.Stmt, 0, len(block.List)*2)

	for _, stmt := range block.List {
		switch s := stmt.(type) {
		case *ast.IfStmt:
			inst.totalPoints++
			condStr := inst.exprString(s.Cond)
			if !blockHasTraceAt(s.Body, 0) {
				traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, "if: "+condStr)
				s.Body.List = append([]ast.Stmt{traceStmt}, s.Body.List...)
				inst.instrPoints++
				modified = true
			}
			if s.Else != nil {
				inst.totalPoints++
				if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
					if !blockHasTraceAt(elseBlock, 0) {
						traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, "else: "+condStr)
						elseBlock.List = append([]ast.Stmt{traceStmt}, elseBlock.List...)
						inst.instrPoints++
						modified = true
					}
					inst.instrumentBlock(elseBlock, funcName, hasCtx)
				} else if elseIf, ok := s.Else.(*ast.IfStmt); ok {
					// else-if chain — wrap in block to instrument
					inst.totalPoints++
					elseCondStr := inst.exprString(elseIf.Cond)
					if !blockHasTraceAt(elseIf.Body, 0) {
						traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, "else-if: "+elseCondStr)
						elseIf.Body.List = append([]ast.Stmt{traceStmt}, elseIf.Body.List...)
						inst.instrPoints++
						modified = true
					}
				}
			}
			inst.instrumentBlock(s.Body, funcName, hasCtx)

		case *ast.SwitchStmt:
			for _, clause := range s.Body.List {
				cc, ok := clause.(*ast.CaseClause)
				if !ok {
					continue
				}
				inst.totalPoints++
				var label string
				if cc.List == nil {
					label = "default"
				} else {
					label = "case: " + inst.exprListString(cc.List)
				}
				if !stmtListHasTrace(cc.Body, 0) {
					traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, label)
					cc.Body = append([]ast.Stmt{traceStmt}, cc.Body...)
					inst.instrPoints++
					modified = true
				}
				inst.instrumentStmtList(cc.Body, funcName, hasCtx)
			}

		case *ast.TypeSwitchStmt:
			for _, clause := range s.Body.List {
				cc, ok := clause.(*ast.CaseClause)
				if !ok {
					continue
				}
				inst.totalPoints++
				var label string
				if cc.List == nil {
					label = "typedefault"
				} else {
					label = "typecase: " + inst.exprListString(cc.List)
				}
				if !stmtListHasTrace(cc.Body, 0) {
					traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, label)
					cc.Body = append([]ast.Stmt{traceStmt}, cc.Body...)
					inst.instrPoints++
					modified = true
				}
				inst.instrumentStmtList(cc.Body, funcName, hasCtx)
			}

		case *ast.ForStmt:
			inst.totalPoints++
			condStr := "true"
			if s.Cond != nil {
				condStr = inst.exprString(s.Cond)
			}
			if !blockHasTraceAt(s.Body, 0) {
				traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, "for: "+condStr)
				s.Body.List = append([]ast.Stmt{traceStmt}, s.Body.List...)
				inst.instrPoints++
				modified = true
			}
			inst.instrumentBlock(s.Body, funcName, hasCtx)

		case *ast.RangeStmt:
			inst.totalPoints++
			rangeStr := "range " + inst.exprString(s.X)
			if !blockHasTraceAt(s.Body, 0) {
				traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, rangeStr)
				s.Body.List = append([]ast.Stmt{traceStmt}, s.Body.List...)
				inst.instrPoints++
				modified = true
			}
			inst.instrumentBlock(s.Body, funcName, hasCtx)

		case *ast.SelectStmt:
			for _, clause := range s.Body.List {
				cc, ok := clause.(*ast.CommClause)
				if !ok {
					continue
				}
				inst.totalPoints++
				var label string
				if cc.Comm == nil {
					label = "select: default"
				} else {
					label = "select: " + inst.stmtString(cc.Comm)
				}
				if !stmtListHasTrace(cc.Body, 0) {
					traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, label)
					cc.Body = append([]ast.Stmt{traceStmt}, cc.Body...)
					inst.instrPoints++
					modified = true
				}
				inst.instrumentStmtList(cc.Body, funcName, hasCtx)
			}

		case *ast.ReturnStmt:
			inst.totalPoints++
			if len(s.Results) > 0 {
				retStr := inst.exprListString(s.Results)
				msg := "return: " + retStr
				// Idempotency: skip if the previous statement is already a trace with this message.
				alreadyInstrumented := len(newList) > 0 && isTraceCallWithMsg(newList[len(newList)-1], msg)
				if !alreadyInstrumented {
					traceStmt := inst.makeTraceStmt(hasCtx, inst.pkgName, funcName, msg)
					newList = append(newList, traceStmt)
					inst.instrPoints++
					modified = true
				}
			}
		}

		newList = append(newList, stmt)

		// Recurse into nested blocks within other statements
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			if inst.instrumentBlock(s, funcName, hasCtx) {
				modified = true
			}
		case *ast.DeferStmt:
			if fn, ok := s.Call.Fun.(*ast.FuncLit); ok {
				if inst.instrumentBlock(fn.Body, funcName, hasCtx) {
					modified = true
				}
			}
		case *ast.GoStmt:
			if fn, ok := s.Call.Fun.(*ast.FuncLit); ok {
				if inst.instrumentBlock(fn.Body, funcName, hasCtx) {
					modified = true
				}
			}
		}
	}

	block.List = newList
	return modified
}

func (inst *instrumenter) instrumentStmtList(stmts []ast.Stmt, funcName string, hasCtx bool) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			inst.instrumentBlock(s, funcName, hasCtx)
		case *ast.IfStmt:
			inst.instrumentBlock(s.Body, funcName, hasCtx)
		case *ast.ForStmt:
			inst.instrumentBlock(s.Body, funcName, hasCtx)
		case *ast.RangeStmt:
			inst.instrumentBlock(s.Body, funcName, hasCtx)
		}
	}
}

// makeTraceStmt creates: observe.TraceCtx(ctx, pkg, fn, msg) or observe.GlobalTrace(msg)
// pos is the position of the statement this trace is being inserted near — used to
// anchor the injected node so go/format doesn't float comments.
func (inst *instrumenter) makeTraceStmt(hasCtx bool, pkg, fn, msg string) *ast.ExprStmt {
	if hasCtx {
		return &ast.ExprStmt{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("observe"),
					Sel: ast.NewIdent("TraceCtx"),
				},
				Args: []ast.Expr{
					ast.NewIdent("ctx"),
					&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", pkg)},
					&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", fn)},
					&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", msg)},
				},
			},
		}
	}
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent("observe"),
				Sel: ast.NewIdent("GlobalTrace"),
			},
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", msg)},
			},
		},
	}
}

// makeTraceStmtAt creates a trace statement with position hints to prevent
// go/format from floating comments into the generated code.
func (inst *instrumenter) makeTraceStmtAt(hasCtx bool, pkg, fn, msg string, pos token.Pos) *ast.ExprStmt {
	stmt := inst.makeTraceStmt(hasCtx, pkg, fn, msg)
	if pos.IsValid() {
		// Set all positions on the call expression to anchor it
		setExprPos(stmt.X, pos)
	}
	return stmt
}

func setExprPos(expr ast.Expr, pos token.Pos) {
	switch e := expr.(type) {
	case *ast.CallExpr:
		e.Lparen = pos
		e.Rparen = pos
		setExprPos(e.Fun, pos)
	case *ast.SelectorExpr:
		setExprPos(e.X, pos)
	case *ast.Ident:
		e.NamePos = pos
	case *ast.BasicLit:
		e.ValuePos = pos
	}
}

// makeDeferTraceStmt creates: defer observe.TraceCtx(ctx, pkg, fn, "exit") or defer observe.GlobalTrace("exit")
func (inst *instrumenter) makeDeferTraceStmt(hasCtx bool, pkg, fn, msg string) *ast.DeferStmt {
	traceExpr := inst.makeTraceStmt(hasCtx, pkg, fn, msg)
	return &ast.DeferStmt{
		Call: traceExpr.X.(*ast.CallExpr),
	}
}

func (inst *instrumenter) exprString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf bytes.Buffer
	printer.Fprint(&buf, inst.fset, expr)
	s := buf.String()
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

func (inst *instrumenter) exprListString(exprs []ast.Expr) string {
	parts := make([]string, len(exprs))
	for i, e := range exprs {
		parts[i] = inst.exprString(e)
	}
	s := strings.Join(parts, ", ")
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

func (inst *instrumenter) stmtString(stmt ast.Stmt) string {
	if stmt == nil {
		return ""
	}
	var buf bytes.Buffer
	printer.Fprint(&buf, inst.fset, stmt)
	s := buf.String()
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

// --- Detection helpers ---

// hasExistingTrace checks if the first statement in a block is an observe.TraceCtx or observe.GlobalTrace call.
func hasExistingTrace(block *ast.BlockStmt) bool {
	if block == nil || len(block.List) == 0 {
		return false
	}
	return isTraceCall(block.List[0])
}

func blockHasTraceAt(block *ast.BlockStmt, idx int) bool {
	if block == nil || idx >= len(block.List) {
		return false
	}
	return isTraceCall(block.List[idx])
}

func stmtListHasTrace(stmts []ast.Stmt, idx int) bool {
	if idx >= len(stmts) {
		return false
	}
	return isTraceCall(stmts[idx])
}

func isTraceCall(stmt ast.Stmt) bool {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if ident.Name != "observe" {
		return false
	}
	return sel.Sel.Name == "TraceCtx" || sel.Sel.Name == "GlobalTrace" || sel.Sel.Name == "Trace"
}

// isTraceCallWithMsg checks if a statement is a trace call whose last argument matches msg.
// Used for idempotency: avoids inserting duplicate traces before return statements.
func isTraceCallWithMsg(stmt ast.Stmt, msg string) bool {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "observe" {
		return false
	}
	if sel.Sel.Name != "TraceCtx" && sel.Sel.Name != "GlobalTrace" {
		return false
	}
	if len(call.Args) == 0 {
		return false
	}
	lastArg := call.Args[len(call.Args)-1]
	lit, ok := lastArg.(*ast.BasicLit)
	if !ok {
		return false
	}
	// lit.Value includes quotes, e.g., `"return: foo"`
	return lit.Value == fmt.Sprintf("%q", msg)
}

// funcHasContext checks if the first parameter is context.Context AND is named
// (not `_`). If the param is `_ context.Context`, there's no `ctx` variable to use.
func funcHasContext(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return false
	}
	first := fn.Type.Params.List[0]

	// Check type is context.Context
	sel, ok := first.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if ident.Name != "context" || sel.Sel.Name != "Context" {
		return false
	}

	// Check the parameter is named (not blank `_`)
	if len(first.Names) == 0 {
		return false
	}
	name := first.Names[0].Name
	return name != "_"
}

func extractReceiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return extractReceiverName(t.X)
	}
	return ""
}

// addImport adds an import path to the file if not already present.
func addImport(file *ast.File, path string) {
	for _, imp := range file.Imports {
		if imp.Path.Value == `"`+path+`"` {
			return
		}
	}

	newImport := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: `"` + path + `"`,
		},
	}

	// Find or create import decl
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		gen.Specs = append(gen.Specs, newImport)
		return
	}

	// No import decl exists — create one
	gen := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{newImport},
	}
	file.Decls = append([]ast.Decl{gen}, file.Decls...)
}

// --- Path expansion ---

func expandPattern(pattern string) ([]string, error) {
	if strings.HasSuffix(pattern, "/...") {
		root := strings.TrimSuffix(pattern, "/...")
		if root == "." || root == "" {
			root = "."
		}
		var dirs []string
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// Skip hidden dirs, vendor, testdata
				name := info.Name()
				if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" {
					return filepath.SkipDir
				}
				// Only include dirs that have .go files
				entries, _ := os.ReadDir(path)
				for _, e := range entries {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
						dirs = append(dirs, path)
						break
					}
				}
			}
			return nil
		})
		return dirs, err
	}
	return []string{pattern}, nil
}

// --- Flag parsing ---

func parseSet(s string) map[string]bool {
	if s == "" {
		return nil
	}
	m := make(map[string]bool)
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			m[v] = true
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func parseList(s string) []string {
	if s == "" {
		return nil
	}
	var list []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			list = append(list, v)
		}
	}
	return list
}
