package notebookedit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type NotebookEditInput struct {
	NotebookPath string `json:"notebook_path" desc:"Absolute path to .ipynb file"`
	CellID       string `json:"cell_id,omitempty" desc:"Cell ID or numeric index to edit"`
	NewSource    string `json:"new_source" desc:"New source content for the cell"`
	CellType     string `json:"cell_type,omitempty" desc:"code or markdown (required for insert)"`
	EditMode     string `json:"edit_mode,omitempty" desc:"replace, insert, or delete (default: replace)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["notebook_path", "new_source"],
	"properties": {
		"notebook_path": {
			"type": "string",
			"description": "The absolute path to the .ipynb file"
		},
		"cell_id": {
			"type": "string",
			"description": "Cell ID or numeric index to edit"
		},
		"new_source": {
			"type": "string",
			"description": "New source content for the cell"
		},
		"cell_type": {
			"type": "string",
			"enum": ["code", "markdown"],
			"description": "Cell type (required for insert mode)"
		},
		"edit_mode": {
			"type": "string",
			"enum": ["replace", "insert", "delete"],
			"description": "Edit mode: replace (default), insert, or delete"
		}
	}
}`)

// notebookContent represents a Jupyter notebook JSON structure.
type notebookContent struct {
	Cells         []notebookCell  `json:"cells"`
	Metadata      json.RawMessage `json:"metadata"`
	NBFormat      int             `json:"nbformat"`
	NBFormatMinor int             `json:"nbformat_minor"`
}

// notebookCell represents a single cell in a notebook.
type notebookCell struct {
	CellType       string          `json:"cell_type"`
	ID             string          `json:"id,omitempty"`
	Source         json.RawMessage `json:"source"`
	Metadata       json.RawMessage `json:"metadata"`
	ExecutionCount *int            `json:"execution_count,omitempty"`
	Outputs        json.RawMessage `json:"outputs,omitempty"`
}

// Tool implements the NotebookEdit tool.
type Tool struct{}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"NotebookEdit\"")
	return "NotebookEdit"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Edit Jupyter notebook cells (replace, insert, or delete).\"")
	return "Edit Jupyter notebook cells (replace, insert, or delete)."
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
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "notebookedit", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "notebookedit", "Tool.CheckPerm", "exit")
	var in struct {
		NotebookPath string `json:"notebook_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.NotebookPath == "" {
		observe.TraceCtx(ctx, "notebookedit", "Tool.CheckPerm", "if: err != nil || in.NotebookPath == \"\"")
		observe.TraceCtx(ctx, "notebookedit", "Tool.CheckPerm", "return: checker.Check(ctx, \"NotebookEdit\", \"\")")
		return checker.Check(ctx, "NotebookEdit", "")
	}
	observe.TraceCtx(ctx, "notebookedit", "Tool.CheckPerm", "return: checker.Check(ctx, \"NotebookEdit\", in.NotebookPath)")
	return checker.Check(ctx, "NotebookEdit", in.NotebookPath)
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var in NotebookEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	if in.NotebookPath == "" {
		observe.GlobalTrace("if: in.NotebookPath == \"\"")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"notebook_path is required\")")
		return tool.InvokeResult{}, fmt.Errorf("notebook_path is required")
	}
	if !strings.HasSuffix(strings.ToLower(in.NotebookPath), ".ipynb") {
		observe.GlobalTrace("if: !strings.HasSuffix(strings.ToLower(in.NotebookPath), \".ipynb\")")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"file must be a .ipynb notebook\")")
		return tool.InvokeResult{}, fmt.Errorf("file must be a .ipynb notebook")
	}

	nbPath := in.NotebookPath
	if !filepath.IsAbs(nbPath) {
		observe.GlobalTrace("if: !filepath.IsAbs(nbPath)")
		nbPath = filepath.Join(state.WorkDir(), nbPath)
	}
	nbPath = filepath.Clean(nbPath)

	editMode := in.EditMode
	if editMode == "" {
		observe.GlobalTrace("if: editMode == \"\"")
		editMode = "replace"
	}

	data, err := os.ReadFile(nbPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"read notebook: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("read notebook: %w", err)
	}

	var nb notebookContent
	if err := json.Unmarshal(data, &nb); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"parse notebook: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("parse notebook: %w", err)
	}

	switch editMode {
	case "replace":
		observe.GlobalTrace("case: \"replace\"")
		return t.doReplace(&nb, nbPath, in)
	case "insert":
		observe.GlobalTrace("case: \"insert\"")
		return t.doInsert(&nb, nbPath, in)
	case "delete":
		observe.GlobalTrace("case: \"delete\"")
		return t.doDelete(&nb, nbPath, in)
	default:
		observe.GlobalTrace("default")
		return tool.InvokeResult{}, fmt.Errorf("unknown edit_mode: %q", editMode)
	}
}

func (t *Tool) doReplace(nb *notebookContent, nbPath string, in NotebookEditInput) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	idx, err := findCell(nb, in.CellID)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}

	cell := &nb.Cells[idx]
	cell.Source = marshalSource(in.NewSource)

	if cell.CellType == "code" {
		observe.GlobalTrace("if: cell.CellType == \"code\"")
		cell.ExecutionCount = nil
		cell.Outputs = json.RawMessage("[]")
	}

	if err := writeNotebook(nbPath, nb); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Updated cell %s in %s\", cellIDStr(c...")

	return tool.InvokeResult{
		Content: fmt.Sprintf("Updated cell %s in %s", cellIDStr(cell, idx), nbPath),
	}, nil
}

func (t *Tool) doInsert(nb *notebookContent, nbPath string, in NotebookEditInput) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cellType := in.CellType
	if cellType == "" {
		observe.GlobalTrace("if: cellType == \"\"")
		cellType = "code"
	}

	newCell := notebookCell{
		CellType: cellType,
		Source:   marshalSource(in.NewSource),
		Metadata: json.RawMessage("{}"),
	}
	if cellType == "code" {
		observe.GlobalTrace("if: cellType == \"code\"")
		newCell.Outputs = json.RawMessage("[]")
	}

	insertIdx := len(nb.Cells)
	if in.CellID != "" {
		observe.GlobalTrace("if: in.CellID != \"\"")
		idx, err := findCell(nb, in.CellID)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: tool.InvokeResult{}, err")
			return tool.InvokeResult{}, err
		}
		insertIdx = idx + 1
	}

	nb.Cells = append(nb.Cells, notebookCell{})
	copy(nb.Cells[insertIdx+1:], nb.Cells[insertIdx:])
	nb.Cells[insertIdx] = newCell

	if err := writeNotebook(nbPath, nb); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Inserted %s cell at index %d in %s\"...")

	return tool.InvokeResult{
		Content: fmt.Sprintf("Inserted %s cell at index %d in %s", cellType, insertIdx, nbPath),
	}, nil
}

func (t *Tool) doDelete(nb *notebookContent, nbPath string, in NotebookEditInput) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	idx, err := findCell(nb, in.CellID)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}

	cellID := cellIDStr(&nb.Cells[idx], idx)
	nb.Cells = append(nb.Cells[:idx], nb.Cells[idx+1:]...)

	if err := writeNotebook(nbPath, nb); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Deleted cell %s from %s\", cellID, n...")

	return tool.InvokeResult{
		Content: fmt.Sprintf("Deleted cell %s from %s", cellID, nbPath),
	}, nil
}

// findCell resolves a cell ID (by ID string or numeric index) to its index.
func findCell(nb *notebookContent, cellID string) (int, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cellID == "" {
		observe.GlobalTrace("if: cellID == \"\"")
		if len(nb.Cells) == 0 {
			observe.GlobalTrace("if: len(nb.Cells) == 0")
			observe.GlobalTrace("return: 0, fmt.Errorf(\"notebook has no cells\")")
			return 0, fmt.Errorf("notebook has no cells")
		}
		observe.GlobalTrace("return: 0, nil")
		return 0, nil
	}

	for i, cell := range nb.Cells {
		observe.GlobalTrace("range nb.Cells")
		if cell.ID == cellID {
			observe.GlobalTrace("if: cell.ID == cellID")
			observe.GlobalTrace("return: i, nil")
			return i, nil
		}
	}

	idx, err := strconv.Atoi(cellID)
	if err == nil {
		observe.GlobalTrace("if: err == nil")
		if idx < 0 || idx >= len(nb.Cells) {
			observe.GlobalTrace("if: idx < 0 || idx >= len(nb.Cells)")
			observe.GlobalTrace("return: 0, fmt.Errorf(\"cell index %d out of range (0-%d)\", idx, len(nb.Cells)-1)")
			return 0, fmt.Errorf("cell index %d out of range (0-%d)", idx, len(nb.Cells)-1)
		}
		observe.GlobalTrace("return: idx, nil")
		return idx, nil
	}
	observe.GlobalTrace("return: 0, fmt.Errorf(\"cell %q not found\", cellID)")

	return 0, fmt.Errorf("cell %q not found", cellID)
}

// marshalSource converts a string to the ipynb source format (array of lines).
func marshalSource(s string) json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lines := strings.SplitAfter(s, "\n")

	if len(lines) > 0 && lines[len(lines)-1] == "" {
		observe.GlobalTrace("if: len(lines) > 0 && lines[len(lines)-1] == \"\"")
		lines = lines[:len(lines)-1]
	}
	data, _ := json.Marshal(lines)
	observe.GlobalTrace("return: data")
	return data
}

func cellIDStr(cell *notebookCell, idx int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cell.ID != "" {
		observe.GlobalTrace("if: cell.ID != \"\"")
		observe.GlobalTrace("return: cell.ID")
		return cell.ID
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"#%d\", idx)")
	return fmt.Sprintf("#%d", idx)
}

func writeNotebook(path string, nb *notebookContent) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"marshal notebook: %w\", err)")
		return fmt.Errorf("marshal notebook: %w", err)
	}

	data = append(data, '\n')
	observe.GlobalTrace("return: os.WriteFile(path, data, 0644)")
	return os.WriteFile(path, data, 0644)
}
