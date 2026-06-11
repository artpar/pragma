package cron

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

var (
	ErrMaxJobs     = errors.New("cron: maximum number of jobs (50) reached")
	ErrInvalidExpr = errors.New("cron: invalid cron expression")
	ErrNotFound    = errors.New("cron: job not found")
)

// Job represents a scheduled task.
type Job struct {
	ID        string    `json:"id"`
	Cron      string    `json:"cron"`
	Prompt    string    `json:"prompt"`
	Recurring bool      `json:"recurring"`
	Durable   bool      `json:"durable"`
	CreatedAt time.Time `json:"created_at"`
	LastFired time.Time `json:"last_fired_at,omitempty"`
	NextFire  time.Time `json:"next_fire"`
}

// Scheduler manages cron jobs. Thread-safe.
type Scheduler struct {
	mu       sync.RWMutex
	jobs     map[string]*Job
	bus      *observe.EventBus
	store    *Store
	seq      atomic.Int64
	stopOnce sync.Once
	stopCh   chan struct{}
}

type FireHandler func(job *Job) error

// NewScheduler creates a new Scheduler. If store is non-nil, durable jobs
// are loaded on creation and saved on mutation.
func NewScheduler(bus *observe.EventBus, store *Store) *Scheduler {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s := &Scheduler{
		jobs:   make(map[string]*Job),
		bus:    bus,
		store:  store,
		stopCh: make(chan struct{}),
	}

	if store != nil {
		observe.GlobalTrace("if: store != nil")
		_ = s.ReloadDurable(time.Now())
	}
	observe.GlobalTrace("return: s")

	return s
}

func (s *Scheduler) ReloadDurable(now time.Time) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if s.store == nil {
		observe.GlobalTrace("if: s.store == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	saved, err := s.store.Load()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}

	durable := make(map[string]*Job, len(saved))
	for i := range saved {
		observe.GlobalTrace("range saved")
		j := saved[i]
		expr, parseErr := Parse(j.Cron)
		if parseErr != nil {
			observe.GlobalTrace("if: parseErr != nil")
			continue
		}
		if j.NextFire.IsZero() {
			observe.GlobalTrace("if: j.NextFire.IsZero()")
			j.NextFire = expr.NextAfter(now)
		}
		durable[j.ID] = &j
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, live := range s.jobs {
		observe.GlobalTrace("range s.jobs")
		if live.Durable {
			observe.GlobalTrace("if: live.Durable")
			if _, ok := durable[id]; !ok {
				observe.GlobalTrace("if: !ok")
				delete(s.jobs, id)
			}
		}
	}
	for id, j := range durable {
		observe.GlobalTrace("range durable")
		copy := *j
		s.jobs[id] = &copy
		if n := extractSeq(id); n > s.seq.Load() {
			observe.GlobalTrace("if: n > s.seq.Load()")
			s.seq.Store(n)
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// extractSeq parses "cron-NNN" and returns NNN.
func extractSeq(id string) int64 {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(id) <= 5 || id[:5] != "cron-" {
		observe.GlobalTrace("if: len(id) <= 5 || id[:5] != \"cron-\"")
		observe.GlobalTrace("return: 0")
		return 0
	}
	var n int64
	for _, c := range id[5:] {
		observe.GlobalTrace("range id[5:]")
		if c < '0' || c > '9' {
			observe.GlobalTrace("if: c < '0' || c > '9'")
			observe.GlobalTrace("return: 0")
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	observe.GlobalTrace("return: n")
	return n
}

// Create adds a new scheduled job. Returns error if the cron expression
// is invalid or if the maximum job count (50) is reached.
func (s *Scheduler) Create(cronExpr, prompt string, recurring, durable bool) (*Job, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	expr, err := Parse(cronExpr)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, errors.Join(ErrInvalidExpr, err)")
		return nil, errors.Join(ErrInvalidExpr, err)
	}

	now := time.Now()
	nextFire := expr.NextAfter(now)
	if nextFire.IsZero() {
		observe.GlobalTrace("if: nextFire.IsZero()")
		observe.GlobalTrace("return: nil, errors.Join(ErrInvalidExpr, errors.New(\"expression never matches\"))")
		return nil, errors.Join(ErrInvalidExpr, errors.New("expression never matches"))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.jobs) >= 50 {
		observe.GlobalTrace("if: len(s.jobs) >= 50")
		observe.GlobalTrace("return: nil, ErrMaxJobs")
		return nil, ErrMaxJobs
	}

	seq := s.seq.Add(1)
	id := "cron-" + itoa(seq)

	j := &Job{
		ID:        id,
		Cron:      cronExpr,
		Prompt:    prompt,
		Recurring: recurring,
		Durable:   durable,
		CreatedAt: now,
		NextFire:  nextFire,
	}
	s.jobs[id] = j

	s.saveDurable()

	copy := *j
	observe.GlobalTrace("return: &copy, nil")
	return &copy, nil
}

// Delete removes a job by ID.
func (s *Scheduler) Delete(id string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[id]; !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: ErrNotFound")
		return ErrNotFound
	}
	delete(s.jobs, id)
	s.saveDurable()
	observe.GlobalTrace("return: nil")
	return nil
}

// List returns copies of all jobs sorted by NextFire.
func (s *Scheduler) List() []*Job {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		observe.GlobalTrace("range s.jobs")
		copy := *j
		result = append(result, &copy)
	}
	sort.Slice(result, func(i, k int) bool {
		return result[i].NextFire.Before(result[k].NextFire)
	})
	observe.GlobalTrace("return: result")
	return result
}

// Start begins the tick loop. It checks every minute for jobs whose
// NextFire <= now and calls handler for each. Successful non-recurring jobs are
// removed after firing. Blocks until ctx is cancelled or Stop() is called.
func (s *Scheduler) Start(ctx context.Context, handler FireHandler) {
	observe.TraceCtx(ctx, "cron", "Scheduler.Start", "enter")
	defer observe.TraceCtx(ctx, "cron", "Scheduler.Start", "exit")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		observe.TraceCtx(ctx, "cron", "Scheduler.Start", "for: true")
		select {
		case <-ctx.Done():
			observe.TraceCtx(ctx, "cron", "Scheduler.Start", "select: <-ctx.Done()")
			return
		case <-s.stopCh:
			observe.TraceCtx(ctx, "cron", "Scheduler.Start", "select: <-s.stopCh")
			return
		case now := <-ticker.C:
			observe.TraceCtx(ctx, "cron", "Scheduler.Start", "select: now := <-ticker.C")
			if err := s.ReloadDurable(now); err != nil && s.bus != nil {
				s.bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
					Severity:     "warn",
					Component:    "cron",
					ErrorType:    "reload_durable_jobs",
					ErrorMessage: err.Error(),
				})
			}
			s.tick(now, handler)
		}
	}
}

func (s *Scheduler) tick(now time.Time, handler FireHandler) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.mu.Lock()
	var toFire []*Job
	for _, j := range s.jobs {
		observe.GlobalTrace("range s.jobs")
		if !j.NextFire.After(now) {
			observe.GlobalTrace("if: !j.NextFire.After(now)")
			copy := *j
			toFire = append(toFire, &copy)
		}
	}
	s.mu.Unlock()

	for _, j := range toFire {
		observe.GlobalTrace("range toFire")
		if err := handler(j); err != nil {
			observe.GlobalTrace("if: err != nil")
			if s.bus != nil {
				observe.GlobalTrace("if: s.bus != nil")
				s.bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", "", observe.NewSpanID(), ""),
					Severity:     "warn",
					Component:    "cron",
					ErrorType:    "fire_failed",
					ErrorMessage: err.Error(),
				})
			}
			continue
		}

		s.mu.Lock()
		live, ok := s.jobs[j.ID]
		if ok {
			observe.GlobalTrace("if: ok")
			live.LastFired = now
			if live.Recurring {
				observe.GlobalTrace("if: live.Recurring")
				expr, err := Parse(live.Cron)
				if err == nil {
					observe.GlobalTrace("if: err == nil")
					live.NextFire = expr.NextAfter(now)
				}
			} else {
				observe.GlobalTrace("else: live.Recurring")
				delete(s.jobs, j.ID)
			}
		}
		s.saveDurable()
		s.mu.Unlock()
	}
}

// Stop shuts down the scheduler's tick loop.
func (s *Scheduler) Stop() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

// saveDurable persists all durable jobs to the store.
// Must be called with s.mu held.
func (s *Scheduler) saveDurable() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if s.store == nil {
		observe.GlobalTrace("if: s.store == nil")
		return
	}
	var durable []Job
	for _, j := range s.jobs {
		observe.GlobalTrace("range s.jobs")
		if j.Durable {
			observe.GlobalTrace("if: j.Durable")
			durable = append(durable, *j)
		}
	}

	if err := s.store.Save(durable); err != nil && s.bus != nil {
		observe.GlobalTrace("if: err != nil && s.bus != nil")
		s.bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", "", observe.NewSpanID(), ""),
			Severity:     "warn",
			Component:    "cron",
			ErrorType:    "store_save",
			ErrorMessage: err.Error(),
		})
	}
}

func itoa(n int64) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if n == 0 {
		observe.GlobalTrace("if: n == 0")
		observe.GlobalTrace("return: \"0\"")
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		observe.GlobalTrace("for: n > 0")
		i--
		buf[i] = byte(n%10) + '0'
		n /= 10
	}
	observe.GlobalTrace("return: string(buf[i:])")
	return string(buf[i:])
}
