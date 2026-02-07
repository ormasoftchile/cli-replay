# Tasks: P0 Critical Enhancements

**Input**: Design documents from `/specs/005-p0-critical-enhancements/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

**Tests**: Included — SC-006 requires ≥ 90% branch coverage for changed code.

**Organization**: Tasks grouped by user story. Each story is independently testable after completion.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story (US1–US4) from spec.md

## Path Conventions

- Go project at repository root
- Source: `internal/`, `cmd/`
- Tests: `*_test.go` co-located with source
- Fixtures: `testdata/scenarios/`

---

## Phase 1: Setup

**Purpose**: Add new dependency required by US1 mismatch color detection

- [x] T001 Add golang.org/x/term dependency by running `go get golang.org/x/term` and verify go.mod/go.sum updated

---

## Phase 2: Foundational — Data Model & State Infrastructure

**Purpose**: Shared type definitions and state format migration that multiple user stories build upon

**⚠️ CRITICAL**: US2, US3, and US4 depend on these model/state changes

- [x] T002 [P] Add CallBounds struct (Min/Max int with yaml tags), Security struct (AllowedCommands []string), Stdin field to Match, Calls *CallBounds to Step, Security *Security to Meta, and EffectiveCalls() method on Step in internal/scenario/model.go
- [x] T003 [P] Add StepCounts []int field to State, implement ConsumedSteps→StepCounts migration in ReadState, update NewState to initialize StepCounts, update Advance() to increment StepCounts, update AllStepsConsumed/IsStepConsumed to use StepCounts in internal/runner/state.go
- [x] T004 Extend Step.Validate() to validate CallBounds (min >= 0, max >= 1, max >= min) with defaulting logic (nil calls → {1,1}; only min given → max = min; only max given → min stays 0) in internal/scenario/model.go
- [x] T005 [P] Tests for CallBounds validation, EffectiveCalls defaults, Security struct, Match.Stdin YAML parsing, and validation edge cases (max:0 rejected, min > max rejected) in internal/scenario/model_test.go
- [x] T006 [P] Tests for StepCounts migration from ConsumedSteps, updated Advance/AllStepsConsumed/IsStepConsumed with StepCounts, and NewState initialization in internal/runner/state_test.go

**Checkpoint**: Foundation ready — all new types exist, state format migrated, user story implementation can begin

---

## Phase 3: User Story 1 — Improved Mismatch Diagnostics (Priority: P1) 🎯 MVP

**Goal**: When a command mismatch occurs, show per-element diff with template pattern context so the developer can identify the problem without opening the scenario file.

**Independent Test**: Trigger a deliberate mismatch (wrong argument) and verify the error output includes a per-element diff with the first divergence point, template pattern display for regex/wildcard, and length mismatch details.

### Implementation

- [x] T007 [P] [US1] Add MatchDetail struct (Matched bool, Kind string, Pattern string, FailReason string) and ElementMatchDetail(pattern, value string) MatchDetail function in internal/matcher/argv.go
- [x] T008 [US1] Rewrite FormatMismatchError to use ElementMatchDetail for per-element diff: show scenario name, 1-based step number, full expected/received argv, first divergence with expected vs received values, template pattern display for regex/wildcard, and length mismatch reporting in internal/runner/errors.go
- [x] T009 [US1] Fix findFirstDiff to use elementMatch instead of raw string comparison (a[i] != b[i]) to eliminate false diffs at template positions in internal/runner/errors.go
- [x] T010 [US1] Add color output support: auto-detect terminal via term.IsTerminal on stderr, respect NO_COLOR env var and CLI_REPLAY_COLOR override in internal/runner/errors.go

### Tests

- [x] T011 [P] [US1] Tests for ElementMatchDetail: literal match, literal fail, wildcard match, regex match, regex fail with FailReason in internal/matcher/argv_test.go
- [x] T012 [US1] Tests for enhanced FormatMismatchError: literal diff, regex pattern display, wildcard skip, length mismatch (extra args, missing args), long argv truncation (>12 elements) in internal/runner/errors_test.go

**Checkpoint**: Mismatch diagnostics fully functional — triggering any argv mismatch shows element-level diff with pattern context

---

## Phase 4: User Story 2 — Call Count Bounds (Priority: P2)

**Goal**: Steps can declare min/max invocation counts to support retry and polling loops while maintaining strict ordering for non-polling steps.

**Independent Test**: Create a scenario with a polling step (calls.min: 1, max: 5), invoke it 3 times, advance to next step, and verify completion with per-step counts.

### Implementation

- [x] T013 [US2] Add IncrementStep(idx int), StepBudgetRemaining(idx, maxCalls int) int, and AllStepsMetMin(steps []scenario.Step) bool methods to State in internal/runner/state.go
- [x] T014 [US2] Implement budget-check-before-match loop (skip exhausted steps), call count increment on match, auto-advance when count reaches max, and soft-advance when current step mismatches but min is met in ExecuteReplay in internal/runner/replay.go
- [x] T015 [US2] Extend MismatchError with SoftAdvanced bool, NextStepIndex int, and NextExpected []string fields in internal/runner/replay.go
- [x] T016 [US2] Update FormatMismatchError to display soft-advance context (both steps tried, counts, and budget info) when SoftAdvanced is true in internal/runner/errors.go
- [x] T017 [P] [US2] Update verify command to check AllStepsMetMin and report per-step invocation counts with min/max bounds in cmd/cli-replay/verify.go
- [x] T018 [P] [US2] Update verify command to check AllStepsMetMin and report per-step invocation counts with min/max bounds in cmd/verify.go

### Tests

- [x] T019 [US2] Tests for IncrementStep, StepBudgetRemaining, AllStepsMetMin in internal/runner/state_test.go
- [x] T020 [US2] Tests for call count replay: repeated calls within budget, auto-advance at max, soft-advance when min met, hard mismatch when min not met, no calls field defaults to exactly-once in internal/runner/replay_test.go
- [x] T021 [P] [US2] Tests for verify with per-step counts: all met, min not met, optional step (min:0) skipped in cmd/cli-replay/verify_test.go
- [x] T022 [P] [US2] Create test fixture testdata/scenarios/call_bounds.yaml with polling step (min:1, max:5) and normal steps

**Checkpoint**: Call count bounds fully functional — polling/retry steps work, verify reports per-step counts

---

## Phase 5: User Story 3 — stdin Matching (Priority: P3)

**Goal**: Validate piped stdin content during replay and capture it during recording.

**Independent Test**: Create a scenario with match.stdin, pipe matching content into the intercepted command, verify the step matches and response is returned.

### Implementation — Replay Path (intercept mode)

- [x] T023 [US3] Add normalizeStdin helper (strings.ReplaceAll \r\n→\n, TrimRight \n) and stdin reading via io.LimitReader(os.Stdin, 1<<20) with comparison against match.stdin in ExecuteReplay in internal/runner/replay.go
- [x] T024 [US3] Add stdin mismatch error formatting (show truncated expected vs received stdin, first 200 chars) to FormatMismatchError in internal/runner/errors.go

### Implementation — Record Path (shim capture)

- [x] T025 [P] [US3] Add Stdin field to RecordedCommand struct in internal/recorder/command.go
- [x] T026 [P] [US3] Add Stdin field to RecordingEntry struct and propagate Stdin in ToRecordedCommands in internal/recorder/log.go
- [x] T027 [US3] Update LogRecording to accept stdin string parameter and write it to the JSONL entry in internal/recorder/shim.go
- [x] T028 [US3] Update ConvertToScenario to populate match.stdin from RecordedCommand.Stdin when non-empty in internal/recorder/converter.go
- [x] T029 [P] [US3] Add stdin capture block to bashShimTemplate: detect piped stdin with `[ ! -t 0 ]` guard, drain to temp file via `cat > "$STDIN_FILE"`, replay to real command via `< "$STDIN_FILE"`, include content in JSONL entry in internal/platform/unix.go
- [x] T030 [P] [US3] Add stdin capture block to ps1ShimTemplate: detect with `[Console]::IsInputRedirected`, read via `[Console]::In.ReadToEnd()`, pass to process via RedirectStandardInput, include in JSONL entry in internal/platform/windows.go

### Tests

- [x] T031 [US3] Tests for stdin matching in replay: exact match, mismatch error, no stdin field ignores input, trailing newline normalization, 1 MB cap in internal/runner/replay_test.go
- [x] T032 [P] [US3] Tests for LogRecording with stdin field in internal/recorder/shim_test.go
- [x] T033 [P] [US3] Tests for ConvertToScenario with stdin: non-empty stdin emitted, empty stdin omitted in internal/recorder/converter_test.go
- [x] T034 [P] [US3] Create test fixture testdata/scenarios/stdin_match.yaml with match.stdin content

**Checkpoint**: stdin matching fully functional — piped content validated during replay, captured during recording

---

## Phase 6: User Story 4 — Security Allowlist (Priority: P4)

**Goal**: Restrict which commands can be intercepted via YAML config or CLI flag, with violations rejected before PATH manipulation.

**Independent Test**: Create a scenario with meta.security.allowed_commands, add a step referencing a disallowed command, verify cli-replay run rejects it before creating intercepts.

### Implementation

- [x] T035 [US4] Add validateAllowlist(steps, yamlList, cliList []string) error function using filepath.Base for command extraction and runtime.GOOS-aware comparison in cmd/run.go
- [x] T036 [US4] Add --allowed-commands string flag to run command, parse comma-separated list, compute intersection with YAML allowed_commands, call validateAllowlist before createIntercept loop in cmd/run.go
- [x] T037 [P] [US4] Add --allowed-commands flag and allowlist validation to run command in cmd/cli-replay/run.go

### Tests

- [x] T038 [US4] Tests for validateAllowlist: all allowed passes, disallowed rejected with error listing command, path-based argv[0] uses base name, empty list allows all, intersection logic in cmd/run_test.go (new file)
- [x] T039 [P] [US4] Create test fixture testdata/scenarios/security_allowlist.yaml with meta.security.allowed_commands set

**Checkpoint**: Security allowlist fully functional — disallowed commands rejected before any intercepts created

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Validation, backward compatibility, and cleanup

- [x] T040 Run full test suite (`go test ./...`) and fix any regressions across all packages
- [x] T041 [P] Verify backward compatibility: run existing test scenarios in testdata/scenarios/ (single_step.yaml, multi_step.yaml) and confirm no behavior changes
- [x] T042 [P] Validate quickstart.md scenarios from specs/005-p0-critical-enhancements/quickstart.md work end-to-end
- [x] T043 Run `go vet ./...` and `golangci-lint run` to ensure no lint violations

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Phase 2 (only needs golang.org/x/term from Phase 1; model types not required but phase gate ensures clean baseline)
- **US2 (Phase 4)**: Depends on Phase 2 (needs CallBounds, StepCounts) + Phase 3 (extends FormatMismatchError)
- **US3 (Phase 5)**: Depends on Phase 2 (needs Match.Stdin) — independent of US1/US2 for model, but extends replay.go after US2
- **US4 (Phase 6)**: Depends on Phase 2 (needs Security struct) — independent of US1/US2/US3
- **Polish (Phase 7)**: Depends on all story phases being complete

### User Story Dependencies

- **US1 (P1)**: Can start after Phase 2 — no dependencies on other stories
- **US2 (P2)**: Should follow US1 (extends FormatMismatchError that US1 rewrites)
- **US3 (P3)**: Should follow US2 (both modify ExecuteReplay; US3 adds stdin logic after US2's budget check)
- **US4 (P4)**: Can start after Phase 2 — independent of US1/US2/US3 (only touches cmd/run.go)

### Within Each User Story

- Implementation tasks before test tasks
- Same-file tasks are sequential
- Different-file tasks marked [P] can run in parallel
- Story checkpoint validates the feature independently

### Parallel Opportunities

- Phase 2: T002 ∥ T003 (model.go ∥ state.go), then T005 ∥ T006 (model_test ∥ state_test)
- Phase 3: T007 → T008–T010 (T008 depends on T007's ElementMatchDetail), then T011 ∥ T012
- Phase 4: T017 ∥ T018 (two verify files), T021 ∥ T022 (verify_test ∥ fixture)
- Phase 5: T025 ∥ T026 ∥ T029 ∥ T030 (command.go ∥ log.go ∥ unix.go ∥ windows.go), T032 ∥ T033 ∥ T034
- Phase 6: T037 ∥ T036 proceeds independently, T038 ∥ T039
- US4 can run in parallel with US3 (different files entirely: cmd/run.go vs internal/runner/)

---

## Parallel Example: Phase 2 (Foundational)

```
# Round 1 — different files, no dependencies:
T002: Add types to internal/scenario/model.go
T003: Add StepCounts to internal/runner/state.go

# Round 2 — depends on Round 1, but parallel with each other:
T004: Extend validation in internal/scenario/model.go (depends on T002)
T005: Tests in internal/scenario/model_test.go (depends on T004)
T006: Tests in internal/runner/state_test.go (depends on T003)
```

## Parallel Example: Phase 5 (US3 — stdin)

```
# Round 1 — four independent files:
T025: RecordedCommand.Stdin in internal/recorder/command.go
T026: RecordingEntry.Stdin in internal/recorder/log.go
T029: bashShimTemplate in internal/platform/unix.go
T030: ps1ShimTemplate in internal/platform/windows.go

# Round 2 — depends on Round 1:
T027: LogRecording stdin param in internal/recorder/shim.go (depends on T026)
T028: ConvertToScenario stdin in internal/recorder/converter.go (depends on T025)

# Round 3 — tests in parallel:
T032: shim_test.go (depends on T027)
T033: converter_test.go (depends on T028)
T034: test fixture (no code dependency)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (add golang.org/x/term)
2. Complete Phase 2: Foundational (model types + state migration)
3. Complete Phase 3: US1 — Mismatch Diagnostics
4. **STOP and VALIDATE**: Run `go test ./...`, trigger a mismatch manually, verify enhanced output
5. This MVP is immediately valuable — every user benefits from better error messages

### Incremental Delivery

1. Setup + Foundational → Stable baseline with new types
2. Add US1 → Test independently → Enhanced mismatch diagnostics (MVP!)
3. Add US2 → Test independently → Polling/retry scenarios work
4. Add US3 → Test independently → Pipe-based workflows validated
5. Add US4 → Test independently → CI security controls in place
6. Each story adds value without breaking previous stories (all backward compatible)

### Sequential Recommendation

Due to shared file modifications (replay.go, errors.go), the recommended execution order is strictly:

```
Phase 1 → Phase 2 → US1 → US2 → US3 → US4 → Polish
```

US4 can optionally be parallelized with US3 (different files).

---

## Notes

- All changes are additive — no breaking changes to existing YAML schemas or CLI behavior
- State file migration (ConsumedSteps → StepCounts) is automatic on read
- `[P]` tasks target different files with no incomplete dependencies
- Two verify implementations exist: cmd/verify.go and cmd/cli-replay/verify.go — both need updating
- Test files are co-located with source per Go convention
- Commit after each completed task or story checkpoint
