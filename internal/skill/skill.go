package skill

import (
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"gopkg.in/yaml.v3"
)

// Skill represents a user-defined skill loaded from a SKILL.md file.
type Skill struct {
	Name         string   // directory name, normalized to lowercase
	Description  string   // from frontmatter
	Content      string   // markdown body after frontmatter
	WhenToUse    string   // helps LLM decide when to invoke
	AllowedTools []string // tool scope restriction (nil = all tools)
	Model        string   // model override for execution
	Context      string   // "inline" (default) or "fork"
	Arguments    []string // named argument definitions
	BaseDir      string   // directory containing SKILL.md
	Source       string   // "user" or "project"
}

// IsForked returns true if the skill runs as a forked sub-agent.
func (s *Skill) IsForked() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.EqualFold(s.Context, \"fork\")")
	return strings.EqualFold(s.Context, "fork")
}

// ParseFrontmatter splits a raw SKILL.md file into YAML metadata and body.
// If no frontmatter delimiters are found, the entire content is returned as body.
func ParseFrontmatter(raw string) (meta map[string]any, body string, err error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	const delimiter = "---"

	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, delimiter) {
		observe.GlobalTrace("if: !strings.HasPrefix(trimmed, delimiter)")
		observe.GlobalTrace("return: nil, raw, nil")
		return nil, raw, nil
	}

	rest := trimmed[len(delimiter):]
	idx := strings.Index(rest, "\n"+delimiter)
	if idx < 0 {
		observe.GlobalTrace("if: idx < 0")
		observe.GlobalTrace("return: nil, raw, nil")

		return nil, raw, nil
	}

	yamlBlock := rest[:idx]

	afterDelimiter := rest[idx+len("\n"+delimiter):]
	if len(afterDelimiter) > 0 && afterDelimiter[0] == '\n' {
		observe.GlobalTrace("if: len(afterDelimiter) > 0 && afterDelimiter[0] == '\\n'")
		afterDelimiter = afterDelimiter[1:]
	} else if len(afterDelimiter) > 1 && afterDelimiter[0] == '\r' && afterDelimiter[1] == '\n' {
		observe.GlobalTrace("else-if: len(afterDelimiter) > 1 && afterDelimiter[0] == '\\r' && afterDelimiter[1] == ...")
		afterDelimiter = afterDelimiter[2:]
	}
	body = afterDelimiter

	meta = make(map[string]any)
	if err := yaml.Unmarshal([]byte(yamlBlock), &meta); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, raw, nil")

		return nil, raw, nil
	}
	observe.GlobalTrace("return: meta, body, nil")

	return meta, body, nil
}

// skillFromMeta populates a Skill from parsed frontmatter metadata.
func skillFromMeta(name, baseDir, source string, meta map[string]any, body string) Skill {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s := Skill{
		Name:    strings.ToLower(name),
		Content: body,
		BaseDir: baseDir,
		Source:  source,
	}

	if v, ok := meta["description"].(string); ok {
		observe.GlobalTrace("if: ok")
		s.Description = v
	}
	if v, ok := meta["when_to_use"].(string); ok {
		observe.GlobalTrace("if: ok")
		s.WhenToUse = v
	}
	if v, ok := meta["model"].(string); ok {
		observe.GlobalTrace("if: ok")
		s.Model = v
	}
	if v, ok := meta["context"].(string); ok {
		observe.GlobalTrace("if: ok")
		s.Context = v
	}

	s.AllowedTools = parseStringOrList(meta, "allowed-tools")
	s.Arguments = parseStringOrList(meta, "arguments")
	observe.GlobalTrace("return: s")

	return s
}

// parseStringOrList reads a frontmatter field that can be either a string
// (comma-separated) or a YAML list.
func parseStringOrList(meta map[string]any, key string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	val, ok := meta[key]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil")
		return nil
	}

	switch v := val.(type) {
	case string:
		observe.GlobalTrace("typecase: string")
		if v == "" {
			observe.GlobalTrace("return: nil")
			return nil
		}
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				observe.GlobalTrace("if: p != \"\"")
				result = append(result, p)
			}
		}
		return result
	case []any:
		observe.GlobalTrace("typecase: []any")
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				observe.GlobalTrace("if: ok && s != \"\"")
				result = append(result, strings.TrimSpace(s))
			}
		}
		return result
	default:
		observe.GlobalTrace("typedefault")
		return nil
	}
}
