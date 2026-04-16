package selftrace

import (
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

// formatEvent returns a compact one-line representation of an event.
func formatEvent(ev observe.Event) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ts := ev.EventTimestamp().Format("15:04:05")
	kind := ev.EventKind()

	switch e := ev.(type) {

	case observe.APIRequestStarted:
		observe.GlobalTrace("typecase: observe.APIRequestStarted")
		return fmt.Sprintf("[%s] APIRequestStarted model=%s msgs=%d tools=%d tokens~%d",
			ts, e.Model, e.MessageCount, e.ToolCount, e.TokenEstimate)
	case observe.APIRequestCompleted:
		observe.GlobalTrace("typecase: observe.APIRequestCompleted")
		return fmt.Sprintf("[%s] APIRequestCompleted stop=%s in=%d out=%d cached=%d dur=%dms model=%s",
			ts, e.StopReason, e.Usage.InputTokens, e.Usage.OutputTokens,
			e.Usage.CacheReadInputTokens, e.DurationMs, e.Model)
	case observe.APIRequestFailed:
		observe.GlobalTrace("typecase: observe.APIRequestFailed")
		return fmt.Sprintf("[%s] APIRequestFailed type=%s msg=%s retryable=%v attempt=%d",
			ts, e.ErrorType, truncate(e.ErrorMessage, 80), e.Retryable, e.Attempt)
	case observe.APIRetryScheduled:
		observe.GlobalTrace("typecase: observe.APIRetryScheduled")
		return fmt.Sprintf("[%s] APIRetryScheduled attempt=%d delay=%dms reason=%s",
			ts, e.Attempt, e.DelayMs, truncate(e.Reason, 60))

	case observe.ToolCallReceived:
		observe.GlobalTrace("typecase: observe.ToolCallReceived")
		return fmt.Sprintf("[%s] ToolCallReceived %s input=%dB",
			ts, e.ToolName, e.InputSizeBytes)
	case observe.ToolExecutionStarted:
		observe.GlobalTrace("typecase: observe.ToolExecutionStarted")
		return fmt.Sprintf("[%s] ToolExecutionStarted %s concurrent=%v",
			ts, e.ToolName, e.Concurrent)
	case observe.ToolExecutionCompleted:
		observe.GlobalTrace("typecase: observe.ToolExecutionCompleted")
		errStr := ""
		if e.IsError {
			errStr = " ERROR"
		}
		return fmt.Sprintf("[%s] ToolExecutionCompleted %s %dms %dB%s",
			ts, e.ToolName, e.DurationMs, e.OutputSizeBytes, errStr)
	case observe.ToolExecutionFailed:
		observe.GlobalTrace("typecase: observe.ToolExecutionFailed")
		return fmt.Sprintf("[%s] ToolExecutionFailed %s type=%s msg=%s",
			ts, e.ToolName, e.ErrorType, truncate(e.ErrorMessage, 80))
	case observe.ToolBatchStarted:
		observe.GlobalTrace("typecase: observe.ToolBatchStarted")
		return fmt.Sprintf("[%s] ToolBatchStarted concurrent=%d serial=%d total=%d",
			ts, e.ConcurrentCount, e.SerialCount, e.TotalCount)
	case observe.ToolBatchCompleted:
		observe.GlobalTrace("typecase: observe.ToolBatchCompleted")
		return fmt.Sprintf("[%s] ToolBatchCompleted total=%dms concurrent=%dms serial=%dms",
			ts, e.TotalDurationMs, e.ConcurrentDurationMs, e.SerialDurationMs)

	case observe.ToolPermissionChecked:
		observe.GlobalTrace("typecase: observe.ToolPermissionChecked")
		return fmt.Sprintf("[%s] ToolPermissionChecked %s decision=%s source=%s",
			ts, e.ToolName, e.Decision, e.Source)
	case observe.ToolPermissionPrompted:
		observe.GlobalTrace("typecase: observe.ToolPermissionPrompted")
		return fmt.Sprintf("[%s] ToolPermissionPrompted %s user=%s dur=%dms",
			ts, e.ToolName, e.UserDecision, e.DurationMs)
	case observe.PermissionRuleMatched:
		observe.GlobalTrace("typecase: observe.PermissionRuleMatched")
		return fmt.Sprintf("[%s] PermissionRuleMatched %s pattern=%s decision=%s",
			ts, e.ToolName, e.Pattern, e.Decision)
	case observe.PermissionDenialEnforced:
		observe.GlobalTrace("typecase: observe.PermissionDenialEnforced")
		return fmt.Sprintf("[%s] PermissionDenialEnforced %s was_executed=%v",
			ts, e.ToolName, e.WasExecuted)

	case observe.ConversationStarted:
		observe.GlobalTrace("typecase: observe.ConversationStarted")
		return fmt.Sprintf("[%s] ConversationStarted id=%s model=%s provider=%s",
			ts, truncate(e.ConversationID, 12), e.Model, e.Provider)
	case observe.MessageAppended:
		observe.GlobalTrace("typecase: observe.MessageAppended")
		return fmt.Sprintf("[%s] MessageAppended role=%s types=%v tokens~%d",
			ts, e.Role, e.ContentTypes, e.TokenEstimate)
	case observe.ConversationForked:
		observe.GlobalTrace("typecase: observe.ConversationForked")
		return fmt.Sprintf("[%s] ConversationForked agent=%s parent=%s child=%s",
			ts, e.AgentName, truncate(e.ParentConvID, 12), truncate(e.ChildConvID, 12))
	case observe.SessionStarted:
		observe.GlobalTrace("typecase: observe.SessionStarted")
		resumed := ""
		if e.ResumedFrom != "" {
			resumed = " resumed=" + truncate(e.ResumedFrom, 12)
		}
		return fmt.Sprintf("[%s] SessionStarted id=%s%s",
			ts, truncate(e.SessionID, 12), resumed)
	case observe.SessionSaved:
		observe.GlobalTrace("typecase: observe.SessionSaved")
		return fmt.Sprintf("[%s] SessionSaved id=%s msgs=%d size=%dB",
			ts, truncate(e.SessionID, 12), e.MessageCount, e.FileSizeBytes)
	case observe.SessionEnded:
		observe.GlobalTrace("typecase: observe.SessionEnded")
		return fmt.Sprintf("[%s] SessionEnded id=%s dur=%dms turns=%d cost=$%.4f",
			ts, truncate(e.SessionID, 12), e.DurationMs, e.TurnCount, e.TotalCostUSD)

	case observe.SubAgentSpawned:
		observe.GlobalTrace("typecase: observe.SubAgentSpawned")
		return fmt.Sprintf("[%s] SubAgentSpawned id=%s name=%s model=%s",
			ts, truncate(e.SubAgentID, 12), e.AgentName, e.Model)
	case observe.SubAgentCompleted:
		observe.GlobalTrace("typecase: observe.SubAgentCompleted")
		return fmt.Sprintf("[%s] SubAgentCompleted id=%s dur=%dms turns=%d in=%d out=%d",
			ts, truncate(e.SubAgentID, 12), e.DurationMs, e.TurnCount,
			e.Usage.InputTokens, e.Usage.OutputTokens)
	case observe.SubAgentFailed:
		observe.GlobalTrace("typecase: observe.SubAgentFailed")
		return fmt.Sprintf("[%s] SubAgentFailed id=%s type=%s msg=%s",
			ts, truncate(e.SubAgentID, 12), e.ErrorType, truncate(e.ErrorMessage, 80))

	case observe.ErrorOccurred:
		observe.GlobalTrace("typecase: observe.ErrorOccurred")
		return fmt.Sprintf("[%s] ErrorOccurred severity=%s component=%s type=%s msg=%s",
			ts, e.Severity, e.Component, e.ErrorType, truncate(e.ErrorMessage, 80))

	case observe.CompactionStarted:
		observe.GlobalTrace("typecase: observe.CompactionStarted")
		return fmt.Sprintf("[%s] CompactionStarted pre_tokens=%d budget=%d msgs=%d",
			ts, e.PreTokenCount, e.BudgetTokens, e.MessageCount)
	case observe.CompactionCompleted:
		observe.GlobalTrace("typecase: observe.CompactionCompleted")
		return fmt.Sprintf("[%s] CompactionCompleted post_tokens=%d summarized=%d dur=%dms",
			ts, e.PostTokenCount, e.SummarizedCount, e.DurationMs)
	case observe.CompactionFailed:
		observe.GlobalTrace("typecase: observe.CompactionFailed")
		return fmt.Sprintf("[%s] CompactionFailed type=%s msg=%s",
			ts, e.ErrorType, truncate(e.ErrorMessage, 80))

	case observe.MCPServerConnected:
		observe.GlobalTrace("typecase: observe.MCPServerConnected")
		return fmt.Sprintf("[%s] MCPServerConnected %s tools=%d dur=%dms",
			ts, e.ServerName, e.ToolCount, e.DurationMs)
	case observe.MCPServerFailed:
		observe.GlobalTrace("typecase: observe.MCPServerFailed")
		return fmt.Sprintf("[%s] MCPServerFailed %s type=%s msg=%s",
			ts, e.ServerName, e.ErrorType, truncate(e.ErrorMessage, 60))
	case observe.MCPToolCallCompleted:
		observe.GlobalTrace("typecase: observe.MCPToolCallCompleted")
		return fmt.Sprintf("[%s] MCPToolCallCompleted %s/%s dur=%dms output=%dB",
			ts, e.ServerName, e.ToolName, e.DurationMs, e.OutputSizeBytes)

	case observe.LifecycleStepStarted:
		observe.GlobalTrace("typecase: observe.LifecycleStepStarted")
		return fmt.Sprintf("[%s] LifecycleStepStarted step=%d nodes=%v",
			ts, e.Step, e.Nodes)
	case observe.LifecycleNodeCompleted:
		observe.GlobalTrace("typecase: observe.LifecycleNodeCompleted")
		errStr := ""
		if e.Error != "" {
			errStr = " error=" + truncate(e.Error, 60)
		}
		return fmt.Sprintf("[%s] LifecycleNodeCompleted step=%d node=%s dur=%s%s",
			ts, e.Step, e.Node, e.Duration, errStr)
	case observe.LifecycleTransition:
		observe.GlobalTrace("typecase: observe.LifecycleTransition")
		return fmt.Sprintf("[%s] LifecycleTransition step=%d %s→%s key=%s",
			ts, e.Step, e.From, e.To, e.RouteKey)
	case observe.LifecycleCompleted:
		observe.GlobalTrace("typecase: observe.LifecycleCompleted")
		errStr := ""
		if e.Error != "" {
			errStr = " error=" + truncate(e.Error, 60)
		}
		return fmt.Sprintf("[%s] LifecycleCompleted steps=%d%s",
			ts, e.TotalSteps, errStr)

	case observe.FlowTrace:
		observe.GlobalTrace("typecase: observe.FlowTrace")
		return fmt.Sprintf("[%s] FlowTrace %s.%s: %s",
			ts, e.Component, e.Function, truncate(e.Message, 80))

	case observe.SlashCommandExecuted:
		observe.GlobalTrace("typecase: observe.SlashCommandExecuted")
		return fmt.Sprintf("[%s] SlashCommandExecuted /%s success=%v dur=%dms",
			ts, e.CommandName, e.Success, e.DurationMs)

	case observe.HookExecuted:
		observe.GlobalTrace("typecase: observe.HookExecuted")
		return fmt.Sprintf("[%s] HookExecuted event=%s outcome=%s exit=%d",
			ts, e.HookEvent, e.Outcome, e.ExitCode)
	case observe.HookBlocked:
		observe.GlobalTrace("typecase: observe.HookBlocked")
		return fmt.Sprintf("[%s] HookBlocked event=%s msg=%s",
			ts, e.HookEvent, truncate(e.Message, 80))
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"[%s] %s\", ts, kind)")

	return fmt.Sprintf("[%s] %s", ts, kind)
}

// summaryAgg accumulates statistics across all events.
type summaryAgg struct {
	firstTime     time.Time
	lastTime      time.Time
	model         string
	provider      string
	apiOK         int
	apiFailed     int
	totalInputTk  int
	totalOutTk    int
	totalCachedTk int
	totalAPIDurMs int64
	toolCalls     map[string]*toolStat
	toolErrors    int
	subAgents     int
	errorKinds    map[string]int
	compactions   int
	lifecycles    int
	messages      int
	totalEvents   int
}

type toolStat struct {
	calls   int
	errors  int
	totalMs int64
}

func (s *summaryAgg) add(ev observe.Event) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if ev.EventKind() == "FlowTrace" {
		observe.GlobalTrace("if: ev.EventKind() == \"FlowTrace\"")
		return
	}
	s.totalEvents++
	ts := ev.EventTimestamp()
	if s.firstTime.IsZero() || ts.Before(s.firstTime) {
		observe.GlobalTrace("if: s.firstTime.IsZero() || ts.Before(s.firstTime)")
		s.firstTime = ts
	}
	if ts.After(s.lastTime) {
		observe.GlobalTrace("if: ts.After(s.lastTime)")
		s.lastTime = ts
	}

	switch e := ev.(type) {
	case observe.ConversationStarted:
		observe.GlobalTrace("typecase: observe.ConversationStarted")
		if s.model == "" {
			s.model = e.Model
			s.provider = e.Provider
		}
	case observe.APIRequestCompleted:
		observe.GlobalTrace("typecase: observe.APIRequestCompleted")
		s.apiOK++
		s.totalInputTk += e.Usage.InputTokens
		s.totalOutTk += e.Usage.OutputTokens
		s.totalCachedTk += e.Usage.CacheReadInputTokens
		s.totalAPIDurMs += e.DurationMs
		if s.model == "" {
			s.model = e.Model
		}
	case observe.APIRequestFailed:
		observe.GlobalTrace("typecase: observe.APIRequestFailed")
		s.apiFailed++
		s.addError("APIRequestFailed")
	case observe.ToolExecutionCompleted:
		observe.GlobalTrace("typecase: observe.ToolExecutionCompleted")
		s.addTool(e.ToolName, e.DurationMs, false)
	case observe.ToolExecutionFailed:
		observe.GlobalTrace("typecase: observe.ToolExecutionFailed")
		s.addTool(e.ToolName, 0, true)
		s.addError("ToolExecutionFailed")
	case observe.SubAgentSpawned:
		observe.GlobalTrace("typecase: observe.SubAgentSpawned")
		s.subAgents++
	case observe.ErrorOccurred:
		observe.GlobalTrace("typecase: observe.ErrorOccurred")
		s.addError("ErrorOccurred:" + e.ErrorType)
	case observe.CompactionCompleted:
		observe.GlobalTrace("typecase: observe.CompactionCompleted")
		s.compactions++
	case observe.CompactionFailed:
		observe.GlobalTrace("typecase: observe.CompactionFailed")
		s.addError("CompactionFailed")
	case observe.LifecycleCompleted:
		observe.GlobalTrace("typecase: observe.LifecycleCompleted")
		s.lifecycles++
	case observe.MessageAppended:
		observe.GlobalTrace("typecase: observe.MessageAppended")
		if e.Role == "user" {
			s.messages++
		}
	case observe.SubAgentFailed:
		observe.GlobalTrace("typecase: observe.SubAgentFailed")
		s.addError("SubAgentFailed")
	case observe.MCPServerFailed:
		observe.GlobalTrace("typecase: observe.MCPServerFailed")
		s.addError("MCPServerFailed")
	}
}

func (s *summaryAgg) addTool(name string, durMs int64, isErr bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if s.toolCalls == nil {
		observe.GlobalTrace("if: s.toolCalls == nil")
		s.toolCalls = make(map[string]*toolStat)
	}
	st, ok := s.toolCalls[name]
	if !ok {
		observe.GlobalTrace("if: !ok")
		st = &toolStat{}
		s.toolCalls[name] = st
	}
	st.calls++
	st.totalMs += durMs
	if isErr {
		observe.GlobalTrace("if: isErr")
		st.errors++
		s.toolErrors++
	}
}

func (s *summaryAgg) addError(kind string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if s.errorKinds == nil {
		observe.GlobalTrace("if: s.errorKinds == nil")
		s.errorKinds = make(map[string]int)
	}
	s.errorKinds[kind]++
}

func (s *summaryAgg) String() string {
	var b strings.Builder

	dur := s.lastTime.Sub(s.firstTime)
	fmt.Fprintf(&b, "Session: %s (%s)\n", s.firstTime.Format("2006-01-02T15:04:05"), formatDuration(dur))
	fmt.Fprintf(&b, "Provider: %s (%s)\n", s.provider, s.model)
	fmt.Fprintf(&b, "User messages: %d\n", s.messages)

	totalAPI := s.apiOK + s.apiFailed
	if totalAPI > 0 {
		avgMs := s.totalAPIDurMs / int64(s.apiOK)
		fmt.Fprintf(&b, "API calls: %d (%d ok, %d failed), avg %s\n",
			totalAPI, s.apiOK, s.apiFailed, formatDuration(time.Duration(avgMs)*time.Millisecond))
	} else {
		b.WriteString("API calls: 0\n")
	}

	fmt.Fprintf(&b, "Tokens: %s input, %s output, %s cached\n",
		formatTokens(s.totalInputTk), formatTokens(s.totalOutTk), formatTokens(s.totalCachedTk))

	totalToolCalls := 0
	for _, st := range s.toolCalls {
		totalToolCalls += st.calls
	}
	if totalToolCalls > 0 {
		// Build per-tool breakdown sorted by call count
		type kv struct {
			name string
			st   *toolStat
		}
		var sorted []kv
		for name, st := range s.toolCalls {
			sorted = append(sorted, kv{name, st})
		}

		for i := 1; i < len(sorted); i++ {
			for j := i; j > 0 && sorted[j].st.calls > sorted[j-1].st.calls; j-- {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			}
		}

		var toolParts []string
		for _, kv := range sorted {
			toolParts = append(toolParts, fmt.Sprintf("%s: %d", kv.name, kv.st.calls))
		}
		fmt.Fprintf(&b, "Tools: %d calls (%s), %d errors\n",
			totalToolCalls, strings.Join(toolParts, ", "), s.toolErrors)

		// Per-tool avg timing
		var timingParts []string
		for _, kv := range sorted {
			if kv.st.calls > 0 && kv.st.totalMs > 0 {
				avg := kv.st.totalMs / int64(kv.st.calls)
				timingParts = append(timingParts, fmt.Sprintf("%s: avg %dms", kv.name, avg))
			}
		}
		if len(timingParts) > 0 {
			fmt.Fprintf(&b, "  %s\n", strings.Join(timingParts, ", "))
		}
	} else {
		b.WriteString("Tools: 0 calls\n")
	}

	fmt.Fprintf(&b, "Sub-agents: %d spawned\n", s.subAgents)
	fmt.Fprintf(&b, "Compactions: %d\n", s.compactions)
	fmt.Fprintf(&b, "Lifecycle runs: %d\n", s.lifecycles)

	totalErrors := 0
	for _, c := range s.errorKinds {
		totalErrors += c
	}
	if totalErrors > 0 {
		var errParts []string
		for kind, count := range s.errorKinds {
			errParts = append(errParts, fmt.Sprintf("%s: %d", kind, count))
		}
		fmt.Fprintf(&b, "Errors: %d (%s)\n", totalErrors, strings.Join(errParts, ", "))
	} else {
		b.WriteString("Errors: 0\n")
	}

	fmt.Fprintf(&b, "Total events: %d (excluding FlowTrace)\n", s.totalEvents)

	return b.String()
}

func truncate(s string, max int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(s) <= max {
		observe.GlobalTrace("if: len(s) <= max")
		observe.GlobalTrace("return: s")
		return s
	}
	observe.GlobalTrace("return: s[:max] + \"...\"")
	return s[:max] + "..."
}

func formatDuration(d time.Duration) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d < time.Second {
		observe.GlobalTrace("if: d < time.Second")
		observe.GlobalTrace("return: fmt.Sprintf(\"%dms\", d.Milliseconds())")
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		observe.GlobalTrace("if: d < time.Minute")
		observe.GlobalTrace("return: fmt.Sprintf(\"%.1fs\", d.Seconds())")
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	observe.GlobalTrace("return: fmt.Sprintf(\"%dm%ds\", m, s)")
	return fmt.Sprintf("%dm%ds", m, s)
}

func formatTokens(n int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n < 1000 {
		observe.GlobalTrace("if: n < 1000")
		observe.GlobalTrace("return: fmt.Sprintf(\"%d\", n)")
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		observe.GlobalTrace("if: n < 1_000_000")
		observe.GlobalTrace("return: fmt.Sprintf(\"%.1fK\", float64(n)/1000)")
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"%.1fM\", float64(n)/1_000_000)")
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}
