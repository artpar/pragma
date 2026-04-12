package lsp

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/artpar/gogent/internal/observe"
)

// DiagnosticSeverity mirrors LSP DiagnosticSeverity values.
const (
	SeverityError   = 1
	SeverityWarning = 2
	SeverityInfo    = 3
	SeverityHint    = 4
)

const (
	maxDiagnosticsPerFile = 10
	maxDiagnosticsTotal   = 30
	maxDeliveredFiles     = 500
)

// Diagnostic is a simplified LSP diagnostic.
type Diagnostic struct {
	Range    DiagRange `json:"range"`
	Severity int       `json:"severity"`
	Code     string    `json:"code,omitempty"`
	Source   string    `json:"source,omitempty"`
	Message  string    `json:"message"`
}

// DiagRange is a simplified LSP range.
type DiagRange struct {
	Start DiagPosition `json:"start"`
	End   DiagPosition `json:"end"`
}

// DiagPosition is a line/character pair (0-based from LSP).
type DiagPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// PendingDiagnostic holds diagnostics from a single publishDiagnostics notification.
type PendingDiagnostic struct {
	ServerName  string
	FileURI     string
	Diagnostics []Diagnostic
}

// DiagnosticResult is a per-file result for delivery.
type DiagnosticResult struct {
	ServerName  string
	FileURI     string
	Diagnostics []Diagnostic
}

// DiagnosticRegistry stores pending and delivered diagnostics.
// Cross-turn deduplication via delivered set. Volume limiting.
type DiagnosticRegistry struct {
	mu             sync.Mutex
	pending        map[string]*PendingDiagnostic // fileURI -> latest diagnostics
	delivered      map[string]map[string]bool    // fileURI -> set of message hashes (LRU approximation)
	deliveredOrder []string                      // track insertion order for LRU eviction
}

// NewDiagnosticRegistry creates a diagnostic registry.
func NewDiagnosticRegistry() *DiagnosticRegistry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &DiagnosticRegistry{\n\tpending:\tmake(map[string]*PendingDiagnostic),\n\tdelivere...")
	observe.GlobalTrace("return: &DiagnosticRegistry{\n\tpending:\tmake(map[string]*PendingDiagnostic),\n\tdelivere...")
	return &DiagnosticRegistry{
		pending:   make(map[string]*PendingDiagnostic),
		delivered: make(map[string]map[string]bool),
	}
}

// Register stores diagnostics from a publishDiagnostics notification.
// Called from the notification handler on the LSP client.
func (r *DiagnosticRegistry) Register(serverName, fileURI string, rawDiags json.RawMessage) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var diags []Diagnostic
	if err := json.Unmarshal(rawDiags, &diags); err != nil {
		observe.GlobalTrace("if: err != nil")
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(diags) == 0 {
		observe.GlobalTrace("if: len(diags) == 0")
		delete(r.pending, fileURI)
		return
	}

	r.pending[fileURI] = &PendingDiagnostic{
		ServerName:  serverName,
		FileURI:     fileURI,
		Diagnostics: diags,
	}
}

// Check returns pending diagnostics, deduplicated against previously delivered.
// Volume limited: 10 per file, 30 total. Sorted by severity (Error first).
func (r *DiagnosticRegistry) Check() []DiagnosticResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.pending) == 0 {
		observe.GlobalTrace("if: len(r.pending) == 0")
		observe.GlobalTrace("return: nil")
		observe.GlobalTrace("return: nil")
		return nil
	}

	var results []DiagnosticResult
	totalCount := 0

	for fileURI, pd := range r.pending {
		observe.GlobalTrace("range r.pending")
		deliveredSet := r.delivered[fileURI]
		if deliveredSet == nil {
			observe.GlobalTrace("if: deliveredSet == nil")
			deliveredSet = make(map[string]bool)
			r.delivered[fileURI] = deliveredSet
			r.deliveredOrder = append(r.deliveredOrder, fileURI)
		}

		sorted := make([]Diagnostic, len(pd.Diagnostics))
		copy(sorted, pd.Diagnostics)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Severity < sorted[j].Severity
		})

		var filtered []Diagnostic
		for _, d := range sorted {
			observe.GlobalTrace("range sorted")
			if len(filtered) >= maxDiagnosticsPerFile {
				observe.GlobalTrace("if: len(filtered) >= maxDiagnosticsPerFile")
				break
			}
			if totalCount >= maxDiagnosticsTotal {
				observe.GlobalTrace("if: totalCount >= maxDiagnosticsTotal")
				break
			}
			key := diagKey(d)
			if deliveredSet[key] {
				observe.GlobalTrace("if: deliveredSet[key]")
				continue
			}
			deliveredSet[key] = true
			filtered = append(filtered, d)
			totalCount++
		}

		if len(filtered) > 0 {
			observe.GlobalTrace("if: len(filtered) > 0")
			results = append(results, DiagnosticResult{
				ServerName:  pd.ServerName,
				FileURI:     fileURI,
				Diagnostics: filtered,
			})
		}
	}

	r.pending = make(map[string]*PendingDiagnostic)

	for len(r.deliveredOrder) > maxDeliveredFiles {
		observe.GlobalTrace("for: len(r.deliveredOrder) > maxDeliveredFiles")
		oldest := r.deliveredOrder[0]
		r.deliveredOrder = r.deliveredOrder[1:]
		delete(r.delivered, oldest)
	}
	observe.GlobalTrace("return: results")
	observe.GlobalTrace("return: results")

	return results
}

// ClearFileDelivered removes delivered tracking for a file,
// so new diagnostics for that file will be shown after edits.
func (r *DiagnosticRegistry) ClearFileDelivered(fileURI string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.delivered, fileURI)
}

// Reset clears all state.
func (r *DiagnosticRegistry) Reset() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = make(map[string]*PendingDiagnostic)
	r.delivered = make(map[string]map[string]bool)
	r.deliveredOrder = nil
}

func diagKey(d Diagnostic) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	key := fmt.Sprintf("%s|%s|%s|%d|%d:%d",
		d.Message, d.Source, d.Code, d.Severity,
		d.Range.Start.Line, d.Range.Start.Character)
	observe.GlobalTrace("return: key")
	observe.GlobalTrace("return: key")
	return key
}
