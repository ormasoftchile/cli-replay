# CLI Contract: 009 P1/P2 Enhancements

**Date**: 2026-02-07  
**Spec**: [spec.md](../spec.md)

---

## 1. Dynamic Capture — YAML Schema Extension

### Step Response `capture` field

```yaml
# Location: step[].respond.capture
# Type: map[string]string
# Optional: yes (omitempty)

steps:
  - match:
      argv: ["<command>", "<args...>"]
    respond:
      exit: <int 0-255>
      stdout: "<template string>"
      capture:
        <identifier>: "<value>"
```

### Constraints

| Rule | Constraint |
|------|-----------|
| Identifier format | `^[a-zA-Z_][a-zA-Z0-9_]*$` |
| Identifier uniqueness | Must not conflict with any key in `meta.vars` |
| Value type | String only |
| Forward references | Templates referencing `{{ .capture.X }}` where step defining `X` has a higher flat index produce a validation error |
| Unordered group siblings | Sibling captures not yet produced resolve to `""` (no error) |
| Optional step captures | If step has `calls.min: 0` and is never invoked, capture resolves to `""` |

### Template Access

```yaml
# Captured values are accessed via the .capture namespace
respond:
  stdout: "Resource: {{ .capture.resource_id }}"
```

### Validation Errors

| Condition | Error message |
|-----------|---------------|
| Invalid identifier format | `capture identifier "X" must match [a-zA-Z_][a-zA-Z0-9_]*` |
| Conflicts with meta.vars | `capture identifier "X" conflicts with meta.vars key "X"` |
| Forward reference detected | `step N references capture "X" first defined at step M (forward reference)` |

---

## 2. Dry-Run Mode — CLI Flag

### `cli-replay run --dry-run`

```
USAGE:
  cli-replay run [--dry-run] <scenario.yaml>

FLAGS:
  --dry-run    Preview the scenario step sequence without creating intercepts
               or modifying the environment. Output goes to stdout.

BEHAVIOR:
  1. Load and validate scenario file
  2. If validation fails → print errors to stderr, exit 1
  3. If valid → print formatted step summary to stdout, exit 0
  4. No side effects: no intercept dirs, no state files, no PATH changes

EXIT CODES:
  0   Scenario is valid, summary printed
  1   Scenario has validation errors

OUTPUT FORMAT:
  Human-readable text to stdout (see data-model.md DryRunReport)
  - Numbered step list with match argv, exit code, stdout preview
  - Group boundaries with name and mode
  - Template variables shown as placeholders (not rendered)
  - Allowlist validation results
  - Capture identifiers listed per step
```

### `cli-replay exec --dry-run`

```
USAGE:
  cli-replay exec [--dry-run] <scenario.yaml> -- <command> [args...]

FLAGS:
  --dry-run    Preview the scenario and validate matching for the given command
               without spawning a child process. Output goes to stdout.

BEHAVIOR:
  1. Load and validate scenario file
  2. Print step summary (same as `run --dry-run`)
  3. No child process is spawned
  4. No intercepts, no state files, no environment changes

EXIT CODES:
  0   Scenario is valid, summary printed
  1   Scenario has validation errors
```

### Flag Compatibility

```
--dry-run + --ttl         → Allowed (TTL info shown in summary)
```

---

## 3. Windows Signal Handling — Behavioral Contract

### Signal Forwarding Behavior

| Platform | Signal           | Action                              |
|----------|------------------|-------------------------------------|
| Unix     | SIGINT (Ctrl+C)  | Forward `SIGINT` to child process   |
| Unix     | SIGTERM          | Forward `SIGTERM` to child process  |
| Windows  | Ctrl+C           | Call `Process.Kill()` on child      |
| Windows  | taskkill /PID    | Best-effort cleanup via defer       |

### Cleanup Contract

| Event                    | Intercept dir removed | State file removed | Child terminated |
|--------------------------|----------------------|-------------------|-----------------|
| Child exits normally     | ✓                    | ✓                 | N/A             |
| Ctrl+C (Unix)           | ✓                    | ✓                 | ✓ (SIGINT)      |
| SIGTERM (Unix)          | ✓                    | ✓                 | ✓ (SIGTERM)     |
| Ctrl+C (Windows)        | ✓                    | ✓                 | ✓ (Kill)        |
| Parent killed (Windows) | Best-effort          | Best-effort       | Not guaranteed  |

### Known Limitations

- Windows `Process.Kill()` does not propagate to grandchild processes. Orphaned grandchildren are a known limitation documented in README.
- PowerShell `eval` pattern (`cli-replay run | Invoke-Expression`) lacks trap-based cleanup. Users must call `cli-replay clean` manually.

---

## 4. Benchmark Contract

### Required Benchmarks

| Benchmark Name                    | Package              | Step Count | Threshold        |
|-----------------------------------|----------------------|------------|------------------|
| `BenchmarkArgvMatch_100`          | `internal/matcher`   | 100        | < 1ms/step avg   |
| `BenchmarkArgvMatch_500`          | `internal/matcher`   | 500        | < 1ms/step avg   |
| `BenchmarkStateRoundTrip_500`     | `internal/runner`    | 500        | < 10ms total     |
| `BenchmarkGroupMatch_50`          | `internal/matcher`   | 50 (group) | < 1ms/step avg   |
| `BenchmarkFormatJSON_200`         | `internal/verify`    | 200        | < 50ms total     |
| `BenchmarkFormatJUnit_200`        | `internal/verify`    | 200        | < 50ms total     |
| `BenchmarkReplayOrchestration_100`| `internal/runner`    | 100        | < 500ms total    |

### Invocation

```bash
go test -bench=. -benchmem ./internal/matcher/ ./internal/runner/ ./internal/verify/
```

### Existing Benchmarks (must not regress)

| Benchmark                | Package            | Current Size |
|--------------------------|--------------------|-------------|
| `BenchmarkFormatJSON`    | `internal/verify`  | 10 steps    |
| `BenchmarkFormatJUnit`   | `internal/verify`  | 10 steps    |
| `BenchmarkBuildResult`   | `internal/verify`  | 10 steps    |
| `TestFormatOverheadUnder5ms` | `internal/verify` | 10 steps |
