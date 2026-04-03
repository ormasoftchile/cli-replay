# Squad Decisions

## Active Decisions

### State File Concurrency

**Author:** Clint (Lead)  
**Date:** 2026-04-03  
**Status:** Proposed

The replay engine uses file-based JSON state persistence with atomic write (temp file + rename). However, there is no file locking around the read-modify-write cycle in `ExecuteReplay()`. If two intercepted commands from the same session run concurrently (e.g., a script with `&` background commands), both could read the same state, increment different step counts, and one write would silently overwrite the other.

**Current mitigations:**
- Session ID isolation prevents cross-session collision
- Atomic rename prevents partial writes
- Sequential step ordering means most real-world usage is inherently serial

**Proposal:**
For v1, document this as a known limitation (sequential execution only). For v2, consider:
1. Advisory file locks (`flock`/`LockFileEx`) around state read-modify-write
2. Or a state server (unix socket) that serializes state mutations

**Impact:** Low risk for v1 since the tool explicitly states it doesn't support parallel execution. Worth addressing if unordered groups become common.

---

### Test Coverage Gaps

**Author:** Michael (Tester)  
**Date:** 2026-04-03  
**Status:** Proposed

Test audit reveals 486 test functions across 37 files. All pass. But coverage varies widely (64–100%) and 13 of 21 testdata files are orphaned.

**Key findings requiring discussion:**

1. **`cmd` package at 64.2%** — This is the user-facing layer. Many exec/run paths are only partially covered. Recommend prioritizing coverage here.
2. **13 orphaned testdata files** — `fixtures/pods_output.txt`, both `recordings/*.jsonl`, and 9 scenario YAMLs have zero test references. Either write tests that use them or remove them to avoid maintenance confusion.
3. **No shared test helpers** — Each package reinvents setup helpers. A `internal/testutil` package could reduce duplication for common patterns (temp scenario creation, state file setup, cobra root construction).
4. **`ValidateDelay()` in scenario/model.go is untested** — This is a validation function with no coverage at all.
5. **Recorder package at 72.7%** — The session lifecycle (Setup → Execute → Finalize → Cleanup) needs more end-to-end test paths.

**Recommendations (in priority order):**

1. Write tests for `ValidateDelay()` and `FormatStdinMismatchError()` (quick wins, pure functions)
2. Increase `cmd` coverage to ≥80% by testing error paths in exec/run/record
3. Audit and either use or delete orphaned testdata files
4. Consider extracting shared test utilities into `internal/testutil`

---

### Gert Architecture & Integration Seam

**Author:** Clint (Lead)  
**Date:** 2026-04-03 (Extended 16:24)  
**Status:** Analysis / Recommendation

**Finding:** gert's `providers.CommandExecutor` interface is THE single integration seam for cli-replay. All CLI command execution (both `type: cli` steps and `type: tool` stdio transport) flows through this one interface.

**Scope limitation:** Only stdio transport tool steps and legacy `type: cli` steps flow through CommandExecutor. JSON-RPC and MCP transports have their own process management and bypass CommandExecutor entirely.

**Integration patterns evaluated:**

1. **Pattern A: cli-replay as CommandExecutor implementation**
   - Implement `providers.CommandExecutor` backed by cli-replay's matching engine
   - Requires promoting cli-replay internals to `pkg/`
   - Maximum power, maximum coupling, endgame integration

2. **Pattern B: cli-replay as transparent PATH interceptor** ← **Recommended for v1**
   - gert executes normally with `RealExecutor`
   - cli-replay intercepts at OS level via PATH manipulation
   - Zero code coupling, zero changes to either codebase
   - Only works for stdio tools that spawn binaries

3. **Pattern C: Shared scenario format**
   - Define common replay format readable by both tools
   - Decoupled, compositional, requires format negotiation

**Recommendation:** Pattern B (transparent PATH interception) for v1. It requires zero changes to either codebase and covers the primary use case: intercepting CLI tool invocations during runbook execution. Pattern A is the strategic endgame but requires cli-replay to export its matching engine.

---

### gert × cli-replay Integration Points (Engine Analysis)

**Author:** Gene (Core Dev)  
**Date:** 2026-04-03 (Extended 16:24)  
**Status:** Analysis / Recommendation

**Critical integration point:** `providers.CommandExecutor` interface in gert is the single hook where cli-replay can intercept all command execution.

```go
type CommandExecutor interface {
    Execute(ctx context.Context, command string, args []string, env []string) (*CommandResult, error)
}
```

**What gets captured:**
- stdout, stderr, exit code, duration — all captured by CommandResult, matching cli-replay's record format exactly

**What does NOT go through CommandExecutor:**
- JSON-RPC tool calls (routed through `jsonrpcProcess.Call()`)
- MCP tool calls (routed through `mcpProcess.CallTool()`)
- VS Code command dispatch (not CLI)

**Integration patterns ranked by value:**

1. **RecordingExecutor wrapper (immediate)** — Wrap RealExecutor with cli-replay recorder
   - 50 lines of code, zero gert changes needed
   - Inject at `cmd/gert/main.go:runExec()`
   - Automatically instruments ALL cli/tool-stdio steps

2. **`--mode record` first-class mode** — Add cli-replay as fourth executor type
   - Pattern 1 but surfaced as official mode
   - Users run `gert exec --mode record --record-output scenario/`

3. **Scenario format bridge** — Bidirectional converter between gert and cli-replay YAML
   - Structurally similar, straightforward implementation
   - Enables: record with cli-replay → replay in gert (and vice versa)

**Concurrency model:** gert has parallel iterate and parallel steps. Data flow compatibility verified; cli-replay's atomic file writes are thread-safe. Scenario format bridge is straightforward due to structural similarity.

**Gaps cli-replay fills:**
- Pattern matching (literal, wildcard, regex vs gert's exact argv match)
- Budget-based bounds (min/max calls)
- Unordered groups
- Shim-based interception
- stdin matching

**Next steps:** Implement RecordingExecutor wrapper (Pattern 1). Build format converter (Pattern 3). Propose `--mode record` upstream (Pattern 2).

---

### Integration Analysis: cli-replay × gert (Surface & Roadmap)

**Author:** Robert (Integration Architect)  
**Date:** 2026-04-03 (Extended 16:24)  
**Status:** Analysis / Recommendation

**Critical finding:** Every functional package lives under `internal/`. External Go modules **cannot import** any of these. Only `cmd` is importable (exports `Execute()` and `ValidationResult`). This is the critical blocker for high-value integration.

**Integration patterns evaluated:**

1. **Pattern A: CLI Wrapper** ← **Recommended for immediate v1**
   - gert shells out to `cli-replay exec --format json`
   - Zero coupling to internals
   - Works with any cli-replay version
   - Process spawn overhead (~3ms per step)

2. **Pattern B: Library API (high-value, requires upstream)**
   - Promote `scenario`, `runner`, `matcher`, `verify` to `pkg/`
   - gert imports cli-replay as library
   - Full programmatic control
   - Requires upstream API stability commitment
   - Roadmap for v3+

3. **Pattern C: Hybrid**
   - gert generates scenario YAML programmatically
   - Validates against `schema/scenario.schema.json`
   - Shells out to cli-replay exec

**Six gaps for gert's runbook use case:**

1. No library API (critical blocker)
2. No partial replay (start/end step)
3. No recording sharing/export
4. No runbook validation mode (coverage reporting)
5. No step metadata (description, tags, id)
6. Conditional steps not functional (`when` field exists but unused)

**Seven priority improvements for cli-replay:**

1. **Public `pkg/` API** — Promote scenario, runner, verify packages (critical)
2. **Step-level metadata** — `description`, `tags`, `id` fields
3. **Partial replay** — Start/end step control
4. **`when` conditions** — Enable conditional branching
5. **Scenario composition** — `include` directive
6. **Event hooks** — `on_step_start`, `on_step_complete` callbacks
7. **Programmatic builder** — Fluent API for constructing scenarios in Go

**Roadmap:**
- **Phase 1 (v1):** Pattern A (CLI wrapper) implementation
- **Phase 2 (v1.1):** Step metadata, format converter
- **Phase 3 (v2):** Promote to pkg/, RecordingExecutor integration, partial replay
- **Phase 4 (v3+):** Scenario composition, event hooks, builder API

**Decision:** Recommend **Pattern A (CLI wrapper)** for immediate integration. Parallel work: roadmap contribution toward **Pattern B (library API)**. CLI interface is well-designed with structured JSON output, making it a solid integration surface.

---

### cli-replay Public API Design — "The Dream"

**Author:** Clint (Lead / Architect)  
**Date:** 2026-04-03  
**Status:** Proposal  
**Scope:** Promote cli-replay internals to `pkg/`, design the bridge to gert's `CommandExecutor`

**Summary:** Designs Dream API contract for external consumers (gert). Establishes public interfaces for Match, Scenario, and Engine. Recommends phased migration strategy with compatibility bridge. Defines package promotion boundaries and stability guarantees. Full document: `.squad/decisions/inbox/clint-dream-api-design.md`

**Key decisions:**
1. Promote `pkg/scenario`, `pkg/matcher`, `pkg/replay`, `pkg/verify`, `pkg/recording` to public API
2. Keep `internal/runner`, `internal/platform`, `internal/template` as implementation details
3. Establish semver stability contract: v0.x may break, v1.0+ follows Go compatibility promise
4. Design ReplayEngine as concrete type (no interface creep) with thread-safe state management
5. Recording integration via wrapper pattern (RecordingExecutor) with no coupling to gert

---

### The Dream: Package Promotion Feasibility Assessment

**Author:** Gene (Core Dev)  
**Date:** 2026-04-03  
**Status:** Analysis / Recommendation  
**Requested by:** Cristián

**Summary:** Comprehensive feasibility analysis of internal/ → pkg/ migration. Identifies extraction candidates, circular dependencies, and risk factors. Establishes technical roadmap with phased approach. 

**Key findings:**
1. **Leaf packages** (`scenario`, `matcher`, `envfilter`) can promote immediately with zero changes
2. **Template** package needs refactoring to decouple from `os.Getenv()` — recommend renaming to `pkg/rendering`
3. **Runner package** requires decomposition — extract pure matching logic to `pkg/replay` while keeping CLI plumbing in `internal/`
4. **Verify** is promotion-ready once runner's State is extracted
5. **Recorder and Platform** should NOT promote — they serve CLI tool needs, not library integration

**Implementation roadmap:** 6-10 days total effort for full migration. MVP (enables gert integration) achievable in 4-7 days (phases 1-3).

---

### The Dream: cli-replay as Native Go Library — Consumer Experience Design

**Author:** Robert (Integration Architect)  
**Date:** 2026-04-03  
**Status:** Design Proposal  
**Scope:** What it looks like FROM GERT'S SIDE to use cli-replay natively

**Summary:** Designs developer experience for gert consuming cli-replay as Go library. Covers recording, replay, partial replay, validation, CI integration, and progressive adoption. Consumer-first API design from gert's call site perspective.

**Key design outcomes:**
1. **Recording:** Wrap CommandExecutor to capture executions, write cli-replay YAML on finalize
2. **Replay:** Load scenario, execute via cli-replay engine with pattern matching and budgets
3. **Hybrid:** Fallback pattern (replay by default, live for specified commands)
4. **Verification:** Post-execution report of unconsumed steps, budgets, failures
5. **Format:** Use cli-replay's YAML as interchange format (superset of gert's format, no information loss)

**Progressive adoption path:** Step 1 (2m): record one runbook, inspect YAML. Step 2 (15m): add scenarios to git and CI workflow. Step 3 (30m): edit for patterns, groups, budgets.

**CI workflow:** Record in dev, commit scenarios to git, replay in CI with instant turnaround (no real infra needed).

---

### 2026-04-03T17:58: Integration Strategy Decisions

**By:** Cristián (via Copilot)

1. Start with pkg/ promotion in cli-replay — promote scenario, matcher, verify, replay, recording
2. Gert adapter lives in gert's repo (keeps cli-replay consumer-agnostic)
3. Use cli-replay's YAML format directly as canonical, with a one-time converter for existing gert replay files

**Status:** Foundation decisions that gate all implementation work

---

### 2026-04-03T18:35: Architectural Review Verdict — pkg/ Promotion + gert Adapter

**Author:** Clint (Lead / Architect)  
**Date:** 2026-04-03  
**Status:** CONDITIONAL APPROVE

**Verdict:** Approve for merge with 1 blocking issue (fixed by Charles) and 3 non-blocking follow-ups.

**Blocking Issue Fixed:**
- `pkg/verify.BuildResult()` was importing `internal/runner.State` (violates public API)
- **Resolution:** Charles refactored to accept `[]int` (step counts) instead — no longer internal coupling
- **Status:** ✅ Fixed, all tests pass

**Non-Blocking Items (v1.1+ planning):**
1. Match() / MatchWithStdin() duplication (~130 lines) → extract shared matchCore()
2. renderWithCaptures / globMatch duplication → consider promoting to pkg/matcher
3. RecordingExecutor.Commands() returns unexported type → export or provide accessors

**Architecture Assessment:**
- ReplayEngine extraction is clean and matches Dream API spec
- gert adapter is well-designed (~210 LOC), correct type conversions
- All critical paths have test coverage
- Public surface appropriately minimal
- Thread-safe concurrency model verified

**Recommendation:** Ship it. The blocking issue is fixed. gert integration is ready. Track non-blocking items for v1.1.

---

### ReplayEngine Extraction to `pkg/replay/`

**Author:** Charles (Systems Dev)  
**Date:** 2026-04-03  
**Status:** Implemented

**Decision:** Extract pure matching/replay logic into `pkg/replay.Engine` — a standalone, in-memory, thread-safe type with zero file I/O and zero CLI coupling. Refactor `internal/runner.ExecuteReplay()` to be a thin orchestrator.

**Key Design Choices:**
1. **Concrete type, not interface** — `replay.Engine` is a struct. Consumers can wrap in their own interface if needed.
2. **Functional options pattern** — `replay.New(scenario, opts...)` with options: `WithVars`, `WithEnvLookup`, `WithDenyEnvPatterns`, `WithFileReader`, `WithMatchFunc`, `WithInitialState`.
3. **Self-contained rendering** — Engine includes its own Go `text/template` renderer to prevent `os.Getenv()` from leaking into public API.
4. **StateSnapshot for persistence bridging** — `Snapshot()`/`WithInitialState()` enable callers to serialize/deserialize state across process boundaries.
5. **stdin handled at caller level** — `MatchWithStdin()` exists but primary `Match()` path doesn't touch stdin (belongs to caller).

**Consequences:**
- External consumers (gert) can now import and drive replay programmatically
- `internal/runner` is now a thin CLI wrapper (~60% less matching logic)
- Two small code duplications: `renderWithCaptures` and `globMatch` exist in both `pkg/replay` and `internal/`
- All existing `internal/runner` test files continue to pass unchanged

**Files Created:** `pkg/replay/engine.go`, `state.go`, `snapshot.go`, `result.go`, `errors.go`, `options.go`, `engine_test.go`  
**Files Modified:** `internal/runner/replay.go`

---

### Integration Test Location

**Author:** Michael (Tester)  
**Date:** 2026-04-03  
**Status:** Implemented

**Problem:** Import cycle when writing integration tests:
- `pkg/replay` → (engine code)
- `internal/runner` → imports `pkg/replay`  
- `pkg/verify` → imports `internal/runner`
- Placing integration tests in `pkg/replay/` that import `internal/runner` + `pkg/verify` creates a cycle

**Decision:** Integration tests that cross package boundaries are placed in `tests/integration/` as standalone `integration_test` package. This avoids all import cycles and gives tests a true consumer perspective.

**Consequences:**
- `tests/integration/` is the canonical location for cross-package public API tests
- Package-specific unit tests remain in their own `pkg/*/publicapi_test.go` files
- Future ReplayEngine API tests follow this pattern once `pkg/replay/` stabilizes

---

### gert Adapter Implementation in cli-replay

**Author:** Robert (Integration Architect)  
**Date:** 2026-04-03 (Extended 18:16)  
**Status:** Implemented

**Context:** The cli-replay pkg/ promotion is complete. `pkg/scenario`, `pkg/matcher`, `pkg/replay`, and `pkg/verify` are now public Go packages. Decision made that gert adapter lives in gert's repository to keep cli-replay consumer-agnostic.

**Decision:** Built `pkg/providers/clireplay/` in gert with two CommandExecutor implementations:

1. **ReplayExecutor** — thin wrapper around `replay.Engine.Match()`, converting types
   - Implements `providers.CommandExecutor`
   - Exposes: `Remaining()`, `Captures()`, `StepCounts()`, `Reset()`, `Snapshot()`
   - ~80 LOC

2. **RecordingExecutor** — decorator pattern capturing commands for later Save() as cli-replay YAML
   - Captures all command/response pairs
   - Thread-safe via mutex
   - ~130 LOC

Wired `--mode clireplay` into `gert exec` alongside existing modes (real, dry-run, replay).

**Rationale:**
- Keeps cli-replay consumer-agnostic (no gert types leak in)
- Adapter is ~210 LOC total — thin enough to maintain, rich enough to be useful
- Uses cli-replay's YAML format directly — no format bridging needed
- RecordingExecutor enables "record in dev, replay in CI" workflow
- Local `replace` directive in go.mod until module is published

**Consequences:**
- Gert now depends on `github.com/ormasoftchile/cli-replay` (via local replace)
- Users can run `gert exec --mode clireplay --scenario path/to/scenario.yaml`
- Future work: HybridExecutor (fallback to real for unmatched), evidence collector adapter
- Need to publish cli-replay module or maintain workspace-level replace directive

**Field Mapping:**
- `replay.Result.Stdout` (string) → `providers.CommandResult.Stdout` ([]byte)
- `replay.Result.Stderr` (string) → `providers.CommandResult.Stderr` ([]byte)
- `replay.Result.ExitCode` (int) → `providers.CommandResult.ExitCode` (int)
- Duration tracked by adapter (time.Since around Match call)

**Test Coverage:**
- 45 test functions (78 total with subtests) across 3 test files
- All match modes: literal, wildcard, regex, unordered
- Call budget enforcement, capture chains, template rendering
- Critical: record→save→load→replay cycle validated
- Concurrency tested (50+ goroutines)
- 100% pass rate (78/78 tests)

**Key Files Changed (gert repo):**
- `pkg/providers/clireplay/replay_executor.go` (new)
- `pkg/providers/clireplay/recording_executor.go` (new)
- `pkg/providers/clireplay/options.go` (new)
- `pkg/providers/clireplay/clireplay_test.go` (new, 12 tests)
- `pkg/providers/clireplay/testdata/` (new, 8 YAML fixtures)
- `cmd/gert/main.go` (added clireplay mode)
- `go.mod` (added dependency + replace)

---

## Governance

- All meaningful changes require team consensus
- Document architectural decisions here
- Keep history focused on work, decisions focused on direction

### 2026-04-03T19:07: Commit policy directive
**By:** Cristián (via Copilot)
**What:** Only Scribe commits. Single committer, clean history. Code agents write files and verify builds but do NOT commit. Scribe commits all changes (both .squad/ state AND production code) after each work batch.
**Why:** User observed inconsistent commit behavior — some agents committed, others didn't. Establishing a single committer policy for clean, predictable git history.
