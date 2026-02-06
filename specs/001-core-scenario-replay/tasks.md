# Tasks: Core Scenario Replay

**Input**: Design documents from `/specs/001-core-scenario-replay/`
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓

**Tests**: Included per Constitution Principle I (Test-First Development - NON-NEGOTIABLE)

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4, US5)
- Exact file paths included in descriptions

## Path Conventions

Based on plan.md project structure:
- Entry point: `cmd/cli-replay/`
- Internal packages: `internal/scenario/`, `internal/matcher/`, `internal/runner/`, `internal/template/`
- Test data: `testdata/scenarios/`, `testdata/fixtures/`

---

## Phase 1: Setup

**Purpose**: Project initialization and tooling

- [X] T001 Create project structure: `cmd/cli-replay/`, `internal/scenario/`, `internal/matcher/`, `internal/runner/`, `internal/template/`, `testdata/`
- [X] T002 Initialize Go module with `go mod init github.com/YOUR_ORG/cli-replay`
- [X] T003 [P] Add dependencies: `go get github.com/spf13/cobra gopkg.in/yaml.v3 github.com/stretchr/testify`
- [X] T004 [P] Create Makefile with targets: `build`, `test`, `lint`, `fmt`
- [X] T005 [P] Create `.golangci.yml` with linter configuration

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure required by ALL user stories

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Tests for Foundational Phase

- [X] T006 [P] Write unit tests for Scenario/Step/Match/Response model validation in `internal/scenario/model_test.go`
- [X] T007 [P] Write unit tests for YAML loader with strict field validation in `internal/scenario/loader_test.go`
- [X] T008 [P] Write unit tests for state file operations (read/write/atomic) in `internal/runner/state_test.go`

### Implementation for Foundational Phase

- [X] T009 [P] Implement Scenario, Meta, Step, Match, Response structs in `internal/scenario/model.go`
- [X] T010 [P] Implement State struct with JSON serialization in `internal/runner/state.go`
- [X] T011 Implement YAML loader with KnownFields(true) strict parsing in `internal/scenario/loader.go`
- [X] T012 Implement state file persistence (atomic write via rename) in `internal/runner/state.go`
- [X] T013 [P] Create test scenario fixtures in `testdata/scenarios/single_step.yaml`, `testdata/scenarios/multi_step.yaml`
- [X] T014 [P] Create test fixture files in `testdata/fixtures/pods_output.txt`

**Checkpoint**: Foundation ready - models load/validate, state persists correctly

---

## Phase 3: User Story 1 - Run Single-Step CLI Scenario (Priority: P1) 🎯 MVP

**Goal**: Match a single CLI command and return predetermined stdout/stderr/exit code

**Independent Test**: Symlink cli-replay as `kubectl`, invoke `kubectl get pods`, verify stdout matches scenario

### Tests for User Story 1

> **TDD: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T015 [P] [US1] Write unit tests for argv matching in `internal/matcher/argv_test.go`
- [X] T016 [P] [US1] Write unit tests for response output (stdout, stderr, exit) in `internal/runner/replay_test.go`
- [X] T017 [P] [US1] Write unit tests for stdout_file/stderr_file loading in `internal/runner/replay_test.go`
- [X] T018 [US1] Write integration test for single-step scenario execution in `internal/runner/replay_integration_test.go`

### Implementation for User Story 1

- [X] T019 [US1] Implement strict argv matcher in `internal/matcher/argv.go`
- [X] T020 [US1] Implement response output (write stdout/stderr, return exit code) in `internal/runner/replay.go`
- [X] T021 [US1] Implement stdout_file/stderr_file content loading in `internal/runner/replay.go`
- [X] T022 [US1] Implement intercept mode detection (argv[0] check) in `cmd/cli-replay/main.go`
- [X] T023 [US1] Implement CLI_REPLAY_SCENARIO environment variable reading in `cmd/cli-replay/main.go`
- [X] T024 [US1] Wire up intercept mode: load scenario → match → respond → exit in `cmd/cli-replay/main.go`

**Checkpoint**: Single-step scenario works end-to-end via symlink

---

## Phase 4: User Story 2 - Run Multi-Step Ordered Scenario (Priority: P1) 🎯 MVP

**Goal**: Track state across invocations, enforce strict step ordering

**Independent Test**: Create 3-step scenario, invoke in order A→B→C, verify each returns correct response; invoke out-of-order, verify error

### Tests for User Story 2

> **TDD: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T025 [P] [US2] Write unit tests for state advancement (current_step increment) in `internal/runner/state_test.go`
- [X] T026 [P] [US2] Write unit tests for step ordering enforcement in `internal/runner/replay_test.go`
- [X] T027 [US2] Write integration test for multi-step scenario in order in `internal/runner/replay_integration_test.go`
- [X] T028 [US2] Write integration test for out-of-order rejection in `internal/runner/replay_integration_test.go`

### Implementation for User Story 2

- [X] T029 [US2] Implement state file path calculation (hash of scenario path) in `internal/runner/state.go`
- [X] T030 [US2] Implement state loading on intercept (read current_step) in `internal/runner/replay.go`
- [X] T031 [US2] Implement state advancement after successful match in `internal/runner/replay.go`
- [X] T032 [US2] Implement `cli-replay run` command (initialize state) in `cmd/cli-replay/run.go`
- [X] T033 [US2] Implement `cli-replay init` command (reset state) in `cmd/cli-replay/init.go`
- [X] T034 [US2] Implement `cli-replay verify` command (check all steps consumed) in `cmd/cli-replay/verify.go`
- [X] T035 [US2] Wire up cobra root command with subcommands in `cmd/cli-replay/main.go`

**Checkpoint**: Multi-step scenarios work with state persistence; verify command confirms completion

---

## Phase 5: User Story 3 - Fail Fast on Unexpected Commands (Priority: P2)

**Goal**: Clear error messages with received vs expected argv, scenario name, step index

**Independent Test**: Invoke command that doesn't match expected step, verify error format on stderr

### Tests for User Story 3

> **TDD: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T036 [P] [US3] Write unit tests for error message formatting in `internal/runner/errors_test.go`
- [X] T037 [US3] Write integration test for mismatch error output in `internal/runner/replay_integration_test.go`

### Implementation for User Story 3

- [X] T038 [US3] Define MismatchError type with structured fields in `internal/runner/errors.go`
- [X] T039 [US3] Implement error message formatting (received, expected, scenario, step index) in `internal/runner/errors.go`
- [X] T040 [US3] Integrate error formatting into replay failure path in `internal/runner/replay.go`

**Checkpoint**: Mismatch errors show all diagnostic information

---

## Phase 6: User Story 4 - Template Variables in Responses (Priority: P2)

**Goal**: Render Go text/template syntax in stdout/stderr with vars from meta and environment

**Independent Test**: Define `meta.vars.cluster`, use `{{ .cluster }}` in stdout, verify rendered output

### Tests for User Story 4

> **TDD: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T041 [P] [US4] Write unit tests for template rendering in `internal/template/render_test.go`
- [X] T042 [P] [US4] Write unit tests for variable merging (env overrides vars) in `internal/template/render_test.go`
- [X] T043 [P] [US4] Write unit tests for missing variable error in `internal/template/render_test.go`
- [X] T044 [US4] Write integration test for templated response output in `internal/runner/replay_integration_test.go`

### Implementation for User Story 4

- [X] T045 [US4] Implement template context builder (merge vars + env) in `internal/template/render.go`
- [X] T046 [US4] Implement text/template rendering with missingkey=error in `internal/template/render.go`
- [X] T047 [US4] Integrate template rendering into response output in `internal/runner/replay.go`

**Checkpoint**: Templated responses render correctly; missing vars produce clear errors

---

## Phase 7: User Story 5 - Trace Mode for Debugging (Priority: P3)

**Goal**: When CLI_REPLAY_TRACE=1, output step index, argv, response, exit code to stderr

**Independent Test**: Set CLI_REPLAY_TRACE=1, invoke command, verify trace output on stderr

### Tests for User Story 5

> **TDD: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T049 [P] [US5] Write unit tests for trace output formatting in `internal/runner/trace_test.go`
- [X] T050 [US5] Write integration test for trace mode output in `internal/runner/replay_integration_test.go`

### Implementation for User Story 5

- [X] T051 [US5] Implement trace output formatter in `internal/runner/trace.go`
- [X] T052 [US5] Implement CLI_REPLAY_TRACE environment check in `internal/runner/replay.go`
- [X] T053 [US5] Integrate trace output at match points in `internal/runner/replay.go`

**Checkpoint**: Trace mode shows all diagnostic information; disabled by default

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, build, and final quality gates

- [X] T054 [P] Implement `cli-replay --version` flag in `cmd/cli-replay/main.go`
- [X] T055 [P] Implement `cli-replay --help` with usage examples in `cmd/cli-replay/main.go`
- [X] T056 [P] Create README.md with problem statement, quickstart, scenario YAML explanation, limitations
- [X] T057 [P] Create example scenario in `examples/kubectl-simple.yaml`
- [X] T058 Run `go test -race -cover ./...` and verify ≥80% coverage
- [X] T059 Run `golangci-lint run` and fix any warnings
- [X] T060 Run `gofmt -s -w .` and `goimports -w .`
- [X] T061 Validate quickstart.md example works end-to-end
- [X] T062 Build static binaries: `CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/cli-replay ./cmd/cli-replay`

---

## Dependencies & Execution Order

### Phase Dependencies

```
Phase 1 (Setup) → Phase 2 (Foundational) → [US1, US2] → [US3, US4] → US5 → Phase 8 (Polish)
                                              ↑
                                         BLOCKS ALL
```

- **Phase 1 (Setup)**: No dependencies
- **Phase 2 (Foundational)**: Depends on Setup; **BLOCKS all user stories**
- **Phase 3 (US1)**: Depends on Foundational
- **Phase 4 (US2)**: Depends on Foundational; integrates with US1 replay loop
- **Phase 5 (US3)**: Depends on US1/US2 (error handling for matching)
- **Phase 6 (US4)**: Depends on US1 (template into response output)
- **Phase 7 (US5)**: Depends on US1/US2 (trace points in replay)
- **Phase 8 (Polish)**: Depends on all user stories

### User Story Independence

| Story | Can Start After | Independently Testable |
|-------|-----------------|------------------------|
| US1 (P1) | Phase 2 complete | ✅ Single-step via symlink |
| US2 (P1) | Phase 2 complete | ✅ Multi-step + verify command |
| US3 (P2) | US1 or US2 | ✅ Error message verification |
| US4 (P2) | US1 | ✅ Templated output verification |
| US5 (P3) | US1 or US2 | ✅ Trace output verification |

### Parallel Opportunities per Phase

**Phase 1**: T003, T004, T005 can run in parallel

**Phase 2**: T006, T007, T008 (tests) in parallel; T009, T010, T013, T014 in parallel

**Phase 3 (US1)**: T015, T016, T017 (tests) in parallel

**Phase 4 (US2)**: T025, T026 (tests) in parallel

**Phase 5 (US3)**: T036 can start while US2 completes

**Phase 6 (US4)**: T041, T042, T043 (tests) in parallel

**Phase 7 (US5)**: T049 can run in parallel with US4

**Phase 8**: T054, T055, T056, T057 all in parallel

---

## Implementation Strategy

### MVP (User Stories 1 + 2 Only)

1. Complete Phase 1: Setup (T001-T005)
2. Complete Phase 2: Foundational (T006-T014)
3. Complete Phase 3: User Story 1 (T015-T024)
4. Complete Phase 4: User Story 2 (T025-T035)
5. **STOP and VALIDATE**: Test multi-step scenario end-to-end
6. MVP deliverable: working cli-replay with ordered step matching

### Full Delivery

7. Add Phase 5: User Story 3 (T036-T040) — Better error messages
8. Add Phase 6: User Story 4 (T041-T048) — Template support
9. Add Phase 7: User Story 5 (T049-T053) — Trace mode
10. Complete Phase 8: Polish (T054-T062)

---

## Summary

| Phase | Tasks | Parallel | Story |
|-------|-------|----------|-------|
| Setup | T001-T005 | 3 | - |
| Foundational | T006-T014 | 6 | - |
| US1 (P1) | T015-T024 | 3 | Single-step |
| US2 (P1) | T025-T035 | 2 | Multi-step |
| US3 (P2) | T036-T040 | 1 | Error messages |
| US4 (P2) | T041-T047 | 3 | Templates |
| US5 (P3) | T049-T053 | 1 | Trace mode |
| Polish | T054-T062 | 4 | - |

**Total**: 61 tasks | **MVP**: 35 tasks (through US2)
