# Feature Specification: P2 Quality of Life Enhancements

**Feature Branch**: `007-p2-quality-of-life`  
**Created**: 2026-02-07  
**Status**: Draft  
**Input**: User description: "P2 Medium Priority Quality of Life: Step Groups (unordered blocks), Machine-Readable Output (JSON/JUnit for verify), and JSON Schema for scenario files"

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Machine-Readable Verify Output (Priority: P1)

A CI pipeline engineer runs `cli-replay verify` as part of an automated test suite. The default human-readable output (checkmarks, step counts) cannot be parsed by CI dashboards, test aggregators, or GitHub Actions job summaries. The engineer needs structured output — JSON for programmatic consumption and JUnit XML for integration with standard test reporting tools (GitHub Actions, Azure DevOps, Jenkins).

**Why this priority**: Lowest complexity of the three features, immediate value for every CI user, and unblocks downstream integration (test dashboards, PR annotations, failure aggregation) without any changes to the scenario model or replay engine.

**Independent Test**: Run `cli-replay verify scenario.yaml --format json` and pipe the output to `jq`; run `cli-replay verify scenario.yaml --format junit` and validate the XML against the JUnit XSD.

**Acceptance Scenarios**:

1. **Given** a completed scenario with all steps consumed, **When** the user runs `cli-replay verify scenario.yaml --format json`, **Then** stdout contains a valid JSON object with scenario name, overall pass/fail status, total steps, consumed count, and per-step detail (step index, argv label, call count, min, max, status).
2. **Given** an incomplete scenario with steps remaining, **When** the user runs `cli-replay verify scenario.yaml --format json`, **Then** the JSON object shows `"passed": false`, the per-step array identifies which steps are incomplete, and the process exits with code 1.
3. **Given** a completed scenario, **When** the user runs `cli-replay verify scenario.yaml --format junit`, **Then** stdout contains valid JUnit XML with one `<testsuite>` element (scenario name) containing one `<testcase>` per step, and the process exits with code 0.
4. **Given** an incomplete scenario, **When** the user runs `cli-replay verify scenario.yaml --format junit`, **Then** incomplete steps appear as `<failure>` elements within their `<testcase>`, and the process exits with code 1.
5. **Given** no `--format` flag, **When** the user runs `cli-replay verify scenario.yaml`, **Then** the existing human-readable stderr output is produced unchanged (backward compatible).
6. **Given** an invalid format value, **When** the user runs `cli-replay verify scenario.yaml --format yaml`, **Then** an error is returned listing the valid formats (text, json, junit).

---

### User Story 2 — JSON Schema for Scenario Files (Priority: P2)

A scenario author creates YAML files in VS Code and gets no autocompletion, no inline validation, and no documentation for available fields. Typos in field names (e.g., `stout` instead of `stdout`) silently produce empty responses. A published JSON Schema enables IDE autocompletion, inline validation, and hover documentation for every field in the scenario format.

**Why this priority**: Low complexity (one-time authoring), no runtime code changes, significant improvement to the scenario authoring experience. Prevents an entire class of silent misconfiguration bugs.

**Independent Test**: Add a `# yaml-language-server: $schema=...` reference to an example scenario YAML, open it in VS Code with the YAML extension installed, and verify that field autocompletion appears and typos are flagged.

**Acceptance Scenarios**:

1. **Given** the JSON Schema file exists in the repository, **When** a user references it from a scenario YAML (via `# yaml-language-server: $schema=...` comment), **Then** VS Code highlights unknown fields, shows autocompletion for `meta`, `steps`, `match`, `respond`, `calls`, `when`, `security`, and all their sub-fields.
2. **Given** a scenario with `stdout` and `stdout_file` both set on the same step, **When** validated against the schema, **Then** the schema reports a validation error (mutual exclusivity).
3. **Given** a scenario with `exit: 300`, **When** validated against the schema, **Then** the schema reports the value is out of range (0–255).
4. **Given** a scenario with a `calls` block where `min: 5` and `max: 3`, **When** loaded by cli-replay, **Then** runtime validation rejects it with a descriptive error (`min > max`). Note: cross-field numeric comparison is not enforceable in JSON Schema; this is a runtime-only check.
5. **Given** a new user who has never written a scenario, **When** they create a new `.yaml` file with the schema reference and type `steps:` then start a new array item, **Then** autocompletion suggests `match` and `respond` as required fields.

---

### User Story 3 — Step Groups with Unordered Matching (Priority: P3)

A DevOps engineer writes a scenario for a deployment script that runs pre-flight checks in an unpredictable order (e.g., `kubectl get nodes`, `az account show`, `docker info`). Today, the engineer must guess which order the script calls them and write steps in that exact sequence. If the script changes its internal ordering, the scenario breaks despite the same commands being called. Step groups allow declaring a set of steps that can be consumed in any order.

**Why this priority**: Highest complexity of the three features — touches the scenario model (Step becomes a union type), the YAML loader, the replay engine's step-matching logic, state tracking, and verification. The strict-ordering philosophy covers most use cases; unordered groups address a real but less common need.

**Independent Test**: Create a scenario with an unordered group of 3 steps followed by an ordered step. Run a script that calls the 3 commands in a different order than listed. Verify all steps are consumed and verification passes.

**Acceptance Scenarios**:

1. **Given** a scenario with an unordered group containing steps A, B, C, **When** a script calls them in order C, A, B, **Then** all three steps are matched and consumed, and verification passes.
2. **Given** a scenario with an unordered group followed by an ordered step D, **When** a script calls A, B, C, D in any order for the group, **Then** D is only matched after all group steps are consumed (group acts as a barrier).
3. **Given** a scenario with an unordered group where step B has `calls: {min: 1, max: 3}`, **When** a script calls B twice then A once, **Then** both calls to B are counted against step B's budget, A is consumed, and verification passes.
4. **Given** a scenario with an unordered group, **When** a script calls a command that matches none of the group's steps and no subsequent ordered step, **Then** a mismatch error is reported listing all unconsumed group steps as possible matches.
5. **Given** a scenario with nested groups (a group inside a group), **When** the scenario is loaded, **Then** validation rejects it with a clear error (nesting not supported in this version).
6. **Given** a scenario with an empty group (no steps), **When** the scenario is loaded, **Then** validation rejects it with a clear error.
7. **Given** a scenario with only ordered steps (no groups), **When** replayed as before, **Then** behavior is identical to today (backward compatible).

---

### Edge Cases

- What happens when `--format json` is used but no state file exists? → JSON output with `"passed": false` and `"error": "no state found"` message; exit code 1.
- What happens when `--format junit` encounters a step with `calls: {min: 0, max: 5}`? → If uncalled, step appears as a `<testcase>` with a `<skipped>` element and message `"optional step (min=0), not called"`. Not counted as a failure (min=0 means optional).
- What happens when all steps in an unordered group have `calls: {min: 0}`? → Group is satisfied immediately; replay advances past it.
- What happens when a command matches a step inside an unordered group AND a subsequent ordered step? → The group step takes priority while the group is active (group must be exhausted before advancing).
- What happens when `--format` is used with `cli-replay exec`? → Exec uses `--report-file <path>` to write structured verification results to a file, keeping stdout for the child. If `--format` is set without `--report-file`, structured output goes to stderr.
- How does schema validation interact with the `when` conditional field? → Schema declares `when` as an optional string; semantic validation of the expression is a runtime concern, not a schema concern.
- What happens when a JSON Schema URL is unreachable? → IDE shows a warning about schema resolution; scenario still loads normally (schema is advisory, not enforced at runtime).

## Requirements *(mandatory)*

### Functional Requirements

**Machine-Readable Output (US1):**

- **FR-001**: The `verify` command MUST accept a `--format` flag with valid values: `text` (default), `json`, `junit`.
- **FR-002**: When `--format json` is specified, `verify` MUST write a JSON object to **stdout** containing: `scenario` (name), `session` (session identifier, or `"default"` if none), `passed` (boolean), `total_steps` (integer), `consumed_steps` (integer), and `steps` (array of per-step objects). The output covers a single session only (the one specified via `--session`, or the latest/default session if omitted).
- **FR-003**: Each per-step JSON object MUST include: `index` (0-based), `label` (argv summary), `call_count` (integer), `min` (integer), `max` (integer), `passed` (boolean). For steps within a group, the `label` MUST be prefixed with `[group:<group-name>]` (e.g., `[group:pre-flight] kubectl get nodes`), and the object MUST include an additional `group` field (string, group name).
- **FR-004**: When `--format junit` is specified, `verify` MUST write a valid JUnit XML document to **stdout** with one `<testsuite>` per scenario and one `<testcase>` per step. Steps within a group MUST be flattened into individual `<testcase>` elements with a name prefix of `[group:<group-name>]` (e.g., `[group:pre-flight] kubectl get nodes`).
- **FR-005**: Failed steps in JUnit output MUST include a `<failure>` element with a message describing the shortfall (e.g., "called 0 times, minimum 1 required").
- **FR-006**: When `--format text` is specified (or `--format` is omitted), `verify` MUST produce the existing human-readable output to **stderr**, unchanged.
- **FR-007**: Structured output (json, junit) MUST go to **stdout** so it can be piped or redirected. Human-readable status messages (if any) remain on **stderr**.
- **FR-008**: The `exec` command MUST support a `--report-file <path>` flag that writes structured verification output (JSON or JUnit, selected via `--format`) to the specified file after the child exits. Stdout remains reserved for the child process. If `--format` is set without `--report-file`, `exec` MUST write the verification output to stderr in the chosen structured format.

**JSON Schema (US2):**

- **FR-009**: A JSON Schema file MUST be published in the repository at a well-known path (e.g., `schema/scenario.schema.json`).
- **FR-010**: The schema MUST validate all fields in the current scenario format: `meta` (name, description, vars, security), `steps` (match, respond, calls, when).
- **FR-011**: The schema MUST enforce mutual exclusivity between `stdout`/`stdout_file` and between `stderr`/`stderr_file`.
- **FR-012**: The schema MUST enforce `exit` range (0–255), `calls.min` ≥ 0, `calls.max` ≥ 1.
- **FR-013**: The schema MUST include `description` annotations on all fields to enable IDE hover documentation.
- **FR-014**: Example scenario files SHOULD include a `# yaml-language-server: $schema=<raw-github-url>` comment pointing to the raw GitHub URL of `schema/scenario.schema.json` for automatic IDE schema binding. The README MUST document the full URL pattern for users to copy.

**Step Groups (US3):**

- **FR-015**: The scenario YAML format MUST support a `group` element at the step level, containing a `mode` field, an optional `name` field, and a nested `steps` array. When `name` is omitted, it MUST be auto-generated as `group-1`, `group-2`, etc. (1-based, by declaration order within the scenario).
- **FR-016**: The only supported `mode` value MUST be `unordered`; unknown modes MUST produce a validation error.
- **FR-017**: Within an unordered group, the replay engine MUST match incoming commands against **all unconsumed steps** in the group, not just the "current" step.
- **FR-018**: A group MUST act as a barrier: subsequent ordered steps are not eligible for matching until all steps in the group meet their minimum call bounds.
- **FR-019**: Steps within a group MUST support the same `calls` bounds as top-level steps (min/max invocation counts).
- **FR-020**: Nested groups (a group inside a group) MUST be rejected during scenario validation with a descriptive error.
- **FR-021**: An empty group (zero steps) MUST be rejected during scenario validation.
- **FR-022**: Verify MUST check that all steps within a group met their minimum call bounds, same as for top-level steps.
- **FR-023**: Mismatch errors within an active group MUST list all unconsumed group steps as potential matches, not just one.

### Key Entities

- **VerifyResult**: Structured output of a verification run — scenario name, pass/fail, per-step details. Serialized to JSON or JUnit XML.
- **StepResult**: Per-step verification detail — index, label, call count, bounds, pass/fail. Nested within VerifyResult.
- **ScenarioSchema**: JSON Schema document describing the scenario YAML format — field types, constraints, descriptions, mutual exclusivity rules.
- **StepGroup**: A container for steps with a matching mode. Contains a mode (`unordered`) and a list of child steps. Acts as a single "super-step" in the top-level step sequence.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: CI pipelines can consume `verify` output without custom parsing — `--format json` produces valid JSON parseable by `jq`, `--format junit` produces valid XML parseable by standard JUnit report tools.
- **SC-002**: Scenario authors using VS Code with the YAML extension get field autocompletion and typo detection within 1 second of typing, without any configuration beyond a schema comment in the YAML file.
- **SC-003**: Scenarios with pre-flight checks in unpredictable order pass verification regardless of execution order, as long as all required commands are called.
- **SC-004**: All existing scenarios and tests continue to pass without modification (backward compatible).
- **SC-005**: The JSON Schema validates 100% of the example scenarios in `examples/recordings/` without false positives.
- **SC-006**: Structured output adds less than 5ms overhead to the `verify` command compared to text output.
- **SC-007**: All new functionality is covered by automated tests.

## Assumptions

- JUnit XML follows the de facto standard schema (no official XSD, but the format used by JUnit 4/5, pytest, and Go test2json converters).
- JSON Schema draft-07 or later is used, as it is the most widely supported by IDE YAML extensions (e.g., redhat.vscode-yaml).
- The `when` conditional field's expression syntax is validated at runtime, not by the JSON Schema (schema only declares it as a string).
- Step groups do not need to support `when` conditions on the group itself in this version (only on individual steps within the group).
- The `--format` flag value is case-insensitive for usability (`JSON`, `json`, `Json` all work).

## Clarifications

### Session 2026-02-07

- Q: How should `exec` handle `--format` output given stdout is used by the child process? → A: Use `--report-file <path>` flag to write structured verification output to a file; stdout reserved for child. If `--format` without `--report-file`, structured output goes to stderr.
- Q: How should step groups (US3) be represented in JUnit output? → A: Flatten — group steps become individual `<testcase>` elements with a name prefix (e.g., `[group:pre-flight] kubectl get nodes`). No nested `<testsuite>` elements.
- Q: Should the group `name` field be required or optional? → A: Optional — auto-generate as `group-1`, `group-2`, etc. (1-based, by declaration order) if omitted. Authors can set an explicit name for readability.
- Q: Where should the JSON Schema be published for URL-based `$schema` references? → A: Raw GitHub URL from this repo (e.g., `https://raw.githubusercontent.com/<org>/<repo>/main/schema/scenario.schema.json`). Zero infrastructure, always in sync with the branch.
- Q: Should `verify --format json` report on one session or all sessions? → A: Single session only — verify the specified session (or latest if omitted), consistent with current `verify` behavior. Multi-session aggregation is out of scope.
