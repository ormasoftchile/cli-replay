# Research: Workflow Recorder

**Feature**: 002-workflow-recorder  
**Created**: February 6, 2026  
**Purpose**: Resolve technical unknowns and establish best practices for recording command executions and generating YAML scenarios

## Research Questions

### 1. Shim Generation Strategy

**Question**: How should the recorder generate and deploy shim executables to intercept commands?

**Alternatives Considered**:

A. **Go template-based shim generation** - Generate Go source files, compile on-the-fly
B. **Shell script shims** - Create bash/sh wrapper scripts
C. **Single recorder binary symlinks** - Symlink target commands to cli-replay itself

**Decision**: **B - Shell script shims**

**Rationale**:
- **Simplicity**: Bash scripts are trivial to generate, no compilation overhead
- **Performance**: Minimal startup time (<10ms vs ~100ms for Go compilation)
- **Portability**: Works on macOS and Linux without additional tooling
- **Transparency**: Users can inspect shims for debugging (plain text vs compiled binary)
- **No CGO concerns**: Avoids any potential dynamic linking issues

**Implementation approach**:
```bash
#!/bin/bash
RECORDING_LOG="${CLI_REPLAY_RECORDING_LOG}"
REAL_CMD="$(which -a "$0" | grep -v "$CLI_REPLAY_SHIM_DIR" | head -1)"

# Capture execution
OUTPUT=$("$REAL_CMD" "$@" 2>&1)
EXIT_CODE=$?

# Log to JSONL
echo "{\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"argv\":$(printf '%s\n' "$0" "$@" | jq -Rs 'split("\n")[:-1]'),\"exit\":$EXIT_CODE,\"stdout\":\"$(echo "$OUTPUT" | jq -Rs .)\",\"stderr\":\"\"}" >> "$RECORDING_LOG"

echo "$OUTPUT"
exit $EXIT_CODE
```

**Trade-offs**:
- Requires `jq` for JSON escaping → **Mitigation**: Document as soft dependency or implement Go-based shim for jq-less systems
- Shell script parsing complexity for special characters → **Mitigation**: Use `printf '%s\n' "$@"` for proper quoting

---

### 2. Recording Storage Format

**Question**: Should recorded commands be stored in memory or written incrementally to disk during capture?

**Alternatives Considered**:

A. **In-memory collection** - Store in process memory, write at session end
B. **JSONL incremental logging** - Append each command to log file immediately
C. **SQLite embedded DB** - Use SQLite for queryable recording history

**Decision**: **B - JSONL incremental logging**

**Rationale**:
- **Crash resilience**: If recorder process crashes, partial recordings are preserved
- **Low memory footprint**: No need to buffer 50+ command outputs in RAM
- **Simplicity**: Line-delimited JSON is trivial to parse and append
- **Streaming support**: Future enhancement could tail the log in real-time
- **No dependencies**: Plain text format, no DB library needed

**Format**:
```jsonl
{"timestamp":"2026-02-06T14:30:22Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"NAME    READY...\n","stderr":""}
{"timestamp":"2026-02-06T14:30:25Z","argv":["kubectl","delete","pod","web-0"],"exit":0,"stdout":"pod \"web-0\" deleted\n","stderr":""}
```

**Trade-offs**:
- File I/O on every command → **Negligible**: Append operations are fast, <1ms overhead
- No built-in deduplication → **Acceptable**: Spec clarifies all executions recorded separately

---

### 3. YAML Generation Approach

**Question**: How should the recorder convert JSONL recordings to scenario YAML?

**Alternatives Considered**:

A. **Direct struct marshaling** - Parse JSONL → Scenario struct → yaml.Marshal
B. **Template-based generation** - Use text/template to render YAML
C. **Hybrid** - Struct for validation, template for formatting

**Decision**: **A - Direct struct marshaling**

**Rationale**:
- **Type safety**: Reuses existing `scenario.Scenario`, `scenario.Step`, `scenario.Response` types
- **Validation**: Automatically validates against schema via `Validate()` methods
- **Maintainability**: Single source of truth (scenario.go) for YAML structure
- **Simplicity**: `yaml.v3` handles multiline strings, escaping, indentation correctly

**Implementation**:
```go
func ConvertToScenario(log *RecordingLog, meta scenario.Meta) (*scenario.Scenario, error) {
    steps := make([]scenario.Step, 0, len(log.Entries))
    for _, entry := range log.Entries {
        steps = append(steps, scenario.Step{
            Match: scenario.Match{Argv: entry.Argv},
            Respond: scenario.Response{
                Exit:   entry.ExitCode,
                Stdout: entry.Stdout,
                Stderr: entry.Stderr,
            },
        })
    }
    s := &scenario.Scenario{Meta: meta, Steps: steps}
    if err := s.Validate(); err != nil {
        return nil, fmt.Errorf("generated invalid scenario: %w", err)
    }
    return s, nil
}
```

**Trade-offs**:
- Less control over YAML formatting → **Acceptable**: yaml.v3 defaults are readable
- No custom comments in output → **Acceptable**: Not a requirement

---

### 4. PATH Manipulation Strategy

**Question**: How should the recorder modify PATH to prioritize shims?

**Alternatives Considered**:

A. **Export PATH in parent shell** - Modify user's environment
B. **Create subprocess with modified PATH** - Spawn shell with custom PATH
C. **Wrapper script** - Generate a script that sets PATH then runs user command

**Decision**: **B - Create subprocess with modified PATH**

**Rationale**:
- **Isolation**: No side effects on user's shell environment
- **Explicit control**: Recorder owns the execution context
- **Testability**: Easy to verify PATH modification in tests
- **Cross-platform**: `exec.Cmd` with custom Env works consistently

**Implementation**:
```go
func (r *Recorder) RunCommand(userCmd []string) error {
    cmd := exec.Command(userCmd[0], userCmd[1:]...)
    cmd.Env = append(os.Environ(),
        "PATH="+r.shimDir+":"+os.Getenv("PATH"),
        "CLI_REPLAY_RECORDING_LOG="+r.logFile,
        "CLI_REPLAY_SHIM_DIR="+r.shimDir,
    )
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
```

**Trade-offs**:
- Subprocess overhead → **Negligible**: <5ms for process spawn
- User sees subprocess, not direct execution → **Acceptable**: Transparent to end user

---

### 5. Command Filtering Implementation

**Question**: How should `--command` flag filter which commands to record?

**Alternatives Considered**:

A. **Shim generation filtering** - Only create shims for specified commands
B. **Post-execution filtering** - Record all, filter during YAML generation
C. **Smart shim** - Shim checks filter list before logging

**Decision**: **A - Shim generation filtering**

**Rationale**:
- **Efficiency**: Don't intercept commands we won't record (no overhead)
- **Clarity**: User sees exactly which commands are shimmed (`ls $SHIM_DIR`)
- **Simplicity**: Filter logic in one place (shim generator)
- **Performance**: No runtime filtering checks in hot path

**Implementation**:
```go
func (r *Recorder) GenerateShims(commands []string) error {
    if len(commands) == 0 {
        // No filter - get all commands in PATH
        commands = getAllExecutables()
    }
    for _, cmd := range commands {
        shimPath := filepath.Join(r.shimDir, cmd)
        if err := r.createShim(cmd, shimPath); err != nil {
            return err
        }
    }
    return nil
}
```

**Trade-offs**:
- Can't capture commands discovered mid-session → **Acceptable**: Static command list aligns with use cases
- Requires PATH scanning for "all commands" mode → **Acceptable**: One-time cost at session start

---

## Best Practices Summary

### Go Idioms for Recorder

1. **Use `os/exec` with context**: Support cancellation for long-running recordings
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
   defer cancel()
   cmd := exec.CommandContext(ctx, ...)
   ```

2. **Defer cleanup**: Ensure temporary shim directory is removed
   ```go
   defer os.RemoveAll(r.shimDir)
   ```

3. **Errors**: Wrap errors with context
   ```go
   if err := r.GenerateShims(commands); err != nil {
       return fmt.Errorf("failed to generate shims: %w", err)
   }
   ```

4. **Table-driven tests**: Test shim generation, JSONL parsing, YAML conversion
   ```go
   tests := []struct {
       name     string
       input    []RecordingEntry
       wantYAML string
   }{
       {"single command", ...},
       {"multi-step", ...},
   }
   ```

### YAML Marshaling

- Use `yaml:"field,omitempty"` for optional fields (stderr, description)
- Use `yaml:"-"` for internal fields not in YAML
- Test round-trip: YAML → Scenario → YAML to ensure stability

### Error Handling

- Validate user inputs (output path writable, commands exist)
- Provide actionable error messages: "command 'kubectl' not found in PATH"
- Fail fast: Don't start recording if setup fails

## Technology Decisions

| Component | Choice | Justification |
|-----------|--------|---------------|
| **Shim format** | Bash script | Portability, simplicity, no compilation |
| **Recording storage** | JSONL | Crash resilience, low memory, streaming |
| **YAML generation** | Direct marshaling | Type safety, validation, maintainability |
| **PATH manipulation** | Subprocess with custom env | Isolation, testability, cross-platform |
| **Command filtering** | Shim generation time | Efficiency, clarity, no runtime overhead |
| **JSON escaping** | `jq` (or Go fallback) | Handles complex strings, widely available |
| **Temp directory** | `os.MkdirTemp` | Standard lib, auto-cleanup pattern |

## Open Questions (for Implementation)

1. Should recorder support `--exclude` flag in addition to `--command`?
   - **Recommendation**: Defer to P3 (not needed for MVP)

2. How to handle commands that don't exist in PATH?
   - **Recommendation**: Error early with clear message

3. Should JSONL log be kept after YAML generation?
   - **Recommendation**: Delete by default, add `--keep-log` flag if needed later

4. How to handle very long stdout (>1MB)?
   - **Recommendation**: No truncation for MVP; address if real-world issue emerges (YAGNI)
