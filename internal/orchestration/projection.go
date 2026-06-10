package orchestration

import (
	"time"

	"github.com/artpar/pragma/internal/query"
)

type Projection struct {
	snapshot query.OrchestrationSnapshot
	active   bool
}

func NewProjection() *Projection {
	return &Projection{}
}

func (p *Projection) Apply(ev query.LoopEvent, now time.Time) (query.OrchestrationSnapshot, bool) {
	if p == nil {
		return query.OrchestrationSnapshot{}, false
	}
	switch e := ev.(type) {
	case query.OrchestrationStartedEvent:
		p.snapshot = newSnapshot()
		p.snapshot.Name = e.Name
		p.snapshot.Initial = e.Initial
		p.snapshot.Current = e.Initial
		p.active = true
	case query.OrchestrationStateStartedEvent:
		p.ensure()
		row := p.state(e.StateID)
		row.Persona = firstNonEmpty(row.Persona, e.PersonaID)
		row.Control = firstNonEmpty(row.Control, e.Control)
		row.Status = "running"
		row.Started = now
		p.snapshot.Current = e.StateID
	case query.OrchestrationStateCompletedEvent:
		p.ensure()
		row := p.state(e.StateID)
		row.Status = "completed"
		row.Completed = now
		row.Duration = e.Duration.Round(time.Second).String()
	case query.OrchestrationControlEvent:
		p.ensure()
		row := p.state(e.StateID)
		row.Control = firstNonEmpty(row.Control, e.Control)
		row.LastEvent = firstNonEmpty(e.Event, row.LastEvent)
	case query.OrchestrationTransitionEvent:
		p.ensure()
		p.snapshot.Transitions = append(p.snapshot.Transitions, query.OrchestrationTransitionSnapshot{
			From: e.From, Event: e.Event, To: e.To,
		})
		if e.To != "" {
			p.snapshot.Current = e.To
		}
	case query.OrchestrationHandoffEvent:
		p.ensure()
		p.snapshot.Handoffs = append(p.snapshot.Handoffs, query.OrchestrationHandoffSnapshot{
			StateID:    e.StateID,
			From:       e.From,
			Event:      e.Event,
			To:         e.To,
			ArtifactID: e.ArtifactID,
			Path:       e.Path,
			Direction:  e.Direction,
		})
	case query.OrchestrationCompletedEvent:
		p.ensure()
		p.snapshot.Name = firstNonEmpty(p.snapshot.Name, e.Name)
		p.snapshot.Completed = true
		p.snapshot.Current = "completed"
	default:
		return query.OrchestrationSnapshot{}, false
	}
	return cloneSnapshot(p.snapshot), true
}

func (p *Projection) ensure() {
	if !p.active || p.snapshot.States == nil {
		p.snapshot = newSnapshot()
		p.active = true
	}
}

func (p *Projection) state(id string) *query.OrchestrationStateSnapshot {
	if id == "" {
		id = "unknown"
	}
	if p.snapshot.States == nil {
		p.snapshot.States = make(map[string]*query.OrchestrationStateSnapshot)
	}
	row := p.snapshot.States[id]
	if row == nil {
		row = &query.OrchestrationStateSnapshot{ID: id}
		p.snapshot.States[id] = row
	}
	return row
}

func newSnapshot() query.OrchestrationSnapshot {
	return query.OrchestrationSnapshot{States: make(map[string]*query.OrchestrationStateSnapshot)}
}

func cloneSnapshot(in query.OrchestrationSnapshot) query.OrchestrationSnapshot {
	out := in
	out.States = make(map[string]*query.OrchestrationStateSnapshot, len(in.States))
	for id, row := range in.States {
		cp := *row
		out.States[id] = &cp
	}
	out.Transitions = append([]query.OrchestrationTransitionSnapshot(nil), in.Transitions...)
	out.Handoffs = append([]query.OrchestrationHandoffSnapshot(nil), in.Handoffs...)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
