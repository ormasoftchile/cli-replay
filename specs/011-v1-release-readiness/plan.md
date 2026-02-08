# Implementation Plan: v1.0 Release Readiness

**Branch**: `011-v1-release-readiness` | **Date**: 2026-02-08 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `/specs/011-v1-release-readiness/spec.md`

## Summary

Harden cli-replay for v1.0 release by closing 8 remaining gaps: fix Unix process group signal forwarding (currently signals only the direct child despite README claiming group signaling), add CI badges, document ReDoS safety (Go RE2), create a reusable GitHub Action for easy CI adoption, and automate releases via GoReleaser. Seven of the original 15 requirements are already implemented; container isolation is explicitly out of scope.

## Technical Context

**Language/Version**: Go 1.21  
**Primary Dependencies**: cobra (CLI), testify (testing), golang.org/x/sys (Windows APIs), gopkg.in/yaml.v3  
**Storage**: N/A (file-based state, no databases)  
**Testing**: `go test -race -cover ./...`, integration tests via `//go:build integration` tag  
**Target Platform**: Linux (amd64/arm64), macOS (amd64/arm64), Windows (amd64)  
**Project Type**: Single Go project (CLI tool)  
**Performance Goals**: Regex pathological pattern matching < 100ms; process group cleanup < 5 seconds  
**Constraints**: No CGo dependencies; zero external runtime requirements for binary users  
**Scale/Scope**: ~4K LOC, 5 platform targets, 1 composite GitHub Action

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is in template form (not customized for this project). No project-specific gates defined. **PASS** — no violations possible.

**Post-Phase 1 re-check**: Design introduces no new abstractions, no new packages, no new data storage. All changes are in existing files (`exec_unix.go`, `root.go`, `bench_test.go`, `README.md`, `SECURITY.md`) plus new infrastructure files (`.goreleaser.yaml`, `release.yml`, `action.yml`). **PASS**.

## Project Structure

### Documentation (this feature)

```text
specs/011-v1-release-readiness/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: research findings
├── data-model.md        # Phase 1: entity definitions
├── quickstart.md        # Phase 1: verification guide
├── contracts/
│   ├── goreleaser.md    # .goreleaser.yaml schema
│   ├── release-workflow.md  # release.yml schema
│   ├── github-action.md     # action.yml schema
│   └── signal-forwarding.md # exec_unix.go contract
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (modified/created files)

```text
.
├── action.yml                          # NEW — reusable GitHub Action
├── .goreleaser.yaml                    # NEW — release configuration
├── .github/workflows/
│   ├── ci.yml                          # EXISTING (unchanged)
│   └── release.yml                     # NEW — tag-triggered release
├── cmd/
│   ├── exec.go                         # EXISTING (unchanged)
│   ├── exec_unix.go                    # MODIFY — process group + group signals
│   ├── exec_windows.go                 # EXISTING (unchanged)
│   └── root.go                         # MODIFY — add Commit/Date vars
├── internal/matcher/
│   └── bench_test.go                   # MODIFY — add BenchmarkRegexPathological
├── README.md                           # MODIFY — add badges
└── SECURITY.md                         # MODIFY — add RE2 + SIGKILL sections
```

**Structure Decision**: Single Go project — no new packages or directories. All changes are in existing source files or new infrastructure files at the repo root or `.github/`.

## Complexity Tracking

No constitution violations. No complexity justification needed.
