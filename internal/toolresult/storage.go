package toolresult

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
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
	return &ContentReplacementState{
		SeenIDs:      make(map[string]bool),
		Replacements: make(map[string]string),
	}
}

func ReconstructContentReplacementState(messages []model.Message, records []model.ContentReplacementRecord) *ContentReplacementState {
	state := NewContentReplacementState()
	for _, group := range collectCandidatesByMessage(messages) {
		for _, c := range group {
			state.SeenIDs[c.toolUseID] = true
		}
	}
	for _, r := range records {
		if r.Kind == model.ContentReplacementKindToolResult && state.SeenIDs[r.ToolUseID] {
			state.Replacements[r.ToolUseID] = r.Replacement
		}
	}
	return state
}

func EffectiveThreshold(declared int) int {
	if declared < 0 {
		return declared
	}
	if declared == 0 {
		declared = DefaultToolMaxResultSizeChars
	}
	if declared > DefaultMaxResultSizeChars {
		return DefaultMaxResultSizeChars
	}
	return declared
}

func ProcessToolResult(part model.ToolResultPart, toolName string, declaredMaxResultSizeChars int, sessionID string) (model.ToolResultPart, error) {
	if strings.TrimSpace(part.Content) == "" {
		part.Content = fmt.Sprintf("(%s completed with no output)", toolName)
		return part, nil
	}
	threshold := EffectiveThreshold(declaredMaxResultSizeChars)
	if threshold < 0 || len(part.Content) <= threshold {
		return part, nil
	}
	replacement, err := persistAndBuildReplacement(part.Content, part.ToolCallID, sessionID)
	if err != nil {
		return part, err
	}
	part.Content = replacement
	return part, nil
}

func ApplyToolResultBudget(messages []model.Message, state *ContentReplacementState, sessionID string, skipToolNames map[string]bool) ([]model.Message, []model.ContentReplacementRecord, error) {
	if state == nil {
		return messages, nil, nil
	}

	candidatesByMessage := collectCandidatesByMessage(messages)
	if len(candidatesByMessage) == 0 {
		return messages, nil, nil
	}

	nameByID := buildToolNameMap(messages)
	replacementMap := make(map[string]string)
	var toPersist []candidate

	for _, candidates := range candidatesByMessage {
		var mustReapply []candidate
		var frozenSize int
		var fresh []candidate
		for _, c := range candidates {
			if replacement, ok := state.Replacements[c.toolUseID]; ok {
				c.content = replacement
				mustReapply = append(mustReapply, c)
				continue
			}
			if state.SeenIDs[c.toolUseID] {
				frozenSize += c.size
				continue
			}
			if skipToolNames[nameByID[c.toolUseID]] {
				state.SeenIDs[c.toolUseID] = true
				continue
			}
			fresh = append(fresh, c)
		}

		for _, c := range mustReapply {
			replacementMap[c.toolUseID] = c.content
			state.SeenIDs[c.toolUseID] = true
		}

		freshSize := 0
		for _, c := range fresh {
			freshSize += c.size
		}
		selected := selectFreshToReplace(fresh, frozenSize, MaxToolResultsPerMessageChars)
		selectedIDs := make(map[string]bool, len(selected))
		for _, c := range selected {
			selectedIDs[c.toolUseID] = true
		}
		if frozenSize+freshSize <= MaxToolResultsPerMessageChars {
			selected = nil
			selectedIDs = nil
		}
		for _, c := range candidates {
			if selectedIDs == nil || !selectedIDs[c.toolUseID] {
				state.SeenIDs[c.toolUseID] = true
			}
		}
		toPersist = append(toPersist, selected...)
	}

	var records []model.ContentReplacementRecord
	var firstErr error
	for _, c := range toPersist {
		replacement, err := persistAndBuildReplacement(c.content, c.toolUseID, sessionID)
		if err != nil {
			if firstErr == nil {
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
		return messages, records, firstErr
	}
	return replaceToolResultContents(messages, replacementMap), records, firstErr
}

func collectCandidatesByMessage(messages []model.Message) [][]candidate {
	var groups [][]candidate
	var current []candidate
	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
		}
		current = nil
	}

	for _, msg := range messages {
		switch msg.Role {
		case model.RoleUser:
			current = append(current, collectCandidatesFromMessage(msg)...)
		case model.RoleAssistant:
			flush()
		}
	}
	flush()
	return groups
}

func collectCandidatesFromMessage(msg model.Message) []candidate {
	var out []candidate
	for _, part := range msg.Content {
		tr, ok := part.(model.ToolResultPart)
		if !ok || tr.Content == "" || isContentAlreadyCompacted(tr.Content) {
			continue
		}
		out = append(out, candidate{
			toolUseID: tr.ToolCallID,
			content:   tr.Content,
			size:      len(tr.Content),
		})
	}
	return out
}

func buildToolNameMap(messages []model.Message) map[string]string {
	out := make(map[string]string)
	for _, msg := range messages {
		if msg.Role != model.RoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			if tc, ok := part.(model.ToolCallPart); ok {
				out[tc.ID] = tc.Name
			}
		}
	}
	return out
}

func selectFreshToReplace(fresh []candidate, frozenSize, limit int) []candidate {
	sorted := append([]candidate(nil), fresh...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].size > sorted[i].size {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	remaining := frozenSize
	for _, c := range sorted {
		remaining += c.size
	}
	var selected []candidate
	for _, c := range sorted {
		if remaining <= limit {
			break
		}
		selected = append(selected, c)
		remaining -= c.size
	}
	return selected
}

func replaceToolResultContents(messages []model.Message, replacements map[string]string) []model.Message {
	out := make([]model.Message, len(messages))
	copy(out, messages)
	for i, msg := range out {
		var changed bool
		content := make([]model.ContentPart, len(msg.Content))
		copy(content, msg.Content)
		for j, part := range content {
			tr, ok := part.(model.ToolResultPart)
			if !ok {
				continue
			}
			if replacement, exists := replacements[tr.ToolCallID]; exists {
				tr.Content = replacement
				content[j] = tr
				changed = true
			}
		}
		if changed {
			msg.Content = content
			out[i] = msg
		}
	}
	return out
}

func persistAndBuildReplacement(content, toolUseID, sessionID string) (string, error) {
	filepath, err := persistToolResult(content, toolUseID, sessionID)
	if err != nil {
		return "", err
	}
	preview, hasMore := generatePreview(content, PreviewSizeBytes)
	var b strings.Builder
	b.WriteString(PersistedOutputTag)
	b.WriteByte('\n')
	fmt.Fprintf(&b, "Output too large (%s). Full output saved to: %s\n\n", formatFileSize(len(content)), filepath)
	fmt.Fprintf(&b, "Preview (first %s):\n", formatFileSize(PreviewSizeBytes))
	b.WriteString(preview)
	if hasMore {
		b.WriteString("\n...\n")
	} else {
		b.WriteByte('\n')
	}
	b.WriteString(PersistedOutputClosingTag)
	return b.String(), nil
}

func persistToolResult(content, toolUseID, sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	sessionsDir, err := config.SessionsDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(sessionsDir, sessionID, "tool-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeToolUseID(toolUseID)+".txt")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return path, nil
		}
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return path, nil
}

func generatePreview(content string, maxBytes int) (string, bool) {
	if len(content) <= maxBytes {
		return content, false
	}
	truncated := content[:maxBytes]
	lastNewline := strings.LastIndex(truncated, "\n")
	cutPoint := maxBytes
	if lastNewline > maxBytes/2 {
		cutPoint = lastNewline
	}
	return content[:cutPoint], true
}

func isContentAlreadyCompacted(content string) bool {
	return strings.HasPrefix(content, PersistedOutputTag)
}

func safeToolUseID(id string) string {
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, string(os.PathSeparator), "_")
	if id == "" {
		return "unknown"
	}
	return id
}

func formatFileSize(sizeInBytes int) string {
	kb := float64(sizeInBytes) / 1024
	if kb < 1 {
		return fmt.Sprintf("%d bytes", sizeInBytes)
	}
	if kb < 1024 {
		return trimFloat(kb) + "KB"
	}
	mb := kb / 1024
	if mb < 1024 {
		return trimFloat(mb) + "MB"
	}
	return trimFloat(mb/1024) + "GB"
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}
