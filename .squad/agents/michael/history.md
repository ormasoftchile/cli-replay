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
