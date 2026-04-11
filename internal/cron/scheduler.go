package cron

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/artpar/gogent/internal/observe"
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

// NewScheduler creates a new Scheduler. If store is non-nil, durable jobs
// are loaded on creation and saved on mutation.
func NewScheduler(bus *observe.EventBus, store *Store) *Scheduler {
	s := &Scheduler{
		jobs:   make(map[string]*Job),
		bus:    bus,
		store:  store,
		stopCh: make(chan struct{}),
	}

	// Load durable jobs from disk
	if store != nil {
		saved, err := store.Load()
		if err == nil {
			now := time.Now()
			for i := range saved {
				j := saved[i]
				// Recompute next fire time from now
				expr, parseErr := Parse(j.Cron)
				if parseErr != nil {
					continue
				}
				j.NextFire = expr.NextAfter(now)
				s.jobs[j.ID] = &j
				// Track highest seq
				if n := extractSeq(j.ID); n > s.seq.Load() {
					s.seq.Store(n)
				}
			}
		}
	}

	return s
}

// extractSeq parses "cron-NNN" and returns NNN.
func extractSeq(id string) int64 {
	if len(id) <= 5 || id[:5] != "cron-" {
		return 0
	}
	var n int64
	for _, c := range id[5:] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

// Create adds a new scheduled job. Returns error if the cron expression
// is invalid or if the maximum job count (50) is reached.
func (s *Scheduler) Create(cronExpr, prompt string, recurring, durable bool) (*Job, error) {
	expr, err := Parse(cronExpr)
	if err != nil {
		return nil, errors.Join(ErrInvalidExpr, err)
	}

	now := time.Now()
	nextFire := expr.NextAfter(now)
	if nextFire.IsZero() {
		return nil, errors.Join(ErrInvalidExpr, errors.New("expression never matches"))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.jobs) >= 50 {
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

	// Return a copy
	copy := *j
	return &copy, nil
}

// Delete removes a job by ID.
func (s *Scheduler) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[id]; !ok {
		return ErrNotFound
	}
	delete(s.jobs, id)
	s.saveDurable()
	return nil
}

// List returns copies of all jobs sorted by NextFire.
func (s *Scheduler) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		copy := *j
		result = append(result, &copy)
	}
	sort.Slice(result, func(i, k int) bool {
		return result[i].NextFire.Before(result[k].NextFire)
	})
	return result
}

// Start begins the tick loop. It checks every minute for jobs whose
// NextFire <= now and calls handler for each. Non-recurring jobs are
// removed after firing. Blocks until ctx is cancelled or Stop() is called.
func (s *Scheduler) Start(ctx context.Context, handler func(job *Job)) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.tick(now, handler)
		}
	}
}

func (s *Scheduler) tick(now time.Time, handler func(job *Job)) {
	s.mu.Lock()
	var toFire []*Job
	for _, j := range s.jobs {
		if !j.NextFire.After(now) {
			copy := *j
			toFire = append(toFire, &copy)
		}
	}
	s.mu.Unlock()

	for _, j := range toFire {
		handler(j)

		s.mu.Lock()
		live, ok := s.jobs[j.ID]
		if ok {
			live.LastFired = now
			if live.Recurring {
				expr, err := Parse(live.Cron)
				if err == nil {
					live.NextFire = expr.NextAfter(now)
				}
			} else {
				delete(s.jobs, j.ID)
			}
		}
		s.saveDurable()
		s.mu.Unlock()
	}
}

// Stop shuts down the scheduler's tick loop.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

// saveDurable persists all durable jobs to the store.
// Must be called with s.mu held.
func (s *Scheduler) saveDurable() {
	if s.store == nil {
		return
	}
	var durable []Job
	for _, j := range s.jobs {
		if j.Durable {
			durable = append(durable, *j)
		}
	}
	// Best-effort save — emit on bus if it fails
	if err := s.store.Save(durable); err != nil && s.bus != nil {
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
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte(n%10) + '0'
		n /= 10
	}
	return string(buf[i:])
}
