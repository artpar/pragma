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
	finishReason *string             // saved from chunk with finish_reason but no usage
	usage        model.TokenUsage    // accumulated from any chunk carrying usage data
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
	observe.TraceCtx(ctx, "openai", "Provider.startStream", "enter")
	defer observe.TraceCtx(ctx, "openai", "Provider.startStream", "exit")
	ch := make(chan provider.StreamChunk, 32)

	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		state := &streamState{
			toolCalls: make(map[int]*inProgressToolCall),
			startTime: time.Now(),
		}

		streamCtx := ctx
		var idleCh chan struct{}
		if p.idleTimeout > 0 {
			observe.TraceCtx(ctx, "openai", "Provider.startStream", "if: p.idleTimeout > 0")
			var watchCancel context.CancelFunc
			streamCtx, watchCancel = context.WithCancel(ctx)
			defer watchCancel()
			idleCh = make(chan struct{}, 1)
			go func() {
				timer := time.NewTimer(p.idleTimeout)
				defer timer.Stop()
				for {
					observe.TraceCtx(ctx, "openai", "Provider.startStream", "for: true")
					select {
					case <-idleCh:
						observe.TraceCtx(ctx, "openai", "Provider.startStream", "select: <-idleCh")
						if !timer.Stop() {
							<-timer.C
						}
						timer.Reset(p.idleTimeout)
					case <-timer.C:
						observe.TraceCtx(ctx, "openai", "Provider.startStream", "select: <-timer.C")
						watchCancel()
						return
					case <-streamCtx.Done():
						observe.TraceCtx(ctx, "openai", "Provider.startStream", "select: <-streamCtx.Done()")
						return
					}
				}
			}()
		}

		p.consumeSSE(streamCtx, resp, mapper, ch, state, bus, traceID, spanID, idleCh)
	}()
	observe.TraceCtx(ctx, "openai", "Provider.startStream", "return: ch")

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
	observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "enter")
	defer observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "exit")
	scanner := bufio.NewScanner(resp.Body)

	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastEventEmit time.Time

	for scanner.Scan() {
		observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "for: scanner.Scan()")
		if ctx.Err() != nil {
			observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: ctx.Err() != nil")
			ch <- provider.StreamChunk{Error: ctx.Err()}
			return
		}

		if idleCh != nil {
			observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: idleCh != nil")
			select {
			case idleCh <- struct{}{}:
				observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "select: idleCh <- struct{}{}")
			default:
				observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "select: default")
			}
		}

		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: !strings.HasPrefix(line, \"data: \")")
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: data == \"[DONE]\"")

			if !state.doneSent && state.finishReason != nil {
				observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: !state.doneSent && state.finishReason != nil")
				ch <- provider.StreamChunk{
					Done: &provider.StreamDone{
						StopReason: stopReasonFromWire(*state.finishReason),
						Usage:      state.usage,
						Model:      state.model,
					},
				}
				state.doneSent = true
				if bus != nil {
					bus.Emit(observe.APIRequestCompleted{
						EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
						StopReason:  stopReasonFromWire(*state.finishReason),
						Usage:       state.usage,
						DurationMs:  time.Since(state.startTime).Milliseconds(),
						Model:       state.model,
					})
				}
			}
			if !state.doneSent {
				observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: !state.doneSent")
				ch <- provider.StreamChunk{Error: model.ErrStreamClosed}
			}
			return
		}

		var chunk wireStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: err != nil")
			if bus != nil {
				observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: bus != nil")
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
			observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: chunk.Model != \"\"")
			state.model = chunk.Model
		}

		p.dispatchStreamChunk(&chunk, mapper, ch, state, bus, traceID, spanID)

		now := time.Now()
		if bus != nil && now.Sub(lastEventEmit) >= time.Second {
			observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: bus != nil && now.Sub(lastEventEmit) >= time.Second")
			bus.Emit(observe.APIStreamChunk{
				EventHeader: observe.NewEventHeader("APIStreamChunk", traceID, spanID, ""),
				ChunkType:   "stream_delta",
			})
			lastEventEmit = now
		}
	}

	if err := scanner.Err(); err != nil {
		observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: err != nil")
		classified := classifyError(err)
		if bus != nil {
			observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: bus != nil")
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

	if !state.doneSent && state.finishReason != nil {
		observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: !state.doneSent && state.finishReason != nil")
		ch <- provider.StreamChunk{
			Done: &provider.StreamDone{
				StopReason: stopReasonFromWire(*state.finishReason),
				Usage:      state.usage,
				Model:      state.model,
			},
		}
		state.doneSent = true
		if bus != nil {
			bus.Emit(observe.APIRequestCompleted{
				EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
				StopReason:  stopReasonFromWire(*state.finishReason),
				Usage:       state.usage,
				DurationMs:  time.Since(state.startTime).Milliseconds(),
				Model:       state.model,
			})
		}
	}

	if !state.doneSent {
		observe.TraceCtx(ctx, "openai", "Provider.consumeSSE", "if: !state.doneSent")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, choice := range chunk.Choices {
		observe.GlobalTrace("range chunk.Choices")
		delta := choice.Delta

		if delta.Content != nil && *delta.Content != "" {
			observe.GlobalTrace("if: delta.Content != nil && *delta.Content != \"\"")
			ch <- provider.StreamChunk{TextDelta: *delta.Content}
		}

		for _, tc := range delta.ToolCalls {
			observe.GlobalTrace("range delta.ToolCalls")
			if tc.ID != "" {
				observe.GlobalTrace("if: tc.ID != \"\"")

				internalID := model.NewUUID()
				mapper.RegisterPair(internalID, tc.ID)
				name := ""
				if tc.Function != nil {
					observe.GlobalTrace("if: tc.Function != nil")
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

			if tc.Function != nil && tc.Function.Arguments != "" {
				observe.GlobalTrace("if: tc.Function != nil && tc.Function.Arguments != \"\"")
				ipc, ok := state.toolCalls[tc.Index]
				if ok {
					observe.GlobalTrace("if: ok")
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

		if choice.FinishReason != nil {
			observe.GlobalTrace("if: choice.FinishReason != nil")

			state.finishReason = choice.FinishReason

			if chunk.Usage != nil {
				observe.GlobalTrace("if: chunk.Usage != nil")
				usage := usageFromWire(*chunk.Usage)
				state.usage = usage
				ch <- provider.StreamChunk{
					Done: &provider.StreamDone{
						StopReason: stopReasonFromWire(*choice.FinishReason),
						Usage:      usage,
						Model:      state.model,
					},
				}
				state.doneSent = true

				if bus != nil {
					observe.GlobalTrace("if: bus != nil")
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

	if chunk.Usage != nil && !state.doneSent {
		observe.GlobalTrace("if: chunk.Usage != nil && !state.doneSent")
		usage := usageFromWire(*chunk.Usage)
		state.usage = usage

		stopReason := model.StopEndTurn
		if state.finishReason != nil {
			observe.GlobalTrace("if: state.finishReason != nil")
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
			observe.GlobalTrace("if: bus != nil")
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
