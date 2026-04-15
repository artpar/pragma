package bridge

import (
	"fmt"

	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/tool"
)

// Infra holds the gogent infrastructure needed by bridge node factories.
// Passed once at wiring time — nodes capture what they need via closures.
type Infra struct {
	Provider     provider.Provider
	Orchestrator *tool.Orchestrator
	Registry     *tool.Registry
	Bus          *observe.EventBus
	Cwd          string
}

// NodeFactory resolves node type names + config into lifecycle.NodeFunc values.
type NodeFactory struct {
	infra Infra
}

// NewNodeFactory creates a NodeFactory backed by the given infrastructure.
func NewNodeFactory(infra Infra) *NodeFactory {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &NodeFactory{infra: infra}")
	return &NodeFactory{infra: infra}
}

// Create resolves a node definition into a NodeFunc.
// nodeType: "llm", "tools", "eval", "reflect"
// config keys depend on the node type:
//
//	llm:     "prompt" (string), "model" (string), "temperature" (float64)
//	tools:   (no config needed — uses orchestrator from Infra)
//	eval:    "criteria" (string), "model" (string)
//	reflect: (no config needed)
func (f *NodeFactory) Create(nodeType string, config map[string]any) (lifecycle.NodeFunc, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch nodeType {
	case "llm":
		observe.GlobalTrace("case: \"llm\"")
		cfg := LLMNodeConfig{}
		if v, ok := config["prompt"].(string); ok {
			cfg.NodePrompt = v
		}
		if v, ok := config["temperature"].(float64); ok {
			cfg.Temperature = &v
		}
		return LLMNode(f.infra.Provider, f.infra.Bus, cfg), nil

	case "tools":
		observe.GlobalTrace("case: \"tools\"")
		return ToolNode(f.infra.Orchestrator, f.infra.Cwd), nil

	case "eval":
		observe.GlobalTrace("case: \"eval\"")
		cfg := EvalNodeConfig{}
		if v, ok := config["criteria"].(string); ok {
			cfg.Criteria = v
		}
		if v, ok := config["model"].(string); ok {
			cfg.Model = v
		}
		return EvalNode(f.infra.Provider, f.infra.Bus, cfg), nil

	case "reflect":
		observe.GlobalTrace("case: \"reflect\"")
		return ReflectNode(f.infra.Provider, f.infra.Bus), nil

	default:
		observe.GlobalTrace("default")
		return nil, fmt.Errorf("unknown node type: %q", nodeType)
	}
}
