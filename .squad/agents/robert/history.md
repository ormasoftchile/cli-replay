# Project Context

- **Owner:** Cristián
- **Project:** cli-replay — A Go framework for instrumenting tools/command calls from workflows/runbooks, enabling replay scenarios without faking from the consumer side.
- **Stack:** Go, CLI, GitHub Actions
- **Created:** 2026-04-03

## Learnings

<!-- Append new learnings below. Each entry is something lasting about the project. -->

### 2026-04-03 — Integration Analysis for gert

**Architecture:**
- All functional code lives under `internal/` — no public Go library API exists today.
- The only importable package is `cmd` (exports `Execute()` and `ValidationResult`).
- Key internal packages: `scenario` (types + loading), `runner` (replay engine + state), `matcher` (argv matching), `recorder` (capture + YAML gen), `verify` (results + JSON/JUnit), `template` (Go text/template), `platform` (OS abstraction), `envfilter` (deny patterns).
- CLI is the primary integration surface: `exec`, `run`, `verify`, `validate`, `record`, `clean`.
- `exec` is the recommended CI command — full lifecycle in one invocation.
- `action.yml` is setup-only (installs binary, does not run scenarios).
- Scenario schema is defined in `schema/scenario.schema.json`.

**Key file paths:**
- `main.go` — dual-mode entry point (management vs intercept based on argv[0])
- `cmd/root.go` — Cobra command tree root
- `cmd/exec.go` — exec subcommand (recommended for CI)
- `cmd/run.go` — run subcommand (eval pattern)
- `cmd/record.go` — record subcommand
- `cmd/verify.go` — verify with JSON/JUnit output
- `cmd/validate.go` — static schema validation
- `internal/runner/` — replay engine, state, mismatch errors
- `internal/scenario/` — Scenario, Step, Meta, Match, Response types
- `internal/matcher/` — ArgvMatch, regex/wildcard support
- `internal/verify/` — VerifyResult, JUnit formatting
- `examples/cookbook/` — copy-paste scenario+script pairs (terraform, helm, kubectl)

**Integration patterns:**
- For gert: CLI wrapper via `cli-replay exec --format json` is the immediate path.
- Schema-driven YAML generation is the medium-term approach.
- Public `pkg/` API promotion is the high-value upstream contribution.

**Gaps identified:**
- No library API (everything is `internal/`)
- No partial replay (start/end step)
- No step-level metadata (description, tags, id)
- `when` conditional field exists but is not evaluated yet
- No scenario composition (`include` directive)
- No event hooks for observability

**Decision:** Filed `robert-gert-integration-analysis.md` in decisions inbox recommending CLI wrapper now, library API later.

## 2026-04-03  Integration Decision Filed

- ** Decision:** Filed robert-gert-integration-analysis.md
- **Recommendation:** Pattern A (CLI wrapper) for Phase 1, CLI \xec --format json\ provides structured integration
- **Roadmap:** Phase 3 targets library API promotion (\pkg/\ migration) for high-value upstream contribution
- **Critical gap:** All functional code is under \internal/\  blocking gert library integration
- **Seven-item priority roadmap** for cli-replay improvements identified (library API, step metadata, partial replay, when conditions, scenario composition, event hooks, programmatic builder)
