#!/usr/bin/env bash
# recording-demo.sh — Demonstrates cli-replay record subcommand
#
# Usage:
#   ./examples/recording-demo.sh
#
# Prerequisites:
#   - cli-replay binary built (go build -o bin/cli-replay .)
#   - Writable current directory for output YAML files

set -euo pipefail

BINARY="${CLI_REPLAY_BIN:-./bin/cli-replay}"
OUTDIR="${TMPDIR:-/tmp}/cli-replay-demo"

cleanup() {
    rm -rf "$OUTDIR"
}
trap cleanup EXIT

mkdir -p "$OUTDIR"

echo "=== cli-replay record demo ==="
echo ""

# ─── Demo 1: Record a single command ──────────────────────────────────
echo "--- Demo 1: Record a single command ---"
"$BINARY" record \
    --output "$OUTDIR/single.yaml" \
    -- echo "hello world"

echo ""
echo "Generated YAML:"
cat "$OUTDIR/single.yaml"
echo ""

# ─── Demo 2: Record with custom metadata ─────────────────────────────
echo "--- Demo 2: Record with custom metadata ---"
"$BINARY" record \
    --output "$OUTDIR/named.yaml" \
    --name "greeting-test" \
    --description "A simple greeting scenario" \
    -- echo "hello from cli-replay"

echo ""
echo "Generated YAML:"
cat "$OUTDIR/named.yaml"
echo ""

# ─── Demo 3: Record a multi-step workflow ─────────────────────────────
echo "--- Demo 3: Record a multi-step workflow ---"
"$BINARY" record \
    --output "$OUTDIR/multi.yaml" \
    --name "multi-step-demo" \
    --description "Three sequential steps" \
    -- bash -c "echo step1 && echo step2 && echo step3"

echo ""
echo "Generated YAML:"
cat "$OUTDIR/multi.yaml"
echo ""

# ─── Demo 4: Record a command with non-zero exit ─────────────────────
echo "--- Demo 4: Record a command with non-zero exit code ---"
"$BINARY" record \
    --output "$OUTDIR/error.yaml" \
    --name "error-scenario" \
    -- bash -c "echo 'something went wrong' >&2; exit 1" || true

echo ""
echo "Generated YAML:"
cat "$OUTDIR/error.yaml"
echo ""

# ─── Demo 5: Validate the generated scenario ─────────────────────────
echo "--- Demo 5: Validate a generated scenario ---"
# Note: 'run' subcommand lives in cmd/cli-replay binary, not the Cobra root
RUN_BINARY="${CLI_REPLAY_RUN_BIN:-./bin/cli-replay-run}"
if [ -x "$RUN_BINARY" ]; then
    echo "Running: $RUN_BINARY run $OUTDIR/single.yaml"
    "$RUN_BINARY" run "$OUTDIR/single.yaml" && echo "Validation: PASS (exit code 0)" || echo "Validation: FAIL (exit code $?)"
else
    echo "Skipping validation (build cmd/cli-replay for 'run' support):"
    echo "  go build -o bin/cli-replay-run ./cmd/cli-replay"
fi
echo ""

echo "=== All demos complete ==="
echo "Output files in: $OUTDIR"
ls -la "$OUTDIR"
