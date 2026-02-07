# Research: P0 Critical Enhancements

**Feature Branch**: `005-p0-critical-enhancements`
**Date**: 2026-02-07

---

## R-001: stdin Capture Strategy

### Decision: Temp file + `[ ! -t 0 ]` guard (Unix), `[Console]::IsInputRedirected` (Windows)

### Rationale
- **Unix bash shim**: `[ ! -t 0 ]` detects piped stdin (POSIX standard). `cat > "$STDIN_FILE"` drains stdin to a temp file. Avoids `VAR=$(cat)` which strips trailing newlines via command substitution. The temp file is replayed to the real command via `< "$STDIN_FILE"`.
- **Windows PS1 shim**: `[Console]::IsInputRedirected` detects piped stdin (available since .NET 4.5 / PowerShell 3.0+). `[Console]::In.ReadToEnd()` reads all input. The `.cmd` entry point passes stdin through automatically — no changes needed there.
- **Replay path (intercept mode)**: When cli-replay is invoked as a symlink, it inherits the pipe from the parent process. The Go binary reads `os.Stdin` directly — no temp file or env var needed. This is the simplest path.
- **Record path (shim mode)**: The shim writes stdin to a temp file, replays it to the real command, and includes the content in the JSONL log entry (new `stdin` field in `RecordingEntry`).

### Alternatives Considered
| Approach | Rejected Because |
|---|---|
| `VAR=$(cat)` in bash | Command substitution strips trailing newlines |
| `cat /dev/stdin` | Not POSIX; absent on some BSDs |
| `$input` in PowerShell | Only works for pipeline objects, not raw process stdin |
| Env var (`CLI_REPLAY_STDIN`) | Size limit: ~128KB Linux, ~32KB Windows |
| CLI argument | Size limit + visible in `ps` output |

### Size Handling
- Temp file on disk: no memory limit for capture
- JSONL embedding: cap at 1 MB for the `stdin` field (base64-encode if non-UTF-8)
- Replay path: `io.LimitReader(os.Stdin, 1<<20)` to enforce the cap in Go

---

## R-002: stdin Trailing Newline Normalization

### Decision: Strip a single trailing `\n` before comparison

### Rationale
- `echo "hello" | cmd` sends `hello\n` (echo appends newline)
- `printf "hello" | cmd` sends `hello` (no trailing newline)
- Scenario authors typically write stdin content without a trailing newline in YAML
- Stripping one trailing `\n` from both expected and actual prevents false mismatches
- Only strip trailing `\n`, not `\r\n` separately — normalize `\r\n` → `\n` first (Windows pipes)

### Implementation
```go
func normalizeStdin(s string) string {
    s = strings.ReplaceAll(s, "\r\n", "\n")
    return strings.TrimRight(s, "\n")
}
```

---

## R-003: Mismatch Diagnostics — Per-Element Diff with Template Context

### Decision: Add `ElementMatchDetail` function in matcher package; enhance `FormatMismatchError` in runner

### Rationale
- The existing `elementMatch` returns `bool` — no way to explain *why* a template pattern failed
- Adding a parallel function `ElementMatchDetail(pattern, value string) MatchDetail` avoids changing the hot-path signature
- Called only when mismatch is already detected (zero cost on happy path)
- Returns `Kind` (literal/wildcard/regex), `Pattern` (the regex string), and `FailReason` (human-readable explanation)

### Key Fix: `findFirstDiff` Is Broken for Templates
The existing `findFirstDiff` in `errors.go` uses raw string comparison (`a[i] != b[i]`). This reports false diffs at wildcard positions where `{{ .any }}` ≠ `"actual-value"`. Must use `elementMatch` inside the diff loop.

### Output Format
- **≤ 12 elements**: Show all elements with ✓/✗ markers
- **> 12 elements**: Show `[0..2]` prefix, `...`, `[diff-2..diff+2]` context, `...`, tail — matching `git diff -U3` mental model
- **Template failures**: Show the pattern and what was tested: `expected pattern "^prod-.*", got "staging-app"`
- **Length mismatches**: Show count difference and list extra/missing elements

### Color Strategy
- Plain text by default
- `NO_COLOR` env var respected (per no-color.org convention)
- Auto-detect via `term.IsTerminal` on stderr when `CLI_REPLAY_COLOR` is unset
- Dependency: `golang.org/x/term` (check if already transitive; add if not)

---

## R-004: Call Count Bounds — State Model

### Decision: Replace `ConsumedSteps []bool` with `StepCounts []int`

### Rationale
- `[]int` is a strict superset of `[]bool` semantically: `count >= 1` ≡ `true`
- State files are ephemeral (in `/tmp`, hashed names, wiped by `clean`) — no long-lived migration needed
- Keeping both fields creates ambiguity about which is authoritative

### Migration Strategy
Keep `ConsumedSteps` as a read-only deprecated JSON field for one release cycle:
```go
type State struct {
    // ... existing fields ...
    StepCounts    []int  `json:"step_counts,omitempty"`
    ConsumedSteps []bool `json:"consumed_steps,omitempty"` // deprecated: migration only
}
```
In `ReadState`, after unmarshalling: if `StepCounts` is nil but `ConsumedSteps` exists, convert `true` → `1`, `false` → `0`. Nil out `ConsumedSteps`. `WriteState` never writes `ConsumedSteps`.

---

## R-005: Call Count Bounds — Replay Engine Flow

### Decision: "Budget check before match" pattern with single-step soft advance

### Replay Loop (New)
```
1. stepIndex = state.CurrentStep
2. WHILE step[stepIndex] budget exhausted (count >= max):
     stepIndex++  (skip past fully-consumed steps)
3. state.CurrentStep = stepIndex
4. TRY match(step[stepIndex], argv):
     IF match:
       state.StepCounts[stepIndex]++
       IF StepCounts[stepIndex] >= step.Calls.Max:
         state.CurrentStep++  (eager advance for next invocation)
       respond(step)
     IF no match AND StepCounts[stepIndex] >= step.Calls.Min:
       SOFT ADVANCE: try step[stepIndex + 1]
       IF match: proceed with next step
       ELSE: MismatchError (report both steps tried)
     IF no match AND StepCounts[stepIndex] < step.Calls.Min:
       HARD MISMATCH: current step hasn't met min
```

### Rationale
- Budget check at start handles crash recovery (state saved after max but before pointer advanced)
- Soft advance is single-step only — scanning all remaining steps would create confusing out-of-order behavior
- Min-gate on soft advance prevents typos from silently skipping steps
- MismatchError enriched with `SoftAdvanced` flag and `NextStepIndex` for debugging

---

## R-006: Call Count Bounds — Scenario Model

### Decision: Optional `*CallBounds` pointer on `Step` with `EffectiveCalls()` helper

### Model
```go
type CallBounds struct {
    Min int `yaml:"min"`
    Max int `yaml:"max"`
}

type Step struct {
    Match   Match       `yaml:"match"`
    Respond Response    `yaml:"respond"`
    Calls   *CallBounds `yaml:"calls,omitempty"` // nil = {min:1, max:1}
    When    string      `yaml:"when,omitempty"`
}
```

### Backward Compatibility
- The YAML loader uses `KnownFields(true)` (strict parsing)
- Old YAML files: `calls` field absent → `Calls` is `nil` → `EffectiveCalls()` returns `{1, 1}` → identical to current behavior
- Zero changes needed for existing scenarios

### Validation Rules
- `calls.min >= 0`
- `calls.max >= 1`
- `calls.max >= calls.min`

---

## R-007: Security Allowlist — Validation Approach

### Decision: Validate at `run` time, before creating any intercepts

### Rationale
- The allowlist restricts which command names can appear in `argv[0]` across all steps
- Validation happens in `cmd/run.go` after scenario loading, before `createIntercept` loop
- If any `argv[0]` is not in the allowlist, `run` exits with a clear error listing all disallowed commands
- No runtime impact — this is a pre-flight check only

### Allowlist Sources (priority order)
1. CLI flag `--allowed-commands kubectl,az,aws` → parsed into `[]string`
2. YAML field `meta.security.allowed_commands: ["kubectl", "az"]` → loaded from scenario
3. Both present → intersection (strictest set wins)
4. Neither present → no restriction (backward compatible)

### Command Name Matching
- Compare base name only: `filepath.Base(argv[0])` against allowlist entries
- Case-sensitive on Unix, case-insensitive on Windows (`strings.EqualFold` gated on `runtime.GOOS`)

---

## R-008: Dependencies

### Decision: Add `golang.org/x/term` for terminal detection (mismatch color support)

### Rationale
- `term.IsTerminal(fd)` is the standard Go idiom for detecting terminal output
- Used to auto-detect whether to emit ANSI color codes in mismatch diagnostics
- Lightweight dependency (part of the Go extended library)
- Check whether it's already a transitive dependency before adding

### No Other New Dependencies
- All other features (stdin capture, call counts, allowlist) use only the standard library and existing dependencies (cobra, yaml.v3, testify)
