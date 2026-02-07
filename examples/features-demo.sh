#!/usr/bin/env bash
# features-demo.sh — Demonstrates cli-replay's core features
#
# Walks through four scenarios:
#   1. Mismatch diagnostics — per-element diff with color output
#   2. Call count bounds — polling/retry steps with min/max
#   3. stdin matching — piped input validation
#   4. Security allowlist — restricting interceptable commands
#
# Usage:
#   make build && ./examples/features-demo.sh
#
# Prerequisites:
#   - cli-replay binary built at ./bin/cli-replay

set -uo pipefail

BINARY="${CLI_REPLAY_BIN:-./bin/cli-replay}"
DEMO_DIR="${PWD}/.demo-tmp"
BOLD=$'\033[1m'
DIM=$'\033[2m'
CYAN=$'\033[36m'
GREEN=$'\033[32m'
YELLOW=$'\033[33m'
RED=$'\033[31m'
RESET=$'\033[0m'

cleanup() {
    rm -rf "$DEMO_DIR"
}
trap cleanup EXIT

mkdir -p "$DEMO_DIR"

banner() {
    echo ""
    echo "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo "${BOLD}${CYAN}  $1${RESET}"
    echo "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
    echo ""
}

step() {
    echo "  ${GREEN}▸${RESET} ${BOLD}$1${RESET}"
}

run_cmd() {
    echo "    ${DIM}\$ $*${RESET}"
    "$@" 2>&1 | sed 's/^/    /'
    echo ""
}

pause() {
    echo "  ${DIM}(press Enter to continue)${RESET}"
    read -r
}

# ═══════════════════════════════════════════════════════════════════════
banner "cli-replay Features Demo"
echo "  This demo walks through the four features added in the"
echo "  P0 Critical Enhancements release:"
echo ""
echo "    1. ${BOLD}Mismatch Diagnostics${RESET} — per-element colored diff"
echo "    2. ${BOLD}Call Count Bounds${RESET}    — retry/polling steps"
echo "    3. ${BOLD}stdin Matching${RESET}       — piped input validation"
echo "    4. ${BOLD}Security Allowlist${RESET}   — command restrictions"
echo ""
pause

# ═══════════════════════════════════════════════════════════════════════
# DEMO 1: Mismatch Diagnostics
# ═══════════════════════════════════════════════════════════════════════
banner "Demo 1: Mismatch Diagnostics"

cat > "$DEMO_DIR/mismatch-demo.yaml" <<'YAML'
meta:
  name: "mismatch-demo"
  description: "Shows rich mismatch error output"
steps:
  - match:
      argv: ["kubectl", "get", "pods", "-n", "production"]
    respond:
      exit: 0
      stdout: |
        NAME       READY   STATUS
        web-0      1/1     Running
YAML

step "Scenario expects: kubectl get pods -n production"
echo ""

step "Setting up intercepts..."
SETUP_OUTPUT=$("$BINARY" run "$DEMO_DIR/mismatch-demo.yaml" 2>&1)
eval "$("$BINARY" run "$DEMO_DIR/mismatch-demo.yaml")"
echo ""

step "Calling with WRONG namespace to trigger mismatch diagnostic:"
echo "    ${DIM}\$ kubectl get pods -n staging${RESET}"
echo ""

# This will fail with a detailed mismatch error — that's the point!
export CLI_REPLAY_COLOR=1
kubectl get pods -n staging 2>&1 | sed 's/^/    /'
echo ""

step "Notice the per-element diff showing exactly which argument didn't match."
step "Regex and wildcard patterns show the expected pattern in the error."
echo ""

# Clean up state for this demo
"$BINARY" clean "$DEMO_DIR/mismatch-demo.yaml" 2>/dev/null || true

pause

# ═══════════════════════════════════════════════════════════════════════
# DEMO 2: Call Count Bounds
# ═══════════════════════════════════════════════════════════════════════
banner "Demo 2: Call Count Bounds"

cat > "$DEMO_DIR/polling-demo.yaml" <<'YAML'
meta:
  name: "k8s-deploy-with-polling"
  description: "Deployment with polling rollout status"
steps:
  # Step 1: Apply deployment (exactly once)
  - match:
      argv: ["kubectl", "apply", "-f", "deploy.yaml"]
    respond:
      exit: 0
      stdout: "deployment.apps/nginx configured\n"

  # Step 2: Poll rollout status (1-5 times)
  - match:
      argv: ["kubectl", "rollout", "status", "deployment/nginx"]
    calls:
      min: 1
      max: 5
    respond:
      exit: 0
      stdout: "Waiting for deployment rollout to finish...\n"

  # Step 3: Final check (exactly once)
  - match:
      argv: ["kubectl", "get", "deployment", "nginx"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   UP-TO-DATE   AVAILABLE
        nginx   3/3     3            3
YAML

step "Scenario: deploy → poll rollout (1-5 times) → final check"
echo ""

cat "$DEMO_DIR/polling-demo.yaml" | grep -A3 "calls:" | sed 's/^/    /'
echo ""

step "Setting up intercepts..."
eval "$("$BINARY" run "$DEMO_DIR/polling-demo.yaml")"
echo ""

step "Step 1: Apply deployment (exactly once)"
run_cmd kubectl apply -f deploy.yaml

step "Step 2: Poll rollout status — calling 3 times (within budget of 1-5)"
run_cmd kubectl rollout status deployment/nginx
run_cmd kubectl rollout status deployment/nginx
run_cmd kubectl rollout status deployment/nginx

step "Step 3: Final check — auto-advances past polling step since min (1) was met"
run_cmd kubectl get deployment nginx

step "Verify with per-step call counts:"
run_cmd "$BINARY" verify "$DEMO_DIR/polling-demo.yaml"

"$BINARY" clean "$DEMO_DIR/polling-demo.yaml" 2>/dev/null || true

pause

# ═══════════════════════════════════════════════════════════════════════
# DEMO 3: stdin Matching
# ═══════════════════════════════════════════════════════════════════════
banner "Demo 3: stdin Matching"

cat > "$DEMO_DIR/stdin-demo.yaml" <<'YAML'
meta:
  name: "stdin-piping"
  description: "Validates piped input content"
steps:
  - match:
      argv: ["kubectl", "apply", "-f", "-"]
      stdin: |
        apiVersion: v1
        kind: Pod
        metadata:
          name: demo-pod
    respond:
      exit: 0
      stdout: "pod/demo-pod created\n"

  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME       READY   STATUS    RESTARTS   AGE
        demo-pod   1/1     Running   0          5s
YAML

step "Scenario expects piped YAML to kubectl apply -f -"
echo ""

step "Setting up intercepts..."
eval "$("$BINARY" run "$DEMO_DIR/stdin-demo.yaml")"
echo ""

step "Piping matching stdin content to kubectl apply:"
echo "    ${DIM}\$ echo '...' | kubectl apply -f -${RESET}"
printf 'apiVersion: v1\nkind: Pod\nmetadata:\n  name: demo-pod\n' | kubectl apply -f - 2>&1 | sed 's/^/    /'
echo ""

step "Follow-up step (no stdin required):"
run_cmd kubectl get pods

step "Verify:"
run_cmd "$BINARY" verify "$DEMO_DIR/stdin-demo.yaml"

"$BINARY" clean "$DEMO_DIR/stdin-demo.yaml" 2>/dev/null || true

pause

# ═══════════════════════════════════════════════════════════════════════
# DEMO 4: Security Allowlist
# ═══════════════════════════════════════════════════════════════════════
banner "Demo 4: Security Allowlist"

cat > "$DEMO_DIR/allowlist-demo.yaml" <<'YAML'
meta:
  name: "security-restricted"
  description: "Only kubectl and az are allowed"
  security:
    allowed_commands:
      - kubectl
      - az
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "NAME    READY   STATUS\nweb-0   1/1     Running\n"

  - match:
      argv: ["az", "account", "show"]
    respond:
      exit: 0
      stdout: '{"name": "my-sub"}'
YAML

step "Scenario YAML restricts interception to: kubectl, az"
echo ""
grep -A3 "security:" "$DEMO_DIR/allowlist-demo.yaml" | sed 's/^/    /'
echo ""

step "This scenario is valid — all steps use allowed commands:"
run_cmd "$BINARY" run "$DEMO_DIR/allowlist-demo.yaml"

"$BINARY" clean "$DEMO_DIR/allowlist-demo.yaml" 2>/dev/null || true

echo ""

# Now show a violation
cat > "$DEMO_DIR/allowlist-violation.yaml" <<'YAML'
meta:
  name: "security-violation"
  description: "Contains a disallowed command"
  security:
    allowed_commands:
      - kubectl
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "ok"

  - match:
      argv: ["docker", "build", "-t", "myapp", "."]
    respond:
      exit: 0
      stdout: "built"
YAML

step "Now trying a scenario with 'docker' (not in allowlist):"
echo ""
grep -A2 "security:" "$DEMO_DIR/allowlist-violation.yaml" | sed 's/^/    /'
echo ""

step "cli-replay run rejects it BEFORE creating any intercepts:"
echo "    ${DIM}\$ cli-replay run allowlist-violation.yaml${RESET}"
"$BINARY" run "$DEMO_DIR/allowlist-violation.yaml" 2>&1 | sed 's/^/    /'
echo ""

step "Also works with --allowed-commands CLI flag (intersection with YAML):"
echo "    ${DIM}\$ cli-replay run --allowed-commands kubectl allowlist-demo.yaml${RESET}"
run_cmd "$BINARY" run --allowed-commands kubectl "$DEMO_DIR/allowlist-demo.yaml"

"$BINARY" clean "$DEMO_DIR/allowlist-demo.yaml" 2>/dev/null || true

# ═══════════════════════════════════════════════════════════════════════
banner "Demo Complete!"
echo "  Features demonstrated:"
echo ""
echo "    ${GREEN}✓${RESET} Mismatch diagnostics — per-element diff with color"
echo "    ${GREEN}✓${RESET} Call count bounds    — polling step called 3x within 1-5 budget"
echo "    ${GREEN}✓${RESET} stdin matching       — piped YAML validated during replay"
echo "    ${GREEN}✓${RESET} Security allowlist   — disallowed command rejected pre-intercept"
echo ""
echo "  Run individual examples:"
echo "    ${DIM}examples/kubectl-simple.yaml${RESET}  — basic replay"
echo "    ${DIM}examples/azure-tsg.yaml${RESET}       — wildcard/regex matching"
echo "    ${DIM}examples/recording-demo.sh${RESET}    — recording workflow"
echo ""
