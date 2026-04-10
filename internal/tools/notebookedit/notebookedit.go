package notebookedit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

func (t *Tool) Name() string                { return "NotebookEdit" }
func (t *Tool) Description() string          { return "Edit Jupyter notebook cells (replace, insert, or delete)." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		NotebookPath string `json:"notebook_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.NotebookPath == "" {
		return checker.Check(ctx, "NotebookEdit", "")
	}
	return checker.Check(ctx, "NotebookEdit", in.NotebookPath)
}

func (t *Tool) Invoke(_ context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	var in NotebookEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	// Validate
	if in.NotebookPath == "" {
		return tool.InvokeResult{}, fmt.Errorf("notebook_path is required")
	}
	if !strings.HasSuffix(strings.ToLower(in.NotebookPath), ".ipynb") {
		return tool.InvokeResult{}, fmt.Errorf("file must be a .ipynb notebook")
	}

	// Resolve path
	nbPath := in.NotebookPath
	if !filepath.IsAbs(nbPath) {
		nbPath = filepath.Join(state.WorkDir(), nbPath)
	}
	nbPath = filepath.Clean(nbPath)

	editMode := in.EditMode
	if editMode == "" {
		editMode = "replace"
	}

	// Read notebook
	data, err := os.ReadFile(nbPath)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("read notebook: %w", err)
	}

	var nb notebookContent
	if err := json.Unmarshal(data, &nb); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("parse notebook: %w", err)
	}

	switch editMode {
	case "replace":
		return t.doReplace(&nb, nbPath, in)
	case "insert":
		return t.doInsert(&nb, nbPath, in)
	case "delete":
		return t.doDelete(&nb, nbPath, in)
	default:
		return tool.InvokeResult{}, fmt.Errorf("unknown edit_mode: %q", editMode)
	}
}

func (t *Tool) doReplace(nb *notebookContent, nbPath string, in NotebookEditInput) (tool.InvokeResult, error) {
	idx, err := findCell(nb, in.CellID)
	if err != nil {
		return tool.InvokeResult{}, err
	}

	cell := &nb.Cells[idx]
	cell.Source = marshalSource(in.NewSource)

	// Reset execution state for code cells
	if cell.CellType == "code" {
		cell.ExecutionCount = nil
		cell.Outputs = json.RawMessage("[]")
	}

	if err := writeNotebook(nbPath, nb); err != nil {
		return tool.InvokeResult{}, err
	}

	return tool.InvokeResult{
		Content: fmt.Sprintf("Updated cell %s in %s", cellIDStr(cell, idx), nbPath),
	}, nil
}

func (t *Tool) doInsert(nb *notebookContent, nbPath string, in NotebookEditInput) (tool.InvokeResult, error) {
	cellType := in.CellType
	if cellType == "" {
		cellType = "code"
	}

	newCell := notebookCell{
		CellType: cellType,
		Source:   marshalSource(in.NewSource),
		Metadata: json.RawMessage("{}"),
	}
	if cellType == "code" {
		newCell.Outputs = json.RawMessage("[]")
	}

	// Determine insertion index
	insertIdx := len(nb.Cells) // append by default
	if in.CellID != "" {
		idx, err := findCell(nb, in.CellID)
		if err != nil {
			return tool.InvokeResult{}, err
		}
		insertIdx = idx + 1 // insert after the referenced cell
	}

	// Splice
	nb.Cells = append(nb.Cells, notebookCell{})
	copy(nb.Cells[insertIdx+1:], nb.Cells[insertIdx:])
	nb.Cells[insertIdx] = newCell

	if err := writeNotebook(nbPath, nb); err != nil {
		return tool.InvokeResult{}, err
	}

	return tool.InvokeResult{
		Content: fmt.Sprintf("Inserted %s cell at index %d in %s", cellType, insertIdx, nbPath),
	}, nil
}

func (t *Tool) doDelete(nb *notebookContent, nbPath string, in NotebookEditInput) (tool.InvokeResult, error) {
	idx, err := findCell(nb, in.CellID)
	if err != nil {
		return tool.InvokeResult{}, err
	}

	cellID := cellIDStr(&nb.Cells[idx], idx)
	nb.Cells = append(nb.Cells[:idx], nb.Cells[idx+1:]...)

	if err := writeNotebook(nbPath, nb); err != nil {
		return tool.InvokeResult{}, err
	}

	return tool.InvokeResult{
		Content: fmt.Sprintf("Deleted cell %s from %s", cellID, nbPath),
	}, nil
}

// findCell resolves a cell ID (by ID string or numeric index) to its index.
func findCell(nb *notebookContent, cellID string) (int, error) {
	if cellID == "" {
		if len(nb.Cells) == 0 {
			return 0, fmt.Errorf("notebook has no cells")
		}
		return 0, nil
	}

	// Try matching by cell ID field
	for i, cell := range nb.Cells {
		if cell.ID == cellID {
			return i, nil
		}
	}

	// Try as numeric index
	idx, err := strconv.Atoi(cellID)
	if err == nil {
		if idx < 0 || idx >= len(nb.Cells) {
			return 0, fmt.Errorf("cell index %d out of range (0-%d)", idx, len(nb.Cells)-1)
		}
		return idx, nil
	}

	return 0, fmt.Errorf("cell %q not found", cellID)
}

// marshalSource converts a string to the ipynb source format (array of lines).
func marshalSource(s string) json.RawMessage {
	lines := strings.SplitAfter(s, "\n")
	// Remove trailing empty string from split if source doesn't end with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	data, _ := json.Marshal(lines)
	return data
}

func cellIDStr(cell *notebookCell, idx int) string {
	if cell.ID != "" {
		return cell.ID
	}
	return fmt.Sprintf("#%d", idx)
}

func writeNotebook(path string, nb *notebookContent) error {
	data, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return fmt.Errorf("marshal notebook: %w", err)
	}
	// ipynb convention: trailing newline
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}
