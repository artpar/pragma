package model

// StopReason indicates why the LLM stopped generating.
// Each provider maps its native stop signal to one of these values.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
	StopPauseTurn StopReason = "pause_turn"
	StopError     StopReason = "error"
)
