# Feature Specification: P3/P4 Enhancements — Production Hardening, CI Ecosystem & Documentation

**Feature Branch**: `010-p3-p4-enhancements`  
**Created**: 2026-02-07  
**Status**: Draft  
**Input**: User description: "Windows Process Management - Implement job objects for reliable process tree termination; Official GitHub Action - uses: ormasoftchile/cli-replay-action@v1 for easy CI adoption; cli-replay validate - Schema and semantic validation before execution; JUnit XML Output - --format junit for CI dashboard integration; SECURITY.md - Document threat model and security boundaries; Scenario Cookbook - Examples for Terraform, Helm, kubectl workflows; --record mode - Auto-generate scenarios from real executions; Performance benchmarks - Validate scaling beyond 100 steps"

## Scope Assessment

The following items from the user request are **already fully implemented** and excluded from this specification:

| Item | Status | Evidence |
|------|--------|----------|
| JUnit XML Output (`--format junit`) | ✅ Complete | `internal/verify/junit.go` with full XML formatter; `--format junit` on both `verify` and `exec` commands; bench tests at 200 steps |
| `--record` mode | ✅ Complete | `cmd/record.go` subcommand with `--output`, `--name`, `--description`, `--command` flags; full `internal/recorder/` package (session, shim, converter, log) |
| Performance benchmarks (100+ steps) | ✅ Complete | `BenchmarkArgvMatch` (100/500), `BenchmarkGroupMatch_50`, `BenchmarkStateRoundTrip` (100/500), `BenchmarkReplayOrchestration_100`, `BenchmarkFormatJSON_200`, `BenchmarkFormatJUnit_200` |

The following items **require work** and are specified below:

1. **Windows Job Objects** — Reliable process tree termination on Windows using job objects instead of single-process `Process.Kill()`
2. **`cli-replay validate`** — Standalone schema and semantic validation command, independent of dry-run
3. **Official GitHub Action** — Reusable composite action for easy CI adoption (`uses: ormasoftchile/cli-replay-action@v1`)
4. **SECURITY.md** — Threat model, trust boundaries, and security guidance documentation
5. **Scenario Cookbook** — Real-world examples for Terraform, Helm, and multi-tool kubectl workflows
6. **Performance Benchmark Validation** — Document baseline numbers and establish regression thresholds

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Windows Job Objects: Reliable Process Tree Termination (Priority: P1)

A CI/CD engineer runs `cli-replay exec` on Windows CI agents (GitHub Actions `windows-latest`, Azure DevOps Windows pools). The child process spawns sub-processes (e.g., `bash -c "kubectl get pods && kubectl apply -f deploy.yaml"` creates multiple child processes). When the CI job times out or the user presses Ctrl+C, `cli-replay exec` must terminate the entire process tree — not just the direct child. Today, `Process.Kill()` only kills the immediate child, leaving grandchild processes orphaned. This wastes CI agent resources, can hold file locks on the intercept directory, and prevents cleanup.

**Why this priority**: This is a correctness and reliability fix for a supported platform. The README documents Windows as a supported CI target, and the current single-process kill is documented as a "known limitation" in the troubleshooting section. Orphaned grandchild processes can block subsequent CI runs and exhaust agent resources. This is the highest-impact reliability improvement remaining.

**Independent Test**: On a Windows system, run `cli-replay exec scenario.yaml -- cmd /c "start /b ping -n 100 localhost & start /b ping -n 100 127.0.0.1"`, send Ctrl+C, and verify that all child processes are terminated and the intercept directory is cleaned up.

**Acceptance Scenarios**:

1. **Given** `cli-replay exec` running a child process that spawns grandchild processes on Windows, **When** Ctrl+C is pressed, **Then** the entire process tree (child + all descendants) is terminated and the intercept directory is removed.
2. **Given** `cli-replay exec` running a child process on Windows, **When** the child process exits normally, **Then** cleanup runs identically to the current behavior — verify, report, clean.
3. **Given** `cli-replay exec` running a child process that spawns grandchild processes on Windows, **When** `taskkill /PID <exec-pid> /F` is used to kill the exec process, **Then** the process tree is terminated on a best-effort basis (grandchildren assigned to the job are killed when the job handle closes).
4. **Given** `cli-replay exec` on a Unix system, **When** the child process is terminated, **Then** behavior is unchanged from today (signal forwarding via SIGTERM/SIGINT).
5. **Given** `cli-replay exec` on Windows with a child process that has already exited, **When** cleanup runs, **Then** no errors are produced from attempting to terminate an already-exited process tree.

---

### User Story 2 — `cli-replay validate`: Pre-Execution Schema and Semantic Validation (Priority: P1)

A scenario author wants to validate their scenario YAML files for correctness without running them. Today, validation only happens implicitly when `run` or `exec` is invoked — meaning the author must attempt a full replay session to discover errors. A standalone `cli-replay validate` command would check schema compliance (valid fields, types, ranges) and semantic rules (no forward capture references, no `min > max`, no duplicate capture keys, allowlist consistency, group structure) — without creating intercept directories, modifying PATH, or requiring a child command.

**Why this priority**: Validation is the foundation of a safe workflow. Every other feature (dry-run, exec, verify) assumes a valid scenario. A dedicated command enables CI pre-flight checks (`cli-replay validate *.yaml`), pre-commit hook integration, and faster authoring feedback loops. The internal validation logic already exists; this story exposes it as a first-class command. Shares P1 with Windows Job Objects because both address correctness gaps for supported workflows.

**Independent Test**: Run `cli-replay validate scenario.yaml` on a valid scenario and verify exit code 0 with a success message. Run it on an invalid scenario (e.g., `calls.min > calls.max`) and verify exit code 1 with descriptive errors on stderr.

**Acceptance Scenarios**:

1. **Given** a valid scenario file, **When** the user runs `cli-replay validate scenario.yaml`, **Then** the tool prints a success confirmation to stderr and exits with code 0.
2. **Given** a scenario with multiple validation errors (missing `meta.name`, invalid capture identifier, forward reference), **When** the user runs `cli-replay validate`, **Then** all errors are reported in a single pass to stderr (not just the first one) and the tool exits with code 1.
3. **Given** a scenario file that does not exist, **When** the user runs `cli-replay validate missing.yaml`, **Then** the tool reports a file-not-found error to stderr and exits with code 1.
4. **Given** a scenario with YAML syntax errors (unclosed quotes, bad indentation), **When** the user runs `cli-replay validate`, **Then** the tool reports the YAML parse error with line context to stderr and exits with code 1.
5. **Given** multiple scenario files, **When** the user runs `cli-replay validate a.yaml b.yaml c.yaml`, **Then** the tool validates all files independently and reports errors per file, exiting with code 1 if any file has errors, code 0 if all pass.
6. **Given** a valid scenario, **When** `cli-replay validate` completes, **Then** no `.cli-replay/` directory, state files, intercept wrappers, or environment changes are produced.
7. **Given** a scenario with `stdout_file` or `stderr_file` references, **When** `cli-replay validate` runs, **Then** the tool checks that the referenced files exist relative to the scenario file's directory and warns if they are missing.
8. **Given** a valid scenario, **When** the user runs `cli-replay validate --format json scenario.yaml`, **Then** the tool outputs a JSON object with `file`, `valid` (boolean), and `errors` (array) fields to stdout.

---

### User Story 3 — Official GitHub Action: One-Line CI Integration (Priority: P2)

A DevOps engineer wants to add cli-replay scenario testing to their GitHub Actions workflow. Today, they must manually install the Go toolchain or download a binary, add it to PATH, and write custom workflow steps for setup, execution, and reporting. An official reusable GitHub Action (`uses: ormasoftchile/cli-replay-action@v1`) would download the correct platform binary, make it available on PATH, and optionally run a scenario — matching the ergonomics of actions like `actions/setup-go` or `azure/cli`.

**Why this priority**: A GitHub Action is the standard distribution mechanism for CI tools in the GitHub ecosystem. The ROADMAP explicitly recommends "Create a GitHub Action wrapper — Lower the barrier for CI adoption." P2 because cli-replay can already be used in CI with manual installation steps — the action improves ergonomics, not capability.

**Independent Test**: In a GitHub Actions workflow, add `uses: ormasoftchile/cli-replay-action@v1` with a scenario path and test command. Verify the action installs cli-replay, runs the scenario, and reports results — on `ubuntu-latest`, `macos-latest`, and `windows-latest` runners.

**Acceptance Scenarios**:

1. **Given** a GitHub Actions workflow file, **When** the engineer adds `uses: ormasoftchile/cli-replay-action@v1` with inputs `scenario: test.yaml` and `run: bash test.sh`, **Then** the action downloads the correct platform binary, runs the scenario in `exec` mode, and reports pass/fail as the step outcome.
2. **Given** a workflow using the action with `format: junit` and `report-file: results.xml`, **When** the workflow completes, **Then** a JUnit XML report is written to `results.xml` and can be consumed by downstream reporting steps.
3. **Given** a workflow using the action on `windows-latest`, **When** the scenario runs, **Then** cli-replay is installed as a `.exe` and the scenario completes correctly with Windows-compatible shims.
4. **Given** a workflow using the action with `version: v1.2.3`, **When** the action runs, **Then** that specific version of cli-replay is installed (not latest).
5. **Given** a workflow using the action without specifying `version`, **When** the action runs, **Then** the latest stable release of cli-replay is installed.
6. **Given** a workflow using the action with `validate-only: true`, **When** the action runs, **Then** it runs `cli-replay validate` instead of `exec` and reports validation results without executing any scenario.
7. **Given** a workflow using the action with `allowed-commands: kubectl,az`, **When** the scenario is executed, **Then** the allowlist is enforced via the `--allowed-commands` flag.

---

### User Story 4 — SECURITY.md: Threat Model and Trust Boundaries (Priority: P2)

A security engineer or enterprise architect evaluating cli-replay for adoption needs to understand its threat model, trust boundaries, and security controls. Today, security information is scattered across the README (allowlist section, `deny_env_vars` section, troubleshooting) and the ROADMAP (trust model comments). A centralized SECURITY.md provides a single authoritative reference for: what cli-replay trusts, what it doesn't, known attack surfaces, and recommended mitigations.

**Why this priority**: Documentation that doesn't block functionality but significantly affects enterprise adoption decisions. The ROADMAP explicitly recommends "Document the trust model." Shares P2 with the GitHub Action since both serve the adoption/distribution goal.

**Independent Test**: Verify the SECURITY.md file exists at the repository root, covers all documented security controls, and is linked from the README.

**Acceptance Scenarios**:

1. **Given** a user navigating to the repository root, **When** they look for security documentation, **Then** a `SECURITY.md` file exists covering: trust model, threat scenarios, security controls, and recommendations.
2. **Given** the SECURITY.md, **When** a security reviewer reads the threat model section, **Then** it clearly states that scenario files are treated as trusted code and should be reviewed like any other source file.
3. **Given** the SECURITY.md, **When** a reader looks for PATH manipulation risks, **Then** the document explains the PATH interception mechanism, why it requires trust, and how `allowed_commands` mitigates shadowing of critical binaries.
4. **Given** the SECURITY.md, **When** a reader looks for secret exfiltration risks, **Then** the document explains how `deny_env_vars` prevents template-based leaking and its limitations (filtering applies at the `MergeVars` level, not inline text scanning).
5. **Given** the README, **When** a user reads the security-related sections, **Then** it links to SECURITY.md for the full threat model.
6. **Given** the SECURITY.md, **When** a security researcher finds a vulnerability, **Then** the document provides a clear responsible disclosure process.

---

### User Story 5 — Scenario Cookbook: Examples for Terraform, Helm, kubectl Workflows (Priority: P3)

A new user who has installed cli-replay wants to test a Terraform deployment pipeline, a Helm chart rollout, or a multi-tool kubectl workflow. Today, the examples directory has a basic kubectl demo and an Azure CLI TSG. Users must write scenarios from scratch for common DevOps tools without reference material. A cookbook of annotated, copy-paste-ready examples dramatically reduces time-to-first-value.

**Why this priority**: Examples are the most effective onboarding tool. The ROADMAP recommends "Add examples — Kubernetes deployment, Terraform workflow, Azure provisioning scenarios." This is a documentation-only effort that requires no code changes but directly impacts adoption rate.

**Independent Test**: For each cookbook example, run `cli-replay validate <example>.yaml` to verify the scenario is valid, then run `cli-replay exec <example>.yaml -- bash <example-script>.sh` to verify it works end-to-end.

**Acceptance Scenarios**:

1. **Given** a user looking for a Terraform example, **When** they browse `examples/cookbook/`, **Then** they find an annotated scenario that intercepts `terraform init`, `terraform plan`, and `terraform apply` with realistic output and a companion test script.
2. **Given** a user looking for a Helm deployment example, **When** they browse `examples/cookbook/`, **Then** they find a scenario that intercepts `helm repo add`, `helm upgrade --install`, and `helm status` with realistic output including release notes.
3. **Given** a user looking for a multi-tool kubectl workflow, **When** they browse `examples/cookbook/`, **Then** they find a scenario demonstrating a deployment pipeline (`kubectl apply`, `kubectl rollout status` with polling via `calls.min/max`, `kubectl get pods` verification) using step groups and dynamic captures.
4. **Given** a user looking for a "When to Use" guide, **When** they read the cookbook README, **Then** they find a decision matrix explaining which example applies to their use case, with links to each scenario.
5. **Given** any cookbook example scenario, **When** validated with `cli-replay validate`, **Then** the scenario passes validation with zero errors.

---

### User Story 6 — Performance Benchmark Validation: Regression Baselines (Priority: P4)

A contributor submitting changes to the replay engine, matcher, or state layer needs confidence they haven't introduced performance regressions. Today, benchmarks exist (up to 500 steps) but there are no documented baseline numbers, no threshold assertions, and no CI guidance to catch regressions automatically. This story establishes benchmark baselines, documents expected thresholds, and provides contributor guidance.

**Why this priority**: Performance validation is a quality gate, not a user-facing feature. The existing benchmarks already cover 100–500 step scenarios. This story adds documentation of baselines, contributor guidance, and threshold expectations — lowest priority because the benchmarks already exist and pass.

**Independent Test**: Run `go test -bench=. -benchmem ./...` and verify that all benchmarks complete. Verify a benchmark reference document exists in the repository.

**Acceptance Scenarios**:

1. **Given** a contributor running `go test -bench=. ./...`, **When** benchmarks execute on a standard development machine, **Then** all benchmarks complete within the documented threshold ranges.
2. **Given** a benchmark reference document, **When** a contributor reads it, **Then** they find baseline numbers for each benchmark (operations/second, nanoseconds/operation, allocations) captured on a reference system.
3. **Given** the existing 100-step replay orchestration benchmark, **When** run on current code, **Then** total processing time is under 500 milliseconds.
4. **Given** the existing 500-step state round-trip benchmark, **When** run on current code, **Then** the round-trip completes in under 10 milliseconds.
5. **Given** the existing 200-step JUnit/JSON formatting benchmarks, **When** run on current code, **Then** each format completes in under 50 milliseconds.

---

### Edge Cases

- What happens when Windows job objects are not available (older Windows versions or restricted environments)? → Fall back to `Process.Kill()` with a warning to stderr, preserving current behavior.
- What happens when the GitHub Action is used with an unsupported runner OS or architecture? → The action reports a clear error listing supported platforms.
- What happens when `cli-replay validate` is given a file that is valid YAML but not a scenario (e.g., a Kubernetes manifest)? → Reports missing required fields (`meta.name`, `steps`) and exits with code 1.
- What happens when `cli-replay validate` is given a non-YAML file (e.g., a binary or `.json`)? → Reports a YAML parse error and exits with code 1.
- What happens when `cli-replay validate` is called with no arguments? → Reports a usage error to stderr and exits with code 1.
- What happens when multiple validation errors exist in a single scenario? → `validate` reports all errors found (not just the first), so the user can fix them all in one iteration.
- What happens when cookbook scenarios reference `stdout_file` fixtures? → Each cookbook example is self-contained with inline stdout/stderr, or includes fixture files in the same directory.
- What happens when job object creation fails due to nested job objects (process already in a job)? → On Windows 8+ and Server 2012+, nested job objects are supported. For older systems, fall back to `Process.Kill()` with a warning.

## Requirements *(mandatory)*

### Functional Requirements

**Windows Job Objects (US1)**

- **FR-001**: On Windows, `cli-replay exec` MUST create a job object and assign the child process to it before allowing the child to run.
- **FR-002**: On Windows, when terminating the child process (Ctrl+C, timeout, or explicit kill), the system MUST terminate the entire job object, killing all processes in the process tree.
- **FR-003**: On Windows, if job object creation fails, the system MUST fall back to the current `Process.Kill()` behavior and emit a warning to stderr.
- **FR-004**: On Unix systems, process termination behavior MUST remain unchanged (SIGTERM/SIGINT forwarding to process group).
- **FR-005**: On Windows, the job object MUST be configured to terminate all processes when the job handle is closed, providing automatic cleanup even if the parent process is killed.

**Standalone Validate Command (US2)**

- **FR-006**: System MUST provide a `cli-replay validate <file>...` command that accepts one or more scenario file paths.
- **FR-007**: Validation MUST check schema compliance: required fields (`meta.name`, `steps`), field types, value ranges (`exit` 0–255), mutual exclusivity (`stdout`/`stdout_file`, `stderr`/`stderr_file`).
- **FR-008**: Validation MUST check semantic rules: `calls.min ≤ calls.max`, no forward capture references, no duplicate capture identifiers, capture identifiers do not conflict with `meta.vars` keys, group constraints (no nesting, non-empty, valid mode).
- **FR-009**: Validation MUST check that `stdout_file` and `stderr_file` paths exist relative to the scenario file location and report an error if missing (contributing to exit code 1).
- **FR-010**: `validate` MUST report all errors per file in a single pass (not fail on the first error).
- **FR-011**: When all files are valid, `validate` MUST exit with code 0 and print a success confirmation.
- **FR-012**: When any file has errors, `validate` MUST report errors to stderr (file path, step index if applicable, error description) and exit with code 1.
- **FR-013**: `validate` MUST accept a `--format` flag with values `text` (default) and `json` for structured error reporting.
- **FR-014**: `validate` MUST NOT create any files, directories, or modify any environment state.

**Official GitHub Action (US3)**

- **FR-015**: A composite GitHub Action MUST be defined in `action.yml` with inputs: `scenario` (required), `run` (optional), `version` (optional, default: `latest`), `format` (optional), `report-file` (optional), `validate-only` (optional, default: `false`), `allowed-commands` (optional).
- **FR-016**: The action MUST download the correct cli-replay binary for the runner's OS and architecture.
- **FR-017**: The action MUST add the cli-replay binary to PATH for subsequent workflow steps.
- **FR-018**: The action MUST invoke `cli-replay exec` with the provided scenario, command, and optional flags when `validate-only` is not set.
- **FR-019**: The action MUST invoke `cli-replay validate` when `validate-only: true`.
- **FR-020**: The action MUST support `ubuntu-latest`, `macos-latest`, and `windows-latest` runners.
- **FR-021**: The action MUST set a non-zero exit code when the scenario fails, causing the workflow step to fail.

**Security Documentation (US4)**

- **FR-022**: A `SECURITY.md` file MUST exist at the repository root.
- **FR-023**: The document MUST describe the trust model: scenarios are treated as code with equivalent trust requirements.
- **FR-024**: The document MUST enumerate threat boundaries: PATH interception risks, environment variable leaking risks and mitigations (`deny_env_vars`, `allowed_commands`), session isolation boundaries, and known limitations.
- **FR-025**: The document MUST include a vulnerability disclosure process (email or GitHub Security Advisory).
- **FR-026**: The document MUST include a "Supported Versions" table indicating which versions receive security patches.
- **FR-027**: The README MUST link to SECURITY.md from the existing security-related sections.

**Scenario Cookbook (US5)**

- **FR-028**: A `examples/cookbook/` directory MUST contain at least three annotated scenario files: Terraform workflow, Helm deployment, and multi-tool kubectl pipeline.
- **FR-029**: Each cookbook scenario MUST be self-contained (no external dependencies beyond the scenario directory) and pass `cli-replay validate`.
- **FR-030**: Each cookbook scenario MUST include inline YAML comments explaining key features used (captures, call bounds, groups, allowlist, etc.).
- **FR-031**: A `examples/cookbook/README.md` MUST provide a decision matrix ("which example fits your use case") and link to each scenario.

**Performance Benchmark Validation (US6)**

- **FR-032**: A `BENCHMARKS.md` document MUST exist documenting baseline benchmark numbers, the reference system, and threshold expectations for each benchmark.
- **FR-033**: The benchmark document MUST include instructions for contributors to run benchmarks locally and interpret results.
- **FR-034**: Existing benchmarks MUST be verified to pass within documented thresholds.

### Key Entities

- **Job Object** (Windows): An operating system resource that groups processes for collective lifecycle management. Created per `exec` invocation, assigned to the child process, configured for terminate-on-close. Not persisted — exists only for the lifetime of the exec session.
- **GitHub Action Definition** (`action.yml`): A composite action manifest declaring inputs, platform detection, binary installation, and cli-replay invocation steps. Published as a tagged release.
- **Validation Result**: The output of `cli-replay validate` — file path, list of errors (each with severity, location, and message), and pass/fail status. Serialized as text (stderr) or JSON (stdout with `--format json`). Not persisted.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On Windows, when `cli-replay exec` terminates a child that has spawned grandchild processes, all processes in the tree are killed within 5 seconds of receiving Ctrl+C or parent termination, with zero orphaned processes when the job object is successfully created and assigned.
- **SC-002**: Users can validate any scenario file in under 1 second using `cli-replay validate`, receiving all errors in a single invocation without any file system side effects.
- **SC-003**: A new GitHub Actions user can add cli-replay scenario testing to their workflow in under 5 minutes by adding a single `uses:` step — no manual binary installation or PATH configuration needed.
- **SC-004**: Security reviewers can assess cli-replay's threat model from a single SECURITY.md document without reading source code or multiple scattered documentation sections.
- **SC-005**: New users can find and adapt a relevant cookbook example for their tool (Terraform, Helm, or kubectl) within 10 minutes of reading the examples directory.
- **SC-006**: All existing benchmarks (100–500 step scales) complete within their documented thresholds, providing a regression baseline for contributors.
- **SC-007**: All new features are backward compatible — existing scenario files, workflows, and CI pipelines continue to work without modification.

## Assumptions

- **Windows version baseline**: Job objects with nested job support require Windows 8+ / Server 2012+. Older systems fall back to `Process.Kill()`. This covers all currently supported GitHub Actions and Azure DevOps Windows runner images.
- **GitHub Action is a separate repository**: The action lives at `ormasoftchile/cli-replay-action`, following the standard pattern for reusable actions (e.g., `actions/setup-go` is separate from `golang/go`). The main cli-replay repo provides builds; the action repo provides the wrapper.
- **Validate command reuses existing logic**: The `validate` command reuses the same `scenario.Load()` and `Scenario.Validate()` code paths used by `run`/`exec`. No new validation rules are introduced — only a new command entry point and multi-file reporting.
- **SECURITY.md follows GitHub conventions**: GitHub automatically surfaces `SECURITY.md` in the repository's Security tab, enabling vulnerability reporting integration.
- **Cookbook examples use synthetic output**: Cookbook scenarios use realistic but synthetic command output to avoid coupling to specific Terraform/Helm/kubectl versions.
- **Benchmark thresholds are conservative**: Documented thresholds are set at 2× the observed baseline to account for CI environment variability. The goal is regression detection, not absolute performance guarantees.
- **Multi-file validation is additive**: When validating multiple files, each file is validated independently. A failure in one file does not prevent validation of others. The overall exit code is 1 if any file fails.
