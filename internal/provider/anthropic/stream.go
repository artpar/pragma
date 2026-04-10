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

		p.consumeStream(ctx, stream, mapper, ch, state, bus, traceID, spanID)
	}()

	return ch
}

// consumeStream processes SSE events and emits StreamChunks.
func (p *Provider) consumeStream(
	ctx context.Context,
	stream *ssestream.Stream[sdk.MessageStreamEventUnion],
	mapper *IDMapper,
	ch chan<- provider.StreamChunk,
	state *streamState,
	bus *observe.EventBus,
	traceID, spanID string,
) {
	var lastEventEmit time.Time

	for stream.Next() {
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
				EventHeader: observe.NewEventHeader("api.stream_chunk", traceID, spanID, ""),
				ChunkType:   event.Type,
			})
			lastEventEmit = now
		}
	}

	// Check for stream error
	if err := stream.Err(); err != nil {
		classified := classifyError(err)
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
		// Nothing to emit; state tracked in provider.go via usage on message_delta

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
	idx := event.Index
	blockType := event.ContentBlock.Type
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
		internalID := state.blockToolIDs[idx]
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
		},
	}
	state.doneSent = true

	if bus != nil {
		bus.Emit(observe.APIRequestCompleted{
			EventHeader: observe.NewEventHeader("api.request_completed", traceID, spanID, ""),
			StopReason:  stopReason,
			Usage:       usage,
			DurationMs:  time.Since(state.startTime).Milliseconds(),
		})
	}
}
