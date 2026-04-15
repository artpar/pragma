package anthropic

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/shared"
)

// streamState tracks content blocks being accumulated during streaming.
type streamState struct {
	blockTypes     map[int64]string // index → "text" | "tool_use" | "thinking"
	blockToolIDs   map[int64]string // index → internal UUID
	blockToolNames map[int64]string // index → tool name
	startTime      time.Time
	doneSent       bool
	model          string
	accText        strings.Builder
	accToolInputs  map[string]*strings.Builder // internalID → accumulated JSON
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
	observe.TraceCtx(ctx, "anthropic", "Provider.startStream", "enter")
	defer observe.TraceCtx(ctx, "anthropic", "Provider.startStream", "exit")
	ch := make(chan provider.StreamChunk, 32)

	go func() {
		defer close(ch)
		defer stream.Close()

		state := &streamState{
			blockTypes:     make(map[int64]string),
			blockToolIDs:   make(map[int64]string),
			blockToolNames: make(map[int64]string),
			startTime:      time.Now(),
			accToolInputs:  make(map[string]*strings.Builder),
		}

		streamCtx := ctx
		var idleCh chan struct{}
		if p.idleTimeout > 0 {
			observe.TraceCtx(ctx, "anthropic", "Provider.startStream", "if: p.idleTimeout > 0")
			var watchCancel context.CancelFunc
			streamCtx, watchCancel = context.WithCancel(ctx)
			defer watchCancel()
			idleCh = make(chan struct{}, 1)
			go func() {
				timer := time.NewTimer(p.idleTimeout)
				defer timer.Stop()
				for {
					observe.TraceCtx(ctx, "anthropic", "Provider.startStream", "for: true")
					select {
					case <-idleCh:
						observe.TraceCtx(ctx, "anthropic", "Provider.startStream", "select: <-idleCh")
						if !timer.Stop() {
							<-timer.C
						}
						timer.Reset(p.idleTimeout)
					case <-timer.C:
						observe.TraceCtx(ctx, "anthropic", "Provider.startStream", "select: <-timer.C")
						watchCancel()
						return
					case <-streamCtx.Done():
						observe.TraceCtx(ctx, "anthropic", "Provider.startStream", "select: <-streamCtx.Done()")
						return
					}
				}
			}()
		}

		p.consumeStream(streamCtx, stream, mapper, ch, state, bus, traceID, spanID, idleCh)
	}()
	observe.TraceCtx(ctx, "anthropic", "Provider.startStream", "return: ch")

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
	observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "enter")
	defer observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "exit")
	var lastEventEmit time.Time

	for stream.Next() {
		observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "for: stream.Next()")

		if idleCh != nil {
			observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "if: idleCh != nil")
			select {
			case idleCh <- struct{}{}:
				observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "select: idleCh <- struct{}{}")
			default:
				observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "select: default")
			}
		}
		if ctx.Err() != nil {
			observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "if: ctx.Err() != nil")
			ch <- provider.StreamChunk{Error: ctx.Err()}
			return
		}

		now := time.Now()
		event := stream.Current()
		p.dispatchEvent(event, mapper, ch, state, bus, traceID, spanID)

		if bus != nil && now.Sub(lastEventEmit) >= time.Second {
			observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "if: bus != nil && now.Sub(lastEventEmit) >= time.Second")
			bus.Emit(observe.APIStreamChunk{
				EventHeader: observe.NewEventHeader("APIStreamChunk", traceID, spanID, ""),
				ChunkType:   event.Type,
			})
			lastEventEmit = now
		}
	}

	if err := stream.Err(); err != nil {
		observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "if: err != nil")
		classified := classifyError(err)
		if bus != nil {
			observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "if: bus != nil")
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

	if !state.doneSent {
		observe.TraceCtx(ctx, "anthropic", "Provider.consumeStream", "if: !state.doneSent")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch event.Type {
	case "message_start":
		observe.GlobalTrace("case: \"message_start\"")

		if string(event.Message.Model) != "" {
			state.model = string(event.Message.Model)
		}

	case "content_block_start":
		observe.GlobalTrace("case: \"content_block_start\"")
		p.handleBlockStart(event, mapper, ch, state)

	case "content_block_delta":
		observe.GlobalTrace("case: \"content_block_delta\"")
		p.handleBlockDelta(event, ch, state)

	case "content_block_stop":
		observe.GlobalTrace("case: \"content_block_stop\"")

	case "message_delta":
		observe.GlobalTrace("case: \"message_delta\"")
		p.handleMessageDelta(event, ch, state, bus, traceID, spanID)

	case "message_stop":
		observe.GlobalTrace("case: \"message_stop\"")

	default:
		observe.GlobalTrace("default")
		if bus != nil && event.Type != "" {
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, spanID, ""),
				Severity:     "warn",
				Component:    "anthropic_stream",
				ErrorType:    "unknown_event_type",
				ErrorMessage: "unhandled stream event type: " + event.Type,
			})
		}
	}
}

func (p *Provider) handleBlockStart(
	event sdk.MessageStreamEventUnion,
	mapper *IDMapper,
	ch chan<- provider.StreamChunk,
	state *streamState,
) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	blockType := event.ContentBlock.Type
	if blockType == "" {
		observe.GlobalTrace("if: blockType == \"\"")
		return
	}
	idx := event.Index
	state.blockTypes[idx] = blockType

	switch blockType {
	case "tool_use":
		observe.GlobalTrace("case: \"tool_use\"")
		internalID := model.NewUUID()
		wireID := event.ContentBlock.ID
		mapper.RegisterPair(internalID, wireID)
		state.blockToolIDs[idx] = internalID
		state.blockToolNames[idx] = event.ContentBlock.Name
		state.accToolInputs[internalID] = &strings.Builder{}

		ch <- provider.StreamChunk{
			ToolCallStart: &model.ToolCallPart{
				ID:   internalID,
				Name: event.ContentBlock.Name,
			},
		}

	case "text", "thinking":
		observe.GlobalTrace("case: \"text\", \"thinking\"")

	case "redacted_thinking":
		observe.GlobalTrace("case: \"redacted_thinking\"")

		ch <- provider.StreamChunk{
			RedactedThinkingBlock: &provider.RedactedThinking{
				Data: event.ContentBlock.Data,
			},
		}

	default:
		observe.GlobalTrace("default")

	}
}

func (p *Provider) handleBlockDelta(
	event sdk.MessageStreamEventUnion,
	ch chan<- provider.StreamChunk,
	state *streamState,
) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	deltaType := event.Delta.Type
	switch deltaType {
	case "text_delta":
		observe.GlobalTrace("case: \"text_delta\"")
		state.accText.WriteString(event.Delta.Text)
		ch <- provider.StreamChunk{TextDelta: event.Delta.Text}

	case "input_json_delta":
		observe.GlobalTrace("case: \"input_json_delta\"")
		idx := event.Index
		internalID, ok := state.blockToolIDs[idx]
		if !ok {
			return
		}
		if b, ok := state.accToolInputs[internalID]; ok {
			b.WriteString(event.Delta.PartialJSON)
		}
		ch <- provider.StreamChunk{
			ToolCallInputDelta: &provider.ToolCallDelta{
				ToolCallID: internalID,
				JSONDelta:  event.Delta.PartialJSON,
			},
		}

	case "thinking_delta":
		observe.GlobalTrace("case: \"thinking_delta\"")
		ch <- provider.StreamChunk{ThinkingDelta: event.Delta.Thinking}

	case "signature_delta":
		observe.GlobalTrace("case: \"signature_delta\"")
		ch <- provider.StreamChunk{ThinkingSignatureDelta: event.Delta.Signature}

	case "citations_delta":
		observe.GlobalTrace("case: \"citations_delta\"")

	}
}

func (p *Provider) handleMessageDelta(
	event sdk.MessageStreamEventUnion,
	ch chan<- provider.StreamChunk,
	state *streamState,
	bus *observe.EventBus,
	traceID, spanID string,
) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
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
		observe.GlobalTrace("if: bus != nil")
		var accContent []model.ContentPart
		if state.accText.Len() > 0 {
			accContent = append(accContent, model.TextPart{Text: state.accText.String()})
		}
		for idx, internalID := range state.blockToolIDs {
			tc := model.ToolCallPart{ID: internalID, Name: state.blockToolNames[idx]}
			if b, ok := state.accToolInputs[internalID]; ok {
				tc.Input = json.RawMessage(b.String())
			}
			accContent = append(accContent, tc)
		}
		bus.Emit(observe.APIRequestCompleted{
			EventHeader: observe.NewEventHeader("APIRequestCompleted", traceID, spanID, ""),
			StopReason:  stopReason,
			Usage:       usage,
			DurationMs:  time.Since(state.startTime).Milliseconds(),
			Content:     shared.MarshalContent(accContent),
		})
	}
}
