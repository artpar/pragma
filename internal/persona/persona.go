package persona

import (
	"fmt"
	"os"

	"github.com/artpar/pragma/internal/llmconfig"
	"github.com/artpar/pragma/internal/observe"
	"gopkg.in/yaml.v3"
)

type Definition struct {
	ID          string            `yaml:"id"`
	Description string            `yaml:"description,omitempty"`
	Properties  map[string]string `yaml:"properties,omitempty"`
	LLM         llmconfig.Config  `yaml:"llm,omitempty"`
	Prompt      string            `yaml:"prompt"`
}

func LoadDefinitionFile(path string) (Definition, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Definition{}, err")
		return Definition{}, err
	}
	var def Definition
	if err := yaml.Unmarshal(raw, &def); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Definition{}, fmt.Errorf(\"parse persona definition %q: %w\", path, err)")
		return Definition{}, fmt.Errorf("parse persona definition %q: %w", path, err)
	}
	if def.ID == "" {
		observe.GlobalTrace("if: def.ID == \"\"")
		observe.GlobalTrace("return: Definition{}, fmt.Errorf(\"persona definition %q requires an id\", path)")
		return Definition{}, fmt.Errorf("persona definition %q requires an id", path)
	}
	if def.Prompt == "" {
		observe.GlobalTrace("if: def.Prompt == \"\"")
		observe.GlobalTrace("return: Definition{}, fmt.Errorf(\"persona definition %q requires a prompt\", path)")
		return Definition{}, fmt.Errorf("persona definition %q requires a prompt", path)
	}
	observe.GlobalTrace("return: def, nil")
	return def, nil
}
