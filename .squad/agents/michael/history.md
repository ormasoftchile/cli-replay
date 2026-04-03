# Project Context

- **Owner:** Cristián
- **Project:** cli-replay — A Go framework for instrumenting tools/command calls from workflows/runbooks, enabling replay scenarios without faking from the consumer side.
- **Stack:** Go, CLI, GitHub Actions
- **Created:** 2026-04-03

## Learnings

<!-- Append new learnings below. Each entry is something lasting about the project. -->

### 2026-04-03 — Test Infrastructure Audit

- **Test count:** 37 test files, 486 test functions, 10 benchmark functions. All pass on Windows.
- **Coverage by package:**
  | Package | Coverage |
  |---------|----------|
  | envfilter | 100.0% |
  | matcher | 97.0% |
  | template | 95.7% |
  | verify | 93.2% |
  | platform | 87.4% |
  | scenario | 83.5% |
  | runner | 77.2% |
  | recorder | 72.7% |
  | cmd | 64.2% |
  | main (root) | 0.0% |
  | platform/testutil | 0.0% (test helper, intentional) |
- **Test style:** Mostly individual test functions; table-driven tests exist in `record_test.go` (TestExtractCommandName), `result_test.go` (TestStepLabel), and `argv_test.go`. Most tests are NOT table-driven.
- **Testing libraries:** `testify/assert` + `testify/require` used consistently. `testifylint` enabled in golangci.
- **No shared test utilities package.** Each package has local helpers (`makeExecRoot`, `createTestScenario`, `newTestPlatform`, etc.).
- **FakePlatform** in `internal/platform/testutil/fake.go` is the main test double — provides configurable hooks and call recording.
- **Testdata directory (21 files):**
  - `fixtures/pods_output.txt` — kubectl fixture (unused in tests!)
  - `recordings/` — 2 JSONL recording files (unused in tests!)
  - `scenarios/` — 17 YAML scenario files; only 4 are referenced from tests (validate-*.yaml, stdin_match.yaml, capture_chain.yaml)
  - `windows-hardening/` — 4 YAML scenarios used by CI integration tests (tag-gated)
- **Benchmarks:** 10 benchmarks across matcher, runner, verify packages. Tracked in BENCHMARKS.md with regression thresholds.
- **CI:** GitHub Actions on ubuntu-latest + windows-latest, Go 1.21, `go test -race -cover ./...`. Separate windows-hardening job runs integration tests with `-tags integration`.
- **Key coverage gaps:**
  1. `cmd` package at 64.2% — lowest tested production package
  2. `recorder` at 72.7% — session lifecycle paths under-tested
  3. `runner` at 77.2% — `FormatStdinMismatchError`, `InterceptDirPath` untested
  4. `scenario/model.go` — `ValidateDelay()` has no test coverage
  5. 13 of 21 testdata files are unused by any test
  6. No integration tests for Unix (only Windows integration tests in CI)
- **Key coverage gaps:**
   1. \cmd\ package at 64.2%  lowest tested production package
      - ** Decision:** Filed michael-test-coverage-gaps.md
      - **Recommendations:** Test error paths (exec/run/record), target 80% coverage
   2. \ecorder\ at 72.7%  session lifecycle paths under-tested
   3. \unner\ at 77.2%  \FormatStdinMismatchError\, \InterceptDirPath\ untested
   4. \scenario/model.go\  \ValidateDelay()\ has no test coverage
   5. 13 of 21 testdata files are unused by any test
   6. No integration tests for Unix (only Windows integration tests in CI)

### 2026-04-03 — Public API Test Coverage for pkg/ Promotion

- **Context:** Gene promoted `scenario`, `matcher`, `verify` to `pkg/`. Charles extracting `ReplayEngine` to `pkg/replay/`. Tests needed to validate the public API surface before gert integration.
- **New tests added:** 4 test files, ~72 test functions/subtests
  - `pkg/scenario/publicapi_test.go` — ValidateDelay (8 cases), CallBounds validation (6 cases), EffectiveCalls defaults, FlatSteps/GroupRanges expansion, YAML round-trip, Session/Security/Capture validation, CallBounds YAML loading, mutual exclusivity, nested group rejection
  - `pkg/matcher/publicapi_test.go` — Mixed pattern types (7 cases), concurrent safety (100 goroutines), ElementMatchDetail comprehensive (8 cases), pattern boundary edge cases (5 cases)
  - `pkg/verify/publicapi_test.go` — Budget-aware verification table (6 cases), group labels/metadata, JSON budget field round-trip, JUnit failure messages, JUnit optional/skipped, JUnit error state, StepLabel edge cases
  - `tests/integration/pipeline_test.go` — Full Load→Match→Verify pipeline for ordered and unordered scenarios, wildcard/regex/literal matching, budget-aware ordered/unordered replay, soft-advance, empty scenario rejection, no-match, nil-state error, group scenario verify with metadata, 8 testdata fixture loading tests (call_bounds, multi_step, single_step, capture_group, capture_chain, security_allowlist, deny_env_vars, session_ttl), mixed match types, JSON output round-trip, match diagnostics
- **Coverage gaps filled:** ValidateDelay (was 0%), CallBounds.Validate (minimal coverage), Session.Validate, concurrent matcher safety, budget-aware verify edge cases
- **Testdata utilization:** Tests now reference 8 of 17 scenario fixtures (up from 4), reducing orphaned testdata
- **Architecture note:** Integration tests placed in `tests/integration/` (not `pkg/replay/`) to avoid import cycles caused by `internal/runner` → `pkg/replay` dependency
- **All tests pass:** `go test ./pkg/matcher/... ./pkg/scenario/... ./pkg/verify/... ./tests/integration/... -count=1` — 4 packages, all OK

### 2026-04-03  Orchestration Checkpoint: Public API Test Coverage Complete

**Status:** COMPLETED  72 new tests covering pkg/ public API surface.

**What was delivered:**
1. **4 new test files:**
   - pkg/scenario/publicapi_test.go  8+ test cases: ValidateDelay, CallBounds, EffectiveCalls, FlatSteps/GroupRanges, YAML round-trip, Session/Security/Capture validation
   - pkg/matcher/publicapi_test.go  7+ test cases: Mixed patterns, concurrent safety (100 goroutines), ElementMatchDetail, pattern boundary edges
   - pkg/verify/publicapi_test.go  6+ test cases: Budget-aware verification, group labels, JSON/JUnit formatting, edge states
   - 	ests/integration/pipeline_test.go  Full pipeline tests: LoadMatchVerify for ordered/unordered, wildcard/regex/literal, budget-aware replay, 8 testdata fixtures

2. **Coverage improvements:**
   - ValidateDelay: 0%  covered (was previously untested)
   - CallBounds validation: enhanced coverage
   - Concurrent matcher safety: explicitly tested (100 goroutines)
   - Budget-aware verify: comprehensive edge cases
   - Testdata utilization: 4 of 17  8 of 17 (reduced orphaned fixtures)

3. **Architecture decision:** Integration tests placed in 	ests/integration/ (not pkg/replay/) to avoid import cycles from internal/runner  pkg/replay

4. **Test status:** All 72 new tests passing. go test ./pkg/matcher/... ./pkg/scenario/... ./pkg/verify/... ./tests/integration/... -count=1 

**Quality metrics:**
- Test pass rate:  100%
- API coverage:  Complete
- Import cycle resolution:  Clean architecture

**Decision captured:** Integration test location decision documented in .squad/decisions.md

**Next phase:** Await verification from gert integration team on API stability and consumer patterns.

### 2026-04-03 — gert ↔ cli-replay Integration Adapter Tests

- **Context:** Robert built `ReplayExecutor` and `RecordingExecutor` adapters in `gert/pkg/providers/clireplay/` that bridge gert's `CommandExecutor` interface to cli-replay's `pkg/replay.Engine`. Wrote comprehensive test suite to validate the integration surface.
- **New test files:** 3 files, ~45 new test functions (78 total with subtests)
  - `replay_executor_test.go` — Tests: single step, multi-step ordered, out-of-order rejection, wildcard matching, regex matching, regex no-match, unordered group (reverse order), group mismatch, call bounds (budget exhaustion + soft advance), error/non-zero exit, capture chain with template rendering, scenario exhaustion, concurrent access (50 goroutines), reset, snapshot + resume, WithVars option passthrough, env parameter handling
  - `recording_executor_test.go` — Tests: stdout/stderr/exit code capture, multi-command ordering, empty output, large output (100KB), special characters (unicode/CRLF/JSON/tabs), concurrent recording (30 goroutines), save + reload YAML, save error paths, path option, result delegation, env passthrough
  - `integration_test.go` — Tests: full record→save→load→replay round-trip, round-trip with errors, YAML format validation (Validate() call), kubectl/Azure CLI/Docker workflow round-trips, testdata fixture loading (7 fixtures), unordered group fixture, programmatic scenario construction, file-based full consumption
- **Test fixtures created:** 8 YAML scenario files in `gert/pkg/providers/clireplay/testdata/`
  - `single_step.yaml`, `multi_step.yaml`, `wildcard_match.yaml`, `regex_match.yaml`, `unordered_group.yaml`, `call_bounds.yaml`, `capture_chain.yaml`, `error_step.yaml`
- **Key test coverage:**
  - All match modes: literal, wildcard (`{{ .any }}`), regex (`{{ .regex "..." }}`)
  - Ordered + unordered matching
  - Call budget enforcement + soft advance
  - Capture chain with template rendering
  - Thread safety (both adapters)
  - Record → save → load → replay cycle (THE dream test)
  - Edge cases: empty output, large output, special chars, CRLF
- **All 78 tests pass:** `go test ./pkg/providers/clireplay/... -count=1` — OK
- **Existing tests unaffected:** `go test ./pkg/providers/... -count=1` — all pass
- **Race detector:** Not available (no GCC on Windows), but concurrent tests exercise thread safety manually
- **Key finding:** Robert's adapter code is clean and well-structured. The `fakeExecutor` in his `clireplay_test.go` is a simple single-result double; my `multiFakeExecutor` adds multi-result sequencing for complex workflows.
