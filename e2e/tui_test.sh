#!/bin/bash
set -e

BINARY="$(cd "$(dirname "$0")/.." && pwd)/bin/gogent"
API_KEY="${ANTHROPIC_API_KEY:-smoke-test}"
PASS=0
FAIL=0

assert_screen() {
    local pattern="$1"
    local test_name="$2"
    if tui-use find "$pattern" >/dev/null 2>&1; then
        echo "  PASS: $test_name"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $test_name — pattern '$pattern' not found on screen"
        echo "  --- screen ---"
        tui-use snapshot 2>/dev/null || true
        echo "  --- end ---"
        FAIL=$((FAIL + 1))
    fi
}

cleanup() {
    tui-use kill 2>/dev/null || true
}
trap cleanup EXIT

echo "Building gogent..."
(cd "$(dirname "$0")/.." && make build)
echo ""

# --------------------------------------------------
# Test 1: /help
# --------------------------------------------------
echo "=== Test: /help ==="
tui-use start env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
tui-use wait 5000 2>/dev/null
tui-use type "/help"
tui-use wait 500 2>/dev/null
tui-use press enter
tui-use wait 3000 2>/dev/null
assert_screen "compact" "/help lists /compact"
assert_screen "exit" "/help lists /exit"
assert_screen "cost" "/help lists /cost"
tui-use kill

# --------------------------------------------------
# Test 2: /cost
# --------------------------------------------------
echo ""
echo "=== Test: /cost ==="
tui-use start env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
tui-use wait 5000 2>/dev/null
tui-use type "/cost"
tui-use wait 500 2>/dev/null
tui-use press enter
tui-use wait 3000 2>/dev/null
assert_screen "0.0000" "/cost shows $0.0000"
tui-use kill

# --------------------------------------------------
# Test 3: /exit quits the program
# --------------------------------------------------
echo ""
echo "=== Test: /exit ==="
tui-use start env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
tui-use wait 5000 2>/dev/null
tui-use type "/exit"
tui-use wait 500
tui-use press enter
tui-use wait 3000 2>/dev/null || true
if tui-use snapshot 2>/dev/null | grep -q "exited"; then
    echo "  PASS: /exit terminated the program"
    PASS=$((PASS + 1))
else
    echo "  FAIL: /exit did not terminate the program"
    tui-use snapshot 2>/dev/null || true
    FAIL=$((FAIL + 1))
fi
tui-use kill 2>/dev/null || true

# --------------------------------------------------
# Test 4: /unknown shows error
# --------------------------------------------------
echo ""
echo "=== Test: /unknown ==="
tui-use start env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
tui-use wait 5000 2>/dev/null
tui-use type "/nonexistent"
tui-use wait 500 2>/dev/null
tui-use press enter
tui-use wait 3000 2>/dev/null
assert_screen "Error\|error\|unknown" "/unknown shows error"
tui-use kill

# --------------------------------------------------
# Test 5: Regular message (only with real API key)
# --------------------------------------------------
if [ "$API_KEY" != "smoke-test" ]; then
    echo ""
    echo "=== Test: regular message ==="
    tui-use start env ANTHROPIC_API_KEY="$API_KEY" "$BINARY"
    tui-use wait 5000 2>/dev/null
    tui-use type "say hello in one word"
    tui-use wait 500
    tui-use press enter
    tui-use wait --text "Assistant" 2>/dev/null || tui-use wait 15000
    tui-use wait 5000
    echo "  Screen after message:"
    tui-use snapshot
    assert_screen "turns: 1" "regular message incremented turn counter"
    tui-use kill
fi

# --------------------------------------------------
# Summary
# --------------------------------------------------
echo ""
echo "========================"
echo "Results: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
echo "All e2e tests passed"
