# Feature Specification: v1.0 Release Readiness

**Feature Branch**: `011-v1-release-readiness`  
**Created**: 2026-02-08  
**Status**: Draft  
**Input**: User description: "v1.0 Release Readiness — safety verification, audit CI, process cleanup hardening, and release gating for production use"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Confident Unix Process Cleanup (Priority: P1)

An SRE runs `cli-replay exec scenario.yaml -- bash deploy.sh` in a CI pipeline. The deploy script spawns several child processes (e.g., `kubectl port-forward` in the background). When the parent cli-replay process exits — whether normally, via SIGINT/SIGTERM, or is killed by the CI runner's timeout — **all** descendant processes are reliably terminated. No orphans linger on the runner.

**Why this priority**: Process orphans on shared CI runners cause port conflicts, resource leaks, and flaky subsequent jobs. This is the most impactful safety gap remaining on Unix. The README currently says "SIGTERM to process group" but the implementation only signals the direct child — fixing this is a correctness issue.

**Independent Test**: Run cli-replay exec with a child that spawns grandchildren, kill the parent, then verify no descendants survive.

**Acceptance Scenarios**:

1. **Given** a scenario running under `exec` with a child that spawns a background grandchild, **When** cli-replay receives SIGTERM, **Then** both the child and grandchild are terminated within 5 seconds
2. **Given** a scenario running under `exec` with a process tree 3 levels deep, **When** cli-replay exits normally after scenario completion, **Then** zero orphan processes remain from the original process group
3. **Given** a scenario running under `exec`, **When** the CI runner's timeout fires a SIGKILL to cli-replay, **Then** the TTL-based session cleanup removes stale artifacts on the next run, and documentation explicitly states that SIGKILL cannot prevent orphans (OS limitation)

---

### User Story 2 - CI Badges Show Trust (Priority: P2)

A prospective user visits the cli-replay GitHub repository. At the top of the README, CI status badges immediately show that the project builds, tests pass on both Unix and Windows, and the project has a clear license. This visual trust signal reduces the "is this production-ready?" friction that blocks adoption.

**Why this priority**: Badges are a universal signal of project maturity in the Go ecosystem. Their absence creates doubt despite solid test coverage and cross-platform CI.

**Independent Test**: Push to main and verify badges render correctly with passing status.

**Acceptance Scenarios**:

1. **Given** the README is viewed on GitHub, **When** the `main` branch CI is green, **Then** badges display passing status for the CI workflow, Go version, and license
2. **Given** a PR breaks a test, **When** the CI workflow fails, **Then** the CI badge reflects the failing status within minutes

---

### User Story 3 - ReDoS Safety Documentation (Priority: P2)

A security reviewer audits cli-replay's use of regex in the `match.argv` pattern matcher. They find clear documentation in SECURITY.md that Go's `regexp` package uses RE2 (guaranteed linear-time matching), and a benchmark test demonstrating safe performance on a known-pathological pattern. The reviewer closes the finding as "mitigated by design."

**Why this priority**: The safety property already exists (Go's RE2 engine), but undocumented security properties are treated as absent by auditors. Documenting this is low effort and closes a perception gap.

**Independent Test**: Run the ReDoS benchmark and verify it completes in under 100ms, and confirm SECURITY.md contains the RE2 statement.

**Acceptance Scenarios**:

1. **Given** a scenario using the regex pattern `^(a+)+$` against a 50-character non-matching string, **When** pattern matching runs, **Then** matching completes in under 100ms (no exponential backtracking)
2. **Given** a security reviewer reads SECURITY.md, **When** they search for regex safety, **Then** they find an explicit statement that Go's RE2 engine prevents ReDoS by design

---

### User Story 4 - Reusable GitHub Action (Priority: P3)

A platform engineer wants to add cli-replay to their team's CI pipeline. Instead of writing custom workflow steps to download, extract, and configure the binary, they add a single `uses: cli-replay/cli-replay@v1` step that installs the correct binary for the runner OS and makes `cli-replay` available on PATH.

**Why this priority**: Reducing adoption friction drives usage. A reusable action is the standard distribution mechanism for CI tools in the GitHub ecosystem. Depends on release automation (Story 5) to have binaries to download.

**Independent Test**: Create a test workflow in a separate repository that uses the action and runs `cli-replay --version`.

**Acceptance Scenarios**:

1. **Given** a GitHub Actions workflow referencing the cli-replay action, **When** the job runs on a Linux runner, **Then** `cli-replay` is installed on PATH and runs successfully
2. **Given** a GitHub Actions workflow referencing the cli-replay action, **When** the job runs on a Windows runner, **Then** `cli-replay.exe` is installed on PATH and runs successfully
3. **Given** the action is referenced with a version tag (e.g., `@v1`), **When** a new patch release is published, **Then** the action resolves the latest matching release within that major version

---

### User Story 5 - Automated Release Pipeline (Priority: P3)

A maintainer tags a commit with `v1.0.0`. An automated release pipeline builds binaries for all supported platforms, generates a changelog from commit history, and publishes a release with downloadable artifacts and checksums. Users can then install via `go install` or by downloading a prebuilt binary.

**Why this priority**: Manual release processes are error-prone and block v1.0. Automated releases with proper versioning and multi-platform binaries are table stakes for a v1.0 launch.

**Independent Test**: Tag a release candidate and verify the pipeline produces binaries for all target platforms with correct version strings.

**Acceptance Scenarios**:

1. **Given** a tagged commit matching `v*`, **When** the release pipeline runs, **Then** binaries for at least 5 platform/architecture combinations are published with the release
2. **Given** a tagged commit, **When** the release is published, **Then** the binary reports the correct version via `cli-replay --version`
3. **Given** a release, **When** a user downloads a binary, **Then** a corresponding SHA256 checksum file is available for verification

---

### Edge Cases

- What happens when a user runs cli-replay on a CI runner with `ulimit` restrictions that prevent process group creation (`setpgid` fails)?
- How does the GitHub Action handle a runner without Go installed (it should download a prebuilt binary, not compile from source)?
- What happens if the release pipeline is triggered on a non-semver tag (e.g., `v1.0.0-beta`)?
- What if a user's CI system intercepts SIGTERM before cli-replay can forward it to the process group?
- What if the binary download URL changes between GitHub Releases API versions?

## Requirements *(mandatory)*

### Functional Requirements

#### Unix Process Group Hardening

- **FR-001**: On Unix, `exec` mode MUST create a new process group for the child process so all descendant processes share a group ID
- **FR-002**: On Unix, when forwarding termination signals (SIGINT, SIGTERM), cli-replay MUST send the signal to the entire process group rather than just the direct child
- **FR-003**: On cleanup (normal exit or signal), cli-replay MUST attempt to terminate the entire process group before exiting
- **FR-004**: If process group creation fails (e.g., permission denied), cli-replay MUST fall back to single-process signal forwarding and emit a warning on stderr
- **FR-005**: SECURITY.md MUST document that SIGKILL cannot be intercepted by any user-space process and that TTL-based cleanup is the mitigation for forceful-kill scenarios

#### CI Badges

- **FR-006**: README MUST display a CI status badge linked to the main branch workflow
- **FR-007**: README MUST display a Go version badge showing the minimum supported version
- **FR-008**: README MUST display a license badge

#### ReDoS Safety Documentation

- **FR-009**: SECURITY.md MUST include a section stating that the regex engine used is linear-time (RE2 class) and inherently prevents ReDoS
- **FR-010**: A performance benchmark MUST exist in the pattern-matching module that exercises a known-pathological regex pattern and demonstrates safe completion

#### GitHub Action

- **FR-011**: A reusable GitHub Action MUST be provided with a manifest that defines the action's inputs, outputs, and execution steps
- **FR-012**: The action MUST accept a `version` input (default: `latest`) to control which cli-replay release to install
- **FR-013**: The action MUST download prebuilt binaries (not compile from source) and add them to the system PATH
- **FR-014**: The action MUST support Linux, macOS, and Windows runners

#### Release Automation

- **FR-015**: The release configuration MUST produce binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, and windows/amd64
- **FR-016**: The build MUST embed the version string so `cli-replay --version` reports the release tag
- **FR-017**: A release workflow MUST trigger automatically on version tags and produce the release artifacts
- **FR-018**: Each release MUST include SHA256 checksums for all published artifacts

### Assumptions

- Container/sandbox isolation for untrusted PRs is explicitly **out of scope** — SECURITY.md already documents "no sandboxing or privilege separation" as a known limitation, and the recommendation is to use container isolation at the CI infrastructure level
- Dedicated audit scripts are replaced by the existing test suite + CI workflow as the primary verification mechanism, supplemented by the process group integration tests added in this feature
- The regex engine's linear-time guarantee is considered sufficient for ReDoS prevention; no additional regex complexity limits or timeouts are needed
- The GitHub Action downloads releases from GitHub Releases (not a package registry)
- Windows process cleanup is already handled via Job Objects (implemented in 010-p3-p4-enhancements) — this feature focuses on the Unix process group gap

### Key Entities

- **Process Group**: A Unix concept where a parent and all descendants share a group ID, enabling group-wide signal delivery
- **GitHub Action**: A reusable workflow step defined by a manifest file, distributed via the repository itself
- **Release Artifact**: A platform-specific binary attached to a versioned release with integrity checksums
- **CI Badge**: A dynamic status image embedded in the README that reflects current CI health, version, or license

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On Unix, when cli-replay exec's parent is killed with SIGTERM, 100% of descendant processes in the child's process group are terminated within 5 seconds
- **SC-002**: Pathological regex patterns (e.g., `^(a+)+$` against 50-character non-matching input) complete matching in under 100ms
- **SC-003**: README displays at least 3 badges (CI status, Go version, license) that render correctly on GitHub
- **SC-004**: A fresh GitHub Actions workflow can install cli-replay via the reusable action and run `cli-replay --version` successfully on Linux, macOS, and Windows runners
- **SC-005**: Tagged releases automatically produce downloadable binaries for at least 5 platform/architecture combinations with SHA256 checksums
- **SC-006**: `cli-replay --version` on a released binary reports the correct semantic version from the Git tag
