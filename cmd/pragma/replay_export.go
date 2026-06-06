package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/sessionpath"
)

type checkpointExport struct {
	CheckpointID        string                     `json:"checkpoint_id"`
	SourceSessionID     string                     `json:"source_session_id"`
	Kind                string                     `json:"kind"`
	Model               string                     `json:"model"`
	Provider            string                     `json:"provider"`
	WorkDir             string                     `json:"work_dir"`
	System              model.SystemPrompt         `json:"system"`
	Messages            []model.Message            `json:"messages"`
	ToolResultArtifacts []exportToolResultArtifact `json:"tool_result_artifacts,omitempty"`
	AgentPrompt         *exportAgentPrompt         `json:"agent_prompt,omitempty"`
	Reconstructed       bool                       `json:"reconstructed"`
	MissingExactFields  []string                   `json:"missing_exact_fields,omitempty"`
	CreatedAt           time.Time                  `json:"created_at,omitempty"`
}

type exportAgentPrompt struct {
	Description string `json:"description,omitempty"`
	Prompt      string `json:"prompt"`
	Model       string `json:"model,omitempty"`
}

type exportToolResultArtifact struct {
	Path          string `json:"path"`
	Bytes         int64  `json:"bytes"`
	ContentBase64 string `json:"content_base64"`
}

type exportConversation struct {
	id                  string
	model               string
	provider            string
	workDir             string
	system              model.SystemPrompt
	createdAt           time.Time
	messages            []model.Message
	toolResultArtifacts []exportToolResultArtifact
}

func replayExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <session-id-or-jsonl>",
		Short: "Export reconstructed replay checkpoints from a session",
		Long: `Export portable JSONL checkpoints from a saved session.

For unrecorded sessions this reconstructs useful request checkpoints from the
session JSONL and marks them as reconstructed. Exact provider payload replay
requires recordings made with --record after request payload recording support.`,
		Args: cobra.ExactArgs(1),
		RunE: replayExportRun,
	}
	cmd.Flags().String("out", "", "write JSONL checkpoints to this file instead of stdout")
	return cmd
}

func replayExportRun(cmd *cobra.Command, args []string) error {
	conv, err := loadExportConversation(args[0])
	if err != nil {
		return err
	}
	checkpoints := buildBugHuntCheckpoints(conv)
	if len(checkpoints) == 0 {
		return fmt.Errorf("no bug-hunt checkpoints found in %s", args[0])
	}

	outPath, _ := cmd.Flags().GetString("out")
	var out *os.File
	if outPath == "" {
		out = os.Stdout
	} else {
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil && filepath.Dir(outPath) != "." {
			return fmt.Errorf("create output directory: %w", err)
		}
		out, err = os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", outPath, err)
		}
		defer out.Close()
	}

	enc := json.NewEncoder(out)
	for _, cp := range checkpoints {
		if err := enc.Encode(cp); err != nil {
			return fmt.Errorf("write checkpoint: %w", err)
		}
	}
	return nil
}

func loadExportConversation(source string) (exportConversation, error) {
	if strings.HasSuffix(source, ".jsonl") || strings.ContainsRune(source, filepath.Separator) {
		return loadExportConversationFile(source)
	}
	store, err := session.NewStore()
	if err != nil {
		return exportConversation{}, err
	}
	sess, err := store.Load(source)
	if err != nil {
		return exportConversation{}, err
	}
	toolResultsDir, _ := store.ToolResultsDir(source)
	return exportConversationFromSession(sess, loadExportToolResultArtifacts(toolResultsDir)), nil
}

func loadExportConversationFile(path string) (exportConversation, error) {
	if !filepath.IsAbs(path) {
		if strings.HasSuffix(path, ".jsonl") {
			abs, err := filepath.Abs(path)
			if err != nil {
				return exportConversation{}, err
			}
			path = abs
		} else {
			dir, err := config.SessionsDir()
			if err != nil {
				return exportConversation{}, err
			}
			path = filepath.Join(dir, path)
		}
	}
	store, err := session.NewStore()
	if err != nil {
		return exportConversation{}, err
	}
	sess, err := store.LoadFile(path)
	if err != nil {
		return exportConversation{}, err
	}
	toolResults := loadExportToolResultArtifacts(sessionpath.ToolResultsDir(filepath.Dir(path), sess.Conversation.ID))
	return exportConversationFromSession(sess, toolResults), nil
}

func exportConversationFromSession(sess session.Session, artifacts []exportToolResultArtifact) exportConversation {
	conv := sess.Conversation
	return exportConversation{
		id:                  conv.ID,
		model:               conv.Model,
		provider:            conv.Provider,
		workDir:             conv.WorkDir,
		system:              conv.System,
		createdAt:           conv.CreatedAt,
		messages:            conv.Messages,
		toolResultArtifacts: artifacts,
	}
}

func loadExportToolResultArtifacts(dir string) []exportToolResultArtifact {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	artifacts := make([]exportToolResultArtifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		artifacts = append(artifacts, exportToolResultArtifact{
			Path:          filepath.Join(sessionpath.ToolResultsDirName, entry.Name()),
			Bytes:         int64(len(data)),
			ContentBase64: base64.StdEncoding.EncodeToString(data),
		})
	}
	return artifacts
}

func buildBugHuntCheckpoints(conv exportConversation) []checkpointExport {
	var checkpoints []checkpointExport
	for i, msg := range conv.messages {
		if msg.Role != model.RoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			tc, ok := part.(model.ToolCallPart)
			if !ok || tc.Name != "Agent" {
				continue
			}
			var in struct {
				Prompt      string `json:"prompt"`
				Description string `json:"description"`
				Model       string `json:"model"`
			}
			if err := json.Unmarshal(tc.Input, &in); err != nil || !isBugHuntAgentPrompt(in.Prompt, in.Description) {
				continue
			}
			messages := append([]model.Message(nil), conv.messages[:i+1]...)
			messages = append(messages, model.Message{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   []model.ContentPart{model.TextPart{Text: in.Prompt}},
				Timestamp: msg.Timestamp,
			})
			checkpoints = append(checkpoints, checkpointExport{
				CheckpointID:        fmt.Sprintf("%s-agent-%d", conv.id, len(checkpoints)+1),
				SourceSessionID:     conv.id,
				Kind:                "agent_prompt",
				Model:               firstNonEmpty(in.Model, conv.model),
				Provider:            conv.provider,
				WorkDir:             conv.workDir,
				System:              conv.system,
				Messages:            messages,
				ToolResultArtifacts: conv.toolResultArtifacts,
				AgentPrompt: &exportAgentPrompt{
					Description: in.Description,
					Prompt:      in.Prompt,
					Model:       in.Model,
				},
				Reconstructed: true,
				MissingExactFields: []string{
					"sub-agent forked conversation state",
					"provider-specific wire payload",
					"tool schemas as sent to provider",
				},
				CreatedAt: msg.Timestamp,
			})
		}
	}

	if idx := finalBugReportIndex(conv.messages); idx >= 0 {
		checkpoints = append(checkpoints, checkpointExport{
			CheckpointID:        fmt.Sprintf("%s-parent-synthesis", conv.id),
			SourceSessionID:     conv.id,
			Kind:                "parent_synthesis",
			Model:               conv.model,
			Provider:            conv.provider,
			WorkDir:             conv.workDir,
			System:              conv.system,
			Messages:            append([]model.Message(nil), conv.messages[:idx]...),
			ToolResultArtifacts: conv.toolResultArtifacts,
			Reconstructed:       true,
			MissingExactFields: []string{
				"provider-specific wire payload",
				"tool schemas as sent to provider",
			},
			CreatedAt: conv.messages[idx].Timestamp,
		})
	}
	return checkpoints
}

func isBugHuntAgentPrompt(prompt, description string) bool {
	text := strings.ToLower(prompt + " " + description)
	return strings.Contains(text, "search this go codebase") &&
		(strings.Contains(text, "bug") ||
			strings.Contains(text, "race") ||
			strings.Contains(text, "memory leak") ||
			strings.Contains(text, "logic"))
}

func finalBugReportIndex(messages []model.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != model.RoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok &&
				strings.Contains(tp.Text, "Bug Report") &&
				strings.Contains(tp.Text, "messagesForQuery = ok") {
				return i
			}
		}
	}
	return -1
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
