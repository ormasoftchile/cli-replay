# CLI Contract: P1 CI/CD Enhancements

**Feature**: 006-p1-cicd-enhancements  
**Date**: 2026-02-07

## New Command: `cli-replay exec`

### Synopsis

```
cli-replay exec [flags] <scenario.yaml> -- <command> [args...]
```

### Description

Run a command under cli-replay interception with automatic lifecycle management. Sets up the intercept directory, spawns the command as a child process with modified PATH, waits for completion, verifies scenario completion, and cleans up — all in a single invocation.

### Arguments

| Argument | Required | Description |
|---|---|---|
| `<scenario.yaml>` | Yes | Path to the scenario YAML file |
| `--` | Yes | Separator between exec flags and the child command |
| `<command> [args...]` | Yes | The command to run under interception |

### Flags

| Flag | Default | Description |
|---|---|---|
| `--allowed-commands` | `""` | Comma-separated list of commands allowed to be intercepted (same as `run`) |
| `--max-delay` | `5m` | Maximum allowed delay duration (same as `run`) |

### Environment Variables Set on Child Process

| Variable | Value | Description |
|---|---|---|
| `PATH` | `<interceptDir>:$PATH` | Intercept directory prepended to inherit PATH |
| `CLI_REPLAY_SCENARIO` | `<absPath>` | Absolute path to scenario file |
| `CLI_REPLAY_SESSION` | `<sessionID>` | Unique session ID for state file isolation |

### Exit Codes

| Code | Meaning |
|---|---|
| 0 | Child exited 0 AND all scenario steps satisfied |
| 1 | Scenario verification failed (steps not consumed), even if child exited 0 |
| N (child's code) | Child exited non-zero (takes precedence; verification still reported to stderr) |
| 126 | Child command found but not executable |
| 127 | Child command not found |
| 128+N | Child killed by signal N (e.g., 130 = SIGINT, 143 = SIGTERM) |

### Behavioral Contract

1. **Pre-spawn validation**: Scenario file is loaded and validated before any child process is spawned. Invalid scenarios produce an error and exit code 1 without creating any state or intercept directory.
2. **Isolation**: Each `exec` invocation generates a unique session ID. Concurrent invocations never share state files or intercept directories.
3. **Signal forwarding**: SIGINT and SIGTERM received by `exec` are forwarded to the child process. The child is allowed to handle them. After the child exits, cleanup proceeds.
4. **Auto-verify**: After the child exits, `exec` checks `state.AllStepsMetMin(steps)`. If verification fails, a diagnostic message is printed to stderr (same format as `cli-replay verify`).
5. **Auto-cleanup**: Intercept directory and state file are removed after the child exits, regardless of exit code, verification result, or signal. Cleanup is idempotent.
6. **No timeout**: `exec` does not impose any timeout on the child process.

### Examples

```bash
# Basic usage — run a test script under interception
cli-replay exec scenario.yaml -- ./test-script.sh

# With allowed commands restriction
cli-replay exec --allowed-commands=kubectl,helm scenario.yaml -- ./deploy.sh

# Running a one-liner
cli-replay exec scenario.yaml -- bash -c 'kubectl get pods && kubectl get svc'

# CI pipeline usage
cli-replay exec scenario.yaml -- make integration-test
echo "Exit code: $?"
```

---

## Modified Command: `cli-replay run`

### Changes

#### New Output: Cleanup Trap (bash/zsh/sh only)

The `eval "$(cli-replay run ...)"` output now includes a cleanup trap after the export statements.

**Before** (existing output):
```sh
export CLI_REPLAY_SESSION='abc123'
export CLI_REPLAY_SCENARIO='/path/to/scenario.yaml'
export PATH='/tmp/cli-replay-intercept-xyz':"$PATH"
```

**After** (new output):
```sh
export CLI_REPLAY_SESSION='abc123'
export CLI_REPLAY_SCENARIO='/path/to/scenario.yaml'
export PATH='/tmp/cli-replay-intercept-xyz':"$PATH"
_cli_replay_clean() { if [ -n "${_cli_replay_cleaned:-}" ]; then return; fi; _cli_replay_cleaned=1; command cli-replay clean "$CLI_REPLAY_SCENARIO" 2>/dev/null; }
trap '_cli_replay_clean' EXIT INT TERM
```

#### PowerShell Output (no trap change)

PowerShell trap equivalent is out of scope for this feature. The existing PowerShell output remains unchanged.

#### cmd.exe Output (no trap change)

No trap equivalent for cmd.exe. The existing cmd output remains unchanged.

### Backward Compatibility

- Existing `eval "$(cli-replay run ...)"` usage continues to work. The trap is additive.
- The trap uses a namespaced function (`_cli_replay_clean`) and guard variable (`_cli_replay_cleaned`) to minimize collision risk with user code.
- If the user manually calls `cli-replay clean` before the trap fires, the trap's cleanup is a no-op (idempotent).
- The trap **overwrites** any existing EXIT/INT/TERM traps. This is documented behavior.

---

## Modified Command: `cli-replay clean`

### Changes

#### Idempotency (FR-018)

`clean` must succeed silently when called on an already-cleaned session. Current behavior is already mostly idempotent:

| Scenario | Current Behavior | Required Behavior |
|---|---|---|
| State file exists | ✅ Deletes state + intercept dir | No change |
| State file missing | ✅ `DeleteState` returns nil | No change |
| Intercept dir missing | ✅ Logs warning, continues | No change |
| Both missing | ✅ Succeeds silently | No change |

**Finding**: Current implementation already satisfies FR-018. No code changes needed — only test coverage.

#### Session Awareness (FR-012)

`clean` already calls `runner.StateFilePath(absPath)` which reads `CLI_REPLAY_SESSION` from the environment. When the trap fires, `CLI_REPLAY_SESSION` is set in the shell, so `clean` finds the correct session-specific state file. **No code changes needed.**

---

## Modified Command: `cli-replay verify`

### Changes

#### Session Awareness (FR-011)

`verify` already calls `runner.StateFilePath(absPath)` which reads `CLI_REPLAY_SESSION` from the environment. **No code changes needed** — only test coverage for session-aware verification.

---

## Unchanged Components

| Component | Reason |
|---|---|
| `runner.ExecuteReplay()` | Already session-aware via `StateFilePath()` |
| `runner.State` struct | No schema changes |
| `runner.StateFilePath` / `StateFilePathWithSession` | Already correct |
| `scenario.Scenario` / `scenario.Step` | No model changes |
| `matcher.ArgvMatch` | No changes |
| `main.go` (intercept mode) | No changes |
