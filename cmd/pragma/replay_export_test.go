package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/session"
)

func TestReplayExportReconstructsBugHuntCheckpoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	createdAt := time.Unix(10, 0)
	header := session.HeaderData{
		SessionID: "session-1",
		Model:     "claude-sonnet-4-20250514",
		Provider:  "anthropic",
		WorkDir:   "/repo",
		CreatedAt: createdAt,
		System:    model.SystemPrompt{Blocks: []model.SystemBlock{{Text: "system"}}},
	}
	messages := []model.Message{
		{
			ID:        "user-1",
			Role:      model.RoleUser,
			Content:   []model.ContentPart{model.TextPart{Text: "check the last agent output"}},
			Timestamp: createdAt,
		},
		{
			ID:   "assistant-tools",
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				agentCallForTest("call-race", "Concurrency bug search", "Search this Go codebase for race bugs and report concrete findings."),
				agentCallForTest("call-logic", "Logic bug search", "Search this Go codebase for logic bugs and report concrete findings."),
				agentCallForTest("call-leak", "Memory leak search", "Search this Go codebase for memory leak bugs and report concrete findings."),
			},
			Timestamp: createdAt.Add(time.Second),
		},
		{
			ID:   "tool-results",
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.ToolResultPart{ToolCallID: "call-race", Content: "finding one"},
				model.ToolResultPart{ToolCallID: "call-logic", Content: "finding two"},
				model.ToolResultPart{ToolCallID: "call-leak", Content: "finding three"},
			},
			Timestamp: createdAt.Add(2 * time.Second),
		},
		{
			ID:        "final",
			Role:      model.RoleAssistant,
			Content:   []model.ContentPart{model.TextPart{Text: "Bug Report\n\n`messagesForQuery = ok` is wrong."}},
			Timestamp: createdAt.Add(3 * time.Second),
		},
	}
	writeSessionForTest(t, path, header, messages)

	conv, err := loadExportConversationFile(path)
	if err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	checkpoints := buildBugHuntCheckpoints(conv)
	if len(checkpoints) != 4 {
		t.Fatalf("checkpoint count: got %d want 4", len(checkpoints))
	}
	for i := 0; i < 3; i++ {
		cp := checkpoints[i]
		if cp.Kind != "agent_prompt" || cp.AgentPrompt == nil {
			t.Fatalf("checkpoint %d not agent prompt: %#v", i, cp)
		}
		if !cp.Reconstructed || len(cp.MissingExactFields) == 0 {
			t.Fatalf("checkpoint %d missing reconstruction metadata: %#v", i, cp)
		}
		if len(cp.Messages) != 3 {
			t.Fatalf("checkpoint %d message count: got %d want 3", i, len(cp.Messages))
		}
		lastText := cp.Messages[len(cp.Messages)-1].Content[0].(model.TextPart).Text
		if !strings.Contains(lastText, "Search this Go codebase") {
			t.Fatalf("checkpoint %d missing agent prompt message: %q", i, lastText)
		}
	}
	final := checkpoints[3]
	if final.Kind != "parent_synthesis" || len(final.Messages) != 3 {
		t.Fatalf("wrong parent synthesis checkpoint: %#v", final)
	}
}

func agentCallForTest(id, description, prompt string) model.ToolCallPart {
	input, _ := json.Marshal(map[string]string{
		"description": description,
		"prompt":      prompt,
		"model":       "claude-sonnet-4-20250514",
	})
	return model.ToolCallPart{ID: id, Name: "Agent", Input: input}
}

func writeSessionForTest(t *testing.T, path string, header session.HeaderData, messages []model.Message) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer f.Close()
	writeEntryForTest(t, f, session.EntryHeader, header)
	for _, msg := range messages {
		writeEntryForTest(t, f, session.EntryMessage, msg)
	}
}

func writeEntryForTest(t *testing.T, f *os.File, kind session.EntryKind, data any) {
	t.Helper()
	entry, err := session.MarshalEntry(kind, data)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal line: %v", err)
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		t.Fatalf("write line: %v", err)
	}
}
