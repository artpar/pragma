package toollsp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/lsp"
	"github.com/artpar/pragma/internal/observe"
)

type location struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type locationLink struct {
	TargetURI            string   `json:"targetUri"`
	TargetRange          lspRange `json:"targetRange"`
	TargetSelectionRange lspRange `json:"targetSelectionRange"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type hover struct {
	Contents hoverContents `json:"contents"`
	Range    *lspRange     `json:"range,omitempty"`
}

type hoverContents struct {
	Kind  string `json:"kind,omitempty"`
	Value string `json:"value,omitempty"`
	// Also handles string and []MarkedString via custom unmarshal
	Raw string
}

func (h *hoverContents) UnmarshalJSON(data []byte) error {
	// Try MarkupContent first
	var mc struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &mc); err == nil && mc.Value != "" {
		h.Kind = mc.Kind
		h.Value = mc.Value
		return nil
	}
	// Try plain string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		h.Raw = s
		return nil
	}
	h.Raw = string(data)
	return nil
}

func (h hoverContents) text() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if h.Value != "" {
		observe.GlobalTrace("if: h.Value != \"\"")
		observe.GlobalTrace("return: h.Value")
		return h.Value
	}
	observe.GlobalTrace("return: h.Raw")
	return h.Raw
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          lspRange         `json:"range"`
	SelectionRange lspRange         `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

type symbolInformation struct {
	Name     string   `json:"name"`
	Kind     int      `json:"kind"`
	Location location `json:"location"`
}

type callHierarchyItem struct {
	Name           string   `json:"name"`
	Kind           int      `json:"kind"`
	URI            string   `json:"uri"`
	Range          lspRange `json:"range"`
	SelectionRange lspRange `json:"selectionRange"`
}

type callHierarchyIncomingCall struct {
	From       callHierarchyItem `json:"from"`
	FromRanges []lspRange        `json:"fromRanges"`
}

type callHierarchyOutgoingCall struct {
	To         callHierarchyItem `json:"to"`
	FromRanges []lspRange        `json:"fromRanges"`
}

func formatLocations(raw json.RawMessage, cwd, label string) (string, int, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	// Try Location[] first
	var locs []location
	if err := json.Unmarshal(raw, &locs); err != nil || len(locs) == 0 {
		observe.GlobalTrace("if: err != nil || len(locs) == 0")
		// Try LocationLink[]
		var links []locationLink
		if err := json.Unmarshal(raw, &links); err == nil {
			observe.GlobalTrace("if: err == nil")
			for _, l := range links {
				observe.GlobalTrace("range links")
				locs = append(locs, location{URI: l.TargetURI, Range: l.TargetSelectionRange})
			}
		}
	}

	if len(locs) == 0 {
		observe.GlobalTrace("if: len(locs) == 0")
		observe.GlobalTrace("return: fmt.Sprintf(\"No %s found.\", label), 0, 0")
		return fmt.Sprintf("No %s found.", label), 0, 0
	}

	byFile := make(map[string][]location)
	var fileOrder []string
	for _, loc := range locs {
		observe.GlobalTrace("range locs")
		path := lsp.URIToPath(loc.URI)
		if _, seen := byFile[path]; !seen {
			observe.GlobalTrace("if: !seen")
			fileOrder = append(fileOrder, path)
		}
		byFile[path] = append(byFile[path], loc)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d %s in %d file(s):\n", len(locs), label, len(fileOrder))
	for _, path := range fileOrder {
		observe.GlobalTrace("range fileOrder")
		rel := relativePath(path, cwd)
		b.WriteString("\n" + rel + ":\n")
		for _, loc := range byFile[path] {
			observe.GlobalTrace("range byFile[path]")
			fmt.Fprintf(&b, "  Line %d, Col %d\n", loc.Range.Start.Line+1, loc.Range.Start.Character+1)
		}
	}
	observe.GlobalTrace("return: b.String(), len(locs), len(fileOrder)")

	return b.String(), len(locs), len(fileOrder)
}

func formatHover(raw json.RawMessage, _ string) (string, int, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var h hover
	if err := json.Unmarshal(raw, &h); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"No hover information.\", 0, 0")
		return "No hover information.", 0, 0
	}
	text := h.Contents.text()
	if text == "" {
		observe.GlobalTrace("if: text == \"\"")
		observe.GlobalTrace("return: \"No hover information.\", 0, 0")
		return "No hover information.", 0, 0
	}
	observe.GlobalTrace("return: text, 1, 0")
	return text, 1, 0
}

func formatDocumentSymbols(raw json.RawMessage, _ string) (string, int, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	// Try hierarchical DocumentSymbol[] first
	var syms []documentSymbol
	if err := json.Unmarshal(raw, &syms); err == nil && len(syms) > 0 {
		observe.GlobalTrace("if: err == nil && len(syms) > 0")
		var b strings.Builder
		count := writeSymbolTree(&b, syms, 0)
		observe.GlobalTrace("return: b.String(), count, 0")
		return b.String(), count, 0
	}

	// Try flat SymbolInformation[]
	var infos []symbolInformation
	if err := json.Unmarshal(raw, &infos); err == nil && len(infos) > 0 {
		observe.GlobalTrace("if: err == nil && len(infos) > 0")
		var b strings.Builder
		for _, info := range infos {
			observe.GlobalTrace("range infos")
			fmt.Fprintf(&b, "%s %s (line %d)\n",
				symbolKindName(info.Kind), info.Name, info.Location.Range.Start.Line+1)
		}
		observe.GlobalTrace("return: b.String(), len(infos), 0")
		return b.String(), len(infos), 0
	}
	observe.GlobalTrace("return: \"No symbols found.\", 0, 0")

	return "No symbols found.", 0, 0
}

func writeSymbolTree(b *strings.Builder, syms []documentSymbol, indent int) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	count := 0
	prefix := strings.Repeat("  ", indent)
	for _, sym := range syms {
		observe.GlobalTrace("range syms")
		detail := ""
		if sym.Detail != "" {
			observe.GlobalTrace("if: sym.Detail != \"\"")
			detail = " — " + sym.Detail
		}
		fmt.Fprintf(b, "%s%s %s%s (line %d)\n",
			prefix, symbolKindName(sym.Kind), sym.Name, detail, sym.Range.Start.Line+1)
		count++
		if len(sym.Children) > 0 {
			observe.GlobalTrace("if: len(sym.Children) > 0")
			count += writeSymbolTree(b, sym.Children, indent+1)
		}
	}
	observe.GlobalTrace("return: count")
	return count
}

func formatWorkspaceSymbols(raw json.RawMessage, cwd string) (string, int, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var infos []symbolInformation
	if err := json.Unmarshal(raw, &infos); err != nil || len(infos) == 0 {
		observe.GlobalTrace("if: err != nil || len(infos) == 0")
		observe.GlobalTrace("return: \"No symbols found.\", 0, 0")
		return "No symbols found.", 0, 0
	}

	byFile := make(map[string][]symbolInformation)
	var fileOrder []string
	for _, info := range infos {
		observe.GlobalTrace("range infos")
		path := lsp.URIToPath(info.Location.URI)
		if _, seen := byFile[path]; !seen {
			observe.GlobalTrace("if: !seen")
			fileOrder = append(fileOrder, path)
		}
		byFile[path] = append(byFile[path], info)
	}
	sort.Strings(fileOrder)

	var b strings.Builder
	fmt.Fprintf(&b, "%d symbol(s) in %d file(s):\n", len(infos), len(fileOrder))
	for _, path := range fileOrder {
		observe.GlobalTrace("range fileOrder")
		rel := relativePath(path, cwd)
		b.WriteString("\n" + rel + ":\n")
		for _, info := range byFile[path] {
			observe.GlobalTrace("range byFile[path]")
			fmt.Fprintf(&b, "  %s %s (line %d)\n",
				symbolKindName(info.Kind), info.Name, info.Location.Range.Start.Line+1)
		}
	}
	observe.GlobalTrace("return: b.String(), len(infos), len(fileOrder)")

	return b.String(), len(infos), len(fileOrder)
}

func formatCallHierarchyItems(raw json.RawMessage, cwd string) (string, int, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var items []callHierarchyItem
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		observe.GlobalTrace("if: err != nil || len(items) == 0")
		observe.GlobalTrace("return: \"No call hierarchy items found.\", 0, 0")
		return "No call hierarchy items found.", 0, 0
	}

	var b strings.Builder
	for _, item := range items {
		observe.GlobalTrace("range items")
		rel := relativePath(lsp.URIToPath(item.URI), cwd)
		fmt.Fprintf(&b, "%s %s (%s:%d)\n",
			symbolKindName(item.Kind), item.Name, rel, item.Range.Start.Line+1)
	}
	observe.GlobalTrace("return: b.String(), len(items), 0")

	return b.String(), len(items), 0
}

func formatIncomingCalls(raw json.RawMessage, cwd string) (string, int, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var calls []callHierarchyIncomingCall
	if err := json.Unmarshal(raw, &calls); err != nil || len(calls) == 0 {
		observe.GlobalTrace("if: err != nil || len(calls) == 0")
		observe.GlobalTrace("return: \"No incoming calls found.\", 0, 0")
		return "No incoming calls found.", 0, 0
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d incoming call(s):\n", len(calls))
	for _, call := range calls {
		observe.GlobalTrace("range calls")
		rel := relativePath(lsp.URIToPath(call.From.URI), cwd)
		fmt.Fprintf(&b, "  %s %s (%s:%d)\n",
			symbolKindName(call.From.Kind), call.From.Name, rel, call.From.Range.Start.Line+1)
	}
	observe.GlobalTrace("return: b.String(), len(calls), 0")

	return b.String(), len(calls), 0
}

func formatOutgoingCalls(raw json.RawMessage, cwd string) (string, int, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var calls []callHierarchyOutgoingCall
	if err := json.Unmarshal(raw, &calls); err != nil || len(calls) == 0 {
		observe.GlobalTrace("if: err != nil || len(calls) == 0")
		observe.GlobalTrace("return: \"No outgoing calls found.\", 0, 0")
		return "No outgoing calls found.", 0, 0
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d outgoing call(s):\n", len(calls))
	for _, call := range calls {
		observe.GlobalTrace("range calls")
		rel := relativePath(lsp.URIToPath(call.To.URI), cwd)
		fmt.Fprintf(&b, "  %s %s (%s:%d)\n",
			symbolKindName(call.To.Kind), call.To.Name, rel, call.To.Range.Start.Line+1)
	}
	observe.GlobalTrace("return: b.String(), len(calls), 0")

	return b.String(), len(calls), 0
}

func relativePath(path, cwd string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cwd == "" {
		observe.GlobalTrace("if: cwd == \"\"")
		observe.GlobalTrace("return: path")
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)+"..") {
		observe.GlobalTrace("if: err != nil || strings.HasPrefix(rel, \"..\"+string(filepath.Separator)+\"..\")")
		observe.GlobalTrace("return: path")
		return path
	}
	if len(rel) < len(path) {
		observe.GlobalTrace("if: len(rel) < len(path)")
		observe.GlobalTrace("return: rel")
		return rel
	}
	observe.GlobalTrace("return: path")
	return path
}

var symbolKindNames = map[int]string{
	1: "File", 2: "Module", 3: "Namespace", 4: "Package",
	5: "Class", 6: "Method", 7: "Property", 8: "Field",
	9: "Constructor", 10: "Enum", 11: "Interface", 12: "Function",
	13: "Variable", 14: "Constant", 15: "String", 16: "Number",
	17: "Boolean", 18: "Array", 19: "Object", 20: "Key",
	21: "Null", 22: "EnumMember", 23: "Struct", 24: "Event",
	25: "Operator", 26: "TypeParameter",
}

func symbolKindName(kind int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if name, ok := symbolKindNames[kind]; ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: name")
		return name
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"Kind(%d)\", kind)")
	return fmt.Sprintf("Kind(%d)", kind)
}
