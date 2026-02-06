# Tasks: Workflow Recorder

**Input**: Design documents from `/specs/002-workflow-recorder/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/cli.md](contracts/cli.md)

**Tests**: Included - Feature follows TDD approach per constitution

**Organization**: Tasks grouped by user story to enable independent implementation and testing. MVP scope includes US1 (P1) and US2 (P2). US3 (P3) and US4 (P4) deferred to future iterations.

## Format: `- [ ] [ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1, US2) - only for story-specific tasks
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Project initialization and package structure

- [X] T001 Create `internal/recorder` package directory structure
- [X] T002 Create `cmd` package directory structure for Cobra commands
- [X] T003 [P] Create `testdata/recordings` directory for test fixtures

---

## Phase 2: Foundational

**Purpose**: Core infrastructure required before any user story implementation

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T004 Create RecordedCommand type in internal/recorder/command.go with fields (Timestamp, Argv, ExitCode, Stdout, Stderr)
- [X] T005 [P] Create RecordingEntry type in internal/recorder/log.go for JSONL parsing with JSON tags
- [X] T006 [P] Create SessionMetadata type in internal/recorder/session.go with Name, Description, RecordedAt fields
- [X] T007 Create RecordingSession type in internal/recorder/session.go with session lifecycle fields

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Basic Command Recording (Priority: P1) 🎯 MVP

**Goal**: Record a single command execution and generate valid YAML scenario file

**Independent Test**: Execute `cli-replay record --output demo.yaml -- kubectl get pods` and verify generated YAML is valid and can be replayed

### Tests for User Story 1 (Write FIRST, ensure they FAIL)

- [X] T008 [P] [US1] Unit test for RecordedCommand validation in internal/recorder/command_test.go (test argv non-empty, exit code 0-255)
- [X] T009 [P] [US1] Unit test for JSONL entry parsing in internal/recorder/log_test.go (valid JSON, required fields)
- [X] T010 [P] [US1] Unit test for SessionMetadata defaults in internal/recorder/session_test.go (auto-generated name format)
- [X] T011 [P] [US1] Unit test for shim script generation in internal/recorder/shim_test.go (bash script content, executable permissions)
- [X] T012 [P] [US1] Unit test for scenario conversion in internal/recorder/converter_test.go (RecordedCommand → scenario.Step mapping)
- [X] T013 [US1] Integration test for single command recording in cmd/record_test.go (end-to-end: execute command, parse JSONL, generate YAML, validate)

### Implementation for User Story 1

- [X] T014 [P] [US1] Implement RecordedCommand Validate() method in internal/recorder/command.go
- [X] T015 [P] [US1] Implement JSONL parsing logic in internal/recorder/log.go (ReadRecordingLog function)
- [X] T016 [P] [US1] Implement shim script generation in internal/recorder/shim.go (GenerateShim function with bash template)
- [X] T017 [US1] Implement RecordingSession New() constructor in internal/recorder/session.go (create temp dir, init log file)
- [X] T018 [US1] Implement RecordingSession SetupShims() method in internal/recorder/session.go (generate shims, set PATH)
- [X] T019 [US1] Implement RecordingSession Execute() method in internal/recorder/session.go (run command with modified PATH)
- [X] T020 [US1] Implement RecordingSession Finalize() method in internal/recorder/session.go (cleanup temp dir)
- [X] T021 [US1] Implement ConvertToScenario function in internal/recorder/converter.go (JSONL → Scenario with validation)
- [X] T022 [US1] Implement GenerateYAML function in internal/recorder/converter.go (Scenario → YAML file)
- [X] T023 [US1] Create Cobra root command in cmd/root.go (basic cobra setup)
- [X] T024 [US1] Create Cobra record subcommand in cmd/record.go (flags: --output required, --name, --description optional)
- [X] T025 [US1] Implement record command handler in cmd/record.go (parse flags, create session, execute, convert, write YAML)
- [X] T026 [US1] Add error handling and user-friendly messages in cmd/record.go (output path validation, command not found, etc.)
- [X] T027 [US1] Add godoc comments for all exported types and functions in internal/recorder package

**Checkpoint**: User Story 1 complete - basic single command recording works end-to-end

---

## Phase 4: User Story 2 - Multi-Step Workflow Recording (Priority: P2)

**Goal**: Record sequences of commands from shell scripts and preserve execution order

**Independent Test**: Record a bash script with 3+ commands, verify YAML contains all steps in correct order, and replay executes sequentially

### Tests for User Story 2 (Write FIRST, ensure they FAIL)

- [X] T028 [P] [US2] Unit test for multi-command JSONL parsing in internal/recorder/log_test.go (multiple entries, order preservation)
- [X] T029 [P] [US2] Unit test for duplicate command recording in internal/recorder/converter_test.go (same argv, different outputs → separate steps)
- [X] T030 [US2] Integration test for multi-step workflow in cmd/record_test.go (bash script with 3 commands, verify all captured)

### Implementation for User Story 2

- [X] T031 [US2] Enhance JSONL parsing to handle multiple entries in internal/recorder/log.go (ensure order preservation)
- [X] T032 [US2] Enhance converter to handle duplicate commands in internal/recorder/converter.go (create separate steps per FR-009b)
- [X] T033 [US2] Add integration test for shell script execution in cmd/record_test.go (test with bash -c 'cmd1 && cmd2 && cmd3')
- [X] T034 [US2] Update quickstart.md with multi-step workflow examples

**Checkpoint**: User Story 2 complete - multi-step workflows recording works, US1 still functional

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Quality improvements, documentation, and final validation

- [X] T035 [P] Add main.go entry point in project root (if not exists, calls cmd/root.go Execute())
- [X] T036 [P] Update README.md with record subcommand documentation and examples
- [X] T037 [P] Add usage examples in examples/recording-demo.sh
- [X] T038 Run golangci-lint and fix any warnings
- [X] T039 Run gofmt -s and goimports on all Go files
- [X] T040 Verify all tests pass with `go test ./...`
- [X] T041 Validate generated YAML scenarios with existing `cli-replay run` command
- [X] T042 Run quickstart.md validation (execute examples, verify outputs)
- [X] T043 Update Makefile with `make record-demo` target (if Makefile exists)
- [X] T044 Create release notes documenting new record subcommand

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational completion - MVP target
- **User Story 2 (Phase 4)**: Depends on Foundational completion - Can run in parallel with US1 if staffed, but recommended sequential for MVP simplicity
- **Polish (Phase 5)**: Depends on US1 and US2 completion

### User Story Dependencies

- **User Story 1 (P1)**: Independent - only requires Foundational phase
- **User Story 2 (P2)**: Independent - only requires Foundational phase (no dependency on US1)
- **User Story 3 (P3)**: DEFERRED - command filtering not in MVP
- **User Story 4 (P4)**: DEFERRED - metadata customization not in MVP (basic defaults in US1)

### Within Each User Story

**User Story 1 flow**:
1. Write all tests (T008-T013) → ensure they FAIL
2. Implement foundation types (T014-T017) → tests start passing
3. Implement session lifecycle (T018-T020) → more tests pass
4. Implement conversion (T021-T022) → converter tests pass
5. Implement CLI (T023-T026) → integration tests pass
6. Add documentation (T027) → complete

**User Story 2 flow**:
1. Write tests (T028-T030) → ensure they FAIL
2. Enhance parsers (T031-T032) → tests pass
3. Add integration tests (T033) → full workflow validated
4. Update docs (T034) → complete

### Parallel Opportunities

**Phase 1 (Setup)**: All 3 tasks [P] can run in parallel

**Phase 2 (Foundational)**: T005 and T006 can run in parallel

**User Story 1 Tests**: T008, T009, T010, T011, T012 can all run in parallel (different test files)

**User Story 1 Implementation**: T014, T015, T016 can run in parallel (different source files)

**User Story 2 Tests**: T028, T029, T030 can run in parallel

**Phase 5 (Polish)**: T035, T036, T037 can run in parallel

---

## Parallel Example: User Story 1 Implementation

```bash
# Launch foundational types in parallel:
Task T014: "Implement RecordedCommand Validate()" → internal/recorder/command.go
Task T015: "Implement JSONL parsing" → internal/recorder/log.go
Task T016: "Implement shim generation" → internal/recorder/shim.go

# Then sequentially (these depend on above):
Task T017: "RecordingSession constructor" → uses all above types
Task T018: "SetupShims method" → uses shim generation
Task T019: "Execute method" → uses session state
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2)

1. **Sprint 1**: Complete Phase 1 (Setup) + Phase 2 (Foundational)
   - Deliverable: Core types and structure ready
2. **Sprint 2**: Complete Phase 3 (User Story 1)
   - Deliverable: Basic single-command recording works
   - **VALIDATE**: Test independently, can demo simple recording
3. **Sprint 3**: Complete Phase 4 (User Story 2)
   - Deliverable: Multi-step workflows work
   - **VALIDATE**: Test independently, can demo complex workflows
4. **Sprint 4**: Complete Phase 5 (Polish)
   - Deliverable: Production-ready, documented, tested
   - **SHIP**: Release v0.2.0 with record subcommand

### Test-First Workflow

For each task marked with test:
1. Write test(s) that define expected behavior
2. Run tests → verify they FAIL with meaningful error
3. Implement minimal code to make tests PASS
4. Refactor while keeping tests GREEN
5. Commit

### Incremental Delivery

- After Phase 1+2: Can develop recorder logic independently
- After Phase 3: Can record single commands → early user feedback
- After Phase 4: Can record workflows → full MVP value
- After Phase 5: Can ship production release

### Parallel Team Strategy

With 2 developers:

1. **Both**: Complete Phase 1 + Phase 2 together (foundational work)
2. **Once Foundational complete**:
   - Developer A: Tests for US1 (T008-T013)
   - Developer B: Tests for US2 (T028-T030)
3. **Implementation**:
   - Developer A: US1 Implementation (T014-T027)
   - Developer B: US2 Implementation (T031-T034) - can start after US1 types exist
4. **Both**: Phase 5 polish tasks in parallel

---

## Success Criteria Validation

Tasks mapped to success criteria from spec.md:

- **SC-001** (record in <30s): Validated by integration tests T013, T030
- **SC-002** (100% output fidelity): Validated by T041 (replay validation)
- **SC-003** (50 commands): Performance test can be added to T030
- **SC-004** (<5% overhead): Can measure in integration tests
- **SC-005** (human-readable YAML): Manual review during T042
- **SC-006** (complex arguments 95%): Edge cases in T013, T030

---

## Deferred Features (Future Iterations)

**User Story 3 (P3) - Selective Command Recording**:
- Requires: `--command` flag filtering, shim generation per command
- Estimated: 5-8 tasks (tests + implementation)
- Dependencies: US1 complete

**User Story 4 (P4) - Metadata Customization**:
- Partially complete in US1 (basic --name, --description flags)
- Full customization: advanced metadata fields
- Estimated: 2-3 tasks
- Dependencies: US1 complete

---

## Notes

- All tasks follow test-first approach per cli-replay constitution
- [P] tasks use different files, can execute in parallel
- [US1]/[US2] labels enable story-focused development
- Constitution gates verified at each checkpoint
- Commit frequently (after each task or logical group)
- Stop at any checkpoint to validate story independently
- MVP scope: US1 + US2 only (US3, US4 future work)

---

## Task Count Summary

- **Total MVP tasks**: 44
- **Phase 1 (Setup)**: 3 tasks
- **Phase 2 (Foundational)**: 4 tasks
- **Phase 3 (US1)**: 20 tasks (7 tests + 13 implementation)
- **Phase 4 (US2)**: 7 tasks (3 tests + 4 implementation)
- **Phase 5 (Polish)**: 10 tasks
- **Parallelizable tasks**: 17 marked [P]
- **Estimated MVP delivery**: 3-4 sprints (2 weeks per sprint)
