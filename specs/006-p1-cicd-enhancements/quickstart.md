# Quickstart: P1 CI/CD Enhancements

**Feature**: 006-p1-cicd-enhancements  
**Date**: 2026-02-07

## 1. Sub-process Execution Mode (`exec`)

The new `exec` command replaces the three-step `eval` / execute / `verify && clean` pattern with a single command.

### Before (eval pattern — still supported)

```bash
# Step 1: Set up interception
eval "$(cli-replay run scenario.yaml)"

# Step 2: Run your script
./deploy.sh

# Step 3: Verify and clean up (easy to forget!)
cli-replay verify scenario.yaml
cli-replay clean scenario.yaml
```

### After (exec mode — recommended for CI)

```bash
# All-in-one: setup → run → verify → cleanup
cli-replay exec scenario.yaml -- ./deploy.sh
```

### What exec does

1. **Validates** the scenario file (fails fast if invalid)
2. **Creates** a temp intercept directory with symlinks to cli-replay
3. **Spawns** your command with `PATH` prepended and session env vars set
4. **Waits** for your command to finish
5. **Verifies** all scenario steps were consumed
6. **Cleans up** the intercept directory and state file — always, regardless of outcome

### Exit codes

```bash
# Success: script passed, all steps matched
cli-replay exec scenario.yaml -- ./test.sh
echo $?  # 0

# Script failed (non-zero exit from your command)
cli-replay exec scenario.yaml -- ./broken-script.sh
echo $?  # whatever the script returned

# Script passed but steps weren't all consumed
cli-replay exec scenario.yaml -- ./incomplete-script.sh
echo $?  # 1 (verification failure)

# Script interrupted (Ctrl+C)
cli-replay exec scenario.yaml -- ./long-script.sh
# ^C
echo $?  # 130 (128 + SIGINT=2)
```

### With flags

```bash
# Restrict which commands can be intercepted
cli-replay exec --allowed-commands=kubectl,helm scenario.yaml -- ./deploy.sh

# Set max delay cap
cli-replay exec --max-delay=30s scenario.yaml -- ./test.sh
```

### CI pipeline example (GitHub Actions)

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run integration test
        run: cli-replay exec tests/scenario.yaml -- ./tests/run-integration.sh
```

---

## 2. Signal-Trap Auto-Cleanup (eval pattern)

When using the `eval` pattern, the `run` command now emits a cleanup trap that automatically cleans up on exit or signal.

### How it works

```bash
# The eval output now includes a trap:
eval "$(cli-replay run scenario.yaml)"
# ^ This sets PATH, CLI_REPLAY_SESSION, CLI_REPLAY_SCENARIO,
#   AND sets a trap on EXIT/INT/TERM to auto-clean

# Run your commands...
kubectl get pods
git status

# If you Ctrl+C here, the trap fires and cleans up automatically!

# If you reach the end normally, you can still verify explicitly:
cli-replay verify scenario.yaml

# The EXIT trap will clean up when the shell exits, or you can clean manually:
cli-replay clean scenario.yaml  # idempotent — safe to call even if trap already cleaned
```

### What the trap does

The emitted trap calls `cli-replay clean` with the session-specific scenario path when the shell exits or receives SIGINT/SIGTERM. It uses a guard variable to prevent double-cleanup.

### Shell compatibility

The trap works in **bash**, **zsh**, and **POSIX sh** (dash). PowerShell and cmd.exe do not receive a trap (out of scope).

---

## 3. Session Isolation (parallel CI)

Session isolation already works via `CLI_REPLAY_SESSION`. Both `verify` and `clean` respect the session environment variable.

### Parallel execution example

```bash
# Terminal/CI job A
eval "$(cli-replay run scenario-a.yaml)"
./test-a.sh
cli-replay verify scenario-a.yaml  # checks session A's state only
cli-replay clean scenario-a.yaml   # cleans session A's state only

# Terminal/CI job B (concurrent, same machine)
eval "$(cli-replay run scenario-b.yaml)"
./test-b.sh
cli-replay verify scenario-b.yaml  # checks session B's state only
cli-replay clean scenario-b.yaml   # cleans session B's state only
```

Each `cli-replay run` generates a unique session ID. State files are stored with session-specific hashes, so parallel invocations never collide.

### With exec mode (recommended)

```bash
# Even simpler — exec handles everything including isolation
cli-replay exec scenario-a.yaml -- ./test-a.sh &
cli-replay exec scenario-b.yaml -- ./test-b.sh &
wait  # both complete independently
```

---

## Migration Guide

| Current Pattern | New Recommended Pattern | Notes |
|---|---|---|
| `eval` + manual `verify` + `clean` | `cli-replay exec ... -- ...` | Single command, auto-cleanup |
| Manual cleanup after Ctrl+C | Automatic via trap (eval) or exec | No more leaked state |
| Serial test execution | Parallel `exec` invocations | Session isolation is automatic |

### No breaking changes

- All existing workflows continue to work unchanged
- The `run` command adds trap output (additive, not breaking)
- `clean` remains idempotent
- `verify` is unchanged
