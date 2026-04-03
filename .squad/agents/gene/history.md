# Project Context

- **Owner:** Cristián
- **Project:** cli-replay — A Go framework for instrumenting tools/command calls from workflows/runbooks, enabling replay scenarios without faking from the consumer side.
- **Stack:** Go, CLI, GitHub Actions
- **Created:** 2026-04-03

## Learnings

<!-- Append new learnings below. Each entry is something lasting about the project. -->

### 2026-04-03 — Deep Architecture Analysis

#### Dual-Mode Entry Point (`main.go`)
- **Management mode** (`cli-replay`): Cobra command tree (`cmd/root.go`) with subcommands: `run`, `exec`, `record`, `verify`, `validate`, `clean`.
- **Intercept mode** (invoked via symlink/copy as e.g. `kubectl`): Detects `os.Args[0] != "cli-replay"`, calls `runner.ExecuteReplay()` to serve canned responses from the scenario file identified by `CLI_REPLAY_SCENARIO` env var.

#### Recording Mechanism (`internal/recorder/`)
- **Two capture modes:**
  1. **Shim mode** (`--command` filters): Platform-specific shim scripts are generated and placed in a temp dir prepended to PATH. On Unix: bash scripts. On Windows: `.cmd` + `.ps1` companion pair. Shims intercept the real command, run it, capture stdout/stderr/exit/stdin/timing, and append a JSON line to a `.jsonl` log file.
  2. **Direct capture mode** (no filters): Runs the command directly via `exec.Command`, captures stdout/stderr with `io.MultiWriter` to both passthrough and buffer, records a single `RecordedCommand`.
- **Data captured per invocation:** `timestamp` (RFC3339), `argv` (string array), `exit` (int 0-255), `stdout`, `stderr`, `stdin` (optional), `encoding` (optional, "base64" for non-UTF-8 content).
- **Storage format:** JSONL (one JSON object per line) during recording. Converted to YAML scenario file via `recorder.ConvertToScenario()` + `recorder.GenerateYAML()`.
- **Non-UTF-8 handling:** If stdout/stderr contain invalid UTF-8 bytes, both are base64-encoded and `encoding: "base64"` is set in the JSONL entry.
- **Session lifecycle:** `recorder.New()` → `SetupShims()` → `Execute()` → `Finalize()` → `Cleanup()`.

#### Replay Mechanism (`internal/runner/replay.go`)
- **Core function:** `ExecuteReplay(scenarioPath, argv, stdout, stderr)` — the hot path called on every intercepted command invocation.
- **Flow:**
  1. Load scenario YAML via `scenario.LoadFile()`.
  2. TTL cleanup of expired sessions if `meta.session.ttl` is configured.
  3. Flatten steps (expand groups inline) via `Scenario.FlatSteps()`.
  4. Calculate scenario hash (SHA256 of file content) for state integrity.
  5. Load or initialize state from `.cli-replay/cli-replay-<hash>.state` (JSON file).
  6. Check if scenario already complete.
  7. **Budget-aware step matching:** Skip exhausted steps (count >= max), try matching current step.
  8. **Group path (unordered):** Linear scan all steps in group with remaining budget, match first hit.
  9. **Ordered path:** Match current step. If no match but min is met, **soft-advance** to next step and retry.
  10. Increment call count for matched step.
  11. Stdin matching: if `match.stdin` is set, read actual stdin and compare (normalized \r\n → \n, trailing newlines trimmed).
  12. Auto-advance `CurrentStep` if budget exhausted.
  13. Render response via `ReplayResponseWithTemplate()` (Go text/template with vars + captures).
  14. Merge step captures into state.
  15. Write trace output if `CLI_REPLAY_TRACE=1`.
  16. Persist state atomically (write .tmp, rename).

#### Matching Strategy (`internal/matcher/argv.go`)
- **Three matching modes per argv element:**
  1. **Literal:** Exact string equality (fast path — checked first).
  2. **Wildcard:** `{{ .any }}` matches any single argument.
  3. **Regex:** `{{ .regex "<pattern>" }}` matches if value satisfies the compiled regex.
- **Matching requires same-length argv arrays** — no partial or prefix matching.
- **Performance:** Fast path checks `pattern == value` before even looking for `{{` template markers.
- `MatchDetail`/`ElementMatchDetail` provide diagnostic info on mismatch (called only on error path).

#### State Management (`internal/runner/state.go`)
- **State struct:** `ScenarioPath`, `ScenarioHash`, `CurrentStep`, `TotalSteps`, `StepCounts []int`, `ActiveGroup *int`, `InterceptDir`, `LastUpdated`, `Captures map[string]string`.
- **Persisted as:** JSON in `.cli-replay/cli-replay-<hash>.state` next to the scenario file.
- **Atomic writes:** Write to `.tmp`, then `os.Rename()`.
- **Parallel isolation:** `CLI_REPLAY_SESSION` env var is included in the hash when computing the state file path, so parallel runs against the same scenario get separate state files.
- **Legacy migration:** Old `ConsumedSteps []bool` auto-migrated to `StepCounts []int` on read.
- **Session TTL cleanup:** `CleanExpiredSessions()` scans `.cli-replay/` for state files older than TTL, removes intercept dirs + state files.

#### Scenario Data Model (`internal/scenario/model.go`)
- **Top-level:** `Scenario { Meta, Steps []StepElement }`
- **Meta:** `Name`, `Description`, `Vars map[string]string`, `Security`, `Session`.
- **Security:** `AllowedCommands []string`, `DenyEnvVars []string`.
- **Session:** `TTL string` (Go duration format).
- **StepElement:** Union type — exactly one of `*Step` or `*StepGroup`.
- **StepGroup:** `Mode string` (only "unordered"), `Name string`, `Steps []StepElement` (no nesting).
- **Step:** `Match { Argv, Stdin }`, `Respond { Exit, Stdout, Stderr, StdoutFile, StderrFile, Delay, Capture }`, `Calls *CallBounds { Min, Max }`, `When string`.
- **CallBounds defaults:** nil → {1, 1}. If only min set, max defaults to min.
- **Validation:** Strict YAML parsing (`KnownFields(true)`), semantic checks (forward capture references, capture-vs-vars conflicts, call bounds consistency).
- **Format:** YAML (gopkg.in/yaml.v3), validated against JSON Schema (`schema/scenario.schema.json`).
- **Custom YAML marshaling/unmarshaling** for StepElement and StepGroup to handle the union type pattern.

#### Template Rendering (`internal/template/render.go`)
- **Engine:** Go `text/template`.
- **Modes:**
  1. `Render()` — plain vars, `missingkey=error`.
  2. `RenderWithEnv()` — vars merged with env overrides.
  3. `RenderWithCaptures()` — vars + captures under `{{ .capture.key }}` namespace, `missingkey=zero` (unresolved captures → empty string).
- **Var merging:** `MergeVars()` copies scenario vars, then overrides from env. `MergeVarsFiltered()` additionally suppresses env overrides for names matching deny patterns.

#### Environment Variable Filtering (`internal/envfilter/filter.go`)
- `IsDenied(name, patterns)` — glob matching via `path.Match`. Fail-open on invalid patterns.
- `IsExempt(name)` — cli-replay's own env vars (`CLI_REPLAY_SESSION`, `CLI_REPLAY_SCENARIO`, `CLI_REPLAY_RECORDING_LOG`, `CLI_REPLAY_SHIM_DIR`, `CLI_REPLAY_TRACE`) are never denied.

#### Platform Abstraction (`internal/platform/`)
- **Interface:** `Platform` = `ShimGenerator` + `ShellExecutor` + `CommandResolver` + `InterceptFactory` + `Name()`.
- **Build-tagged:** `unix.go` (symlinks, bash shims), `windows.go` (.cmd + .ps1 shims, .exe copies).
- **Unix shims:** Bash scripts that strip shim dir from PATH, find real command, capture output, append JSONL.
- **Windows shims:** `.cmd` entry-point invokes companion `_<cmd>_shim.ps1` that uses `System.Diagnostics.Process` for capture.
- **JobObject** (`jobobject_windows.go`): Windows Job Object for process tree lifecycle management. `CREATE_SUSPENDED` + assign to job + resume pattern to prevent race conditions with grandchild processes.
- **FakePlatform** (`testutil/fake.go`): Test double recording all calls.

#### Signal Forwarding & Process Tree Management
- **Unix (`cmd/exec_unix.go`):** `Setpgid: true` → child gets own process group. SIGINT/SIGTERM forwarded to entire group via `Kill(-pgid, sig)`. Cleanup: SIGTERM → 100ms → SIGKILL. Fallback to single-process if Setpgid fails.
- **Windows (`cmd/exec_windows.go`):** Job Object with `KILL_ON_JOB_CLOSE`. Child started `CREATE_SUSPENDED`, assigned to job, then threads resumed. Ctrl+C → `TerminateJobObject(1)`.

#### Verification (`internal/verify/`)
- **BuildResult():** Checks each step's `callCount >= min` from state. Produces `VerifyResult` with per-step pass/fail.
- **Output formats:** Text (human-readable), JSON (`FormatJSON`), JUnit XML (`FormatJUnit`).
- **JUnit:** Full XML with test suites/cases, failure messages for unmet steps, skipped for optional (min=0) uncalled steps.

#### Child Environment (`internal/runner/childenv.go`)
- `BuildChildEnv()` builds env slice with intercept dir prepended to PATH + `CLI_REPLAY_SESSION` + `CLI_REPLAY_SCENARIO` set. Cross-platform PATH separator handling.

#### Dry Run (`internal/runner/dryrun.go`)
- `BuildDryRunReport()` produces a preview with per-step info, group membership, captures, call bounds, allowlist validation. `FormatDryRunReport()` renders as a formatted table.

#### Error Formatting (`internal/runner/errors.go`)
- Rich diagnostic output for argv mismatches: ANSI colors (auto/on/off via `CLI_REPLAY_COLOR`/`NO_COLOR`), per-element diff using `ElementMatchDetail`, regex pattern display, truncation for long argv, soft-advance context. Also handles stdin mismatch formatting.

#### Trace (`internal/runner/trace.go`)
- `CLI_REPLAY_TRACE=1` enables trace output: step index, argv, exit code per replay invocation. Also traces denied env vars.

#### Key Environment Variables
- `CLI_REPLAY_SCENARIO` — path to active scenario file.
- `CLI_REPLAY_SESSION` — session ID for parallel isolation.
- `CLI_REPLAY_TRACE` — enables trace output.
- `CLI_REPLAY_COLOR` — overrides ANSI color output.
- `CLI_REPLAY_RECORDING_LOG` — path to JSONL log file (set by recording session).
- `CLI_REPLAY_SHIM_DIR` — path to shim directory (set by recording session).
- `CLI_REPLAY_IN_SHIM` — recursion guard in shim scripts.
- `NO_COLOR` — standard no-color env var.

#### Key File Paths
- `main.go` — Entry point, dual-mode dispatch.
- `cmd/root.go` — Cobra root command, version info.
- `cmd/run.go` — Session init, intercept creation, shell setup emission.
- `cmd/exec.go` — All-in-one lifecycle (setup → spawn → wait → verify → cleanup).
- `cmd/record.go` — Recording session orchestration.
- `cmd/verify.go` — Post-run verification.
- `cmd/validate.go` — Offline schema + semantic validation.
- `cmd/clean.go` — Session cleanup (single, TTL, recursive).
- `internal/scenario/model.go` — Core data model.
- `internal/scenario/loader.go` — YAML parsing with strict field validation.
- `internal/runner/replay.go` — Hot-path replay logic.
- `internal/runner/state.go` — State persistence and session management.
- `internal/matcher/argv.go` — Argv matching engine.
- `internal/recorder/session.go` — Recording session lifecycle.
- `internal/recorder/shim.go` — JSONL log writer.
- `internal/recorder/converter.go` — RecordedCommand → Scenario conversion.
- `internal/platform/platform.go` — OS abstraction interfaces.
- `schema/scenario.schema.json` — JSON Schema for scenario files.

#### Concurrency Model
- **No goroutines in the replay hot path.** Each intercepted command invocation is a separate process that reads/writes state atomically (write .tmp + rename).
- **Parallel session isolation** via `CLI_REPLAY_SESSION` → separate state files per session.
- **Signal forwarding** uses a goroutine to relay signals to child process group (Unix) or job object (Windows).
- **State file locking:** No explicit file locking — relies on atomic rename for write safety. Concurrent reads during rename may briefly fail, but the state file is always in a consistent state.

#### Architecture Patterns
- **Symlink-based interception:** `cli-replay run` creates intercepts (symlinks on Unix, .exe copies on Windows) named after target commands. When the shell resolves the command name, it finds the intercept first (prepended to PATH), which triggers intercept mode in `main.go`.
- **Shim-based recording:** Recording uses shell-native shim scripts (bash/PowerShell) that wrap the real command, capture I/O, and log to JSONL. The shim dir is prepended to PATH during recording.
- **Strict YAML parsing:** `KnownFields(true)` rejects unknown fields at all nesting levels. Custom `UnmarshalYAML` for the step/group union type.
- **Budget-based step matching:** Each step has a call count budget (min/max). Steps are skipped when budget exhausted, soft-advanced when min met but no match.
- **Unordered groups:** Linear scan within group boundary, matching first available step with remaining budget.

#### Build & Test
- Go modules (`go.mod`), Cobra CLI framework, YAML v3.
- Build tags for platform-specific code (`windows`/`!windows`).
- `Makefile` + `build.ps1` for builds.
- `.golangci.yml` for linting, `.goreleaser.yaml` for releases.
- Tests follow Go conventions: `*_test.go` co-located with source. Integration tests suffixed `_integration_test.go`.

### 2026-04-03 — Deep Analysis of gert's Execution Engine

#### Entry Point & Command Layer (`cmd/gert/main.go`)
- **Entry:** `main()` → `loadDotEnv()` → Cobra root command dispatch.
- **Cobra tree:** `validate`, `exec`, `test`, `tui`, `serve`, `mcp`, `schema export`, `version`, `doctor`, `init`, `graph`, `freshness`, `render`, `diagram`, `runs`, `inspect`, `resume`, `annotate`, `migrate`.
- **`gert exec`** is the primary execution path: validates runbook, resolves inputs (CLI → defaults → prompt), creates executor based on mode, creates engine, runs.

#### The Three Execution Modes
1. **`real`:** `providers.RealExecutor{}` — uses `os/exec.CommandContext` directly.
2. **`replay`:** `replay.ReplayExecutor` — matches commands against pre-recorded scenario entries, returns canned responses. Two sub-modes:
   - **Scenario file** (single YAML): `replay.LoadScenario()` → `ScenarioCommand.Argv` matching.
   - **Scenario directory**: `replay.LoadStepScenario()` loads per-step JSON responses from `steps/*.json`, with optional time rebasing.
3. **`dry-run`:** `DryRunExecutor{}` — prints "would execute" and returns placeholders.

#### Core Interface: `providers.CommandExecutor`
```go
type CommandExecutor interface {
    Execute(ctx context.Context, command string, args []string, env []string) (*CommandResult, error)
}
```
- **THIS IS THE CRITICAL INTEGRATION POINT.** Every CLI command gert executes goes through this single interface.
- `CommandResult` captures: `Stdout []byte`, `Stderr []byte`, `ExitCode int`, `Duration time.Duration`.
- All three modes implement this interface. **cli-replay can integrate here as a fourth executor implementation or by wrapping the real executor.**

#### `providers.RealExecutor.Execute()` — The Actual Command Invocation
- **Direct `exec.CommandContext()`** call, no wrappers.
- Captures stdout/stderr into `bytes.Buffer` via `cmd.Stdout`/`cmd.Stderr`.
- **Windows fallback:** If command is not found, retries through `cmd.exe /C <full command line>` for shell builtins.
- Exit code extracted from `exec.ExitError`. Non-ExitError errors are propagated.
- Duration measured with `time.Since(start)`.
- **No stdin forwarding** — commands run without interactive input.

#### Engine Architecture (`pkg/engine/engine.go` — 2700+ lines)
- **`Engine` struct** is the central runtime:
  - `Runbook *schema.Runbook` — parsed runbook.
  - `State *RunState` — mutable execution state (vars, captures, history, current position).
  - `Executor providers.CommandExecutor` — injected command executor.
  - `Collector providers.EvidenceCollector` — injected evidence collector.
  - `Trace *TraceWriter` — JSONL trace writer.
  - `ToolManager *tools.Manager` — loaded tool definitions.
  - `StepScenario *replay.StepScenario` — per-step replay data.
  - `Gov *governance.GovernanceEngine` — allowlist/denylist enforcement.
  - `Redact []*governance.CompiledRedaction` — output redaction rules.
  - `OnEvent func(event EngineEvent)` — structured event callback.
  - `Logger *slog.Logger` — structured logging.
  - `Store RunStore` — durable run store (optional).

#### Step Execution Path (Critical Path)
```
Engine.Run(ctx)
  → runTree(ctx, nodes) [or runFlat(ctx)]
    → for each node:
        → evalCondition(step.When) — skip if false
        → executeStepWithRetry(ctx, index, step)
          → executeStep(ctx, index, step)
            → applyStepDelay() — optional pre-execution delay
            → getStepTimeout() — optional context.WithTimeout
            → switch step.Type:
                case "cli":   executeCLIStep()   → Executor.Execute()
                case "tool":  executeToolStep()  → ToolManager.Execute() → Executor.Execute()
                case "manual": executeManualStep() → Collector.Prompt*()
                case "invoke": executeInvokeStep() → child Engine.Run()
                case "assert": executeAssertStep() — pure assertion eval
                case "branch": executeBranchStep() — conditional fork
                case "parallel": executeParallelStep() — concurrent branches
                case "end":   executeEndStep() — terminal outcome
                case "extension": executeExtensionStep() — JSON-RPC binary
                case "noop":  — template-based capture only
          → retry loop if step.Retry configured (linear/exponential backoff)
        → Trace.Write(result)
        → SaveSnapshot / Store.SaveCheckpoint
        → merge captures → State.Captures
        → evaluate outcomes / branches
```

#### `executeCLIStep()` — Direct Command Execution
1. Resolve template vars in `step.With.Argv[]` via `resolveArgv()`.
2. Governance check: `Gov.CheckCommand(argv[0])`.
3. `Executor.Execute(ctx, argv[0], argv[1:], nil)`.
4. Apply redaction rules to stdout/stderr.
5. Extract captures: `stdout`, `stderr`, `stdout.field` (JSON path extraction).
6. Evaluate assertions: contains, not_contains, matches, exit_code, equals, not_equals, json_path.
7. Set status: passed if all assertions pass, failed otherwise.

#### Tool Execution Layer (`pkg/tools/`)
- **`Manager`** manages tool lifecycle: loading definitions, spawning processes, routing actions.
- **Three transport modes:**
  1. **`stdio`** (default): Spawns binary per action call. `executeStdio()` resolves argv templates, calls `Executor.Execute()`.
  2. **`jsonrpc`**: Persistent process via JSON-RPC 2.0 over stdio. `spawnJSONRPC()` → `proc.Call(method, params)`.
  3. **`mcp`**: MCP (Model Context Protocol) server process. `spawnMCP()` → `proc.CallTool()`.
- **Key insight:** stdio tools **share the CommandExecutor** — meaning cli-replay integration would automatically instrument tool calls too.
- Tool actions declare: argv template, args with types/defaults/enums, capture rules, governance constraints.
- `executeWithBinaryFallback()` retries with alternate binary names if not found.

#### Data Flow Through Execution
1. **Inputs:** `meta.inputs` → CLI `--var` → defaults → prompt → `State.Vars`.
2. **Template resolution:** `{{ .varName }}` in argv, instructions, args, recommendations → `resolveTemplate()` using `text/template` + `runbookFuncMap`.
3. **Condition evaluation:** `expr-lang` for clean expressions (`status == "resolved"`), Go templates for legacy `{{ }}` syntax.
4. **Captures:** CLI stdout/stderr → `step.capture` mapping → `State.Captures` → available as `{{ .captureName }}` in subsequent steps.
5. **Output routing:** `result.Output` (stdout), `result.Stderr`, `result.ExitCode` → trace, snapshot, event callback.

#### Variable/Template System
- **Two expression engines:**
  1. **Go `text/template`** with custom funcMap: `hasPrefix`, `hasSuffix`, `contains`, `list`, `has`, `lower`, `upper`, `split`, `join`, `replace`, `trimPrefix`, `trimSuffix`.
  2. **`expr-lang`** for conditions: typed evaluation, boolean results, native Go operators.
- **Variable sources:** `State.Vars` (inputs) + `State.Captures` (step outputs).
- **`buildEnv()`** merges both maps into `map[string]interface{}` for expression evaluation.
- **`parseCapture()`** auto-parses JSON arrays/objects, numbers, booleans from capture strings.
- **`extractCaptureField()`** navigates JSON dot-paths for `stdout.field.subfield` syntax.
- **`unwrapEnvelope()`** auto-unwraps `{"data": [{rows}]}` API response patterns.

#### Error Handling & Recovery
- **Step failure:** `result.Status = "failed"` → error branch (`_error` condition), `continue_on_fail`, or propagate error.
- **Retry:** `step.Retry {Max, Interval, Backoff}` — linear or exponential. Context cancellation honored between retries.
- **Resume:** `ResumeEngine()` loads latest snapshot from `.runbook/runs/<run_id>/snapshots/`, increments step index, reopens trace.
- **Crash recovery:** `RecoverRun(store)` loads latest checkpoint + replays trace events since. Event-sourced state reconstruction.
- **Timeout:** Per-step `context.WithTimeout()`, applied after delay completes.
- **Governance deny:** Step fails immediately with governance error.

#### Concurrency Model
- **Tree execution:** Sequential by default. Each step runs to completion before the next.
- **Parallel step type (`type: parallel`):** Branches run as goroutines. Contract conflict detection → fallback to sequential if write conflicts detected.
- **Parallel iterate (`concurrency > 1`):** Semaphore-based bounded concurrency. `cloneForIteration()` creates lightweight engine copy with own variable scope. Results merged in deterministic (declaration) order.
- **Approval guard:** Parallel iterate forces sequential if any step has approval requirements.
- **No goroutine pool** — simple channel-based semaphore pattern.
- **Thread safety:** `tools.Manager` uses `sync.Mutex` for process map access. Engine state is NOT thread-safe — parallel iterate clones the engine.

#### Runbook Chaining & Invoke
- **Outcome chaining (`next_runbook`):** Creates child `Engine`, maps captures/vars, runs child to completion. Max depth = 5.
- **Invoke steps (`type: invoke`):** Inline sub-runbook execution. Maps inputs/captures via template resolution. Gate conditions control parent behavior on child failure.
- **Import resolution:** `Runbook.Imports` maps aliases to file paths. Package-qualified refs resolved via `Project.ResolveRunbookRef()`.

#### State Persistence
- **`RunState`:** RunID, Mode, StartedAt, Actor, CurrentStepIndex, Vars, Captures, History, Status, Path (TreePath), InvokeStack, IterateState, ParallelSlots.
- **Snapshots:** JSON files in `.runbook/runs/<run_id>/snapshots/step-NNNN.json`.
- **Trace:** JSONL in `.runbook/runs/<run_id>/trace.jsonl`. Secret auto-redaction.
- **Annotations:** Append-only JSONL in `annotations.jsonl`.
- **Manifest:** `run.yaml` with run metadata, outcome, step summary, child run refs.
- **Durable events:** `DurableEvent {Seq, Type, Timestamp, RunID, Path, Data}` for event-sourced recovery.
- **RunStore interface:** `WriteTrace`, `ReadTraceSince`, `SaveCheckpoint`, `LoadLatestCheckpoint`, `AcquireLock`, `ReleaseLock`, `WriteManifest`, `WriteAnnotation`.
- **FileRunIndex:** Cross-run queries via `.runbook/index.jsonl`.

#### Governance System
- **Command allowlist/denylist:** `GovernanceEngine.CheckCommand()` validates argv[0]. Deny takes precedence.
- **Env var blocking:** Glob pattern matching via `filepath.Match`.
- **Output redaction:** Compiled regex patterns applied to stdout/stderr before storage.
- **Effects-based governance:** `Contract {Effects, Reads, Writes, Deterministic, Idempotent, Secrets}` → `RiskLevel` → governance rules match by risk/effects/writes → action: allow/require-approval/deny.
- **Tool-level governance:** Per-action `requires_approval`, `read_only` flags.
- **Secret auto-redaction:** Contract.Secrets env var values redacted from trace.

#### Replay System (`pkg/replay/`)
- **`Scenario`:** `Commands []ScenarioCommand` + `Evidence map[step_id → name → EvidenceValue]`.
- **`ReplayExecutor.Execute()`:** Linear scan of scenario commands, exact argv match, marks entries as used. Fail-closed (error if no match).
- **`StepScenario`:** Per-step JSON responses loaded from `steps/*.json` directory. Time rebasing via `TimeRebaser` (shifts ISO 8601 timestamps in JSON data).
- **Scenario export:** `gert exec --record <dir>` saves inputs.yaml + step JSON responses.

#### Schema Model (`pkg/schema/`)
- **`Runbook`:** APIVersion, Imports, Tools, ToolPaths, Meta, Steps (flat), Tree (nested TreeNode).
- **Step types:** tool, manual, assert, branch, parallel, end, extension, cli (legacy), invoke (legacy), noop.
- **`TreeNode`:** Step + optional Branches + optional IterateBlock. Recursive structure.
- **`IterateBlock`:** Two modes: convergence (max + until) and list (over + as). Collect expressions for aggregation. Concurrency support.
- **`ToolDefinition`:** apiVersion, Meta, Transport (stdio/jsonrpc/mcp), Governance, Capabilities, Actions.
- **`ToolAction`:** Argv templates, Method (JSON-RPC), MCPTool, typed Args, Capture rules, per-action Governance.
- **Project model:** `gert.yaml` with name, paths (runbooks/tools/scenarios), require (package deps).
- **Validation:** Strict YAML parsing (`KnownFields(true)`), domain-level semantic checks, deep cross-reference validation.

#### Testing Infrastructure (`pkg/testing/`)
- **Scenario-based testing:** `gert test` discovers scenarios at `{runbooks}/{name}/scenarios/*/`, replays each, compares against `test.yaml` assertions.
- **`test.yaml`:** Step assertions (contains, exit_code, json_path), outcome assertions (category, code), variable capture checks.

#### Key Files
- `cmd/gert/main.go` — CLI entry, 950+ lines, all Cobra commands and init().
- `pkg/engine/engine.go` — Execution engine, 2700+ lines, ALL step type handlers.
- `pkg/engine/types.go` — RunState, DurableEvent, RunManifest, Annotation.
- `pkg/engine/trace.go` — TraceWriter with secret redaction.
- `pkg/engine/snapshot.go` — JSON snapshot load/save.
- `pkg/engine/resume.go` — ResumeEngine, ResumeForServe, RestoreStepCounts.
- `pkg/engine/recover.go` — RecoverRun, event-sourced state reconstruction.
- `pkg/engine/runstore.go` — DirRunStore, FileRunIndex.
- `pkg/engine/treepath.go` — TreePath encoding for nested tree positions.
- `pkg/engine/annotations.go` — AnnotationWriter, JSONL-based.
- `pkg/providers/provider.go` — CommandExecutor, EvidenceCollector, StepResult interfaces.
- `pkg/providers/cli.go` — RealExecutor (os/exec.CommandContext).
- `pkg/providers/manual.go` — InteractiveCollector, DryRunCollector, ScenarioCollector.
- `pkg/replay/scenario.go` — Scenario YAML model.
- `pkg/replay/replay.go` — ReplayExecutor.
- `pkg/replay/step_scenario.go` — StepScenario with time rebasing.
- `pkg/tools/manager.go` — Tool Manager, process lifecycle, three transport modes.
- `pkg/tools/stdio.go` — stdio tool execution via shared CommandExecutor.
- `pkg/tools/jsonrpc.go` — Persistent JSON-RPC process management.
- `pkg/tools/mcp.go` — MCP server process management.
- `pkg/schema/schema.go` — Runbook, Step, Meta, TreeNode, IterateBlock, Branch.
- `pkg/schema/tool.go` — ToolDefinition, ToolAction, ToolArg.
- `pkg/schema/project.go` — Project, package resolution.
- `pkg/governance/*.go` — Allowlist, redaction, contract evaluation, env blocking.
- `pkg/contract/contract.go` — Effects-based risk model.
- `pkg/assertions/assertions.go` — 7 assertion types.
- `pkg/evidence/evidence.go` — Evidence types, SHA256 hashing.
- `pkg/eval/eval.go` — Standalone expr-lang + template evaluation.
- `pkg/inputs/types.go` — InputProvider interface for external input resolution.

### 2026-04-03 — gert Execution Engine Deep-Dive

#### The Single Integration Seam

Every CLI command gert executes — whether via `type: cli` steps or `type: tool` (stdio transport) — flows through one interface:

```go
type CommandExecutor interface {
    Execute(ctx context.Context, command string, args []string, env []string) (*CommandResult, error)
}

type CommandResult struct {
    Stdout   []byte
    Stderr   []byte
    ExitCode int
    Duration time.Duration
}
```

Current implementations:
1. **RealExecutor** — `os/exec.CommandContext` direct call (`pkg/providers/cli.go`)
2. **ReplayExecutor** — exact argv matching against scenario file (`pkg/replay/replay.go`)
3. **DryRunExecutor** — prints "would execute", returns placeholders

cli-replay can intercept all command execution by implementing this interface.

#### What Gets Captured

CommandResult already captures **stdout, stderr, exit code, and duration** — exactly what cli-replay records. The mapping is nearly 1:1.

#### What Does NOT Go Through CommandExecutor

- JSON-RPC tool calls (`mode: jsonrpc`) — routed through `jsonrpcProcess.Call()`, bypass executor
- MCP tool calls (`mode: mcp`) — routed through `mcpProcess.CallTool()`, bypass executor
- VS Code command dispatch — not via CLI at all
- Manual step evidence collection — uses `EvidenceCollector` interface

These are architectural constraints — unavoidable in Pattern A integration.

#### Integration Patterns (Ranked by Implementation Value)

**Pattern 1: RecordingExecutor Wrapper** ← **IMMEDIATE**

Create a wrapper executor:

```go
type RecordingExecutor struct {
    inner    providers.CommandExecutor
    recorder *clireplay.Recorder
}

func (r *RecordingExecutor) Execute(ctx context.Context, command string, args []string, env []string) (*providers.CommandResult, error) {
    result, err := r.inner.Execute(ctx, command, args, env)
    if err == nil {
        r.recorder.Record(command, args, result.Stdout, result.Stderr, result.ExitCode, result.Duration)
    }
    return result, err
}
```

**Advantages:**
- Zero changes to gert's engine
- Automatically instruments ALL cli/tool-stdio steps
- Natural injection point: `gert exec` already sets up executor based on mode
- Duration, stdout, stderr, exit code all already captured

**Where to inject:** `cmd/gert/main.go:runExec()` at line ~370, where executor is created:
```go
case "real":
    executor = &providers.RealExecutor{}
    // → executor = NewRecordingExecutor(&providers.RealExecutor{}, recorder)
```

**Pattern 2: cli-replay as a Fourth Execution Mode**

Add `--mode record` to `gert exec`:

```go
case "record":
    real := &providers.RealExecutor{}
    executor = clireplay.NewRecordingExecutor(real, outputPath)
    collector = providers.NewInteractiveCollector()
```

Pattern 1 but surfaced as first-class CLI mode. Users run `gert exec --mode record --record-output scenario/` and get a cli-replay scenario file generated from the run.

**Pattern 3: Scenario Format Bridge**

gert already has its own replay scenario format. A bidirectional converter between gert scenarios and cli-replay scenarios would let users:
- Record with cli-replay → replay in gert
- Record with gert → replay with cli-replay

gert scenario format:
```yaml
commands:
  - argv: [kubectl, get, pods, -n, default]
    stdout: "NAME  READY  STATUS..."
    exit_code: 0
```

cli-replay scenario format:
```yaml
steps:
  - match:
      argv: [kubectl, get, pods, -n, default]
    respond:
      stdout: "NAME  READY  STATUS..."
      exit: 0
```

The schemas are structurally similar; converter is straightforward.

#### Concurrency Considerations

gert has parallel iterate (`concurrency > 1`) and parallel steps (`type: parallel`). During parallel execution:
- `cloneForIteration()` creates lightweight engine copies that share the SAME executor instance
- Multiple goroutines call `Executor.Execute()` concurrently
- Any recording executor MUST be thread-safe

cli-replay's current recorder uses JSONL append (file-level atomicity) which is safe. A wrapping executor would need to ensure recording buffer is synchronized.

#### Data Flow Compatibility Matrix

| Data Point | cli-replay captures | gert captures | Compatible? |
|---|---|---|---|
| Command (argv[0]) | ✅ | ✅ | ✅ |
| Arguments (argv[1:]) | ✅ | ✅ | ✅ |
| Stdout | ✅ | ✅ | ✅ |
| Stderr | ✅ | ✅ | ✅ |
| Exit code | ✅ | ✅ | ✅ |
| Duration/timing | ✅ | ✅ | ✅ |
| Stdin | ✅ | ❌ (no stdin forwarding) | N/A |
| Encoding (base64) | ✅ | ❌ | cli-replay-only |
| Env vars | ✅ (selective) | ✅ (passed to exec) | Compatible |
| Step metadata | ❌ | ✅ | gert-only |

#### Gaps cli-replay Fills

1. **Pattern matching in replay** — gert's `ReplayExecutor.argvMatch()` requires exact argv equality. cli-replay supports wildcards (`{{ .any }}`) and regex (`{{ .regex }}`).
2. **Budget-based matching** — gert marks entries as "used" (one-shot). cli-replay has min/max call budgets per step.
3. **Unordered matching** — gert is strictly sequential. cli-replay supports unordered groups.
4. **Shim-based interception** — gert only intercepts commands it explicitly calls. cli-replay can intercept ANY command via PATH shimming.
5. **stdin matching** — gert doesn't capture or match stdin. cli-replay supports `match.stdin`.

#### Recommended Next Steps

1. **Immediate:** Implement `RecordingExecutor` wrapper (Pattern 1) — 50 lines of code, zero gert changes needed
2. **Near-term:** Build scenario format converter (Pattern 3) — enables cross-tool replay
3. **Medium-term:** Propose `--mode record` to gert upstream (Pattern 2) — first-class integration
4. **Long-term:** If cli-replay gains a `pkg/` API (per Robert's recommendation), gert could import cli-replay's matching engine to replace its own basic `argvMatch()` with cli-replay's wildcard/regex/budget system

#### Architectural Insight

gert's `CommandExecutor` interface is a perfect seam for cli-replay. The interface is minimal (one method), well-defined (context + argv + env → result), and already used as a dependency injection point. gert's existing replay mode proves the architecture supports executor swapping. Adding cli-replay as a recording/playback layer is a natural extension of gert's existing design.

The fact that `tools.Manager.executeStdio()` routes through the same `CommandExecutor` means tool calls get instrumented for free — no special handling needed.

### 2026-04-03 — Package Promotion Feasibility ("The Dream")

#### Key Findings

Conducted a full audit of all 8 internal packages for promotion to `pkg/` to enable gert library integration. Findings:

**Promote (4 packages):**
1. `internal/scenario` → `pkg/scenario` — Pure data model + YAML loader. Zero internal deps. Cleanest candidate. **Small effort.**
2. `internal/matcher` → `pkg/matcher` — Pure argv matching. Zero internal deps. Born ready. **Small effort.**
3. `internal/envfilter` → `pkg/envfilter` — Glob-based env filtering. Zero internal deps. **Small effort.**
4. `internal/runner` → `pkg/replay` (EXTRACTED, not moved) — Requires decomposing the 200-line `ExecuteReplay()` into a clean `Engine` struct that separates matching logic from file I/O, env vars, stdin reading, and trace output. **Large effort (3-5 days).**

**Defer (2 packages):**
5. `internal/template` → `pkg/rendering` — Needs refactoring: `os.Getenv()` coupling must become explicit env maps for library use. Package name collides with stdlib `text/template`. **Medium effort, but not needed for MVP.**
6. `internal/verify` → `pkg/verify` — Clean types, but blocked on runner decomposition (needs `State` type). **Small-Medium effort, defer to Phase 4.**

**Do NOT promote (2 packages):**
7. `internal/recorder` — CLI recording tool, not needed for gert integration (gert has own recording).
8. `internal/platform` — Maximum platform coupling (build tags, unsafe pointers, bash/PowerShell shim templates). Zero library value.

#### Dependency Graph Insight

Three packages are **true leaves** (zero internal deps): `scenario`, `matcher`, `envfilter`. These can be promoted in parallel with zero risk. Everything else depends on at least one of these.

The critical path is: `scenario` + `matcher` → `pkg/replay` Engine extraction → `verify`.

#### Minimal Viable Promotion

For gert to implement a `RecordingExecutor`, it needs only: `pkg/scenario` (load scenarios), `pkg/matcher` (match argv), `pkg/replay` (engine with state). That's 3 packages out of 8.

Without template rendering support (gert gets raw stdout/stderr strings), we can even skip `template` and `envfilter` entirely for the MVP.

#### The Critical Refactoring: Engine Extraction

`ExecuteReplay()` in `internal/runner/replay.go` is a 200-line function mixing pure matching logic with: file I/O (scenario loading, state persistence), `os.Stdin` reading, `os.Getenv()` calls, template rendering, trace output, TTL cleanup. 

The refactoring creates `pkg/replay.Engine` with a single `Match(argv []string) (*MatchResult, error)` method containing ONLY the matching algorithm and state mutation. All side effects stay in `internal/runner` as a thin CLI wrapper that delegates to `Engine.Match()`.

#### Risk Assessment

- **API stability**: Mitigated by v0.x versioning (Go convention: v0 can break freely)
- **State serialization**: JSON tags already stable (used for on-disk persistence today)
- **Premature abstraction**: Mitigated by building gert prototype first, then extracting API from actual usage
- **Rollback**: `internal/` originals remain untouched — CLI tool works regardless of `pkg/` state

#### Total Estimated Effort: 6-10 days

MVP (gert can integrate): 4-7 days (Phase 1 leaves + Phase 3 engine extraction).

Full analysis written to `.squad/decisions/inbox/gene-dream-feasibility.md`.

---

### 2026-04-03T17:01 — Scribe Team Sync & Decision Consolidation

**Team produced:**
1. **Clint:** Dream API contract & pkg/ promotion design (21.3 KB artifact)
2. **Gene:** internal/ → pkg/ feasibility & refactoring plan (27.2 KB artifact)
3. **Robert:** Dream consumer experience design (40.3 KB artifact)

**Scribe actions completed:**
- 3 orchestration logs (one per agent) filed
- 1 session log filed documenting parallel design sprint
- Decision inbox merged into .squad/decisions.md (consolidated 3 large artifacts into 3 decision entries)
- Inbox files deleted post-merge
- Team updates appended to agent history files
- All metadata committed to git

**Team alignment achieved:**
- All three agents aligned on phased approach to pkg/ promotion
- Consumer requirements (Robert) drive API design (Clint)
- Technical feasibility (Gene) informs promotion strategy (Clint)
- Reference implementations (Robert) validate API patterns (Clint)

**Key deliverables archived:**
- clint-dream-api-design.md → decisions.md (API contract, package boundaries, stability rules, ReplayEngine design)
- gene-dream-feasibility.md → decisions.md (extraction roadmap, dependency graph, phased implementation plan)
- robert-dream-consumer-experience.md → decisions.md (gert integration patterns, error UX, CI workflows, progressive adoption)
