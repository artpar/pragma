package app

import (
	"sync"
	"testing"
)

func TestStateStoreSnapshot(t *testing.T) {
	store := NewStateStore(AppState{
		CWD:   "/tmp/work",
		Model: "claude-sonnet-4-20250514",
	})

	snap := store.Snapshot()
	if snap.CWD != "/tmp/work" {
		t.Errorf("CWD: got %q", snap.CWD)
	}
	if snap.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: got %q", snap.Model)
	}
}

func TestStateStoreUpdate(t *testing.T) {
	store := NewStateStore(AppState{CWD: "/tmp"})

	store.Update(func(s *AppState) {
		s.CWD = "/home"
		s.Model = "gpt-4"
	})

	snap := store.Snapshot()
	if snap.CWD != "/home" {
		t.Errorf("CWD after update: got %q", snap.CWD)
	}
	if snap.Model != "gpt-4" {
		t.Errorf("Model after update: got %q", snap.Model)
	}
}

func TestStateStoreSnapshotIsCopy(t *testing.T) {
	store := NewStateStore(AppState{CWD: "/tmp"})

	snap := store.Snapshot()
	snap.CWD = "/modified"

	fresh := store.Snapshot()
	if fresh.CWD == "/modified" {
		t.Error("Snapshot returned a reference, not a copy")
	}
}

func TestStateStoreConcurrent(t *testing.T) {
	store := NewStateStore(AppState{CWD: "/tmp", MaxTokens: 0})

	var wg sync.WaitGroup
	n := 100

	// Concurrent writers
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			store.Update(func(s *AppState) {
				s.MaxTokens++
			})
		}()
	}

	// Concurrent readers
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			_ = store.Snapshot()
		}()
	}

	wg.Wait()

	snap := store.Snapshot()
	if snap.MaxTokens != n {
		t.Errorf("MaxTokens: got %d, want %d", snap.MaxTokens, n)
	}
}

func TestStateStoreWorkDir(t *testing.T) {
	store := NewStateStore(AppState{CWD: "/workspace"})
	snap := store.Snapshot()
	if snap.WorkDir() != "/workspace" {
		t.Errorf("WorkDir: got %q", snap.WorkDir())
	}
}
