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
