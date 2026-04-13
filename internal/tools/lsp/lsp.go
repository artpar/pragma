package toollsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artpar/gogent/internal/lsp"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

const maxFileSizeBytes = 10 * 1024 * 1024 // 10 MB

type lspInput struct {
	Operation string `json:"operation" desc:"LSP operation to perform"`
	FilePath  string `json:"file_path" desc:"Absolute or relative file path"`
	Line      int    `json:"line" desc:"1-based line number"`
	Character int    `json:"character" desc:"1-based character/column number"`
	Query     string `json:"query,omitempty" desc:"Search query for workspaceSymbol"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["operation", "file_path"],
	"properties": {
		"operation": {
			"type": "string",
			"enum": ["goToDefinition", "findReferences", "hover", "documentSymbol", "workspaceSymbol", "goToImplementation", "prepareCallHierarchy", "incomingCalls", "outgoingCalls"],
			"description": "The LSP operation to perform"
		},
		"file_path": {
			"type": "string",
			"description": "Absolute or relative file path"
		},
		"line": {
			"type": "integer",
			"description": "1-based line number (required for position-based operations)"
		},
		"character": {
			"type": "integer",
			"description": "1-based character/column number (required for position-based operations)"
		},
		"query": {
			"type": "string",
			"description": "Search query for workspaceSymbol operation"
		}
	}
}`)

// Tool provides code intelligence via LSP servers.
type Tool struct {
	Manager *lsp.Manager
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"LSP\"")
	return "LSP"
}
func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: `Provides code intelligence via Language Server Protocol. Operations: goToDef...")
	return `Provides code intelligence via Language Server Protocol. Operations: goToDefinition, findReferences, hover, documentSymbol, workspaceSymbol, goToImplementation, prepareCallHierarchy, incomingCalls, outgoingCalls. Requires LSP servers configured in .gogent/lsp.json.`
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "toollsp", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "toollsp", "Tool.CheckPerm", "exit")
	var in lspInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "toollsp", "Tool.CheckPerm", "if: err != nil")
		observe.TraceCtx(ctx, "toollsp", "Tool.CheckPerm", "return: checker.Check(ctx, \"LSP\", \"\")")
		return checker.Check(ctx, "LSP", "")
	}
	observe.TraceCtx(ctx, "toollsp", "Tool.CheckPerm", "return: checker.Check(ctx, \"LSP\", in.FilePath)")
	return checker.Check(ctx, "LSP", in.FilePath)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "exit")
	var in lspInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	if in.Operation == "" {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: in.Operation == \"\"")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"operation is required\")")
		return tool.InvokeResult{}, fmt.Errorf("operation is required")
	}
	if in.FilePath == "" {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: in.FilePath == \"\"")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file_path is required\")")
		return tool.InvokeResult{}, fmt.Errorf("file_path is required")
	}

	absPath := in.FilePath
	if !filepath.IsAbs(absPath) {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: !filepath.IsAbs(absPath)")
		absPath = filepath.Join(state.WorkDir(), absPath)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file not found: %s\", absPath)")
		return tool.InvokeResult{}, fmt.Errorf("file not found: %s", absPath)
	}
	if info.IsDir() {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: info.IsDir()")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"path is a directory: %s\", absPath)")
		return tool.InvokeResult{}, fmt.Errorf("path is a directory: %s", absPath)
	}
	if info.Size() > maxFileSizeBytes {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: info.Size() > maxFileSizeBytes")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file too large (%d bytes, max %d)\", info.Siz...")
		return tool.InvokeResult{}, fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxFileSizeBytes)
	}

	needsPosition := in.Operation != "documentSymbol" && in.Operation != "workspaceSymbol"
	if needsPosition {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: needsPosition")
		if in.Line < 1 {
			observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: in.Line < 1")
			observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"line must be >= 1 (got %d)\", in.Line)")
			return tool.InvokeResult{}, fmt.Errorf("line must be >= 1 (got %d)", in.Line)
		}
		if in.Character < 1 {
			observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: in.Character < 1")
			observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"character must be >= 1 (got %d)\", in.Character)")
			return tool.InvokeResult{}, fmt.Errorf("character must be >= 1 (got %d)", in.Character)
		}
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"read file: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("read file: %w", err)
	}
	if err := t.Manager.OpenFile(ctx, absPath, string(content)); err != nil {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"open file in LSP: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("open file in LSP: %w", err)
	}

	fileURI := lsp.PathToURI(absPath)
	cwd := state.WorkDir()

	line := in.Line - 1
	char := in.Character - 1

	var result json.RawMessage
	var formatted string
	var resultCount, fileCount int

	switch in.Operation {
	case "goToDefinition":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"goToDefinition\"")
		result, err = t.Manager.SendRequest(ctx, absPath, "textDocument/definition", positionParams(fileURI, line, char))
		if err == nil {
			result = filterLocationResults(ctx, cwd, result)
			formatted, resultCount, fileCount = formatLocations(result, cwd, "definition(s)")
		}

	case "findReferences":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"findReferences\"")
		params := positionParams(fileURI, line, char)
		params["context"] = map[string]bool{"includeDeclaration": true}
		result, err = t.Manager.SendRequest(ctx, absPath, "textDocument/references", params)
		if err == nil {
			result = filterLocationResults(ctx, cwd, result)
			formatted, resultCount, fileCount = formatLocations(result, cwd, "reference(s)")
		}

	case "hover":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"hover\"")
		result, err = t.Manager.SendRequest(ctx, absPath, "textDocument/hover", positionParams(fileURI, line, char))
		if err == nil {
			formatted, resultCount, fileCount = formatHover(result, cwd)
		}

	case "documentSymbol":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"documentSymbol\"")
		result, err = t.Manager.SendRequest(ctx, absPath, "textDocument/documentSymbol", map[string]any{
			"textDocument": map[string]string{"uri": fileURI},
		})
		if err == nil {
			formatted, resultCount, fileCount = formatDocumentSymbols(result, cwd)
		}

	case "workspaceSymbol":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"workspaceSymbol\"")
		query := in.Query
		result, err = t.Manager.SendRequest(ctx, absPath, "workspace/symbol", map[string]string{
			"query": query,
		})
		if err == nil {
			formatted, resultCount, fileCount = formatWorkspaceSymbols(result, cwd)
		}

	case "goToImplementation":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"goToImplementation\"")
		result, err = t.Manager.SendRequest(ctx, absPath, "textDocument/implementation", positionParams(fileURI, line, char))
		if err == nil {
			result = filterLocationResults(ctx, cwd, result)
			formatted, resultCount, fileCount = formatLocations(result, cwd, "implementation(s)")
		}

	case "prepareCallHierarchy":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"prepareCallHierarchy\"")
		result, err = t.Manager.SendRequest(ctx, absPath, "textDocument/prepareCallHierarchy", positionParams(fileURI, line, char))
		if err == nil {
			formatted, resultCount, fileCount = formatCallHierarchyItems(result, cwd)
		}

	case "incomingCalls":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"incomingCalls\"")
		result, err = t.twoStepCallHierarchy(ctx, absPath, fileURI, line, char, "callHierarchy/incomingCalls")
		if err == nil && result != nil {
			formatted, resultCount, fileCount = formatIncomingCalls(result, cwd)
		} else if err == nil {
			formatted = "No call hierarchy item at position."
		}

	case "outgoingCalls":
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "case: \"outgoingCalls\"")
		result, err = t.twoStepCallHierarchy(ctx, absPath, fileURI, line, char, "callHierarchy/outgoingCalls")
		if err == nil && result != nil {
			formatted, resultCount, fileCount = formatOutgoingCalls(result, cwd)
		} else if err == nil {
			formatted = "No call hierarchy item at position."
		}

	default:
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "default")
		return tool.InvokeResult{}, fmt.Errorf("unknown operation: %s", in.Operation)
	}

	if err != nil {
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"LSP error: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("LSP error: %v", err)}, nil
	}

	output := struct {
		Operation   string `json:"operation"`
		Result      string `json:"result"`
		FilePath    string `json:"filePath"`
		ResultCount int    `json:"resultCount,omitempty"`
		FileCount   int    `json:"fileCount,omitempty"`
	}{
		Operation:   in.Operation,
		Result:      formatted,
		FilePath:    absPath,
		ResultCount: resultCount,
		FileCount:   fileCount,
	}
	data, _ := json.Marshal(output)
	observe.TraceCtx(ctx, "toollsp", "Tool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

func (t *Tool) twoStepCallHierarchy(ctx context.Context, absPath, fileURI string, line, char int, method string) (json.RawMessage, error) {
	observe.TraceCtx(ctx, "toollsp", "Tool.twoStepCallHierarchy", "enter")
	defer observe.TraceCtx(ctx, "toollsp", "Tool.twoStepCallHierarchy", "exit")

	prepResult, err := t.Manager.SendRequest(ctx, absPath, "textDocument/prepareCallHierarchy", positionParams(fileURI, line, char))
	if err != nil {
		observe.TraceCtx(ctx, "toollsp", "Tool.twoStepCallHierarchy", "if: err != nil")
		observe.TraceCtx(ctx, "toollsp", "Tool.twoStepCallHierarchy", "return: nil, err")
		return nil, err
	}

	var items []callHierarchyItem
	if err := json.Unmarshal(prepResult, &items); err != nil || len(items) == 0 {
		observe.TraceCtx(ctx, "toollsp", "Tool.twoStepCallHierarchy", "if: err != nil || len(items) == 0")
		observe.TraceCtx(ctx, "toollsp", "Tool.twoStepCallHierarchy", "return: nil, nil")
		return nil, nil
	}

	result, err := t.Manager.SendRequest(ctx, absPath, method, map[string]any{
		"item": items[0],
	})
	observe.TraceCtx(ctx, "toollsp", "Tool.twoStepCallHierarchy", "return: result, err")
	return result, err
}

// filterLocationResults removes gitignored files from location-based LSP results.
// Works with both Location[] and LocationLink[] formats.
func filterLocationResults(ctx context.Context, cwd string, raw json.RawMessage) json.RawMessage {
	observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "enter")
	defer observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "exit")
	// Extract URIs from Location[] format
	var locs []location
	if err := json.Unmarshal(raw, &locs); err == nil && len(locs) > 0 {
		observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "if: err == nil && len(locs) > 0")
		uris := make([]string, len(locs))
		for i, loc := range locs {
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "range locs")
			uris[i] = loc.URI
		}
		kept := filterGitIgnored(ctx, cwd, uris)
		if len(kept) == len(uris) {
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "if: len(kept) == len(uris)")
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "return: raw")
			return raw
		}
		keptSet := make(map[string]bool, len(kept))
		for _, u := range kept {
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "range kept")
			keptSet[u] = true
		}
		var filtered []location
		for _, loc := range locs {
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "range locs")
			if keptSet[loc.URI] {
				observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "if: keptSet[loc.URI]")
				filtered = append(filtered, loc)
			}
		}
		out, _ := json.Marshal(filtered)
		observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "return: out")
		return out
	}

	// Try LocationLink[] format
	var links []locationLink
	if err := json.Unmarshal(raw, &links); err == nil && len(links) > 0 {
		observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "if: err == nil && len(links) > 0")
		uris := make([]string, len(links))
		for i, l := range links {
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "range links")
			uris[i] = l.TargetURI
		}
		kept := filterGitIgnored(ctx, cwd, uris)
		if len(kept) == len(uris) {
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "if: len(kept) == len(uris)")
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "return: raw")
			return raw
		}
		keptSet := make(map[string]bool, len(kept))
		for _, u := range kept {
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "range kept")
			keptSet[u] = true
		}
		var filtered []locationLink
		for _, l := range links {
			observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "range links")
			if keptSet[l.TargetURI] {
				observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "if: keptSet[l.TargetURI]")
				filtered = append(filtered, l)
			}
		}
		out, _ := json.Marshal(filtered)
		observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "return: out")
		return out
	}
	observe.TraceCtx(ctx, "toollsp", "filterLocationResults", "return: raw")

	return raw
}

func positionParams(fileURI string, line, char int) map[string]any {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: map[string]any{\n\t\"textDocument\":\tmap[string]string{\"uri\": fileURI},\n\t\"positio...")
	return map[string]any{
		"textDocument": map[string]string{"uri": fileURI},
		"position":     map[string]int{"line": line, "character": char},
	}
}
