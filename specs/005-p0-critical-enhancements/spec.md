# Feature Specification: P0 Critical Enhancements

**Feature Branch**: `005-p0-critical-enhancements`
**Created**: 2026-02-07
**Status**: Draft
**Input**: User description: "Specify the P0 Critical items from ROADMAP: stdin support, call count bounds, enhanced mismatch diagnostics, and security allowlist for CI"

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Improved Mismatch Diagnostics (Priority: P1)

A developer runs a deployment script under cli-replay. One of the commands differs from the expected step (wrong flag, wrong resource name). The developer needs to immediately understand *what* didn't match and *where* so they can fix the script or the scenario.

**Why this priority**: Debugging failures is the most common friction point. Every user of cli-replay hits a mismatch eventually. The current error message (`argv mismatch at step N`) forces manual comparison. This improvement has the highest value-to-effort ratio and benefits all existing users immediately.

**Independent Test**: Can be fully tested by triggering a deliberate mismatch (wrong argument in a step) and verifying that the error output includes a per-element diff with the first divergence point highlighted.

**Acceptance Scenarios**:

1. **Given** a scenario with step expecting `["kubectl", "get", "pods"]`, **When** the intercepted command is `kubectl get deployments`, **Then** the error output shows both expected and received argv, identifies position 2 as the first difference, and displays `expected "pods", got "deployments"`.

2. **Given** a scenario with step expecting `["az", "group", "list", "--subscription", "{{ .any }}"]`, **When** the intercepted command is `az group list` (missing `--subscription` argument), **Then** the error output shows a length mismatch (`expected 5 arguments, received 3`) and lists the missing arguments.

3. **Given** a scenario with step expecting `["kubectl", "get", "pods", "-n", "{{ .regex \"^prod-.*\" }}"]`, **When** the intercepted command is `kubectl get pods -n staging-app`, **Then** the error output shows the pattern that failed to match and the value that was received (e.g., `expected pattern "^prod-.*", got "staging-app"`).

4. **Given** a three-step scenario where step 2 fails to match, **When** the mismatch error is displayed, **Then** the output includes the scenario name, the step number (1-based), and the full expected vs. received argv arrays so the developer does not need to open the YAML file to correlate.

---

### User Story 2 — Call Count Bounds for Retry/Polling Steps (Priority: P2)

A platform engineer writes a deployment script that polls `kubectl get pods` in a loop, waiting for pods to become Ready. The polling count varies per run (2–8 calls typically). The engineer needs cli-replay to tolerate this repetition without breaking strict ordering for the non-polling steps.

**Why this priority**: Retry and polling loops are a fundamental pattern in CLI orchestration (health checks, convergence waits, status polling). Without call count bounds, users cannot test any workflow that includes variable-frequency commands. This unlocks a large class of real-world scenarios.

**Independent Test**: Can be fully tested by creating a scenario with a polling step (`calls.min: 1, calls.max: 5`), invoking that step 3 times, then advancing to the next step and verifying completion.

**Acceptance Scenarios**:

1. **Given** a step with `calls: { min: 1, max: 5 }`, **When** the matching command is invoked 3 times followed by the next step's command, **Then** the 3 invocations are accepted, the step is marked consumed, and the scenario advances to the next step.

2. **Given** a step with `calls: { min: 2, max: 5 }`, **When** the matching command is invoked only once and the next step's command arrives, **Then** the replay rejects the next command with an error indicating the current step requires at least 2 calls but only received 1.

3. **Given** a step with `calls: { max: 3 }`, **When** the matching command is invoked a 4th time, **Then** the replay does not re-match the exhausted step and instead attempts to match against the next step in sequence.

4. **Given** a step with `calls: { min: 1, max: 5 }` that was invoked 3 times, **When** `cli-replay verify` is run, **Then** the step is reported as consumed with an invocation count of 3 (meeting the minimum of 1).

5. **Given** a step with `calls: { min: 3 }` that was invoked only 2 times, **When** `cli-replay verify` is run, **Then** the scenario is reported as incomplete, identifying the step that did not meet its minimum call count.

6. **Given** a step *without* a `calls` field, **When** it is invoked, **Then** behavior is identical to today: exactly one invocation is expected (equivalent to `calls: { min: 1, max: 1 }`).

---

### User Story 3 — stdin Matching for Pipe-Based Workflows (Priority: P3)

A DevOps engineer has a deployment script that pipes a YAML manifest into `kubectl apply -f -`. They want to validate that the correct manifest content is piped into the command during replay, not just that the right command was called.

**Why this priority**: stdin piping is common in Kubernetes, Terraform, and Docker workflows. However, the implementation touches platform-specific shim layers (bash shims on Unix, .cmd wrappers on Windows), making it the most complex P0 item. The value is high but the effort justifies a lower relative priority within P0.

**Independent Test**: Can be fully tested by creating a scenario with a `match.stdin` field, piping content into the intercepted command, and verifying that the stdin content matches and the correct response is returned.

**Acceptance Scenarios**:

1. **Given** a step with `match.stdin: "apiVersion: v1\nkind: Pod"`, **When** the intercepted command receives that exact content on stdin, **Then** the step matches and the predetermined response is returned.

2. **Given** a step with `match.stdin` set, **When** the intercepted command receives different stdin content, **Then** the step fails with a mismatch error that shows expected vs. received stdin (truncated to a reasonable length for readability).

3. **Given** a step with `match.argv` set but *no* `match.stdin` field, **When** the intercepted command receives any stdin, **Then** stdin is ignored and matching proceeds on argv only (backward compatible).

4. **Given** a step with `match.stdin` set, **When** `cli-replay record` captures a command that receives stdin, **Then** the recorded YAML includes the stdin content in the `match.stdin` field.

5. **Given** a step with `match.stdin` that contains leading/trailing whitespace or a trailing newline, **When** the intercepted command receives the same content, **Then** matching normalizes trailing newlines to avoid false mismatches from shell-appended newlines.

---

### User Story 4 — Security Allowlist for CI Pipelines (Priority: P4)

A security-conscious CI administrator wants to ensure that cli-replay scenarios can only intercept a predefined set of commands (e.g., `kubectl`, `az`, `aws`) and cannot shadow critical system binaries like `bash`, `sudo`, or `curl`.

**Why this priority**: PATH manipulation is a trust boundary. While cli-replay is designed for testing, the ability to intercept arbitrary binaries is a risk in shared CI environments. The implementation is simple (validation at `run` time) and addresses enterprise security reviews. Lower priority because it doesn't block core functionality — it's a governance control.

**Independent Test**: Can be fully tested by creating a scenario with `meta.security.allowed_commands`, running a scenario that references a command outside the allowlist, and verifying that `cli-replay run` rejects it before creating any intercepts.

**Acceptance Scenarios**:

1. **Given** a scenario with `meta.security.allowed_commands: ["kubectl", "az"]`, **When** all steps reference only `kubectl` or `az` in `argv[0]`, **Then** `cli-replay run` proceeds normally and creates intercepts.

2. **Given** a scenario with `meta.security.allowed_commands: ["kubectl"]`, **When** a step references `docker` in `argv[0]`, **Then** `cli-replay run` exits with an error before creating any intercepts, listing the disallowed command.

3. **Given** a scenario *without* a `meta.security` section, **When** steps reference any commands, **Then** `cli-replay run` proceeds without restriction (backward compatible).

4. **Given** a CLI flag `--allowed-commands kubectl,az,aws`, **When** the scenario references those commands, **Then** the flag-based allowlist is enforced even if the scenario YAML has no `security` section.

5. **Given** both a YAML `allowed_commands` and a CLI `--allowed-commands` flag, **When** they overlap partially, **Then** the intersection is enforced (the stricter set wins).

---

### Edge Cases

- **Call count bounds — max: 0**: Treated as invalid and rejected at load time. A step that can never be called is meaningless.
- **Call count bounds — min > max**: Rejected at load time with a clear validation error.
- **stdin matching — binary content**: Initial implementation supports text-only. Binary stdin matching is out of scope.
- **stdin matching — very large stdin**: If stdin exceeds 1 MB, the shim truncates or rejects with an error rather than consuming unbounded memory.
- **Mismatch diagnostics — very long argv**: If argv contains many arguments, the diff shows context around the first difference, not the entire array.
- **Security allowlist — case sensitivity**: Command names matched case-sensitively on Unix, case-insensitively on Windows.
- **Security allowlist — path-based commands**: If `argv[0]` contains a path (e.g., `/usr/bin/kubectl`), only the base name is compared against the allowlist.

## Requirements *(mandatory)*

### Functional Requirements

**Mismatch Diagnostics:**

- **FR-001**: When a command does not match the expected step, the error output MUST include the scenario name, step number (1-based), and full expected and received argv arrays.
- **FR-002**: The error output MUST identify the index of the first differing argument and display both the expected and received values at that position.
- **FR-003**: When the expected argument uses a template pattern (`{{ .any }}`, `{{ .regex "..." }}`), the error output MUST show the pattern that failed and the value that was tested against it.
- **FR-004**: When the argv lengths differ, the error output MUST report the length mismatch and list the extra or missing arguments.

**Call Count Bounds:**

- **FR-005**: Steps MAY include a `calls` field with `min` and/or `max` sub-fields to specify the allowed invocation range.
- **FR-006**: When `calls` is omitted, the step MUST behave as `calls: { min: 1, max: 1 }` (exactly one invocation, preserving current behavior).
- **FR-007**: A step with remaining call budget (invocations < max) MUST remain the current step and accept additional matching commands.
- **FR-008**: When a step's max invocations are exhausted, the replay engine MUST advance to the next step.
- **FR-009**: The `verify` command MUST check that every step's minimum call count was reached and report shortfalls.
- **FR-010**: Scenario validation MUST reject `calls.max: 0` and `calls.min > calls.max` at load time with clear error messages.

**stdin Matching:**

- **FR-011**: The `Match` section MAY include a `stdin` field containing the expected stdin content.
- **FR-012**: When `match.stdin` is set, the intercepted command's actual stdin MUST be compared against the expected value. If they differ, the step MUST fail with a mismatch error.
- **FR-013**: When `match.stdin` is not set, any stdin received by the intercepted command MUST be ignored (backward compatible).
- **FR-014**: The shim layer MUST capture stdin and make it available to the matching engine before the intercepted binary would consume it.
- **FR-015**: The `record` command MUST capture stdin content and include it in the generated YAML when the recorded command receives stdin input.
- **FR-016**: Trailing newline normalization MUST be applied during stdin comparison to prevent false mismatches.

**Security Allowlist:**

- **FR-017**: The `meta` section MAY include a `security.allowed_commands` list restricting which commands can be intercepted.
- **FR-018**: When `allowed_commands` is set, the `run` command MUST validate all `argv[0]` values against the list and reject the scenario before creating intercepts if any command is disallowed.
- **FR-019**: When `allowed_commands` is not set, all commands MUST be allowed (backward compatible).
- **FR-020**: A CLI flag `--allowed-commands` MUST be supported on the `run` command to enforce an allowlist independently of the YAML.
- **FR-021**: When both YAML and CLI allowlists are present, the intersection (stricter set) MUST be enforced.

### Key Entities

- **Match**: Extended with optional `Stdin` field for piped input matching.
- **Step**: Extended with optional `Calls` struct (`Min`, `Max` integers) for invocation bounds.
- **State**: Extended from `ConsumedSteps []bool` to `StepCounts []int` for tracking per-step invocation counts.
- **MismatchError**: Extended with per-element diff information and template pattern context.
- **Meta**: Extended with optional `Security` struct containing `AllowedCommands []string`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: When a command mismatch occurs, the user can identify the exact differing argument and its position from the error output alone, without opening the scenario file.
- **SC-002**: Scenarios with retry/polling steps (variable call counts) can be fully validated end-to-end, with `verify` confirming minimum thresholds were met.
- **SC-003**: Pipe-based CLI workflows (commands receiving stdin) can be recorded and replayed with stdin content validated during matching.
- **SC-004**: CI administrators can restrict interceptable commands via YAML configuration or CLI flags, with violations rejected before any PATH manipulation occurs.
- **SC-005**: All new features are backward compatible — existing scenario files continue to work without modification.
- **SC-006**: All new matching behaviors (stdin, call counts) are covered by unit and integration tests with ≥ 90% branch coverage for changed code.

## Assumptions

- **Text-only stdin**: Initial stdin support handles text content only. Binary stdin matching is deferred to a future enhancement.
- **stdin size limit**: stdin capture is capped at 1 MB to prevent unbounded memory usage in the shim layer.
- **Call count defaults**: Omitting `calls` means exactly 1 invocation (`min: 1, max: 1`), preserving strict current behavior.
- **Allowlist granularity**: Allowlist matches on the base command name only (e.g., `kubectl`), not full paths or arguments.
- **Platform parity**: stdin capture requires platform-specific shim changes; Windows PowerShell `.cmd` shims will follow the same semantics as Unix bash shims.
- **Mismatch output destination**: Enhanced diagnostics are written to stderr, consistent with all existing cli-replay user-facing output.

## Scope Boundaries

**In scope:**
- Mismatch diagnostics: per-element diff, template pattern display, length mismatch reporting
- Call count bounds: `calls.min`/`calls.max` on steps, state tracking, verify validation
- stdin matching: `match.stdin` field, shim capture, record support, normalization
- Security allowlist: `meta.security.allowed_commands`, `--allowed-commands` CLI flag, intersection logic

**Out of scope:**
- Step groups / unordered execution (P2, separate feature)
- Sub-process `exec` mode (P1, separate feature)
- Signal-trap auto-cleanup (P1, separate feature)
- JSON/JUnit output format for `verify` (P2, separate feature)
- Binary stdin matching
- Regex or pattern-based stdin matching (future enhancement on top of exact match)
