# Project Context

- **Owner:** Cristián
- **Project:** cli-replay — A Go framework for instrumenting tools/command calls from workflows/runbooks, enabling replay scenarios without faking from the consumer side.
- **Stack:** Go, CLI, GitHub Actions
- **Created:** 2026-04-03

## Learnings

<!-- Append new learnings below. Each entry is something lasting about the project. -->

### 2026-04-03 — Integration Analysis for gert

**Architecture:**
- All functional code lives under `internal/` — no public Go library API exists today.
- The only importable package is `cmd` (exports `Execute()` and `ValidationResult`).
- Key internal packages: `scenario` (types + loading), `runner` (replay engine + state), `matcher` (argv matching), `recorder` (capture + YAML gen), `verify` (results + JSON/JUnit), `template` (Go text/template), `platform` (OS abstraction), `envfilter` (deny patterns).
- CLI is the primary integration surface: `exec`, `run`, `verify`, `validate`, `record`, `clean`.
- `exec` is the recommended CI command — full lifecycle in one invocation.
- `action.yml` is setup-only (installs binary, does not run scenarios).
- Scenario schema is defined in `schema/scenario.schema.json`.

**Key file paths:**
- `main.go` — dual-mode entry point (management vs intercept based on argv[0])
- `cmd/root.go` — Cobra command tree root
- `cmd/exec.go` — exec subcommand (recommended for CI)
- `cmd/run.go` — run subcommand (eval pattern)
- `cmd/record.go` — record subcommand
- `cmd/verify.go` — verify with JSON/JUnit output
- `cmd/validate.go` — static schema validation
- `internal/runner/` — replay engine, state, mismatch errors
- `internal/scenario/` — Scenario, Step, Meta, Match, Response types
- `internal/matcher/` — ArgvMatch, regex/wildcard support
- `internal/verify/` — VerifyResult, JUnit formatting
- `examples/cookbook/` — copy-paste scenario+script pairs (terraform, helm, kubectl)

**Integration patterns:**
- For gert: CLI wrapper via `cli-replay exec --format json` is the immediate path.
- Schema-driven YAML generation is the medium-term approach.
- Public `pkg/` API promotion is the high-value upstream contribution.

**Gaps identified:**
- No library API (everything is `internal/`)
- No partial replay (start/end step)
- No step-level metadata (description, tags, id)
- `when` conditional field exists but is not evaluated yet
- No scenario composition (`include` directive)
- No event hooks for observability

**Decision:** Filed `robert-gert-integration-analysis.md` in decisions inbox recommending CLI wrapper now, library API later.

## 2026-04-03  Integration Decision Filed

- ** Decision:** Filed robert-gert-integration-analysis.md
- **Recommendation:** Pattern A (CLI wrapper) for Phase 1, CLI \xec --format json\ provides structured integration
- **Roadmap:** Phase 3 targets library API promotion (\pkg/\ migration) for high-value upstream contribution
- **Critical gap:** All functional code is under \internal/\  blocking gert library integration
- **Seven-item priority roadmap** for cli-replay improvements identified (library API, step metadata, partial replay, when conditions, scenario composition, event hooks, programmatic builder)

### 2026-04-03 — Deep Gert Codebase Study for Integration Blueprint

**Gert Architecture — Integration-Critical Findings:**

1. **CommandExecutor interface** (`pkg/providers/provider.go:43-45`) is THE seam:
   ```go
   type CommandExecutor interface {
       Execute(ctx context.Context, command string, args []string, env []string) (*CommandResult, error)
   }
   ```
   Three implementations exist: `RealExecutor` (os/exec), `ReplayExecutor` (pkg/replay), `DryRunExecutor` (prints only). This is the exact abstraction cli-replay would plug into.

2. **EvidenceCollector interface** (`pkg/providers/provider.go:49-54`) — second seam for manual steps. Three implementations: `InteractiveCollector`, `ScenarioCollector`, `DryRunCollector`.

3. **Engine** (`pkg/engine/engine.go:50-72`) — takes CommandExecutor + EvidenceCollector + mode string ("real"/"replay"/"dry-run"). `StepScenario` field for per-step replay data.

4. **Tool Manager** (`pkg/tools/manager.go`) — `NewManager(executor CommandExecutor, ...)` — shares the executor across all tool calls. Three transport modes: stdio, jsonrpc, mcp. Stdio transport resolves argv templates then calls `executor.Execute()`.

5. **Existing Replay Package** (`pkg/replay/`):
   - `Scenario`: `commands []ScenarioCommand` + `evidence map[string]map[string]*EvidenceValue`
   - `ReplayExecutor`: exact argv matching only, fail-closed, ordered consumption
   - `StepScenario`: per-step JSON responses loaded from `steps/*.json` files
   - `TimeRebaser`: shifts timestamps for realistic replay

6. **Testing Package** (`pkg/testing/`):
   - Convention: `{runbook-dir}/scenarios/{runbook-name}/*/`
   - Each scenario dir: `inputs.yaml` + `test.yaml` + `steps/*.json`
   - `TestSpec`: `expected_outcome`, `expected_captures`, `must_reach`, `must_not_reach`, `expected_step_status`
   - Runner creates engine in replay mode, evaluates assertions

7. **Recording** (`cmd/gert/main.go:522-593`):
   - `--record <dir>` flag exports run artifacts as replayable scenario
   - Creates `scenario.yaml`, `inputs.yaml`, `steps/*.json`
   - Mirrors cli-replay's `record` command but at a higher level

8. **Runbook Schema** (`pkg/schema/schema.go`):
   - Step types: tool, cli, manual, assert, branch, parallel, end, extension, invoke, noop
   - Tools declared at runbook level (`tools: [ping, curl]`), resolved via project
   - Tool step: `tool.name`, `tool.action`, `tool.args` → dispatched through ToolManager → CommandExecutor

9. **Profiles** (`pkg/schema/profile.go`, `pkg/schema/project.go`):
   - `gert.yaml` has `profiles:` map with named defaults
   - No "test" or "replay" profile convention yet — opportunity for cli-replay integration

**Critical Insight: Overlap and Divergence**

Gert's `ReplayExecutor` is a simplified version of cli-replay's replay engine:
- Gert: exact argv match only, ordered consumption, no templates, no captures
- cli-replay: regex/wildcard matching, stdin matching, templates, captures, groups, delay, ordered/unordered
- cli-replay's matcher is strictly more powerful

Both use YAML scenarios, both match argv, both return canned stdout/stderr/exit_code.

**Three Integration Strategies Identified:**

A. **cli-replay as Enhanced CommandExecutor** (library integration) — cli-replay exports a Go type implementing gert's `CommandExecutor` interface, replacing `pkg/replay/ReplayExecutor`

B. **Scenario Format Bridge** (schema compatibility) — bidirectional converter between gert scenario YAML and cli-replay scenario YAML

C. **cli-replay as Transparent Shim Layer** (process-level) — `cli-replay run --scenario X -- gert exec runbook.yaml` — gert's `RealExecutor` calls tools that are actually cli-replay shims

**Quick Win:** Strategy C works TODAY with zero code changes to either project.

**Filed:** `robert-gert-cli-replay-integration-blueprint.md` with detailed strategy, file paths, interfaces, and phased roadmap.

### 2026-04-03 — Integration Blueprint & 4-Phase Roadmap

#### Critical Finding: The internal/ Boundary

Every functional package lives under `internal/`:
- `scenario/` — Scenario types + loading
- `runner/` — Replay engine + state
- `matcher/` — Argv matching
- `recorder/` — Capture + YAML generation
- `verify/` — Results formatting

External Go modules **cannot import** any of these. Only `cmd` is importable (exports `Execute()` and `ValidationResult`). This is the critical blocker for Pattern A (library integration).

#### Audited gert's Public Surface

- `gert exec` — Run a runbook (primary command)
- `gert run` — Plan+apply workflow
- `gert plan` — Dry-run execution
- `gert apply` — Real execution + evidence collection
- Tool transport modes: stdio (CLI), JSON-RPC, MCP
- CommandExecutor injection at construction time

#### Integration Patterns Ranked

**Pattern A: CLI Wrapper** ← **RECOMMENDED v1**
- gert shells out to `cli-replay exec --format json`
- Zero coupling to cli-replay internals
- Works with any cli-replay version
- Process spawn overhead (~3ms per step)
- Immediate implementation path

**Pattern B: Library API (requires upstream promotion)**
- Promote `scenario`, `runner`, `verify` to `pkg/`
- gert imports cli-replay as library
- Full programmatic control
- Requires upstream API stability commitment
- Roadmap: v3+

**Pattern C: Hybrid (schema-driven)**
- gert generates scenario YAML programmatically
- Validates against `schema/scenario.schema.json`
- Shells out to cli-replay exec

#### Six Gaps for gert's Runbook Use Case

1. **No library API (critical blocker)** — Everything under `internal/`, can't import
2. **No partial replay** — Can't start/end at specific step for resumable runbooks
3. **No recording sharing/export** — Can't compose recorded scenarios programmatically
4. **No runbook validation mode** — No coverage reporting like `gert verify`
5. **No step metadata** — Missing description, tags, id fields
6. **Conditional steps not functional** — `when` field exists in schema but unused in engine

#### Seven Priority Improvements for cli-replay

1. **Public `pkg/` API** ← **CRITICAL** — Promote scenario, runner, verify packages
2. **Step-level metadata** — `description`, `tags`, `id` fields
3. **Partial replay** — Start/end step control via CLI flag
4. **`when` conditions** — Enable conditional branching (implement the unused field)
5. **Scenario composition** — `include` directive for modular runbooks
6. **Event hooks** — `on_step_start`, `on_step_complete` callbacks for observability
7. **Programmatic builder** — Fluent API for constructing scenarios in Go

#### Four-Phase Roadmap

**Phase 1 (v1): Immediate — CLI Wrapper Integration**
- Implement Pattern A
- gert shells out to `cli-replay exec --format json`
- Parse JSON output for structured error handling
- Gating: CLI interface stability confirmed
- Success: Full end-to-end runbook → cli-replay integration working

**Phase 2 (v1.1): Quality + Capability Improvements**
- Extract shared test utilities into `internal/testutil`
- Build bidirectional Scenario format converter (Pattern C gating)
- Add step metadata fields (description, tags, id)
- Gating: Format converter enables cross-tool replay
- Success: Both tools understand each other's scenario formats

**Phase 3 (v2): Library API Promotion (Upstream Contribution)**
- Propose PR: Promote `scenario`, `runner`, `verify` packages to `pkg/`
- Stabilize public API with semver guarantees
- Publish go.mod v2 if breaking changes needed
- Gating: Upstream PR review + merge
- Success: gert can import cli-replay as library

**Phase 4 (v3+): Deep Integration (After Phase 3)**
- Implement RecordingExecutor wrapper in gert
- Implement partial replay (start/end step) API
- Scenario composition (`include` directive)
- Event hooks for observability
- Programmatic builder API
- Gating: Phase 3 complete, design review complete
- Success: Deep, first-class integration between tools

#### Why Pattern A (CLI Wrapper) First

1. **Works today** — Zero upstream dependencies
2. **Low risk** — No API stability commitments
3. **Proven path** — Structured JSON output from `cli-replay exec` is well-designed
4. **Unblocks Phase 1** — Full runbook integration possible immediately
5. **Paves way for Phase 3** — Identifies which internals MUST go public

#### Key Architectural Insights

- gert's tool transport model (stdio, JSON-RPC, MCP) is orthogonal to cli-replay's concerns
- cli-replay's recording captures at command level; gert records at step level
- Scenario format convergence is the bridge strategy (Phase 2)
- Pattern A (CLI wrapper) is the wedge; Pattern B (library API) is the target

#### File Paths for Integration (Gert)

- `cmd/gert/main.go:runExec()` — Where executor is instantiated
- `pkg/providers/provider.go` — CommandExecutor interface definition
- `pkg/tools/manager.go` — Tool lifecycle management
- `pkg/tools/stdio.go` — stdio transport (CommandExecutor consumer)
- `pkg/replay/replay.go` — ReplayExecutor (for reference)
