package google

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

// streamState tracks content being accumulated during streaming.
type streamState struct {
	toolCalls    map[string]*inProgressToolCall // wireID -> tool call record
	model        string
	doneSent     bool
	finishReason *string
	usage        model.TokenUsage
	startTime    time.Time
}

// inProgressToolCall tracks a tool call received during streaming.
// Google sends complete function calls per chunk (not incremental like OpenAI).
type inProgressToolCall struct {
	wireID     string
	internalID string
	name       string
}

// startStream begins consuming a Google SSE stream and writing StreamChunks
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
		defer func() { _ = resp.Body.Close() }()

		state := &streamState{
			toolCalls: make(map[string]*inProgressToolCall),
			startTime: time.Now(),
		}

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
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastEventEmit time.Time

	for scanner.Scan() {
		if ctx.Err() != nil {
			ch <- provider.StreamChunk{Error: ctx.Err()}
			return
		}

		if idleCh != nil {
			select {
			case idleCh <- struct{}{}:
			default:
			}
		}

		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk wireStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			if bus != nil {
				bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, spanID, ""),
					Severity:     "warning",
					Component:    "google/stream",
					ErrorType:    "malformed_chunk",
					ErrorMessage: fmt.Sprintf("skipping malformed stream chunk: %v", err),
				})
			}
			continue
		}

		if chunk.ModelVersion != "" {
			state.model = chunk.ModelVersion
		}

		p.dispatchStreamChunk(&chunk, mapper, ch, state, bus, traceID, spanID)

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

	// Google SSE has no [DONE] sentinel — stream ends when body closes.
	// Emit Done if we haven't already.
	if !state.doneSent {
		stopReason := model.StopEndTurn
		if state.finishReason != nil {
			stopReason = stopReasonFromWire(*state.finishReason)
		}
		// Check if we have tool calls — override stop reason
		if len(state.toolCalls) > 0 && stopReason == model.StopEndTurn {
			stopReason = model.StopToolUse
		}
		ch <- provider.StreamChunk{
			Done: &provider.StreamDone{
				StopReason: stopReason,
				Usage:      state.usage,
				Model:      state.model,
			},
		}
		state.doneSent = true
		if bus != nil {
			bus.Emit(observe.APIRequestCompleted{
				EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
				StopReason:  stopReason,
				Usage:       state.usage,
				DurationMs:  time.Since(state.startTime).Milliseconds(),
				Model:       state.model,
			})
		}
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
	// Accumulate usage from any chunk
	if chunk.UsageMetadata != nil {
		state.usage = usageFromWire(*chunk.UsageMetadata)
	}

	for _, candidate := range chunk.Candidates {
		if candidate.FinishReason != "" {
			state.finishReason = &candidate.FinishReason
		}

		for _, part := range candidate.Content.Parts {
			// Thinking text
			if part.Text != "" && part.Thought != nil && *part.Thought {
				ch <- provider.StreamChunk{ThinkingDelta: part.Text}
				continue
			}

			// Regular text
			if part.Text != "" {
				ch <- provider.StreamChunk{TextDelta: part.Text}
				continue
			}

			// Function call — Google sends complete function calls per chunk
			if part.FunctionCall != nil {
				internalID := model.NewUUID()
				// Gemini 3 provides a unique id; use it as the wire ID.
				wireID := part.FunctionCall.Id
				if wireID == "" {
					wireID = mapper.NextWireID(part.FunctionCall.Name)
				}
				mapper.RegisterPair(internalID, wireID)

				state.toolCalls[wireID] = &inProgressToolCall{
					wireID:     wireID,
					internalID: internalID,
					name:       part.FunctionCall.Name,
				}

				ch <- provider.StreamChunk{
					ToolCallStart: &model.ToolCallPart{
						ID:   internalID,
						Name: part.FunctionCall.Name,
					},
				}

				argsStr := string(part.FunctionCall.Args)
				if argsStr != "" && argsStr != "null" {
					ch <- provider.StreamChunk{
						ToolCallInputDelta: &provider.ToolCallDelta{
							ToolCallID: internalID,
							JSONDelta:  argsStr,
						},
					}
				}
				continue
			}
		}

		// If we got a finish reason AND usage, emit Done immediately
		if candidate.FinishReason != "" && chunk.UsageMetadata != nil && !state.doneSent {
			stopReason := stopReasonFromWire(candidate.FinishReason)
			if len(state.toolCalls) > 0 && stopReason == model.StopEndTurn {
				stopReason = model.StopToolUse
			}
			ch <- provider.StreamChunk{
				Done: &provider.StreamDone{
					StopReason: stopReason,
					Usage:      state.usage,
					Model:      state.model,
				},
			}
			state.doneSent = true

			if bus != nil {
				bus.Emit(observe.APIRequestCompleted{
					EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
					StopReason:  stopReason,
					Usage:       state.usage,
					DurationMs:  time.Since(state.startTime).Milliseconds(),
					Model:       state.model,
				})
			}
		}
	}
}
