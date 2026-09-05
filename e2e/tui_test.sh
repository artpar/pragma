#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
source "$SCRIPT_DIR/harness.sh"

BINARY="$PROJECT_DIR/bin/pragma"
API_KEY="${ANTHROPIC_API_KEY:-smoke-test}"
PASS=0
FAIL=0

trap cleanup_all EXIT
mkdir -p "$SNAPSHOT_DIR"

echo "Building pragma..."
(cd "$PROJECT_DIR" && make build)
echo ""

# --------------------------------------------------
# Test 1: /help
# --------------------------------------------------
test_help() {
    local S="pragma-e2e-help-$$"
    echo "=== Test: /help ==="
    tmux_start "$S" env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
    tmux_wait_ready "$S"
    tmux_send "$S" "/help"
    tmux_enter "$S"
    tmux_wait_for "$S" "compact" 5
    tmux_assert "$S" "compact" "/help lists /compact"
    tmux_assert "$S" "exit" "/help lists /exit"
    tmux_assert "$S" "cost" "/help lists /cost"
    tmux_kill "$S"
}

# --------------------------------------------------
# Test 2: /cost
# --------------------------------------------------
test_cost() {
    local S="pragma-e2e-cost-$$"
    echo ""
    echo "=== Test: /cost ==="
    tmux_start "$S" env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
    tmux_wait_ready "$S"
    tmux_send "$S" "/cost"
    tmux_enter "$S"
    tmux_wait_for "$S" "0\\.00" 5
    tmux_assert "$S" "0\\.00" "/cost shows cost"
    tmux_kill "$S"
}

# --------------------------------------------------
# Test 3: /exit quits the program
# --------------------------------------------------
test_exit() {
    local S="pragma-e2e-exit-$$"
    echo ""
    echo "=== Test: /exit ==="
    tmux_start "$S" env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
    tmux_wait_ready "$S"
    tmux_send "$S" "/exit"
    tmux_enter "$S"
    sleep 3
    if tmux_session_alive "$S"; then
        echo "  FAIL: /exit did not terminate the program"
        tmux_save_snapshot "$S" "exit_still_running"
        FAIL=$((FAIL + 1))
        tmux_kill "$S"
    else
        echo "  PASS: /exit terminated the program"
        PASS=$((PASS + 1))
    fi
}

# --------------------------------------------------
# Test 4: /unknown command shows error
# --------------------------------------------------
test_unknown() {
    local S="pragma-e2e-unknown-$$"
    echo ""
    echo "=== Test: /unknown ==="
    tmux_start "$S" env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
    tmux_wait_ready "$S"
    tmux_send "$S" "/nonexistent"
    tmux_enter "$S"
    tmux_wait_for "$S" "[Ee]rror|[Uu]nknown" 5
    tmux_assert "$S" "[Ee]rror|[Uu]nknown" "/unknown shows error"
    tmux_kill "$S"
}

# --------------------------------------------------
# Test 5: Regular message (real API key only)
# --------------------------------------------------
test_message() {
    if [ "$API_KEY" = "smoke-test" ]; then
        echo ""
        echo "=== Test: regular message === (SKIPPED: no real API key)"
        return
    fi
    local S="pragma-e2e-msg-$$"
    echo ""
    echo "=== Test: regular message ==="
    tmux_start "$S" env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
    tmux_wait_ready "$S"
    tmux_send "$S" "say hello in one word"
    tmux_enter "$S"
    tmux_wait_for "$S" "turns: 1" 30
    tmux_assert "$S" "turns: 1" "regular message incremented turn counter"
    tmux_kill "$S"
}

# --------------------------------------------------
# Test 6: Provider picker (interactive selection)
# --------------------------------------------------
test_provider_picker() {
    echo ""
    echo "=== Test: provider picker ==="

    local TEMP_HOME
    TEMP_HOME=$(mktemp -d)
    harness_track_tmpdir "$TEMP_HOME"
    mkdir -p "$TEMP_HOME/.pragma"
    cat > "$TEMP_HOME/.pragma/credentials.yml" << 'CREDS'
providers:
  google:
    api_key: fake-google-key-for-picker-test
  openrouter:
    api_key: fake-openrouter-key-for-picker-test
CREDS

    local S="pragma-e2e-picker-$$"
    tmux_start "$S" env -i HOME="$TEMP_HOME" PATH="$PATH" TERM="${TERM:-xterm-256color}" "$BINARY"

    if ! tmux_wait_for "$S" "Select provider" 10; then
        echo "  FAIL: provider picker menu did not appear"
        tmux_save_snapshot "$S" "picker_no_menu"
        FAIL=$((FAIL + 1))
        tmux_kill "$S"
        return
    fi

    tmux_assert "$S" "google" "picker shows google"
    tmux_assert "$S" "openrouter" "picker shows openrouter"

    # google is alphabetically first → option 1
    tmux_send "$S" "1"
    tmux_enter "$S"

    tmux_wait_ready "$S" 15
    tmux_assert "$S" "gemini" "picker selected google (shows gemini model)"

    tmux_kill "$S"
}

# --- Run all tests ---
test_help
test_cost
test_exit
test_unknown
test_message
test_provider_picker

# --- Summary ---
echo ""
echo "========================"
echo "Results: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
echo "All e2e tests passed"
