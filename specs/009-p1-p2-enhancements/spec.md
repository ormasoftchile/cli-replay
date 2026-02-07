# Feature Specification: P1/P2 Enhancements — Dynamic Capture, Dry-Run, Windows Audit & Benchmarks

**Feature Branch**: `009-p1-p2-enhancements`  
**Created**: 2026-02-07  
**Status**: Draft  
**Input**: User description: "Unordered Step Groups, Windows Compatibility Audit, Dynamic Capture Documentation, Dry-run mode, JUnit XML output for CI dashboards, Performance benchmarks for 100+ step scenarios"

## Clarifications

### Session 2026-02-07

- Q: In unordered groups, if step B references a capture from sibling step A, what happens if B executes before A? → A: Best-effort — sibling captures resolve to empty string if the producing step hasn't run yet; no implicit ordering is introduced.
- Q: Does dry-run output go to stdout or stderr? → A: stdout — shell setup is suppressed in dry-run mode, so stdout is free for human-readable output and supports file redirection.
- Q: When are captures persisted — per-step or in-memory only? → A: Per-step — captures are written to the state file as part of the existing atomic state write after each step, providing crash safety at no additional I/O cost.

## Scope Assessment

The following items from the user request are **already fully implemented** and excluded from this specification:

| Item | Status | Evidence |
|------|--------|----------|
| Unordered Step Groups | ✅ Complete | Model, loader, runner, state, verify, and README all support `group: { mode: unordered }` with barrier semantics, call bounds, and per-step matching |
| JUnit XML output | ✅ Complete | `--format junit` on `verify` and `exec` commands, full test coverage in `internal/verify/junit_test.go` |

The following items **require work** and are specified below:

1. **Dynamic Capture** — capture output from one step and inject it into subsequent steps via template variables
2. **Dry-Run Mode** — preview a scenario's step sequence without creating intercepts or modifying the environment
3. **Windows Signal Handling Audit** — verify and fix signal forwarding and cleanup on Windows
4. **Performance Benchmarks at Scale** — benchmark the replay engine, matcher, and state I/O with 100+ step scenarios

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Dynamic Capture: Chain Output Between Steps (Priority: P1)

A platform engineer writes a multi-step deployment scenario where the output of one command (e.g., a resource ID) is needed as input to subsequent commands. Today, templates can only render static `meta.vars` values or environment variables. With dynamic capture, the engineer defines a capture identifier on a step's response, and later steps reference that captured value in their response bodies.

**Why this priority**: Stateful workflows are one of the most common patterns in real-world CLI orchestration (e.g., `az group create` returns a resource ID used by `az vm create`). Without capture, users must hard-code IDs or use external scripting to bridge step outputs — defeating the purpose of declarative scenarios.

**Independent Test**: Create a 3-step scenario where step 1 captures a resource ID, step 2 captures a VM ID, and step 3's response template references both captured values via `{{ .capture.rg_id }}` and `{{ .capture.vm_id }}`. Run the scenario and verify step 3's output contains both captured values.

**Acceptance Scenarios**:

1. **Given** a scenario with a step whose response defines `capture: { pod_name: "web-0" }`, **When** a later step's `respond.stdout` uses `{{ .capture.pod_name }}`, **Then** the rendered output contains `web-0`.
2. **Given** a scenario with a capture identifier defined in step 3, **When** step 2 (which runs before step 3) references that capture, **Then** the tool reports an error indicating the capture is not yet available.
3. **Given** a scenario where a step defines a capture with an identifier that conflicts with an existing `meta.vars` key, **When** the scenario is loaded, **Then** the tool reports a validation error about the naming conflict.
4. **Given** a scenario in a step group with captures, **When** group steps execute in varied order, **Then** captures from already-executed group steps are available to subsequent steps, but sibling captures from not-yet-executed group steps resolve to empty string (no implicit ordering is introduced).
5. **Given** a scenario with a capture defined but the step is optional (`calls.min: 0`) and never invoked, **When** a later step references that capture, **Then** the captured value resolves to an empty string.

---

### User Story 2 — Dry-Run Mode: Preview Scenario Without Interception (Priority: P1)

A platform engineer wants to validate a scenario file and preview its step sequence before actually running it. Today, the only way to test a scenario is to run it, which creates intercept directories, symlinks/wrappers, and modifies PATH. A dry-run mode would load the scenario, validate it, and print a human-readable summary of each step — including match patterns, response previews, call bounds, and group membership — without making any changes to the file system or environment.

**Why this priority**: Dry-run is a standard developer expectation for any tool that modifies the environment. It enables safe exploration, scenario debugging, and CI preflight validation (e.g., "will this scenario's allowlist match my pipeline commands?").

**Independent Test**: Run `cli-replay run --dry-run scenario.yaml` on a multi-step scenario with groups, call bounds, and template variables. Verify the output lists all steps with their match patterns, expected response summary, call bounds, group membership, and template variable usage — and that no files or environment modifications are created.

**Acceptance Scenarios**:

1. **Given** a valid scenario file, **When** the user runs with `--dry-run`, **Then** the tool prints a numbered list of steps with match argv, response summary (exit code, stdout preview), and call bounds.
2. **Given** a scenario with template variables, **When** dry-run is invoked, **Then** template variable names are shown as placeholders (e.g., `{{ .cluster }}`) rather than rendered values, so the user can see the template structure.
3. **Given** a scenario with step groups, **When** dry-run is invoked, **Then** group boundaries are clearly indicated with group name and mode.
4. **Given** a scenario with validation errors (e.g., missing `meta.name`), **When** dry-run is invoked, **Then** the validation error is reported to stderr and the tool exits with a non-zero code.
5. **Given** a valid scenario, **When** dry-run is invoked, **Then** no `.cli-replay/` directory, no intercept wrappers, no state files, and no environment variable changes are produced.
6. **Given** a scenario with a security allowlist, **When** dry-run is invoked, **Then** the allowlist is displayed and any steps referencing commands not in the allowlist are flagged.

---

### User Story 3 — Windows Signal Handling Audit (Priority: P1)

A CI/CD engineer uses cli-replay in Windows-based pipelines (GitHub Actions `windows-latest`, Azure DevOps Windows agents). When a CI job times out or is cancelled, the child process spawned by `cli-replay exec` must be terminated and the intercept directory cleaned up. Today, signal forwarding in `exec` mode uses `syscall.SIGTERM`, which is unreliable on Windows — `Process.Signal()` with SIGTERM may fail silently, leaving orphan processes and stale state.

**Why this priority**: Windows CI pipelines are a supported platform per the README. Signal failure means stale intercept directories accumulate, and child processes may be orphaned on timeout — a correctness and reliability issue.

**Independent Test**: On a Windows system, run `cli-replay exec` with a long-running child process, send a cancellation signal (Ctrl+C / `taskkill`), and verify the child process is terminated and the intercept directory is cleaned up.

**Acceptance Scenarios**:

1. **Given** `cli-replay exec` running a child process on Windows, **When** Ctrl+C is pressed, **Then** the child process is terminated and the intercept directory is removed.
2. **Given** `cli-replay exec` running a child process on Windows, **When** the parent process is killed via `taskkill /PID`, **Then** cleanup runs (intercept dir removed) on a best-effort basis.
3. **Given** `cli-replay exec` running a child process on Windows, **When** the child process exits normally, **Then** cleanup runs identically to Unix behavior (verify, report, clean).
4. **Given** a PowerShell session using the `eval` pattern (`cli-replay run | Invoke-Expression`), **When** the user invokes `cli-replay clean`, **Then** the session is cleaned up correctly (acknowledging that PowerShell does not support automatic trap-based cleanup).

---

### User Story 4 — Performance Benchmarks for 100+ Step Scenarios (Priority: P2)

A contributor or maintainer wants confidence that cli-replay performs well with large scenarios (100+ steps, large groups, many state file operations). Today, existing benchmarks only cover 10-step scenarios in the verification layer. Benchmarks should cover the hot path: argument matching, state persistence, replay orchestration, and group matching at scale.

**Why this priority**: Performance at scale is a quality attribute, not a user-facing feature. However, without benchmarks, regressions in the hot path (intercept → match → respond) could go undetected.

**Independent Test**: Run `go test -bench=. ./...` and verify benchmarks exist for 100-step and 500-step scenarios covering matching, state I/O, and verification formatting.

**Acceptance Scenarios**:

1. **Given** a 100-step linear scenario, **When** the full replay engine processes all steps sequentially, **Then** total processing time is under 500 milliseconds.
2. **Given** a scenario with a 50-step unordered group, **When** the group matching engine processes all 50 steps, **Then** per-step matching time averages under 1 millisecond.
3. **Given** a state file representing a 500-step scenario, **When** the state is serialized and deserialized, **Then** the round-trip completes in under 10 milliseconds.
4. **Given** verification output for a 200-step scenario, **When** formatted as JSON and JUnit XML, **Then** each format completes in under 50 milliseconds.

---

### Edge Cases

- What happens when a capture identifier is referenced but the capturing step was skipped (optional step with `calls.min: 0`)? → Resolves to empty string.
- What happens when dry-run encounters a `stdout_file` reference that doesn't exist? → Reports the file path in the preview without attempting to read it.
- What happens when Windows `Process.Kill()` is used instead of SIGTERM and the child has spawned grandchild processes? → Grandchildren may be orphaned; document this as a known limitation for Windows.
- What happens when a capture identifier contains special characters? → Only alphanumeric characters and underscores are allowed in capture identifiers (same rules as Go template variable names).
- What happens when a group step references a capture from a sibling step that hasn't executed yet? → Resolves to empty string (best-effort semantics; no implicit ordering).
- What happens when dry-run is used with flags intended for other commands (e.g., `--ttl`)? → Dry-run only applies to `run` and `exec` commands; `--ttl` info is shown in the summary if present.

## Requirements *(mandatory)*

### Functional Requirements

**Dynamic Capture (US1)**

- **FR-001**: System MUST support a `capture` field on step responses that defines named key-value pairs.
- **FR-002**: System MUST make captured values available to subsequent steps via the template namespace `{{ .capture.<identifier> }}`.
- **FR-003**: System MUST reject scenarios where a capture identifier conflicts with an existing `meta.vars` key at load time.
- **FR-004**: System MUST report an error when a step template references a capture identifier that has not yet been produced by a prior step (forward reference detection at validation time where possible).
- **FR-005**: System MUST persist capture state in the state file as part of the existing atomic write after each step advancement, so that multi-invocation scenarios and crash recovery maintain captured values.
- **FR-006**: System MUST resolve capture references from unexecuted optional steps (`calls.min: 0`) to an empty string.
- **FR-007**: Capture identifiers MUST consist only of alphanumeric characters and underscores, and MUST NOT start with a digit.
- **FR-007a**: Within unordered groups, captures from sibling steps that have not yet executed MUST resolve to an empty string. No implicit ordering MUST be introduced by capture references between group siblings.

**Dry-Run Mode (US2)**

- **FR-008**: System MUST accept a `--dry-run` flag on the `run` command.
- **FR-009**: System MUST accept a `--dry-run` flag on the `exec` command.
- **FR-010**: When `--dry-run` is active, the system MUST NOT create any intercept directories, symlinks, wrappers, or state files.
- **FR-011**: When `--dry-run` is active, the system MUST NOT modify `PATH` or emit shell setup commands.
- **FR-012**: Dry-run MUST print a numbered summary to **stdout** including: step index, match argv pattern, response exit code, stdout preview (first 80 characters or "[file: path]"), call bounds, and group membership.
- **FR-013**: Dry-run MUST show template variable placeholders as-is (not rendered) so users can see the template structure.
- **FR-014**: Dry-run MUST validate the scenario and report any validation errors to stderr with a non-zero exit code.
- **FR-015**: Dry-run MUST display the security allowlist (if configured) and flag any step commands not in the allowlist.

**Windows Signal Handling (US3)**

- **FR-016**: On Windows, the system MUST use a reliable process termination mechanism when forwarding termination signals to child processes.
- **FR-017**: On Windows, the system MUST clean up the intercept directory after child process termination, regardless of how the child was terminated.
- **FR-018**: On Windows, `cli-replay exec` MUST handle Ctrl+C (SIGINT equivalent) by terminating the child process and running cleanup.
- **FR-019**: Signal handling behavior differences between Windows and Unix MUST be documented in the README troubleshooting section.

**Performance Benchmarks (US4)**

- **FR-020**: Benchmark tests MUST exist for argument matching with 100+ step scenarios.
- **FR-021**: Benchmark tests MUST exist for state file serialization/deserialization with 500-step state.
- **FR-022**: Benchmark tests MUST exist for unordered group matching with 50-step groups.
- **FR-023**: Benchmark tests MUST exist for verification formatting (JSON and JUnit) with 200-step results.
- **FR-024**: Existing 10-step benchmarks MUST continue to pass without regression.

### Key Entities

- **Capture**: A named key-value pair produced by a step's response, persisted in the state file on every step write (atomic, crash-safe), and available to subsequent step templates via `{{ .capture.<id> }}`. Scoped to the replay session.
- **DryRunReport**: A structured representation of a scenario's step sequence for preview display, including step index, match pattern, response summary, call bounds, group info, and allowlist validation results.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can define a 3-step capture chain scenario and verify that captured values propagate correctly across all steps without external scripting.
- **SC-002**: Users can preview any valid scenario's full step sequence including groups, bounds, and templates in under 2 seconds using dry-run mode, with zero side effects on the file system.
- **SC-003**: On Windows 10+, `cli-replay exec` correctly terminates child processes and cleans up intercept directories when receiving Ctrl+C, with no orphaned processes or stale state in 95% of test runs.
- **SC-004**: Benchmark suite covers 100-step, 200-step, and 500-step scenarios across matching, state I/O, and formatting, with all benchmarks completing within their specified time thresholds.
- **SC-005**: All existing tests continue to pass with zero regressions after all enhancements are implemented.
- **SC-006**: Dry-run mode detects and reports allowlist violations for 100% of misconfigured scenario steps.

## Assumptions

- **Capture values are strings only** — Captures store string values. No structured data (JSON parsing, array indexing) is supported in the initial implementation. Users who need structured extraction can use external tools.
- **Capture does not modify match behavior** — Captures only apply to response rendering. Match patterns cannot reference captures (because the match happens before the response).
- **Dry-run output is text only** — No `--format json` support for dry-run in the initial implementation. Structured dry-run output can be added later if needed.
- **Windows audit scope is limited to exec mode** — The `eval` pattern (`run | Invoke-Expression`) on PowerShell lacks native trap support; this is documented as a known limitation, not a bug to fix.
- **Benchmark thresholds are guidelines** — Specific millisecond targets are set conservatively. The primary goal is detecting regressions, not guaranteeing absolute performance.
- **Capture identifiers use the same namespace as meta.vars** — They are accessed via `{{ .capture.id }}` to avoid collisions with `meta.vars` keys which use `{{ .id }}`.
