# Implementation Plan: P2 Quality of Life Enhancements

**Branch**: `007-p2-quality-of-life` | **Date**: 2026-02-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-p2-quality-of-life/spec.md`

## Summary

Add three quality-of-life features to cli-replay: (1) machine-readable verify output via `--format json|junit` flags on `verify` and `--report-file` on `exec`, (2) a JSON Schema for scenario YAML files to enable IDE autocompletion and validation, and (3) step groups with unordered matching to support scenarios where command execution order is non-deterministic. US1 touches only the `cmd/verify.go` and `cmd/exec.go` output path. US2 is a standalone schema artifact. US3 is the most complex — it extends the scenario model, YAML loader, replay engine, state tracking, and verification. Zero new external dependencies.

## Technical Context

**Language/Version**: Go 1.21+ (module: `github.com/cli-replay/cli-replay`)  
**Primary Dependencies**: cobra v1.8.0, gopkg.in/yaml.v3 v3.0.1, testify v1.8.4, golang.org/x/term v0.18.0  
**Storage**: JSON state files on disk in `.cli-replay/` adjacent to scenario file  
**Testing**: `go test` + `testify/assert`, table-driven tests  
**Target Platform**: Cross-platform (macOS arm64/amd64, Linux arm64/amd64, Windows), CGO_ENABLED=0  
**Project Type**: Single binary CLI tool  
**Performance Goals**: Structured output adds < 5ms overhead vs text output (SC-006)  
**Constraints**: No new external dependencies; single binary; backward compatible with all existing scenarios  
**Scale/Scope**: 3 features (23 FRs), ~12 files modified/created, 1 schema artifact

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-Research Check ✅

The constitution is a blank template with no project-specific constraints defined. All principles below are inferred from established project conventions (prior features 001–006).

| Principle | Status | Notes |
|-----------|--------|-------|
| **Single Binary Distribution** | ✅ PASS | No new dependencies; `encoding/json`, `encoding/xml` are stdlib. JSON Schema is a static file, not a runtime dep |
| **Backward Compatibility** | ✅ PASS | `--format text` is default, existing behavior unchanged; step groups are opt-in via new YAML syntax |
| **Idiomatic Go** | ✅ PASS | Struct serialization for JSON/XML output; `encoding/xml` for JUnit; YAML union type via custom UnmarshalYAML |
| **Test Coverage** | ✅ PASS | Each output format gets table-driven tests; step group matching gets exhaustive order-permutation tests |
| **Simplicity & YAGNI** | ✅ PASS | JUnit via stdlib `encoding/xml` (no template engine); schema is hand-authored JSON (no code generation); groups support only `unordered` mode |

## Project Structure

### Documentation (this feature)

```text
specs/007-p2-quality-of-life/
├── plan.md              # This file
├── research.md          # Phase 0: Research decisions
├── data-model.md        # Phase 1: Extended entities and relationships
├── quickstart.md        # Phase 1: Usage examples for all 3 features
├── contracts/
│   ├── verify-json.md       # JSON output schema
│   ├── verify-junit.md      # JUnit XML output schema
│   └── scenario-schema.md   # JSON Schema design reference
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
schema/
└── scenario.schema.json     # CREATE: JSON Schema for scenario YAML files (US2)

internal/
├── scenario/
│   ├── model.go             # MODIFY: Add StepGroup type, StepElement union type
│   ├── model_test.go        # MODIFY: Add group validation tests
│   ├── loader.go            # MODIFY: Custom UnmarshalYAML for step/group union
│   └── loader_test.go       # MODIFY: Add group loading/parsing tests
├── runner/
│   ├── replay.go            # MODIFY: Add group-aware matching branch
│   ├── replay_test.go       # MODIFY: Add unordered group matching tests
│   ├── state.go             # MODIFY: Extend state for group step tracking
│   └── state_test.go        # MODIFY: Add group state tests
├── verify/
│   ├── result.go            # CREATE: VerifyResult, StepResult types
│   ├── result_test.go       # CREATE: Result construction tests
│   ├── json.go              # CREATE: JSON formatter
│   ├── json_test.go         # CREATE: JSON output tests
│   ├── junit.go             # CREATE: JUnit XML formatter
│   └── junit_test.go        # CREATE: JUnit output tests

cmd/
├── verify.go                # MODIFY: Add --format flag, wire formatters
├── verify_test.go           # MODIFY: Add format flag integration tests
├── exec.go                  # MODIFY: Add --report-file and --format flags
├── exec_test.go             # MODIFY: Add report-file tests
├── run.go                   # MODIFY: Migrate scn.Steps → scn.FlatSteps()
└── run_test.go              # MODIFY: Update tests for FlatSteps() migration

examples/
└── recordings/
    └── step-group-demo.yaml # CREATE: Example unordered group scenario
```

**Structure Decision**: Existing Go package layout is preserved. A new `internal/verify/` package encapsulates structured output formatting (JSON, JUnit) — cleanly separated from the `cmd/` layer and reusable by both `verify` and `exec` commands. The `schema/` directory at the repo root is the conventional location for schema artifacts.

## Complexity Tracking

> No constitution violations. All design decisions align with established project conventions.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(none)* | — | — |

### Post-Design Check ✅

| Principle | Status | Notes |
|-----------|--------|-------|
| **Single Binary Distribution** | ✅ PASS | Zero new deps confirmed. `encoding/json`, `encoding/xml` are stdlib. JSON Schema is a static file. |
| **Backward Compatibility** | ✅ PASS | `StepElement` union type unmarshals existing step-only YAML identically. `FlatSteps()` bridge preserves `[]Step` semantics. State file adds optional `active_group` field (nil = no group). |
| **Idiomatic Go** | ✅ PASS | Custom `UnmarshalYAML(*yaml.Node)` for union dispatch. `encoding/xml` struct tags for JUnit. `VerifyResult` is a plain serializable struct. |
| **Test Coverage** | ✅ PASS | New `internal/verify/` package fully testable in isolation. Group matching tested via order-permutation table-driven tests. Schema validated against all example scenarios. |
| **Simplicity & YAGNI** | ✅ PASS | Only `unordered` mode (no `ordered` groups — that's just regular steps). Flat `StepCounts` preserved (no nested state). First-match-wins scanning (no best-match heuristics). |

## Artifacts

| Artifact | Path | Status |
|----------|------|--------|
| Feature Spec | [spec.md](spec.md) | ✅ Complete (5 clarifications resolved) |
| Quality Checklist | [checklists/requirements.md](checklists/requirements.md) | ✅ All passed |
| Research | [research.md](research.md) | ✅ Complete (R1-R4) |
| Data Model | [data-model.md](data-model.md) | ✅ Complete |
| Verify JSON Contract | [contracts/verify-json.md](contracts/verify-json.md) | ✅ Complete |
| Verify JUnit Contract | [contracts/verify-junit.md](contracts/verify-junit.md) | ✅ Complete |
| Scenario Schema Contract | [contracts/scenario-schema.md](contracts/scenario-schema.md) | ✅ Complete |
| Quickstart | [quickstart.md](quickstart.md) | ✅ Complete |
