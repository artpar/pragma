package query

// ShouldPersistSessionEvent reports whether a loop event represents a durable
// session mutation or checkpoint.
func ShouldPersistSessionEvent(ev LoopEvent) bool {
	switch ev.(type) {
	case ModelRequestEvent,
		ModelResponseEvent,
		ToolCallEvent,
		ToolResultEvent,
		TurnCompleteEvent,
		CompactionEvent:
		return true
	default:
		return false
	}
}
