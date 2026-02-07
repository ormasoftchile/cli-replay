# Implementation Plan: Environment Variable Filtering & Session TTL

**Branch**: `008-env-filter-session-ttl` | **Date**: 2026-02-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/008-env-filter-session-ttl/spec.md`

## Summary

Add two security/reliability features to cli-replay: (1) `deny_env_vars` glob-pattern filtering in `meta.security` to prevent host environment variable exfiltration through template rendering, and (2) `meta.session.ttl` auto-cleanup of stale replay sessions that survive SIGKILL/CI timeouts. Both features are backward-compatible additive changes to the existing data model, template system, and CLI commands.

## Technical Context

**Language/Version**: Go 1.21+
**Primary Dependencies**: `github.com/spf13/cobra` (CLI), `gopkg.in/yaml.v3` (YAML parsing), `text/template` (Go stdlib), `path` (glob matching)
**Storage**: File-based (`.cli-replay/cli-replay-*.state` JSON files, `.cli-replay/intercept-*` temp dirs)
**Testing**: `go test` + `github.com/stretchr/testify/assert` + `github.com/stretchr/testify/require`
**Target Platform**: Linux, macOS, Windows (cross-platform)
**Project Type**: Single binary CLI tool
**Performance Goals**: TTL cleanup of 50 sessions < 2 seconds (SC-003)
**Constraints**: Zero breaking changes; all existing scenarios must work identically (SC-002, SC-004)
**Scale/Scope**: ~15 files modified/created, ~600 LOC added

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The constitution template is unpopulated (placeholder content only). No project-specific constraints are defined. Gate passes by default — no violations to evaluate.

**Post-Phase-1 re-check**: No violations. The design adds two optional fields to existing structs, modifies one function signature (`MergeVars`), and extends one CLI command (`clean`). All changes are additive and backward-compatible.

## Project Structure

### Documentation (this feature)

```text
specs/008-env-filter-session-ttl/
├── plan.md              # This file
├── research.md          # Phase 0 output — 5 research decisions
├── data-model.md        # Phase 1 output — entity changes
├── quickstart.md        # Phase 1 output — usage guide
├── contracts/
│   └── cli.md           # Phase 1 output — CLI contract changes
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── scenario/
│   ├── model.go          # MODIFY: Add DenyEnvVars to Security, add Session struct to Meta
│   ├── model_test.go     # MODIFY: Add tests for new fields, validation, YAML parsing
│   └── loader.go         # MODIFY: (if needed) for Session YAML unmarshaling
├── template/
│   ├── render.go         # MODIFY: Add MergeVarsFiltered() with deny pattern support
│   └── render_test.go    # MODIFY: Tests for filtering, glob matching, exemptions
├── runner/
│   ├── state.go          # MODIFY: Add CleanExpiredSessions() function
│   ├── state_test.go     # MODIFY: Tests for TTL scanning, cleanup, idempotency
│   ├── replay.go         # MODIFY: Call TTL cleanup before replay, pass deny list to template
│   ├── replay_test.go    # MODIFY: Integration tests for TTL + deny filtering
│   └── trace.go          # MODIFY: Add WriteDeniedEnvTrace() for FR-010
├── envfilter/
│   ├── filter.go         # NEW: GlobMatch, IsDenied, IsExempt functions
│   └── filter_test.go    # NEW: Tests for glob patterns, wildcards, exemptions
cmd/
├── clean.go              # MODIFY: Add --ttl and --recursive flags
├── clean_test.go         # MODIFY: Tests for new flags
├── run.go                # MODIFY: Call TTL cleanup at session start
└── exec.go               # MODIFY: Call TTL cleanup at session start
schema/
└── scenario.schema.json  # MODIFY: Add deny_env_vars and session definitions
```

**Structure Decision**: Single project layout (existing). New code goes in existing packages with one new `internal/envfilter` package for the glob-matching logic (keeps it independently testable and reusable).

## Key Design Decisions

### D1: Filtering at MergeVars level (R1)

Environment variable filtering is applied inside `template.MergeVars` (or a new `MergeVarsFiltered` function). This is the single chokepoint where host env values enter the template data map. All rendering contexts (stdout, stderr, stdout_file, stderr_file) automatically benefit because they all flow through MergeVars → Render.

**Why not a new `{{ .Env.* }}` namespace**: Would be a breaking change to existing template syntax. Current templates use flat keys (`{{ .cluster }}`), and env vars override via `MergeVars`. Preserving this model means zero changes to existing scenarios.

### D2: `path.Match` for glob patterns (R2)

Go's `path.Match` is used for deny_env_vars pattern matching. It handles `*` wildcards correctly for non-path strings (env var names don't contain `/`). Zero dependencies.

### D3: State file scanning for TTL (R3)

`os.ReadDir` on `.cli-replay/` + filter `cli-replay-*.state` glob + parse each JSON to check `last_updated`. Each state file already contains `intercept_dir` for associated cleanup. No manifest file needed.

### D4: `--recursive` requires `--ttl` (contract)

The `--recursive` flag on `clean` requires `--ttl` to prevent accidental deletion of all sessions. This is a safety guard for cron/automation scenarios.

## Complexity Tracking

No constitution violations to justify. Both features are additive changes within the existing architecture.
