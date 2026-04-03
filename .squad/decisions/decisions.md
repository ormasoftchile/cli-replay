# Decisions

## Decision: `gert run` as Separate Command (Not Flags on `exec`)

**Author:** Robert (Integration Architect)
**Date:** 2026-04-03
**Status:** Implemented

### Context

The task was to wire cli-replay's `--record` and `--replay` flags into gert's CLI. Two approaches were considered:

1. **Add flags to `gert exec`** — the existing execution command
2. **Create a new `gert run` subcommand** — a parallel, streamlined command

### Decision

Created `gert run` as a new subcommand rather than modifying `gert exec`.

### Rationale

1. **Naming conflict:** `gert exec` already has a `--record <dir>` flag (string) that exports gert's own scenario format to a directory. The cli-replay `--record` is a bool toggle. Same flag name, incompatible semantics.

2. **Separation of concerns:** `exec` is the power-user command with `--mode`, `--scenario`, `--format jsonl`, `--resume`, `--rebase-time`. Adding cli-replay flags would further overload it. `run` is a focused command for the common workflow.

3. **User story clarity:** `gert run --record` vs `gert exec --mode real --record ./scenarios/` — the `run` form communicates intent more clearly.

4. **Zero disruption:** No changes to `main.go` or existing commands. The `init()` in `run.go` self-registers via cobra.

### Consequences

- `gert run` and `gert exec` both execute runbooks — users need to understand when to use which
- `gert runs` (plural, lists past runs) is very close in name — potential confusion
- Future: may want to consolidate or alias these commands
- `gert run` does not support `--format jsonl`, `--resume`, or `--rebase-time` — these remain exec-only features

### Files Changed

- `cmd/gert/run.go` (new, ~220 LOC)
