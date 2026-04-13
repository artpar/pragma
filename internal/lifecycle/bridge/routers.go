package bridge

import (
	"fmt"

	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/model"
)

// StopReasonRouter returns a RouterFunc that routes based on state["stop_reason"].
// Maps "tool_use" → "continue", everything else → "end".
func StopReasonRouter() lifecycle.RouterFunc {
	return func(s lifecycle.State) string {
		reason := StopReason(s)
		if reason == string(model.StopToolUse) {
			return "continue"
		}
		return "end"
	}
}

// FieldRouter returns a RouterFunc that returns the string value of a state key.
// For bool values, returns "true" or "false".
func FieldRouter(key string) lifecycle.RouterFunc {
	return func(s lifecycle.State) string {
		v := s[key]
		if v == nil {
			return ""
		}
		switch val := v.(type) {
		case string:
			return val
		case bool:
			if val {
				return "true"
			}
			return "false"
		default:
			return fmt.Sprintf("%v", val)
		}
	}
}

// PassFailRouter returns a RouterFunc that routes based on state["passed"].
// Returns "pass" if true, "fail" if false.
func PassFailRouter() lifecycle.RouterFunc {
	return func(s lifecycle.State) string {
		if Passed(s) {
			return "pass"
		}
		return "fail"
	}
}
