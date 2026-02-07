# Implementation Plan: P1 CI/CD Enhancements

**Branch**: `006-p1-cicd-enhancements` | **Date**: 2026-02-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/006-p1-cicd-enhancements/spec.md`

## Summary

Add three capabilities to improve CI/CD workflows: (1) a new `cli-replay exec` command that manages the full intercept lifecycle (setup → spawn child → verify → cleanup) in a single invocation, (2) integration test coverage proving `verify` and `clean` correctly respect `CLI_REPLAY_SESSION` in parallel sessions, and (3) shell trap auto-cleanup for the eval pattern. The approach uses Go's `os/exec` for child process management, `os/signal` for signal forwarding, and POSIX-compliant trap syntax generation.

## Technical Context

**Language/Version**: Go 1.21  
**Primary Dependencies**: cobra v1.8.0 (CLI framework), yaml.v3 (scenario loading), testify v1.8.4 (testing), golang.org/x/term v0.18.0  
**Storage**: JSON state files in `os.TempDir()` (SHA256-hashed filenames via `StateFilePathWithSession`)  
**Testing**: `go test ./...` (stdlib testing + testify assertions)  
**Target Platform**: Unix/Linux/macOS (Windows exec mode explicitly out of scope)  
**Project Type**: Single Go module, dual-mode CLI binary  
**Performance Goals**: Exec setup < 100ms overhead; signal cleanup < 5 seconds  
**Constraints**: Zero leaked state files/intercept dirs after exec completes; backward compatible with existing eval workflow  
**Scale/Scope**: ~2500 LOC existing; this feature adds ~600-800 LOC across cmd/exec.go, cmd/run.go trap emission, and tests

### Architecture Notes

- **Dual-mode binary**: `main.go` detects invocation name via `filepath.Base(os.Args[0])`. When invoked as `cli-replay`, enters cobra command tree. When invoked as any other name (symlink), enters intercept mode calling `runner.ExecuteReplay()`.
- **State management**: `runner.StateFilePath(path)` reads `CLI_REPLAY_SESSION` env var internally and delegates to `StateFilePathWithSession(path, session)`. This means `verify` and `clean` already get session-aware paths **if** `CLI_REPLAY_SESSION` is set in the environment.
- **Session handling confirmed**: `emitShellSetup` already exports `CLI_REPLAY_SESSION`, `CLI_REPLAY_SCENARIO`, and prepends the intercept directory to PATH in all shell variants (bash, PowerShell, cmd). Both `cmd/verify.go` and `cmd/clean.go` call `runner.StateFilePath(absPath)` which reads `CLI_REPLAY_SESSION` from the environment. Session isolation works end-to-end; the gap is **integration test coverage** and **trap auto-cleanup emission**.
- **Intercept setup**: `cmd/run.go:runRun()` creates the intercept dir, symlinks, generates a session ID, and emits shell commands via `emitShellSetup()`. The session ID is exported correctly. What is **missing** is cleanup trap emission (`trap '_cli_replay_clean' EXIT INT TERM`) — this is the US3 work item.
- **Exec mode approach**: New `cmd/exec.go` will reuse `extractCommands`, `createIntercept`, `generateSessionID`, and `hashScenarioFile` from `cmd/run.go`, but instead of emitting shell setup for eval, will directly set env vars on the `exec.Cmd` via `buildChildEnv()` and manage the child process lifecycle (setup → spawn → signal-forward → wait → verify → cleanup).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is an unfilled template — no project-specific principles, constraints, or gates are defined. **PASS** (no violations possible).

## Project Structure

### Documentation (this feature)

```text
specs/006-p1-cicd-enhancements/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (CLI contract)
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
├── exec.go              # NEW — cli-replay exec command (FR-001 through FR-010)
├── exec_test.go         # NEW — exec command unit tests
├── run.go               # MODIFIED — add trap generation (FR-015 through FR-017), export CLI_REPLAY_SESSION
├── clean.go             # VERIFIED — already idempotent per research R6; add test coverage only
├── verify.go            # VERIFIED — already session-aware via StateFilePath()
└── cli-replay/
    ├── run.go           # EVALUATE — has no emitShellSetup; may need one added if this entry point supports eval pattern
    └── verify.go        # VERIFIED — already session-aware via StateFilePath()

internal/
├── runner/
│   ├── replay.go        # NO CHANGES — ExecuteReplay already session-aware
│   ├── state.go         # NO CHANGES — StateFilePath/StateFilePathWithSession already correct
│   └── errors.go        # NO CHANGES
└── ...

main.go                  # NO CHANGES — intercept mode already correct
```

**Structure Decision**: Single Go module, existing package layout. New code is a single `cmd/exec.go` file plus modifications to `cmd/run.go` (trap generation + session export) and `cmd/clean.go` (idempotency). No new packages needed.

## Complexity Tracking

> No constitution violations — section not applicable.
