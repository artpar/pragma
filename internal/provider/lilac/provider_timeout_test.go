package lilac

import (
	"testing"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

func TestProviderUsesLilacRequestTimeout(t *testing.T) {
	p, err := New("test-key", observe.NewEventBus(1))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := p.httpClient.Timeout, 360*time.Second; got != want {
		t.Fatalf("HTTP client timeout = %s, want %s", got, want)
	}
}
