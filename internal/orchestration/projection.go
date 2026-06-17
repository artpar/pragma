package orchestration

import (
	"time"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/query"
)

type Projection struct {
	snapshot query.OrchestrationSnapshot
	active   bool
}

func NewProjection() *Projection {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Projection{}")
	return &Projection{}
}

func (p *Projection) Apply(ev query.LoopEvent, now time.Time) (query.OrchestrationSnapshot, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if p == nil {
		observe.GlobalTrace("if: p == nil")
		observe.GlobalTrace("return: query.OrchestrationSnapshot{}, false")
		return query.OrchestrationSnapshot{}, false
	}
	switch e := ev.(type) {
	case query.OrchestrationStartedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStartedEvent")
		p.snapshot = newSnapshot()
		p.snapshot.Name = e.Name
		p.snapshot.Initial = e.Initial
		p.snapshot.Current = e.Initial
		p.active = true
	case query.OrchestrationStateStartedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStateStartedEvent")
		p.ensure()
		row := p.state(e.StateID)
		row.Persona = firstNonEmpty(row.Persona, e.PersonaID)
		row.Control = firstNonEmpty(row.Control, e.Control)
		row.Status = "running"
		row.Started = now
		p.snapshot.Current = e.StateID
	case query.OrchestrationStateCompletedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStateCompletedEvent")
		p.ensure()
		row := p.state(e.StateID)
		row.Status = "completed"
		row.Completed = now
		row.Duration = e.Duration.Round(time.Second).String()
	case query.OrchestrationControlEvent:
		observe.GlobalTrace("typecase: query.OrchestrationControlEvent")
		p.ensure()
		row := p.state(e.StateID)
		row.Control = firstNonEmpty(row.Control, e.Control)
		row.LastEvent = firstNonEmpty(e.Event, row.LastEvent)
	case query.OrchestrationTransitionEvent:
		observe.GlobalTrace("typecase: query.OrchestrationTransitionEvent")
		p.ensure()
		p.snapshot.Transitions = append(p.snapshot.Transitions, query.OrchestrationTransitionSnapshot{
			From: e.From, Event: e.Event, To: e.To,
		})
		if e.To != "" {
			p.snapshot.Current = e.To
		}
	case query.OrchestrationHandoffEvent:
		observe.GlobalTrace("typecase: query.OrchestrationHandoffEvent")
		p.ensure()
		p.snapshot.Handoffs = append(p.snapshot.Handoffs, query.OrchestrationHandoffSnapshot{
			StateID:    e.StateID,
			From:       e.From,
			Event:      e.Event,
			To:         e.To,
			ArtifactID: e.ArtifactID,
			Path:       e.Path,
			Direction:  e.Direction,
			Bytes:      e.Bytes,
			SHA256:     e.SHA256,
		})
	case query.OrchestrationCompletedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationCompletedEvent")
		p.ensure()
		p.snapshot.Name = firstNonEmpty(p.snapshot.Name, e.Name)
		p.snapshot.Completed = true
		p.snapshot.Current = "completed"
	default:
		observe.GlobalTrace("typedefault")
		return query.OrchestrationSnapshot{}, false
	}
	observe.GlobalTrace("return: cloneSnapshot(p.snapshot), true")
	return cloneSnapshot(p.snapshot), true
}

func (p *Projection) ensure() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !p.active || p.snapshot.States == nil {
		observe.GlobalTrace("if: !p.active || p.snapshot.States == nil")
		p.snapshot = newSnapshot()
		p.active = true
	}
}

func (p *Projection) state(id string) *query.OrchestrationStateSnapshot {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if id == "" {
		observe.GlobalTrace("if: id == \"\"")
		id = "unknown"
	}
	if p.snapshot.States == nil {
		observe.GlobalTrace("if: p.snapshot.States == nil")
		p.snapshot.States = make(map[string]*query.OrchestrationStateSnapshot)
	}
	row := p.snapshot.States[id]
	if row == nil {
		observe.GlobalTrace("if: row == nil")
		row = &query.OrchestrationStateSnapshot{ID: id}
		p.snapshot.States[id] = row
	}
	observe.GlobalTrace("return: row")
	return row
}

func newSnapshot() query.OrchestrationSnapshot {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: query.OrchestrationSnapshot{States: make(map[string]*query.OrchestrationState...")
	return query.OrchestrationSnapshot{States: make(map[string]*query.OrchestrationStateSnapshot)}
}

func cloneSnapshot(in query.OrchestrationSnapshot) query.OrchestrationSnapshot {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := in
	out.States = make(map[string]*query.OrchestrationStateSnapshot, len(in.States))
	for id, row := range in.States {
		observe.GlobalTrace("range in.States")
		cp := *row
		out.States[id] = &cp
	}
	out.Transitions = append([]query.OrchestrationTransitionSnapshot(nil), in.Transitions...)
	out.Handoffs = append([]query.OrchestrationHandoffSnapshot(nil), in.Handoffs...)
	observe.GlobalTrace("return: out")
	return out
}

func firstNonEmpty(values ...string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, value := range values {
		observe.GlobalTrace("range values")
		if value != "" {
			observe.GlobalTrace("if: value != \"\"")
			observe.GlobalTrace("return: value")
			return value
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}
