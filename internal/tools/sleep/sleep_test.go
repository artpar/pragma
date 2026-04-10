package sleep

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSleep(t *testing.T) {
	tool := &Tool{}

	t.Run("name and flags", func(t *testing.T) {
		if tool.Name() != "Sleep" {
			t.Fatalf("expected Sleep, got %s", tool.Name())
		}
		flags := tool.Flags()
		if !flags.ReadOnly {
			t.Fatal("expected ReadOnly")
		}
		if !flags.Concurrent {
			t.Fatal("expected Concurrent")
		}
	})

	t.Run("short sleep completes", func(t *testing.T) {
		input := json.RawMessage(`{"seconds": 1}`)
		start := time.Now()
		result, err := tool.Invoke(context.Background(), input, nil)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed < 900*time.Millisecond {
			t.Fatalf("slept too short: %v", elapsed)
		}
		if result.Content != "Slept for 1 second." {
			t.Fatalf("unexpected content: %s", result.Content)
		}
	})

	t.Run("context cancellation interrupts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		input := json.RawMessage(`{"seconds": 300}`)

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		result, err := tool.Invoke(ctx, input, nil)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed > 1*time.Second {
			t.Fatalf("cancel took too long: %v", elapsed)
		}
		if result.Content != "Sleep cancelled." {
			t.Fatalf("unexpected content: %s", result.Content)
		}
	})

	t.Run("clamps to bounds", func(t *testing.T) {
		// Test lower bound clamp
		input := json.RawMessage(`{"seconds": 0}`)
		start := time.Now()
		result, err := tool.Invoke(context.Background(), input, nil)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed < 900*time.Millisecond {
			t.Fatalf("lower clamp failed, slept: %v", elapsed)
		}
		if result.Content != "Slept for 1 second." {
			t.Fatalf("unexpected content: %s", result.Content)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		input := json.RawMessage(`{"seconds": "abc"}`)
		_, err := tool.Invoke(context.Background(), input, nil)
		if err == nil {
			t.Fatal("expected error for invalid input")
		}
	})

	t.Run("schema is valid JSON", func(t *testing.T) {
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
			t.Fatalf("invalid schema JSON: %v", err)
		}
	})
}
