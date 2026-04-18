package definition

import "fmt"

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
	if def == nil || len(def.Graph.Nodes) == 0 {
		return
	}

	// Build set of LLM nodes that already have stop_reason routing.
	hasStopRouting := make(map[string]bool)
	for _, ce := range def.Graph.ConditionalEdges {
		if ce.Router == "stop_reason" {
			hasStopRouting[ce.From] = true
		}
	}

	// Collect LLM nodes that need fixing: they have unconditional edges but no
	// stop_reason conditional edge.
	type fixTarget struct {
		nodeName string
		edgeIdx  int // index in def.Graph.Edges
		target   string
	}
	var fixes []fixTarget

	for i, e := range def.Graph.Edges {
		spec, ok := def.Graph.Nodes[e.From]
		if !ok || spec.Type != "llm" {
			continue
		}
		if hasStopRouting[e.From] {
			continue
		}
		fixes = append(fixes, fixTarget{
			nodeName: e.From,
			edgeIdx:  i,
			target:   e.To,
		})
	}

	if len(fixes) == 0 {
		return
	}

	// Find existing tools node and its current back-edges.
	existingToolsNode := ""
	for name, spec := range def.Graph.Nodes {
		if spec.Type == "tools" {
			existingToolsNode = name
			break
		}
	}

	// Check if the existing tools node's back-edges are compatible with reuse.
	// Reuse is safe only when all existing back-edges go to the node being fixed,
	// so we don't create multi-target back-edges that cause parallel execution.
	existingToolsBackEdgeTarget := ""
	existingToolsBackEdgeCount := 0
	if existingToolsNode != "" {
		for _, e := range def.Graph.Edges {
			if e.From == existingToolsNode {
				existingToolsBackEdgeCount++
				existingToolsBackEdgeTarget = e.To
			}
		}
	}

	canReuseExisting := len(fixes) == 1 && existingToolsNode != "" &&
		(existingToolsBackEdgeCount == 0 ||
			(existingToolsBackEdgeCount == 1 && existingToolsBackEdgeTarget == fixes[0].nodeName))

	// Process fixes in reverse order so edge indices stay valid during removal.
	for i := len(fixes) - 1; i >= 0; i-- {
		fix := fixes[i]

		// Determine which tools node this LLM node routes to.
		var toolsNode string
		if canReuseExisting {
			toolsNode = existingToolsNode
		} else {
			toolsNode = fmt.Sprintf("%s_tools", fix.nodeName)
			def.Graph.Nodes[toolsNode] = NodeSpec{Type: "tools"}
			// Dedicated tools node always routes back to its LLM node.
			def.Graph.Edges = append(def.Graph.Edges, EdgeSpec{
				From: toolsNode,
				To:   fix.nodeName,
			})
		}

		// Remove the unconditional edge.
		def.Graph.Edges = append(def.Graph.Edges[:fix.edgeIdx], def.Graph.Edges[fix.edgeIdx+1:]...)

		// Add stop_reason conditional edge.
		def.Graph.ConditionalEdges = append(def.Graph.ConditionalEdges, ConditionalEdgeSpec{
			From:   fix.nodeName,
			Router: "stop_reason",
			Paths: map[string]string{
				"continue": toolsNode,
				"end":      fix.target,
			},
		})

		// For reused existing tools node, add back-edge if not present.
		if canReuseExisting {
			hasBackEdge := false
			for _, e := range def.Graph.Edges {
				if e.From == toolsNode && e.To == fix.nodeName {
					hasBackEdge = true
					break
				}
			}
			if !hasBackEdge {
				def.Graph.Edges = append(def.Graph.Edges, EdgeSpec{
					From: toolsNode,
					To:   fix.nodeName,
				})
			}
		}
	}
}
