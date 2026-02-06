# Implementation Plan: Core Scenario Replay

**Branch**: `001-core-scenario-replay` | **Date**: 2026-02-05 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-core-scenario-replay/spec.md`

## Summary

Build cli-replay v0 — a scenario-driven CLI replay tool that acts as a fake command-line executable for testing systems that orchestrate external CLI tools. The tool intercepts CLI invocations via PATH manipulation, matches them against YAML-defined scenarios in strict order, and returns predetermined responses. State is persisted via temp files to track progress across invocations.

## Technical Context

**Language/Version**: Go 1.21+  
**Primary Dependencies**: cobra (CLI), gopkg.in/yaml.v3 (YAML parsing), testify/assert (testing)  
**Storage**: Temp file for state persistence (`/tmp/cli-replay-<hash>.state`)  
**Testing**: `go test` + testify/assert, table-driven tests  
**Target Platform**: Windows, macOS (arm64/amd64), Linux (arm64/amd64)  
**Project Type**: Single project (CLI tool)  
**Performance Goals**: <100ms for scenarios with up to 20 steps  
**Constraints**: Single static binary, CGO_ENABLED=0, <20MB binary size  
**Scale/Scope**: Single-process sequential execution, scenario files <1MB

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Test-First Development | ✅ PASS | TDD enforced: tests written before implementation per tasks template |
| II. Quality Gates | ✅ PASS | Plan includes unit tests, integration tests, golangci-lint, gofmt |
| III. Single Binary Distribution | ✅ PASS | FR-014 requires static binary; CGO_ENABLED=0 in build |
| IV. Idiomatic Go | ✅ PASS | Error returns, no panics, context.Context for cancellation |
| V. Simplicity & YAGNI | ✅ PASS | v0 scope excludes regex matching, record mode, parallel steps |

**Pre-Research Gate**: ✅ ALL PRINCIPLES SATISFIED — Proceed to Phase 0

### Post-Design Re-Check (Phase 1 Complete)

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Test-First Development | ✅ PASS | data-model.md defines testable validation rules; contracts/cli.md defines observable behaviors |
| II. Quality Gates | ✅ PASS | Every entity has validation rules; error messages specified for all edge cases |
| III. Single Binary Distribution | ✅ PASS | Design uses only stdlib + pure-Go dependencies (yaml.v3, cobra, testify) |
| IV. Idiomatic Go | ✅ PASS | Design uses error returns, no global state, standard Go project layout |
| V. Simplicity & YAGNI | ✅ PASS | Minimal entities (5); no over-engineering (JSON state vs SQLite) |

**Post-Design Gate**: ✅ ALL PRINCIPLES SATISFIED — Ready for /speckit.tasks

## Project Structure

### Documentation (this feature)

```text
specs/001-core-scenario-replay/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (CLI interface spec)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
cmd/
└── cli-replay/
    └── main.go              # Entry point, cobra root command

internal/
├── scenario/
│   ├── loader.go            # YAML parsing and validation
│   ├── model.go             # Scenario, Step, Match, Response structs
│   └── loader_test.go
├── matcher/
│   ├── argv.go              # Strict argv comparison
│   └── argv_test.go
├── runner/
│   ├── replay.go            # Core replay logic
│   ├── state.go             # State file persistence
│   ├── replay_test.go
│   └── state_test.go
└── template/
    ├── render.go            # Go text/template rendering
    └── render_test.go

testdata/
├── scenarios/               # Test scenario YAML files
└── fixtures/                # stdout_file/stderr_file test data

go.mod
go.sum
Makefile
.golangci.yml
```

**Structure Decision**: Single project layout following Go conventions. `cmd/` for entry point, `internal/` for private packages, `testdata/` for test fixtures.

## Complexity Tracking

> No violations — all principles satisfied without deviation.
