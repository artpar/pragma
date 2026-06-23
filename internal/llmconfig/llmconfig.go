package llmconfig

import (
	"strings"
	"github.com/artpar/pragma/internal/observe"
)

// Config describes an optional LLM runtime override for a persona or state.
type Config struct {
	Provider       string   `yaml:"provider,omitempty"`
	Model          string   `yaml:"model,omitempty"`
	MaxTokens      int      `yaml:"max_tokens,omitempty"`
	Temperature    *float64 `yaml:"temperature,omitempty"`
	Thinking       *bool    `yaml:"thinking,omitempty"`
	ThinkingBudget int      `yaml:"thinking_budget,omitempty"`
	SystemPrompt   string   `yaml:"system_prompt,omitempty"`
}

func (c Config) IsZero() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.TrimSpace(c.Provider) == \"\" &&\n\tstrings.TrimSpace(c.Model) == \"\" &&\n\t...")
	return strings.TrimSpace(c.Provider) == "" &&
		strings.TrimSpace(c.Model) == "" &&
		c.MaxTokens == 0 &&
		c.Temperature == nil &&
		c.Thinking == nil &&
		c.ThinkingBudget == 0 &&
		strings.TrimSpace(c.SystemPrompt) == ""
}

// Merge overlays non-zero override fields on top of base.
func Merge(base, override Config) Config {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := base
	if strings.TrimSpace(override.Provider) != "" {
		observe.GlobalTrace("if: strings.TrimSpace(override.Provider) != \"\"")
		out.Provider = override.Provider
	}
	if strings.TrimSpace(override.Model) != "" {
		observe.GlobalTrace("if: strings.TrimSpace(override.Model) != \"\"")
		out.Model = override.Model
	}
	if override.MaxTokens != 0 {
		observe.GlobalTrace("if: override.MaxTokens != 0")
		out.MaxTokens = override.MaxTokens
	}
	if override.Temperature != nil {
		observe.GlobalTrace("if: override.Temperature != nil")
		out.Temperature = override.Temperature
	}
	if override.Thinking != nil {
		observe.GlobalTrace("if: override.Thinking != nil")
		out.Thinking = override.Thinking
	}
	if override.ThinkingBudget != 0 {
		observe.GlobalTrace("if: override.ThinkingBudget != 0")
		out.ThinkingBudget = override.ThinkingBudget
	}
	if strings.TrimSpace(override.SystemPrompt) != "" {
		observe.GlobalTrace("if: strings.TrimSpace(override.SystemPrompt) != \"\"")
		out.SystemPrompt = override.SystemPrompt
	}
	observe.GlobalTrace("return: out")
	return out
}
