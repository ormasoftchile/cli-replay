# Implementation Plan: P1/P2 Enhancements — Dynamic Capture, Dry-Run, Windows Audit & Benchmarks

**Branch**: `009-p1-p2-enhancements` | **Date**: 2026-02-07 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `specs/009-p1-p2-enhancements/spec.md`

## Summary

This plan covers four enhancements for cli-replay: (1) **Dynamic Capture** — a `capture` field on step responses that persists key-value pairs in the state file and injects them into subsequent step templates via `{{ .capture.<id> }}`; (2) **Dry-Run Mode** — a `--dry-run` flag on `run` and `exec` commands that loads, validates, and prints a human-readable step summary without any file system or environment side effects; (3) **Windows Signal Handling Audit** — build-tagged signal handling in `cmd/exec.go` so that `exec` mode correctly terminates child processes via `Process.Kill()` on Windows instead of the unsupported `SIGTERM`; (4) **Performance Benchmarks** — scaling existing 10-step benchmarks to 100, 200, and 500-step scenarios across matching, state I/O, group matching, and verification formatting.

Research findings (see [research.md](research.md)) confirm that the state file's atomic JSON write pattern supports captures with zero schema migration, that `ReplayResponseWithTemplate` is the single injection point for template variables, that Windows `SIGTERM` is definitively broken and `Process.Kill()` is the correct alternative, and that the `run` command has a natural dry-run cut point after allowlist validation (line ~75 in `cmd/run.go`) before any side effects.

## Technical Context

**Language/Version**: Go 1.21+  
**Primary Dependencies**: cobra (CLI), yaml.v3 (scenario parsing), testify (testing), text/template (rendering), golang.org/x/term  
**Storage**: JSON state files (atomic write via temp + rename)  
**Testing**: `go test` with testify assertions; table-driven tests; `testing.B` for benchmarks  
**Target Platform**: Linux, macOS, Windows (cross-platform with build tags)  
**Project Type**: Single CLI binary  
**Performance Goals**: Matching < 1ms/step, state I/O < 10ms for 500 steps, formatting < 50ms for 200 steps  
**Constraints**: Zero external dependencies added; no breaking changes to existing YAML schema  
**Scale/Scope**: Scenarios up to 500 steps for benchmarks; real-world usage typically 5–50 steps

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is a blank template — no project-specific principles or gates are defined. No violations to evaluate. Gate passes by default.

**Post-Phase 1 re-check**: Design decisions are consistent with existing codebase patterns:
- Capture field on `Response` follows existing YAML struct pattern with `omitempty`
- State persistence uses existing `WriteState`/`ReadState` atomic pattern
- Build-tagged files follow existing `internal/platform/` convention
- All new features have corresponding test coverage
- No new external dependencies introduced

✅ Constitution check: PASS (no gates defined)

## Project Structure

### Documentation (this feature)

```text
specs/009-p1-p2-enhancements/
├── plan.md              # This file
├── spec.md              # Feature specification (created by /speckit.specify)
├── research.md          # Phase 0 output — 6 research findings
├── data-model.md        # Phase 1 output — entity definitions
├── quickstart.md        # Phase 1 output — usage examples
├── contracts/
│   └── cli.md           # Phase 1 output — CLI flag and YAML schema contracts
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (created by /speckit.tasks — NOT this command)
```

### Source Code (repository root)

```text
internal/
├── scenario/
│   └── model.go         # MODIFY: Add Capture field to Response struct + validation
├── runner/
│   ├── state.go         # MODIFY: Add Captures field to State struct
│   ├── replay.go        # MODIFY: Inject captures into template vars in ReplayResponseWithTemplate
│   ├── dryrun.go        # NEW: DryRunReport struct + formatting logic
│   ├── dryrun_test.go   # NEW: Dry-run output tests
│   ├── state_test.go    # MODIFY: Add capture persistence round-trip tests
│   ├── replay_test.go   # MODIFY: Add capture rendering tests
│   └── bench_test.go    # NEW: State I/O benchmarks at 500 steps, replay orchestration at 100 steps
├── matcher/
│   └── bench_test.go    # NEW: Matching benchmarks at 100/500 steps, group matching at 50 steps
├── verify/
│   └── bench_test.go    # MODIFY: Add 200-step formatting benchmarks
├── template/
│   └── render.go        # MODIFY: Support map[string]interface{} for nested capture namespace
└── platform/            # EXISTING: Build-tagged OS helpers (no changes needed)

cmd/
├── run.go               # MODIFY: Add --dry-run flag, cut-point after validation
├── exec.go              # MODIFY: Add --dry-run flag, extract signal handling to build-tagged files
├── exec_unix.go         # NEW: Unix signal forwarding (extracted from exec.go)
└── exec_windows.go      # NEW: Windows signal handling with Process.Kill()

testdata/
├── scenarios/
│   ├── capture_chain.yaml       # NEW: 3-step capture chain test scenario
│   ├── capture_conflict.yaml    # NEW: Capture/vars conflict validation test
│   ├── capture_forward_ref.yaml # NEW: Forward reference detection test
│   └── capture_group.yaml       # NEW: Group capture semantics test
```

**Structure Decision**: Single project layout. All new code goes into existing packages following established patterns. The only new files are `dryrun.go` (runner), build-tagged exec files (cmd), benchmark files, and test scenarios. No new packages needed.

## Complexity Tracking

No constitution violations to justify — constitution is blank. Design follows existing patterns throughout.
