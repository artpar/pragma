BINARY := bin/pragma
BINARY_WATCH := bin/pragma-watch

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOVERSION ?= $(shell go version | awk '{print $$3}')
LDFLAGS   := -X github.com/artpar/pragma/internal/buildinfo.Version=$(VERSION) \
             -X github.com/artpar/pragma/internal/buildinfo.Commit=$(COMMIT) \
             -X github.com/artpar/pragma/internal/buildinfo.Date=$(DATE) \
             -X github.com/artpar/pragma/internal/buildinfo.GoVersion=$(GOVERSION)

.PHONY: build watch test smoke ci clean completions instrument

# Re-run AST instrumentation (idempotent)
instrument:
	go run ./cmd/pragma-instrument/ ./internal/...

# Build the binary (instrumentation runs first)
build: instrument
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/pragma/

# Build the self-observer (no instrumentation needed)
watch:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_WATCH) ./cmd/pragma-watch/

# Generate shell completion scripts
completions: build
	@mkdir -p completions
	./$(BINARY) completion bash > completions/pragma.bash
	./$(BINARY) completion zsh > completions/pragma.zsh
	./$(BINARY) completion fish > completions/pragma.fish

# Run all unit tests
test:
	go test ./...

# Smoke test: build the binary and verify it survives initialization.
# No API key needed — we expect "API key required" (exit 1) which proves
# init succeeded. Any earlier crash (panic, registration error) is a failure.
smoke: build
	@echo "=== Smoke test: verifying binary starts ==="
	@OUTPUT=$$(ANTHROPIC_API_KEY=smoke-test ./$(BINARY) --prompt "test" 2>&1 || true); \
	if echo "$$OUTPUT" | grep -qi "panic\|fatal\|register tool.*already registered"; then \
		echo "$$OUTPUT"; \
		echo ""; \
		echo "FAIL: binary crashed during initialization"; \
		exit 1; \
	fi; \
	echo "OK: init succeeded"; \
	echo "=== Smoke test passed ==="

# E2E TUI tests using tmux (real terminal, real keystrokes, real screen capture)
e2e: build
	bash e2e/tui_test.sh

# Full CI pipeline: unit tests, smoke test, and e2e
ci: test smoke e2e
	@echo ""
	@echo "=== All CI checks passed ==="

clean:
	rm -rf bin/ completions/
