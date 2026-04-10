package sysprompt

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// maxAgentMDBytes caps AGENT.md file size to prevent system prompt explosion.
const maxAgentMDBytes = 25 * 1024

// AgentMDSource is a loaded AGENT.md file with its metadata.
type AgentMDSource struct {
	Path    string // absolute path
	Content string // frontmatter stripped, possibly truncated
	Scope   string // "global", "project", "local"
}

// LoadAgentMD reads AGENT.md files from all scopes in priority order.
// Missing files are silently skipped (with event emission).
func LoadAgentMD(workDir string, bus *observe.EventBus) []AgentMDSource {
	type candidate struct {
		path  string
		scope string
	}

	globalPath, globalErr := config.GlobalAgentMDPath()
	if globalErr != nil && bus != nil {
		bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
			Severity:     "warning",
			Component:    "sysprompt",
			ErrorType:    "home_dir_resolution",
			ErrorMessage: fmt.Sprintf("cannot resolve global AGENT.md path: %v", globalErr),
		})
	}
	candidates := []candidate{
		{globalPath, "global"},
		{config.ProjectAgentMDPath(workDir), "project"},
		{config.LocalAgentMDPath(workDir), "local"},
	}

	var sources []AgentMDSource
	for _, c := range candidates {
		if c.path == "" {
			continue
		}
		data, err := os.ReadFile(c.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if bus != nil {
					bus.Emit(observe.AgentMDNotFound{
						EventHeader: observe.NewEventHeader("AgentMDNotFound", "", observe.NewSpanID(), ""),
						Path:        c.path,
						Scope:       c.scope,
					})
				}
				continue
			}
			// Permission denied or other I/O errors: warn and skip
			if bus != nil {
				bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
					Severity:     "warning",
					Component:    "sysprompt",
					ErrorType:    "agent_md_read_error",
					ErrorMessage: fmt.Sprintf("cannot read %s (%s): %v", c.path, c.scope, err),
				})
			}
			continue
		}

		content := stripFrontmatter(string(data))
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}

		// Truncate oversized files
		if len(content) > maxAgentMDBytes {
			content = content[:maxAgentMDBytes] + "\n\n[truncated — file exceeds 25KB limit]"
		}

		sources = append(sources, AgentMDSource{
			Path:    c.path,
			Content: content,
			Scope:   c.scope,
		})

		if bus != nil {
			bus.Emit(observe.AgentMDLoaded{
				EventHeader: observe.NewEventHeader("AgentMDLoaded", "", observe.NewSpanID(), ""),
				Path:        c.path,
				Scope:       c.scope,
				Bytes:       len(content),
			})
		}
	}
	return sources
}

// stripFrontmatter removes YAML frontmatter (content between --- markers)
// from the beginning of a string. If no frontmatter is found, returns the
// original string unchanged.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	// Find the closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		// Unclosed frontmatter — return original
		return content
	}
	// Skip past the closing --- and its newline
	after := rest[idx+4:]
	if len(after) > 0 && after[0] == '\n' {
		after = after[1:]
	}
	return after
}

// agentMDBlock formats loaded AGENT.md sources into a single SystemBlock
// with path headers for attribution.
func agentMDBlock(sources []AgentMDSource) model.SystemBlock {
	if len(sources) == 0 {
		return model.SystemBlock{}
	}

	var b strings.Builder
	b.WriteString("Codebase and user instructions are shown below. Be sure to adhere to these instructions.\n")
	b.WriteString("IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written.\n\n")

	for i, src := range sources {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "Contents of %s (%s):\n\n", src.Path, scopeDescription(src.Scope))
		b.WriteString(src.Content)
		b.WriteString("\n")
	}

	return model.SystemBlock{Text: b.String(), Cacheable: false}
}

func scopeDescription(scope string) string {
	switch scope {
	case "global":
		return "global instructions"
	case "project":
		return "project instructions"
	case "local":
		return "local instructions, not checked into version control"
	default:
		return scope
	}
}
