# Data Model: P1 CI/CD Enhancements

**Feature**: 006-p1-cicd-enhancements  
**Date**: 2026-02-07

## Entity Overview

```
┌───────────────────────┐
│     ExecSession       │ NEW — managed lifecycle for exec mode
│  (transient, in-proc) │
├───────────────────────┤
│ scenarioPath  string  │──→ scenario.Scenario (existing)
│ absPath       string  │
│ sessionID     string  │──→ State.key (via StateFilePathWithSession)
│ interceptDir  string  │
│ stateFile     string  │
│ childCmd      *Cmd    │──→ os/exec.Cmd (stdlib)
│ cleaned       bool    │    (idempotent guard)
└───────────────────────┘
         │ creates
         ▼
┌───────────────────────┐
│       State           │ EXISTING — no schema changes
│  (persisted JSON)     │
├───────────────────────┤
│ scenario_path  string │
│ scenario_hash  string │
│ current_step   int    │
│ total_steps    int    │
│ step_counts    []int  │
│ intercept_dir  string │
│ last_updated   Time   │
└───────────────────────┘

┌───────────────────────┐
│   CleanupTrap         │ NEW — emitted shell code (transient)
│  (stdout text)        │
├───────────────────────┤
│ functionName  string  │  "_cli_replay_clean"
│ guardVar      string  │  "_cli_replay_cleaned"
│ signals       []str   │  ["EXIT", "INT", "TERM"]
│ cleanCommand  string  │  'command cli-replay clean "$CLI_REPLAY_SCENARIO"'
└───────────────────────┘
```

## Entities

### ExecSession (NEW — transient, in-process)

**Purpose**: Encapsulates the full lifecycle of a `cli-replay exec` invocation: setup, spawn, wait, verify, cleanup.

**Not persisted** — this is a Go struct used during `runExec()` execution. It does not appear in state files.

| Field | Type | Description |
|---|---|---|
| `scenarioPath` | `string` | Original scenario path from CLI args |
| `absPath` | `string` | Absolute resolved path to scenario YAML |
| `sessionID` | `string` | Unique hex session ID (from `generateSessionID()`) |
| `interceptDir` | `string` | Temp directory containing symlinks/copies of cli-replay binary |
| `stateFile` | `string` | Path to session-specific state JSON file |
| `childCmd` | `*exec.Cmd` | The user's spawned command |
| `cleaned` | `bool` | Guard flag for idempotent cleanup |

**Lifecycle**:
1. `Setup()` — load scenario, validate, create intercept dir, symlinks, init state file
2. `Spawn()` — build `exec.Cmd` with modified env, start child process
3. `Wait()` — wait for child exit, capture exit code
4. `Verify()` — reload state, check `AllStepsMetMin`
5. `Cleanup()` — remove intercept dir, delete state file (idempotent)

**Validation Rules**:
- `scenarioPath` must point to a valid YAML file (validated before spawn)
- `sessionID` must be non-empty (generated automatically)
- Child command (argv after `--`) must be non-empty

### State (EXISTING — no changes)

**Purpose**: Tracks scenario progress across CLI invocations. Already supports session isolation via `StateFilePathWithSession()`.

**No schema changes**. The `State` struct and its JSON serialization remain identical:

```json
{
  "scenario_path": "/abs/path/to/scenario.yaml",
  "scenario_hash": "abc123...",
  "current_step": 2,
  "total_steps": 5,
  "step_counts": [3, 1, 0, 0, 0],
  "intercept_dir": "/tmp/cli-replay-intercept-xyz",
  "last_updated": "2026-02-07T10:30:00Z"
}
```

**Key invariant**: `StateFilePath(scenarioPath)` internally reads `CLI_REPLAY_SESSION` and produces a unique file path per session. Exec mode sets `CLI_REPLAY_SESSION` on the child process's environment, ensuring its intercept calls use the same state file.

### CleanupTrap (NEW — transient, emitted as shell text)

**Purpose**: Shell code emitted by `emitShellSetup()` to ensure auto-cleanup on exit or signal.

**Not a Go struct** — this is generated text output. The "entity" describes the structure of the emitted code for design clarity.

| Component | Value | Description |
|---|---|---|
| Function name | `_cli_replay_clean` | Cleanup function, namespaced to avoid collisions |
| Guard variable | `_cli_replay_cleaned` | Prevents double execution |
| Signals | `EXIT INT TERM` | Maximum portability across POSIX shells |
| Clean command | `command cli-replay clean "$CLI_REPLAY_SCENARIO"` | `command` prefix bypasses PATH intercepts |

**Emitted output** (appended to existing `emitShellSetup` output for bash/zsh/sh):
```sh
_cli_replay_clean() { if [ -n "${_cli_replay_cleaned:-}" ]; then return; fi; _cli_replay_cleaned=1; command cli-replay clean "$CLI_REPLAY_SCENARIO" 2>/dev/null; }
trap '_cli_replay_clean' EXIT INT TERM
```

## Relationships

```
User CLI invocation
  │
  ├─── cli-replay exec scenario.yaml -- ./script.sh
  │         │
  │         ▼
  │    ExecSession (transient)
  │         │
  │         ├── Creates → State (persisted JSON, session-specific)
  │         ├── Creates → InterceptDir (temp dir with symlinks)
  │         ├── Spawns  → Child Process (user's command)
  │         ├── Waits   → Child exit code
  │         ├── Reads   → State (for verification)
  │         └── Deletes → State + InterceptDir (cleanup)
  │
  ├─── eval "$(cli-replay run scenario.yaml)"
  │         │
  │         ├── Emits → export statements (PATH, SESSION, SCENARIO)
  │         ├── Emits → CleanupTrap (function + trap statement)
  │         └── Creates → State + InterceptDir
  │
  └─── cli-replay verify / clean
            │
            └── Reads CLI_REPLAY_SESSION from env → session-specific State file
```

## State Transitions

### ExecSession Lifecycle

```
[Created] ──Setup──→ [Ready] ──Spawn──→ [Running] ──Wait──→ [Exited]
                                                                │
                                                          ┌─────┴─────┐
                                                          ▼           ▼
                                                   [Verify OK]  [Verify Fail]
                                                          │           │
                                                          └─────┬─────┘
                                                                ▼
                                                          [Cleaned Up]
```

**Signal interruption** can occur during `[Running]` → transitions directly to `[Cleaned Up]` via the signal handler calling `cleanup()`.

### State File Lifecycle (unchanged)

```
[Not Exists] ──NewState──→ [Active] ──IncrementStep──→ [Active]
                                                          │
                                                    AllStepsMetMin?
                                                    ┌─────┴─────┐
                                                    ▼           ▼
                                              [Complete]   [Active]
                                                    │
                                              DeleteState
                                                    │
                                                    ▼
                                             [Not Exists]
```
