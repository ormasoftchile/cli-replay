# Feature Specification: P1 CI/CD Enhancements

**Feature Branch**: `006-p1-cicd-enhancements`  
**Created**: 2026-02-07  
**Status**: Draft  
**Input**: User description: "Specify the P1 - High Priority items from ROADMAP: sub-process execution mode (exec), session isolation verification, and signal-trap auto-cleanup"

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Sub-process Execution Mode (`exec`) (Priority: P1)

A CI pipeline engineer runs deployment scripts under cli-replay. Today they must use the three-step `eval "$(cli-replay run ...)"` / execute script / `cli-replay verify && cli-replay clean` pattern. This is error-prone: if the script fails or the pipeline is aborted, the intercept directory and state file are leaked. The engineer wants a single command that sets up interception, runs their script in an isolated child process, and automatically verifies completion and cleans up — regardless of whether the script succeeds or fails.

**Why this priority**: This is the single highest-impact UX improvement for CI users. The current eval pattern is the #1 source of confusion in onboarding, the #1 cause of leaked state in CI, and requires three separate commands where one would suffice. The exec mode also naturally subsumes signal-trap cleanup (User Story 3), making it the foundational CI integration primitive.

**Independent Test**: Can be fully tested by running `cli-replay exec scenario.yaml -- ./test-script.sh`, verifying that the script receives the intercepted commands, the scenario is auto-verified on exit, and no intercept directory or state file remains after the command completes.

**Acceptance Scenarios**:

1. **Given** a scenario file and a shell script that invokes the expected commands in order, **When** the user runs `cli-replay exec scenario.yaml -- ./script.sh`, **Then** the script runs to completion with intercepted commands returning canned responses, cli-replay auto-verifies all steps were consumed, and exits with the script's exit code (0 on success).

2. **Given** a scenario file and a script that invokes a command not matching the expected step, **When** the user runs `cli-replay exec scenario.yaml -- ./script.sh`, **Then** the script receives the mismatch error from the intercept, the script's non-zero exit propagates as exec's exit code, and the intercept directory and state are cleaned up automatically.

3. **Given** a scenario file and a script that is interrupted (SIGINT/SIGTERM), **When** the signal is received during `cli-replay exec`, **Then** the child process is terminated, the intercept directory and state file are removed, and exec exits with a non-zero code indicating the signal.

4. **Given** a scenario file, **When** `cli-replay exec scenario.yaml -- ./script.sh` completes and all steps were consumed, **Then** no intercept directory exists on disk and no state file remains in the temp directory.

5. **Given** a scenario file, **When** `cli-replay exec scenario.yaml -- ./script.sh` completes but not all steps were consumed, **Then** exec exits with a non-zero exit code and prints a verification failure message to stderr (similar to `cli-replay verify` output), and cleanup still occurs.

6. **Given** multiple concurrent CI jobs, **When** each runs `cli-replay exec` with different scenarios, **Then** each exec instance uses an isolated session (unique intercept directory and state file) with no cross-contamination between jobs.

---

### User Story 2 — Session Isolation Verification (Priority: P2)

A platform team runs multiple cli-replay-based tests in parallel across CI matrix builds. Session isolation via `CLI_REPLAY_SESSION` is already implemented, but the team needs confidence that `verify` and `clean` commands correctly find session-specific state files when invoked after `eval "$(cli-replay run ...)"` in the same shell environment.

**Why this priority**: Session isolation is already implemented (the hard part is done). This story is about verification and gap-filling — ensuring the existing session mechanism works end-to-end in parallel CI scenarios. Lower priority because it's a confidence/testing task rather than new functionality, and the exec mode (US1) will be the recommended path for new CI integrations.

**Independent Test**: Can be fully tested by launching two cli-replay sessions in parallel with different `CLI_REPLAY_SESSION` values, invoking commands in each, running `verify` and `clean` in each, and confirming that each session's state is independent — no cross-talk.

**Acceptance Scenarios**:

1. **Given** two concurrent shell sessions each running `eval "$(cli-replay run scenario.yaml)"` with different auto-generated session IDs, **When** commands are invoked in session A, **Then** session B's state is unaffected and vice versa.

2. **Given** a shell session with `CLI_REPLAY_SESSION` set via the eval'd output, **When** `cli-replay verify scenario.yaml` is run in that same shell, **Then** verify finds the session-specific state file and reports the correct step completion status.

3. **Given** a shell session with `CLI_REPLAY_SESSION` set, **When** `cli-replay clean scenario.yaml` is run in that same shell, **Then** clean removes only the session-specific intercept directory and state file, leaving other sessions' files intact.

4. **Given** two parallel sessions using the same scenario file, **When** session A completes all steps and session B completes only 1 of 3 steps, **Then** `verify` in session A reports success while `verify` in session B reports failure — each independently and correctly.

---

### User Story 3 — Signal-Trap Auto-Cleanup (Priority: P3)

A developer runs a deployment script under cli-replay using the `eval` pattern. During execution, they press Ctrl+C to abort the script. Currently, the intercept directory and state file are leaked, requiring manual cleanup. The developer wants interrupted sessions to clean up automatically.

**Why this priority**: Leaked state after interruption is a real problem — it can cause confusing failures on subsequent runs if the stale intercept directory is still on PATH. However, the exec mode (US1) solves this problem more elegantly for CI. This story specifically addresses the eval pattern for interactive/development use where exec mode may not be preferred. Lowest priority because it's a UX polish for a usage pattern that will become secondary once exec mode exists.

**Independent Test**: Can be fully tested by running `eval "$(cli-replay run scenario.yaml)"` in a shell, sending SIGINT to a running script, and verifying that the intercept directory and state file are removed.

**Acceptance Scenarios**:

1. **Given** a shell session with `eval "$(cli-replay run scenario.yaml)"` applied, **When** the eval output includes a shell trap for cleanup, **Then** the trap targets `EXIT`, `INT`, and `TERM` signals and invokes `cli-replay clean` for the specific scenario file.

2. **Given** a shell session with the cleanup trap set, **When** the user's script is interrupted with Ctrl+C (SIGINT), **Then** the trap fires, `cli-replay clean` removes the intercept directory and state file, and no stale files remain.

3. **Given** a shell session with the cleanup trap set, **When** the script completes normally and the user runs `cli-replay verify` followed by `cli-replay clean`, **Then** the explicit clean succeeds (idempotent — trap may have already cleaned, or clean handles already-cleaned state gracefully).

4. **Given** a shell session with the cleanup trap set, **When** the user opens a new sub-shell or sources the eval output in a function scope, **Then** the trap is scoped to the appropriate shell level and does not interfere with parent shell traps.

5. **Given** a shell session using zsh, bash, or sh, **When** `eval "$(cli-replay run scenario.yaml)"` is applied, **Then** the trap syntax is compatible with all three shells (POSIX-compliant trap command).

---

### Edge Cases

- **exec mode — script not found or not executable**: `cli-replay exec scenario.yaml -- ./nonexistent.sh` should report a clear error before setting up any interception, similar to how the shell reports "command not found".
- **exec mode — empty argv after `--`**: `cli-replay exec scenario.yaml --` with no command should report a usage error.
- **exec mode — script exits with signal (e.g., SIGSEGV)**: The exec command should propagate the signal-induced exit code (128 + signal number) after cleanup.
- **exec mode — scenario loading fails**: If the scenario YAML is invalid, exec should exit with an error before spawning any child process.
- **Signal cleanup — nested traps**: If the user's script also sets EXIT traps, the cli-replay trap should not overwrite them. The eval output should append to existing traps or use a pattern that chains traps.
- **Signal cleanup — rapid repeated signals**: Multiple SIGINT signals in quick succession should not cause double-cleanup or errors from cleaning an already-cleaned state.
- **Session isolation — expired/stale sessions**: If a session's state file exists but the intercept directory is already removed (manual cleanup), verify and clean should handle this gracefully without crashing.
- **exec mode — very long-running scripts**: exec should not impose any timeout on the child process — it runs until completion or signal.

## Requirements *(mandatory)*

### Functional Requirements

**Sub-process Execution Mode (exec):**

- **FR-001**: The system MUST provide a `cli-replay exec <scenario> -- <command> [args...]` command that runs the target command as a child process with interception active.
- **FR-002**: The exec command MUST set up the intercept directory and modify the child process's PATH (not the parent's) before spawning the command.
- **FR-003**: The exec command MUST automatically verify scenario completion after the child process exits, reporting any unconsumed or under-called steps to stderr.
- **FR-004**: The exec command MUST automatically clean up the intercept directory and state file after the child process exits, regardless of exit code or signal.
- **FR-005**: The exec command MUST propagate the child process's exit code as its own exit code when all scenario steps are satisfied.
- **FR-006**: When scenario verification fails (steps not consumed), exec MUST exit with a non-zero code even if the child process exited with 0.
- **FR-007**: The exec command MUST forward signals (SIGINT, SIGTERM) to the child process and perform cleanup after the child terminates.
- **FR-008**: The exec command MUST generate a unique session ID for each invocation to support parallel execution.
- **FR-009**: The exec command MUST validate the scenario file before spawning the child process, exiting with an error if the scenario is invalid.
- **FR-010**: The exec command MUST support the same applicable flags as the primary `run` command (`--allowed-commands`). The `--shell` flag is not applicable to exec mode (exec manages the environment directly, not via shell eval).

**Session Isolation Verification:**

- **FR-011**: The `verify` command MUST respect the `CLI_REPLAY_SESSION` environment variable when locating state files.
- **FR-012**: The `clean` command MUST respect the `CLI_REPLAY_SESSION` environment variable when locating state files and intercept directories.
- **FR-013**: When `CLI_REPLAY_SESSION` is set, `verify` and `clean` MUST operate only on the session-specific state file, not the default (sessionless) state file.
- **FR-014**: When `CLI_REPLAY_SESSION` is not set, `verify` and `clean` MUST behave as they do today (backward compatible).

**Signal-Trap Auto-Cleanup:**

- **FR-015**: The `run` command's shell output MUST include a trap statement that cleans up the intercept directory and state file on EXIT, INT, and TERM signals.
- **FR-016**: The trap syntax MUST be POSIX-compliant (compatible with bash, zsh, and sh).
- **FR-017**: The trap MUST reference the specific scenario file path to clean up only the relevant session's resources.
- **FR-018**: The `clean` command MUST be idempotent — calling it when resources are already cleaned MUST NOT produce an error.

### Key Entities

- **Exec Session**: A managed lifecycle encompassing intercept setup, child process execution, verification, and cleanup — all within a single command invocation.
- **Child Process**: The user's script or command spawned by `exec` with a modified environment (PATH prepended with intercept directory, session ID set).
- **Cleanup Trap**: A shell trap statement emitted by the `run` command that ensures cleanup on signal or exit when using the eval pattern.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can run a complete replay scenario (setup, execute, verify, clean) with a single command (`cli-replay exec`) instead of three separate commands.
- **SC-002**: After `cli-replay exec` completes (success or failure), zero intercept directories and zero state files from that session remain on disk.
- **SC-003**: Two concurrent `cli-replay exec` invocations with different scenarios complete independently with no cross-contamination of state or interception.
- **SC-004**: When a script is interrupted during `cli-replay exec`, cleanup occurs within 5 seconds of the signal, leaving no leaked resources.
- **SC-005**: When using the eval pattern, an interrupted script (Ctrl+C) triggers automatic cleanup with no manual intervention required.
- **SC-006**: Existing `run` / `verify` / `clean` workflow continues to work without modification (backward compatible).
- **SC-007**: All new functionality is covered by tests verifying the key behaviors (exec lifecycle, signal handling, session isolation, trap generation).

## Assumptions

- **Shell compatibility**: The trap auto-cleanup targets POSIX-compatible shells (bash, zsh, sh). Fish shell and PowerShell are out of scope for the trap approach; exec mode works universally.
- **Signal forwarding**: exec mode forwards SIGINT and SIGTERM to the child process. Other signals (SIGHUP, SIGUSR1, etc.) are not forwarded unless explicitly needed.
- **Exit code convention**: When exec's auto-verify fails, the exit code is 1 (general error), distinct from the child's own exit codes. If the child exits non-zero, that code takes precedence.
- **No timeout**: exec does not impose any timeout on the child process. Users can combine with external timeout tools if needed.
- **Session isolation already works**: The existing `CLI_REPLAY_SESSION` + `StateFilePathWithSession()` mechanism is functionally correct; US2 is about adding test coverage and fixing any discovered gaps.
- **Clean is idempotent**: The `clean` command already handles missing directories gracefully or will be made to do so as part of FR-018.

## Scope Boundaries

**In scope:**
- New `exec` command with child process management, auto-verify, auto-cleanup, and signal forwarding
- Session isolation end-to-end testing for `verify` and `clean` commands
- Shell trap generation in `run` command output for auto-cleanup on signals
- Making `clean` command idempotent

**Out of scope:**
- Windows-specific signal handling (SIGINT/SIGTERM are Unix concepts; Windows exec support deferred)
- Fish shell or PowerShell trap equivalents
- Timeout enforcement on child processes
- Remote/distributed session management
- Step groups or unordered execution (P2, separate feature)
- JSON/JUnit output format for verify (P2, separate feature)
