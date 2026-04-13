package observe

import (
	"context"
	"time"
)

// MCPWatchdog periodically checks MCP server health and emits events.
// Uses a function closure for status to avoid importing internal/mcp/ (DAG compliance).
// Reports only — never kills servers (avoids GitHub bug #40207).
type MCPWatchdog struct {
	statusFn func() map[string]string
	bus      *EventBus
	interval time.Duration
	prev     map[string]string
}

// NewMCPWatchdog creates a watchdog that polls server status at the given interval.
// statusFn should return map[serverName]status (e.g., "connected", "disconnected").
func NewMCPWatchdog(statusFn func() map[string]string, bus *EventBus, interval time.Duration) *MCPWatchdog {
	return &MCPWatchdog{
		statusFn: statusFn,
		bus:      bus,
		interval: interval,
		prev:     make(map[string]string),
	}
}

// Start runs the watchdog loop until ctx is cancelled.
// Intended to be called as a goroutine: go watchdog.Start(ctx)
func (w *MCPWatchdog) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *MCPWatchdog) check() {
	current := w.statusFn()

	for name, status := range current {
		// Emit health check event for each server
		w.bus.Emit(MCPHealthCheck{
			EventHeader: NewEventHeader("MCPHealthCheck", "", "", ""),
			ServerName:  name,
			Status:      status,
		})

		// Detect transitions: connected → disconnected
		prevStatus, existed := w.prev[name]
		if existed && prevStatus == "connected" && status == "disconnected" {
			w.bus.Emit(MCPServerDisconnected{
				EventHeader: NewEventHeader("MCPServerDisconnected", "", "", ""),
				ServerName:  name,
				Reason:      "watchdog detected disconnection",
			})
		}
	}

	// Detect servers that disappeared entirely (process died)
	for name, prevStatus := range w.prev {
		if _, exists := current[name]; !exists && prevStatus == "connected" {
			w.bus.Emit(MCPServerDisconnected{
				EventHeader: NewEventHeader("MCPServerDisconnected", "", "", ""),
				ServerName:  name,
				Reason:      "server no longer in status map",
			})
		}
	}

	// Store current for next tick
	w.prev = current
}
