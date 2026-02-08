# Implementation Plan: P3/P4 Enhancements — Production Hardening, CI Ecosystem & Documentation

**Branch**: `010-p3-p4-enhancements` | **Date**: 2026-02-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/010-p3-p4-enhancements/spec.md`

## Summary

This plan covers six enhancements for cli-replay spanning production reliability, CI ecosystem, and documentation: (1) **Windows Job Objects** — replace single-process `Process.Kill()` with job-object-based process tree termination in `cmd/exec_windows.go` for reliable grandchild cleanup on Windows CI runners; (2) **`cli-replay validate`** — a new cobra subcommand that exposes existing `scenario.Load()`/`Scenario.Validate()` logic as a standalone pre-flight check with multi-file support and JSON output; (3) **Official GitHub Action** — a composite action in a separate `ormasoftchile/cli-replay-action` repository that downloads, installs, and invokes cli-replay for CI workflows; (4) **SECURITY.md** — centralized threat model and trust boundary documentation; (5) **Scenario Cookbook** — annotated Terraform, Helm, and kubectl example scenarios in `examples/cookbook/`; (6) **Benchmark Documentation** — baseline numbers and regression thresholds for existing benchmarks.

Research findings (see [research.md](research.md)) confirm that `golang.org/x/sys/windows` provides the job object APIs needed with no new module dependencies (already indirect), that the existing `scenario.LoadFile()` + `Scenario.Validate()` pipeline accumulates all validation errors and can be reused directly by the validate command, and that composite GitHub Actions using shell steps are the correct pattern for multi-platform binary distribution.

## Technical Context

**Language/Version**: Go 1.21+
**Primary Dependencies**: cobra v1.8.0 (CLI), yaml.v3 (scenario parsing), testify v1.8.4 (testing), golang.org/x/term v0.18.0 (terminal detection), golang.org/x/sys v0.19.0 (system calls, Windows APIs)
**Storage**: JSON state files (atomic write via temp + rename); YAML scenario files
**Testing**: `go test` with testify assertions; table-driven tests; `testing.B` for benchmarks
**Target Platform**: Linux, macOS, Windows (cross-platform with `_unix.go`/`_windows.go` build tags)
**Project Type**: Single CLI binary
**Performance Goals**: Validate command completes in < 1 second for any scenario; job object creation overhead < 5ms per exec invocation
**Constraints**: Zero new direct dependencies; backward compatible with all existing scenarios and workflows; job object fallback to `Process.Kill()` on failure
**Scale/Scope**: Scenarios typically 5–50 steps; benchmarks cover 100–500 steps; 3 cookbook examples; 1 GitHub Action definition

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is a blank template — no project-specific principles or gates are defined. No violations to evaluate. Gate passes by default.

**Post-Phase 1 re-check**: Design decisions are consistent with existing codebase patterns:
- `validate` command follows identical cobra registration pattern (`init()` + `rootCmd.AddCommand()`)
- Job object abstraction uses build-tagged `_windows.go` files matching existing `exec_windows.go`/`exec_unix.go` convention
- `setupSignalForwarding()` function signature unchanged — internal implementation detail only
- `scenario.LoadFile()` + `Scenario.Validate()` reused directly — no new validation logic
- `golang.org/x/sys` already at v0.19.0 (indirect); promoted to direct import — no new module dependency
- All new features have corresponding test coverage planned
- No breaking changes to existing YAML schema, CLI interface, or file formats
- Documentation files (`SECURITY.md`, `BENCHMARKS.md`, cookbook) are standalone additions
- GitHub Action lives in a separate repository — no impact on cli-replay codebase

✅ Constitution check: PASS (no gates defined)

## Project Structure

### Documentation (this feature)

```text
specs/010-p3-p4-enhancements/
├── plan.md              # This file
├── spec.md              # Feature specification (created by /speckit.specify)
├── research.md          # Phase 0 output — research findings
├── data-model.md        # Phase 1 output — entity definitions
├── quickstart.md        # Phase 1 output — usage examples
├── contracts/
│   ├── cli.md           # validate command CLI contract
│   └── action.md        # GitHub Action inputs/outputs contract
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (created by /speckit.tasks — NOT this command)
```

### Source Code (repository root)

```text
cmd/
├── exec_windows.go      # MODIFY: Replace Process.Kill() with job object termination
├── exec_unix.go         # NO CHANGE: Signal forwarding remains as-is (FR-004)
├── exec.go              # MINOR: Possible hook for job object cleanup in defer
├── validate.go          # NEW: cli-replay validate subcommand (cobra command + runValidate)
└── validate_test.go     # NEW: validate command integration tests

internal/
├── platform/
│   ├── windows.go       # NO CHANGE: Shim generation unaffected
│   └── jobobject_windows.go # NEW: Job object create/assign/terminate helpers (build-tagged)
├── scenario/
│   ├── model.go         # NO CHANGE: Validate() already collects all errors in a single pass
│   └── loader.go        # NO CHANGE: LoadFile() reused by validate command
├── runner/              # NO CHANGE: Not involved in validate or job objects
├── verify/              # NO CHANGE: Not involved in this feature set
├── matcher/             # NO CHANGE
├── template/            # NO CHANGE
├── envfilter/           # NO CHANGE
└── recorder/            # NO CHANGE

# Repository root (new files)
SECURITY.md              # NEW: Threat model and security documentation (FR-022–FR-027)
BENCHMARKS.md            # NEW: Benchmark baselines and regression thresholds (FR-032–FR-034)

examples/
├── cookbook/
│   ├── README.md        # NEW: Decision matrix and cookbook overview (FR-031)
│   ├── terraform-workflow.yaml    # NEW: Terraform init/plan/apply scenario (FR-028)
│   ├── terraform-test.sh          # NEW: Companion test script
│   ├── helm-deployment.yaml       # NEW: Helm repo add/upgrade/status scenario (FR-028)
│   ├── helm-test.sh               # NEW: Companion test script
│   ├── kubectl-pipeline.yaml      # NEW: Multi-tool kubectl with groups/captures (FR-028)
│   └── kubectl-test.sh            # NEW: Companion test script

# Separate repository: ormasoftchile/cli-replay-action
# (design documented here; implementation in external repo)
action.yml               # Composite action definition (FR-015–FR-021)

testdata/
└── scenarios/
    ├── validate-valid.yaml      # NEW: Valid scenario for validate tests
    ├── validate-invalid.yaml    # NEW: Multi-error invalid scenario
    └── validate-bad-yaml.yaml   # NEW: YAML parse error test
```

**Structure Decision**: Single project layout. All new Go code goes into existing packages following established patterns. The `validate` command joins the existing cobra subcommand set in `cmd/`. Job object support uses build-tagged platform files matching the existing `_windows.go`/`_unix.go` convention. The GitHub Action lives in a separate repository per GitHub convention. Documentation files (`SECURITY.md`, `BENCHMARKS.md`) go at the repository root. Cookbook examples extend the existing `examples/` directory.

## Complexity Tracking

No constitution violations to justify — constitution is blank. Design follows existing patterns throughout.
