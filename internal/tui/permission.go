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
		return nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}

	switch keyMsg.String() {
	case "up", "k":
		d.selected--
		if d.selected < 0 {
			d.selected = permOptCount - 1
		}
	case "down", "j":
		d.selected++
		if d.selected >= permOptCount {
			d.selected = 0
		}
	case "enter":
		return d.confirm()
	case "esc":
		return d.deny()
	}
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
	var rule *permission.Rule

	switch d.selected {
	case permOptAllow:
		decision = permission.DecisionAllow
	case permOptDeny:
		decision = permission.DecisionDeny
	case permOptAlwaysAllow:
		decision = permission.DecisionAllow
		rule = &permission.Rule{
			ToolName: req.ToolName,
			Content:  req.Content,
			Decision: permission.DecisionAllow,
			Source:   permission.SourceSession,
		}
	}

	resp := PermResponseMsg{Decision: decision, Rule: rule}
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
	if len(input) == 0 {
		return toolPreview{}
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(input, &raw) != nil {
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
		p.FilePath = getString("file_path")
		p.OldString = getString("old_string")
		p.NewString = getString("new_string")
	case "Write":
		p.FilePath = getString("file_path")
		p.Content = getString("content")
	case "Bash":
		p.Command = getString("command")
	}
	return p
}

// View renders the permission dialog.
func (d permissionDialog) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active || d.request == nil {
		return ""
	}

	preview := parseToolPreview(d.request.ToolName, d.request.ToolInput)

	var b strings.Builder

	// Title + subtitle (matching TS PermissionRequestTitle)
	title, subtitle := permDialogTitle(d.request.ToolName, preview)
	b.WriteString(permTitleStyle.Render(title))
	if subtitle != "" {
		b.WriteString("\n")
		b.WriteString(permSubtitleStyle.Render(subtitle))
	}
	b.WriteString("\n")

	// Content area (tool-specific preview)
	content := permDialogContent(d.request, preview)
	if content != "" {
		b.WriteString("\n")
		b.WriteString(content)
		b.WriteString("\n")
	}

	// Options
	b.WriteString("\n")
	for i, label := range permOptionLabels {
		cursor := "  "
		style := permUnselectedStyle
		if i == d.selected {
			cursor = "> "
			style = permSelectedStyle
		}
		b.WriteString(cursor + style.Render(label) + "\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(permUnselectedStyle.Render("[↑↓] navigate  [Enter] confirm  [Esc] deny"))

	return permDialogBorderStyle.Render(b.String())
}

// permDialogTitle returns the title and subtitle for a permission dialog.
func permDialogTitle(toolName string, preview toolPreview) (string, string) {
	switch toolName {
	case "Edit":
		return "Edit file", preview.FilePath
	case "Write":
		return "Write file", preview.FilePath
	case "Bash":
		return "Run command", ""
	default:
		return "Tool use", ""
	}
}

// permDialogContent renders the tool-specific content area.
func permDialogContent(req *PermRequestMsg, preview toolPreview) string {
	switch req.ToolName {
	case "Edit":
		return renderEditPreview(preview)
	case "Write":
		return renderWritePreview(preview)
	case "Bash":
		return renderBashPreview(preview)
	default:
		return renderDefaultPreview(req)
	}
}

// renderEditPreview renders old_string → new_string diff for Edit tool.
func renderEditPreview(p toolPreview) string {
	if p.OldString == "" && p.NewString == "" {
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
		maxOld = min(len(oldLines), maxLines/2)
		maxNew = min(len(newLines), maxLines-maxOld)
	}

	for i := 0; i < maxOld; i++ {
		b.WriteString("  ")
		b.WriteString(permDiffRemove.Render("- " + oldLines[i]))
		b.WriteString("\n")
	}
	if maxOld < len(oldLines) {
		b.WriteString("  " + permUnselectedStyle.Render(fmt.Sprintf("  ... (+%d lines)", len(oldLines)-maxOld)) + "\n")
	}
	for i := 0; i < maxNew; i++ {
		b.WriteString("  ")
		b.WriteString(permDiffAdd.Render("+ " + newLines[i]))
		b.WriteString("\n")
	}
	if maxNew < len(newLines) {
		b.WriteString("  " + permUnselectedStyle.Render(fmt.Sprintf("  ... (+%d lines)", len(newLines)-maxNew)) + "\n")
	}

	// Trim trailing newline
	result := b.String()
	if strings.HasSuffix(result, "\n") {
		result = result[:len(result)-1]
	}
	return result
}

// renderWritePreview renders a content preview for Write tool.
func renderWritePreview(p toolPreview) string {
	if p.Content == "" {
		return ""
	}
	lines := strings.Split(p.Content, "\n")
	const maxLines = 10
	show := min(len(lines), maxLines)

	var b strings.Builder
	for i := 0; i < show; i++ {
		b.WriteString("  ")
		b.WriteString(permDiffAdd.Render("+ " + lines[i]))
		b.WriteString("\n")
	}
	if len(lines) > maxLines {
		b.WriteString("  " + permUnselectedStyle.Render(fmt.Sprintf("(+%d more lines)", len(lines)-maxLines)) + "\n")
	}

	result := b.String()
	if strings.HasSuffix(result, "\n") {
		result = result[:len(result)-1]
	}
	return result
}

// renderBashPreview renders the command for Bash tool.
func renderBashPreview(p toolPreview) string {
	if p.Command == "" {
		return ""
	}
	// Truncate very long commands (#48248)
	lines := strings.Split(p.Command, "\n")
	const maxLines = 5
	show := min(len(lines), maxLines)
	var b strings.Builder
	for i := 0; i < show; i++ {
		b.WriteString("  ")
		b.WriteString(permCommandStyle.Render(lines[i]))
		b.WriteString("\n")
	}
	if len(lines) > maxLines {
		b.WriteString("  " + permUnselectedStyle.Render(fmt.Sprintf("... (+%d lines)", len(lines)-maxLines)) + "\n")
	}
	result := b.String()
	if strings.HasSuffix(result, "\n") {
		result = result[:len(result)-1]
	}
	return result
}

// renderDefaultPreview renders the fallback content for unknown tools.
func renderDefaultPreview(req *PermRequestMsg) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("  Tool:    %s\n", req.ToolName))
	content := req.Content
	// Truncate to 3 lines (matching TS truncateToLines(description, 3))
	if lines := strings.Split(content, "\n"); len(lines) > 3 {
		content = strings.Join(lines[:3], "\n") + "..."
	}
	if len([]rune(content)) > 200 {
		content = string([]rune(content)[:197]) + "..."
	}
	b.WriteString(fmt.Sprintf("  Content: %s", content))
	if req.Reason != "" {
		b.WriteString(fmt.Sprintf("\n  Reason:  %s", req.Reason))
	}
	return b.String()
}
