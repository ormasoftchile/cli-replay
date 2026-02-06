# Feature Specification: Core Scenario Replay

**Feature Branch**: `001-core-scenario-replay`  
**Created**: 2026-02-05  
**Status**: Draft  
**Input**: Build cli-replay v0 — a scenario-driven CLI replay tool that acts as a fake command-line executable for testing systems that orchestrate external CLI tools.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run Single-Step CLI Scenario (Priority: P1)

As a developer testing a script that invokes an external CLI, I want to run cli-replay with a simple scenario file so that I can verify my script handles expected CLI output correctly without installing the real CLI.

**Why this priority**: This is the foundational capability—without being able to match a single command and return a response, nothing else works. It validates the entire PATH interception model.

**Independent Test**: Can be fully tested by creating a one-step scenario file, symlinking cli-replay as `kubectl`, invoking `kubectl get pods`, and verifying the expected stdout is returned with exit code 0.

**Acceptance Scenarios**:

1. **Given** a scenario file with one step matching `["kubectl", "get", "pods"]` responding with stdout "pod1\npod2", **When** the user invokes cli-replay (symlinked as kubectl) with `kubectl get pods`, **Then** stdout contains "pod1\npod2" and exit code is 0.

2. **Given** a scenario file with one step responding with exit code 1 and stderr "error: connection refused", **When** the matching command is invoked, **Then** stderr contains "error: connection refused" and exit code is 1.

3. **Given** a scenario file with stdout_file pointing to `fixtures/output.txt`, **When** the matching command is invoked, **Then** stdout contains the exact contents of that file.

---

### User Story 2 - Run Multi-Step Ordered Scenario (Priority: P1)

As a developer testing a multi-step automation workflow, I want cli-replay to match commands in strict order so that I can verify my workflow executes CLI commands in the expected sequence.

**Why this priority**: Real-world CLI orchestration involves multiple commands in sequence. Testing ordering is critical for automation correctness and is equally foundational.

**Independent Test**: Can be fully tested by creating a 3-step scenario, invoking commands in order, and verifying each returns the correct response; then invoking out-of-order to verify failure.

**Acceptance Scenarios**:

1. **Given** a scenario with steps A→B→C, **When** the user invokes commands in order A, B, C, **Then** each command returns its defined response and all steps complete successfully.

2. **Given** a scenario with steps A→B→C, **When** the user invokes command B before A, **Then** cli-replay fails immediately with an error message showing expected vs received command.

3. **Given** a scenario with 3 steps, **When** only 2 commands are invoked and `cli-replay verify` is called, **Then** cli-replay exits with code 1 and reports unused scenario steps remain.

---

### User Story 3 - Fail Fast on Unexpected Commands (Priority: P2)

As a developer, I want cli-replay to fail immediately with a clear error when my code invokes an unexpected command so that I can quickly identify bugs in my automation logic.

**Why this priority**: Strict failure mode is essential for catching regressions but builds on the matching capability from P1 stories.

**Independent Test**: Can be fully tested by invoking a command that doesn't match the current expected step and verifying the error message format.

**Acceptance Scenarios**:

1. **Given** a scenario expecting `["kubectl", "get", "pods"]`, **When** `kubectl get nodes` is invoked, **Then** cli-replay exits with code 1 and stderr shows: received argv, expected argv, scenario name, and step index.

2. **Given** a scenario expecting specific arguments, **When** extra arguments are provided, **Then** the mismatch is detected and reported clearly.

---

### User Story 4 - Template Variables in Responses (Priority: P2)

As a developer, I want to use template variables in my scenario responses so that I can create reusable scenarios that adapt to different environments.

**Why this priority**: Templating enables scenario reuse across environments but is not required for basic functionality.

**Independent Test**: Can be fully tested by defining vars in meta section, using `{{ .varname }}` in stdout, and verifying the rendered output.

**Acceptance Scenarios**:

1. **Given** a scenario with `meta.vars.cluster: "prod-eus2"` and stdout `"Cluster {{ .cluster }} is ready"`, **When** the command is invoked, **Then** stdout contains "Cluster prod-eus2 is ready".

2. **Given** an environment variable `NAMESPACE=default`, **When** a scenario uses `{{ .NAMESPACE }}` in a response, **Then** the environment variable value is rendered.

---

### User Story 5 - Trace Mode for Debugging (Priority: P3)

As a developer debugging why my test is failing, I want to enable trace logging so that I can see exactly what commands cli-replay receives and how it responds.

**Why this priority**: Observability is valuable but not required for core functionality.

**Independent Test**: Can be fully tested by setting `CLI_REPLAY_TRACE=1` and verifying trace output appears on stderr.

**Acceptance Scenarios**:

1. **Given** `CLI_REPLAY_TRACE=1` is set, **When** a command is matched, **Then** stderr shows: step index, received argv, rendered response, and exit code.

2. **Given** trace mode is disabled (default), **When** commands are invoked, **Then** no trace output is produced.

---

### Edge Cases

- What happens when scenario file path is invalid or file doesn't exist?
  - cli-replay MUST exit with error code 1 and clear message indicating file not found
- What happens when scenario YAML is malformed?
  - cli-replay MUST exit with error and report the YAML parse error with line number if available
- What happens when stdout_file reference doesn't exist?
  - cli-replay MUST fail at scenario load time, not at match time
- What happens when template variable is undefined?
  - cli-replay MUST fail with clear error indicating the missing variable name
- What happens when no scenario is provided (no CLI arg, no env var)?
  - cli-replay MUST exit with usage help message
- What happens with empty argv (process invoked with no arguments)?
  - Match against scenario step with empty argv array; if no match, fail as unexpected

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST load scenario files from a path provided via CLI argument or `CLI_REPLAY_SCENARIO` environment variable
- **FR-002**: System MUST parse YAML scenario files with strict validation (reject unknown fields)
- **FR-003**: System MUST match incoming commands against scenario steps using strict ordered matching
- **FR-004**: System MUST compare argv exactly: same length, same ordering, string equality
- **FR-005**: System MUST return the defined exit code, stdout, and stderr for matched commands
- **FR-006**: System MUST support loading stdout/stderr from external files via `stdout_file`/`stderr_file`
- **FR-007**: System MUST advance to the next scenario step after each successful match
- **FR-008**: System MUST fail immediately when a command does not match the expected step
- **FR-009**: System MUST provide a `cli-replay verify <scenario>` command that fails if scenario steps remain unused
- **FR-010**: System MUST render template variables in response stdout/stderr using Go text/template syntax (match patterns are literal, not templated)
- **FR-011**: System MUST source template variables from `meta.vars` and environment variables, with environment variables taking precedence over meta.vars
- **FR-012**: System MUST output trace information to stderr when `CLI_REPLAY_TRACE=1` is set
- **FR-013**: System MUST provide human-readable error messages including: received argv, expected argv, scenario name, step index
- **FR-014**: System MUST be distributable as a single static binary with no external runtime dependencies
- **FR-015**: System MUST support cross-compilation for Windows, macOS (arm64/amd64), and Linux (arm64/amd64)

### Key Entities

- **Scenario**: A complete test definition containing metadata and an ordered list of steps; identified by name; loaded from YAML file
- **Step**: A single command-response pair within a scenario; contains match criteria and response definition; executed in strict order
- **Match**: Criteria for identifying an incoming command; contains argv array for exact comparison
- **Response**: Output definition for a matched command; contains exit code, stdout (literal or file), stderr (literal or file)
- **Meta**: Scenario metadata including name, description, and template variables

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Developers can test a 5-step CLI workflow without installing any real CLI tools
- **SC-002**: Scenario files are human-readable and can be authored in under 5 minutes for simple cases
- **SC-003**: Test execution completes in under 100ms for scenarios with up to 20 steps
- **SC-004**: Error messages enable developers to identify the problem without additional debugging in 90% of cases
- **SC-005**: The binary can be dropped into any CI pipeline without additional setup or dependencies
- **SC-006**: A new team member can understand and use cli-replay from the README alone within 10 minutes
- **SC-007**: All scenario mismatches are detected immediately (no silent failures or incorrect behavior)

## Assumptions

- Users have basic familiarity with YAML syntax
- Users understand PATH environment variable mechanics
- Test frameworks can capture exit codes, stdout, and stderr
- Scenario files are small enough to load entirely into memory (< 1MB typical)
- Single-process, sequential execution is sufficient for v0 (no concurrent CLI invocations)
- State is persisted between CLI invocations via a temp file (`/tmp/cli-replay-<scenario-hash>.state`)

## Clarifications

### Session 2026-02-05

- Q: How is scenario progress persisted across multiple CLI invocations? → A: State file in temp directory (e.g., `/tmp/cli-replay-<hash>.state`)
- Q: When is unused steps check performed? → A: Explicit `cli-replay verify <scenario.yaml>` command called by test
- Q: When same variable exists in meta.vars and environment, which takes precedence? → A: Environment variables override meta.vars
