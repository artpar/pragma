package anthropic

import (
	"context"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// streamState tracks content blocks being accumulated during streaming.
type streamState struct {
	blockTypes   map[int64]string // index → "text" | "tool_use" | "thinking"
	blockToolIDs map[int64]string // index → internal UUID
	startTime    time.Time
	doneSent     bool
	model        string
}

// startStream begins consuming an Anthropic SSE stream and writing StreamChunks
// to the returned channel. The channel is closed when the stream ends or errors.
// A watchdog goroutine cancels the stream if no events arrive within idleTimeout.
func (p *Provider) startStream(
	ctx context.Context,
	stream *ssestream.Stream[sdk.MessageStreamEventUnion],
	mapper *IDMapper,
	bus *observe.EventBus,
	traceID, spanID string,
) <-chan provider.StreamChunk {
	ch := make(chan provider.StreamChunk, 32)

	go func() {
		defer close(ch)
		defer stream.Close()

		state := &streamState{
			blockTypes:   make(map[int64]string),
			blockToolIDs: make(map[int64]string),
			startTime:    time.Now(),
		}

		// Idle timeout watchdog — cancels stream if no events arrive within idleTimeout
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

		p.consumeStream(streamCtx, stream, mapper, ch, state, bus, traceID, spanID, idleCh)
	}()

	return ch
}

// consumeStream processes SSE events and emits StreamChunks.
// idleCh may be nil if idle timeout is disabled.
func (p *Provider) consumeStream(
	ctx context.Context,
	stream *ssestream.Stream[sdk.MessageStreamEventUnion],
	mapper *IDMapper,
	ch chan<- provider.StreamChunk,
	state *streamState,
	bus *observe.EventBus,
	traceID, spanID string,
	idleCh chan<- struct{},
) {
	var lastEventEmit time.Time

	for stream.Next() {
		// Reset idle timeout watchdog
		if idleCh != nil {
			select {
			case idleCh <- struct{}{}:
			default:
			}
		}
		if ctx.Err() != nil {
			ch <- provider.StreamChunk{Error: ctx.Err()}
			return
		}

		now := time.Now()
		event := stream.Current()
		p.dispatchEvent(event, mapper, ch, state, bus, traceID, spanID)

		// Throttled observability event (at most 1/sec)
		if bus != nil && now.Sub(lastEventEmit) >= time.Second {
			bus.Emit(observe.APIStreamChunk{
				EventHeader: observe.NewEventHeader("APIStreamChunk", traceID, spanID, ""),
				ChunkType:   event.Type,
			})
			lastEventEmit = now
		}
	}

	// Check for stream error
	if err := stream.Err(); err != nil {
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

	// If Done was never sent, emit an error
	if !state.doneSent {
		ch <- provider.StreamChunk{Error: model.ErrStreamClosed}
	}
}

// dispatchEvent routes a single SSE event to the appropriate handler.
func (p *Provider) dispatchEvent(
	event sdk.MessageStreamEventUnion,
	mapper *IDMapper,
	ch chan<- provider.StreamChunk,
	state *streamState,
	bus *observe.EventBus,
	traceID, spanID string,
) {
	switch event.Type {
	case "message_start":
		// Capture model from the initial message
		if string(event.Message.Model) != "" {
			state.model = string(event.Message.Model)
		}

	case "content_block_start":
		p.handleBlockStart(event, mapper, ch, state)

	case "content_block_delta":
		p.handleBlockDelta(event, ch, state)

	case "content_block_stop":
		// Block finalized, no chunk needed

	case "message_delta":
		p.handleMessageDelta(event, ch, state, bus, traceID, spanID)

	case "message_stop":
		// Done already sent on message_delta
	}
}

func (p *Provider) handleBlockStart(
	event sdk.MessageStreamEventUnion,
	mapper *IDMapper,
	ch chan<- provider.StreamChunk,
	state *streamState,
) {
	blockType := event.ContentBlock.Type
	if blockType == "" {
		return
	}
	idx := event.Index
	state.blockTypes[idx] = blockType

	switch blockType {
	case "tool_use":
		internalID := model.NewUUID()
		wireID := event.ContentBlock.ID
		mapper.RegisterPair(internalID, wireID)
		state.blockToolIDs[idx] = internalID

		ch <- provider.StreamChunk{
			ToolCallStart: &model.ToolCallPart{
				ID:   internalID,
				Name: event.ContentBlock.Name,
			},
		}

	case "text", "thinking":
		// Track but don't emit a chunk

	case "redacted_thinking":
		// Redacted thinking arrives as a complete block (no deltas).
		// Emit directly — accumulator appends as ThinkingPart{Redacted: true}.
		ch <- provider.StreamChunk{
			RedactedThinkingBlock: &provider.RedactedThinking{
				Data: event.ContentBlock.Data,
			},
		}

	default:
		// Unknown block type — ignore
	}
}

func (p *Provider) handleBlockDelta(
	event sdk.MessageStreamEventUnion,
	ch chan<- provider.StreamChunk,
	state *streamState,
) {
	deltaType := event.Delta.Type
	switch deltaType {
	case "text_delta":
		ch <- provider.StreamChunk{TextDelta: event.Delta.Text}

	case "input_json_delta":
		idx := event.Index
		internalID, ok := state.blockToolIDs[idx]
		if !ok {
			return // orphaned delta — block start was skipped or unknown
		}
		ch <- provider.StreamChunk{
			ToolCallInputDelta: &provider.ToolCallDelta{
				ToolCallID: internalID,
				JSONDelta:  event.Delta.PartialJSON,
			},
		}

	case "thinking_delta":
		ch <- provider.StreamChunk{ThinkingDelta: event.Delta.Thinking}

	case "signature_delta":
		ch <- provider.StreamChunk{ThinkingSignatureDelta: event.Delta.Signature}

	case "citations_delta":
		// Skip citation deltas for now
	}
}

func (p *Provider) handleMessageDelta(
	event sdk.MessageStreamEventUnion,
	ch chan<- provider.StreamChunk,
	state *streamState,
	bus *observe.EventBus,
	traceID, spanID string,
) {
	usage := model.TokenUsage{
		InputTokens:              int(event.Usage.InputTokens),
		OutputTokens:             int(event.Usage.OutputTokens),
		CacheCreationInputTokens: int(event.Usage.CacheCreationInputTokens),
		CacheReadInputTokens:     int(event.Usage.CacheReadInputTokens),
	}

	stopReason := stopReasonFromWire(event.Delta.StopReason)

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
		})
	}
}
