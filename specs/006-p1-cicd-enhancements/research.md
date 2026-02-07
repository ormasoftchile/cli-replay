# Research: P1 CI/CD Enhancements

**Feature**: 006-p1-cicd-enhancements  
**Date**: 2026-02-07

## R1: Go Child Process Lifecycle Management (US1 — exec mode)

### Decision: Use `os/exec` with inherited environment + PATH prepend

### Rationale
Go's `os/exec` provides a clean API for spawning child processes with modified environments. Setting `cmd.Env` to a modified copy of `os.Environ()` gives full control over the child's environment without affecting the parent. This is exactly what exec mode needs: prepend the intercept directory to PATH, set `CLI_REPLAY_SESSION` and `CLI_REPLAY_SCENARIO`, then spawn the user's command.

### Key Patterns

**Environment setup:**
```go
cmd := exec.Command(binary, args...)
cmd.Env = prependPath(os.Environ(), interceptDir)
// Also add CLI_REPLAY_SESSION=<sessionID> and CLI_REPLAY_SCENARIO=<absPath>
cmd.Stdin = os.Stdin
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
```

**Critical gotcha**: `cmd.Env = nil` inherits everything; `cmd.Env = []string{}` gives an empty environment. Always start from `os.Environ()` and modify.

**Exit code extraction:**
```go
func exitCodeFromError(err error) int {
    if err == nil { return 0 }
    var exitErr *exec.ExitError
    if !errors.As(err, &exitErr) { return 1 }
    if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
        if ws.Signaled() { return 128 + int(ws.Signal()) }
        return ws.ExitStatus()
    }
    if code := exitErr.ExitCode(); code >= 0 { return code }
    return 1
}
```

**Critical gotcha**: `ExitCode()` returns -1 for signal-killed processes. Must use `syscall.WaitStatus.Signaled()` + `Signal()` for the conventional `128 + signum` exit code.

### Alternatives Considered

- **`syscall.Exec` (replace parent process)**: Would make cleanup impossible since the parent is replaced. Rejected.
- **Process groups (`Setpgid: true`)**: Kills entire subtree but prevents child from receiving terminal SIGINT naturally. Overkill for a CLI wrapper; adds complexity without benefit since cli-replay intercepts are single commands. Rejected.

---

## R2: Signal Forwarding to Child Process (US1 — exec mode)

### Decision: Direct signal forwarding via `cmd.Process.Signal()` with `signal.Notify`

### Rationale
For a CLI wrapper tool, direct signaling is simpler and more predictable than process groups. The child's own signal handling works normally. The race condition where the child exits before the signal is forwarded is safe — `Process.Signal()` returns an ignorable error.

### Key Pattern
```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
go func() {
    for sig := range sigCh {
        if cmd.Process != nil {
            _ = cmd.Process.Signal(sig) // safe if already dead
        }
    }
}()
err := cmd.Wait()
signal.Stop(sigCh)
close(sigCh)
```

### Platform Notes
- macOS and Linux behave identically for SIGINT/SIGTERM forwarding
- Signal numbers differ per platform (e.g., SIGUSR1 = 30 on macOS, 10 on Linux) but Go constants abstract this
- Exit codes with `128 + signum` match native shell behavior per platform

### Alternatives Considered
- **Process groups (`syscall.Kill(-pid, sig)`)**: Would kill grandchildren but breaks terminal SIGINT delivery to child. Rejected for simplicity.

---

## R3: Cleanup Guarantees (US1 — exec mode)

### Decision: `defer` + signal handler with idempotent guard

### Rationale
Using both `defer cleanup()` and a signal handler ensures cleanup runs in all survivable cases. An idempotent guard variable prevents double-cleanup. The `os.Exit()` call must be in `main()` only — never inside the function with the `defer`, since `os.Exit()` skips deferred functions.

### Key Pattern
```go
func execRun() int {
    cleaned := false
    cleanup := func() {
        if cleaned { return }
        cleaned = true
        os.RemoveAll(interceptDir)
        runner.DeleteState(stateFile)
    }
    defer cleanup()
    // ... signal handler also calls cleanup() before forwarding ...
    return exitCodeFromError(err)
}
```

### Unrecoverable Cases
| Scenario | Cleanup runs? |
|---|---|
| Normal exit | ✅ defer |
| SIGINT/SIGTERM | ✅ signal handler |
| SIGKILL (kill -9) | ❌ — process dies immediately |
| OOM killer | ❌ |

SIGKILL resilience is out of scope. The temp directory (`os.TempDir()`) is cleaned periodically by the OS, and stale state files are harmless (they just cause a "scenario already complete" message that `clean` resolves).

---

## R4: POSIX Shell Trap Syntax (US3 — signal-trap auto-cleanup)

### Decision: Emit `trap '_cli_replay_clean' EXIT INT TERM` with guard variable

### Rationale
Using all three signals (EXIT, INT, TERM) ensures maximum portability across bash, zsh, and POSIX sh. The EXIT trap fires on normal exit in all shells. The INT/TERM traps ensure cleanup on signals in shells where EXIT may not fire on signal-induced exit (some POSIX sh implementations). A guard variable `_cli_replay_cleaned` prevents double-cleanup when both INT and EXIT fire (which happens in bash on Ctrl-C).

### Key Pattern (emitted by `emitShellSetup`)
```sh
_cli_replay_clean() {
    if [ -n "${_cli_replay_cleaned:-}" ]; then return; fi
    _cli_replay_cleaned=1
    command cli-replay clean "$CLI_REPLAY_SCENARIO" 2>/dev/null
}
trap '_cli_replay_clean' EXIT INT TERM
```

### Design Decisions
| Decision | Choice | Rationale |
|---|---|---|
| Trap signals | `EXIT INT TERM` | Portable across sh/bash/zsh |
| Guard variable | `_cli_replay_cleaned` | Prevents double-fire in bash |
| Path quoting | Use `$CLI_REPLAY_SCENARIO` | Already exported; avoids quoting hell |
| `command` prefix | Yes | Bypasses intercept shims in PATH |
| Trap chaining | No (overwrite) | Simpler; documented behavior |
| Single-quoted body | Yes | Variables resolve at fire-time, not set-time |

### Shell Compatibility
| Feature | bash | zsh | POSIX sh (dash) |
|---|---|---|---|
| `trap 'cmd' EXIT INT TERM` | ✅ | ✅ | ✅ |
| EXIT fires on signal exit | ✅ always | ✅ if handler calls exit | ⚠️ varies |
| Guard prevents double-fire | ✅ handles it | ✅ handles it | ✅ handles it |

### Eval Context
`eval "$(cli-replay run ...)"` executes in the **current shell** context (not a subshell). The trap persists in the current shell and fires when that shell exits. This is POSIX-specified behavior.

### Alternatives Considered
- **EXIT trap only**: Simpler but unreliable in some POSIX sh implementations on signal-induced exit. Rejected for portability.
- **Trap chaining (preserve existing traps)**: Complex, fragile across shells (`trap -p` output format varies). Rejected — overwrite is acceptable; documented.
- **PowerShell equivalent (`Register-EngineEvent`)**: Out of scope per spec (PowerShell trap deferred).

---

## R5: Session Isolation Gap Analysis (US2 — session isolation verification)

### Decision: Existing code is functionally correct; gap is integration test coverage only

### Rationale
Code review reveals that the session isolation mechanism is already implemented correctly:

1. **`runner.StateFilePath(path)`** reads `CLI_REPLAY_SESSION` from the environment and delegates to `StateFilePathWithSession(path, session)`.
2. **`cmd/verify.go`** calls `runner.StateFilePath(absPath)` — this picks up `CLI_REPLAY_SESSION` from the environment, which is set by `emitShellSetup`.
3. **`cmd/clean.go`** also calls `runner.StateFilePath(absPath)` — same mechanism.
4. **`emitShellSetup`** already exports `CLI_REPLAY_SESSION` in all shell variants (bash, PowerShell, cmd).
5. **`runner.ExecuteReplay()`** calls `StateFilePath(absPath)` — picks up the session from env.

**Finding**: The code is correct end-to-end. The real gap is **integration test coverage** — there are no tests that verify parallel sessions don't interfere with each other. The US2 acceptance scenarios are testable with the existing code.

### What needs to happen
- Add integration tests that run two sessions in parallel with different `CLI_REPLAY_SESSION` values
- Verify that `verify` finds session-specific state
- Verify that `clean` removes only session-specific state
- No code changes needed for `verify` or `clean` (they already use `StateFilePath` which is session-aware)

---

## R6: Clean Command Idempotency (FR-018)

### Decision: Add graceful handling for already-cleaned state

### Rationale
Current `cmd/clean.go` implementation:
- `runner.DeleteState(stateFile)` already handles missing files (returns nil if `os.IsNotExist`)
- `os.RemoveAll(state.InterceptDir)` fails silently if the directory is already removed (it logs a warning but doesn't error)
- The main concern is when the state file itself doesn't exist (already cleaned or never created) — `ReadState` returns `os.ErrNotExist`, and `DeleteState` handles it

**Finding**: `clean` is *mostly* idempotent already. The one gap: if the state file doesn't exist, `DeleteState` succeeds silently, but the user still gets the "state reset for X" success message. This is acceptable behavior. The only change needed is to not error when the state file is already gone — which is already the case via `runner.DeleteState`.

**Conclusion**: FR-018 is already satisfied by the existing implementation. Just need test coverage to confirm.
