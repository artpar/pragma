package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/gogent/internal/permission"
)

const (
	permOptAllow       = 0
	permOptDeny        = 1
	permOptAlwaysAllow = 2
	permOptCount       = 3
)

var permOptionLabels = [permOptCount]string{
	"Allow (once)",
	"Deny",
	"Always Allow (session)",
}

// permissionDialog renders an interactive permission prompt.
type permissionDialog struct {
	active   bool
	request  *PermRequestMsg
	selected int
}

func newPermissionDialog() permissionDialog {
	return permissionDialog{}
}

// Show activates the dialog for a permission request.
func (d *permissionDialog) Show(req *PermRequestMsg) {
	d.active = true
	d.request = req
	d.selected = permOptAllow
}

// Update handles key events when the dialog is active.
// Returns a tea.Cmd if the user has made a decision (nil otherwise).
func (d *permissionDialog) Update(msg tea.Msg) tea.Cmd {
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
	req := d.request
	d.active = false
	d.request = nil

	resp := PermResponseMsg{Decision: permission.DecisionDeny}
	return func() tea.Msg {
		req.Response <- resp
		return resp
	}
}

// View renders the permission dialog box.
func (d permissionDialog) View() string {
	if !d.active || d.request == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(permTitleStyle.Render("Permission Required"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Tool:    %s\n", d.request.ToolName))
	b.WriteString(fmt.Sprintf("Content: %s\n", truncateStr(d.request.Content, 80)))
	if d.request.Reason != "" {
		b.WriteString(fmt.Sprintf("Reason:  %s\n", d.request.Reason))
	}
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

	return permDialogBorderStyle.Render(b.String())
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
