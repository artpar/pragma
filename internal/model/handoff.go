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
	SchemaVersion                  string                   `json:"schema_version"`
	Goal                           string                   `json:"goal"`
	Invariants                     []string                 `json:"invariants,omitempty"`
	CurrentFocus                   string                   `json:"current_focus,omitempty"`
	Investigation                  HandoffInvestigation     `json:"investigation"`
	CertifiedFacts                 map[string]CertifiedFact `json:"certified_facts,omitempty"`
	Todos                          []HandoffTodo            `json:"todos,omitempty"`
	Completed                      []string                 `json:"completed,omitempty"`
	LatestToolResultInterpretation string                   `json:"latest_tool_result_interpretation,omitempty"`
	NextAction                     string                   `json:"next_action,omitempty"`
	RecentActions                  []string                 `json:"recent_actions,omitempty"`
	OpenQuestions                  []string                 `json:"open_questions,omitempty"`
	DoNot                          []string                 `json:"do_not,omitempty"`
	Evidence                       []string                 `json:"evidence,omitempty"`
	Decisions                      []string                 `json:"decisions,omitempty"`
	Files                          HandoffFiles             `json:"files,omitempty"`
	Risks                          []string                 `json:"risks,omitempty"`
	Extra                          map[string]any           `json:"-"`
}

type HandoffInvestigation struct {
	ObservedContracts []HandoffObservedContract `json:"observed_contracts,omitempty"`
	AcceptanceChecks  []HandoffAcceptanceCheck  `json:"acceptance_checks,omitempty"`
	CertifiedFactRefs []string                  `json:"certified_fact_refs,omitempty"`
	ReadyForChanges   bool                      `json:"ready_for_changes"`
	Blockers          []string                  `json:"blockers,omitempty"`
}

type HandoffObservedContract struct {
	Name     string   `json:"name,omitempty"`
	Source   string   `json:"source,omitempty"`
	Evidence string   `json:"evidence,omitempty"`
	Fields   []string `json:"fields,omitempty"`
	FactRefs []string `json:"fact_refs,omitempty"`
}

type HandoffAcceptanceCheck struct {
	Description string   `json:"description,omitempty"`
	Command     string   `json:"command,omitempty"`
	Expected    string   `json:"expected,omitempty"`
	Status      string   `json:"status,omitempty"`
	FactRefs    []string `json:"fact_refs,omitempty"`
}

type CertifiedFact struct {
	ID              string         `json:"id"`
	Kind            string         `json:"kind"`
	Source          string         `json:"source,omitempty"`
	Claim           string         `json:"claim,omitempty"`
	Evidence        string         `json:"evidence,omitempty"`
	Fields          []string       `json:"fields,omitempty"`
	MatchingRecords int            `json:"matching_records,omitempty"`
	SampleHash      string         `json:"sample_hash,omitempty"`
	ToolCallID      string         `json:"tool_call_id,omitempty"`
	Verified        bool           `json:"verified"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type HandoffTodo struct {
	ID     string `json:"id,omitempty"`
	Task   string `json:"task"`
	Status string `json:"status"`
}

type HandoffFiles struct {
	Read    []string       `json:"read,omitempty"`
	Changed []string       `json:"changed,omitempty"`
	Extra   map[string]any `json:"-"`
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
			"Certified facts are written only by CertifyFact; PatchHandoffState may only reference certified fact IDs.",
			"Maintain todos for long-running tasks; use statuses pending, in_progress, completed, or blocked.",
			"Interpret each tool_result into durable state and choose a non-repeating next_action.",
			"Before non-read-only tools, use CertifyFact, reference certified facts from investigation.certified_fact_refs, observed_contracts.fact_refs, and acceptance_checks.fact_refs, and set investigation.ready_for_changes.",
			"Do not end with a plan when implementation or verification work remains; keep using tools until todos are completed or blocked.",
			"Preserve user constraints and do_not items unless the user explicitly changes them.",
		},
		CurrentFocus:                   "Start from the user's latest task.",
		LatestToolResultInterpretation: "No tool results interpreted yet.",
		NextAction:                     "Choose the first useful tool call.",
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
	cp.Investigation = s.Investigation.DeepCopy()
	cp.CertifiedFacts = copyCertifiedFacts(s.CertifiedFacts)
	cp.Todos = copyHandoffTodos(s.Todos)
	cp.Completed = copyStrings(s.Completed)
	cp.RecentActions = copyStrings(s.RecentActions)
	cp.OpenQuestions = copyStrings(s.OpenQuestions)
	cp.DoNot = copyStrings(s.DoNot)
	cp.Evidence = copyStrings(s.Evidence)
	cp.Decisions = copyStrings(s.Decisions)
	cp.Files.Read = copyStrings(s.Files.Read)
	cp.Files.Changed = copyStrings(s.Files.Changed)
	cp.Files.Extra = copyJSONMap(s.Files.Extra)
	cp.Risks = copyStrings(s.Risks)
	cp.Extra = copyJSONMap(s.Extra)
	return cp
}

func (i HandoffInvestigation) DeepCopy() HandoffInvestigation {
	cp := i
	cp.ObservedContracts = copyObservedContracts(i.ObservedContracts)
	cp.AcceptanceChecks = copyAcceptanceChecks(i.AcceptanceChecks)
	cp.CertifiedFactRefs = copyStrings(i.CertifiedFactRefs)
	cp.Blockers = copyStrings(i.Blockers)
	return cp
}

func (s HandoffState) ChangeGateMissing() []string {
	var missing []string
	if !hasCertifiedFactRef(s.Investigation.CertifiedFactRefs, s.CertifiedFacts) {
		missing = append(missing, "investigation.certified_fact_refs referencing verified certified_facts")
	}
	if !hasObservedContract(s.Investigation.ObservedContracts, s.CertifiedFacts) {
		missing = append(missing, "investigation.observed_contracts with source, evidence, and verified fact_refs")
	}
	if !hasAcceptanceCheck(s.Investigation.AcceptanceChecks, s.CertifiedFacts) {
		missing = append(missing, "investigation.acceptance_checks with description, command or expected result, and verified fact_refs")
	}
	if !s.Investigation.ReadyForChanges {
		missing = append(missing, "investigation.ready_for_changes=true")
	}
	return missing
}

func (s *HandoffState) AddCertifiedFact(f CertifiedFact) {
	if s.CertifiedFacts == nil {
		s.CertifiedFacts = make(map[string]CertifiedFact)
	}
	f.Verified = true
	s.CertifiedFacts[f.ID] = f.DeepCopy()
}

func (f CertifiedFact) DeepCopy() CertifiedFact {
	cp := f
	cp.Fields = copyStrings(f.Fields)
	cp.Metadata = copyJSONMap(f.Metadata)
	return cp
}

func (s HandoffState) AllowsChanges() bool {
	return len(s.ChangeGateMissing()) == 0
}

func (s HandoffState) MarshalJSON() ([]byte, error) {
	type alias HandoffState
	base, err := json.Marshal(alias(s))
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(base, &doc); err != nil {
		return nil, err
	}
	delete(doc, "Extra")
	for k, v := range s.Extra {
		doc[k] = v
	}
	return json.Marshal(doc)
}

func (s *HandoffState) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var out HandoffState
	extra := map[string]any{}
	unmarshalKnown(raw, extra, "schema_version", &out.SchemaVersion)
	unmarshalKnown(raw, extra, "goal", &out.Goal)
	unmarshalKnown(raw, extra, "invariants", &out.Invariants)
	unmarshalKnown(raw, extra, "current_focus", &out.CurrentFocus)
	unmarshalKnown(raw, extra, "investigation", &out.Investigation)
	unmarshalKnown(raw, extra, "certified_facts", &out.CertifiedFacts)
	unmarshalKnown(raw, extra, "todos", &out.Todos)
	unmarshalKnown(raw, extra, "completed", &out.Completed)
	unmarshalKnown(raw, extra, "latest_tool_result_interpretation", &out.LatestToolResultInterpretation)
	unmarshalKnown(raw, extra, "next_action", &out.NextAction)
	unmarshalKnown(raw, extra, "recent_actions", &out.RecentActions)
	unmarshalKnown(raw, extra, "open_questions", &out.OpenQuestions)
	unmarshalKnown(raw, extra, "do_not", &out.DoNot)
	unmarshalKnown(raw, extra, "evidence", &out.Evidence)
	unmarshalKnown(raw, extra, "decisions", &out.Decisions)
	unmarshalKnown(raw, extra, "files", &out.Files)
	unmarshalKnown(raw, extra, "risks", &out.Risks)
	for k, v := range raw {
		if _, reserved := handoffStateReservedFields[k]; reserved {
			continue
		}
		var decoded any
		if err := json.Unmarshal(v, &decoded); err == nil {
			extra[k] = decoded
		}
	}
	if len(extra) > 0 {
		out.Extra = extra
	}
	*s = out
	return nil
}

func (f HandoffFiles) MarshalJSON() ([]byte, error) {
	type alias HandoffFiles
	base, err := json.Marshal(alias(f))
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(base, &doc); err != nil {
		return nil, err
	}
	delete(doc, "Extra")
	for k, v := range f.Extra {
		doc[k] = v
	}
	return json.Marshal(doc)
}

func (f *HandoffFiles) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var out HandoffFiles
	extra := map[string]any{}
	unmarshalKnown(raw, extra, "read", &out.Read)
	unmarshalKnown(raw, extra, "changed", &out.Changed)
	for k, v := range raw {
		if _, reserved := handoffFilesReservedFields[k]; reserved {
			continue
		}
		var decoded any
		if err := json.Unmarshal(v, &decoded); err == nil {
			extra[k] = decoded
		}
	}
	if len(extra) > 0 {
		out.Extra = extra
	}
	*f = out
	return nil
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
		if targetsCertifiedFacts(op.Path) {
			continue
		}
		applyPatchOp(&doc, op)
	}

	doc = objectDoc(doc)
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
		"todos",
		"completed",
		"recent_actions",
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
	investigation, ok := root["investigation"].(map[string]any)
	if !ok {
		investigation = map[string]any{}
		root["investigation"] = investigation
	}
	for _, key := range []string{"observed_contracts", "acceptance_checks", "certified_fact_refs", "blockers"} {
		if _, ok := investigation[key]; !ok {
			investigation[key] = []any{}
		}
	}
	if _, ok := investigation["ready_for_changes"]; !ok {
		investigation["ready_for_changes"] = false
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

func applyPatchOp(doc *any, op HandoffPatchOp) {
	path := parsePatchPath(op.Path)
	if isRemoveOp(op.Op) {
		removePatchPath(doc, path)
		return
	}
	var value any
	if len(op.Value) > 0 {
		if err := json.Unmarshal(op.Value, &value); err != nil {
			value = string(op.Value)
		}
	}
	setPatchPath(doc, path, value)
}

func parsePatchPath(path string) []string {
	if path == "" {
		return nil
	}
	if path[0] != '/' {
		return []string{path}
	}
	raw := strings.Split(path[1:], "/")
	out := make([]string, len(raw))
	for i, p := range raw {
		p = strings.ReplaceAll(p, "~1", "/")
		p = strings.ReplaceAll(p, "~0", "~")
		out[i] = p
	}
	return out
}

func targetsCertifiedFacts(path string) bool {
	parts := parsePatchPath(path)
	return len(parts) > 0 && parts[0] == "certified_facts"
}

func isRemoveOp(op string) bool {
	switch strings.ToLower(op) {
	case "remove", "delete", "unset":
		return true
	default:
		return false
	}
}

func objectDoc(doc any) any {
	if _, ok := doc.(map[string]any); ok {
		return doc
	}
	return map[string]any{"value": doc}
}

func setPatchPath(target *any, parts []string, value any) {
	if len(parts) == 0 {
		*target = value
		return
	}
	first := parts[0]
	switch cur := (*target).(type) {
	case map[string]any:
		if len(parts) == 1 {
			cur[first] = value
			return
		}
		next, ok := cur[first]
		if !ok || !isPatchContainer(next) {
			next = newPatchContainer(parts[1])
		}
		setPatchPath(&next, parts[1:], value)
		cur[first] = next
	case []any:
		idx := flexibleArrayIndex(first, len(cur))
		for len(cur) <= idx {
			cur = append(cur, nil)
		}
		if len(parts) == 1 {
			cur[idx] = value
			*target = cur
			return
		}
		next := cur[idx]
		if !isPatchContainer(next) {
			next = newPatchContainer(parts[1])
		}
		setPatchPath(&next, parts[1:], value)
		cur[idx] = next
		*target = cur
	default:
		next := newPatchContainer(first)
		*target = next
		setPatchPath(target, parts, value)
	}
}

func removePatchPath(target *any, parts []string) {
	if len(parts) == 0 {
		*target = map[string]any{}
		return
	}
	first := parts[0]
	switch cur := (*target).(type) {
	case map[string]any:
		if len(parts) == 1 {
			delete(cur, first)
			return
		}
		next, ok := cur[first]
		if !ok {
			return
		}
		removePatchPath(&next, parts[1:])
		cur[first] = next
	case []any:
		idx, ok := existingArrayIndex(first, len(cur))
		if !ok {
			return
		}
		if len(parts) == 1 {
			cur = append(cur[:idx], cur[idx+1:]...)
			*target = cur
			return
		}
		next := cur[idx]
		removePatchPath(&next, parts[1:])
		cur[idx] = next
		*target = cur
	}
}

func isPatchContainer(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func newPatchContainer(nextPart string) any {
	if nextPart == "-" {
		return []any{}
	}
	if _, err := strconv.Atoi(nextPart); err == nil {
		return []any{}
	}
	return map[string]any{}
}

func flexibleArrayIndex(part string, length int) int {
	if part == "-" {
		return length
	}
	idx, err := strconv.Atoi(part)
	if err != nil || idx < 0 {
		return length
	}
	return idx
}

func existingArrayIndex(part string, length int) (int, bool) {
	idx, err := strconv.Atoi(part)
	if err != nil || idx < 0 || idx >= length {
		return 0, false
	}
	return idx, true
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyHandoffTodos(in []HandoffTodo) []HandoffTodo {
	if len(in) == 0 {
		return nil
	}
	out := make([]HandoffTodo, len(in))
	copy(out, in)
	return out
}

func copyObservedContracts(in []HandoffObservedContract) []HandoffObservedContract {
	if len(in) == 0 {
		return nil
	}
	out := make([]HandoffObservedContract, len(in))
	copy(out, in)
	for i := range out {
		out[i].Fields = copyStrings(in[i].Fields)
		out[i].FactRefs = copyStrings(in[i].FactRefs)
	}
	return out
}

func copyAcceptanceChecks(in []HandoffAcceptanceCheck) []HandoffAcceptanceCheck {
	if len(in) == 0 {
		return nil
	}
	out := make([]HandoffAcceptanceCheck, len(in))
	copy(out, in)
	for i := range out {
		out[i].FactRefs = copyStrings(in[i].FactRefs)
	}
	return out
}

func copyCertifiedFacts(in map[string]CertifiedFact) map[string]CertifiedFact {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]CertifiedFact, len(in))
	for k, v := range in {
		out[k] = v.DeepCopy()
	}
	return out
}

func hasCertifiedFactRef(refs []string, facts map[string]CertifiedFact) bool {
	for _, ref := range refs {
		if f, ok := facts[ref]; ok && f.Verified {
			return true
		}
	}
	return false
}

func hasObservedContract(contracts []HandoffObservedContract, facts map[string]CertifiedFact) bool {
	for _, c := range contracts {
		if strings.TrimSpace(c.Source) == "" || strings.TrimSpace(c.Evidence) == "" {
			continue
		}
		if hasCertifiedFactRef(c.FactRefs, facts) {
			return true
		}
	}
	return false
}

func hasAcceptanceCheck(checks []HandoffAcceptanceCheck, facts map[string]CertifiedFact) bool {
	for _, c := range checks {
		if strings.TrimSpace(c.Description) == "" {
			continue
		}
		if strings.TrimSpace(c.Command) == "" && strings.TrimSpace(c.Expected) == "" {
			continue
		}
		if hasCertifiedFactRef(c.FactRefs, facts) {
			return true
		}
	}
	return false
}

func copyJSONMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		out := make(map[string]any, len(in))
		for k, v := range in {
			out[k] = v
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		out = make(map[string]any, len(in))
		for k, v := range in {
			out[k] = v
		}
	}
	return out
}

func unmarshalKnown(raw map[string]json.RawMessage, extra map[string]any, key string, target any) {
	data, ok := raw[key]
	if !ok {
		return
	}
	if err := json.Unmarshal(data, target); err == nil {
		return
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err == nil {
		extra[key] = decoded
	}
}

var handoffStateReservedFields = map[string]struct{}{
	"schema_version":                    {},
	"goal":                              {},
	"invariants":                        {},
	"current_focus":                     {},
	"investigation":                     {},
	"certified_facts":                   {},
	"todos":                             {},
	"completed":                         {},
	"latest_tool_result_interpretation": {},
	"next_action":                       {},
	"recent_actions":                    {},
	"open_questions":                    {},
	"do_not":                            {},
	"evidence":                          {},
	"decisions":                         {},
	"files":                             {},
	"risks":                             {},
}

var handoffFilesReservedFields = map[string]struct{}{
	"read":    {},
	"changed": {},
}
