# Tasks: P2 Quality of Life Enhancements

**Input**: Design documents from `/specs/007-p2-quality-of-life/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (verify-json.md, verify-junit.md, scenario-schema.md), quickstart.md

**Tests**: Included alongside implementation per project convention. Written alongside each component.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- All file paths are relative to repository root

---

## Phase 1: Setup

**Purpose**: Confirm green baseline and create new package structure

- [x] T001 Run existing test suite with `go test ./...` and confirm all tests pass
- [x] T002 [P] Create `internal/verify/` package directory and `schema/` directory at repository root

---

## Phase 2: Foundational (Shared Prerequisites)

**Purpose**: Create shared verification result types used by US1 (formatters) and US3 (group-aware verification)

**Note**: US2 (JSON Schema) is fully independent of Go code and can start after Phase 1. US1 and US3 both build on the VerifyResult/StepResult types created here. The `group` field is included from the start with `omitempty` for forward compatibility.

- [x] T003 Create VerifyResult and StepResult types with BuildResult() constructor in internal/verify/result.go per data-model.md — fields: scenario, session, passed, total_steps, consumed_steps, error (omitempty), steps array with index, label, group (omitempty), call_count, min, max, passed
- [x] T004 [P] Write unit tests for VerifyResult construction (all-passed, incomplete, no-state-error, optional-step-with-min-zero scenarios) in internal/verify/result_test.go

**Checkpoint**: Shared result types ready — formatter and user story implementation can begin

---

## Phase 3: User Story 1 — Machine-Readable Verify Output (Priority: P1) 🎯 MVP

**Goal**: Add `--format json|junit` flag to `verify` and `--report-file <path>` + `--format` to `exec`, enabling CI pipelines to consume structured verification results without custom parsing.

**Independent Test**: Run `cli-replay verify scenario.yaml --format json | jq .` and confirm valid JSON; run `cli-replay verify scenario.yaml --format junit` and validate the XML structure.

### Implementation for User Story 1

- [x] T005 [P] [US1] Create JSON formatter function (FormatJSON) in internal/verify/json.go — compact JSON to io.Writer per contracts/verify-json.md, suppress human-readable output when active
- [x] T006 [P] [US1] Create JUnit XML formatter (FormatJUnit) with encoding/xml struct-based marshaling in internal/verify/junit.go — define JUnitTestSuites, JUnitTestSuite, JUnitTestCase, JUnitFailure, JUnitSkipped structs per contracts/verify-junit.md, emit XML header, handle failure/skipped/error states
- [x] T007 [P] [US1] Write unit tests for JSON formatter (all-passed output, incomplete steps, no-state error, group-prefixed labels round-trip, compact encoding) in internal/verify/json_test.go
- [x] T008 [P] [US1] Write unit tests for JUnit formatter (passed testcases, failure elements with message, skipped for min-zero uncalled, error state, XML validity, attribute mapping) in internal/verify/junit_test.go
- [x] T009 [US1] Refactor runVerify in cmd/verify.go — extract result-building logic into BuildResult() call, add --format flag (text|json|junit, case-insensitive via strings.ToLower), dispatch to FormatJSON/FormatJUnit for stdout or existing text output to stderr
- [x] T010 [US1] Add --report-file and --format flags to cmd/exec.go — write structured verification output to file path when --report-file set, fall back to stderr when --format set without --report-file, keep stdout reserved for child process
- [x] T011 [P] [US1] Add --format flag integration tests (json output parseable by encoding/json, junit output parseable by encoding/xml, text default unchanged, invalid format returns error listing valid values) in cmd/verify_test.go
- [x] T012 [P] [US1] Add --report-file and --format flag tests (file written at path, stderr fallback without --report-file, no structured output when --format omitted) in cmd/exec_test.go

**Checkpoint**: `verify --format json|junit` and `exec --report-file` work end-to-end. CI dashboards can consume output.

---

## Phase 4: User Story 2 — JSON Schema for Scenario Files (Priority: P2)

**Goal**: Publish a JSON Schema (draft-07) that enables IDE autocompletion, inline validation, and hover documentation for scenario YAML files, including step group syntax.

**Independent Test**: Add `# yaml-language-server: $schema=...` to an example YAML, open in VS Code with YAML extension, verify autocompletion for fields and typo detection.

### Implementation for User Story 2

- [x] T013 [US2] Create JSON Schema (draft-07) at schema/scenario.schema.json with definitions for meta, security, step, match, respond, calls, step_element (oneOf step/group_wrapper), step_group — include description and markdownDescription annotations on all fields, mutual exclusivity rules (stdout/stdout_file, stderr/stderr_file via allOf+if/then), enum constraints, range constraints (exit 0-255, min>=0, max>=1) per contracts/scenario-schema.md
- [x] T014 [P] [US2] Add `# yaml-language-server: $schema=...` modeline comment to example scenario files in examples/recordings/*.yaml and testdata/scenarios/*.yaml
- [x] T015 [P] [US2] Document JSON Schema URL pattern, per-file modeline syntax, and VS Code workspace settings configuration in README.md

**Checkpoint**: Schema validates all example scenarios without false positives. VS Code shows autocompletion and flags typos.

---

## Phase 5: User Story 3 — Step Groups with Unordered Matching (Priority: P3)

**Goal**: Allow scenarios to declare unordered groups of steps that can be consumed in any order, with barrier semantics preventing advancement past the group until all step minimums are met.

**Independent Test**: Create a scenario with an unordered group of 3 steps followed by an ordered step. Run a script calling the 3 in reversed order. Verify all steps consumed and verification passes.

### Model & Loader Changes

- [x] T016 [US3] Add StepElement union type (Step *Step | Group *StepGroup) with Validate() and StepGroup struct (Mode, Name, Steps) with Validate() to internal/scenario/model.go — enforce: exactly one of Step/Group non-nil, no nested groups, non-empty group steps, mode must be "unordered", auto-generate name as "group-N" (1-based declaration order) when omitted
- [x] T017 [US3] Add FlatSteps() []Step and GroupRanges() []GroupRange methods to Scenario in internal/scenario/model.go — FlatSteps expands groups inline returning contiguous leaf steps, GroupRanges returns {Start, End, Name, TopIndex} per data-model.md flat index mapping
- [x] T018 [US3] Change Scenario.Steps field type from []Step to []StepElement in internal/scenario/model.go and update Scenario.Validate() to iterate StepElements
- [x] T019 [P] [US3] Write StepElement/StepGroup validation tests (valid group passes, empty group rejected, nested group rejected, unknown mode rejected, auto-naming sequential, explicit name preserved, exactly-one-field invariant) in internal/scenario/model_test.go
- [x] T020 [US3] Add custom UnmarshalYAML(*yaml.Node) for StepElement — scan mapping keys for "group" → decode as group wrapper, else decode as Step — and MarshalYAML for round-trip support in internal/scenario/loader.go per research R3
- [x] T021 [P] [US3] Write group YAML loading tests (group decodes correctly, plain step still decodes, mixed steps+groups, group with name, group without name, validation errors surface on Load) in internal/scenario/loader_test.go

### Consumer Migration

- [x] T022 [US3] Migrate all direct scn.Steps access to scn.FlatSteps() in cmd/verify.go, cmd/exec.go, cmd/run.go, cmd/cli-replay/verify.go, internal/runner/replay.go, and internal/runner/state.go — one-line change per call site, preserving existing behavior for step-only scenarios

### Engine & State Changes

- [x] T023 [US3] Add ActiveGroup *int field and group helper methods (isInGroup, currentGroupRange) to State in internal/runner/state.go — ActiveGroup is index into GroupRanges(), nil when outside a group, persisted in state JSON
- [x] T024 [P] [US3] Write ActiveGroup state tracking tests (enter group sets index, exit group clears to nil, persist and restore via JSON, nil when no groups) in internal/runner/state_test.go
- [x] T025 [US3] Add GroupMismatchError type with fields (Scenario, GroupName, GroupIndex, Candidates, CandidateArgv, Received) and Error() string method to internal/runner/errors.go per data-model.md
- [x] T026 [US3] Implement group-aware matching branch in ExecuteReplay in internal/runner/replay.go — compute GroupRanges from scenario, detect active group via findGroupContaining(), linear scan all unconsumed group steps for first match, increment StepCounts on match, force-advance when all maxes hit, soft-advance when all mins met and no match, emit GroupMismatchError when mins not met and no match per research R4
- [x] T027 [P] [US3] Write unordered group replay tests with order permutations (any-order matching succeeds, barrier blocks subsequent ordered step, call bounds tracked per step within group, group exhaustion advances, GroupMismatchError with candidate list, all-min-zero immediate advance, adjacent groups, group as first/last step) in internal/runner/replay_test.go

### Verification & Output

- [x] T028 [US3] Update VerifyResult building in cmd/verify.go to populate group field and prepend [group:name] prefix to label for steps inside groups — use GroupRanges() to determine group membership per flat index
- [x] T029 [P] [US3] Create example unordered group scenario in examples/recordings/step-group-demo.yaml with named group (pre-flight), 3 unordered steps with call bounds, and a post-group ordered step per quickstart.md

**Checkpoint**: Unordered groups work end-to-end. Scenarios with groups verify correctly in text, JSON, and JUnit output formats.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, backward compatibility validation, and end-to-end smoke testing

- [x] T030 [P] Update README.md with P2 feature documentation (--format flag usage, --report-file flag, step groups YAML syntax, JSON Schema setup instructions)
- [x] T031 Run all tests with `go test ./...` and verify backward compatibility — all existing scenarios in testdata/ and examples/ must work unchanged
- [x] T032 Run quickstart.md examples end-to-end to validate documented usage (JSON pipe to jq, JUnit output, exec --report-file, step group scenario)
- [x] T033 [P] Validate schema/scenario.schema.json against all example and testdata scenarios (examples/recordings/*.yaml, testdata/scenarios/*.yaml) — confirm zero false-positive validation errors per SC-005
- [x] T034 [P] Run a simple benchmark comparing `verify --format text` vs `--format json` on a multi-step scenario — confirm structured output adds < 5ms overhead per SC-006

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — creates shared VerifyResult/StepResult types
- **US1 (Phase 3)**: Depends on Foundational — builds JSON/JUnit formatters on result types
- **US2 (Phase 4)**: Depends on Setup only — standalone schema artifact, **can run in parallel with US1**
- **US3 (Phase 5)**: Depends on US1 completion — modifies cmd/verify.go and cmd/exec.go that US1 refactors
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Depends on Foundational (Phase 2) only — no dependencies on other stories
- **US2 (P2)**: Independent — no Go code dependencies, can start after Setup
- **US3 (P3)**: Soft dependency on US1 — shares cmd/verify.go (T009 refactors it, T028 extends it). Recommended: complete US1 before starting US3

### Within Each User Story

- Types/models before formatters/services
- Formatters before command wiring
- Core logic before consumer migration (US3)
- Implementation before integration tests

### Parallel Opportunities

**Within US1**:
- T005 + T006 (JSON and JUnit formatters — different files)
- T007 + T008 (formatter tests — different files)
- T011 + T012 (command tests — different files)

**Across Stories**:
- US1 and US2 can run fully in parallel (no shared files)

**Within US3**:
- T019 + T021 (model tests + loader tests — different files)
- T024 + T027 (state tests + replay tests — different files)

---

## Parallel Example: User Story 1

```bash
# After T003-T004 (Foundational) complete:

# Launch formatter implementations together:
Task T005: "Create JSON formatter in internal/verify/json.go"
Task T006: "Create JUnit formatter in internal/verify/junit.go"

# Launch formatter tests together:
Task T007: "Write JSON formatter tests in internal/verify/json_test.go"
Task T008: "Write JUnit formatter tests in internal/verify/junit_test.go"

# After T009-T010 (command wiring) complete:
Task T011: "Add verify format tests in cmd/verify_test.go"
Task T012: "Add exec report-file tests in cmd/exec_test.go"
```

## Parallel Example: User Story 3

```bash
# After T016-T018 (model types + field change) complete:
Task T019: "StepElement/StepGroup validation tests in model_test.go"
Task T021: "Group YAML parsing tests in loader_test.go"

# After T023, T025-T026 (engine + error changes) complete:
Task T024: "ActiveGroup state tests in state_test.go"
Task T027: "Unordered group replay tests in replay_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (result types)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: `verify --format json|junit` works, existing tests pass
5. Ship if ready — CI gets structured output immediately

### Incremental Delivery

1. Setup + Foundational → Result types ready
2. Add US1 → Test independently → Deploy (**MVP — CI structured output!**)
3. Add US2 → Test independently → Deploy (**schema — better authoring experience!**)
4. Add US3 → Test independently → Deploy (**groups — order-independent scenarios!**)
5. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: US1 (Machine-Readable Output)
   - Developer B: US2 (JSON Schema) — fully independent
3. After US1 complete:
   - Developer A: US3 (Step Groups)
4. All: Polish phase

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- Zero new external dependencies — all formatters use stdlib (`encoding/json`, `encoding/xml`)
- StepElement union type with custom UnmarshalYAML is the most complex change (T016-T020)
- FlatSteps() bridge minimizes migration effort: one-line change per consumer (T022)
- JSON Schema includes step group definitions from the start (forward-looking, not dependent on Go implementation)
- All `--format` flag values are case-insensitive per spec clarification
- Commit after each task or logical group of tasks
