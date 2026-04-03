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
