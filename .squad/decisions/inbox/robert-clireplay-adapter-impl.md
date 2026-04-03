# Decision: cli-replay Adapter Implementation in Gert

**Author:** Robert (Integration Architect)  
**Date:** 2026-04-03  
**Status:** Implemented  
**Branch:** `feature/clireplay-adapter` (in gert repo)

## Context

The cli-replay pkg/ promotion is complete. `pkg/scenario`, `pkg/matcher`, `pkg/replay`, and `pkg/verify` are now public Go packages. The team decided the gert adapter lives in gert's repo.

## Decision

Built `pkg/providers/clireplay/` in gert with two CommandExecutor implementations:

1. **ReplayExecutor** — thin wrapper around `replay.Engine.Match()`, converting types
2. **RecordingExecutor** — decorator pattern capturing commands for later Save() as cli-replay YAML

Wired `--mode clireplay` into `gert exec` alongside existing modes.

## Rationale

- Keeps cli-replay consumer-agnostic (no gert types leak in)
- Adapter is ~210 LOC total — thin enough to maintain, rich enough to be useful
- Uses cli-replay's YAML format directly — no format bridging needed
- RecordingExecutor enables "record in dev, replay in CI" workflow
- Local `replace` directive in go.mod until module is published

## Consequences

- Gert now depends on `github.com/cli-replay/cli-replay` (via local replace)
- Users can run `gert exec --mode clireplay --scenario path/to/scenario.yaml`
- Future work: HybridExecutor (fallback to real for unmatched), evidence collector adapter
- Need to publish cli-replay module or maintain workspace-level replace directive

## Files Changed

- `pkg/providers/clireplay/replay_executor.go` (new)
- `pkg/providers/clireplay/recording_executor.go` (new)
- `pkg/providers/clireplay/options.go` (new)
- `pkg/providers/clireplay/clireplay_test.go` (new, 12 tests)
- `cmd/gert/main.go` (added clireplay mode)
- `go.mod` (added dependency + replace)
