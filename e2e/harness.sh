#!/bin/bash
# e2e/harness.sh — Reusable tmux test primitives for gogent E2E tests.
# Source this file; do not execute directly.

if ! command -v tmux &>/dev/null; then
    echo "FATAL: tmux is required for E2E tests but not found in PATH" >&2
    exit 1
fi

# --- Configuration ---
SNAPSHOT_DIR="${SNAPSHOT_DIR:-e2e/snapshots}"
DEFAULT_TIMEOUT=10
POLL_INTERVAL=0.5
TMUX_WIDTH=120
TMUX_HEIGHT=30

# Tracks temp dirs for cleanup.
_HARNESS_TMPDIRS=()

# --- Core Functions ---

# tmux_start <session> <cmd...>
#   Creates a detached tmux session running the given command.
tmux_start() {
    local session="$1"; shift
    tmux new-session -d -s "$session" -x "$TMUX_WIDTH" -y "$TMUX_HEIGHT" "$@"
}

# tmux_send <session> <text>
#   Sends literal text to the session (no trailing Enter).
tmux_send() {
    tmux send-keys -t "$1" "$2"
}

# tmux_enter <session>
#   Sends Enter key.
tmux_enter() {
    tmux send-keys -t "$1" Enter
}

# tmux_capture <session>
#   Prints current pane content to stdout.
tmux_capture() {
    tmux capture-pane -t "$1" -p
}

# tmux_wait_for <session> <pattern> [timeout_secs]
#   Polls capture-pane until grep -qE matches pattern or timeout.
#   Returns 0 on match, 1 on timeout.
tmux_wait_for() {
    local session="$1" pattern="$2" timeout="${3:-$DEFAULT_TIMEOUT}"
    local deadline=$((SECONDS + timeout))
    while [ "$SECONDS" -lt "$deadline" ]; do
        if tmux_capture "$session" 2>/dev/null | grep -qE "$pattern"; then
            return 0
        fi
        sleep "$POLL_INTERVAL"
    done
    return 1
}

# tmux_wait_ready <session> [timeout_secs]
#   Waits for the TUI to show the input prompt.
tmux_wait_ready() {
    tmux_wait_for "$1" "Type a message" "${2:-15}"
}

# tmux_assert <session> <pattern> <description>
#   Asserts pattern exists in current screen content.
#   Increments global PASS/FAIL counters. Saves snapshot on failure.
tmux_assert() {
    local session="$1" pattern="$2" desc="$3"
    if tmux_capture "$session" | grep -qE "$pattern"; then
        echo "  PASS: $desc"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $desc -- pattern '$pattern' not found"
        tmux_save_snapshot "$session" "$desc"
        FAIL=$((FAIL + 1))
    fi
}

# tmux_session_alive <session>
#   Returns 0 if the tmux session still exists.
tmux_session_alive() {
    tmux has-session -t "$1" 2>/dev/null
}

# tmux_kill <session>
#   Kills the session, silently ignoring if already gone.
tmux_kill() {
    tmux kill-session -t "$1" 2>/dev/null || true
}

# tmux_save_snapshot <session> <label>
#   Saves pane content to snapshots dir and prints to stderr.
tmux_save_snapshot() {
    local session="$1" label="$2"
    mkdir -p "$SNAPSHOT_DIR"
    local file="$SNAPSHOT_DIR/$(echo "$label" | tr ' /' '_-').txt"
    tmux_capture "$session" > "$file"
    echo "  --- snapshot saved: $file ---" >&2
    cat "$file" >&2
    echo "  --- end ---" >&2
}

# harness_track_tmpdir <path>
#   Registers a temp directory for cleanup.
harness_track_tmpdir() {
    _HARNESS_TMPDIRS+=("$1")
}

# cleanup_all
#   Kills all gogent-e2e-* sessions and removes tracked temp dirs.
cleanup_all() {
    local sessions
    sessions=$(tmux list-sessions -F '#{session_name}' 2>/dev/null | grep '^gogent-e2e-' || true)
    for s in $sessions; do
        tmux kill-session -t "$s" 2>/dev/null || true
    done
    for d in "${_HARNESS_TMPDIRS[@]}"; do
        [ -d "$d" ] && rm -rf "$d"
    done
}
