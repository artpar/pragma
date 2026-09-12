package interactive

import (
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/slash"
)

// Event is the presentation-facing interactive stream.
type Event interface {
	interactiveEventSealed()
}

type AcceptedPromptEvent struct {
	Prompt string
}

func (AcceptedPromptEvent) interactiveEventSealed() {}

type RejectedPromptEvent struct {
	Prompt string
	Reason string
}

func (RejectedPromptEvent) interactiveEventSealed() {}

// QueuedPromptEvent reports a plain-text prompt submitted while a turn was
// running (INT-001): it was appended to the conversation for the next
// request boundary instead of starting a new turn.
type QueuedPromptEvent struct {
	Prompt string
}

func (QueuedPromptEvent) interactiveEventSealed() {}

// LoopEvent carries a domain/runtime query event through the interactive stream.
type LoopEvent struct {
	Event query.LoopEvent
}

func (LoopEvent) interactiveEventSealed() {}

// SlashResultEvent carries slash-command UI actions for interactive surfaces.
type SlashResultEvent struct {
	Result slash.Result
}

func (SlashResultEvent) interactiveEventSealed() {}

// RuntimeTerminatedEvent signals that the interactive runtime has completed
// its shutdown path has started and presentation surfaces should stop accepting input.
type RuntimeTerminatedEvent struct {
	Reason string
}

func (RuntimeTerminatedEvent) interactiveEventSealed() {}
