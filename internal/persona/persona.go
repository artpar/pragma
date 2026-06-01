package persona

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Definition struct {
	ID          string            `yaml:"id"`
	Description string            `yaml:"description,omitempty"`
	Properties  map[string]string `yaml:"properties,omitempty"`
	Prompt      string            `yaml:"prompt"`
}

func LoadDefinitionFile(path string) (Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}
	var def Definition
	if err := yaml.Unmarshal(raw, &def); err != nil {
		return Definition{}, fmt.Errorf("parse persona definition %q: %w", path, err)
	}
	if def.ID == "" {
		return Definition{}, fmt.Errorf("persona definition %q requires an id", path)
	}
	if def.Prompt == "" {
		return Definition{}, fmt.Errorf("persona definition %q requires a prompt", path)
	}
	return def, nil
}
