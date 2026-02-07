# Tasks: P1 CI/CD Enhancements

**Input**: Design documents from `/specs/006-p1-cicd-enhancements/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

**Tests**: Included — spec SC-007 requires all new functionality to be covered by tests.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup

**Purpose**: Extract shared helpers from `cmd/run.go` to enable reuse by the new `exec` command

- [X] T001 Extract `exitCodeFromError()` helper into `internal/runner/exit.go` — converts `exec.Cmd.Wait()` error to integer exit code using `syscall.WaitStatus` for signal-killed processes (128+signum pattern from research.md R1)
- [X] T002 [P] Add unit tests for `exitCodeFromError()` in `internal/runner/exit_test.go` — cover nil error (returns 0), normal non-zero exit, signal-killed (SIGINT→130, SIGTERM→143), and non-ExitError fallback

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared env-building helpers that both `exec` and `run` commands need

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T003 Extract `buildChildEnv()` helper into `internal/runner/env.go` — takes `interceptDir`, `sessionID`, `scenarioPath` and returns `[]string` env slice with PATH prepended, `CLI_REPLAY_SESSION` and `CLI_REPLAY_SCENARIO` set (uses `os.Environ()` as base, modifies PATH in-place per research.md R1)
- [X] T004 [P] Add unit tests for `buildChildEnv()` in `internal/runner/env_test.go` — verify PATH is prepended (not appended), session and scenario vars are present, existing env vars are preserved, and duplicate PATH entries are handled

**Checkpoint**: Foundation ready — user story implementation can now begin

---

## Phase 3: User Story 1 — Sub-process Execution Mode (`exec`) (Priority: P1) 🎯 MVP

**Goal**: Provide `cli-replay exec <scenario> -- <command> [args...]` that manages the full intercept lifecycle (setup → spawn → verify → cleanup) in a single command.

**Independent Test**: Run `cli-replay exec scenario.yaml -- ./test-script.sh`, verify script receives intercepted responses, auto-verify reports completion, and no intercept dir or state file remains on disk.

### Tests for User Story 1

- [X] T005 [P] [US1] Add test for cobra command registration and flag parsing in `cmd/exec_test.go` — verify `exec` subcommand exists on root, `--allowed-commands` flag is registered (`--max-delay` not applicable since primary `cmd/run.go` lacks it; add only if delay capping is needed for exec), `ArgsLenAtDash()` returns correct separator index
- [X] T006 [P] [US1] Add test for `runExec` with valid scenario + successful child in `cmd/exec_test.go` — set up a temp scenario YAML, run exec with `-- echo hello`, verify exit code 0, verify no state file or intercept dir remains
- [X] T007 [P] [US1] Add test for `runExec` with invalid scenario file in `cmd/exec_test.go` — pass a nonexistent YAML path, verify error returned before any child process is spawned, verify no intercept dir was created
- [X] T008 [P] [US1] Add test for `runExec` with missing command after `--` in `cmd/exec_test.go` — pass no args after dash separator, verify usage error returned
- [X] T009 [P] [US1] Add test for exit code propagation in `cmd/exec_test.go` — run exec with `-- sh -c "exit 42"`, verify `runExec` returns exit code 42
- [X] T010 [P] [US1] Add test for verification failure (unconsumed steps) in `cmd/exec_test.go` — create a 2-step scenario, run exec with a script that only triggers 1 step, verify non-zero exit and stderr contains verification failure message
- [X] T011 [P] [US1] Add test for idempotent cleanup in `cmd/exec_test.go` — verify calling cleanup twice does not panic or error (guard variable pattern from research.md R3)

### Implementation for User Story 1

- [X] T012 [US1] Create `cmd/exec.go` — register `exec` cobra command with `Use: "exec [flags] <scenario.yaml> -- <command> [args...]"`, `Short` and `Long` descriptions, `--allowed-commands` flag (matching primary `cmd/run.go`; `--shell` not applicable to exec mode). Add `--max-delay` only if delay capping is required for exec child processes. `RunE: runExec`
- [X] T013 [US1] Implement `runExec()` Phase 1 (pre-spawn validation) in `cmd/exec.go` — parse args to extract scenario path and child command (using `cmd.ArgsLenAtDash()`), resolve absolute path, load and validate scenario via `scenario.LoadFile()`, validate delays via `step.Respond.ValidateDelay()`, validate allowlist via `checkAllowlist()` (FR-009, FR-010)
- [X] T014 [US1] Implement `runExec()` Phase 2 (setup) in `cmd/exec.go` — generate session ID via `generateSessionID()`, create intercept dir and symlinks via `extractCommands()` + `createIntercept()`, initialize state via `runner.NewState()` + `runner.WriteState()`, compute `stateFile` via `runner.StateFilePathWithSession()` (FR-002, FR-008)
- [X] T015 [US1] Implement `runExec()` Phase 3 (spawn + wait) in `cmd/exec.go` — build `exec.Cmd` with `buildChildEnv()`, set `cmd.Stdin/Stdout/Stderr` to `os.Stdin/Stdout/Stderr`, call `cmd.Start()`, set up signal forwarding goroutine (SIGINT/SIGTERM via `signal.Notify` → `cmd.Process.Signal()`), call `cmd.Wait()`, extract exit code via `exitCodeFromError()` (FR-001, FR-005, FR-007)
- [X] T016 [US1] Implement `runExec()` Phase 4 (verify + cleanup) in `cmd/exec.go` — defer idempotent `cleanup()` function (removes intercept dir via `os.RemoveAll` + deletes state via `runner.DeleteState`), after child exits reload state via `runner.ReadState()`, check `state.AllStepsMetMin(scn.Steps)`, print verification diagnostics to stderr on failure (reuse `printPerStepCounts` pattern from `cmd/verify.go`), return child exit code if non-zero, return 1 if verification failed with child exit 0 (FR-003, FR-004, FR-006)
- [X] T017 [US1] Handle edge cases in `cmd/exec.go` — child command not found (exit 127), not executable (exit 126), signal-killed child (128+signum), empty argv after `--` (usage error), and ensure `cleanup()` is called in all code paths via defer

**Checkpoint**: `cli-replay exec` works end-to-end. Can run `cli-replay exec scenario.yaml -- ./script.sh` with auto-setup, auto-verify, auto-cleanup.

---

## Phase 4: User Story 2 — Session Isolation Verification (Priority: P2)

**Goal**: Verify that `verify` and `clean` commands correctly respect `CLI_REPLAY_SESSION` for parallel session isolation. Per research.md R5, the code is already correct — this story is about test coverage.

**Independent Test**: Launch two sessions with different `CLI_REPLAY_SESSION` values, invoke commands in each, run `verify` and `clean` in each, confirm no cross-talk.

### Tests for User Story 2

- [X] T018 [P] [US2] Add session-aware verify test in `cmd/verify_test.go` — create a state file with `StateFilePathWithSession(path, "session-A")`, set `CLI_REPLAY_SESSION=session-A` in env, run `runVerify`, verify it finds the session-specific state and reports correct status
- [X] T019 [P] [US2] Add session-aware clean test in `cmd/clean_test.go` — create a state file and intercept dir with session B, set `CLI_REPLAY_SESSION=session-B` in env, run `runClean`, verify only session B's files are removed
- [X] T020 [P] [US2] Add parallel session isolation test in `cmd/verify_test.go` — create two state files (session-A complete, session-B incomplete) for the same scenario path, verify that `runVerify` with `CLI_REPLAY_SESSION=session-A` returns success while `CLI_REPLAY_SESSION=session-B` returns failure
- [X] T021 [P] [US2] Add backward compatibility test in `cmd/verify_test.go` — unset `CLI_REPLAY_SESSION`, create a state file via `StateFilePath(path)` (no session), run `runVerify`, verify it finds the sessionless state file (FR-014)
- [X] T022 [P] [US2] Add clean idempotency test in `cmd/clean_test.go` — run `runClean` when state file does not exist, verify no error is returned (FR-018)

### Implementation for User Story 2

- [X] T023 [US2] Verify `cmd/clean.go` handles already-cleaned state gracefully — confirm `runClean` does not error when state file is missing (scenario file still required to exist for validation); if needed, add guard to skip `ReadState` error when file not found and proceed to `DeleteState` which is already safe

**Checkpoint**: Session isolation verified with tests. `verify` and `clean` proven to work correctly with `CLI_REPLAY_SESSION` in parallel scenarios.

---

## Phase 5: User Story 3 — Signal-Trap Auto-Cleanup (Priority: P3)

**Goal**: Emit a POSIX-compliant cleanup trap in `cli-replay run` output so that the eval pattern auto-cleans on exit or signal.

**Independent Test**: Run `eval "$(cli-replay run scenario.yaml)"`, capture output, verify it contains the trap function and trap statement targeting EXIT/INT/TERM.

### Tests for User Story 3

- [X] T024 [P] [US3] Add trap emission test for bash/zsh/sh in `cmd/run_test.go` — capture `emitShellSetup` output for the default (bash) shell case, verify output contains `_cli_replay_clean()` function definition, `_cli_replay_cleaned` guard variable, `command cli-replay clean`, and `trap '_cli_replay_clean' EXIT INT TERM`
- [X] T025 [P] [US3] Add trap emission test for powershell in `cmd/run_test.go` — capture `emitShellSetup` output for powershell case, verify it does NOT contain a trap statement (out of scope)
- [X] T026 [P] [US3] Add trap emission test for cmd.exe in `cmd/run_test.go` — capture `emitShellSetup` output for cmd case, verify it does NOT contain a trap statement (out of scope)

### Implementation for User Story 3

- [X] T027 [US3] Add cleanup trap emission to `emitShellSetup()` in `cmd/run.go` — append two lines after the existing export statements in the default (bash/zsh/sh) case: (1) `_cli_replay_clean()` function with guard variable and `command cli-replay clean "$CLI_REPLAY_SCENARIO" 2>/dev/null`, (2) `trap '_cli_replay_clean' EXIT INT TERM` (FR-015, FR-016, FR-017)
- [X] T028 [US3] Evaluate trap emission for `cmd/cli-replay/run.go` — N/A: the alternate command tree does not emit shell exports or support the eval pattern; it only initializes/resumes state. No trap emission needed.

**Checkpoint**: `eval "$(cli-replay run scenario.yaml)"` now emits cleanup traps. Interrupted sessions auto-clean.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Integration validation, documentation, and binary rebuild

- [X] T029 [P] Run `go vet ./...` and `go test ./...` to verify all tests pass and no vet issues
- [X] T030 [P] Rebuild binary via `make build` and verify `cli-replay exec --help` shows the new command
- [X] T031 [P] Add exec mode example scenario in `examples/recordings/` — create a simple scenario YAML and a test script that exercises `cli-replay exec`
- [X] T032 Update README.md — add exec mode documentation (synopsis, examples, CI usage), document trap auto-cleanup behavior, mention session isolation for parallel CI
- [X] T033 Run quickstart.md validation — execute the examples from `specs/006-p1-cicd-enhancements/quickstart.md` against the built binary to confirm they work

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Can proceed in parallel with Phase 1 (different files)
- **US1 (Phase 3)**: Depends on Phase 1 (`exitCodeFromError`) and Phase 2 (`buildChildEnv`) completion — MAIN WORK
- **US2 (Phase 4)**: Depends on Phase 2 foundational only — can proceed in parallel with US1
- **US3 (Phase 5)**: No dependencies on US1 or US2 — can proceed in parallel after Phase 2
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Depends on Phase 1 + Phase 2 helpers. No dependencies on US2 or US3.
- **User Story 2 (P2)**: No code dependencies on other stories — purely test coverage. Can start after Phase 2.
- **User Story 3 (P3)**: No code dependencies on other stories — modifies `cmd/run.go` only. Can start after Phase 2.

### Within Each User Story

- Tests are written first (T005-T011 before T012-T017)
- Implementation tasks are sequential within each story (setup → spawn → verify → cleanup)
- Edge case handling (T017) depends on core implementation (T012-T016)

### Parallel Opportunities

- **Phase 1 + Phase 2**: T001-T004 can all be worked in parallel (different files: `exit.go`, `exit_test.go`, `env.go`, `env_test.go`)
- **All US1 tests** (T005-T011): Can be written in parallel (all in `cmd/exec_test.go` but independent test functions)
- **US2 + US3**: Can be implemented in parallel with US1 (different files, no code dependencies)
- **All US2 tests** (T018-T022): Can be written in parallel (different test files)
- **All US3 tests** (T024-T026): Can be written in parallel
- **Polish tasks** (T029-T031): Can run in parallel

---

## Parallel Example: User Story 1

```text
# Phase 1+2 (all parallel — different files):
T001: exitCodeFromError() in internal/runner/exit.go
T002: exit_test.go
T003: buildChildEnv() in internal/runner/env.go
T004: env_test.go

# US1 Tests (all parallel — independent test functions):
T005: cobra registration test in cmd/exec_test.go
T006: happy path test
T007: invalid scenario test
T008: missing command test
T009: exit code propagation test
T010: verification failure test
T011: idempotent cleanup test

# US1 Implementation (sequential):
T012: Register exec command → T013: Pre-spawn validation → T014: Setup
→ T015: Spawn+wait → T016: Verify+cleanup → T017: Edge cases
```

---

## Parallel Example: All User Stories

```text
# After Phase 2 completes, all three stories can proceed in parallel:

Developer A (US1):  T005-T011 → T012-T017
Developer B (US2):  T018-T022 → T023
Developer C (US3):  T024-T026 → T027-T028
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (`exitCodeFromError` helper)
2. Complete Phase 2: Foundational (`buildChildEnv` helper)
3. Complete Phase 3: User Story 1 (exec command)
4. **STOP and VALIDATE**: Run `cli-replay exec scenario.yaml -- ./test.sh` end-to-end
5. Ship as MVP — exec mode is the highest-impact feature

### Incremental Delivery

1. Setup + Foundational → helpers ready
2. Add User Story 1 → Test exec mode independently → Ship (MVP!)
3. Add User Story 2 → Verify session isolation with tests → Ship
4. Add User Story 3 → Trap auto-cleanup for eval pattern → Ship
5. Each story adds value without breaking previous stories

---

## Notes

- **No changes to `internal/runner/state.go`** — State struct and StateFilePath are already correct
- **No changes to `internal/runner/replay.go`** — ExecuteReplay is already session-aware
- **No changes to `main.go`** — intercept mode is unchanged
- `cmd/exec.go` reuses helpers from `cmd/run.go` (`extractCommands`, `createIntercept`, `generateSessionID`, `hashScenarioFile`, `checkAllowlist`)
- Per research.md R5: `verify` and `clean` already handle `CLI_REPLAY_SESSION` correctly via `StateFilePath()` — US2 is purely testing
- Per research.md R6: `clean` is already idempotent — US2 T022 confirms this with test coverage
- The `cmd/cli-replay/` alternate command tree does NOT currently have `emitShellSetup` — T028 evaluates whether it needs one added for eval support
