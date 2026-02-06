<!--
SYNC IMPACT REPORT
==================
Version change: 0.0.0 → 1.0.0 (Initial ratification)

Modified principles: N/A (initial version)

Added sections:
  - I. Test-First Development (NON-NEGOTIABLE)
  - II. Quality Gates
  - III. Single Binary Distribution
  - IV. Idiomatic Go
  - V. Simplicity & YAGNI
  - Technology Stack
  - Development Workflow

Removed sections: N/A (initial version)

Templates requiring updates:
  ✅ plan-template.md - Constitution Check section ready for principle validation
  ✅ spec-template.md - User scenarios align with test-first approach
  ✅ tasks-template.md - Test-first task ordering compatible

Follow-up TODOs: None
-->

# cli-replay Constitution

## Core Principles

### I. Test-First Development (NON-NEGOTIABLE)

All features MUST follow test-driven development:

1. **Write tests FIRST** — Tests define expected behavior before any implementation
2. **Tests MUST fail** — Verify tests fail with meaningful messages before writing code
3. **Implement to pass** — Write minimal code to make tests pass
4. **Refactor with safety net** — Improve code only when tests are green

**Rationale**: cli-replay is a testing tool; dogfooding TDD ensures the tool's design
serves real testing needs and maintains confidence in every release.

**Enforcement**:
- PRs without corresponding test changes for new functionality MUST be rejected
- Coverage MUST NOT decrease; new code requires ≥80% line coverage
- `go test ./...` MUST pass before any merge

### II. Quality Gates

Every task MUST pass these gates before completion:

| Gate | Requirement |
|------|-------------|
| **Unit Tests** | All new/modified functions have table-driven tests |
| **Integration Tests** | CLI commands tested end-to-end with scenario files |
| **Linting** | `golangci-lint run` passes with zero warnings |
| **Formatting** | `gofmt -s` and `goimports` applied |
| **Documentation** | Public APIs have godoc comments; CLI flags have help text |

**Rationale**: Automated gates catch defects early and maintain consistent quality
across contributors.

### III. Single Binary Distribution

cli-replay MUST ship as a single static binary:

- No external runtime dependencies (no Python, Node, etc.)
- Cross-compile for Windows, macOS (arm64/amd64), Linux (arm64/amd64)
- Binary size SHOULD remain under 20MB
- `CGO_ENABLED=0` for maximum portability

**Rationale**: Users invoke cli-replay as a drop-in fake executable; complex
installation defeats the tool's purpose of simplifying test setup.

### IV. Idiomatic Go

Code MUST follow Go idioms and conventions:

- Use `error` returns, not panics, for recoverable failures
- Prefer composition over inheritance; embed interfaces judiciously
- Use `context.Context` for cancellation and timeouts
- Avoid global state; pass dependencies explicitly
- Follow [Effective Go](https://go.dev/doc/effective_go) and
  [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)

**Rationale**: Idiomatic code is predictable, readable, and maintainable by any
Go developer.

### V. Simplicity & YAGNI

- **Start simple** — Implement the minimal solution that satisfies requirements
- **No speculative features** — Do not add functionality "just in case"
- **Complexity requires justification** — Any deviation from the simplest approach
  MUST be documented with rationale in the PR or spec

**Rationale**: cli-replay targets deterministic, sequential CLI replay; complexity
undermines reliability and debuggability.

## Technology Stack

| Component | Choice | Constraints |
|-----------|--------|-------------|
| **Language** | Go 1.21+ | Use latest stable; no beta features |
| **CLI Framework** | `cobra` | Standard flags; `--help` and `--version` required |
| **YAML Parsing** | `gopkg.in/yaml.v3` | Strict mode; reject unknown fields |
| **Testing** | `testing` + `testify/assert` | Table-driven tests preferred |
| **Linting** | `golangci-lint` | Config in `.golangci.yml` |
| **Build** | `go build` / `goreleaser` | Reproducible builds via Makefile |

## Development Workflow

### Branch Strategy

- `main` — Always releasable; protected branch
- `feat/###-description` — Feature branches
- `fix/###-description` — Bug fix branches

### Commit Standards

- Use conventional commits: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`
- Reference issue numbers: `feat: add scenario validation (#42)`
- Keep commits atomic; one logical change per commit

### Code Review Requirements

1. All PRs require at least one approval
2. Reviewer MUST verify:
   - Tests exist and are meaningful
   - Quality gates pass (CI green)
   - Constitution principles are followed
3. Self-merge allowed only for documentation typos

## Governance

This constitution supersedes all informal practices and ad-hoc decisions.

**Amendment Process**:
1. Propose changes via PR to this file
2. Document rationale and migration impact
3. Require approval from at least two maintainers
4. Update `LAST_AMENDED_DATE` and increment version per SemVer:
   - MAJOR: Principle removal or backward-incompatible governance change
   - MINOR: New principle or significant guidance expansion
   - PATCH: Clarifications, typo fixes, non-semantic updates

**Compliance**:
- All PRs MUST pass Constitution Check in implementation plans
- Violations require explicit justification in the Complexity Tracking section
- Repeated violations warrant team retrospective

**Version**: 1.0.0 | **Ratified**: 2026-02-05 | **Last Amended**: 2026-02-05
