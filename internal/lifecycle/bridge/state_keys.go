package bridge

import (
	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
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
	KeyTotalUsage  = "total_usage" // model.TokenUsage
)

// Messages extracts []model.Message from state. Returns nil if missing or wrong type.
func Messages(s lifecycle.State) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, ok := s[KeyMessages].([]model.Message)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: v")
	return v
}

// System extracts model.SystemPrompt from state. Returns zero value if missing.
func System(s lifecycle.State) model.SystemPrompt {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, ok := s[KeySystem].(model.SystemPrompt)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: model.SystemPrompt{}")
		return model.SystemPrompt{}
	}
	observe.GlobalTrace("return: v")
	return v
}

// ModelID extracts the model identifier string from state.
func ModelID(s lifecycle.State) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, _ := s[KeyModelID].(string)
	observe.GlobalTrace("return: v")
	return v
}

// MaxTokens extracts the max tokens int from state. Returns 0 if missing.
func MaxTokens(s lifecycle.State) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, _ := s[KeyMaxTokens].(int)
	observe.GlobalTrace("return: v")
	return v
}

// Tools extracts []model.ToolDef from state. Returns nil if missing.
func Tools(s lifecycle.State) []model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, ok := s[KeyTools].([]model.ToolDef)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: v")
	return v
}

// StopReason extracts the stop reason string from state.
func StopReason(s lifecycle.State) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, _ := s[KeyStopReason].(string)
	observe.GlobalTrace("return: v")
	return v
}

// Response extracts model.Response from state. Returns zero value if missing.
func Response(s lifecycle.State) model.Response {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, ok := s[KeyResponse].(model.Response)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: model.Response{}")
		return model.Response{}
	}
	observe.GlobalTrace("return: v")
	return v
}

// Passed extracts the evaluation pass/fail bool from state.
func Passed(s lifecycle.State) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, _ := s[KeyPassed].(bool)
	observe.GlobalTrace("return: v")
	return v
}

// Score extracts the evaluation score from state.
func Score(s lifecycle.State) float64 {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, _ := s[KeyScore].(float64)
	observe.GlobalTrace("return: v")
	return v
}

// TotalUsage extracts the accumulated token usage from state.
func TotalUsage(s lifecycle.State) model.TokenUsage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, ok := s[KeyTotalUsage].(model.TokenUsage)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: model.TokenUsage{}")
		return model.TokenUsage{}
	}
	observe.GlobalTrace("return: v")
	return v
}

// Reflections extracts the accumulated reflections from state.
func Reflections(s lifecycle.State) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, ok := s[KeyReflections].([]string)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: v")
	return v
}
