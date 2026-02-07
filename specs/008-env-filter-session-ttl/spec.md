# Feature Specification: Environment Variable Filtering & Session TTL

**Feature Branch**: `008-env-filter-session-ttl`  
**Created**: 2026-02-07  
**Status**: Draft  
**Input**: User description: "Environment Variable Filtering (deny_env_vars in meta.security) to prevent secret exfiltration; Session TTL (ttl: '5m') to auto-cleanup stale sessions in CI environments"

## Clarifications

### Session 2026-02-07

- Q: Does `deny_env_vars` filtering apply only to environment variable overrides in template rendering, or should it also scrub denied values from `meta.vars` and inline `respond.stdout`/`respond.stderr` text (output scanning)? → A: Template rendering only (env var overrides via `MergeVars`). Inline respond text is static/scenario-authored and `meta.vars` are scenario-defined — neither reads from the host environment dynamically. Note: templates use flat keys (`{{ .key }}`), not a `{{ .Env.* }}` namespace; host env vars enter through `MergeVars` which overrides same-named `meta.vars` keys.
- Q: When a denied env var is substituted with an empty string, should cli-replay log that it was filtered? → A: Log filtered variable names to stderr only when `CLI_REPLAY_TRACE=1` is enabled. Keeps production output clean while providing opt-in diagnostics.
- Q: When TTL cleanup removes a stale session at replay startup, should the current invocation auto-initialize a fresh session and proceed, or exit requiring a re-run? → A: Auto-initialize and proceed. TTL cleanup is designed for unattended CI recovery; requiring a re-run would defeat the self-healing purpose.
- Q: Should `allow_env_vars` (default-deny allowlist posture) also be supported alongside `deny_env_vars`? → A: Deny-list only for this feature. An allow-list is a different security posture with higher complexity; defer to a future feature if demand arises.

## User Scenarios & Testing *(mandatory)*

### User Story 1 – Deny Environment Variables in Untrusted Scenarios (Priority: P1)

As a platform engineer who accepts scenario files from external contributors (e.g., community-submitted TSGs), I need to prevent scenario stdout templates from reading sensitive environment variables (API keys, tokens, connection strings) so that executing an untrusted scenario cannot exfiltrate secrets through replay output.

**Why this priority**: This is the highest-value security control. Without it, any scenario that declares a `meta.vars` key matching a sensitive env var name (e.g., `SECRET_KEY`) can leak secrets via `{{ .SECRET_KEY }}` in a respond.stdout template to the console or CI logs. This directly affects the trust model documented in the ROADMAP: "untrusted scenarios (external PRs) — needs env var filtering."

**Independent Test**: Can be fully tested by creating a scenario with `deny_env_vars` configured and verifying that denied variables resolve to empty strings in template output, while allowed variables render normally.

**Acceptance Scenarios**:

1. **Given** a scenario with `meta.security.deny_env_vars: ["AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN"]` and `meta.vars: {AWS_SECRET_ACCESS_KEY: ""}`, **When** a step respond.stdout contains `{{ .AWS_SECRET_ACCESS_KEY }}` and the host env has that variable set, **Then** the env override is suppressed — the rendered output contains an empty string (the `meta.vars` default) and the replay succeeds.
2. **Given** a scenario with `meta.security.deny_env_vars: ["DB_PASSWORD"]` and `meta.vars: {DB_PASSWORD: ""}`, and the host has `DB_PASSWORD=hunter2` set, **When** the scenario is replayed, **Then** the value `hunter2` never appears in stdout, stderr, or any output stream.
3. **Given** a scenario with `deny_env_vars` configured, **When** a step template references a variable NOT in the deny list (e.g., `{{ .HOME }}` with `meta.vars: {HOME: ""}`), **Then** the variable renders its actual host env value normally.
4. **Given** a scenario with `deny_env_vars: ["*"]` (wildcard deny-all), **When** any template expression references a `meta.vars` key that has a same-named host env var, **Then** all environment variable overrides are suppressed and `meta.vars` defaults are used.
5. **Given** a scenario without any `meta.security` section, **When** templates reference environment variables, **Then** all variables render normally (backward-compatible behavior; no filtering applied).
6. **Given** a scenario with `deny_env_vars` containing glob patterns like `AWS_*` and `meta.vars: {AWS_ACCESS_KEY_ID: "", AWS_SECRET_ACCESS_KEY: ""}`, **When** a template references `{{ .AWS_ACCESS_KEY_ID }}` or `{{ .AWS_SECRET_ACCESS_KEY }}`, **Then** both env var overrides are denied and the `meta.vars` defaults (empty strings) are used.

---

### User Story 2 – Session TTL for Automatic Stale Session Cleanup (Priority: P1)

As a CI/CD pipeline operator running cli-replay on self-hosted Jenkins or bare-metal CI agents, I need stale replay sessions to be automatically cleaned up after a configurable time limit so that disk space is not exhausted by orphaned `.cli-replay/` state and intercept directories when processes are killed by `SIGKILL` or CI timeout mechanisms (which bypass signal traps).

**Why this priority**: Signal traps (`EXIT`, `INT`, `TERM`) handle graceful cleanup, but `SIGKILL` (sent by CI timeout mechanisms, OOM killer, etc.) cannot be caught. On persistent CI agents, orphaned sessions accumulate and risk disk exhaustion. This was identified as a P0 gap for "self-hosted persistent CI" readiness in the ROADMAP.

**Independent Test**: Can be fully tested by creating a state file with a `last_updated` timestamp older than the TTL, running a replay or clean command, and verifying the stale session is automatically removed.

**Acceptance Scenarios**:

1. **Given** a scenario with `meta.session.ttl: "5m"` and a state file whose `last_updated` is 10 minutes ago, **When** `cli-replay run` or `cli-replay exec` is invoked for that scenario, **Then** the stale session (state file + intercept directory) is cleaned up before starting a new session.
2. **Given** a scenario with `meta.session.ttl: "30m"` and a state file whose `last_updated` is 5 minutes ago, **When** a replay command is invoked, **Then** the existing session is preserved and replay continues normally.
3. **Given** a scenario with `meta.session.ttl: "1h"` and multiple state files in `.cli-replay/` (from parallel sessions), **When** cleanup is triggered, **Then** only sessions whose `last_updated` exceeds the TTL are removed; active sessions within TTL are preserved.
4. **Given** a scenario without any `meta.session` section, **When** sessions are used, **Then** no automatic TTL cleanup occurs (backward-compatible behavior; sessions persist until explicit `cli-replay clean`).
5. **Given** a `cli-replay clean` command with `--ttl 10m` flag override (note: `--ttl` is a `clean`-only flag, not available on `run`/`exec`), **When** invoked, **Then** the flag value takes precedence over the scenario's `meta.session.ttl` for that invocation.
6. **Given** a scenario with `meta.session.ttl: "5m"` and a state file from a different session (different `CLI_REPLAY_SESSION` value) that is expired, **When** a new session starts, **Then** the expired session's state file and its associated intercept directory are both removed.

---

### User Story 3 – Proactive Cleanup on `cli-replay clean` with TTL Awareness (Priority: P2)

As a DevOps engineer managing CI infrastructure, I want to run a periodic cleanup command that removes all expired sessions across all scenarios in a directory so that I can maintain disk hygiene without tracking individual scenario paths.

**Why this priority**: Complements Story 2 by providing a manual/cron-driven cleanup mechanism. Less urgent because Story 2 handles the common case (cleanup at next run), but important for environments where scenarios may not be re-run for extended periods.

**Independent Test**: Can be tested by creating multiple `.cli-replay/` directories with expired state files and running a single cleanup command to verify all are removed.

**Acceptance Scenarios**:

1. **Given** multiple scenario directories each containing `.cli-replay/` with expired state files, **When** `cli-replay clean --ttl 10m --recursive .` is invoked, **Then** all state files and intercept directories older than 10 minutes are removed across all discovered scenarios.
2. **Given** a mix of expired and active sessions, **When** recursive cleanup runs, **Then** only expired sessions are removed and active sessions are untouched.
3. **Given** no expired sessions exist, **When** cleanup runs, **Then** the command completes successfully with a message indicating zero sessions cleaned.

---

### Edge Cases

- What happens when the system clock is adjusted (e.g., NTP sync) and `last_updated` appears to be in the future?  
  **Assumption**: Sessions with `last_updated` in the future are treated as active (not expired). A warning is emitted to stderr.

- What happens when a `deny_env_vars` pattern matches a cli-replay internal variable (e.g., `CLI_REPLAY_SESSION`)?  
  **Assumption**: Internal variables (`CLI_REPLAY_*`) are never filtered regardless of `deny_env_vars` patterns, to prevent breaking replay mechanics.

- What happens when the `.cli-replay/` directory has file permission issues during TTL cleanup?  
  **Assumption**: Cleanup logs a warning to stderr for each file/directory that cannot be removed but continues processing remaining files (best-effort).

- What happens when two parallel sessions attempt TTL cleanup simultaneously?  
  **Assumption**: Cleanup is idempotent; if a file is already removed by another process, the error is silently ignored.

## Requirements *(mandatory)*

### Functional Requirements

#### Environment Variable Filtering

- **FR-001**: The scenario data model MUST support a `deny_env_vars` field under `meta.security` that accepts a list of strings.
- **FR-002**: Each entry in `deny_env_vars` MUST support exact variable name matching (e.g., `"AWS_SECRET_ACCESS_KEY"`).
- **FR-003**: Each entry in `deny_env_vars` MUST support glob-style wildcard patterns (e.g., `"AWS_*"`, `"*_TOKEN"`, `"*"`).
- **FR-004**: When a template key has a same-named host environment variable that matches a `deny_env_vars` pattern, the system MUST suppress the env var override during `MergeVars` — the original `meta.vars` value is preserved (or empty string if no `meta.vars` entry exists). Note: templates use flat keys (`{{ .key }}`); env vars enter through `MergeVars` which overrides same-named `meta.vars` keys. Filtering is applied at this merge point; inline `respond.stdout`/`respond.stderr` text and `meta.vars` values themselves are not scanned or filtered.
- **FR-005**: Variables NOT matching any `deny_env_vars` pattern MUST render their actual values unchanged.
- **FR-006**: The filtering MUST apply to env var overrides within all rendering contexts: `respond.stdout`, `respond.stderr`, `respond.stdout_file` content, and `respond.stderr_file` content. Because all contexts flow through `MergeVars` → `Render`, filtering at `MergeVars` automatically covers all contexts. No output-level value scrubbing is performed.
- **FR-007**: Internal cli-replay variables (`CLI_REPLAY_SESSION`, `CLI_REPLAY_SCENARIO`, `CLI_REPLAY_RECORDING_LOG`, `CLI_REPLAY_SHIM_DIR`) MUST be exempt from deny filtering regardless of patterns.
- **FR-008**: When `deny_env_vars` is absent or empty, all environment variables MUST render normally (backward compatibility).
- **FR-009**: The scenario validation step MUST reject `deny_env_vars` entries that are empty strings.
- **FR-010**: When `CLI_REPLAY_TRACE=1` is set and a denied variable is substituted, the system MUST emit a trace line to stderr with the variable name (e.g., `cli-replay[trace]: denied env var AWS_SECRET_ACCESS_KEY`). When trace is disabled, no output is produced for filtered variables.

#### Session TTL

- **FR-011**: The scenario data model MUST support a `ttl` field under `meta.session` that accepts a duration string (e.g., `"5m"`, `"1h"`, `"30s"`).
- **FR-012**: The system MUST validate the TTL duration format during scenario loading and reject invalid values.
- **FR-013**: When a replay operation begins (`run`, `exec`, or intercept), the system MUST scan the `.cli-replay/` directory for state files whose `last_updated` timestamp exceeds the configured TTL.
- **FR-014**: For each expired state file, the system MUST remove both the state file and its associated intercept directory (if recorded in the state). After cleanup, the current invocation MUST auto-initialize a fresh session and proceed with replay (no re-run required).
- **FR-015**: Active sessions (within TTL) MUST NOT be affected by TTL cleanup.
- **FR-016**: The `cli-replay clean` command MUST accept an optional `--ttl` flag that overrides the scenario's configured TTL for that invocation.
- **FR-017**: When no `meta.session.ttl` is configured and no `--ttl` flag is provided, no automatic TTL cleanup MUST occur (backward compatibility).
- **FR-018**: TTL cleanup MUST be idempotent — concurrent cleanup attempts for the same session MUST NOT produce errors.
- **FR-019**: TTL cleanup MUST log a summary to stderr indicating how many sessions were cleaned up (e.g., `cli-replay: cleaned 3 expired sessions`).
- **FR-020**: The `cli-replay clean` command MUST support a `--recursive` flag that discovers and cleans `.cli-replay/` directories under a given path.

### Key Entities

- **Security Configuration** (`meta.security`): Extended with `deny_env_vars` alongside existing `allowed_commands`. Represents the trust boundary for scenario execution.
- **Session Configuration** (`meta.session`): New section containing `ttl` duration. Governs session lifecycle and automatic cleanup behavior.
- **State File** (`cli-replay-*.state`): Existing entity with `last_updated` timestamp. TTL calculations use this timestamp to determine expiration.
- **Intercept Directory** (`.cli-replay/intercept-*`): Temporary directory created per-session. Must be cleaned up alongside expired state files.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: No denied environment variable value appears in any output stream when `deny_env_vars` is configured — 100% filtering accuracy.
- **SC-002**: Existing scenarios without `meta.security.deny_env_vars` continue to function identically — zero regressions.
- **SC-003**: On a CI agent with 50 orphaned sessions, TTL cleanup recovers disk space within one subsequent `run`/`exec` invocation — cleanup completes in under 2 seconds.
- **SC-004**: Existing scenarios without `meta.session.ttl` continue to function identically — zero regressions in session persistence behavior.
- **SC-005**: Parallel sessions with different `CLI_REPLAY_SESSION` values operate independently — TTL cleanup of one expired session does not affect another active session.
- **SC-006**: Users can configure both `deny_env_vars` and `ttl` in the same scenario without conflicts — features are composable.

## Assumptions

- **Duration format**: TTL durations follow Go's `time.ParseDuration` conventions (e.g., `"5m"`, `"1h30m"`, `"300s"`). This is consistent with the existing `delay` field in `respond`.
- **Glob matching**: Glob patterns in `deny_env_vars` use simple prefix/suffix wildcard matching (`*` at start, end, or both), not full filesystem glob semantics. This keeps the matching logic simple and predictable.
- **Cleanup scope**: TTL cleanup only removes files within `.cli-replay/` directories. It never modifies scenario files, test fixtures, or any files outside the `.cli-replay/` directory tree.
- **Logging verbosity**: TTL cleanup messages are written to stderr (consistent with existing cli-replay diagnostic messages) and only appear when sessions are actually cleaned.
- **Performance**: TTL cleanup performs a single directory listing per `.cli-replay/` directory. No recursive filesystem walks are performed during normal replay operations (only during `--recursive` clean).

## Out of Scope

- **`allow_env_vars` (default-deny allowlist)**: A complementary allowlist-based security posture is intentionally deferred. The deny-list model (`deny_env_vars`) covers the stated exfiltration-prevention use case. An allowlist can be added in a future feature without breaking backward compatibility.
- **Output-level value scrubbing**: Scanning all output streams for denied variable values is not performed. Filtering applies exclusively to `{{ .Env.* }}` template expressions (the only dynamic host-environment access vector).
- **Encrypted or redacted placeholders**: Denied variables are substituted with empty strings, not with redacted markers like `***REDACTED***`. Placeholder text could be considered in a future enhancement.
