# Research: 009 P1/P2 Enhancements

**Date**: 2026-02-07  
**Spec**: [spec.md](spec.md)

## R1: State File Schema & Persistence Pattern

### Decision
Add `Captures map[string]string` to the `State` struct with JSON tag `json:"captures,omitempty"`.

### Rationale
- `WriteState` uses `json.MarshalIndent` → atomic rename (`path.tmp` → `path`). Adding a map field requires zero changes to persist/read logic — `encoding/json` handles it automatically.
- Old state files missing `"captures"` will unmarshal with a `nil` map, which is safe. `omitempty` keeps serialized output clean when no captures exist.
- The `NewState` function initializes fields explicitly; we add `Captures: make(map[string]string)` there for consistency so callers can write without nil-checking.
- Per-step persistence (spec clarification) is already satisfied: `WriteState` is called after every step advancement in the replay loop.

### Alternatives Considered
- **Separate captures file**: Rejected — adds I/O complexity, breaks atomicity guarantee, and the state file is already small (JSON with step counts).
- **Embedded struct for captures**: Rejected — a simple `map[string]string` matches the `meta.vars` pattern and avoids unnecessary abstraction for V1.

---

## R2: Template Rendering Pipeline — Capture Injection Point

### Decision
Inject captures into the template variable map in `ReplayResponseWithTemplate` after `MergeVars`/`MergeVarsFiltered` and before `template.Render`. Captures are namespaced under the `capture` key via a nested map.

### Rationale
- `ReplayResponseWithTemplate` at `internal/runner/replay.go` L55-L111 is the single point where template variables are assembled and passed to `template.Render`.
- Current flow: `template.MergeVars(scn.Meta.Vars)` → `template.Render(content, vars)`.
- `template.Render` converts `map[string]string` to `map[string]interface{}` before executing the Go template. We change the function signature to accept `map[string]interface{}` directly (or add captures as a nested map post-merge).
- Using `{{ .capture.pod_name }}` namespace avoids collision with `meta.vars` keys (which use `{{ .var_name }}`).
- The spec requires validation that capture IDs don't conflict with `meta.vars` keys — this is enforced at scenario load/validation time, not at render time.

### Alternatives Considered
- **Inject captures into `MergeVars` itself**: Rejected — `MergeVars` handles env+vars merging; capture injection is a different lifecycle (runtime state, not scenario config).
- **Separate template pass for captures**: Rejected — double-rendering is fragile and produces unexpected behavior with escaped braces.

---

## R3: Exec Signal Handling on Windows

### Decision
Use build-tagged signal handling: `exec_unix.go` uses `syscall.SIGTERM`; `exec_windows.go` uses `Process.Kill()` for termination. Document that grandchild processes may be orphaned on Windows.

### Rationale
- Current code in `cmd/exec.go` L142-L178 uses `syscall.SIGTERM` which **does not work on Windows** — `Process.Signal(syscall.SIGTERM)` returns `"not supported by windows"`.
- `Process.Kill()` is the only reliable cross-platform forced termination on Windows.
- `os.Interrupt` (SIGINT) works on Windows via `GenerateConsoleCtrlEvent` in Go 1.20+, so Ctrl+C is already partially handled. The gap is SIGTERM forwarding.
- The `internal/platform/` package already uses build tags for OS-specific behavior (`platform_windows.go`, `platform_unix.go`) — consistent pattern to follow.
- Cleanup (intercept dir removal, state file cleanup) already runs via `defer cleanup()` which is unaffected by signal changes.

### Alternatives Considered
- **Windows Job Objects**: Would allow killing entire process trees. Rejected for V1 — requires Windows-specific syscalls, adds significant complexity. Document as future improvement.
- **Context-based cancellation**: Rejected — `exec.CommandContext` kills the process but doesn't allow signal forwarding to the child first.

---

## R4: Scenario Model — Capture Field on Response

### Decision
Add `Capture map[string]string` to the `Response` struct with YAML tag `yaml:"capture,omitempty"`. Add validation in `Response.Validate()` for capture identifier format and in `Scenario.Validate()` for conflicts with `meta.vars`.

### Rationale
- The `Response` struct at `internal/scenario/model.go` L202-L209 is the natural home — captures are semantically part of the step's response (they define what values a response "produces").
- `Response.Validate()` at L227-L238 already validates exit codes and file mutual exclusivity — extending it with capture identifier validation (alphanumeric + underscore, no leading digit) fits the pattern.
- Cross-cutting validation (capture ID vs `meta.vars` key conflict) must be in `Scenario.Validate()` since it spans the meta section and individual steps.
- Forward-reference detection (step N referencing a capture from step M where M > N) requires iterating `FlatSteps()` with an accumulating set of defined capture IDs.

### Alternatives Considered
- **Separate `Capture` struct**: Rejected — for V1, `map[string]string` is sufficient. A struct adds indirection without benefit since captures are simple key-value pairs.
- **Capture on `Step` instead of `Response`**: Rejected — semantically, captures are produced by the response, not the step. Placing on `Response` makes the YAML more intuitive: `respond: { stdout: "...", capture: { id: "value" } }`.

---

## R5: Benchmark Patterns — Scaling to 100+ Steps

### Decision
Extend the existing benchmark infrastructure in `internal/verify/bench_test.go` and add new benchmark files in `internal/matcher/` and `internal/runner/` for matching, state I/O, and full replay orchestration at 100, 200, and 500 step counts.

### Rationale
- Existing benchmarks in `internal/verify/bench_test.go` use a `benchResult()` helper that constructs a 10-step `VerifyResult`. The pattern is: helper creates data → benchmark loop calls the function under test → `b.ResetTimer()` before the hot loop.
- `internal/matcher/argv_test.go` has **no benchmarks** currently — this is a gap since matching is on the hot path for every intercepted command.
- State I/O benchmarks should cover `WriteState` → `ReadState` round-trips at 500 steps to ensure the atomic rename + JSON pattern scales.
- Parametric benchmarks via `b.Run(fmt.Sprintf("steps=%d", n), ...)` allow testing multiple sizes in a single benchmark function.

### Alternatives Considered
- **External benchmarking tool (e.g., `hyperfine`)**: Rejected for unit-level benchmarks — Go's built-in `testing.B` is the idiomatic approach and integrates with `go test -bench`.
- **Benchmark only at integration level**: Rejected — integration benchmarks conflate I/O with logic. Unit benchmarks isolate the hot path.

---

## R6: Dry-Run Feasibility — Natural Cut Point in `run` Command

### Decision
Insert the `--dry-run` check in `cmd/run.go` after allowlist validation (line ~62) and before hashing/intercept creation (line ~77). For `exec`, insert after scenario load and before child process launch. Output a structured text summary to stdout.

### Rationale
- At the cut point in `runRun` (after step 4 in the flow), all information needed for a dry-run summary is available:
  - `scn.Meta.Name`, `scn.Meta.Description`
  - `scn.FlatSteps()` — all steps with match/respond/calls
  - `scn.GroupRanges()` — group boundaries
  - `commands` — extracted command list
  - Security allowlist from `scn.Meta.Security`
  - Template variable names from `scn.Meta.Vars`
- No side effects have occurred yet — no intercept dir, no state file, no binary resolution.
- For `exec`, the dry-run path is: load scenario → validate → print summary → return (no child process spawned).
- Stdout is the correct output destination (spec clarification) since shell setup is suppressed in dry-run mode.

### Alternatives Considered
- **Separate `dry-run` subcommand**: Rejected — `--dry-run` as a flag is the standard CLI pattern (cf. `terraform plan`, `kubectl --dry-run`).
- **Dry-run at scenario load time only**: Rejected — allowlist validation and group structure are needed for a useful preview. The cut point after validation provides the most complete summary.
