package definition

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Parse reads YAML data and returns a GraphDef.
func Parse(data []byte) (*GraphDef, error) {
	var def GraphDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse graph definition: %w", err)
	}
	if err := validate(&def); err != nil {
		return nil, err
	}
	return &def, nil
}

// ParseFile reads a YAML file and returns a GraphDef.
func ParseFile(path string) (*GraphDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read graph definition: %w", err)
	}
	return Parse(data)
}

// validate checks basic structural requirements on a parsed GraphDef.
func validate(def *GraphDef) error {
	if def.Graph.Initial == "" {
		return fmt.Errorf("graph definition: initial node is required")
	}
	if len(def.Graph.Nodes) == 0 {
		return fmt.Errorf("graph definition: at least one node is required")
	}
	if _, ok := def.Graph.Nodes[def.Graph.Initial]; !ok {
		return fmt.Errorf("graph definition: initial node %q not found in nodes", def.Graph.Initial)
	}
	for _, e := range def.Graph.Edges {
		if _, ok := def.Graph.Nodes[e.From]; !ok {
			return fmt.Errorf("graph definition: edge from unknown node %q", e.From)
		}
		if _, ok := def.Graph.Nodes[e.To]; !ok {
			return fmt.Errorf("graph definition: edge to unknown node %q", e.To)
		}
	}
	for _, ce := range def.Graph.ConditionalEdges {
		if _, ok := def.Graph.Nodes[ce.From]; !ok {
			return fmt.Errorf("graph definition: conditional edge from unknown node %q", ce.From)
		}
		for _, target := range ce.Paths {
			if target != "" {
				if _, ok := def.Graph.Nodes[target]; !ok {
					return fmt.Errorf("graph definition: conditional edge path to unknown node %q", target)
				}
			}
		}
	}
	return nil
}
