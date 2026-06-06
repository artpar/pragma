package observe

import (
	"context"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
)

// --- Context-carried trace ---

type traceKey struct{}

// TraceFunc is a function that emits a trace event.
type TraceFunc func(component, function, msg string)

// WithTrace attaches a TraceFunc to a context.
func WithTrace(ctx context.Context, fn TraceFunc) context.Context {
	return context.WithValue(ctx, traceKey{}, fn)
}

// TraceCtx emits a FlowTrace event via the TraceFunc stored in ctx.
// No-op if ctx has no TraceFunc.
func TraceCtx(ctx context.Context, component, function, msg string) {
	if fn, ok := ctx.Value(traceKey{}).(TraceFunc); ok && fn != nil {
		if traceFilterPass(component, function, msg) {
			fn(component, function, msg)
		}
	}
}

// --- Global trace (for functions without ctx) ---

var globalBus atomic.Pointer[EventBus]

// SetGlobalBus sets the package-level EventBus used by GlobalTrace.
// Called once at startup.
func SetGlobalBus(bus *EventBus) {
	globalBus.Store(bus)
}

// InstallGlobalBus sets the package-level EventBus and returns a restore
// function that only rolls back if the installed bus is still current.
func InstallGlobalBus(bus *EventBus) func() {
	previous := globalBus.Swap(bus)
	return func() {
		globalBus.CompareAndSwap(bus, previous)
	}
}

// GlobalTrace emits a FlowTrace using runtime.Caller to derive component/function.
// Works in any function regardless of signature — no ctx needed.
func GlobalTrace(msg string) {
	bus := globalBus.Load()
	if bus == nil {
		return
	}
	pc, file, _, ok := runtime.Caller(1)
	if !ok {
		return
	}
	component := filepath.Base(filepath.Dir(file))
	function := "unknown"
	if fn := runtime.FuncForPC(pc); fn != nil {
		// fn.Name() returns "github.com/artpar/pragma/internal/model.(*Conversation).Append"
		// We want just "Append" or "(*Conversation).Append"
		name := fn.Name()
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			function = name[idx+1:]
		}
		// Handle method receivers: if there's a second dot, include the receiver
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			short := name[idx+1:]
			if dotIdx := strings.Index(short, "."); dotIdx >= 0 {
				function = short[dotIdx+1:]
			}
		}
	}

	if traceFilterPass(component, function, msg) {
		bus.Trace(component, function, msg)
	}
}

// --- Runtime trace filter ---

// TraceFilter controls which trace events are emitted at runtime.
// All nil maps/patterns mean "pass everything".
type TraceFilter struct {
	IncludeComponents map[string]bool // nil = all
	ExcludeComponents map[string]bool // checked first
	IncludeFunctions  map[string]bool
	ExcludeFunctions  map[string]bool
	MessagePattern    *regexp.Regexp // nil = all messages
}

var activeFilter atomic.Pointer[TraceFilter]

// SetTraceFilter sets the runtime trace filter. nil clears the filter.
func SetTraceFilter(f *TraceFilter) {
	activeFilter.Store(f)
}

// InstallTraceFilter sets the runtime trace filter and returns a restore
// function that only rolls back if the installed filter is still current.
func InstallTraceFilter(f *TraceFilter) func() {
	previous := activeFilter.Swap(f)
	return func() {
		activeFilter.CompareAndSwap(f, previous)
	}
}

// GetTraceFilter returns the current runtime trace filter (may be nil).
func GetTraceFilter() *TraceFilter {
	return activeFilter.Load()
}

// traceFilterPass checks whether a trace event should be emitted.
func traceFilterPass(component, function, msg string) bool {
	f := activeFilter.Load()
	if f == nil {
		return true
	}

	// Exclude checks first — exclude always wins
	if f.ExcludeComponents != nil && f.ExcludeComponents[component] {
		return false
	}
	if f.ExcludeFunctions != nil && f.ExcludeFunctions[function] {
		return false
	}

	// Include checks — whitelist mode when set
	if f.IncludeComponents != nil && !f.IncludeComponents[component] {
		return false
	}
	if f.IncludeFunctions != nil && !f.IncludeFunctions[function] {
		return false
	}

	// Message pattern
	if f.MessagePattern != nil && !f.MessagePattern.MatchString(msg) {
		return false
	}

	return true
}

// ParseTraceFilter parses a filter string in the format:
// "component=query,permission;exclude_func=consumeStream;pattern=stop_reason"
func ParseTraceFilter(s string) *TraceFilter {
	if s == "" {
		return nil
	}
	f := &TraceFilter{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		vals := strings.Split(value, ",")
		switch key {
		case "component":
			f.IncludeComponents = toSet(vals)
		case "exclude_component":
			f.ExcludeComponents = toSet(vals)
		case "function", "func":
			f.IncludeFunctions = toSet(vals)
		case "exclude_func", "exclude_function":
			f.ExcludeFunctions = toSet(vals)
		case "pattern", "grep":
			if re, err := regexp.Compile(value); err == nil {
				f.MessagePattern = re
			}
		}
	}
	return f
}

func toSet(vals []string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			m[v] = true
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
