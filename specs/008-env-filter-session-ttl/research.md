# Research: Environment Variable Filtering & Session TTL

**Feature**: `008-env-filter-session-ttl`  
**Date**: 2026-02-07

## R1: Template Data Model & Env Var Access

**Question**: How do environment variables flow into template rendering, and where should deny filtering be applied?

**Finding**: The current template system uses **flat keys**, not namespaced `{{ .Env.* }}` syntax:

- `meta.vars` declares template keys (e.g., `cluster: "dev"`)
- `template.MergeVars(meta.Vars)` copies these keys and overrides with same-named env vars
- Templates reference as `{{ .cluster }}`, not `{{ .Env.cluster }}`
- `ReplayResponseWithTemplate` calls `MergeVars` → `Render`, passing the merged map

**Decision**: Filtering should be applied inside `MergeVars` (or a new variant that accepts deny patterns). When an env var name matches a deny pattern, the env override is suppressed (the original `meta.vars` value is used, or empty string if no base value exists). This is the single chokepoint where host env values enter the template pipeline.

**Rationale**: Modifying `MergeVars` is the least-invasive change. It affects all template rendering contexts (stdout, stderr, stdout_file, stderr_file) automatically since they all call `MergeVars` → `Render`.

**Alternatives considered**:
- Adding an `{{ .Env.* }}` namespace to the template model: Would be a breaking change to the existing template syntax. Rejected.
- Post-render output scrubbing: Complex, error-prone (partial value matches), and explicitly out of scope per clarification.

## R2: Glob Pattern Matching for deny_env_vars

**Question**: What Go mechanism should be used for matching env var names against glob patterns like `AWS_*`?

**Finding**: Go's `path.Match` function supports `*` wildcards but it does NOT match path separators. Since env var names don't contain `/`, `path.Match` works correctly for patterns like:
- `AWS_*` → matches `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`
- `*_TOKEN` → matches `GITHUB_TOKEN`, `NPM_TOKEN`
- `*` → matches everything

**Decision**: Use `path.Match` from Go stdlib for glob matching.

**Rationale**: Standard library, zero dependencies, well-tested, handles the exact pattern syntax described in the spec (prefix/suffix wildcards). No need for regex or custom matching.

**Alternatives considered**:
- Hand-rolled prefix/suffix matching: Simpler but less flexible (no `DB_*_PASSWORD` patterns). Rejected.
- `filepath.Match`: Identical to `path.Match` for non-path strings. Either works.
- Regex: Overkill for the stated patterns; worse UX for scenario authors. Rejected.

## R3: Scanning `.cli-replay/` for Expired State Files

**Question**: How should TTL cleanup discover all state files in a `.cli-replay/` directory?

**Finding**: State files are named `cli-replay-{hash}.state` inside `.cli-replay/` adjacent to the scenario file. Intercept directories are named `intercept-{random}`. The current code only reads a specific state file by path (via `StateFilePath`).

**Decision**: Use `os.ReadDir` to list `.cli-replay/` directory contents, filter entries matching `cli-replay-*.state` glob, parse each to check `last_updated` against TTL.

**Rationale**: `os.ReadDir` is the idiomatic Go approach. The `.cli-replay/` directory is flat (no subdirectories to recurse for state files). Each state file embeds the intercept directory path in its `intercept_dir` JSON field, so cleanup can find and remove the associated intercept directory.

**Alternatives considered**:
- `filepath.Glob`: Simpler API but allocates path strings. `os.ReadDir` is more efficient for large directories.
- Tracking sessions in a separate manifest file: Adds complexity and another file to maintain. Rejected — the state files themselves are the manifest.

## R4: Concurrent TTL Cleanup Safety

**Question**: How to ensure idempotent TTL cleanup when multiple sessions clean simultaneously?

**Finding**: Go's `os.Remove` returns `os.ErrNotExist` when the target doesn't exist. `os.RemoveAll` returns nil for non-existent paths. The existing `DeleteState` already handles the `os.ErrNotExist` case.

**Decision**: TTL cleanup should use the same pattern as existing `DeleteState` — attempt removal and silently ignore `os.ErrNotExist`. No file locking is needed.

**Rationale**: The operations (read state → check TTL → remove) have a TOCTOU race, but the worst case is harmless: two processes both try to remove the same file, one succeeds, the other gets `ErrNotExist` which is ignored. No data corruption is possible because state files are consumed (not shared) artifacts.

**Alternatives considered**:
- Advisory file locking (`flock`): Adds complexity, platform-specific, not needed for this use case.
- Atomic lock directory: Overkill for best-effort cleanup.

## R5: Recursive Clean Discovery

**Question**: How should `--recursive` discover `.cli-replay/` directories under a path?

**Decision**: Use `filepath.WalkDir` with early pruning — skip `.git`, `node_modules`, and other common non-scenario directories. Only process directories named `.cli-replay` that contain state files.

**Rationale**: `filepath.WalkDir` is Go's idiomatic recursive directory walker, efficiently handles large trees with the `fs.DirEntry` interface (no stat calls needed for type checking).

**Alternatives considered**:
- `filepath.Glob("**/.cli-replay")`: Go's `Glob` doesn't support `**`. Rejected.
- User provides explicit scenario paths: Defeats the purpose of `--recursive`. Rejected.
