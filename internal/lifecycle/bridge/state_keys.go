package bridge

import (
	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/model"
)

// State key constants for bridge nodes. All bridge nodes use these
// to communicate through lifecycle.State — no raw string keys.
const (
	KeyMessages    = "messages"    // []model.Message — use MessageReducer
	KeySystem      = "system"      // model.SystemPrompt
	KeyModelID     = "model_id"    // string
	KeyMaxTokens   = "max_tokens"  // int
	KeyTools       = "tools"       // []model.ToolDef
	KeyStopReason  = "stop_reason" // string (model.StopReason cast to string)
	KeyResponse    = "response"    // model.Response
	KeyPassed      = "passed"      // bool
	KeyScore       = "score"       // float64
	KeyReflections = "reflections" // []string
	KeyTurnCount   = "turn_count"  // int
)

// Messages extracts []model.Message from state. Returns nil if missing or wrong type.
func Messages(s lifecycle.State) []model.Message {
	v, ok := s[KeyMessages].([]model.Message)
	if !ok {
		return nil
	}
	return v
}

// System extracts model.SystemPrompt from state. Returns zero value if missing.
func System(s lifecycle.State) model.SystemPrompt {
	v, ok := s[KeySystem].(model.SystemPrompt)
	if !ok {
		return model.SystemPrompt{}
	}
	return v
}

// ModelID extracts the model identifier string from state.
func ModelID(s lifecycle.State) string {
	v, _ := s[KeyModelID].(string)
	return v
}

// MaxTokens extracts the max tokens int from state. Returns 0 if missing.
func MaxTokens(s lifecycle.State) int {
	v, _ := s[KeyMaxTokens].(int)
	return v
}

// Tools extracts []model.ToolDef from state. Returns nil if missing.
func Tools(s lifecycle.State) []model.ToolDef {
	v, ok := s[KeyTools].([]model.ToolDef)
	if !ok {
		return nil
	}
	return v
}

// StopReason extracts the stop reason string from state.
func StopReason(s lifecycle.State) string {
	v, _ := s[KeyStopReason].(string)
	return v
}

// Response extracts model.Response from state. Returns zero value if missing.
func Response(s lifecycle.State) model.Response {
	v, ok := s[KeyResponse].(model.Response)
	if !ok {
		return model.Response{}
	}
	return v
}

// Passed extracts the evaluation pass/fail bool from state.
func Passed(s lifecycle.State) bool {
	v, _ := s[KeyPassed].(bool)
	return v
}

// Score extracts the evaluation score from state.
func Score(s lifecycle.State) float64 {
	v, _ := s[KeyScore].(float64)
	return v
}

// Reflections extracts the accumulated reflections from state.
func Reflections(s lifecycle.State) []string {
	v, ok := s[KeyReflections].([]string)
	if !ok {
		return nil
	}
	return v
}
