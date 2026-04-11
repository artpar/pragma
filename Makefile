BINARY := bin/gogent

.PHONY: build test archtest smoke ci clean

# Build the binary
build:
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/gogent/

# Run all unit tests
test:
	go test ./...

# Run architecture constraint tests
archtest:
	go test ./internal/archtest/ -v

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

# E2E TUI tests using tui-use (real PTY, real keystrokes, real screen capture)
e2e: build
	bash e2e/tui_test.sh

# Full CI pipeline: unit tests → arch tests → smoke test → e2e
ci: test archtest smoke e2e
	@echo ""
	@echo "=== All CI checks passed ==="

clean:
	rm -rf bin/
