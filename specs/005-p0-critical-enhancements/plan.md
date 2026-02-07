# Implementation Plan: P0 Critical Enhancements

**Branch**: `005-p0-critical-enhancements` | **Date**: 2026-02-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/005-p0-critical-enhancements/spec.md`

## Summary

Four P0 critical enhancements for cli-replay: (1) per-element mismatch diagnostics with template pattern display, (2) call count bounds for retry/polling steps via `calls.min`/`calls.max`, (3) stdin matching for pipe-based CLI workflows, and (4) security allowlist restricting interceptable commands. All changes are backward compatible — existing scenarios work without modification.

**Technical approach** (from research):
- Mismatch diagnostics: new `ElementMatchDetail()` in matcher + rewrite `findFirstDiff()` to use element-level matching. Auto-detect color support via `term.IsTerminal`.
- Call count bounds: replace `ConsumedSteps []bool` with `StepCounts []int`. Budget-check-before-match pattern with soft advance when min is met. `*CallBounds` on Step.
- stdin matching: temp file capture in shims (`[ ! -t 0 ]` guard on Unix, `[Console]::IsInputRedirected` on Windows). Go binary reads `os.Stdin` directly in intercept mode. 1 MB cap.
- Security allowlist: `meta.security.allowed_commands` + `--allowed-commands` CLI flag. Intersection when both set. Validate at `run` time before creating intercepts.

## Technical Context

**Language/Version**: Go 1.21
**Primary Dependencies**: cobra v1.8.0, yaml.v3, testify v1.8.4
**Storage**: JSON state files in `os.TempDir()` (SHA256-hashed filenames)
**Testing**: `go test ./...` (unit + integration tests with testify assertions)
**Target Platform**: macOS, Linux, Windows (cross-platform via `internal/platform` abstraction)
**Project Type**: Single CLI binary (dual-mode: management commands + intercept mode)
**Performance Goals**: Intercept mode < 5ms latency (shim overhead); stdin capture < 50ms for 1 MB
**Constraints**: One new dependency (`golang.org/x/term` for terminal detection — Go extended stdlib). Backward compatible state files. 1 MB stdin cap.
**Scale/Scope**: ~4K LOC changes across ~15 files, 4 features implemented incrementally

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Status**: Constitution file (`.specify/memory/constitution.md`) is an unfilled template — no project-specific principles, gates, or governance rules defined. No gates to evaluate.

**Post-design re-check**: N/A — no constitution gates configured.

## Project Structure

### Documentation (this feature)

```text
specs/005-p0-critical-enhancements/
├── plan.md              # This file
├── spec.md              # Feature specification (4 user stories, 21 FRs)
├── research.md          # Phase 0: 8 research decisions (R-001 through R-008)
├── data-model.md        # Phase 1: entity changes, state transitions
├── quickstart.md        # Phase 1: developer guide for each feature
├── contracts/
│   ├── cli-contract.md  # Phase 1: CLI flag/output/error changes
│   └── yaml-schema.md   # Phase 1: YAML schema additions
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (files to modify)

```text
internal/
├── matcher/
│   ├── argv.go          # Add ElementMatchDetail(), MatchDetail struct
│   └── argv_test.go     # Tests for detail matching
├── runner/
│   ├── errors.go        # Rewrite FormatMismatchError(), fix findFirstDiff()
│   ├── errors_test.go   # Tests for enhanced diagnostics
│   ├── replay.go        # Call count budget check, soft advance, stdin read
│   ├── replay_test.go   # Tests for call count + stdin replay
│   ├── state.go         # StepCounts []int, migration, new methods
│   └── state_test.go    # Tests for state migration + count tracking
├── scenario/
│   ├── model.go         # CallBounds, Security, Match.Stdin, validation
│   └── model_test.go    # Tests for new types + validation
├── recorder/
│   ├── command.go       # Stdin field in RecordingEntry
│   ├── converter.go     # Emit match.stdin in YAML generation
│   └── shim.go          # Stdin capture in LogRecording
├── platform/
│   ├── unix.go          # Stdin capture block in bash shim
│   └── windows.go       # Stdin capture in PS1 shim
cmd/
├── run.go               # --allowed-commands flag, security validation
├── verify.go            # Per-step count reporting
└── cli-replay/
    ├── run.go           # --allowed-commands flag (if separate)
    └── verify.go        # Per-step count check

testdata/
├── scenarios/
│   ├── call_bounds.yaml       # New test fixture
│   ├── stdin_match.yaml       # New test fixture
│   └── security_allowlist.yaml # New test fixture
```

**Structure Decision**: Single Go project. All changes are within existing package boundaries — no new packages needed. New types are added to existing files (`model.go`, `argv.go`, `state.go`). New test fixtures in `testdata/scenarios/`.

## Complexity Tracking

> No constitution violations — no justifications needed.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *None* | — | — |
