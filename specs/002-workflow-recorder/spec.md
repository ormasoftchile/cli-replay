# Feature Specification: Workflow Recorder

**Feature Branch**: `002-workflow-recorder`  
**Created**: February 6, 2026  
**Status**: Draft  
**Input**: User description: "Add a record subcommand to cli-replay that captures real command executions and automatically generates replay scenario YAML files. This eliminates the manual work of crafting scenarios by recording actual workflows."

## Clarifications

### Session 2026-02-06

- Q: When recording a command that spawns background processes (e.g., `kubectl port-forward` or servers), how should the recorder handle completion? → A: Record immediately upon spawn (doesn't wait for background process)
- Q: Should stderr be captured in a separate field from stdout in the YAML response block, or combined? → A: Separate `stdout` and `stderr` fields in respond block (enables precise testing)
- Q: For the `--command` flag that filters which commands to record, should it support multiple commands in one invocation? → A: Allow multiple: `--command kubectl --command docker` (standard CLI pattern)
- Q: When recording commands with shell special characters (pipes `|`, redirects `>`, `<`, wildcards `*`), how should these be captured? → A: Literal argv only
- Q: When the same command is executed multiple times with different outputs during a recording session, should each execution be recorded as a separate step? → A: Record each execution as separate step (preserves temporal sequence)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Basic Command Recording (Priority: P1)

As a developer, I want to record a simple command execution so that I can quickly capture its output and generate a replay scenario without manually crafting YAML.

**Why this priority**: This is the core MVP functionality - the ability to record and replay a single command provides immediate value and validates the entire recording approach.

**Independent Test**: Can be fully tested by executing `cli-replay record --output demo.yaml -- kubectl get pods`, which should capture the command execution and generate a valid YAML scenario that can be replayed.

**Acceptance Scenarios**:

1. **Given** a user has cli-replay installed, **When** they run `cli-replay record --output output.yaml -- kubectl get pods`, **Then** a YAML file is created containing the command arguments, exit code, and stdout/stderr
2. **Given** a recorded YAML scenario exists, **When** the user replays it using `cli-replay run output.yaml`, **Then** the command executes with the same outputs as recorded
3. **Given** a command that fails with non-zero exit code, **When** the user records it, **Then** the YAML captures the exit code and error output correctly
4. **Given** a command with multiline output, **When** recorded, **Then** the YAML preserves line breaks and formatting

---

### User Story 2 - Multi-Step Workflow Recording (Priority: P2)

As a trainer, I want to record a sequence of commands in a shell script so that I can create reproducible training scenarios that demonstrate multi-step workflows.

**Why this priority**: Extends the basic recording to handle realistic workflows with multiple sequential commands, enabling more valuable use cases like demos and training.

**Independent Test**: Can be fully tested by recording a bash script with 3+ commands and verifying that the generated YAML contains all steps in correct order, and that replay executes them sequentially.

**Acceptance Scenarios**:

1. **Given** a bash script with multiple commands, **When** user runs `cli-replay record --output workflow.yaml -- bash script.sh`, **Then** all commands are captured in sequence
2. **Given** recorded multi-step workflow, **When** replayed, **Then** commands execute in the same order as recorded
3. **Given** a workflow where step 2 depends on step 1's output, **When** recorded and replayed, **Then** the dependency is preserved and replay succeeds

---

### User Story 3 - Selective Command Recording (Priority: P3)

As a QA engineer, I want to record only specific commands (e.g., kubectl, docker) while ignoring others (e.g., ls, cd) so that my scenarios focus on the relevant CLI interactions without noise.

**Why this priority**: Improves usability for complex workflows by filtering out irrelevant commands, but the feature is still valuable without this filtering capability.

**Independent Test**: Can be fully tested by running `cli-replay record --command kubectl --output filtered.yaml -- bash mixed-script.sh` where the script contains both kubectl and other commands, verifying only kubectl commands appear in the YAML.

**Acceptance Scenarios**:

1. **Given** a workflow with mixed commands, **When** user specifies `--command kubectl`, **Then** only kubectl commands are recorded
2. **Given** a workflow with multiple target commands, **When** user specifies `--command kubectl --command docker`, **Then** only kubectl and docker commands are recorded
3. **Given** no command filter specified, **When** recording, **Then** all commands are captured

---

### User Story 4 - Metadata Customization (Priority: P4)

As a developer, I want to add custom name and description metadata during recording so that my generated scenarios are self-documenting and organized.

**Why this priority**: Improves scenario maintainability but is not essential for core functionality.

**Independent Test**: Can be fully tested by recording with `--name "pod-demo" --description "Pod restart workflow"` and verifying the YAML contains these metadata fields.

**Acceptance Scenarios**:

1. **Given** user provides `--name` flag, **When** recording, **Then** YAML contains the custom name in meta section
2. **Given** user provides `--description` flag, **When** recording, **Then** YAML contains the description
3. **Given** no metadata flags provided, **When** recording, **Then** YAML contains auto-generated name with timestamp

---

### Edge Cases

- Commands with shell special characters (pipes, redirects, wildcards) are recorded as literal argv after shell expansion (the actual command arguments, not the shell syntax)
- How does the system handle commands with arguments containing spaces?
- Commands that spawn background processes are recorded immediately upon spawn without waiting for the background process to complete
- How does the system handle commands that read from stdin interactively?
- The same command executed multiple times is recorded as separate steps to preserve temporal sequence and different outputs
- How does recording handle commands that modify environment variables?
- What happens when recording a command that changes working directory?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a `record` subcommand that accepts command execution arguments
- **FR-002**: System MUST capture command argv (program name and all arguments) exactly as executed
- **FR-003**: System MUST capture command exit code (0 for success, non-zero for errors)
- **FR-004**: System MUST capture stdout output from executed commands
- **FR-005**: System MUST capture stderr output from executed commands in a separate field from stdout
- **FR-006**: System MUST generate YAML files conforming to existing scenario schema (meta section with name/description, steps array with match/respond blocks)
- **FR-007**: System MUST support recording single commands via `-- command args` syntax
- **FR-008**: System MUST support recording shell scripts via `-- bash script.sh` syntax
- **FR-009**: System MUST preserve command execution order in multi-step recordings
- **FR-009b**: System MUST record each command execution as a separate step, including duplicate commands with different outputs
- **FR-010**: System MUST write output to file specified by `--output` flag
- **FR-011**: System MUST capture literal argv (after shell expansion) for commands with arguments containing spaces, quotes, and special characters
- **FR-012**: System MUST preserve multiline output formatting (line breaks, indentation)
- **FR-013**: System MUST allow specifying which commands to record via `--command` flag (optional filter), supporting multiple command filters via repeated `--command` flags
- **FR-014**: System MUST allow custom metadata via `--name` and `--description` flags
- **FR-015**: System MUST generate valid YAML that can be loaded and replayed by existing `cli-replay run` command
- **FR-016**: Recording mechanism MUST not require root/sudo privileges
- **FR-017**: Recording MUST work on both macOS and Linux operating systems
- **FR-018**: Recording MUST execute actual commands (not simulate them) to capture real output

### Key Entities

- **RecordedCommand**: Represents a single command execution with argv, exit code, stdout, stderr, timestamp
- **RecordingSession**: Collection of RecordedCommands in execution order, with session metadata (start time, end time, filters)
- **ScenarioConverter**: Transforms RecordingSession into YAML scenario format matching existing schema

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can record a single command execution and generate valid YAML in under 30 seconds
- **SC-002**: Generated YAML scenarios successfully replay with 100% output fidelity for recorded commands
- **SC-003**: Recording captures multi-step workflows with up to 50 commands without performance degradation
- **SC-004**: Recording overhead adds less than 5% execution time compared to running commands directly
- **SC-005**: Generated YAML is human-readable and can be manually edited after recording
- **SC-006**: System correctly handles commands with complex arguments (quotes, spaces, special characters) in 95% of common CLI tools (kubectl, docker, git, aws)

## Scope

### In Scope

- Recording command executions via PATH shim interception
- Capturing argv, exit codes, stdout, stderr
- Generating YAML in existing scenario format
- Single command and shell script recording
- Command filtering (include/exclude specific commands)
- Custom metadata (name, description)
- macOS and Linux support
- Documentation and usage examples

### Out of Scope

- Interactive command recording (prompts, stdin input) - commands must be non-interactive
- Environment variable capture and replay
- Working directory state tracking
- Recording of command aliases or shell functions
- Windows support (initial release)
- Terminal control sequences or ANSI codes
- Recording timing information (delays between commands)
- Modification of recorded data during capture (edit-on-the-fly)
- Merging multiple recording sessions
- Smart deduplication of similar commands
- Template variable detection

## Assumptions

- Users will primarily record non-interactive commands suitable for demos and testing
- Most target commands (kubectl, docker, etc.) are deterministic enough for replay scenarios
- Users can manually edit generated YAML for any post-processing needs
- Recording will use temporary directory approach or PATH manipulation (specific implementation to be determined during planning)
- Existing scenario YAML schema supports all necessary fields without modification
- Users have write permissions to the output directory
- Commands to be recorded are available in system PATH
- Shell script recording uses bash as the shell interpreter

## Dependencies

- Existing scenario YAML schema (internal/scenario/model.go)
- Existing scenario loader (internal/scenario/loader.go)
- Existing replay runner (internal/runner/replay.go)
- YAML parsing/generation library (likely gopkg.in/yaml.v3 or similar)
- Access to system PATH and ability to create temporary executables
