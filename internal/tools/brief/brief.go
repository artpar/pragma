package brief

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/util"
)

// BriefInput defines the parameters for the SendUserMessage tool.
type BriefInput struct {
	Message     string   `json:"message" desc:"The message for the user. Supports markdown."`
	Attachments []string `json:"attachments,omitempty" desc:"Optional file paths to reference alongside the message"`
	Status      string   `json:"status,omitempty" desc:"Message type: normal reply or proactive notification"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["message"],
	"properties": {
		"message": {
			"type": "string",
			"description": "The message for the user. Supports markdown formatting."
		},
		"attachments": {
			"type": "array",
			"items": {"type": "string"},
			"description": "Optional file paths (absolute or relative to cwd) for images, screenshots, logs"
		},
		"status": {
			"type": "string",
			"enum": ["normal", "proactive"],
			"default": "normal",
			"description": "normal: replying to user. proactive: unsolicited update (task done, blocker hit)."
		}
	}
}`)

// Tool implements the SendUserMessage tool for primary user communication.
type Tool struct {
	Bus *observe.EventBus
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"SendUserMessage\"")
	return "SendUserMessage"
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: `Send a message to the user.\n\nUse this tool to communicate directly with the ...")
	return `Send a message to the user.

Use this tool to communicate directly with the user. The message supports markdown formatting.

Optionally attach file paths that the user should see alongside the message.

Use status "proactive" when surfacing something unsolicited — a completed background task, a blocker you hit, or an update the user didn't explicitly ask for.`
}

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}

func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(_ context.Context, _ json.RawMessage, _ permission.Checker) permission.CheckResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: permission.CheckResult{Decision: permission.DecisionAllow}")
	return permission.CheckResult{Decision: permission.DecisionAllow}
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "brief", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "brief", "Tool.Invoke", "exit")

	var in BriefInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Invalid input: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Invalid input: %v", err)}, nil
	}

	if in.Message == "" {
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "if: in.Message == \"\"")
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "return: tool.InvokeResult{Content: \"Message is required.\"}, nil")
		return tool.InvokeResult{Content: "Message is required."}, nil
	}

	if in.Status == "" {
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "if: in.Status == \"\"")
		in.Status = "normal"
	}

	var attachments []attachmentInfo
	for _, rawPath := range in.Attachments {
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "range in.Attachments")
		info, err := resolveAttachment(rawPath, state.WorkDir())
		if err != nil {
			observe.TraceCtx(ctx, "brief", "Tool.Invoke", "attachment error: "+err.Error())

			attachments = append(attachments, attachmentInfo{
				Path:  rawPath,
				Error: err.Error(),
			})
			continue
		}
		attachments = append(attachments, info)
	}

	briefAttachments := make([]observe.BriefAttachment, 0, len(attachments))
	userAttachments := make([]tool.UserMessageAttachment, 0, len(attachments))
	for _, a := range attachments {
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "range attachments")
		if a.Error != "" {
			observe.TraceCtx(ctx, "brief", "Tool.Invoke", "if: a.Error != \"\"")
			continue
		}
		briefAttachments = append(briefAttachments, observe.BriefAttachment{
			Path:    a.Path,
			Size:    a.Size,
			IsImage: a.IsImage,
		})
		userAttachments = append(userAttachments, tool.UserMessageAttachment{
			Path:    a.Path,
			Size:    a.Size,
			IsImage: a.IsImage,
		})
	}

	if t.Bus != nil {
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "if: t.Bus != nil")
		t.Bus.Emit(observe.BriefMessageSent{
			EventHeader: observe.NewEventHeader("BriefMessageSent", "", observe.NewSpanID(), ""),
			Message:     in.Message,
			Attachments: briefAttachments,
			Status:      in.Status,
		})
	}

	validCount := 0
	for _, a := range attachments {
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "range attachments")
		if a.Error == "" {
			observe.TraceCtx(ctx, "brief", "Tool.Invoke", "if: a.Error == \"\"")
			validCount++
		}
	}

	content := "Message delivered to user."
	if validCount > 0 {
		observe.TraceCtx(ctx, "brief", "Tool.Invoke", "if: validCount > 0")
		noun := "attachment"
		if validCount != 1 {
			observe.TraceCtx(ctx, "brief", "Tool.Invoke", "if: validCount != 1")
			noun = "attachments"
		}
		content = fmt.Sprintf("Message delivered to user. (%d %s included)", validCount, noun)
	}
	observe.TraceCtx(ctx, "brief", "Tool.Invoke", "return: tool.InvokeResult{Content: content}, nil")
	observe.TraceCtx(ctx, "brief", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent:\tcontent,\n\tUserMessages: []tool.UserMessage{{\n\t\tM...")

	return tool.InvokeResult{
		Content: content,
		UserMessages: []tool.UserMessage{{
			Message:     in.Message,
			Status:      in.Status,
			Attachments: userAttachments,
		}},
	}, nil
}

// attachmentInfo holds resolved metadata about a file attachment.
type attachmentInfo struct {
	Path    string
	Size    int64
	IsImage bool
	Error   string
}

// imageExtensions lists file extensions considered images.
var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".webp": true, ".svg": true,
}

// resolveAttachment validates and resolves a file path to attachment metadata.
func resolveAttachment(rawPath, workDir string) (attachmentInfo, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	p := util.ExpandPath(rawPath, workDir)
	p = filepath.Clean(p)

	wdPrefix := workDir
	if !strings.HasSuffix(wdPrefix, string(filepath.Separator)) {
		observe.GlobalTrace("if: !strings.HasSuffix(wdPrefix, string(filepath.Separator))")
		wdPrefix += string(filepath.Separator)
	}
	if !strings.HasPrefix(p, wdPrefix) && p != workDir {
		observe.GlobalTrace("if: !strings.HasPrefix(p, wdPrefix) && p != workDir")
		observe.GlobalTrace("return: attachmentInfo{}, fmt.Errorf(\"path %q is outside working directory\", rawPath)")
		return attachmentInfo{}, fmt.Errorf("path %q is outside working directory", rawPath)
	}

	fi, err := os.Stat(p)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		if os.IsNotExist(err) {
			observe.GlobalTrace("if: os.IsNotExist(err)")
			observe.GlobalTrace("return: attachmentInfo{}, fmt.Errorf(\"file does not exist: %s (cwd: %s)\", rawPath, wo...")
			return attachmentInfo{}, fmt.Errorf("file does not exist: %s (cwd: %s)", rawPath, workDir)
		}
		if os.IsPermission(err) {
			observe.GlobalTrace("if: os.IsPermission(err)")
			observe.GlobalTrace("return: attachmentInfo{}, fmt.Errorf(\"permission denied: %s\", rawPath)")
			return attachmentInfo{}, fmt.Errorf("permission denied: %s", rawPath)
		}
		observe.GlobalTrace("return: attachmentInfo{}, err")
		return attachmentInfo{}, err
	}

	if fi.IsDir() {
		observe.GlobalTrace("if: fi.IsDir()")
		observe.GlobalTrace("return: attachmentInfo{}, fmt.Errorf(\"path is a directory, not a file: %s\", rawPath)")
		return attachmentInfo{}, fmt.Errorf("path is a directory, not a file: %s", rawPath)
	}

	ext := strings.ToLower(filepath.Ext(p))
	observe.GlobalTrace("return: attachmentInfo{\n\tPath:\t\tp,\n\tSize:\t\tfi.Size(),\n\tIsImage:\timageExtensions[ext],...")
	return attachmentInfo{
		Path:    p,
		Size:    fi.Size(),
		IsImage: imageExtensions[ext],
	}, nil
}
