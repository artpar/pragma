package definition

import (
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/lifecycle"
)

// RouterCreator resolves router spec strings into RouterFunc values.
type RouterCreator func(spec string) (lifecycle.RouterFunc, error)

// DefaultRouterCreator returns a RouterCreator that handles built-in router types:
//   - "stop_reason": routes on state["stop_reason"], "tool_use" → "continue", else → "end"
//   - "field:<key>": routes on the string/bool value of state[key]
//   - "pass_fail": routes on state["passed"], true → "pass", false → "fail"
func DefaultRouterCreator() RouterCreator {
	return func(spec string) (lifecycle.RouterFunc, error) {
		switch {
		case spec == "stop_reason":
			return stopReasonRouter(), nil
		case strings.HasPrefix(spec, "field:"):
			key := strings.TrimPrefix(spec, "field:")
			if key == "" {
				return nil, fmt.Errorf("router spec %q: empty field key", spec)
			}
			return fieldRouter(key), nil
		case spec == "pass_fail":
			return passFailRouter(), nil
		default:
			return nil, fmt.Errorf("unknown router spec: %q", spec)
		}
	}
}

func stopReasonRouter() lifecycle.RouterFunc {
	return func(s lifecycle.State) string {
		reason, _ := s["stop_reason"].(string)
		if reason == "tool_use" {
			return "continue"
		}
		return "end"
	}
}

func fieldRouter(key string) lifecycle.RouterFunc {
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

func passFailRouter() lifecycle.RouterFunc {
	return func(s lifecycle.State) string {
		if v, _ := s["passed"].(bool); v {
			return "pass"
		}
		return "fail"
	}
}
