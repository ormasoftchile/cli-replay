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

## Governance

- All meaningful changes require team consensus
- Document architectural decisions here
- Keep history focused on work, decisions focused on direction
