# Tasks: P3/P4 Enhancements — Production Hardening, CI Ecosystem & Documentation

**Input**: Design documents from `/specs/010-p3-p4-enhancements/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

**Tests**: Test tasks are included for code-change user stories (US1, US2) since those are P1 correctness/reliability changes. Documentation-only stories (US4, US5, US6) and the external-repo story (US3) do not include test tasks.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Dependency updates and test fixture preparation

- [X] T001 Promote `golang.org/x/sys` from indirect to direct dependency in `go.mod` by adding `import "golang.org/x/sys/windows"` usage (run `go mod tidy` after Phase 3 implementation)
- [X] T002 [P] Create test fixture `testdata/scenarios/validate-valid.yaml` — a minimal valid scenario with `meta.name`, at least one step, valid `stdout_file` reference
- [X] T003 [P] Create test fixture `testdata/scenarios/validate-invalid.yaml` — scenario with multiple errors: empty `meta.name`, `calls.min > calls.max`, `stdout`/`stdout_file` both set, forward capture reference
- [X] T004 [P] Create test fixture `testdata/scenarios/validate-bad-yaml.yaml` — YAML with syntax errors (unclosed quotes, bad indentation)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: No shared foundational infrastructure is needed for this feature set. Each user story is self-contained:

- US1 modifies `cmd/exec_windows.go` + adds `internal/platform/jobobject_windows.go`
- US2 adds `cmd/validate.go` + reuses `internal/scenario/`
- US3 is a separate repository (no code changes here)
- US4, US5, US6 are documentation-only

**⚠️ No blocking phase**: All user stories can begin independently after Phase 1 (Setup).

**Checkpoint**: Test fixtures created — user story implementation can now begin in parallel

---

## Phase 3: User Story 1 — Windows Job Objects: Reliable Process Tree Termination (Priority: P1) 🎯 MVP

**Goal**: Replace single-process `Process.Kill()` with job-object-based process tree termination on Windows so that Ctrl+C kills all descendant processes (grandchildren, etc.)

**Independent Test**: On a Windows system, run `cli-replay exec scenario.yaml -- cmd /c "start /b ping -n 100 localhost & start /b ping -n 100 127.0.0.1"`, send Ctrl+C, and verify all child processes are terminated and the intercept directory is cleaned up.

### Tests for User Story 1

- [X] T005 [P] [US1] Create unit test file `internal/platform/jobobject_windows_test.go` with build tag `//go:build windows` — test `NewJobObject()` returns valid handle, test `Close()` releases handle, test `Terminate()` on empty job succeeds, test double-close is safe
- [X] T006 [P] [US1] Create integration test in `cmd/exec_test.go` (or `cmd/exec_windows_test.go` with build tag) — test that `setupSignalForwarding` returns a valid cleanup function, test fallback behavior when job creation is mocked to fail

### Implementation for User Story 1

- [X] T007 [US1] Create `internal/platform/jobobject_windows.go` with build tag `//go:build windows` — implement `JobObject` struct with `handle windows.Handle` and `assigned bool` fields, implement `NewJobObject()` (CreateJobObject + SetInformationJobObject with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE), implement `AssignProcess(pid int) error` (OpenProcess + AssignProcessToJobObject + CloseHandle), implement `Terminate(exitCode uint32) error`, implement `Close() error`
- [X] T008 [US1] Rewrite `setupSignalForwarding()` in `cmd/exec_windows.go` — create job object via `platform.NewJobObject()` before child spawn, set `childCmd.SysProcAttr.CreationFlags = CREATE_SUSPENDED` for race-free job assignment, after `childCmd.Start()` call `job.AssignProcess(childCmd.Process.Pid)` then `ResumeThread` to begin execution, on `os.Interrupt` call `job.Terminate(1)` instead of `Process.Kill()`, in cleanup function call `job.Close()`, if `NewJobObject()` fails log warning to stderr, remove `CREATE_SUSPENDED` flag, and fall back to current `Process.Kill()` behavior
- [X] T009 [US1] Verify `cmd/exec.go` defer/cleanup path correctly invokes the cleanup function returned by `setupSignalForwarding` — ensure job handle is closed even on normal child exit

**Checkpoint**: Windows job object process tree termination is functional. Unix behavior unchanged. Fallback to `Process.Kill()` on error.

---

## Phase 4: User Story 2 — `cli-replay validate`: Pre-Execution Validation (Priority: P1)

**Goal**: Expose existing `scenario.LoadFile()` + `Scenario.Validate()` as a standalone `cli-replay validate` command with multi-file support, JSON output, and no side effects.

**Independent Test**: Run `cli-replay validate testdata/scenarios/validate-valid.yaml` → exit 0 with success message. Run `cli-replay validate testdata/scenarios/validate-invalid.yaml` → exit 1 with all errors listed.

### Tests for User Story 2

- [X] T010 [P] [US2] Create `cmd/validate_test.go` — table-driven tests: valid file → exit 0 + success message, invalid file → exit 1 + all errors reported, YAML parse error → exit 1 + parse error message, file not found → exit 1 + file-not-found error, multiple files (mix valid/invalid) → exit 1 + per-file reporting, `--format json` → valid JSON array with `file`/`valid`/`errors` fields on stdout
- [X] T011 [P] [US2] Create test for `stdout_file`/`stderr_file` existence check — scenario with `stdout_file` referencing non-existent file → error in results, scenario with valid `stdout_file` → no error

### Implementation for User Story 2

- [X] T012 [US2] Create `cmd/validate.go` — define `ValidationResult` struct (`File string`, `Valid bool`, `Errors []string` with JSON tags), define `validateCmd` cobra command (`Use: "validate <file>..."`, `Short`, `Long` descriptions per contracts/cli.md, `Args: cobra.MinimumNArgs(1)`, `RunE: runValidate`), register `--format` flag (`text` default, `json`), register command in `init()` via `rootCmd.AddCommand(validateCmd)`
- [X] T013 [US2] Implement `runValidate()` in `cmd/validate.go` — validate `--format` flag (text or json, error on invalid), iterate over `args` file paths, for each file call `validateFile(path)`, collect `[]ValidationResult`, output text format to stderr or JSON format to stdout, exit 1 if any file has errors
- [X] T014 [US2] Implement `validateFile(path string) ValidationResult` in `cmd/validate.go` — resolve absolute path via `filepath.Abs()`, call `scenario.LoadFile(absPath)`, if error → return `ValidationResult{File: path, Valid: false, Errors: [err.Error()]}`, if success → check `stdout_file`/`stderr_file` existence relative to scenario directory, build and return result
- [X] T015 [US2] Implement text output formatter — success: `✓ <file>: valid` to stderr, failure: `✗ <file>:` followed by `  - <error>` lines to stderr, summary line: `Result: N/M files valid`
- [X] T016 [US2] Implement JSON output formatter — encode `[]ValidationResult` to stdout via `json.NewEncoder(os.Stdout).Encode(results)`, use `json.MarshalIndent` for human-readable output

**Checkpoint**: `cli-replay validate` command is functional with text and JSON output. All 18 validation rules enforced. No side effects produced.

---

## Phase 5: User Story 3 — Official GitHub Action (Priority: P2)

**Goal**: Create a reusable composite GitHub Action in a separate repository (`ormasoftchile/cli-replay-action`) that downloads, installs, and invokes cli-replay for CI workflows.

**Independent Test**: In a GitHub Actions workflow, add `uses: ormasoftchile/cli-replay-action@v1` with a scenario and test command. Verify on `ubuntu-latest`, `macos-latest`, and `windows-latest`.

**Note**: This user story targets a **separate repository** (`ormasoftchile/cli-replay-action`). Tasks below create the action definition files locally for review/transfer.

### Implementation for User Story 3

- [X] T017 [US3] Create `action.yml` definition file per contracts/action.md — composite action with `using: "composite"`, 7 inputs (scenario, run, version, format, report-file, validate-only, allowed-commands), 1 output (cli-replay-version), install step with platform detection (`runner.os` → GOOS, `runner.arch` → GOARCH), binary download from GitHub Releases, PATH setup via `$GITHUB_PATH`, run step with conditional exec/validate/setup-only modes
- [X] T018 [P] [US3] Create `README.md` for the action repository — usage examples (basic, JUnit reporting, cross-platform matrix, validate-only, pinned version, allowed-commands), supported platforms table, input/output reference, versioning policy
- [X] T019 [P] [US3] Create `.github/workflows/test.yml` for the action repository — test matrix across `ubuntu-latest`, `macos-latest`, `windows-latest`, test install step, test exec mode, test validate-only mode

**Checkpoint**: GitHub Action definition is complete and ready to be pushed to `ormasoftchile/cli-replay-action` repository.

---

## Phase 6: User Story 4 — SECURITY.md: Threat Model and Trust Boundaries (Priority: P2)

**Goal**: Create centralized security documentation covering trust model, threat boundaries, security controls, and vulnerability disclosure process.

**Independent Test**: Verify `SECURITY.md` exists at repo root, covers all FR-022–FR-027 sections, and is linked from README.

### Implementation for User Story 4

- [X] T020 [US4] Create `SECURITY.md` at repository root — Security Policy section (supported versions table), Reporting a Vulnerability section (email contact, GitHub Security Advisory link, response SLA), Trust Model section (scenarios are trusted code, review like source files), Threat Boundaries section (PATH interception mechanism + `allowed_commands` mitigation, environment variable leaking + `deny_env_vars` mitigation, session isolation boundaries, recording mode trust), Security Controls section (`allowed_commands`, `deny_env_vars`, `KnownFields(true)` strict parsing, session TTL), Known Limitations section (deny_env_vars is pattern-based not content scanning, shim scripts run as current user), Recommendations section (review scenarios in PRs, restrict allowed_commands in production, use deny_env_vars for secrets)
- [X] T021 [US4] Update `README.md` to link to `SECURITY.md` from existing security-related sections (allowlist section, deny_env_vars section)

**Checkpoint**: Security documentation is complete. GitHub will automatically surface `SECURITY.md` in the repository Security tab.

---

## Phase 7: User Story 5 — Scenario Cookbook (Priority: P3)

**Goal**: Provide annotated, copy-paste-ready scenario examples for Terraform, Helm, and kubectl workflows with a decision matrix.

**Independent Test**: For each cookbook example, run `cli-replay validate <example>.yaml` to verify validity, then `cli-replay exec <example>.yaml -- bash <script>.sh` to verify end-to-end execution.

### Implementation for User Story 5

- [X] T022 [US5] Create `examples/cookbook/README.md` — decision matrix table (use case → example → key features), overview of cookbook purpose, links to each scenario, quick-start instructions
- [X] T023 [P] [US5] Create `examples/cookbook/terraform-workflow.yaml` — scenario intercepting `terraform init`, `terraform plan`, `terraform apply` with realistic multi-line JSON output, inline YAML comments explaining features used (linear steps, exit codes, multi-line stdout), `meta.security.allowed_commands: ["terraform"]`
- [X] T024 [P] [US5] Create `examples/cookbook/terraform-test.sh` — companion test script that runs `terraform init && terraform plan -out=plan.tfplan && terraform apply plan.tfplan`
- [X] T025 [P] [US5] Create `examples/cookbook/helm-deployment.yaml` — scenario intercepting `helm repo add`, `helm upgrade --install`, `helm status` with captures and template variables, inline YAML comments explaining captures and template features, `meta.security.allowed_commands: ["helm"]`
- [X] T026 [P] [US5] Create `examples/cookbook/helm-test.sh` — companion test script that runs `helm repo add bitnami https://charts.bitnami.com/bitnami && helm upgrade --install myrelease bitnami/nginx && helm status myrelease`
- [X] T027 [P] [US5] Create `examples/cookbook/kubectl-pipeline.yaml` — scenario demonstrating `kubectl apply`, `kubectl rollout status` (with `calls.min`/`calls.max` for polling), `kubectl get pods` verification, using step groups (unordered mode), dynamic captures, `meta.security.allowed_commands: ["kubectl"]`, inline YAML comments explaining groups, call bounds, captures
- [X] T028 [P] [US5] Create `examples/cookbook/kubectl-test.sh` — companion test script running multi-step kubectl deployment pipeline

**Checkpoint**: All three cookbook examples pass `cli-replay validate`. Decision matrix links all examples.

---

## Phase 8: User Story 6 — Performance Benchmark Documentation (Priority: P4)

**Goal**: Document baseline benchmark numbers, establish regression thresholds, and provide contributor guidance for running and interpreting benchmarks.

**Independent Test**: Run `go test -bench=. -benchmem ./...` and verify all benchmarks complete. Verify `BENCHMARKS.md` exists with baseline numbers.

### Implementation for User Story 6

- [X] T029 [US6] Run all benchmarks locally via `go test -bench=. -benchmem -count=5 ./internal/matcher/ ./internal/runner/ ./internal/verify/` and capture baseline numbers (ns/op, B/op, allocs/op) for each benchmark
- [X] T030 [US6] Create `BENCHMARKS.md` at repository root — Overview section (purpose, how to run, reference system specs), Benchmark Results table (each benchmark with baseline ns/op, B/op, allocs/op from T029), Threshold Policy section (2× baseline = warning, 4× baseline = regression per spec), Contributing section (how to add benchmarks, naming convention `Benchmark<Area>_<Scale>`, how to compare with `benchstat`), CI Guidance section (how to integrate benchmark checks in PR workflow)

**Checkpoint**: Benchmark baselines documented. Contributors have clear guidance for regression detection.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Final integration, cross-story validation, and cleanup

- [X] T031 Run `go mod tidy` to finalize `go.mod` / `go.sum` after all code changes
- [X] T032 [P] Run `go vet ./...` and `go build ./...` to verify no compilation errors across all platforms (`GOOS=windows`, `GOOS=linux`, `GOOS=darwin`)
- [X] T033 [P] Run full test suite `go test ./...` to verify no regressions
- [X] T034 [P] Validate all cookbook examples with `cli-replay validate examples/cookbook/*.yaml`
- [X] T035 Run quickstart.md scenarios end-to-end to verify documented examples work
- [X] T036 [P] Review all new files for consistent code style, Go doc comments, and inline documentation

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Skipped — no blocking infrastructure needed
- **User Story 1 (Phase 3)**: Depends on T001 (go.mod update) — Windows-only code
- **User Story 2 (Phase 4)**: Depends on T002–T004 (test fixtures) — cross-platform code
- **User Story 3 (Phase 5)**: No code dependencies — separate repository; can start after US2 is complete (action uses `validate` command)
- **User Story 4 (Phase 6)**: No code dependencies — documentation only; can start anytime
- **User Story 5 (Phase 7)**: No code dependencies — documentation only; best started after US2 (validate used for testing cookbook)
- **User Story 6 (Phase 8)**: No code dependencies — documentation + benchmark capture; can start anytime
- **Polish (Phase 9)**: Depends on all code-change stories (US1, US2) being complete

### User Story Dependencies

- **US1** (Windows Job Objects) ← independent, only needs `go.mod` update from Setup
- **US2** (validate command) ← independent, only needs test fixtures from Setup
- **US3** (GitHub Action) ← soft dependency on US2 (action uses `validate` command); can scaffold action before US2 is complete
- **US4** (SECURITY.md) ← fully independent, documentation only
- **US5** (Cookbook) ← soft dependency on US2 (cookbook validated with `validate` command)
- **US6** (Benchmarks) ← fully independent, documentation only

### Within Each User Story

- Tests written before implementation (US1, US2)
- Platform abstraction (JobObject) before command integration (exec_windows.go)
- Core logic (validateFile) before formatters (text/json output)
- Scenario files before companion scripts (cookbook)

### Parallel Opportunities

**Phase 1** — T002, T003, T004 can all run in parallel (different fixture files)

**Phase 3 (US1)** — T005 and T006 can run in parallel (different test files)

**Phase 4 (US2)** — T010 and T011 can run in parallel (different test aspects)

**Phase 5 (US3)** — T018 and T019 can run in parallel (different files)

**Phase 7 (US5)** — T023, T024, T025, T026, T027, T028 can all run in parallel (independent files)

**Cross-story parallelism** — US1 and US2 can run in parallel (different files, no shared code). US4, US5, US6 can all run in parallel with each other and with US1/US2.

---

## Parallel Example: User Story 2

```
# Phase 1 test fixtures (parallel):
T002: testdata/scenarios/validate-valid.yaml
T003: testdata/scenarios/validate-invalid.yaml
T004: testdata/scenarios/validate-bad-yaml.yaml

# Then US2 tests (parallel):
T010: cmd/validate_test.go
T011: cmd/validate_test.go (stdout_file check tests)

# Then US2 implementation (sequential):
T012: cmd/validate.go (struct + command definition)
T013: cmd/validate.go (runValidate core logic)
T014: cmd/validate.go (validateFile helper)
T015: cmd/validate.go (text formatter)
T016: cmd/validate.go (JSON formatter)
```

---

## Implementation Strategy

### MVP First (User Story 1 + User Story 2)

1. Complete Phase 1: Setup (test fixtures + go.mod)
2. Complete Phase 3: User Story 1 — Windows Job Objects (P1)
3. Complete Phase 4: User Story 2 — validate command (P1)
4. **STOP and VALIDATE**: Both P1 stories independently testable
5. Run `go test ./...` — all tests pass, no regressions

### Incremental Delivery

1. Setup (Phase 1) → fixtures ready
2. US1 (Phase 3) → Windows process tree termination works → **Test on Windows CI**
3. US2 (Phase 4) → validate command works → **Test: `cli-replay validate examples/*.yaml`**
4. US3 (Phase 5) → GitHub Action ready → **Test in separate repo workflow**
5. US4 (Phase 6) → SECURITY.md published → **Review: GitHub Security tab**
6. US5 (Phase 7) → Cookbook published → **Test: validate + exec all examples**
7. US6 (Phase 8) → Benchmarks documented → **Test: run benchmarks, compare baselines**
8. Polish (Phase 9) → Final validation and cleanup

### Parallel Strategy

With capacity for parallel work:

1. All complete Setup together
2. Once Setup is done, in parallel:
   - Track A: US1 (Windows Job Objects) + US2 (validate command)
   - Track B: US4 (SECURITY.md) + US5 (Cookbook) + US6 (Benchmarks)
3. After US2 complete: US3 (GitHub Action)
4. After all code stories: Phase 9 (Polish)

---

## Notes

- US3 (GitHub Action) files are created locally but belong in `ormasoftchile/cli-replay-action` repository
- US1 code is Windows-only (`//go:build windows`); tests require Windows CI runner or manual testing
- `golang.org/x/sys` v0.19.0 is already an indirect dependency — promoting to direct adds no new module
- All 18 validation rules already exist in `internal/scenario/`; US2 only adds a command entry point
- Cookbook scenarios use synthetic output; no real Terraform/Helm/kubectl installation needed
- Benchmark baselines are captured on the contributor's machine; thresholds are 2× baseline
