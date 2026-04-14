package definition

import (
	"fmt"
	"os"

	"github.com/artpar/gogent/internal/observe"
	"gopkg.in/yaml.v3"
)

// Parse reads YAML data and returns a GraphDef.
func Parse(data []byte) (*GraphDef, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var def GraphDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"parse graph definition: %w\", err)")
		return nil, fmt.Errorf("parse graph definition: %w", err)
	}
	if err := validate(&def); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	observe.GlobalTrace("return: &def, nil")
	return &def, nil
}

// ParseFile reads a YAML file and returns a GraphDef.
func ParseFile(path string) (*GraphDef, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read graph definition: %w\", err)")
		return nil, fmt.Errorf("read graph definition: %w", err)
	}
	observe.GlobalTrace("return: Parse(data)")
	return Parse(data)
}

// validate checks basic structural requirements on a parsed GraphDef.
func validate(def *GraphDef) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if def.Graph.Initial == "" {
		observe.GlobalTrace("if: def.Graph.Initial == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"graph definition: initial node is required\")")
		return fmt.Errorf("graph definition: initial node is required")
	}
	if len(def.Graph.Nodes) == 0 {
		observe.GlobalTrace("if: len(def.Graph.Nodes) == 0")
		observe.GlobalTrace("return: fmt.Errorf(\"graph definition: at least one node is required\")")
		return fmt.Errorf("graph definition: at least one node is required")
	}
	if _, ok := def.Graph.Nodes[def.Graph.Initial]; !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: fmt.Errorf(\"graph definition: initial node %q not found in nodes\", def.Graph....")
		return fmt.Errorf("graph definition: initial node %q not found in nodes", def.Graph.Initial)
	}
	for _, e := range def.Graph.Edges {
		observe.GlobalTrace("range def.Graph.Edges")
		if _, ok := def.Graph.Nodes[e.From]; !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: fmt.Errorf(\"graph definition: edge from unknown node %q\", e.From)")
			return fmt.Errorf("graph definition: edge from unknown node %q", e.From)
		}
		if _, ok := def.Graph.Nodes[e.To]; !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: fmt.Errorf(\"graph definition: edge to unknown node %q\", e.To)")
			return fmt.Errorf("graph definition: edge to unknown node %q", e.To)
		}
	}
	for _, ce := range def.Graph.ConditionalEdges {
		observe.GlobalTrace("range def.Graph.ConditionalEdges")
		if _, ok := def.Graph.Nodes[ce.From]; !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: fmt.Errorf(\"graph definition: conditional edge from unknown node %q\", ce.From)")
			return fmt.Errorf("graph definition: conditional edge from unknown node %q", ce.From)
		}
		for _, target := range ce.Paths {
			observe.GlobalTrace("range ce.Paths")
			if target != "" {
				observe.GlobalTrace("if: target != \"\"")
				if _, ok := def.Graph.Nodes[target]; !ok {
					observe.GlobalTrace("if: !ok")
					observe.GlobalTrace("return: fmt.Errorf(\"graph definition: conditional edge path to unknown node %q\", target)")
					return fmt.Errorf("graph definition: conditional edge path to unknown node %q", target)
				}
			}
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}
