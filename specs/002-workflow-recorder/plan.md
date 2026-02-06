# Implementation Plan: Workflow Recorder

**Branch**: `002-workflow-recorder` | **Date**: February 6, 2026 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-workflow-recorder/spec.md`

## Summary

Add a `record` subcommand to cli-replay that captures real command executions and automatically generates replay scenario YAML files. The recorder will use PATH shim interception to transparently capture command argv, exit codes, stdout, and stderr, then convert these recordings into valid scenario YAML files conforming to the existing schema. This eliminates manual YAML crafting and enables users to create realistic replay scenarios from actual workflows.

## Technical Context

**Language/Version**: Go 1.21+  
**Primary Dependencies**: `cobra` (CLI framework), `gopkg.in/yaml.v3` (YAML generation)  
**Storage**: Temporary directory for shim executables, JSONL log file for session data, output YAML file  
**Testing**: `testing` package with `testify/assert`, table-driven tests, integration tests with real command execution  
**Target Platform**: macOS (arm64/amd64), Linux (arm64/amd64)  
**Project Type**: Single project - CLI tool extension  
**Performance Goals**: <5% overhead vs direct command execution, handle 50+ commands per session  
**Constraints**: No root/sudo required, single binary distribution, no external runtime dependencies  
**Scale/Scope**: MVP with P1 (basic recording) and P2 (multi-step), P3-P4 deferred to future iterations

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence/Notes |
|-----------|--------|----------------|
| **I. Test-First Development** | ✅ PASS | Plan includes unit tests for recorder, shim generator, YAML converter; integration tests for end-to-end recording workflows |
| **II. Quality Gates** | ✅ PASS | Standard gates apply: table-driven unit tests, integration tests with real scenarios, golangci-lint, gofmt/goimports, godoc for public APIs |
| **III. Single Binary Distribution** | ✅ PASS | No external dependencies added; recorder uses temporary directories and PATH manipulation; continues CGO_ENABLED=0 compilation |
| **IV. Idiomatic Go** | ✅ PASS | Uses error returns, context.Context for command execution, explicit dependency passing, follows existing codebase patterns |
| **V. Simplicity & YAGNI** | ✅ PASS | Minimal MVP: P1 (basic recording) + P2 (multi-step); P3-P4 deferred; no speculative features like template detection or smart deduplication |
| **Technology Stack** | ✅ PASS | Reuses existing dependencies (cobra, yaml.v3, testify); no new external libraries |

**Overall**: ✅ ALL GATES PASS - No constitutional violations

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

## Project Structure

### Documentation (this feature)

```text
specs/002-workflow-recorder/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output - shim generation, JSONL vs in-memory, YAML marshaling
├── data-model.md        # Phase 1 output - RecordedCommand, RecordingSession, ScenarioConverter
├── quickstart.md        # Phase 1 output - Usage examples and workflow guide
├── contracts/           # Phase 1 output - CLI interface spec
│   └── cli.md          # record subcommand flags and behavior
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── recorder/            # NEW: Recording functionality
│   ├── session.go      # RecordingSession type, session lifecycle
│   ├── session_test.go
│   ├── shim.go         # Shim executable generation and PATH setup
│   ├── shim_test.go
│   ├── converter.go    # RecordingSession -> Scenario YAML conversion
│   ├── converter_test.go
│   └── command.go      # RecordedCommand type and capture logic
│   └── command_test.go
├── scenario/            # EXISTING: Scenario model (reused for validation)
│   ├── model.go
│   ├── loader.go
│   └── ...
├── matcher/             # EXISTING: Not directly used by recorder
├── runner/              # EXISTING: Used to validate generated scenarios
└── template/            # EXISTING: May be used for meta field defaults

cmd/                     # NEW: CLI commands (if not exists, create)
├── root.go             # Root cobra command
├── record.go           # NEW: record subcommand implementation
└── record_test.go      # Integration tests for record command

testdata/
├── scenarios/          # EXISTING: Test scenarios
└── recordings/         # NEW: Test recording fixtures
    ├── simple_command.jsonl
    └── multi_step.jsonl
```

**Structure Decision**: Single project structure with new `internal/recorder` package for recording logic and new `cmd/` directory for cobra CLI commands (currently cli-replay doesn't have explicit cmd/ structure based on codebase analysis). The recorder package will depend on existing `internal/scenario` types for validation and YAML generation.

## Complexity Tracking

*No constitutional violations - this section intentionally left empty.*

## Phase 0: Research (Complete)

✅ **Research artifacts generated**: [research.md](research.md)

**Key decisions**:
1. **Shim strategy**: Shell script shims (bash) for simplicity and portability
2. **Storage format**: JSONL incremental logging for crash resilience
3. **YAML generation**: Direct struct marshaling using existing scenario types
4. **PATH manipulation**: Subprocess with custom environment
5. **Command filtering**: Shim generation time filtering

## Phase 1: Design (Complete)

✅ **Design artifacts generated**:
- [data-model.md](data-model.md) - RecordedCommand, RecordingSession, SessionMetadata entities
- [contracts/cli.md](contracts/cli.md) - CLI interface specification
- [quickstart.md](quickstart.md) - User guide and examples

**Data model highlights**:
- `RecordedCommand`: Immutable capture of single execution (argv, exit, stdout, stderr)
- `RecordingSession`: Manages lifecycle, shim setup, JSONL logging
- `SessionMetadata`: User-provided scenario metadata
- Conversion flow: JSONL → RecordingLog → RecordingSession → Scenario → YAML

**CLI interface highlights**:
- Required flag: `--output` (YAML output path)
- Optional flags: `--name`, `--description`, `--command` (repeatable)
- Command separator: `--` to distinguish flags from user command
- Exit codes: 0 (success), 1 (setup failure), 2 (user command failed), 3 (YAML generation failed)

## Phase 1: Constitution Re-Check

| Principle | Status | Post-Design Evidence |
|-----------|--------|---------------------|
| **I. Test-First Development** | ✅ PASS | Test plan includes: session lifecycle tests, shim generation tests, JSONL parsing tests, YAML conversion tests, end-to-end integration tests |
| **II. Quality Gates** | ✅ PASS | All public APIs documented (godoc), table-driven tests specified, integration tests defined |
| **III. Single Binary Distribution** | ✅ PASS | Design uses only stdlib + existing deps (cobra, yaml.v3); bash shims generated at runtime; no external binaries required |
| **IV. Idiomatic Go** | ✅ PASS | Data model uses immutable structs, error wrapping, context for cancellation, explicit dependency injection |
| **V. Simplicity & YAGNI** | ✅ PASS | Deferred P3/P4 features (interactive mode, template detection, smart dedup); MVP focuses on basic + multi-step recording only |

**Overall**: ✅ ALL GATES STILL PASS - Design maintains constitutional compliance

## Next Steps

**Phase 2**: Generate implementation tasks using `/speckit.tasks` command

**Expected task categories**:
1. Setup: Create `cmd/` structure, `internal/recorder/` package
2. Core: Implement RecordedCommand, RecordingSession, shim generation
3. Conversion: JSONL parsing, scenario conversion, YAML marshaling
4. CLI: Cobra integration, flag handling, error messages
5. Testing: Unit tests, integration tests, validation
6. Documentation: Update README, add usage examples

**Implementation order** (test-first):
1. RecordedCommand struct + tests
2. JSONL logging + parsing tests
3. Shim generation + tests
4. RecordingSession lifecycle + tests
5. Scenario conversion + tests
6. CLI command integration + tests
7. End-to-end integration tests
