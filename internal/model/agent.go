package model

// Agent is a definition loaded from .pragma/agents/.
// At runtime, spawning an agent = fork conversation + create Engine with agent's config.
// The Provider field allows sub-agents to use a different LLM entirely.
type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Model        string   `json:"model"`
	Provider     string   `json:"provider,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
}
