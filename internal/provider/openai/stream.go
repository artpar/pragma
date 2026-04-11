package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// streamState tracks content blocks being accumulated during streaming.
type streamState struct {
	toolCalls    map[int]*inProgressToolCall // index -> accumulating tool call
	model        string
	doneSent     bool
	finishReason *string // saved from chunk with finish_reason but no usage
	startTime    time.Time
}

// inProgressToolCall tracks a tool call being streamed in incrementally.
type inProgressToolCall struct {
	wireID     string
	internalID string
	name       string
	argsBuf    strings.Builder
}

// startStream begins consuming an OpenAI SSE stream and writing StreamChunks
// to the returned channel. The channel is closed when the stream ends or errors.
func (p *Provider) startStream(
	ctx context.Context,
	resp *http.Response,
	mapper *IDMapper,
	bus *observe.EventBus,
	traceID, spanID string,
) <-chan provider.StreamChunk {
	ch := make(chan provider.StreamChunk, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		state := &streamState{
			toolCalls: make(map[int]*inProgressToolCall),
			startTime: time.Now(),
		}

		// Idle timeout watchdog
		streamCtx := ctx
		var idleCh chan struct{}
		if p.idleTimeout > 0 {
			var watchCancel context.CancelFunc
			streamCtx, watchCancel = context.WithCancel(ctx)
			defer watchCancel()
			idleCh = make(chan struct{}, 1)
			go func() {
				timer := time.NewTimer(p.idleTimeout)
				defer timer.Stop()
				for {
					select {
					case <-idleCh:
						if !timer.Stop() {
							<-timer.C
						}
						timer.Reset(p.idleTimeout)
					case <-timer.C:
						watchCancel()
						return
					case <-streamCtx.Done():
						return
					}
				}
			}()
		}

		p.consumeSSE(streamCtx, resp, mapper, ch, state, bus, traceID, spanID, idleCh)
	}()

	return ch
}

// consumeSSE reads SSE lines from the HTTP response body and dispatches chunks.
func (p *Provider) consumeSSE(
	ctx context.Context,
	resp *http.Response,
	mapper *IDMapper,
	ch chan<- provider.StreamChunk,
	state *streamState,
	bus *observe.EventBus,
	traceID, spanID string,
	idleCh chan<- struct{},
) {
	scanner := bufio.NewScanner(resp.Body)
	// Allow large lines (tool call arguments can be big)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastEventEmit time.Time

	for scanner.Scan() {
		if ctx.Err() != nil {
			ch <- provider.StreamChunk{Error: ctx.Err()}
			return
		}

		// Reset idle timeout
		if idleCh != nil {
			select {
			case idleCh <- struct{}{}:
			default:
			}
		}

		line := scanner.Text()

		// SSE: only process "data:" lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		// Stream termination
		if data == "[DONE]" {
			// If we have a saved finish_reason but never got usage, emit Done anyway
			if !state.doneSent && state.finishReason != nil {
				if bus != nil {
					bus.Emit(observe.ErrorOccurred{
						EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, spanID, ""),
						Severity:     "warning",
						Component:    "openai/stream",
						ErrorType:    "missing_usage",
						ErrorMessage: "stream ended without usage data; cost tracking will report $0",
					})
				}
				ch <- provider.StreamChunk{
					Done: &provider.StreamDone{
						StopReason: stopReasonFromWire(*state.finishReason),
						Model:      state.model,
					},
				}
				state.doneSent = true
			}
			if !state.doneSent {
				ch <- provider.StreamChunk{Error: model.ErrStreamClosed}
			}
			return
		}

		var chunk wireStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			if bus != nil {
				bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, spanID, ""),
					Severity:     "warning",
					Component:    "openai/stream",
					ErrorType:    "malformed_chunk",
					ErrorMessage: fmt.Sprintf("skipping malformed stream chunk: %v", err),
				})
			}
			continue
		}

		if chunk.Model != "" {
			state.model = chunk.Model
		}

		p.dispatchStreamChunk(&chunk, mapper, ch, state, bus, traceID, spanID)

		// Throttled observability event (at most 1/sec)
		now := time.Now()
		if bus != nil && now.Sub(lastEventEmit) >= time.Second {
			bus.Emit(observe.APIStreamChunk{
				EventHeader: observe.NewEventHeader("APIStreamChunk", traceID, spanID, ""),
				ChunkType:   "stream_delta",
			})
			lastEventEmit = now
		}
	}

	if err := scanner.Err(); err != nil {
		classified := classifyError(err)
		if bus != nil {
			bus.Emit(observe.APIRequestFailed{
				EventHeader:  observe.NewEventHeader("APIRequestFailed", traceID, spanID, ""),
				ErrorType:    classified.errorType,
				ErrorMessage: classified.wrapped.Error(),
				Retryable:    classified.retryable,
			})
		}
		ch <- provider.StreamChunk{Error: classified.wrapped}
		return
	}

	// If we have a finish_reason but never got usage, emit Done with what we have
	if !state.doneSent && state.finishReason != nil {
		if bus != nil {
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, spanID, ""),
				Severity:     "warning",
				Component:    "openai/stream",
				ErrorType:    "missing_usage",
				ErrorMessage: "stream ended without usage data; cost tracking will report $0",
			})
		}
		ch <- provider.StreamChunk{
			Done: &provider.StreamDone{
				StopReason: stopReasonFromWire(*state.finishReason),
				Model:      state.model,
			},
		}
		state.doneSent = true
	}

	if !state.doneSent {
		ch <- provider.StreamChunk{Error: model.ErrStreamClosed}
	}
}

// dispatchStreamChunk processes a single parsed SSE chunk and emits StreamChunks.
func (p *Provider) dispatchStreamChunk(
	chunk *wireStreamChunk,
	mapper *IDMapper,
	ch chan<- provider.StreamChunk,
	state *streamState,
	bus *observe.EventBus,
	traceID, spanID string,
) {
	for _, choice := range chunk.Choices {
		delta := choice.Delta

		// Text content
		if delta.Content != nil && *delta.Content != "" {
			ch <- provider.StreamChunk{TextDelta: *delta.Content}
		}

		// Tool calls
		for _, tc := range delta.ToolCalls {
			if tc.ID != "" {
				// New tool call starting
				internalID := model.NewUUID()
				mapper.RegisterPair(internalID, tc.ID)
				name := ""
				if tc.Function != nil {
					name = tc.Function.Name
				}
				state.toolCalls[tc.Index] = &inProgressToolCall{
					wireID:     tc.ID,
					internalID: internalID,
					name:       name,
				}
				ch <- provider.StreamChunk{
					ToolCallStart: &model.ToolCallPart{
						ID:   internalID,
						Name: name,
					},
				}
			}

			// Argument delta
			if tc.Function != nil && tc.Function.Arguments != "" {
				ipc, ok := state.toolCalls[tc.Index]
				if ok {
					ipc.argsBuf.WriteString(tc.Function.Arguments)
					ch <- provider.StreamChunk{
						ToolCallInputDelta: &provider.ToolCallDelta{
							ToolCallID: ipc.internalID,
							JSONDelta:  tc.Function.Arguments,
						},
					}
				}
			}
		}

		// Finish reason
		if choice.FinishReason != nil {
			// Save finish reason — usage may come in a separate chunk
			state.finishReason = choice.FinishReason

			if chunk.Usage != nil {
				usage := usageFromWire(*chunk.Usage)
				ch <- provider.StreamChunk{
					Done: &provider.StreamDone{
						StopReason: stopReasonFromWire(*choice.FinishReason),
						Usage:      usage,
						Model:      state.model,
					},
				}
				state.doneSent = true

				if bus != nil {
					bus.Emit(observe.APIRequestCompleted{
						EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
						StopReason:  stopReasonFromWire(*choice.FinishReason),
						Usage:       usage,
						DurationMs:  time.Since(state.startTime).Milliseconds(),
						Model:       state.model,
					})
				}
			}
		}
	}

	// Usage in a separate chunk (after choices with finish_reason, common with stream_options)
	if chunk.Usage != nil && !state.doneSent {
		usage := usageFromWire(*chunk.Usage)
		// Use saved finish reason from a prior chunk
		stopReason := model.StopEndTurn
		if state.finishReason != nil {
			stopReason = stopReasonFromWire(*state.finishReason)
		}
		ch <- provider.StreamChunk{
			Done: &provider.StreamDone{
				StopReason: stopReason,
				Usage:      usage,
				Model:      state.model,
			},
		}
		state.doneSent = true

		if bus != nil {
			bus.Emit(observe.APIRequestCompleted{
				EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
				StopReason:  stopReason,
				Usage:       usage,
				DurationMs:  time.Since(state.startTime).Milliseconds(),
				Model:       state.model,
			})
		}
	}
}
