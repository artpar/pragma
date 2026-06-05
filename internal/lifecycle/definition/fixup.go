package definition

import (
	"fmt"
	"github.com/artpar/pragma/internal/observe"
)

// FixLLMToolRouting ensures every LLM node has stop_reason routing to a tools
// node. LLM-generated graphs frequently omit this routing, causing tool calls
// from LLM nodes to be silently discarded when the graph unconditionally
// transitions to the next node.
//
// When only one LLM node needs fixing and an existing tools node has no
// conflicting back-edges, the existing tools node is reused. Otherwise, each
// LLM node gets a dedicated tools node (named "<llm>_tools") to prevent the
// executor from running multiple LLM nodes in parallel after tools execution.
func FixLLMToolRouting(def *GraphDef) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if def == nil || len(def.Graph.Nodes) == 0 {
		observe.GlobalTrace("if: def == nil || len(def.Graph.Nodes) == 0")
		return
	}

	hasConditionalRouting := make(map[string]bool)
	for _, ce := range def.Graph.ConditionalEdges {
		observe.GlobalTrace("range def.Graph.ConditionalEdges")
		hasConditionalRouting[ce.From] = true
	}

	// Collect LLM nodes that need fixing: they have non-END unconditional edges
	// and no conditional routing.
	type fixTarget struct {
		nodeName string
		edgeIdx  int // index in def.Graph.Edges
		target   string
	}
	var fixes []fixTarget

	for i, e := range def.Graph.Edges {
		observe.GlobalTrace("range def.Graph.Edges")
		spec, ok := def.Graph.Nodes[e.From]
		if !ok || spec.Type != "llm" {
			observe.GlobalTrace("if: !ok || spec.Type != \"llm\"")
			continue
		}
		if e.To == "" {
			observe.GlobalTrace("if: e.To == \"\"")
			continue
		}
		if hasConditionalRouting[e.From] {
			observe.GlobalTrace("if: hasConditionalRouting[e.From]")
			continue
		}
		fixes = append(fixes, fixTarget{
			nodeName: e.From,
			edgeIdx:  i,
			target:   e.To,
		})
	}

	if len(fixes) == 0 {
		observe.GlobalTrace("if: len(fixes) == 0")
		return
	}

	existingToolsNode := ""
	for name, spec := range def.Graph.Nodes {
		observe.GlobalTrace("range def.Graph.Nodes")
		if spec.Type == "tools" {
			observe.GlobalTrace("if: spec.Type == \"tools\"")
			existingToolsNode = name
			break
		}
	}

	existingToolsBackEdgeTarget := ""
	existingToolsBackEdgeCount := 0
	if existingToolsNode != "" {
		observe.GlobalTrace("if: existingToolsNode != \"\"")
		for _, e := range def.Graph.Edges {
			observe.GlobalTrace("range def.Graph.Edges")
			if e.From == existingToolsNode {
				observe.GlobalTrace("if: e.From == existingToolsNode")
				existingToolsBackEdgeCount++
				existingToolsBackEdgeTarget = e.To
			}
		}
	}

	canReuseExisting := len(fixes) == 1 && existingToolsNode != "" &&
		(existingToolsBackEdgeCount == 0 ||
			(existingToolsBackEdgeCount == 1 && existingToolsBackEdgeTarget == fixes[0].nodeName))

	for i := len(fixes) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		fix := fixes[i]

		// Determine which tools node this LLM node routes to.
		var toolsNode string
		if canReuseExisting {
			observe.GlobalTrace("if: canReuseExisting")
			toolsNode = existingToolsNode
		} else {
			observe.GlobalTrace("else: canReuseExisting")
			toolsNode = fmt.Sprintf("%s_tools", fix.nodeName)
			def.Graph.Nodes[toolsNode] = NodeSpec{Type: "tools"}

			def.Graph.Edges = append(def.Graph.Edges, EdgeSpec{
				From: toolsNode,
				To:   fix.nodeName,
			})
		}

		def.Graph.Edges = append(def.Graph.Edges[:fix.edgeIdx], def.Graph.Edges[fix.edgeIdx+1:]...)

		def.Graph.ConditionalEdges = append(def.Graph.ConditionalEdges, ConditionalEdgeSpec{
			From:   fix.nodeName,
			Router: "stop_reason",
			Paths: map[string]string{
				"continue": toolsNode,
				"end":      fix.target,
			},
		})

		if canReuseExisting {
			observe.GlobalTrace("if: canReuseExisting")
			hasBackEdge := false
			for _, e := range def.Graph.Edges {
				observe.GlobalTrace("range def.Graph.Edges")
				if e.From == toolsNode && e.To == fix.nodeName {
					observe.GlobalTrace("if: e.From == toolsNode && e.To == fix.nodeName")
					hasBackEdge = true
					break
				}
			}
			if !hasBackEdge {
				observe.GlobalTrace("if: !hasBackEdge")
				def.Graph.Edges = append(def.Graph.Edges, EdgeSpec{
					From: toolsNode,
					To:   fix.nodeName,
				})
			}
		}
	}
}
