package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
)

const (
	permOptAllow       = 0
	permOptAlwaysAllow = 1
	permOptDeny        = 2
	permOptCount       = 3
)

var permOptionLabels = [permOptCount]string{
	"Yes",
	"Yes, for this session",
	"No",
}

// permissionDialog renders an interactive permission prompt.
type permissionDialog struct {
	active   bool
	request  *PermRequestMsg
	selected int
}

func newPermissionDialog() permissionDialog {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: permissionDialog{}")
	return permissionDialog{}
}

// Show activates the dialog for a permission request.
func (d *permissionDialog) Show(req *PermRequestMsg) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.active = true
	d.request = req
	d.selected = permOptAllow
}

// Update handles key events when the dialog is active.
// Returns a tea.Cmd if the user has made a decision (nil otherwise).
func (d *permissionDialog) Update(msg tea.Msg) tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active {
		observe.GlobalTrace("if: !d.active")
		observe.GlobalTrace("return: nil")
		return nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil")
		return nil
	}

	switch keyMsg.String() {
	case "up", "k":
		observe.GlobalTrace("case: \"up\", \"k\"")
		d.selected--
		if d.selected < 0 {
			d.selected = permOptCount - 1
		}
	case "down", "j":
		observe.GlobalTrace("case: \"down\", \"j\"")
		d.selected++
		if d.selected >= permOptCount {
			d.selected = 0
		}
	case "enter":
		observe.GlobalTrace("case: \"enter\"")
		return d.confirm()
	case "esc":
		observe.GlobalTrace("case: \"esc\"")
		return d.deny()
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// confirm sends the selected decision and deactivates the dialog.
func (d *permissionDialog) confirm() tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	req := d.request
	d.active = false
	d.request = nil

	var decision permission.Decision
	var remember bool

	switch d.selected {
	case permOptAllow:
		observe.GlobalTrace("case: permOptAllow")
		decision = permission.DecisionAllow
	case permOptDeny:
		observe.GlobalTrace("case: permOptDeny")
		decision = permission.DecisionDeny
	case permOptAlwaysAllow:
		observe.GlobalTrace("case: permOptAlwaysAllow")
		decision = permission.DecisionAllow
		remember = true
	}

	resp := PermResponseMsg{Decision: decision, Remember: remember}
	observe.GlobalTrace("return: func() tea.Msg {\n\treq.Response <- resp\n\treturn resp\n}")
	return func() tea.Msg {
		req.Response <- resp
		return resp
	}
}

// deny sends a deny decision and deactivates the dialog.
func (d *permissionDialog) deny() tea.Cmd {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	req := d.request
	d.active = false
	d.request = nil

	resp := PermResponseMsg{Decision: permission.DecisionDeny}
	observe.GlobalTrace("return: func() tea.Msg {\n\treq.Response <- resp\n\treturn resp\n}")
	return func() tea.Msg {
		req.Response <- resp
		return resp
	}
}

// toolPreview holds parsed tool input fields for rendering previews.
type toolPreview struct {
	FilePath  string
	OldString string
	NewString string
	Content   string
	Command   string
}

// parseToolPreview extracts display-relevant fields from tool input JSON.
// Returns zero values on parse failure (graceful degradation).
func parseToolPreview(toolName string, input json.RawMessage) toolPreview {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(input) == 0 {
		observe.GlobalTrace("if: len(input) == 0")
		observe.GlobalTrace("return: toolPreview{}")
		return toolPreview{}
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(input, &raw) != nil {
		observe.GlobalTrace("if: json.Unmarshal(input, &raw) != nil")
		observe.GlobalTrace("return: toolPreview{}")
		return toolPreview{}
	}
	var p toolPreview
	getString := func(key string) string {
		v, ok := raw[key]
		if !ok {
			return ""
		}
		var s string
		if json.Unmarshal(v, &s) != nil {
			return ""
		}
		return s
	}
	switch toolName {
	case "Edit":
		observe.GlobalTrace("case: \"Edit\"")
		p.FilePath = getString("file_path")
		p.OldString = getString("old_string")
		p.NewString = getString("new_string")
	case "Write":
		observe.GlobalTrace("case: \"Write\"")
		p.FilePath = getString("file_path")
		p.Content = getString("content")
	case "Bash":
		observe.GlobalTrace("case: \"Bash\"")
		p.Command = getString("command")
	}
	observe.GlobalTrace("return: p")
	return p
}

// View renders the permission dialog.
func (d permissionDialog) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active || d.request == nil {
		observe.GlobalTrace("if: !d.active || d.request == nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	preview := parseToolPreview(d.request.ToolName, d.request.ToolInput)

	var b strings.Builder

	title, subtitle := permDialogTitle(d.request.ToolName, preview)
	b.WriteString(permTitleStyle.Render(title))
	if subtitle != "" {
		observe.GlobalTrace("if: subtitle != \"\"")
		b.WriteString("\n")
		b.WriteString(permSubtitleStyle.Render(subtitle))
	}
	b.WriteString("\n")

	content := permDialogContent(d.request, preview)
	if content != "" {
		observe.GlobalTrace("if: content != \"\"")
		b.WriteString("\n")
		b.WriteString(content)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	for i, label := range permOptionLabels {
		observe.GlobalTrace("range permOptionLabels")
		cursor := "  "
		style := permUnselectedStyle
		if i == d.selected {
			observe.GlobalTrace("if: i == d.selected")
			cursor = "> "
			style = permSelectedStyle
		}
		b.WriteString(cursor + style.Render(label) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(permUnselectedStyle.Render("[↑↓] navigate  [Enter] confirm  [Esc] deny"))
	observe.GlobalTrace("return: permDialogBorderStyle.Render(b.String())")

	return permDialogBorderStyle.Render(b.String())
}

// permDialogTitle returns the title and subtitle for a permission dialog.
func permDialogTitle(toolName string, preview toolPreview) (string, string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch toolName {
	case "Edit":
		observe.GlobalTrace("case: \"Edit\"")
		return "Edit file", preview.FilePath
	case "Write":
		observe.GlobalTrace("case: \"Write\"")
		return "Write file", preview.FilePath
	case "Bash":
		observe.GlobalTrace("case: \"Bash\"")
		return "Run command", ""
	default:
		observe.GlobalTrace("default")
		return toolName, ""
	}
}

// permDialogContent renders the tool-specific content area.
func permDialogContent(req *PermRequestMsg, preview toolPreview) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch req.ToolName {
	case "Edit":
		observe.GlobalTrace("case: \"Edit\"")
		return renderEditPreview(preview)
	case "Write":
		observe.GlobalTrace("case: \"Write\"")
		return renderWritePreview(preview)
	case "Bash":
		observe.GlobalTrace("case: \"Bash\"")
		return renderBashPreview(preview)
	default:
		observe.GlobalTrace("default")
		return renderDefaultPreview(req)
	}
}

// renderEditPreview renders old_string → new_string diff for Edit tool.
func renderEditPreview(p toolPreview) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if p.OldString == "" && p.NewString == "" {
		observe.GlobalTrace("if: p.OldString == \"\" && p.NewString == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	var b strings.Builder
	oldLines := strings.Split(p.OldString, "\n")
	newLines := strings.Split(p.NewString, "\n")

	// Truncate to avoid overwhelming the terminal (#48248, #46190)
	const maxLines = 20
	totalLines := len(oldLines) + len(newLines)
	truncated := totalLines > maxLines

	maxOld := len(oldLines)
	maxNew := len(newLines)
	if truncated {
		observe.GlobalTrace("if: truncated")
		maxOld = min(len(oldLines), maxLines/2)
		maxNew = min(len(newLines), maxLines-maxOld)
	}

	for i := 0; i < maxOld; i++ {
		observe.GlobalTrace("for: i < maxOld")
		b.WriteString("  ")
		b.WriteString(permDiffRemove.Render("- " + oldLines[i]))
		b.WriteString("\n")
	}
	if maxOld < len(oldLines) {
		observe.GlobalTrace("if: maxOld < len(oldLines)")
		b.WriteString("  " + permUnselectedStyle.Render(fmt.Sprintf("  ... (+%d lines)", len(oldLines)-maxOld)) + "\n")
	}
	for i := 0; i < maxNew; i++ {
		observe.GlobalTrace("for: i < maxNew")
		b.WriteString("  ")
		b.WriteString(permDiffAdd.Render("+ " + newLines[i]))
		b.WriteString("\n")
	}
	if maxNew < len(newLines) {
		observe.GlobalTrace("if: maxNew < len(newLines)")
		b.WriteString("  " + permUnselectedStyle.Render(fmt.Sprintf("  ... (+%d lines)", len(newLines)-maxNew)) + "\n")
	}

	result := b.String()
	if strings.HasSuffix(result, "\n") {
		observe.GlobalTrace("if: strings.HasSuffix(result, \"\\n\")")
		result = result[:len(result)-1]
	}
	observe.GlobalTrace("return: result")
	return result
}

// renderWritePreview renders a content preview for Write tool.
func renderWritePreview(p toolPreview) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if p.Content == "" {
		observe.GlobalTrace("if: p.Content == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	lines := strings.Split(p.Content, "\n")
	const maxLines = 10
	show := min(len(lines), maxLines)

	var b strings.Builder
	for i := 0; i < show; i++ {
		observe.GlobalTrace("for: i < show")
		b.WriteString("  ")
		b.WriteString(permDiffAdd.Render("+ " + lines[i]))
		b.WriteString("\n")
	}
	if len(lines) > maxLines {
		observe.GlobalTrace("if: len(lines) > maxLines")
		b.WriteString("  " + permUnselectedStyle.Render(fmt.Sprintf("(+%d more lines)", len(lines)-maxLines)) + "\n")
	}

	result := b.String()
	if strings.HasSuffix(result, "\n") {
		observe.GlobalTrace("if: strings.HasSuffix(result, \"\\n\")")
		result = result[:len(result)-1]
	}
	observe.GlobalTrace("return: result")
	return result
}

// renderBashPreview renders the command for Bash tool.
func renderBashPreview(p toolPreview) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if p.Command == "" {
		observe.GlobalTrace("if: p.Command == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	lines := strings.Split(p.Command, "\n")
	const maxLines = 5
	show := min(len(lines), maxLines)
	var b strings.Builder
	for i := 0; i < show; i++ {
		observe.GlobalTrace("for: i < show")
		b.WriteString("  ")
		b.WriteString(permCommandStyle.Render(lines[i]))
		b.WriteString("\n")
	}
	if len(lines) > maxLines {
		observe.GlobalTrace("if: len(lines) > maxLines")
		b.WriteString("  " + permUnselectedStyle.Render(fmt.Sprintf("... (+%d lines)", len(lines)-maxLines)) + "\n")
	}
	result := b.String()
	if strings.HasSuffix(result, "\n") {
		observe.GlobalTrace("if: strings.HasSuffix(result, \"\\n\")")
		result = result[:len(result)-1]
	}
	observe.GlobalTrace("return: result")
	return result
}

// renderDefaultPreview renders the fallback content for unknown tools.
// When Content is empty (most tools besides Edit/Write/Bash), parses ToolInput
// JSON to show top-level string fields as key=value pairs for context.
func renderDefaultPreview(req *PermRequestMsg) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder

	if req.Content != "" {
		observe.GlobalTrace("if: req.Content != \"\"")
		content := req.Content
		if lines := strings.Split(content, "\n"); len(lines) > 3 {
			observe.GlobalTrace("if: len(lines) > 3")
			content = strings.Join(lines[:3], "\n") + "..."
		}
		if len([]rune(content)) > 200 {
			observe.GlobalTrace("if: len([]rune(content)) > 200")
			content = string([]rune(content)[:197]) + "..."
		}
		b.WriteString(fmt.Sprintf("  %s", content))
	} else if len(req.ToolInput) > 0 {
		observe.GlobalTrace("else-if: len(req.ToolInput) > 0")
		b.WriteString(renderInputFields(req.ToolInput))
	}

	if req.Reason != "" {
		observe.GlobalTrace("if: req.Reason != \"\"")
		if b.Len() > 0 {
			observe.GlobalTrace("if: b.Len() > 0")
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("  %s", permUnselectedStyle.Render(req.Reason)))
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// renderInputFields extracts top-level string fields from tool input JSON
// and renders them as indented key: "value" lines (max 3 fields, 80-char values).
func renderInputFields(input json.RawMessage) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var raw map[string]json.RawMessage
	if json.Unmarshal(input, &raw) != nil {
		observe.GlobalTrace("if: json.Unmarshal(input, &raw) != nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	var b strings.Builder
	shown := 0
	const maxFields = 3
	const maxValueLen = 80
	for key, val := range raw {
		observe.GlobalTrace("range raw")
		if shown >= maxFields {
			observe.GlobalTrace("if: shown >= maxFields")
			break
		}
		var s string
		if json.Unmarshal(val, &s) != nil {
			observe.GlobalTrace("if: json.Unmarshal(val, &s) != nil")
			continue
		}
		if s == "" {
			observe.GlobalTrace("if: s == \"\"")
			continue
		}
		if len([]rune(s)) > maxValueLen {
			observe.GlobalTrace("if: len([]rune(s)) > maxValueLen")
			s = string([]rune(s)[:maxValueLen-3]) + "..."
		}
		b.WriteString(fmt.Sprintf("  %s: %q\n", key, s))
		shown++
	}
	result := b.String()
	if strings.HasSuffix(result, "\n") {
		observe.GlobalTrace("if: strings.HasSuffix(result, \"\\n\")")
		result = result[:len(result)-1]
	}
	observe.GlobalTrace("return: result")
	return result
}
