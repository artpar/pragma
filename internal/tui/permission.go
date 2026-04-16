package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
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
	var rule *permission.Rule

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
		rule = &permission.Rule{
			ToolName: req.ToolName,
			Content:  req.Content,
			Decision: permission.DecisionAllow,
			Source:   permission.SourceSession,
		}
	}

	resp := PermResponseMsg{Decision: decision, Rule: rule}
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

// View renders the permission dialog box.
func (d permissionDialog) View() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !d.active || d.request == nil {
		observe.GlobalTrace("if: !d.active || d.request == nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	var b strings.Builder
	b.WriteString(permTitleStyle.Render("Permission Required"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Tool:    %s\n", d.request.ToolName))
	b.WriteString(fmt.Sprintf("Content: %s\n", truncateStr(d.request.Content, 80)))
	if d.request.Reason != "" {
		observe.GlobalTrace("if: d.request.Reason != \"\"")
		b.WriteString(fmt.Sprintf("Reason:  %s\n", d.request.Reason))
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

// truncateStr truncates a string to maxLen runes (not bytes).
// Safe for multi-byte UTF-8 characters.
func truncateStr(s string, maxLen int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	runes := []rune(s)
	if len(runes) <= maxLen {
		observe.GlobalTrace("if: len(runes) <= maxLen")
		observe.GlobalTrace("return: s")
		return s
	}
	observe.GlobalTrace("return: string(runes[:maxLen-3]) + \"...\"")
	return string(runes[:maxLen-3]) + "..."
}
