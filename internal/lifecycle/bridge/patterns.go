package bridge

import "github.com/artpar/pragma/internal/observe"

// StructureExamples returns example natural-language structure descriptions
// that can be passed to GenerateGraph. These are documentation, not code paths.
func StructureExamples() map[string]string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: map[string]string{...}")
	observe.GlobalTrace("return: map[string]string{\n\t\"build test fix\":\t\"Implement components, build/test after...")
	return map[string]string{
		"build test fix":    "Implement components, build/test after each, reflect on failures and fix until passing",
		"attempt and retry": "Attempt the task with tools, evaluate if successful, if not reflect on what went wrong and retry",
		"plan then execute": "Plan steps first, execute each with tools, verify result before next step",
		"parallel analysis": "Analyze from multiple perspectives in parallel, then merge findings",
	}
}
