# Tasks: v1.0 Release Readiness

**Input**: Design documents from `/specs/011-v1-release-readiness/`  
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓, quickstart.md ✓

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Version embedding infrastructure needed by multiple stories

- [x] T001 Add `Commit` and `Date` build-time variables alongside existing `Version` in cmd/root.go
- [x] T002 Enrich version template to include commit and build date in cmd/root.go

**Checkpoint**: `go build && ./cli-replay --version` shows "cli-replay version dev" with commit/date fields

---

## Phase 2: Foundational (No User Story Blockers)

**Purpose**: This feature has no shared foundational infrastructure beyond Phase 1 — all user stories operate on independent files. User story phases can begin immediately after Setup.

---

## Phase 3: User Story 1 — Confident Unix Process Cleanup (Priority: P1) 🎯 MVP

**Goal**: Fix Unix signal forwarding to use process groups instead of single-process signaling. All descendant processes are terminated when cli-replay exits.

**Independent Test**: Build cli-replay, run exec with a child that spawns grandchildren, kill the parent with SIGTERM, verify no orphans survive.

### Implementation for User Story 1

- [x] T003 [US1] Set `Setpgid: true` on child process SysProcAttr in cmd/exec_unix.go (FR-001)
- [x] T004 [US1] Replace `Process.Signal(sig)` with `syscall.Kill(-pgid, sig)` for group-wide signal forwarding in cmd/exec_unix.go (FR-002)
- [x] T005 [US1] Add process group cleanup (SIGTERM → 100ms wait → SIGKILL) to the cleanup function in cmd/exec_unix.go (FR-003)
- [x] T006 [US1] Add Setpgid fallback in cmd/exec_unix.go and cmd/exec.go: if cmd.Start() fails with Setpgid, setupSignalForwarding returns a fallback-aware postStart/cleanup pair; exec.go retries cmd.Start() without SysProcAttr and emits warning to stderr. Both files are modified. (FR-004)
- [x] T007 [P] [US1] Add process group integration test in cmd/exec_unix_test.go: spawn a child that creates a grandchild, send SIGTERM, verify entire group is terminated within 200ms. Use `//go:build integration` tag. (FR-001, FR-002, FR-003)
- [x] T008 [P] [US1] Document SIGKILL limitation and TTL mitigation in SECURITY.md (FR-005)

**Checkpoint**: On Unix, `cli-replay exec scenario.yaml -- bash -c "sleep 100 & echo ok"` terminates the background sleep on exit. `go test -race ./cmd/...` passes. `go test -tags=integration ./cmd/...` passes the process group test.

---

## Phase 4: User Story 2 — CI Badges Show Trust (Priority: P2)

**Goal**: Add CI status, Go version, and license badges to the top of README.

**Independent Test**: View README on GitHub after push to main — three badges render correctly.

### Implementation for User Story 2

- [x] T009 [P] [US2] Add CI status badge (GitHub native URL), Go version badge (shields.io), and license badge (shields.io) after the `# cli-replay` heading in README.md (FR-006, FR-007, FR-008)

**Checkpoint**: Badges render when viewing README.md in a markdown preview.

---

## Phase 5: User Story 3 — ReDoS Safety Documentation (Priority: P2)

**Goal**: Document Go's RE2 regex safety in SECURITY.md and add a pathological-pattern benchmark.

**Independent Test**: Run `go test -bench=BenchmarkRegexPathological ./internal/matcher/` — completes in microseconds. SECURITY.md contains RE2 statement.

### Implementation for User Story 3

- [x] T010 [P] [US3] Add "Regex Safety (ReDoS Prevention)" section to SECURITY.md explaining Go's RE2 engine guarantees linear-time matching (FR-009)
- [x] T011 [P] [US3] Add `BenchmarkRegexPathological` test using pattern `^(a+)+$` against 50-char non-matching input in internal/matcher/bench_test.go (FR-010)

**Checkpoint**: `go test -bench=BenchmarkRegexPathological ./internal/matcher/` completes successfully. SECURITY.md has RE2 section.

---

## Phase 6: User Story 5 — Automated Release Pipeline (Priority: P3)

**Goal**: Tag `v*` triggers GoReleaser to build 5-platform binaries with checksums and publish a GitHub Release.

**Independent Test**: Run `goreleaser check` locally to validate config. After tagging, verify release artifacts appear on GitHub Releases.

> **Note**: Story 5 is implemented before Story 4 because the GitHub Action (Story 4) depends on release artifacts existing to download.

### Implementation for User Story 5

- [x] T012 [P] [US5] Create .goreleaser.yaml with 5-platform build matrix, archive naming, SHA256 checksums, and changelog per contracts/goreleaser.md (FR-015, FR-016, FR-018)
- [x] T013 [P] [US5] Create .github/workflows/release.yml triggered on `v*` tags using goreleaser-action@v6 per contracts/release-workflow.md (FR-017)

**Checkpoint**: `goreleaser check` passes. Release workflow YAML is valid.

---

## Phase 7: User Story 4 — Reusable GitHub Action (Priority: P3)

**Goal**: Consumers add `uses: cli-replay/cli-replay@v1` to install the binary on any runner OS.

**Independent Test**: After a release exists, a test workflow using the action installs cli-replay and runs `cli-replay --version`.

### Implementation for User Story 4

- [x] T014 [US4] Create action.yml composite action at repo root with version input, OS/arch detection, binary download, and PATH setup per contracts/github-action.md (FR-011, FR-012, FR-013, FR-014)

**Checkpoint**: `action.yml` is valid YAML. The action's install script logic handles Linux, macOS, and Windows runners.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Final validation across all stories

- [x] T015 [P] Run `go test -race -cover ./...` to verify no regressions across all packages
- [x] T016 [P] Run `go vet ./...` to verify no static analysis issues
- [x] T017 Run quickstart.md validation scenarios (process group cleanup, version output, ReDoS benchmark)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phases 3–5 (US1, US2, US3)**: Depend on Phase 1 — can all proceed **in parallel** after Setup
- **Phase 6 (US5 — Release)**: Depends on Phase 1 (version vars) — can proceed in parallel with US1/US2/US3
- **Phase 7 (US4 — GitHub Action)**: Depends on Phase 6 (needs release artifacts to exist)
- **Phase 8 (Polish)**: Depends on all previous phases

### User Story Dependencies

```
Phase 1 (Setup: T001–T002)
    ├── Phase 3 (US1: T003–T008) ──┐
    ├── Phase 4 (US2: T009)        ├── Phase 8 (Polish: T015–T017)
    ├── Phase 5 (US3: T010–T011)   │
    └── Phase 6 (US5: T012–T013)   │
            └── Phase 7 (US4: T014)┘
```

### Within User Story 1

- T003 (Setpgid) → T004 (group signal) → T005 (group cleanup) → T006 (fallback)
- T007 (integration test) is independent [P] — can run in parallel with T003–T006 (write test against expected contract)
- T008 (SECURITY.md docs) is independent [P] — can run in parallel with T003–T006

### Parallel Opportunities

```
# After Phase 1 completes, launch all independent stories:
T003–T006 (US1: process group)   # sequential within story
T007      (US1: integration test) # parallel with T003–T006
T008      (US1: SIGKILL docs)    # parallel with T003–T006
T009      (US2: badges)          # parallel with everything
T010      (US3: SECURITY.md)     # parallel with everything
T011      (US3: benchmark)       # parallel with everything
T012      (US5: goreleaser)      # parallel with everything
T013      (US5: release workflow) # parallel with everything

# After Phase 6 completes:
T014      (US4: action.yml)      # depends on release config existing
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001–T002)
2. Complete Phase 3: User Story 1 (T003–T008)
3. **STOP and VALIDATE**: Build, run `cli-replay exec` with background children, verify group cleanup
4. This alone fixes the most critical safety gap

### Incremental Delivery

1. **Setup** → Version embedding ready
2. **US1** (P1) → Process group safety fixed → **MVP milestone**
3. **US2 + US3** (P2) → Badges + ReDoS docs → Trust signals complete
4. **US5** (P3) → Release automation ready
5. **US4** (P3) → GitHub Action available → **v1.0 release-ready**
6. **Polish** → Full validation

### Total Task Count

- **Setup**: 2 tasks
- **US1 (P1)**: 6 tasks (includes integration test)
- **US2 (P2)**: 1 task
- **US3 (P2)**: 2 tasks
- **US4 (P3)**: 1 task
- **US5 (P3)**: 2 tasks
- **Polish**: 3 tasks
- **Total**: 17 tasks
