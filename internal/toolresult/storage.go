package toolresult

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

const (
	DefaultMaxResultSizeChars     = 50_000
	DefaultToolMaxResultSizeChars = 100_000
	MaxToolResultTokens           = 100_000
	BytesPerToken                 = 4
	MaxToolResultBytes            = MaxToolResultTokens * BytesPerToken
	MaxToolResultsPerMessageChars = 200_000
	PreviewSizeBytes              = 2_000

	PersistedOutputTag        = "<persisted-output>"
	PersistedOutputClosingTag = "</persisted-output>"
	ToolResultClearedMessage  = "[Old tool result content cleared]"
)

type ContentReplacementState struct {
	SeenIDs      map[string]bool
	Replacements map[string]string
}

type candidate struct {
	toolUseID string
	content   string
	size      int
}

func NewContentReplacementState() *ContentReplacementState {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &ContentReplacementState{\n\tSeenIDs:\tmake(map[string]bool),\n\tReplacements:\tmak...")
	return &ContentReplacementState{
		SeenIDs:      make(map[string]bool),
		Replacements: make(map[string]string),
	}
}

func ReconstructContentReplacementState(messages []model.Message, records []model.ContentReplacementRecord) *ContentReplacementState {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	state := NewContentReplacementState()
	for _, group := range collectCandidatesByMessage(messages) {
		observe.GlobalTrace("range collectCandidatesByMessage(messages)")
		for _, c := range group {
			observe.GlobalTrace("range group")
			state.SeenIDs[c.toolUseID] = true
		}
	}
	for _, r := range records {
		observe.GlobalTrace("range records")
		if r.Kind == model.ContentReplacementKindToolResult && state.SeenIDs[r.ToolUseID] {
			observe.GlobalTrace("if: r.Kind == model.ContentReplacementKindToolResult && state.SeenIDs[r.ToolUseID]")
			state.Replacements[r.ToolUseID] = r.Replacement
		}
	}
	observe.GlobalTrace("return: state")
	return state
}

func EffectiveThreshold(declared int) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if declared < 0 {
		observe.GlobalTrace("if: declared < 0")
		observe.GlobalTrace("return: declared")
		return declared
	}
	if declared == 0 {
		observe.GlobalTrace("if: declared == 0")
		declared = DefaultToolMaxResultSizeChars
	}
	if declared > DefaultMaxResultSizeChars {
		observe.GlobalTrace("if: declared > DefaultMaxResultSizeChars")
		observe.GlobalTrace("return: DefaultMaxResultSizeChars")
		return DefaultMaxResultSizeChars
	}
	observe.GlobalTrace("return: declared")
	return declared
}

func ProcessToolResult(part model.ToolResultPart, toolName string, declaredMaxResultSizeChars int, sessionID string) (model.ToolResultPart, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(part.Content) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(part.Content) == \"\"")
		part.Content = fmt.Sprintf("(%s completed with no output)", toolName)
		observe.GlobalTrace("return: part, nil")
		return part, nil
	}
	threshold := EffectiveThreshold(declaredMaxResultSizeChars)
	if threshold < 0 || len(part.Content) <= threshold {
		observe.GlobalTrace("if: threshold < 0 || len(part.Content) <= threshold")
		observe.GlobalTrace("return: part, nil")
		return part, nil
	}
	replacement, err := persistAndBuildReplacement(part.Content, part.ToolCallID, sessionID)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: part, err")
		return part, err
	}
	part.Content = replacement
	observe.GlobalTrace("return: part, nil")
	return part, nil
}

func ApplyToolResultBudget(messages []model.Message, state *ContentReplacementState, sessionID string, skipToolNames map[string]bool) ([]model.Message, []model.ContentReplacementRecord, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if state == nil {
		observe.GlobalTrace("if: state == nil")
		observe.GlobalTrace("return: messages, nil, nil")
		return messages, nil, nil
	}

	candidatesByMessage := collectCandidatesByMessage(messages)
	if len(candidatesByMessage) == 0 {
		observe.GlobalTrace("if: len(candidatesByMessage) == 0")
		observe.GlobalTrace("return: messages, nil, nil")
		return messages, nil, nil
	}

	nameByID := buildToolNameMap(messages)
	replacementMap := make(map[string]string)
	var toPersist []candidate

	for _, candidates := range candidatesByMessage {
		observe.GlobalTrace("range candidatesByMessage")
		var mustReapply []candidate
		var frozenSize int
		var fresh []candidate
		for _, c := range candidates {
			observe.GlobalTrace("range candidates")
			if replacement, ok := state.Replacements[c.toolUseID]; ok {
				observe.GlobalTrace("if: ok")
				c.content = replacement
				mustReapply = append(mustReapply, c)
				continue
			}
			if state.SeenIDs[c.toolUseID] {
				observe.GlobalTrace("if: state.SeenIDs[c.toolUseID]")
				frozenSize += c.size
				continue
			}
			if skipToolNames[nameByID[c.toolUseID]] {
				observe.GlobalTrace("if: skipToolNames[nameByID[c.toolUseID]]")
				state.SeenIDs[c.toolUseID] = true
				continue
			}
			fresh = append(fresh, c)
		}

		for _, c := range mustReapply {
			observe.GlobalTrace("range mustReapply")
			replacementMap[c.toolUseID] = c.content
			state.SeenIDs[c.toolUseID] = true
		}

		freshSize := 0
		for _, c := range fresh {
			observe.GlobalTrace("range fresh")
			freshSize += c.size
		}
		selected := selectFreshToReplace(fresh, frozenSize, MaxToolResultsPerMessageChars)
		selectedIDs := make(map[string]bool, len(selected))
		for _, c := range selected {
			observe.GlobalTrace("range selected")
			selectedIDs[c.toolUseID] = true
		}
		if frozenSize+freshSize <= MaxToolResultsPerMessageChars {
			observe.GlobalTrace("if: frozenSize+freshSize <= MaxToolResultsPerMessageChars")
			selected = nil
			selectedIDs = nil
		}
		for _, c := range candidates {
			observe.GlobalTrace("range candidates")
			if selectedIDs == nil || !selectedIDs[c.toolUseID] {
				observe.GlobalTrace("if: selectedIDs == nil || !selectedIDs[c.toolUseID]")
				state.SeenIDs[c.toolUseID] = true
			}
		}
		toPersist = append(toPersist, selected...)
	}

	var records []model.ContentReplacementRecord
	var firstErr error
	for _, c := range toPersist {
		observe.GlobalTrace("range toPersist")
		replacement, err := persistAndBuildReplacement(c.content, c.toolUseID, sessionID)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			if firstErr == nil {
				observe.GlobalTrace("if: firstErr == nil")
				firstErr = err
			}
			continue
		}
		state.SeenIDs[c.toolUseID] = true
		replacementMap[c.toolUseID] = replacement
		state.Replacements[c.toolUseID] = replacement
		records = append(records, model.ContentReplacementRecord{
			Kind:        model.ContentReplacementKindToolResult,
			ToolUseID:   c.toolUseID,
			Replacement: replacement,
		})
	}

	if len(replacementMap) == 0 {
		observe.GlobalTrace("if: len(replacementMap) == 0")
		observe.GlobalTrace("return: messages, records, firstErr")
		return messages, records, firstErr
	}
	observe.GlobalTrace("return: replaceToolResultContents(messages, replacementMap), records, firstErr")
	return replaceToolResultContents(messages, replacementMap), records, firstErr
}

func collectCandidatesByMessage(messages []model.Message) [][]candidate {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var groups [][]candidate
	var current []candidate
	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
		}
		current = nil
	}

	for _, msg := range messages {
		observe.GlobalTrace("range messages")
		switch msg.Role {
		case model.RoleUser:
			observe.GlobalTrace("case: model.RoleUser")
			current = append(current, collectCandidatesFromMessage(msg)...)
		case model.RoleAssistant:
			observe.GlobalTrace("case: model.RoleAssistant")
			flush()
		}
	}
	flush()
	observe.GlobalTrace("return: groups")
	return groups
}

func collectCandidatesFromMessage(msg model.Message) []candidate {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var out []candidate
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		tr, ok := part.(model.ToolResultPart)
		if !ok || tr.Content == "" || isContentAlreadyCompacted(tr.Content) {
			observe.GlobalTrace("if: !ok || tr.Content == \"\" || isContentAlreadyCompacted(tr.Content)")
			continue
		}
		out = append(out, candidate{
			toolUseID: tr.ToolCallID,
			content:   tr.Content,
			size:      len(tr.Content),
		})
	}
	observe.GlobalTrace("return: out")
	return out
}

func buildToolNameMap(messages []model.Message) map[string]string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make(map[string]string)
	for _, msg := range messages {
		observe.GlobalTrace("range messages")
		if msg.Role != model.RoleAssistant {
			observe.GlobalTrace("if: msg.Role != model.RoleAssistant")
			continue
		}
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			if tc, ok := part.(model.ToolCallPart); ok {
				observe.GlobalTrace("if: ok")
				out[tc.ID] = tc.Name
			}
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

func selectFreshToReplace(fresh []candidate, frozenSize, limit int) []candidate {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sorted := append([]candidate(nil), fresh...)
	for i := 0; i < len(sorted); i++ {
		observe.GlobalTrace("for: i < len(sorted)")
		for j := i + 1; j < len(sorted); j++ {
			observe.GlobalTrace("for: j < len(sorted)")
			if sorted[j].size > sorted[i].size {
				observe.GlobalTrace("if: sorted[j].size > sorted[i].size")
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	remaining := frozenSize
	for _, c := range sorted {
		observe.GlobalTrace("range sorted")
		remaining += c.size
	}
	var selected []candidate
	for _, c := range sorted {
		observe.GlobalTrace("range sorted")
		if remaining <= limit {
			observe.GlobalTrace("if: remaining <= limit")
			break
		}
		selected = append(selected, c)
		remaining -= c.size
	}
	observe.GlobalTrace("return: selected")
	return selected
}

func replaceToolResultContents(messages []model.Message, replacements map[string]string) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]model.Message, len(messages))
	copy(out, messages)
	for i, msg := range out {
		observe.GlobalTrace("range out")
		var changed bool
		content := make([]model.ContentPart, len(msg.Content))
		copy(content, msg.Content)
		for j, part := range content {
			observe.GlobalTrace("range content")
			tr, ok := part.(model.ToolResultPart)
			if !ok {
				observe.GlobalTrace("if: !ok")
				continue
			}
			if replacement, exists := replacements[tr.ToolCallID]; exists {
				observe.GlobalTrace("if: exists")
				tr.Content = replacement
				content[j] = tr
				changed = true
			}
		}
		if changed {
			observe.GlobalTrace("if: changed")
			msg.Content = content
			out[i] = msg
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

func persistAndBuildReplacement(content, toolUseID, sessionID string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	filepath, err := persistToolResult(content, toolUseID, sessionID)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	preview, hasMore := generatePreview(content, PreviewSizeBytes)
	var b strings.Builder
	fmt.Fprintf(
		&b,
		`<persisted-output tool_call_id=%q bytes=%q preview_bytes=%q path=%q>`,
		toolUseID,
		fmt.Sprintf("%d", len(content)),
		fmt.Sprintf("%d", len(preview)),
		filepath,
	)
	b.WriteByte('\n')
	fmt.Fprintf(&b, "Output too large (%s). Full output saved to: %s\n\n", formatFileSize(len(content)), filepath)
	fmt.Fprintf(&b, "Preview (first %s):\n", formatFileSize(PreviewSizeBytes))
	b.WriteString(preview)
	if hasMore {
		observe.GlobalTrace("if: hasMore")
		b.WriteString("\n...\n")
	} else {
		observe.GlobalTrace("else: hasMore")
		b.WriteByte('\n')
	}
	b.WriteString(PersistedOutputClosingTag)
	observe.GlobalTrace("return: b.String(), nil")
	return b.String(), nil
}

func persistToolResult(content, toolUseID, sessionID string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return persistToolResultBytes([]byte(content), toolUseID, sessionID, ".txt")
}

// PersistBinaryOutput persists binary tool output under the session tool-result artifact tree.
func PersistBinaryOutput(content []byte, artifactID, sessionID, ext string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if ext == "" || !strings.HasPrefix(ext, ".") {
		ext = ".bin"
	}
	return persistToolResultBytes(content, artifactID, sessionID, ext)
}

func persistToolResultBytes(content []byte, toolUseID, sessionID, ext string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if sessionID == "" {
		observe.GlobalTrace("if: sessionID == \"\"")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"session id is required\")")
		return "", fmt.Errorf("session id is required")
	}
	sessionsDir, err := config.SessionsDir()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	dir := filepath.Join(sessionsDir, sessionID, "tool-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	path := filepath.Join(dir, safeToolUseID(toolUseID)+ext)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		if os.IsExist(err) {
			observe.GlobalTrace("if: os.IsExist(err)")
			observe.GlobalTrace("return: path, nil")
			return path, nil
		}
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(content); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	observe.GlobalTrace("return: path, nil")
	return path, nil
}

// PersistedOutputPath returns the session-local persisted tool-result path for a tool call.
func PersistedOutputPath(sessionID, toolUseID string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if sessionID == "" {
		observe.GlobalTrace("if: sessionID == \"\"")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"session id is required\")")
		return "", fmt.Errorf("session id is required")
	}
	sessionsDir, err := config.SessionsDir()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	observe.GlobalTrace("return: filepath.Join(sessionsDir, sessionID, \"tool-results\", safeToolUseID(toolUseID...")
	return filepath.Join(sessionsDir, sessionID, "tool-results", safeToolUseID(toolUseID)+".txt"), nil
}

func generatePreview(content string, maxBytes int) (string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(content) <= maxBytes {
		observe.GlobalTrace("if: len(content) <= maxBytes")
		observe.GlobalTrace("return: content, false")
		return content, false
	}
	truncated := content[:maxBytes]
	lastNewline := strings.LastIndex(truncated, "\n")
	cutPoint := maxBytes
	if lastNewline > maxBytes/2 {
		observe.GlobalTrace("if: lastNewline > maxBytes/2")
		cutPoint = lastNewline
	}
	observe.GlobalTrace("return: content[:cutPoint], true")
	return content[:cutPoint], true
}

func isContentAlreadyCompacted(content string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.HasPrefix(content, PersistedOutputTag)")
	observe.GlobalTrace("return: strings.HasPrefix(content, PersistedOutputTag) || strings.HasPrefix(content, ...")
	return strings.HasPrefix(content, PersistedOutputTag) || strings.HasPrefix(content, "<persisted-output ")
}

func safeToolUseID(id string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, string(os.PathSeparator), "_")
	if id == "" {
		observe.GlobalTrace("if: id == \"\"")
		observe.GlobalTrace("return: \"unknown\"")
		return "unknown"
	}
	observe.GlobalTrace("return: id")
	return id
}

func formatFileSize(sizeInBytes int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	kb := float64(sizeInBytes) / 1024
	if kb < 1 {
		observe.GlobalTrace("if: kb < 1")
		observe.GlobalTrace("return: fmt.Sprintf(\"%d bytes\", sizeInBytes)")
		return fmt.Sprintf("%d bytes", sizeInBytes)
	}
	if kb < 1024 {
		observe.GlobalTrace("if: kb < 1024")
		observe.GlobalTrace("return: trimFloat(kb) + \"KB\"")
		return trimFloat(kb) + "KB"
	}
	mb := kb / 1024
	if mb < 1024 {
		observe.GlobalTrace("if: mb < 1024")
		observe.GlobalTrace("return: trimFloat(mb) + \"MB\"")
		return trimFloat(mb) + "MB"
	}
	observe.GlobalTrace("return: trimFloat(mb/1024) + \"GB\"")
	return trimFloat(mb/1024) + "GB"
}

func trimFloat(v float64) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s := fmt.Sprintf("%.1f", v)
	observe.GlobalTrace("return: strings.TrimSuffix(s, \".0\")")
	return strings.TrimSuffix(s, ".0")
}
