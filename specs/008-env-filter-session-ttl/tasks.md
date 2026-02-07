# Tasks: Environment Variable Filtering & Session TTL

**Input**: Design documents from `/specs/008-env-filter-session-ttl/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/cli.md, quickstart.md

**Tests**: Not explicitly requested in the feature specification. Test tasks are included because the existing codebase follows a test-alongside-implementation pattern (`*_test.go` files exist for all packages).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Create the new package and extend the data model — shared infrastructure that all user stories depend on.

- [X] T001 Create `internal/envfilter/` package directory
- [X] T002 [P] Implement `IsDenied(name string, patterns []string) bool` and `IsExempt(name string) bool` using `path.Match` in `internal/envfilter/filter.go` (R2, FR-002, FR-003, FR-007)
- [X] T003 [P] Implement unit tests for glob matching, wildcards, exemptions, and edge cases in `internal/envfilter/filter_test.go`
- [X] T004 Add `DenyEnvVars []string` field to `Security` struct in `internal/scenario/model.go` (data-model.md, FR-001)
- [X] T005 Add `Session` struct with `TTL string` field and add `Session *Session` to `Meta` struct in `internal/scenario/model.go` (data-model.md, FR-011)
- [X] T006 Add validation: reject empty `DenyEnvVars` entries (FR-009) and validate TTL format via `time.ParseDuration` (FR-012) in `internal/scenario/model.go`
- [X] T007 Add unit tests for new model fields, YAML unmarshaling, and validation in `internal/scenario/model_test.go`

**Checkpoint**: New `envfilter` package is independently testable. Data model accepts `deny_env_vars` and `session.ttl` from YAML.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Extend the template system and state management with the core functions that user stories depend on.

**⚠️ CRITICAL**: No user story integration work can begin until this phase is complete.

- [X] T008 Implement `MergeVarsFiltered(vars map[string]string, denyPatterns []string) (map[string]string, []string)` in `internal/template/render.go` — returns the filtered map AND a slice of denied variable names. Suppresses denied env var overrides using `envfilter.IsDenied`, preserves original `meta.vars` defaults (R1, D1, FR-004, FR-005)
- [X] T009 [P] Add `WriteDeniedEnvTrace(varName string)` function to `internal/runner/trace.go` that emits `cli-replay[trace]: denied env var <name>` when `CLI_REPLAY_TRACE=1` (FR-010)
- [X] T010 Add trace integration in caller: after calling `MergeVarsFiltered`, iterate the returned denied-names slice and call `WriteDeniedEnvTrace` for each in `internal/runner/replay.go` (FR-010)
- [X] T011 Add unit tests for `MergeVarsFiltered` — deny patterns, glob matching, exemptions, backward compat (no patterns = no filtering) in `internal/template/render_test.go`
- [X] T012 Implement `CleanExpiredSessions(cliReplayDir string, ttl time.Duration) (int, error)` in `internal/runner/state.go` that scans `cli-replay-*.state` files, checks `last_updated` vs TTL, removes expired state files + intercept dirs (R3, R4, FR-013, FR-014, FR-015, FR-018)
- [X] T013 Add unit tests for `CleanExpiredSessions` — expired removal, active preservation, future timestamps, idempotency, empty directory in `internal/runner/state_test.go`

**Checkpoint**: `MergeVarsFiltered` and `CleanExpiredSessions` are independently testable. All core building blocks ready.

---

## Phase 3: User Story 1 — Deny Environment Variables in Untrusted Scenarios (Priority: P1) 🎯 MVP

**Goal**: Prevent scenario templates from reading sensitive host environment variables when `deny_env_vars` is configured.

**Independent Test**: Create a scenario with `deny_env_vars` configured and verify denied variables resolve to empty strings while allowed variables render normally.

### Implementation for User Story 1

- [X] T014 [US1] Update `ReplayResponseWithTemplate` in `internal/runner/replay.go` to pass `scn.Meta.Security.DenyEnvVars` to `MergeVarsFiltered` instead of `MergeVars` (FR-004, FR-006)
- [X] T015 [US1] Handle nil `Security` and nil/empty `DenyEnvVars` gracefully — fall back to existing `MergeVars` behavior in `internal/runner/replay.go` (FR-008)
- [X] T016 [P] [US1] Add `deny_env_vars` to the `security` definition in `schema/scenario.schema.json` (contracts/cli.md)
- [X] T017 [US1] Add integration tests: denied var → empty string, allowed var → real value, no security section → passthrough, wildcard `*` deny-all, glob patterns `AWS_*` in `internal/runner/replay_test.go`

**Checkpoint**: User Story 1 fully functional. Denied env vars resolve to empty strings. Existing scenarios without `deny_env_vars` work identically.

---

## Phase 4: User Story 2 — Session TTL for Automatic Stale Session Cleanup (Priority: P1)

**Goal**: Automatically clean up stale replay sessions on `run`/`exec` startup when `meta.session.ttl` is configured.

**Independent Test**: Create a state file with an old `last_updated` timestamp, configure TTL, run a replay command, and verify the stale session is auto-cleaned.

### Implementation for User Story 2

- [X] T018 [US2] Add TTL cleanup call at session startup in `cmd/run.go` — read `meta.session.ttl`, call `CleanExpiredSessions` before initializing new session (FR-013, FR-014)
- [X] T019 [US2] Add TTL cleanup call at session startup in `cmd/exec.go` — same logic as `run` (FR-013)
- [X] T020 [US2] Add TTL cleanup call in intercept shim path in `internal/runner/replay.go` — read scenario TTL, call cleanup before matching (FR-013)
- [X] T021 [US2] Emit cleanup summary to stderr: `cli-replay: cleaned N expired sessions` when N > 0 in the TTL cleanup callers (FR-019)
- [X] T022 [P] [US2] Add `session` object with `ttl` property to the `meta` definition in `schema/scenario.schema.json` (contracts/cli.md)
- [X] T023 [US2] Add integration tests: stale session cleaned on run, active session preserved, no TTL configured → no cleanup, summary message emitted in `internal/runner/replay_integration_test.go`

**Checkpoint**: User Story 2 fully functional. Stale sessions auto-cleaned at startup. No change to sessions without TTL.

---

## Phase 5: User Story 3 — Proactive Cleanup on `cli-replay clean` with TTL Awareness (Priority: P2)

**Goal**: Allow `cli-replay clean --ttl <duration>` and `--recursive` for cron-driven bulk cleanup of expired sessions.

**Independent Test**: Create multiple `.cli-replay/` directories with expired state files, run `cli-replay clean --ttl 10m --recursive .`, verify all expired sessions removed.

### Implementation for User Story 3

- [X] T024 [US3] Add `--ttl` string flag to `clean` command in `cmd/clean.go` — parse as `time.Duration`, pass to `CleanExpiredSessions` (FR-016)
- [X] T025 [US3] Add `--recursive` bool flag to `clean` command in `cmd/clean.go` — require `--ttl` when set, error if `--recursive` used without `--ttl` (FR-020, D4)
- [X] T026 [US3] Implement recursive directory walk using `filepath.WalkDir` in `cmd/clean.go` — discover `.cli-replay/` dirs, call `CleanExpiredSessions` for each, skip `.git`/`node_modules` (R5)
- [X] T027 [US3] Update `clean` stderr output for TTL mode: `cli-replay: scanned N directories, cleaned M expired sessions` and `cli-replay: no expired sessions found` (contracts/cli.md)
- [X] T028 [US3] Add tests for `--ttl` flag, `--recursive` flag, recursive-requires-ttl validation, and multi-directory cleanup in `cmd/clean_test.go`

**Checkpoint**: All three user stories complete. `clean --ttl --recursive` works for bulk cleanup.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Edge cases, documentation, and final validation across all stories.

- [X] T029 [P] Handle edge case: `last_updated` in the future → warn to stderr, treat as active in `internal/runner/state.go` (Edge Case 1)
- [X] T030 [P] Handle edge case: file permission errors during cleanup → warn to stderr, continue processing in `internal/runner/state.go` (Edge Case 3)
- [X] T031 [P] Add test scenarios to `testdata/scenarios/` — `deny_env_vars.yaml` and `session_ttl.yaml` for manual validation
- [X] T032 [P] Add benchmark test: create 50 state files, time `CleanExpiredSessions`, assert < 2s in `internal/runner/state_test.go` (SC-003)
- [X] T033 [P] Add composability test: scenario with both `deny_env_vars` and `session.ttl` configured — verify both features work without interference in `internal/runner/replay_test.go` (SC-006)
- [X] T034 Run `go vet ./...` and `go test ./...` to verify zero regressions (SC-002, SC-004)
- [X] T035 Run quickstart.md validation steps end-to-end on Windows (quickstart.md)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 completion — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Phase 2 (`MergeVarsFiltered`)
- **User Story 2 (Phase 4)**: Depends on Phase 2 (`CleanExpiredSessions`)
- **User Story 3 (Phase 5)**: Depends on Phase 2 (`CleanExpiredSessions`) — can run in parallel with US1/US2
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2 — No dependencies on US2 or US3
- **User Story 2 (P1)**: Can start after Phase 2 — No dependencies on US1 or US3
- **User Story 3 (P2)**: Can start after Phase 2 — No dependencies on US1 or US2

### Within Each Phase

- Tasks marked **[P]** within the same phase can run in parallel
- Non-[P] tasks should be executed in order listed
- Integration tests come after implementation tasks within each story

### Parallel Opportunities

**Phase 1** — Three parallel tracks:
```
Track A: T002 (filter.go) + T003 (filter_test.go)
Track B: T004 + T005 + T006 (model.go changes) → T007 (model_test.go)
```

**Phase 2** — Two parallel tracks after T008:
```
Track A: T008 (MergeVarsFiltered) → T010 (trace integration) → T011 (render tests)
Track B: T009 (trace.go)  [parallel with T008]
Track C: T012 (CleanExpiredSessions) → T013 (state tests)  [parallel with Track A]
```

**Phase 3+4+5** — All three stories can start in parallel after Phase 2:
```
Track A: T014-T017 (User Story 1 — deny env vars)
Track B: T018-T023 (User Story 2 — session TTL)
Track C: T024-T028 (User Story 3 — clean --ttl --recursive)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001–T007)
2. Complete Phase 2: Foundational (T008–T013)
3. Complete Phase 3: User Story 1 (T014–T017)
4. **STOP and VALIDATE**: Run `go test ./...`, verify deny filtering works end-to-end
5. This is a deployable increment — env var security is functional

### Incremental Delivery

1. Setup + Foundational → Core building blocks ready
2. Add User Story 1 → Test independently → **MVP: env var filtering works**
3. Add User Story 2 → Test independently → **Increment: TTL auto-cleanup at startup**
4. Add User Story 3 → Test independently → **Increment: bulk cleanup via CLI**
5. Polish → Edge cases, docs, full regression suite
6. Each story adds value without breaking previous stories

### Single Developer (Sequential)

Execute phases 1 → 2 → 3 → 4 → 5 → 6 in order. Within each phase, execute [P] tasks first (they're independent), then dependent tasks.
