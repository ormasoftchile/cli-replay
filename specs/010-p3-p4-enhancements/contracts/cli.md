# CLI Contract: 010 P3/P4 Enhancements — validate Command & Job Object Platform Abstraction

**Date**: 2026-02-07  
**Spec**: [spec.md](../spec.md)

---

## 1. `cli-replay validate` — Standalone Validation Command

### Command Definition

```
USAGE:
  cli-replay validate [--format <text|json>] <file>...

DESCRIPTION:
  Validate one or more scenario YAML files for schema and semantic correctness
  without executing them. Does not create any files, directories, or modify any
  environment state.

ARGUMENTS:
  <file>...    One or more paths to scenario YAML files (required; at least one)

FLAGS:
  --format string    Output format: text (default) or json

EXIT CODES:
  0   All files pass validation
  1   One or more files have errors, or usage error (no files specified)
```

### Behavior Contract

| Phase | Action | Side Effects |
|-------|--------|-------------|
| 1. Parse args | Validate `--format` flag, resolve file paths to absolute | None |
| 2. For each file | `scenario.LoadFile(absPath)` → strict YAML parse + `Validate()` | None |
| 3. File ref check | Check `stdout_file`/`stderr_file` paths exist relative to scenario dir | None (read-only `os.Stat`) |
| 4. Report | Text to stderr, or JSON to stdout | None |
| 5. Exit | Code 0 if all valid, code 1 if any errors | None |

### No-Side-Effects Guarantee (FR-014)

The `validate` command MUST NOT:
- Create `.cli-replay/` directories
- Create or modify state files
- Create intercept directories or shim scripts
- Modify `PATH` or any environment variables
- Write any files to disk
- Invoke any external processes

### Multi-File Semantics (FR-010)

- Each file is validated independently
- A failure in file A does not prevent validation of file B
- All results are collected before output
- Exit code 1 if **any** file has errors

### Format: text (default)

Output goes to **stderr**:

```
✓ scenario-a.yaml: valid
✗ scenario-b.yaml:
  - meta: name must be non-empty
  - step 2: calls: min (5) must be <= max (3)
✗ scenario-c.yaml:
  - failed to parse scenario: yaml: line 5: did not find expected key

Result: 1/3 files valid
```

### Format: json

Output goes to **stdout** (for piping/parsing):

```json
[
  {
    "file": "scenario-a.yaml",
    "valid": true,
    "errors": []
  },
  {
    "file": "scenario-b.yaml",
    "valid": false,
    "errors": [
      "meta: name must be non-empty",
      "step 2: calls: min (5) must be <= max (3)"
    ]
  }
]
```

### Error Reporting

| Input Condition | Behavior |
|----------------|----------|
| No arguments | cobra prints usage error, exit 1 (via `cobra.MinimumNArgs(1)`) |
| File not found | `ValidationResult{Valid: false, Errors: ["failed to open scenario file: ..."]}` |
| YAML syntax error | `ValidationResult{Valid: false, Errors: ["failed to parse scenario: yaml: ..."]}` |
| Unknown YAML fields | `ValidationResult{Valid: false, Errors: ["failed to parse scenario: ..."]}` |
| Schema violations | `ValidationResult{Valid: false, Errors: ["<path>: <message>", ...]}` |
| Semantic violations | `ValidationResult{Valid: false, Errors: ["step N: <message>", ...]}` |
| Missing `stdout_file` | `ValidationResult{Valid: false, Errors: ["step N: stdout_file \"...\" not found ..."]}` |
| All valid | `ValidationResult{Valid: true, Errors: []}` per file |

### Validation Rules (exhaustive — all from existing `scenario.LoadFile`)

| # | Rule | Checked By |
|---|------|-----------|
| 1 | `meta.name` non-empty | `Meta.Validate()` |
| 2 | At least one step | `Scenario.Validate()` |
| 3 | `exit` in 0–255 | `Response.Validate()` |
| 4 | `stdout`/`stdout_file` mutually exclusive | `Response.Validate()` |
| 5 | `stderr`/`stderr_file` mutually exclusive | `Response.Validate()` |
| 6 | Capture identifier matches `^[a-zA-Z_][a-zA-Z0-9_]*$` | `Response.Validate()` |
| 7 | `argv` non-empty | `Match.Validate()` |
| 8 | `calls.min ≤ calls.max` | `CallBounds.Validate()` |
| 9 | `calls.max ≥ 1` | `CallBounds.Validate()` |
| 10 | No nested groups | `StepGroup.Validate()` |
| 11 | Groups non-empty | `StepGroup.Validate()` |
| 12 | Group mode = `"unordered"` | `StepGroup.Validate()` |
| 13 | No forward capture references | `Scenario.validateCaptures()` |
| 14 | No capture-vs-vars conflicts | `Scenario.validateCaptures()` |
| 15 | Unknown YAML fields rejected | `KnownFields(true)` in loader |
| 16 | `deny_env_vars` entries non-empty | `Security.Validate()` |
| 17 | Session TTL valid duration + positive | `Session.Validate()` |
| 18 | `stdout_file`/`stderr_file` exist (warn) | **New in validate** |

---

## 2. Windows Job Object — Platform Abstraction Contract

### Signal Forwarding Contract (updated `cmd/exec_windows.go`)

```
FUNCTION:
  setupSignalForwarding(childCmd *exec.Cmd) func()

PLATFORM:
  //go:build windows

OLD BEHAVIOR:
  - Catch os.Interrupt
  - Call childCmd.Process.Kill() (single process only)

NEW BEHAVIOR:
  - Create Windows Job Object with KILL_ON_JOB_CLOSE
  - After childCmd.Start(): assign child to job object
  - On os.Interrupt: call TerminateJobObject() (kills entire tree)
  - On cleanup: CloseHandle(job) (safety net via KILL_ON_JOB_CLOSE)
  - If job object creation fails: fall back to Process.Kill() + warning

RETURN:
  func() — cleanup function that:
    1. Stops signal notification
    2. Closes signal channel
    3. Terminates job object (if active)
    4. Closes job handle
```

### Job Object Lifecycle Contract

| Operation | Success | Failure |
|-----------|---------|---------|
| `NewJobObject()` | Job handle valid | Warning to stderr, fallback mode |
| `SetInformationJobObject(KILL_ON_JOB_CLOSE)` | Auto-cleanup on handle close | Warning, still try to use job |
| `AssignProcess(pid)` | Process tree tracked | Warning, fall back to `Process.Kill()` |
| `Terminate(1)` | All processes in tree killed | Ignore error (may already be dead) |
| `Close()` | Handle released | Ignore error |

### Behavioral Matrix

| Scenario | Unix | Windows (new) | Windows (fallback) |
|----------|------|---------------|-------------------|
| Ctrl+C, child only | SIGINT forwarded | Job terminated | Process.Kill() |
| Ctrl+C, child + grandchildren | SIGINT to process group | Job terminated (all) | Process.Kill() (child only) |
| Child exits normally | Cleanup runs | Cleanup runs | Cleanup runs |
| Parent killed | OS cleanup | KILL_ON_JOB_CLOSE activates | Best-effort |
| Already-exited child | Signal error ignored | TerminateJobObject ignored | Kill error ignored |

### Interface Boundary

The job object is an internal implementation detail of `cmd/exec_windows.go`. It does NOT:
- Change the `setupSignalForwarding` function signature
- Affect the Unix code path
- Change the `exec` command's public interface
- Add new CLI flags
- Change exit code behavior

---

## 3. Schema Extension — scenario.schema.json

No schema changes are required for this feature set. The existing `schema/scenario.schema.json` already covers all YAML fields. The `validate` command reuses the same schema validation.

If the `capture` field is missing from the schema (it was added in 009), verify it is present:

```json
{
  "respond": {
    "properties": {
      "capture": {
        "type": "object",
        "description": "Named key-value pairs produced by this step's response.",
        "additionalProperties": { "type": "string" }
      }
    }
  }
}
```

---

## 4. Flag Compatibility Matrix

| Flag | `run` | `exec` | `verify` | `clean` | `validate` (new) |
|------|-------|--------|----------|---------|-------------------|
| `--format` | — | json, junit | text, json, junit | — | text, json |
| `--report-file` | — | ✓ | — | — | — |
| `--dry-run` | ✓ | ✓ | — | — | — |
| `--allowed-commands` | ✓ | ✓ | — | — | — |
| `--format json` output | N/A | stderr/file | stdout | N/A | stdout |
| `--format text` output | N/A | stderr | stderr | N/A | stderr |
