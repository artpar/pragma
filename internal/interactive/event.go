package interactive

import (
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/slash"
)

// Event is the presentation-facing interactive stream.
type Event interface {
	interactiveEventSealed()
}

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
