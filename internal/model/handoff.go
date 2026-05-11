package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	ContextModeChat         = "chat"
	ContextModeStateHandoff = "state-handoff"
	HandoffSchemaV1         = "handoff.v1"
)

type HandoffState struct {
	SchemaVersion                  string       `json:"schema_version"`
	Goal                           string       `json:"goal"`
	Invariants                     []string     `json:"invariants,omitempty"`
	CurrentFocus                   string       `json:"current_focus,omitempty"`
	Completed                      []string     `json:"completed,omitempty"`
	LatestToolResultInterpretation string       `json:"latest_tool_result_interpretation,omitempty"`
	NextAction                     string       `json:"next_action,omitempty"`
	OpenQuestions                  []string     `json:"open_questions,omitempty"`
	DoNot                          []string     `json:"do_not,omitempty"`
	Evidence                       []string     `json:"evidence,omitempty"`
	Decisions                      []string     `json:"decisions,omitempty"`
	Files                          HandoffFiles `json:"files,omitempty"`
	Risks                          []string     `json:"risks,omitempty"`
}

type HandoffFiles struct {
	Read    []string `json:"read,omitempty"`
	Changed []string `json:"changed,omitempty"`
}

type HandoffPatch struct {
	Ops []HandoffPatchOp `json:"ops"`
}

type HandoffPatchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
}

func NewHandoffState(goal string) HandoffState {
	return HandoffState{
		SchemaVersion: HandoffSchemaV1,
		Goal:          goal,
		Invariants: []string{
			"Use current_handoff_state plus only the latest assistant tool_call blocks and matching tool_result blocks.",
			"Every tool-use response must call PatchHandoffState first, before any real tool.",
			"Preserve user constraints and do_not items unless the user explicitly changes them.",
		},
		CurrentFocus: "Start from the user's latest task.",
		OpenQuestions: []string{
			"What is the first useful tool result interpretation?",
		},
	}
}

func (s HandoffState) IsZero() bool {
	return s.SchemaVersion == "" && s.Goal == ""
}

func (s HandoffState) DeepCopy() HandoffState {
	cp := s
	cp.Invariants = copyStrings(s.Invariants)
	cp.Completed = copyStrings(s.Completed)
	cp.OpenQuestions = copyStrings(s.OpenQuestions)
	cp.DoNot = copyStrings(s.DoNot)
	cp.Evidence = copyStrings(s.Evidence)
	cp.Decisions = copyStrings(s.Decisions)
	cp.Files.Read = copyStrings(s.Files.Read)
	cp.Files.Changed = copyStrings(s.Files.Changed)
	cp.Risks = copyStrings(s.Risks)
	return cp
}

func (s HandoffState) PrettyJSON() string {
	if s.SchemaVersion == "" {
		s.SchemaVersion = HandoffSchemaV1
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func ApplyHandoffPatch(state HandoffState, raw json.RawMessage) (HandoffState, error) {
	if state.SchemaVersion == "" {
		state.SchemaVersion = HandoffSchemaV1
	}
	var patch HandoffPatch
	if err := json.Unmarshal(raw, &patch); err != nil {
		return state, fmt.Errorf("parse handoff patch: %w", err)
	}
	if len(patch.Ops) == 0 {
		return state, fmt.Errorf("handoff patch must include at least one op")
	}

	data, err := json.Marshal(state)
	if err != nil {
		return state, fmt.Errorf("marshal handoff state: %w", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return state, fmt.Errorf("decode handoff state: %w", err)
	}
	seedHandoffPatchContainers(doc)

	for _, op := range patch.Ops {
		if err := applyPatchOp(&doc, op); err != nil {
			return state, err
		}
	}

	outData, err := json.Marshal(doc)
	if err != nil {
		return state, fmt.Errorf("marshal patched handoff state: %w", err)
	}
	var out HandoffState
	if err := json.Unmarshal(outData, &out); err != nil {
		return state, fmt.Errorf("patched handoff state is invalid: %w", err)
	}
	if out.SchemaVersion == "" {
		out.SchemaVersion = HandoffSchemaV1
	}
	return out, nil
}

func seedHandoffPatchContainers(doc any) {
	root, ok := doc.(map[string]any)
	if !ok {
		return
	}
	for _, key := range []string{
		"invariants",
		"completed",
		"open_questions",
		"do_not",
		"evidence",
		"decisions",
		"risks",
	} {
		if _, ok := root[key]; !ok {
			root[key] = []any{}
		}
	}
	files, ok := root["files"].(map[string]any)
	if !ok {
		files = map[string]any{}
		root["files"] = files
	}
	for _, key := range []string{"read", "changed"} {
		if _, ok := files[key]; !ok {
			files[key] = []any{}
		}
	}
}

func applyPatchOp(doc *any, op HandoffPatchOp) error {
	if op.Op != "add" && op.Op != "replace" && op.Op != "remove" {
		return fmt.Errorf("unsupported handoff patch op %q", op.Op)
	}
	if op.Path == "" || op.Path[0] != '/' {
		return fmt.Errorf("invalid handoff patch path %q", op.Path)
	}
	var value any
	if op.Op != "remove" {
		if len(op.Value) == 0 {
			return fmt.Errorf("handoff patch op %q at %q requires value", op.Op, op.Path)
		}
		if err := json.Unmarshal(op.Value, &value); err != nil {
			return fmt.Errorf("parse handoff patch value at %q: %w", op.Path, err)
		}
	}

	parts := parseJSONPointer(op.Path)
	parent, key, err := patchParent(*doc, parts)
	if err != nil {
		return err
	}
	switch p := parent.(type) {
	case map[string]any:
		if op.Op == "remove" {
			delete(p, key)
			return nil
		}
		if op.Op == "replace" {
			if _, ok := p[key]; !ok {
				return fmt.Errorf("cannot replace missing handoff path %q", op.Path)
			}
		}
		p[key] = value
		return nil
	case []any:
		idx, err := patchArrayIndex(key, len(p), op.Op == "add")
		if err != nil {
			return fmt.Errorf("invalid array handoff path %q: %w", op.Path, err)
		}
		if op.Op == "remove" {
			p = append(p[:idx], p[idx+1:]...)
		} else if op.Op == "add" {
			if idx == len(p) {
				p = append(p, value)
			} else {
				p = append(p, nil)
				copy(p[idx+1:], p[idx:])
				p[idx] = value
			}
		} else {
			p[idx] = value
		}
		return setParentArray(doc, parts[:len(parts)-1], p)
	default:
		return fmt.Errorf("handoff patch parent at %q is not object or array", op.Path)
	}
}

func parseJSONPointer(path string) []string {
	raw := strings.Split(path[1:], "/")
	out := make([]string, len(raw))
	for i, p := range raw {
		p = strings.ReplaceAll(p, "~1", "/")
		p = strings.ReplaceAll(p, "~0", "~")
		out[i] = p
	}
	return out
}

func patchParent(doc any, parts []string) (any, string, error) {
	if len(parts) == 0 {
		return nil, "", fmt.Errorf("handoff patch cannot target document root")
	}
	cur := doc
	for _, part := range parts[:len(parts)-1] {
		switch c := cur.(type) {
		case map[string]any:
			next, ok := c[part]
			if !ok {
				return nil, "", fmt.Errorf("missing handoff patch path component %q", part)
			}
			cur = next
		case []any:
			idx, err := patchArrayIndex(part, len(c), false)
			if err != nil {
				return nil, "", err
			}
			cur = c[idx]
		default:
			return nil, "", fmt.Errorf("handoff patch path component %q is not traversable", part)
		}
	}
	return cur, parts[len(parts)-1], nil
}

func patchArrayIndex(part string, length int, allowAppend bool) (int, error) {
	if part == "-" {
		if allowAppend {
			return length, nil
		}
		return 0, fmt.Errorf("append marker is only valid for add")
	}
	idx, err := strconv.Atoi(part)
	if err != nil {
		return 0, err
	}
	if idx < 0 || idx >= length {
		return 0, fmt.Errorf("index %d out of range", idx)
	}
	return idx, nil
}

func setParentArray(doc *any, parts []string, value []any) error {
	if len(parts) == 0 {
		*doc = value
		return nil
	}
	parent, key, err := patchParent(*doc, parts)
	if err != nil {
		return err
	}
	switch p := parent.(type) {
	case map[string]any:
		p[key] = value
		return nil
	case []any:
		idx, err := patchArrayIndex(key, len(p), false)
		if err != nil {
			return err
		}
		p[idx] = value
		return setParentArray(doc, parts[:len(parts)-1], p)
	default:
		return fmt.Errorf("handoff patch parent array path is not writable")
	}
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
