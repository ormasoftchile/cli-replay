# Project Context

- **Owner:** Cristián
- **Project:** cli-replay — A Go framework for instrumenting tools/command calls from workflows/runbooks, enabling replay scenarios without faking from the consumer side.
- **Stack:** Go, CLI, GitHub Actions
- **Created:** 2026-04-03

## Learnings

<!-- Append new learnings below. Each entry is something lasting about the project. -->

### 2026-04-03 — Full Architectural Analysis

#### Architecture Overview

cli-replay is a **scenario-driven CLI replay framework** that intercepts command-line tool invocations via PATH manipulation and replays deterministic responses from YAML scenario files. It has two execution modes:

1. **Management mode** (`cli-replay <subcommand>`) — Cobra command tree for lifecycle management
2. **Intercept mode** (invoked as a symlinked/copied binary, e.g., `kubectl`) — Replays canned responses

The binary name at invocation determines the mode (`main.go:23`).

#### Component Map & Dependency Graph

```
main.go
├── cmd/           (Cobra commands: run, exec, record, validate, verify, clean)
│   ├── root.go    (Version, Commit, Date build-time vars)
│   ├── run.go     (3-step eval pattern: setup intercepts, emit shell env)
│   ├── exec.go    (1-step CI pattern: setup→spawn→verify→cleanup)
│   ├── record.go  (Record real command executions to YAML)
│   ├── validate.go (Schema + semantic validation without side effects)
│   ├── verify.go  (Check all steps consumed, JSON/JUnit output)
│   ├── clean.go   (Session cleanup, TTL-based expiry, recursive walk)
│   ├── exec_unix.go / exec_windows.go (Signal forwarding, platform-specific)
│   └── [tests for each]
│
└── internal/
    ├── scenario/   (YAML model + loader + strict validation)
    │   ├── model.go   (Scenario, Meta, Step, StepElement, StepGroup, Match, Response, CallBounds, Session, Security)
    │   └── loader.go  (Load, LoadFile, custom UnmarshalYAML for union types)
    │
    ├── runner/     (Core replay engine + state management)
    │   ├── replay.go     (ExecuteReplay: load→state→match→respond→persist)
    │   ├── state.go      (State struct, file-based JSON persistence, session isolation, TTL cleanup)
    │   ├── errors.go     (MismatchError, StdinMismatchError formatting, color output)
    │   ├── childenv.go   (BuildChildEnv for exec subprocess)
    │   ├── dryrun.go     (DryRunReport builder + formatter)
    │   ├── trace.go      (CLI_REPLAY_TRACE debug output)
    │   └── exit.go       (ExitCodeFromError helper)
    │
    ├── matcher/    (Argv matching: literal, {{ .any }}, {{ .regex "..." }})
    │   └── argv.go
    │
    ├── template/   (Go text/template rendering with vars + captures + env)
    │   └── render.go
    │
    ├── envfilter/  (Glob-based deny-list for env var filtering)
    │   └── filter.go
    │
    ├── recorder/   (Record mode: session lifecycle, shim management, YAML generation)
    │   ├── session.go    (RecordingSession: setup shims, execute, finalize)
    │   ├── shim.go       (Shim generation via Platform interface)
    │   ├── log.go        (JSONL recording log I/O)
    │   ├── converter.go  (RecordedCommand → Scenario YAML)
    │   └── command.go    (RecordedCommand type)
    │
    ├── verify/     (Structured output: JSON + JUnit XML)
    │   ├── result.go  (VerifyResult, StepResult builders)
    │   ├── json.go    (FormatJSON)
    │   └── junit.go   (FormatJUnit)
    │
    └── platform/   (OS abstraction layer — build-tagged)
        ├── platform.go  (Platform interface: ShimGenerator + ShellExecutor + CommandResolver + InterceptFactory)
        ├── unix.go      (bash shim template, symlink intercepts)
        ├── windows.go   (cmd+ps1 dual-file shims, .cmd intercepts, Job Objects)
        └── jobobject_windows.go (Windows process tree management)
```

#### Key Design Patterns

1. **Binary Name Dispatch** — Single binary serves as both manager and interceptor. `main.go` checks `filepath.Base(os.Args[0])` to route to management or intercept mode.

2. **PATH Interception** — Intercepts are symlinks (Unix) or .exe copies (Windows) placed in a directory prepended to PATH. When the OS resolves `kubectl`, it finds the cli-replay binary first.

3. **File-based State** — State persisted as JSON in `.cli-replay/` next to the scenario. Atomic write via temp+rename. SHA256-based filenames enable parallel sessions.

4. **Union Type Pattern** — `StepElement` is `Step | Group` (custom UnmarshalYAML inspects YAML keys to dispatch). Groups support `mode: unordered` for non-sequential matching.

5. **Budget-based Matching** — Steps have `CallBounds{Min, Max}`. The replay engine skips exhausted steps, soft-advances past satisfied steps, and validates min counts on verify.

6. **Platform Abstraction** — `platform.Platform` composite interface with build-tagged implementations for Unix (bash shims, symlinks, process groups) and Windows (.cmd+.ps1 shims, file copies, Job Objects).

7. **Template Engine** — Go `text/template` with capture namespace (`{{ .capture.key }}`) and env var override (with deny-list filtering).

#### Core Interfaces & Types

- `scenario.Scenario` — Root type: Meta + Steps (StepElement union array)
- `scenario.Step` — Match (argv + stdin) + Respond (exit, stdout, stderr, delay, capture) + CallBounds
- `scenario.StepGroup` — Unordered group container with Mode + Name + Steps
- `runner.State` — CurrentStep, TotalSteps, StepCounts, Captures, ActiveGroup, InterceptDir
- `runner.ReplayResult` — ExitCode, Matched, StepIndex, ScenarioName
- `platform.Platform` — ShimGenerator + ShellExecutor + CommandResolver + InterceptFactory
- `verify.VerifyResult` — Scenario, Session, Passed, Steps (StepResult array)

#### Extension Points & Configuration

- **YAML scenario files** — Consumers define match/respond pairs declaratively
- **`meta.vars`** — Template variables (overridden by env vars)
- **`meta.security.allowed_commands`** — Command allowlist (intersected with `--allowed-commands` CLI flag)
- **`meta.security.deny_env_vars`** — Glob patterns to block env var leaking
- **`meta.session.ttl`** — Auto-cleanup of stale sessions
- **`calls.min`/`calls.max`** — Retry/polling support per step
- **`--format json|junit`** — Machine-readable output for CI
- **`--dry-run`** — Preview mode without side effects
- **`CLI_REPLAY_TRACE`** — Debug tracing env var
- **`CLI_REPLAY_COLOR` / `NO_COLOR`** — Color output control

#### Build System

- **Makefile** — Unix: `make build`, `make test`, `make lint` (golangci-lint), `make build-all` (6 platform targets)
- **build.ps1** — Windows: `-Test`, `-Lint`, `-Clean`, `-All` switches
- **action.yml** — GitHub Actions composite action: downloads release binary, supports version pinning
- **Go 1.21**, CGO_ENABLED=0, static binaries, `-ldflags="-s -w"`
- **Tests**: `go test -cover ./...` — all pass, coverage ranges 64-100% across packages

#### Strengths

1. **Clean separation of concerns** — Each internal/ package has a single responsibility
2. **Strong YAML validation** — Strict parsing (KnownFields), semantic checks (forward refs, capture conflicts, allowlist), re-encoding trick for nested strict validation
3. **Cross-platform from day one** — Build-tagged platform layer with proper Unix/Windows abstractions
4. **Two usage modes** — `run` (eval pattern for interactive use) and `exec` (one-shot for CI) cover both human and automation needs
5. **Security model** — Allowlist intersection, env var deny-list, internal prefix exemptions
6. **Good error diagnostics** — Per-element diff, color output, regex pattern display, soft-advance context
7. **Parallel-safe** — Session-ID-based state isolation, atomic file writes

#### Gaps / Risks

1. **No locking on state file** — Concurrent writes from the same session could corrupt state (atomic rename helps but doesn't prevent read-modify-write races)
   - **📋 Decision:** Filed clint-state-file-locking.md proposing v1 documentation as limitation, v2 advisory locks or state server
   - **Impact:** Low risk for sequential execution; runbook-style workflows don't require parallel step execution
2. **`ExecuteReplay` is long** (~170 lines) — The budget/group/ordered matching logic could benefit from extraction
3. **Recording mode is newer** — Lower test coverage (72.7% for recorder) compared to core packages
4. **Windows intercept via .exe copy** — Expensive for large binaries; consider hardlinks or .cmd wrappers pointing at the original binary
5. **No structured logging** — All diagnostics go to stderr as fmt.Fprintf; no leveled logging
6. **`verify` reads state but `run` might use session-scoped path** — There's a subtle mismatch: `verify` uses `StateFilePath` (reads CLI_REPLAY_SESSION from env) while `run` uses `StateFilePathWithSession` explicitly
7. **Schema file exists but isn't validated programmatically** — `schema/scenario.schema.json` exists but validation uses Go struct parsing, not JSON Schema

#### Key File Paths

- Entry point: `main.go`
- CLI commands: `cmd/run.go`, `cmd/exec.go`, `cmd/record.go`
- Core replay: `internal/runner/replay.go`
- State machine: `internal/runner/state.go`
- Scenario model: `internal/scenario/model.go`
- Matcher: `internal/matcher/argv.go`
- Platform: `internal/platform/platform.go`
- Recorder: `internal/recorder/session.go`
- Build: `Makefile` (Unix), `build.ps1` (Windows)
- CI action: `action.yml`
- Dependencies: `go.mod` (cobra, testify, yaml.v3, x/sys, x/term)

### 2026-04-03 — Gert Codebase Architectural Analysis

#### What is Gert?

Gert (Governed Executable Runbook Engine) is a platform for **governed, executable, debuggable runbooks** with full traceability, evidence capture, and pluggable tool integrations. It solves the problem of turning incident response procedures (TSGs) into executable, auditable workflows with governance gates, evidence collection, and deterministic replay.

#### Architecture Overview — Package Structure

```
cmd/gert/main.go         — CLI entry point (Cobra), 44KB+ mega-file
go.work                  — Multi-module workspace: core + 6 ext modules
├── ext/serve/           — JSON-RPC 2.0 server (VS Code extension backend)
├── ext/tui/             — Bubble Tea terminal UI
├── ext/debug/           — Interactive REPL debugger
├── ext/diagram/         — Mermaid/ASCII diagram generation
├── ext/render/          — (render support)
├── ext/mcp/             — MCP protocol support
pkg/
├── schema/              — YAML parsing, JSON Schema validation, domain rules
│   ├── schema.go        — Runbook model: Runbook, TreeNode, Step, Meta, GovernancePolicy
│   ├── tool.go          — ToolDefinition, ToolAction, ToolArg, ToolCapture, 3 transports
│   ├── provider.go      — Input provider schema
│   ├── project.go       — gert.yaml project manifest
│   └── validate.go      — JSON Schema (Draft 2020-12) + domain validation
├── engine/              — Core execution engine (83KB engine.go)
│   ├── engine.go        — Engine struct, Run(), runTree(), executeStep() dispatch
│   ├── types.go         — RunState, DurableEvent, RunManifest, Annotation
│   ├── treepath.go      — TreePath: nested execution position encoding
│   ├── trace.go         — JSONL trace writer with secret auto-redaction
│   ├── snapshot.go      — JSON checkpoint save/load
│   ├── resume.go        — Run resumption from checkpoints
│   ├── runstore.go      — RunStore interface + DirRunStore + FileRunIndex
│   └── recover.go       — Crash recovery
├── providers/           — Execution abstraction layer ★ CRITICAL FOR CLI-REPLAY ★
│   ├── provider.go      — CommandExecutor, EvidenceCollector, Provider interfaces
│   ├── cli.go           — RealExecutor: os/exec.Command with Windows fallback
│   └── manual.go        — Interactive + DryRun + Scenario collectors
├── replay/              — ReplayExecutor + StepScenario + TimeRebaser
│   ├── replay.go        — ReplayExecutor implements CommandExecutor (exact argv match)
│   ├── scenario.go      — Scenario YAML model (commands + evidence)
│   └── step_scenario.go — StepScenario: per-step JSON responses, timestamp rebasing
├── tools/               — Tool lifecycle manager ★ CRITICAL FOR CLI-REPLAY ★
│   ├── manager.go       — Manager: load, validate, execute, shutdown
│   ├── stdio.go         — stdio transport: spawn binary per call
│   ├── jsonrpc.go       — JSON-RPC 2.0 persistent process transport
│   └── mcp.go           — MCP (Model Context Protocol) transport
├── governance/          — Command allowlist/denylist, redaction, env blocking, contract eval
├── contract/            — Effects-based governance model (reads/writes/effects/risk)
├── assertions/          — 7 assertion evaluators (contains, matches, exit_code, json_path, etc.)
├── evidence/            — Evidence types (text, checklist, attachment) with SHA256 hashing
├── inputs/              — Input provider framework (JSON-RPC providers, prefix routing)
├── eval/                — Expression evaluation (expr-lang + Go templates)
├── testing/             — Scenario replay test runner + assertion engine
├── diagram/             — Mermaid/ASCII diagram generation
tools/                   — Built-in tool definitions (curl.tool.yaml, ping, nslookup)
schemas/                 — JSON Schema files (runbook-v0, runbook-v1, tool-v0, gert-project)
vscode/                  — VS Code extension (TypeScript)
```

#### Runbook Model

Runbooks are YAML files with `apiVersion: runbook/v0` (or v1). Key structure:

- **`meta`**: Name, kind (mitigation/reference/composable/rca), vars, inputs, defaults, governance, prose
- **`tools`**: Declared tool names resolved to `tools/<name>.tool.yaml`
- **`tree`**: Execution tree of `TreeNode` objects (preferred over flat `steps`)
- **`imports`**: Sub-runbook references for invocation

**Step types** (10): `tool`, `manual`, `assert`, `branch`, `parallel`, `end`, `extension`, `cli` (legacy), `invoke` (legacy), `noop`

**TreeNode** contains: Step + optional Branches (conditional forks) + optional Iterate (convergence or list loops)

**Tool definitions** (.tool.yaml): Typed actions with argv templates, 3 transports (stdio, jsonrpc, mcp), governance, capture rules

#### Execution Engine — Lifecycle

1. **NewEngine()**: Create engine with runbook, executor, collector, mode, actor → sets up trace writer, governance engine, redaction rules, initial state
2. **Run()**: Dispatch to `runTree()` (tree mode) or `runFlat()` (legacy)
3. **runTree()**: Recursive tree walker — evaluates `when:` guards, executes steps, evaluates branches, handles outcomes, iterate blocks
4. **executeStep()**: Type-based dispatch switch (10 step types) → precondition check → delay → timeout → governance contract eval → step-type handler
5. **executeStepWithRetry()**: Wraps executeStep with configurable retry (linear/exponential backoff)
6. **Tracing**: Every step result written to JSONL trace file; snapshots saved after each step
7. **Outcomes**: Terminal states (resolved/escalated/no_action/needs_rca) with optional next_runbook chaining

#### Tool/Command Invocation — THE INTEGRATION SURFACE

**Critical interfaces for cli-replay integration:**

1. **`providers.CommandExecutor`** interface:
   ```go
   Execute(ctx, command string, args []string, env []string) (*CommandResult, error)
   ```
   - `RealExecutor`: Uses `os/exec.CommandContext` directly (cli.go)
   - `ReplayExecutor`: Matches argv against pre-recorded scenarios (replay.go)
   - This is THE injection point — gert already swaps executors for replay mode

2. **`tools.Manager`**: Manages tool lifecycle, takes a `CommandExecutor` at construction
   - `NewManager(executor CommandExecutor, redact)` — executor is injected
   - `executeStdio()`: Calls `m.executor.Execute(ctx, binary, argv, nil)` — goes through the injected executor
   - `executeJSONRPC()`: Persistent process via stdin/stdout JSON-RPC
   - `executeMCP()`: MCP protocol transport

3. **Execution chain for `type: tool` steps:**
   ```
   Engine.executeToolStep() → ToolManager.Execute() → [validate args, resolve templates]
     → stdio: m.executor.Execute(ctx, binary, resolvedArgv, nil)  ← CommandExecutor interface
     → jsonrpc: proc.Call(method, params)                          ← persistent process
     → mcp: proc.CallTool(ctx, toolName, args)                    ← MCP protocol
   ```

4. **Execution chain for `type: cli` steps (legacy):**
   ```
   Engine.executeCLIStep() → Gov.CheckCommand() → Executor.Execute(ctx, argv[0], argv[1:], nil)
   ```

**KEY FINDING**: Only `stdio` transport flows through CommandExecutor. JSON-RPC and MCP transports bypass it entirely — they manage their own processes.

#### State Management

- **RunState**: Complete execution state (run_id, mode, vars, captures, history, path, iterate state, parallel slots)
- **Snapshots**: JSON checkpoint after every step at `.runbook/runs/<run_id>/snapshots/step-NNNN.json`
- **Trace**: Append-only JSONL at `.runbook/runs/<run_id>/trace.jsonl`
- **Resume**: `ResumeEngine()` loads latest snapshot, advances step index, reopens trace
- **RunStore interface**: Abstraction over filesystem persistence (DirRunStore implementation)
- **RunIndex**: Cross-run queries over `.runbook/index.jsonl`
- **Lock files**: PID-based lock at `.runbook/runs/<run_id>/lock` with stale detection
- **Annotations**: Append-only JSONL for DRI notes on runs

#### Expression System

Two systems:
1. **expr-lang** (modern): For conditions (`when:`, `until:`, branch conditions) — clean syntax
2. **Go text/template** (legacy): For string interpolation (argv, instructions) — `{{ .varName }}`

Condition evaluation falls back to templates if `{{` is detected.

#### Extension Points

1. **CommandExecutor interface**: Swap real/replay/dry-run execution — primary integration point
2. **EvidenceCollector interface**: Interactive/scenario/dry-run evidence collection
3. **Provider interface**: Step-type-specific validate + execute
4. **InputProvider interface**: External input resolution (JSON-RPC providers)
5. **Tool definitions (.tool.yaml)**: Declarative tool registration with 3 transport modes
6. **Extension steps**: `type: extension` runs custom JSON-RPC 2.0 binaries
7. **OnEvent callback**: Structured event stream during execution
8. **RunStore interface**: Pluggable persistence layer
9. **go.work multi-module**: ext/ modules for serve, tui, debug, diagram, mcp, render

#### Dependencies (go.mod)

- **Cobra** — CLI framework
- **yaml.v3** — YAML parsing
- **expr-lang/expr** — Expression evaluation
- **santhosh-tekuri/jsonschema/v6** — JSON Schema validation (Draft 2020-12)
- **invopop/jsonschema** — Go struct → JSON Schema generation
- **charmbracelet/bubbletea + bubbles + lipgloss + glamour** — TUI framework
- **chzyer/readline** — Debugger REPL
- **mattn/go-runewidth** — Terminal width handling
- Go 1.25.7

#### Key Design Patterns

1. **Strategy Pattern** — `CommandExecutor` interface enables real/replay/dry-run swap at construction
2. **Type Dispatch** — `executeStep()` uses switch on step.Type for 10 step types
3. **Tree Recursion** — `runTree()` recursively walks TreeNode arrays with branch/iterate nesting
4. **Checkpoint/Resume** — Every step persists full state; `ResumeEngine()` restores from latest
5. **Append-Only Logging** — JSONL traces and annotations are never modified
6. **Transport Abstraction** — Tools support stdio/jsonrpc/mcp via transport.mode in .tool.yaml
7. **Contract-Based Governance** — Effects/reads/writes declarations evaluated against policy rules
8. **go.work Modular Architecture** — Core engine in main module, UI/server/debug split into ext/ modules

#### Critical Integration Points for cli-replay × gert

1. **CommandExecutor is THE seam** — Gert's `providers.CommandExecutor` is functionally identical to what cli-replay intercepts. Gert already swaps it for replay. cli-replay could either:
   - Replace gert's ReplayExecutor with a cli-replay-backed one (library integration)
   - Or wrap real execution through cli-replay's PATH interception (process-level)

2. **gert's ReplayExecutor is simpler than cli-replay** — Exact argv match only, no regex/wildcards, no budget/bounds, no ordered groups. cli-replay is strictly more powerful for command matching.

3. **StepScenario adds per-step JSON responses** — gert extends basic replay with step-keyed responses (for jsonrpc/mcp tools), timestamp rebasing, and evidence. cli-replay would only cover the stdio transport path.

4. **Recording is per-step** — gert's `--record` flag exports run artifacts as replayable scenarios. cli-replay's recording captures at the command level, not step level.

5. **Everything is in `pkg/`** — Unlike cli-replay's `internal/`, gert's packages are importable. External consumers CAN import gert's types.

6. **Three integration patterns viable:**
   - **A: cli-replay as CommandExecutor** — Implement `providers.CommandExecutor` that delegates to cli-replay's matcher
   - **B: cli-replay as PATH interceptor** — gert runs normally, cli-replay intercepts at OS level (transparent)
   - **C: Shared scenario format** — cli-replay generates scenarios in gert's format or vice versa

### 2026-04-03 — Gert Architecture Deep-Dive for Integration

#### CommandExecutor: The Natural Seam

Gert's `providers.CommandExecutor` interface is functionally equivalent to what cli-replay intercepts:

```go
Execute(ctx context.Context, command string, args []string, env []string) (*CommandResult, error)
```

Gert already swaps this interface for three modes:
- `RealExecutor` — `os/exec.CommandContext` direct call
- `ReplayExecutor` — exact argv matching against pre-recorded YAML
- `DryRunExecutor` — no-op

#### Scope Limitation: What Goes Through CommandExecutor

**Only stdio transport tool steps and legacy `type: cli` steps.** JSON-RPC and MCP transports bypass entirely — they manage their own persistent processes.

This means cli-replay can only intercept:
- Commands invoked by `type: cli` steps (legacy)
- Commands invoked by `type: tool` with `transport: stdio`

Not interceptable:
- JSON-RPC tools (routed through `jsonrpcProcess.Call()`)
- MCP tools (routed through `mcpProcess.CallTool()`)
- VS Code command dispatch

#### Gert's ReplayExecutor vs cli-replay

| Feature | gert ReplayExecutor | cli-replay |
|---------|-------------------|------------|
| Matching | Exact argv match | Literal, `{{ .any }}`, `{{ .regex }}` |
| Ordering | Sequential, consumed-once | Sequential + unordered groups |
| Call bounds | No (single use) | `calls.min`/`calls.max` |
| State | In-memory `used[]` array | File-based JSON persistence |
| Recording | Per-step JSON export | Session-based YAML generation |
| Scope | Commands only | Commands + stdin matching |

cli-replay is strictly more powerful for command matching. gert's replay is simpler but covers its use case.

#### Three Integration Patterns

**Pattern A: cli-replay as CommandExecutor implementation**
- Implement `providers.CommandExecutor` backed by cli-replay's matching engine
- Requires promoting cli-replay internals to `pkg/`
- Gives gert access to regex matching, budget-based bounds, unordered groups
- Maximum power, maximum coupling, endgame integration

**Pattern B: cli-replay as transparent PATH interceptor** ← **RECOMMENDED**
- gert executes normally with `RealExecutor`
- cli-replay intercepts at OS level via PATH manipulation
- Zero code coupling, zero changes to either codebase
- Only works for stdio tools that spawn binaries
- Covers primary use case

**Pattern C: Shared scenario format**
- Define common replay format readable by both
- gert generates scenarios from `--record`, cli-replay consumes them (or vice versa)
- Decoupled, compositional
- Requires format negotiation

#### Package Accessibility

Unlike cli-replay (`internal/`), gert uses `pkg/` — all packages are importable. This means gert can import cli-replay (once exported), but cli-replay can also import gert's types for building scenario-compatible outputs.

#### Recommendation for v1

**Pattern B (transparent PATH interception)** requires zero changes to either codebase and covers the primary use case: intercepting CLI tool invocations during runbook execution.

Pattern A is the strategic endgame for deep integration but requires cli-replay to export its matching engine to `pkg/`.
