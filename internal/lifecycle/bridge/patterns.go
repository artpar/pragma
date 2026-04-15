package bridge

// StructureExamples returns example natural-language structure descriptions
// that can be passed to GenerateGraph. These are documentation, not code paths.
func StructureExamples() map[string]string {
	return map[string]string{
		"tool-calling loop":  "LLM calls tools in a loop until done (ReAct pattern)",
		"plan then execute":  "Plan steps first, execute each with tools, replan if needed",
		"attempt and retry":  "Attempt the task with tools, evaluate if successful, if not reflect on what went wrong and retry",
		"parallel analysis":  "Analyze from multiple perspectives in parallel, then merge findings",
	}
}
