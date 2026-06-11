package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/provider/rawcapture"
)

type inspectRawHTTPInputKind string

const (
	inspectRawHTTPInputCapture inspectRawHTTPInputKind = "capture"
	inspectRawHTTPInputDump    inspectRawHTTPInputKind = "dump"
)

type inspectRawHTTPInput struct {
	Kind inspectRawHTTPInputKind
	Root string
}

type inspectRawHTTPTurn struct {
	Turn             string   `json:"turn"`
	SourceDir        string   `json:"source_dir"`
	RequestPath      string   `json:"request_path"`
	ResponsePath     string   `json:"response_path,omitempty"`
	StartedAt        string   `json:"started_at,omitempty"`
	CompletedAt      string   `json:"completed_at,omitempty"`
	Status           string   `json:"status,omitempty"`
	StatusCode       int      `json:"status_code,omitempty"`
	Model            string   `json:"model,omitempty"`
	RequestBytes     int64    `json:"request_bytes,omitempty"`
	ResponseBytes    int64    `json:"response_bytes,omitempty"`
	MessageCount     int      `json:"message_count,omitempty"`
	ToolCount        int      `json:"tool_count,omitempty"`
	ResponseKind     string   `json:"response_kind,omitempty"`
	FinishReason     string   `json:"finish_reason,omitempty"`
	Usage            string   `json:"usage,omitempty"`
	FinalUserPreview string   `json:"final_user_preview,omitempty"`
	ReasoningPreview string   `json:"reasoning_preview,omitempty"`
	ContentPreview   string   `json:"content_preview,omitempty"`
	ToolCalls        []string `json:"tool_calls,omitempty"`
	ErrorPreview     string   `json:"error_preview,omitempty"`
	requestText      string
	finalUser        string
	reasoning        string
	content          string
	errorText        string
}

func inspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect captured Pragma artifacts",
	}
	cmd.AddCommand(inspectRawHTTPCmd())
	cmd.AddCommand(inspectPhasesCmd())
	return cmd
}

func inspectPhasesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "phases <run-dir|capture-dir|turn-payloads-dir>",
		Short: "Show turn-to-orchestration-phase correspondence",
		Args:  cobra.ExactArgs(1),
		RunE:  inspectPhasesRun,
	}
	cmd.Flags().String("format", "markdown", "output format: markdown, json, or tsv")
	return cmd
}

func inspectPhasesRun(cmd *cobra.Command, args []string) error {
	input, err := resolveInspectRawHTTPInput(args[0])
	if err != nil {
		return err
	}
	turns, err := loadInspectRawHTTPTurns(input)
	if err != nil {
		return err
	}

	report := buildInspectPhaseReport(args[0], input, turns)
	format, _ := cmd.Flags().GetString("format")
	switch format {
	case "", "markdown":
		return writeInspectPhaseMarkdown(cmd.OutOrStdout(), report)
	case "json":
		return writeInspectPhaseJSON(cmd.OutOrStdout(), report)
	case "tsv":
		return writeInspectPhaseTSV(cmd.OutOrStdout(), report)
	default:
		return fmt.Errorf("unsupported inspect format %q; expected markdown, json, or tsv", format)
	}
}

func inspectRawHTTPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raw-http <run-dir|capture-dir|turn-payloads-dir>",
		Short: "Summarize raw HTTP conversation payloads",
		Args:  cobra.ExactArgs(1),
		RunE:  inspectRawHTTPRun,
	}
	cmd.Flags().String("format", "markdown", "output format: markdown, json, or tsv")
	cmd.Flags().String("turn", "", "print a detailed report for one turn number")
	cmd.Flags().Bool("errors", false, "show only failed, malformed, or non-200 turns")
	cmd.Flags().Bool("tools", false, "show only turns with assistant tool calls")
	cmd.Flags().Int("limit", 0, "limit timeline rows (0 means no limit)")
	return cmd
}

func inspectRawHTTPRun(cmd *cobra.Command, args []string) error {
	input, err := resolveInspectRawHTTPInput(args[0])
	if err != nil {
		return err
	}
	turns, err := loadInspectRawHTTPTurns(input)
	if err != nil {
		return err
	}

	turnFilter, _ := cmd.Flags().GetString("turn")
	onlyErrors, _ := cmd.Flags().GetBool("errors")
	onlyTools, _ := cmd.Flags().GetBool("tools")
	limit, _ := cmd.Flags().GetInt("limit")
	format, _ := cmd.Flags().GetString("format")

	if turnFilter != "" {
		turn, ok := findInspectRawHTTPTurn(turns, turnFilter)
		if !ok {
			return fmt.Errorf("turn %s not found", turnFilter)
		}
		return writeInspectRawHTTPTurnReport(cmd.OutOrStdout(), turn, format)
	}

	filtered := filterInspectRawHTTPTurns(turns, onlyErrors, onlyTools, limit)
	switch format {
	case "", "markdown":
		return writeInspectRawHTTPMarkdown(cmd.OutOrStdout(), input, turns, filtered)
	case "json":
		return writeInspectRawHTTPJSON(cmd.OutOrStdout(), filtered)
	case "tsv":
		return writeInspectRawHTTPTSV(cmd.OutOrStdout(), filtered)
	default:
		return fmt.Errorf("unsupported inspect format %q; expected markdown, json, or tsv", format)
	}
}

func resolveInspectRawHTTPInput(input string) (inspectRawHTTPInput, error) {
	info, err := os.Stat(input)
	if err != nil {
		return inspectRawHTTPInput{}, err
	}
	if !info.IsDir() {
		return inspectRawHTTPInput{}, fmt.Errorf("%s is not a directory", input)
	}
	if isInspectRawHTTPDumpDir(input) {
		return inspectRawHTTPInput{Kind: inspectRawHTTPInputDump, Root: input}, nil
	}
	captureDir, err := resolveRawHTTPCaptureDir(input)
	if err != nil {
		return inspectRawHTTPInput{}, err
	}
	return inspectRawHTTPInput{Kind: inspectRawHTTPInputCapture, Root: captureDir}, nil
}

func isInspectRawHTTPDumpDir(input string) bool {
	if _, err := os.Stat(filepath.Join(input, "index.tsv")); err == nil {
		return true
	}
	entries, err := os.ReadDir(input)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "turn-") {
			return true
		}
	}
	return false
}

func loadInspectRawHTTPTurns(input inspectRawHTTPInput) ([]inspectRawHTTPTurn, error) {
	var dirs []string
	var err error
	switch input.Kind {
	case inspectRawHTTPInputDump:
		dirs, err = inspectRawHTTPDumpTurnDirs(input.Root)
	case inspectRawHTTPInputCapture:
		dirs, err = rawHTTPTurnDirs(input.Root)
	default:
		err = fmt.Errorf("unsupported raw HTTP input kind %q", input.Kind)
	}
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no raw HTTP turns found in %s", input.Root)
	}
	turns := make([]inspectRawHTTPTurn, 0, len(dirs))
	for _, dir := range dirs {
		turn, err := loadInspectRawHTTPTurn(input, dir)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func inspectRawHTTPDumpTurnDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "turn-") {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return inspectRawHTTPTurnSortKey(filepath.Base(dirs[i])) < inspectRawHTTPTurnSortKey(filepath.Base(dirs[j]))
	})
	return dirs, nil
}

func loadInspectRawHTTPTurn(input inspectRawHTTPInput, dir string) (inspectRawHTTPTurn, error) {
	turn := inspectRawHTTPTurn{
		SourceDir: filepath.ToSlash(dir),
	}
	switch input.Kind {
	case inspectRawHTTPInputDump:
		turn.Turn = strings.TrimPrefix(filepath.Base(dir), "turn-")
	case inspectRawHTTPInputCapture:
		turn.Turn = rawHTTPTurnNumber(filepath.Base(dir))
	}

	requestPath := filepath.Join(dir, "request.json")
	turn.RequestPath = filepath.ToSlash(requestPath)
	reqInfo, err := os.Stat(requestPath)
	if err != nil {
		return turn, fmt.Errorf("inspect turn %s request.json: %w", turn.Turn, err)
	}
	turn.RequestBytes = reqInfo.Size()

	var req map[string]any
	if err := readJSONFile(requestPath, &req); err != nil {
		return turn, fmt.Errorf("inspect turn %s request.json: %w", turn.Turn, err)
	}
	turn.Model, _ = req["model"].(string)
	turn.MessageCount = len(asSlice(req["messages"]))
	turn.ToolCount = len(asSlice(req["tools"]))
	turn.requestText = inspectRawHTTPRequestText(req)
	turn.finalUser = inspectRawHTTPFinalUser(req)
	turn.FinalUserPreview = trimInspectPreview(turn.finalUser)

	reqMeta := readInspectRawHTTPMeta(filepath.Join(dir, "request.meta.json"))
	respMeta := readInspectRawHTTPMeta(filepath.Join(dir, "response.meta.json"))
	turn.StartedAt = firstNonEmptyRawHTTPDump(respMeta.string("started_at"), reqMeta.string("started_at"))
	turn.CompletedAt = respMeta.string("completed_at")
	turn.Status = firstNonEmptyRawHTTPDump(respMeta.string("status"), inspectRawHTTPStatusFromCode(respMeta.int("status_code")))
	turn.StatusCode = respMeta.int("status_code")
	if v := reqMeta.int64("request_bytes"); v > 0 {
		turn.RequestBytes = v
	}
	if v := respMeta.int64("response_bytes"); v > 0 {
		turn.ResponseBytes = v
	}

	var body []byte
	responsePath := ""
	if input.Kind == inspectRawHTTPInputCapture {
		evidence, err := rawcapture.ReadResponseEvidence(dir)
		if err != nil {
			return turn, fmt.Errorf("inspect turn %s response evidence: %w", turn.Turn, err)
		}
		if evidence.Meta.StartedAt != "" {
			turn.StartedAt = firstNonEmptyRawHTTPDump(evidence.Meta.StartedAt, reqMeta.string("started_at"))
		}
		turn.CompletedAt = evidence.Meta.CompletedAt
		turn.Status = firstNonEmptyRawHTTPDump(evidence.Meta.Status, inspectRawHTTPStatusFromCode(evidence.Meta.StatusCode))
		turn.StatusCode = evidence.Meta.StatusCode
		if evidence.Meta.ResponseBytes > 0 {
			turn.ResponseBytes = evidence.Meta.ResponseBytes
		} else if evidence.FileBytes > 0 {
			turn.ResponseBytes = evidence.FileBytes
		}
		if !evidence.Complete {
			turn.ResponseKind = rawHTTPResponseEvidenceStatus(evidence)
			turn.Status = turn.ResponseKind
			if evidence.RawPath != "" {
				turn.ResponsePath = filepath.ToSlash(evidence.RawPath)
			}
			turn.errorText = summarizeRawHTTPResponseEvidenceProblem(evidence)
			turn.ErrorPreview = trimInspectPreview(turn.errorText)
			return turn, nil
		}
		responsePath = evidence.RawPath
		body = evidence.Body
	} else {
		responsePath = inspectRawHTTPResponsePath(dir)
		if responsePath != "" {
			body, err = os.ReadFile(responsePath)
			if err != nil {
				return turn, err
			}
		}
	}
	if responsePath == "" {
		turn.ResponseKind = "missing"
		return turn, nil
	}
	turn.ResponsePath = filepath.ToSlash(responsePath)
	if turn.ResponseBytes == 0 {
		if info, err := os.Stat(responsePath); err == nil {
			turn.ResponseBytes = info.Size()
		}
	}
	applyInspectRawHTTPResponse(&turn, body)
	return turn, nil
}

type inspectRawHTTPMeta map[string]any

func readInspectRawHTTPMeta(path string) inspectRawHTTPMeta {
	var meta map[string]any
	_ = readJSONFile(path, &meta)
	return inspectRawHTTPMeta(meta)
}

func (m inspectRawHTTPMeta) string(key string) string {
	if m == nil {
		return ""
	}
	value, _ := m[key].(string)
	return value
}

func (m inspectRawHTTPMeta) int(key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := strconv.Atoi(v.String())
		return n
	default:
		return 0
	}
}

func (m inspectRawHTTPMeta) int64(key string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := strconv.ParseInt(v.String(), 10, 64)
		return n
	default:
		return 0
	}
}

func inspectRawHTTPResponsePath(dir string) string {
	for _, name := range []string{"response.json", "response.raw"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func applyInspectRawHTTPResponse(turn *inspectRawHTTPTurn, body []byte) {
	if len(strings.TrimSpace(string(body))) == 0 {
		turn.ResponseKind = "empty"
		return
	}
	if isRawHTTPSSE(body) {
		turn.ResponseKind = "sse"
		stream := parseRawHTTPSSE(body)
		turn.FinishReason = strings.Join(stream.Finish, ", ")
		if len(stream.Usage) > 0 {
			turn.Usage = rawHTTPUsageSummary(map[string]any{"usage": stream.Usage})
		}
		turn.reasoning = strings.TrimSpace(stream.Reasoning.String())
		turn.content = strings.TrimSpace(stream.Content.String())
		turn.ReasoningPreview = trimInspectPreview(turn.reasoning)
		turn.ContentPreview = trimInspectPreview(turn.content)
		turn.ToolCalls = inspectRawHTTPStreamToolNames(stream)
		if stream.ParseError != nil {
			turn.errorText = stream.ParseError.Error()
			turn.ErrorPreview = turn.errorText
		}
		return
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		turn.ResponseKind = "non_json"
		turn.ErrorPreview = trimInspectPreview(string(body))
		return
	}
	turn.ResponseKind = "json"
	if errValue, ok := obj["error"]; ok {
		turn.errorText = readableContent(errValue)
		turn.ErrorPreview = trimInspectPreview(turn.errorText)
	}
	turn.FinishReason = rawHTTPFinishReason(obj)
	turn.Usage = rawHTTPUsageSummary(obj)
	turn.reasoning = strings.TrimSpace(renderRawHTTPReasoning(obj))
	turn.ReasoningPreview = trimInspectPreview(turn.reasoning)
	if content, err := renderRawHTTPContent(body); err == nil {
		turn.content = strings.TrimSpace(string(content))
		turn.ContentPreview = trimInspectPreview(turn.content)
	}
	turn.ToolCalls = inspectRawHTTPJSONToolNames(obj)
}

func inspectRawHTTPFinalUser(req map[string]any) string {
	messages := asSlice(req["messages"])
	for i := len(messages) - 1; i >= 0; i-- {
		msg, _ := messages[i].(map[string]any)
		role, _ := msg["role"].(string)
		if role != "user" {
			continue
		}
		return strings.TrimSpace(readableContent(msg["content"]))
	}
	return ""
}

func inspectRawHTTPRequestText(req map[string]any) string {
	var parts []string
	for _, item := range asSlice(req["messages"]) {
		msg, _ := item.(map[string]any)
		role, _ := msg["role"].(string)
		content := strings.TrimSpace(readableContent(msg["content"]))
		if content == "" {
			continue
		}
		parts = append(parts, role+"\n"+content)
	}
	return strings.Join(parts, "\n\n")
}

type inspectPhaseReport struct {
	Input         string                     `json:"input"`
	Kind          inspectRawHTTPInputKind    `json:"kind"`
	Source        string                     `json:"source"`
	TurnCount     int                        `json:"turn_count"`
	Ranges        []inspectPhaseRange        `json:"ranges"`
	ControlEvents []inspectPhaseControlEvent `json:"control_events,omitempty"`
	Turns         []inspectPhaseTurn         `json:"turns"`
	Warnings      []string                   `json:"warnings,omitempty"`
}

type inspectPhaseTurn struct {
	Turn      string `json:"turn"`
	Phase     string `json:"phase"`
	StartedAt string `json:"started_at,omitempty"`
	Preview   string `json:"preview,omitempty"`
	Inferred  bool   `json:"inferred,omitempty"`
}

type inspectPhaseRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Phase string `json:"phase"`
	Count int    `json:"count"`
}

type inspectPhaseControlEvent struct {
	State      string `json:"state"`
	Kind       string `json:"kind,omitempty"`
	Event      string `json:"event,omitempty"`
	Transition string `json:"transition,omitempty"`
}

func buildInspectPhaseReport(arg string, input inspectRawHTTPInput, rawTurns []inspectRawHTTPTurn) inspectPhaseReport {
	report := inspectPhaseReport{
		Input:     filepath.ToSlash(arg),
		Kind:      input.Kind,
		Source:    "artifact_contracts",
		TurnCount: len(rawTurns),
	}
	stdoutBody := ""
	var stdoutMarkers []inspectPhaseStdoutMarker
	if stdoutPath := inspectPhaseStdoutPath(arg, input); stdoutPath != "" {
		report.Source = "orchestration_stdout+request_prompts"
		report.ControlEvents = inspectPhaseControlEvents(stdoutPath)
		stdoutBody = readInspectPhaseStdout(stdoutPath)
		stdoutMarkers = inspectPhaseStdoutMarkers(stdoutBody)
	}
	stdoutCursor := 0
	for _, raw := range rawTurns {
		phase := ""
		if stdoutBody != "" && len(stdoutMarkers) > 0 {
			var ok bool
			phase, stdoutCursor, ok = inferInspectPhaseFromStdout(raw, stdoutBody, stdoutMarkers, stdoutCursor)
			if !ok {
				phase = ""
			}
		}
		if phase == "" {
			phase = inferInspectPhaseFromRequest(raw)
		}
		if phase == "" {
			phase = "unknown"
		}
		report.Turns = append(report.Turns, inspectPhaseTurn{
			Turn:      raw.Turn,
			Phase:     phase,
			StartedAt: raw.StartedAt,
			Preview:   firstNonEmptyRawHTTPDump(raw.ContentPreview, raw.ReasoningPreview, raw.FinalUserPreview),
			Inferred:  true,
		})
	}
	report.Ranges = inspectPhaseRanges(report.Turns)
	if hasInspectPhase(report.Turns, "unknown") {
		report.Warnings = append(report.Warnings, "some turns could not be mapped to an orchestration state from stdout or request prompts")
	}
	return report
}

type inspectPhaseStdoutMarker struct {
	Offset int
	Phase  string
}

func readInspectPhaseStdout(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(body)
}

func inspectPhaseStdoutMarkers(body string) []inspectPhaseStdoutMarker {
	var markers []inspectPhaseStdoutMarker
	offset := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[orchestration: ") && strings.Contains(trimmed, " persona=") {
			inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "[orchestration: "), "]")
			state := strings.Fields(inner)
			if len(state) > 0 {
				markers = append(markers, inspectPhaseStdoutMarker{Offset: offset, Phase: state[0]})
			}
		}
		offset += len(line)
	}
	return markers
}

func inferInspectPhaseFromStdout(turn inspectRawHTTPTurn, body string, markers []inspectPhaseStdoutMarker, cursor int) (string, int, bool) {
	needle := inspectPhaseStdoutNeedle(turn)
	if needle == "" || cursor >= len(body) {
		return "", cursor, false
	}
	relative := strings.Index(body[cursor:], needle)
	if relative < 0 {
		relative = strings.Index(body, needle)
		if relative < 0 {
			return "", cursor, false
		}
	} else {
		relative += cursor
	}
	phase := inspectPhaseAtStdoutOffset(markers, relative)
	if phase == "" {
		return "", cursor, false
	}
	return phase, relative + len(needle), true
}

func inspectPhaseStdoutNeedle(turn inspectRawHTTPTurn) string {
	for _, candidate := range []string{turn.content, turn.reasoning, turn.ContentPreview, turn.ReasoningPreview} {
		candidate = strings.TrimSpace(candidate)
		if len(candidate) >= 16 {
			return candidate
		}
	}
	return ""
}

func inspectPhaseAtStdoutOffset(markers []inspectPhaseStdoutMarker, offset int) string {
	phase := ""
	for _, marker := range markers {
		if marker.Offset > offset {
			break
		}
		phase = marker.Phase
	}
	return phase
}

func inferInspectPhaseFromRequest(turn inspectRawHTTPTurn) string {
	text := turn.requestText
	if phase := inspectPhasePromptRole(text, "You are `", "`"); phase != "" {
		return phase
	}
	if phase := inspectPhasePromptRole(text, "You are the ", "\n"); phase != "" {
		return strings.ReplaceAll(strings.Trim(strings.TrimSuffix(phase, "."), " "), " ", "_")
	}
	return ""
}

func inspectPhasePromptRole(text, prefix, suffix string) string {
	start := strings.Index(text, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(text[start:], suffix)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(text[start : start+end])
}

func inspectPhaseRanges(turns []inspectPhaseTurn) []inspectPhaseRange {
	if len(turns) == 0 {
		return nil
	}
	ranges := []inspectPhaseRange{{
		Start: turns[0].Turn,
		End:   turns[0].Turn,
		Phase: turns[0].Phase,
		Count: 1,
	}}
	for _, turn := range turns[1:] {
		last := &ranges[len(ranges)-1]
		if turn.Phase == last.Phase {
			last.End = turn.Turn
			last.Count++
			continue
		}
		ranges = append(ranges, inspectPhaseRange{Start: turn.Turn, End: turn.Turn, Phase: turn.Phase, Count: 1})
	}
	return ranges
}

func hasInspectPhase(turns []inspectPhaseTurn, phase string) bool {
	for _, turn := range turns {
		if turn.Phase == phase {
			return true
		}
	}
	return false
}

func inspectPhaseStdoutPath(arg string, input inspectRawHTTPInput) string {
	candidates := []string{filepath.Join(arg, "pragma.stdout.log")}
	if filepath.Base(input.Root) == "raw-http-pragma" {
		candidates = append(candidates, filepath.Join(filepath.Dir(input.Root), "pragma.stdout.log"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func inspectPhaseControlEvents(stdoutPath string) []inspectPhaseControlEvent {
	body, err := os.ReadFile(stdoutPath)
	if err != nil {
		return nil
	}
	var events []inspectPhaseControlEvent
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[control: ") && strings.Contains(line, " (") {
			inner := strings.TrimSuffix(strings.TrimPrefix(line, "[control: "), "]")
			state, kind, _ := strings.Cut(inner, " (")
			events = append(events, inspectPhaseControlEvent{
				State: strings.TrimSpace(state),
				Kind:  strings.TrimSuffix(strings.TrimSpace(kind), ")"),
			})
			continue
		}
		if strings.HasPrefix(line, "[control: ") && strings.Contains(line, " emitted ") {
			inner := strings.TrimSuffix(strings.TrimPrefix(line, "[control: "), "]")
			state, event, _ := strings.Cut(inner, " emitted ")
			if len(events) > 0 && events[len(events)-1].State == strings.TrimSpace(state) {
				events[len(events)-1].Event = strings.TrimSpace(event)
			}
			continue
		}
		if strings.HasPrefix(line, "[transition: ") {
			transition := strings.TrimSuffix(strings.TrimPrefix(line, "[transition: "), "]")
			if len(events) > 0 && events[len(events)-1].Transition == "" {
				events[len(events)-1].Transition = transition
			}
		}
	}
	return events
}

func writeInspectPhaseMarkdown(w io.Writer, report inspectPhaseReport) error {
	fmt.Fprintln(w, "# Turn Phase Correspondence")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Input: `%s`\n", report.Input)
	fmt.Fprintf(w, "Kind: %s\n", report.Kind)
	fmt.Fprintf(w, "Source: %s\n", report.Source)
	fmt.Fprintf(w, "Turns: %d\n", report.TurnCount)
	for _, warning := range report.Warnings {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Phase Ranges")
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "RANGE\tPHASE\tTURNS")
	for _, r := range report.Ranges {
		fmt.Fprintf(tw, "%s-%s\t%s\t%d\n", r.Start, r.End, r.Phase, r.Count)
	}
	tw.Flush()

	if len(report.ControlEvents) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Control Events")
		fmt.Fprintln(w)
		tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "STATE\tKIND\tEVENT\tTRANSITION")
		for _, event := range report.ControlEvents {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", event.State, event.Kind, event.Event, event.Transition)
		}
		tw.Flush()
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Turns")
	fmt.Fprintln(w)
	tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "TURN\tPHASE\tTIME\tPREVIEW")
	for _, turn := range report.Turns {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", turn.Turn, turn.Phase, shortInspectTime(turn.StartedAt), turn.Preview)
	}
	return tw.Flush()
}

func writeInspectPhaseJSON(w io.Writer, report inspectPhaseReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func writeInspectPhaseTSV(w io.Writer, report inspectPhaseReport) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "turn\tphase\tstarted_at\tpreview")
	for _, turn := range report.Turns {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", turn.Turn, turn.Phase, turn.StartedAt, turn.Preview)
	}
	return tw.Flush()
}

func inspectRawHTTPStreamToolNames(stream rawHTTPStreamResponse) []string {
	var names []string
	for _, index := range stream.ToolOrder {
		tc := stream.ToolCalls[index]
		if tc == nil {
			continue
		}
		names = append(names, firstNonEmptyRawHTTPDump(tc.Name, tc.ID, fmt.Sprintf("tool[%d]", tc.Index)))
	}
	return names
}

func inspectRawHTTPJSONToolNames(obj map[string]any) []string {
	var names []string
	for _, choice := range asSlice(obj["choices"]) {
		ch, _ := choice.(map[string]any)
		msg, _ := ch["message"].(map[string]any)
		for _, item := range asSlice(msg["tool_calls"]) {
			call, _ := item.(map[string]any)
			function, _ := call["function"].(map[string]any)
			name, _ := function["name"].(string)
			id, _ := call["id"].(string)
			names = append(names, firstNonEmptyRawHTTPDump(name, id, "tool"))
		}
	}
	return names
}

func filterInspectRawHTTPTurns(turns []inspectRawHTTPTurn, onlyErrors, onlyTools bool, limit int) []inspectRawHTTPTurn {
	var out []inspectRawHTTPTurn
	for _, turn := range turns {
		if onlyErrors && !inspectRawHTTPTurnHasError(turn) {
			continue
		}
		if onlyTools && len(turn.ToolCalls) == 0 {
			continue
		}
		out = append(out, turn)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func inspectRawHTTPTurnHasError(turn inspectRawHTTPTurn) bool {
	if turn.StatusCode >= 400 {
		return true
	}
	switch turn.ResponseKind {
	case "missing", "empty", "non_json", "missing_response", "missing_response_meta", "invalid_response_meta", "incomplete_response", "response_transport_error", "capture_write_error", "response_byte_mismatch", "missing_response_sha256", "response_sha256_mismatch":
		return true
	}
	return turn.ErrorPreview != ""
}

func findInspectRawHTTPTurn(turns []inspectRawHTTPTurn, want string) (inspectRawHTTPTurn, bool) {
	want = strings.TrimPrefix(want, "turn-")
	for _, turn := range turns {
		if trimInspectRawHTTPLeadingZeros(turn.Turn) == trimInspectRawHTTPLeadingZeros(want) || turn.Turn == want {
			return turn, true
		}
	}
	return inspectRawHTTPTurn{}, false
}

func writeInspectRawHTTPMarkdown(w io.Writer, input inspectRawHTTPInput, all, turns []inspectRawHTTPTurn) error {
	summary := summarizeInspectRawHTTP(all)
	fmt.Fprintln(w, "# Raw HTTP Conversation Inspect")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Input: `%s`\n", filepath.ToSlash(input.Root))
	fmt.Fprintf(w, "Kind: %s\n", input.Kind)
	fmt.Fprintf(w, "Turns: %d", summary.TotalTurns)
	if len(turns) != len(all) {
		fmt.Fprintf(w, " (%d shown)", len(turns))
	}
	fmt.Fprintln(w)
	if summary.StartedAt != "" || summary.CompletedAt != "" {
		fmt.Fprintf(w, "Time span: %s -> %s\n", summary.StartedAt, summary.CompletedAt)
	}
	if summary.ErrorTurns > 0 {
		fmt.Fprintf(w, "Errors: %d\n", summary.ErrorTurns)
	}
	if summary.ToolTurns > 0 {
		fmt.Fprintf(w, "Tool-call turns: %d\n", summary.ToolTurns)
	}
	if summary.Usage != "" {
		fmt.Fprintf(w, "Usage: %s\n", summary.Usage)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Timeline")
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "TURN\tTIME\tSTATUS\tMODEL\tFINISH\tTOOLS\tPREVIEW")
	for _, turn := range turns {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			turn.Turn,
			shortInspectTime(turn.StartedAt),
			firstNonEmptyRawHTTPDump(turn.Status, turn.ResponseKind),
			turn.Model,
			turn.FinishReason,
			strings.Join(turn.ToolCalls, ","),
			firstNonEmptyRawHTTPDump(turn.ContentPreview, turn.ReasoningPreview, turn.ErrorPreview),
		)
	}
	tw.Flush()

	significant := significantInspectRawHTTPTurns(turns)
	if len(significant) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Significant Turns")
		fmt.Fprintln(w)
		for _, turn := range significant {
			fmt.Fprintf(w, "- turn %s: %s\n", turn.Turn, inspectRawHTTPTurnSignificance(turn))
		}
	}
	return nil
}

type inspectRawHTTPSummary struct {
	TotalTurns  int
	ErrorTurns  int
	ToolTurns   int
	StartedAt   string
	CompletedAt string
	Usage       string
}

func summarizeInspectRawHTTP(turns []inspectRawHTTPTurn) inspectRawHTTPSummary {
	summary := inspectRawHTTPSummary{TotalTurns: len(turns)}
	usage := make(map[string]int)
	for i, turn := range turns {
		if i == 0 {
			summary.StartedAt = turn.StartedAt
		}
		if turn.CompletedAt != "" {
			summary.CompletedAt = turn.CompletedAt
		}
		if inspectRawHTTPTurnHasError(turn) {
			summary.ErrorTurns++
		}
		if len(turn.ToolCalls) > 0 {
			summary.ToolTurns++
		}
		addInspectUsage(usage, turn.Usage)
	}
	summary.Usage = formatInspectUsage(usage)
	return summary
}

func writeInspectRawHTTPJSON(w io.Writer, turns []inspectRawHTTPTurn) error {
	data, err := json.MarshalIndent(turns, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func writeInspectRawHTTPTSV(w io.Writer, turns []inspectRawHTTPTurn) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "turn\tstarted_at\tstatus\tmodel\tfinish_reason\ttools\tcontent_preview")
	for _, turn := range turns {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			turn.Turn,
			turn.StartedAt,
			turn.Status,
			turn.Model,
			turn.FinishReason,
			strings.Join(turn.ToolCalls, ","),
			turn.ContentPreview,
		)
	}
	return tw.Flush()
}

func writeInspectRawHTTPTurnReport(w io.Writer, turn inspectRawHTTPTurn, format string) error {
	switch format {
	case "", "markdown":
		fmt.Fprintf(w, "# Raw HTTP Turn %s\n\n", turn.Turn)
		fmt.Fprintf(w, "Request: `%s`\n", turn.RequestPath)
		if turn.ResponsePath != "" {
			fmt.Fprintf(w, "Response: `%s`\n", turn.ResponsePath)
		}
		fmt.Fprintf(w, "Status: %s\n", firstNonEmptyRawHTTPDump(turn.Status, turn.ResponseKind))
		fmt.Fprintf(w, "Model: %s\n", turn.Model)
		fmt.Fprintf(w, "Messages: %d\n", turn.MessageCount)
		fmt.Fprintf(w, "Tools in request: %d\n", turn.ToolCount)
		if turn.FinishReason != "" {
			fmt.Fprintf(w, "Finish reason: %s\n", turn.FinishReason)
		}
		if turn.Usage != "" {
			fmt.Fprintf(w, "Usage: %s\n", turn.Usage)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Final User Message")
		fmt.Fprintln(w)
		fmt.Fprintln(w, firstNonEmptyRawHTTPDump(turn.finalUser, "none"))
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Reasoning")
		fmt.Fprintln(w)
		fmt.Fprintln(w, firstNonEmptyRawHTTPDump(turn.reasoning, "none"))
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Assistant Content")
		fmt.Fprintln(w)
		fmt.Fprintln(w, firstNonEmptyRawHTTPDump(turn.content, "none"))
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Tool Calls")
		fmt.Fprintln(w)
		if len(turn.ToolCalls) == 0 {
			fmt.Fprintln(w, "none")
		} else {
			for _, name := range turn.ToolCalls {
				fmt.Fprintf(w, "- %s\n", name)
			}
		}
		if turn.ErrorPreview != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "## Error")
			fmt.Fprintln(w)
			fmt.Fprintln(w, firstNonEmptyRawHTTPDump(turn.errorText, turn.ErrorPreview))
		}
		return nil
	case "json":
		return writeInspectRawHTTPJSON(w, []inspectRawHTTPTurn{turn})
	case "tsv":
		return writeInspectRawHTTPTSV(w, []inspectRawHTTPTurn{turn})
	default:
		return fmt.Errorf("unsupported inspect format %q; expected markdown, json, or tsv", format)
	}
}

func significantInspectRawHTTPTurns(turns []inspectRawHTTPTurn) []inspectRawHTTPTurn {
	var out []inspectRawHTTPTurn
	for _, turn := range turns {
		if inspectRawHTTPTurnSignificance(turn) != "" {
			out = append(out, turn)
		}
	}
	return out
}

func inspectRawHTTPTurnSignificance(turn inspectRawHTTPTurn) string {
	if inspectRawHTTPTurnHasError(turn) {
		return firstNonEmptyRawHTTPDump(turn.ErrorPreview, turn.Status, turn.ResponseKind)
	}
	if len(turn.ToolCalls) > 0 {
		return "tool calls: " + strings.Join(turn.ToolCalls, ", ")
	}
	if turn.FinishReason != "" && turn.FinishReason != "stop" {
		return "finish: " + turn.FinishReason
	}
	return ""
}

func addInspectUsage(out map[string]int, usage string) {
	for _, field := range strings.Fields(usage) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(value)
		if err == nil {
			out[key] += n
		}
	}
}

func formatInspectUsage(usage map[string]int) string {
	var parts []string
	for _, key := range []string{"prompt", "cached", "completion", "total", "reasoning"} {
		if usage[key] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", key, usage[key]))
		}
	}
	return strings.Join(parts, " ")
}

func inspectRawHTTPStatusFromCode(code int) string {
	if code == 0 {
		return ""
	}
	return fmt.Sprintf("%d", code)
}

func inspectRawHTTPTurnSortKey(name string) string {
	return strings.TrimPrefix(name, "turn-")
}

func trimInspectRawHTTPLeadingZeros(value string) string {
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return "0"
	}
	return value
}

func trimInspectPreview(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 180 {
		return value[:180] + "..."
	}
	return value
}

func shortInspectTime(value string) string {
	if len(value) >= len("2006-01-02T15:04:05") {
		return value[11:19]
	}
	return value
}
