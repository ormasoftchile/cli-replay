# Tasks: P1/P2 Enhancements — Dynamic Capture, Dry-Run, Windows Audit & Benchmarks

**Input**: Design documents from `/specs/009-p1-p2-enhancements/`
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/cli.md ✓, quickstart.md ✓

**Tests**: Tests are included inline within each user story phase (Go idiomatic: tests live next to implementation code).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4)
- Exact file paths included in all task descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Test fixtures and scenario files that multiple user stories depend on

- [X] T001 Create capture chain test scenario in testdata/scenarios/capture_chain.yaml — 3-step scenario where step 1 captures `rg_id`, step 2 captures `vm_id`, step 3 references both via `{{ .capture.rg_id }}` and `{{ .capture.vm_id }}`
- [X] T002 [P] Create capture conflict test scenario in testdata/scenarios/capture_conflict.yaml — scenario where a capture identifier matches an existing `meta.vars` key
- [X] T003 [P] Create capture forward reference test scenario in testdata/scenarios/capture_forward_ref.yaml — scenario where step 1 references `{{ .capture.x }}` but `x` is first defined in step 2
- [X] T004 [P] Create capture group test scenario in testdata/scenarios/capture_group.yaml — scenario with an unordered group where sibling steps define and reference captures

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema changes to scenario model and state that ALL user stories depend on

**⚠️ CRITICAL**: US1 (Dynamic Capture) and US2 (Dry-Run) cannot begin until this phase is complete

- [X] T005 Add `Capture map[string]string` field with tag `yaml:"capture,omitempty"` to `Response` struct in internal/scenario/model.go (after `Delay` field at line 225)
- [X] T006 Add capture identifier validation to `Response.Validate()` in internal/scenario/model.go — reject identifiers not matching `^[a-zA-Z_][a-zA-Z0-9_]*$`
- [X] T007 Add capture-vs-vars conflict detection to `Scenario.Validate()` in internal/scenario/model.go — iterate `FlatSteps()` and reject any capture key that exists in `meta.vars`
- [X] T008 Add forward-reference detection to `Scenario.Validate()` in internal/scenario/model.go — iterate `FlatSteps()` with accumulating set of defined capture IDs, parse `respond.stdout`/`respond.stderr` templates using `text/template/parse` (AST-based) to extract `.capture.X` field references, reject any reference to a capture ID not yet in the accumulated set
- [X] T009 Add `Captures map[string]string` field with tag `json:"captures,omitempty"` to `State` struct in internal/runner/state.go (after `LastUpdated` field at line 25)
- [X] T010 Initialize `Captures: make(map[string]string)` in `NewState()` function in internal/runner/state.go (at line ~237)
- [X] T011 Add unit tests for capture identifier validation (valid, invalid, digit-leading) in internal/scenario/model_test.go
- [X] T012 Add unit tests for capture-vars conflict and forward-reference detection in internal/scenario/model_test.go
- [X] T013 Add state round-trip test for `Captures` field — `WriteState` → `ReadState` with captures present, and backward-compat test with old state JSON missing `captures` key, in internal/runner/state_test.go

**Checkpoint**: Scenario model accepts `capture` field, state persists captures, all validation rules enforced

---

## Phase 3: User Story 1 — Dynamic Capture: Chain Output Between Steps (Priority: P1) 🎯 MVP

**Goal**: Captured key-value pairs from step responses propagate to subsequent step templates via `{{ .capture.<id> }}`

**Independent Test**: Load `testdata/scenarios/capture_chain.yaml`, run replay, verify step 3 output contains captured values from steps 1 and 2

### Template Rendering Changes

- [X] T014 [US1] Add `RenderWithCaptures(tmpl string, vars map[string]string, captures map[string]string) (string, error)` function in internal/template/render.go — builds `map[string]interface{}` with flat vars + nested `"capture"` key holding captures sub-map, then executes template. IMPORTANT: use `missingkey=zero` (not `missingkey=error`) for the template option so that unresolved capture references (from optional steps or unordered group siblings) resolve to empty string instead of erroring. The existing `Render` function retains `missingkey=error` for backward compatibility.
- [X] T015 [US1] Add unit tests for `RenderWithCaptures` in internal/template/render_test.go — test `{{ .capture.x }}` resolution, test that referencing an undefined capture key resolves to empty string (not error), test with empty captures map, test that top-level vars still error on missing keys

### Replay Pipeline Changes

- [X] T016 [US1] Update `ReplayResponseWithTemplate` in internal/runner/replay.go to accept a `captures map[string]string` parameter and call `RenderWithCaptures` instead of `Render` for stdout/stderr content
- [X] T017 [US1] Update `ExecuteReplay` in internal/runner/replay.go to merge `step.Respond.Capture` into `state.Captures` after each step response is served (before `WriteState`), and pass `state.Captures` to `ReplayResponseWithTemplate`
- [X] T018 [US1] Handle unordered group capture semantics in `ExecuteReplay` — within a group, pass current `state.Captures` (which only contains captures from already-executed steps) without error for missing sibling captures
- [X] T019 [US1] Handle optional step captures — if step has `calls.min: 0` and is never invoked, ensure its captures are not added to `state.Captures` (no-op by default since capture merge only happens on invocation)

### Integration Tests

- [X] T020 [US1] Add integration test in internal/runner/replay_test.go — load `capture_chain.yaml`, simulate 3-step replay, assert step 3 stdout contains captured values from steps 1 and 2
- [X] T021 [US1] Add integration test in internal/runner/replay_test.go — load `capture_group.yaml`, simulate group replay with varied ordering, assert best-effort capture resolution (empty string for not-yet-executed siblings)
- [X] T022 [US1] Add validation test in internal/scenario/loader_test.go — load `capture_conflict.yaml` and assert validation error about naming conflict; load `capture_forward_ref.yaml` and assert forward reference error

**Checkpoint**: Dynamic capture is fully functional — 3-step chain scenario works end-to-end, validation catches conflicts and forward refs

---

## Phase 4: User Story 2 — Dry-Run Mode: Preview Without Interception (Priority: P1)

**Goal**: `--dry-run` flag on `run` and `exec` commands prints step summary to stdout with zero file system side effects

**Independent Test**: Run `cli-replay run --dry-run testdata/scenarios/multi_step.yaml`, verify stdout contains numbered step list, no `.cli-replay/` directory created

### Dry-Run Report Engine

- [X] T023 [P] [US2] Create `DryRunReport` and `DryRunStep` structs in internal/runner/dryrun.go per data-model.md — fields for scenario name, description, steps, groups, commands, allowlist, template vars, session TTL
- [X] T024 [US2] Implement `BuildDryRunReport(scn *scenario.Scenario) *DryRunReport` function in internal/runner/dryrun.go — populate report from scenario metadata, `FlatSteps()`, `GroupRanges()`, extract commands and allowlist issues
- [X] T025 [US2] Implement `FormatDryRunReport(report *DryRunReport, w io.Writer) error` function in internal/runner/dryrun.go — format the numbered step table with match argv, exit code, stdout preview (first 80 chars or `[file: path]`), call bounds, group membership, capture identifiers, and allowlist validation results per data-model.md output format
- [X] T026 [P] [US2] Add unit tests for `BuildDryRunReport` in internal/runner/dryrun_test.go — test with linear scenario, grouped scenario, scenario with captures, scenario with allowlist
- [X] T027 [US2] Add unit tests for `FormatDryRunReport` in internal/runner/dryrun_test.go — assert output contains numbered steps, group indicators, template placeholders shown as-is, allowlist flags

### CLI Integration — `run` command

- [X] T028 [US2] Add `--dry-run` bool flag to `run` command in cmd/run.go (in `init()` alongside existing flags at line ~49)
- [X] T029 [US2] Insert dry-run early return in `runRun()` in cmd/run.go — after `checkAllowlist` (line ~75) and before TTL cleanup (line ~76): if `--dry-run`, call `runner.BuildDryRunReport` + `runner.FormatDryRunReport` to stdout, then `return nil`

### CLI Integration — `exec` command

- [X] T031 [US2] Add `--dry-run` bool flag to `exec` command in cmd/exec.go (in `init()` alongside existing flags)
- [X] T032 [US2] Insert dry-run early return in exec command handler in cmd/exec.go — after scenario load and validation, before child process creation: if `--dry-run`, call `runner.BuildDryRunReport` + `runner.FormatDryRunReport` to stdout, then `return nil`

### CLI Tests

- [X] T033 [P] [US2] Add tests for `run --dry-run` in cmd/run_test.go — valid scenario prints summary to stdout, invalid scenario writes error to stderr and exits non-zero, no state files or intercept dirs created
- [X] T034 [P] [US2] Add tests for `exec --dry-run` in cmd/exec_test.go — valid scenario prints summary, no child process spawned

**Checkpoint**: Dry-run mode works on both `run` and `exec` — zero side effects, full step summary, allowlist validation

---

## Phase 5: User Story 3 — Windows Signal Handling Audit (Priority: P1)

**Goal**: `exec` mode correctly terminates child processes on Windows using `Process.Kill()` instead of unsupported `SIGTERM`

**Independent Test**: On Windows, build and run `cli-replay exec` with a long-running child, Ctrl+C, verify child terminates and intercept dir is cleaned up

### Extract Signal Handling to Build-Tagged Files

- [X] T036 [P] [US3] Create cmd/exec_unix.go with build tag `//go:build !windows` — extract signal setup function `setupSignalForwarding(childCmd *exec.Cmd) (cleanup func())` that registers `SIGINT` + `SIGTERM` via `signal.Notify`, spawns goroutine forwarding signals via `Process.Signal(sig)`, returns cleanup function that calls `signal.Stop`/`close`
- [X] T037 [P] [US3] Create cmd/exec_windows.go with build tag `//go:build windows` — implement `setupSignalForwarding(childCmd *exec.Cmd) (cleanup func())` that registers `os.Interrupt` only, spawns goroutine that calls `Process.Kill()` on signal receipt, returns cleanup function
- [X] T038 [US3] Refactor signal handling block in cmd/exec.go (lines 142-162) to call `setupSignalForwarding(childCmd)` — remove inline `signal.Notify`/goroutine code, replace with single function call, remove `syscall` import from exec.go

### Documentation

- [X] T039 [US3] Add Windows signal handling section to README.md troubleshooting — document that `SIGTERM` is not supported on Windows, `Process.Kill()` is used instead, grandchild processes may be orphaned, and recommend `cli-replay clean` for stale state

### Testing

- [X] T040 [US3] Add build-constrained test in cmd/exec_test.go — on Windows: verify `setupSignalForwarding` returns a non-nil cleanup function; on Unix: verify signal channels are registered for SIGINT and SIGTERM

**Checkpoint**: Windows signal handling is reliable — child processes terminate on Ctrl+C, cleanup runs, behavior documented

---

## Phase 6: User Story 4 — Performance Benchmarks for 100+ Step Scenarios (Priority: P2)

**Goal**: Benchmark suite covers matching, state I/O, group matching, and formatting at 100/200/500 step scales

**Independent Test**: Run `go test -bench=. -benchmem ./internal/matcher/ ./internal/runner/ ./internal/verify/` and verify all new benchmarks execute within thresholds

### Matcher Benchmarks

- [X] T041 [P] [US4] Create internal/matcher/bench_test.go — implement `BenchmarkArgvMatch` with parametric sub-benchmarks for 100 and 500 steps using `b.Run(fmt.Sprintf("steps=%d", n), ...)`, generate step patterns with varying argv lengths
- [X] T042 [P] [US4] Add `BenchmarkGroupMatch_50` in internal/matcher/bench_test.go — benchmark matching 50 steps within an unordered group (iterate all steps to find match)

### State I/O Benchmarks

- [X] T043 [P] [US4] Create internal/runner/bench_test.go — implement `BenchmarkStateRoundTrip` with parametric sub-benchmarks for 100 and 500 steps, measure `WriteState` → `ReadState` round-trip using `t.TempDir()` for isolation

### Verify Benchmarks (Extend Existing)

- [X] T044 [P] [US4] Add `benchResultN(n int)` parametric helper to internal/verify/bench_test.go — generalize existing `benchResult()` to accept step count, generate N steps with 2+ groups
- [X] T045 [US4] Add `BenchmarkFormatJSON_200` and `BenchmarkFormatJUnit_200` in internal/verify/bench_test.go — use `benchResultN(200)` helper, same pattern as existing 10-step benchmarks
- [X] T046 [US4] Verify existing 10-step benchmarks (`BenchmarkFormatJSON`, `BenchmarkFormatJUnit`, `BenchmarkBuildResult`, `TestFormatOverheadUnder5ms`) still pass without modification in internal/verify/bench_test.go

### Replay Orchestration Benchmark

- [X] T047 [US4] Add `BenchmarkReplayOrchestration_100` in internal/runner/bench_test.go — benchmark `ExecuteReplay` with a 100-step synthetic scenario, measure end-to-end processing time

**Checkpoint**: Benchmark suite covers 100/200/500 step scenarios, all benchmarks complete within thresholds

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, validation, and cleanup across all user stories

- [X] T048 [P] Update README.md usage section — add dynamic capture example from quickstart.md, add `--dry-run` flag documentation for `run` and `exec` commands
- [X] T049 [P] Update README.md YAML reference — document `capture` field on `respond` with identifier constraints and `{{ .capture.<id> }}` template access
- [X] T050 Run `go vet ./...` and `go test ./...` across entire project — verify zero regressions from all changes
- [X] T051 Run quickstart.md scenarios manually (capture-demo, dry-run preview, benchmarks) to validate end-to-end behavior matches documentation

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — create test fixture files immediately
- **Foundational (Phase 2)**: Depends on Setup (T001-T004) for test scenarios — BLOCKS US1 and US2
- **US1 Dynamic Capture (Phase 3)**: Depends on Foundational (Phase 2) — needs `Capture` field on Response and `Captures` on State
- **US2 Dry-Run (Phase 4)**: Depends on Foundational (Phase 2) — needs `Capture` field for capture display in dry-run output; can be implemented in parallel with US1
- **US3 Windows Signals (Phase 5)**: Depends only on Setup (Phase 1) — **can run in parallel** with US1/US2 since it modifies different files (`cmd/exec.go`, `cmd/exec_unix.go`, `cmd/exec_windows.go`)
- **US4 Benchmarks (Phase 6)**: Depends on Setup (Phase 1) — **can run in parallel** with US1/US2/US3 since it only adds new test files
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Depends on Phase 2 — No dependencies on other user stories
- **US2 (P1)**: Depends on Phase 2 — Should reference capture field in dry-run output (soft dependency on US1 schema, but schema is in Phase 2)
- **US3 (P1)**: Independent — Only touches cmd/exec*.go files
- **US4 (P2)**: Independent — Only adds bench_test.go files

### Within Each User Story

- Schema/model changes before service/logic changes
- Logic changes before CLI integration
- Unit tests alongside or immediately after implementation
- Integration tests after core implementation

### Parallel Opportunities

**After Phase 2 completes, these can all run simultaneously:**
- US1 (Phase 3): Template rendering + replay pipeline
- US2 (Phase 4): Dry-run report engine + CLI integration
- US3 (Phase 5): Build-tagged signal refactor
- US4 (Phase 6): All benchmark files

---

## Parallel Example: After Foundational Phase

```
# All of these can launch simultaneously after Phase 2:

# US1 — Template + Replay (developer A)
Task T014: "RenderWithCaptures function in internal/template/render.go"
Task T015: "RenderWithCaptures tests in internal/template/render_test.go"

# US2 — Dry-Run (developer B)
Task T023: "DryRunReport structs in internal/runner/dryrun.go"
Task T026: "BuildDryRunReport tests in internal/runner/dryrun_test.go"

# US3 — Windows Signals (developer C)
Task T036: "cmd/exec_unix.go with build tag"
Task T037: "cmd/exec_windows.go with build tag"

# US4 — Benchmarks (developer D)
Task T041: "internal/matcher/bench_test.go"
Task T043: "internal/runner/bench_test.go"
Task T044: "benchResultN helper in internal/verify/bench_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (test fixtures)
2. Complete Phase 2: Foundational (schema + validation)
3. Complete Phase 3: User Story 1 (Dynamic Capture)
4. **STOP and VALIDATE**: Run `capture_chain.yaml` end-to-end
5. Commit and verify — capture works independently

### Incremental Delivery

1. Setup + Foundational → Schema ready
2. Add US1 → Test capture chain → Commit (MVP!)
3. Add US2 → Test dry-run preview → Commit
4. Add US3 → Test Windows signal handling → Commit
5. Add US4 → Run benchmarks → Commit
6. Polish → README, full test suite → Commit
7. Each story adds value without breaking previous stories

---

## Notes

- [P] tasks = different files, no dependencies on in-progress tasks
- [Story] label maps task to specific user story for traceability
- All file paths are relative to repository root (`c:\One\OpenSource\cli-replay`)
- Go convention: tests live alongside source in `*_test.go` files
- Build tags: `//go:build windows` / `//go:build !windows` (Go 1.17+ syntax)
- Commit after each phase checkpoint for safe incremental progress
