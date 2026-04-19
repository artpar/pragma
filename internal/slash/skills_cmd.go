package slash

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/observe"
)

func handleSkills(_ context.Context, args string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if deps.SkillLoader == nil {
		return Result{DisplayText: "Skills loader not available."}, nil
	}

	args = strings.TrimSpace(args)

	// Detail view for a specific skill
	if args != "" {
		s, err := deps.SkillLoader.Load(args)
		if err != nil {
			return Result{DisplayText: fmt.Sprintf("Skill not found: %s", args)}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Skill: %s\n", s.Name)
		b.WriteString(strings.Repeat("─", 40) + "\n\n")

		if s.Description != "" {
			fmt.Fprintf(&b, "  description:  %s\n", s.Description)
		}
		if s.Source != "" {
			fmt.Fprintf(&b, "  source:       %s\n", s.Source)
		}
		if s.Context != "" {
			fmt.Fprintf(&b, "  context:      %s\n", s.Context)
		}
		if s.Model != "" {
			fmt.Fprintf(&b, "  model:        %s\n", s.Model)
		}
		if len(s.AllowedTools) > 0 {
			fmt.Fprintf(&b, "  tools:        %s\n", strings.Join(s.AllowedTools, ", "))
		}
		if len(s.Arguments) > 0 {
			fmt.Fprintf(&b, "  arguments:    %s\n", strings.Join(s.Arguments, ", "))
		}
		if s.WhenToUse != "" {
			fmt.Fprintf(&b, "  when_to_use:  %s\n", s.WhenToUse)
		}

		if s.Content != "" {
			b.WriteString("\nContent:\n")
			lines := strings.Split(s.Content, "\n")
			limit := 10
			if len(lines) < limit {
				limit = len(lines)
			}
			for _, line := range lines[:limit] {
				fmt.Fprintf(&b, "  %s\n", line)
			}
			if len(lines) > limit {
				fmt.Fprintf(&b, "  ... (%d more lines)\n", len(lines)-limit)
			}
		}

		return Result{DisplayText: b.String()}, nil
	}

	// List all skills
	skills, err := deps.SkillLoader.LoadAll()
	if err != nil {
		return Result{DisplayText: fmt.Sprintf("Error loading skills: %s", err)}, nil
	}

	if len(skills) == 0 {
		return Result{DisplayText: "No skills found.\n\nSkills can be added to ~/.pragma/skills/ or .pragma/skills/"}, nil
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	var b strings.Builder
	fmt.Fprintf(&b, "Available Skills (%d)\n", len(skills))
	b.WriteString(strings.Repeat("─", 40) + "\n\n")

	for _, s := range skills {
		desc := s.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		ctx := s.Context
		if ctx == "" {
			ctx = "inline"
		}
		fmt.Fprintf(&b, "  %-20s %-8s %-8s %s\n", s.Name, s.Source, ctx, desc)
	}

	b.WriteString("\nRun /skills <name> for details.")

	return Result{DisplayText: b.String()}, nil
}
