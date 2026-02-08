# cli-replay

[![CI](https://github.com/cli-replay/cli-replay/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/cli-replay/cli-replay/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.21-blue?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

A scenario-driven CLI replay tool for **black-box testing of systems that orchestrate external CLI tools**.

Record real command executions, replay them deterministically, and verify that your scripts, runbooks, and deployment pipelines call the right commands in the right order — without touching the network, requiring credentials, or spinning up real services.

## What cli-replay Does

- **Intercepts CLI calls** via PATH manipulation (symlinks on Unix, `.cmd` wrappers on Windows)
- **Records real command executions** into declarative YAML scenarios with `cli-replay record`
- **Replays predetermined responses** (stdout, stderr, exit code) when intercepted commands are invoked
- **Enforces strict step ordering** — validates that commands execute in the exact expected sequence
- **Tracks state across invocations** — each CLI call advances the scenario by one step
- **Supports flexible matching** — use `{{ .any }}` wildcards and `{{ .regex "..." }}` patterns for dynamic arguments
- **Call count bounds** — steps can declare `calls.min`/`calls.max` to support retry loops and polling without duplicating steps
- **stdin matching** — validate piped input content during replay, and capture it during recording
- **Security allowlist** — restrict which commands can be intercepted via YAML config or `--allowed-commands` flag
- **Environment variable filtering** — block sensitive env vars from leaking into template rendering via glob patterns (`deny_env_vars`)
- **Session TTL** — automatically clean up stale replay sessions older than a configurable duration
- **Rich mismatch diagnostics** — per-element diff with color output, regex pattern display, and length mismatch detail
- **Runs cross-platform** — single Go binary, no runtime dependencies, works on macOS, Linux, and Windows

## What cli-replay Does NOT Do

- **Does not test application logic** — it validates *orchestration* (which commands run, in what order, with what flags), not business logic. Use unit tests and in-process mocks for that.
- **Does not support parallel/unordered execution** — steps are matched in strict sequence. If your workflow runs commands concurrently, cli-replay cannot validate that today.
- **Does not emulate real APIs** — it replays fixed responses. If you need real service behavior, use Testcontainers or LocalStack.
- **Does not perform load/performance testing** — it's for functional validation of command sequences.
- **Does not modify your code** — it's a pure black-box tool. No interfaces, no dependency injection, no code changes required.

## When to Use cli-replay

| ✅ Good fit | ❌ Not the right tool |
|---|---|
| Validating TSG/runbook execution order | Testing how code handles real API responses |
| Testing deployment scripts that call multiple CLIs | Load or performance testing |
| CI smoke tests for multi-tool workflows | Testing application business logic |
| Ensuring scripts call the right commands with correct flags | Emulating stateful service behavior |
| Validating piped stdin content in CLI workflows | Testing concurrent/parallel command execution |
| Recording golden-path command sequences for regression | Dynamic response logic based on runtime state |

## How It Compares

| Tool | Approach | Best For | Trade-offs |
|------|----------|----------|------------|
| **cli-replay** | PATH interception + record/replay | CLI orchestration workflows | Strict ordering, no parallel execution |
| bats + mocking | Shell function overrides | Simple shell script tests | Manual mock maintenance, not cross-platform |
| Testcontainers | Real services in containers | Integration tests | Slow startup, resource-intensive |
| LocalStack | AWS API emulation | AWS-specific workflows | Limited to AWS, requires Docker |
| VCR/go-vcr | HTTP record/replay | API client testing | HTTP-only, doesn't cover CLI tools |
| In-process mocks | Interface mocking | Unit tests | White-box, requires code changes |

## Prerequisites

- **Go 1.21+** — [Install Go](https://go.dev/doc/install)
  ```bash
  # macOS: brew install go
  # Windows: winget install GoLang.Go
  go version
  ```

## Installation

```bash
# From source (Unix)
go install github.com/cli-replay/cli-replay@latest

# From source (Windows)
go build -o cli-replay.exe .

# Or download binary from releases
```

## Quick Start

### 1. Create a scenario file

```yaml
# scenario.yaml
meta:
  name: "kubectl-demo"
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS    RESTARTS   AGE
        web-0   1/1     Running   0          5m
```

### 2. Setup fake kubectl

**Unix (macOS/Linux):**
```bash
# Initialize session — creates symlinks, sets up PATH and env
eval "$(cli-replay run scenario.yaml)"
```

**Windows (PowerShell):**
```powershell
# Initialize session — creates wrappers, sets up PATH and env
.\cli-replay.exe run scenario.yaml | Invoke-Expression
```

### 3. Run your test

```bash
# This is intercepted by cli-replay
kubectl get pods

# Output:
# NAME    READY   STATUS    RESTARTS   AGE
# web-0   1/1     Running   0          5m
```

### 4. Verify completion

```bash
cli-replay verify scenario.yaml
# cli-replay: scenario "kubectl-demo" complete (1/1 steps)
```

## Scenario YAML Format

```yaml
meta:
  name: "scenario-name"           # Required: human-readable identifier
  description: "Description"       # Optional
  vars:                            # Optional: template variables
    namespace: "production"
  security:                        # Optional: restrict interceptable commands
    allowed_commands:
      - kubectl
      - az
    deny_env_vars:                 # Optional: block env vars from templates
      - "AWS_*"
      - "SECRET_*"
  session:                         # Optional: auto-cleanup stale sessions
    ttl: "10m"                     # Go duration (e.g., 10m, 1h, 30s)

steps:
  - match:
      argv: ["kubectl", "get", "pods", "-n", "{{ .namespace }}"]
      stdin: |                     # Optional: expected piped input content
        apiVersion: v1
        kind: Pod
    respond:
      exit: 0                      # Required: exit code (0-255)
      stdout: "inline output"      # Optional: literal stdout
      stderr: "error message"      # Optional: literal stderr
      stdout_file: "fixtures/out.txt"  # Optional: file-based stdout
      stderr_file: "fixtures/err.txt"  # Optional: file-based stderr
      capture:                     # Optional: capture key-value pairs for later steps
        rg_id: "/subscriptions/abc123/resourceGroups/demo-rg"
    calls:                         # Optional: call count bounds (default: exactly once)
      min: 1                       # Minimum invocations required
      max: 5                       # Maximum invocations allowed
```

### Validation Rules

- `meta.name` is required and must be non-empty
- `steps` must contain at least one step
- `match.argv` must be non-empty
- `exit` must be 0-255
- `stdout` and `stdout_file` are mutually exclusive
- `stderr` and `stderr_file` are mutually exclusive
- `calls.min` must be ≥ 0, `calls.max` must be ≥ `min` (when specified)
- `calls.min: 0` creates an optional step (can be skipped entirely)
- `deny_env_vars` entries must be non-empty strings (glob patterns via `path.Match`)
- `session.ttl` must be a valid Go duration (`time.ParseDuration`) and positive
- `capture` identifiers must match `[a-zA-Z_][a-zA-Z0-9_]*`
- `capture` keys must not conflict with `meta.vars` keys
- Templates referencing `{{ .capture.X }}` must not forward-reference (X must be defined in an earlier step)
- Unknown fields are rejected (strict YAML parsing)

### Step Groups (Unordered Matching)

Steps can be grouped for order-independent matching. Commands within a group can be called in any order, but all group steps must be satisfied before the scenario advances past the group (barrier semantics).

```yaml
steps:
  - match:
      argv: [git, status]
    respond:
      exit: 0
      stdout: "On branch main"

  - group:
      mode: unordered
      name: pre-flight        # Optional: auto-generated as "group-N" if omitted
      steps:
        - match:
            argv: [az, account, show]
          respond:
            exit: 0
            stdout: '{"name": "my-sub"}'
        - match:
            argv: [docker, info]
          respond:
            exit: 0
            stdout: "Server Version: 24.0"
        - match:
            argv: [kubectl, cluster-info]
          respond:
            exit: 0
            stdout: "Kubernetes control plane is running"

  - match:
      argv: [kubectl, apply, -f, app.yaml]
    respond:
      exit: 0
      stdout: "deployment.apps/app configured"
```

**Group rules:**
- `mode` must be `"unordered"` (the only supported mode)
- Groups cannot be nested (no groups inside groups)
- Each group must contain at least one step
- Call bounds (`calls.min`/`calls.max`) work per-step within groups
- When all steps in a group meet their `min` counts, a non-matching command advances past the group
- When all steps reach their `max` counts, the group is automatically exhausted

## Commands

### cli-replay record

Record a command execution and generate a YAML scenario file:

```bash
cli-replay record --output demo.yaml -- kubectl get pods
```

#### Flags

| Flag | Short | Type | Required | Description |
|------|-------|------|----------|-------------|
| `--output` | `-o` | string | Yes | Output YAML file path |
| `--name` | `-n` | string | No | Scenario name (default: auto-generated) |
| `--description` | `-d` | string | No | Scenario description |
| `--command` | `-c` | []string | No | Commands to intercept (can be repeated) |

#### Examples

```bash
# Record a single command
cli-replay record --output simple.yaml -- echo "hello world"

# Record with custom metadata
cli-replay record \
  --name "my-test" \
  --description "Test scenario" \
  --output test.yaml \
  -- kubectl get pods

# Record a multi-step workflow
cli-replay record \
  --output workflow.yaml \
  --name "deploy-flow" \
  -- bash -c "kubectl get pods && kubectl apply -f deploy.yaml && kubectl get pods"

# Record only specific commands from a script
cli-replay record \
  --output kubectl-only.yaml \
  --command kubectl \
  --command docker \
  -- bash deploy.sh
```

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success — recording completed and YAML written |
| 1 | Setup failure (invalid flags, output path not writable) |
| 2 | User command failed (YAML is still generated) |
| 3 | YAML generation or validation failed |

#### How Recording Works

1. **Direct capture mode** (no `--command` flags): The command runs directly; stdout, stderr, and exit code are captured
2. **Shim mode** (`--command` flags specified): Bash shim scripts are generated in a temporary directory and prepended to PATH, intercepting specified commands and logging executions to a JSONL file

### cli-replay run

Initialize or resume a replay session:

```bash
cli-replay run scenario.yaml
```

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--shell` | string | auto-detect | Output format: `powershell`, `bash`, `cmd` |
| `--allowed-commands` | string | `""` | Comma-separated list of commands allowed to be intercepted |
| `--max-delay` | string | `5m` | Maximum allowed delay duration (e.g., `5m`, `30s`) |
| `--dry-run` | bool | `false` | Preview the scenario step sequence without creating intercepts |

#### Security Allowlist

Restrict which commands can be intercepted before any PATH manipulation occurs:

```bash
# Via CLI flag
cli-replay run --allowed-commands kubectl,az scenario.yaml

# Via YAML (meta.security.allowed_commands)
# If both are set, the intersection is used
```

If a scenario step references a command not in the allowlist, `cli-replay run` exits with an error before creating any intercepts.

> 📖 See [SECURITY.md](SECURITY.md) for the full threat model, trust boundaries, and security recommendations.

### cli-replay verify

Check all steps were satisfied:

```bash
cli-replay verify scenario.yaml
```

Exit code 0 if all steps met their minimum call counts, 1 if steps remain unsatisfied.

#### Structured Output

Use `--format` to get machine-readable output for CI pipelines:

```bash
# JSON output (pipe to jq for pretty-printing)
cli-replay verify scenario.yaml --format json | jq .

# JUnit XML output (for CI dashboards)
cli-replay verify scenario.yaml --format junit > results.xml

# Default text output (human-readable)
cli-replay verify scenario.yaml --format text
```

When call count bounds are used, verify reports per-step invocation counts:

```
✓ Scenario "my-test" completed: 3/3 steps consumed
  Step 1: kubectl get pods — 4 calls (min: 1, max: 5) ✓
  Step 2: kubectl apply — 1 call (min: 1, max: 1) ✓
  Step 3: kubectl rollout status — 2 calls (min: 1, max: 3) ✓
```

Steps inside groups show the group name in the label:

```
  Step 2: [group:pre-flight] az account show — 1 call (min: 1, max: 2) ✓
```

### cli-replay exec

Run a child process with full intercept lifecycle management in a single command — setup, spawn, verify, and cleanup are handled automatically:

```bash
cli-replay exec scenario.yaml -- bash -c 'kubectl get pods && kubectl apply -f deploy.yaml'
```

This is the recommended mode for CI pipelines. No `eval`, no manual cleanup.

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--allowed-commands` | string | `""` | Comma-separated list of commands allowed to be intercepted |
| `--format` | string | `""` | Output format for verification: `json`, `junit`, or `text` |
| `--report-file` | string | `""` | Write structured verification output to a file path |
| `--dry-run` | bool | `false` | Preview the scenario without spawning a child process |

When `--report-file` is set, verification results are written to the specified file. When `--format` is set without `--report-file`, structured output goes to stderr (stdout is reserved for the child process).

```bash
# Write JUnit report for CI dashboard
cli-replay exec --format junit --report-file results.xml scenario.yaml -- make deploy

# JSON output to stderr
cli-replay exec --format json scenario.yaml -- bash test.sh
```

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Child process exited 0 **and** all scenario steps were satisfied |
| 1 | Verification failure — child exited 0 but scenario steps were not fully consumed |
| N | Child process exited with code N (propagated directly) |
| 126 | Child command found but not executable |
| 127 | Child command not found |
| 128+N | Child process killed by signal N (e.g., 143 = SIGTERM) |

#### How It Works

1. **Pre-spawn** — Loads the scenario, validates the security allowlist, and creates an isolated session ID
2. **Setup** — Creates the intercept directory with symlinks (or `.cmd` wrappers on Windows), initializes the state file, and builds a modified environment with `PATH`, `CLI_REPLAY_SESSION`, and `CLI_REPLAY_SCENARIO`
3. **Spawn** — Runs the child process with the modified environment. Signals (SIGINT, SIGTERM) are forwarded to the child
4. **Verify + Cleanup** — After the child exits, reloads state, checks all steps met their minimum call counts, prints diagnostics, and cleans up the intercept directory. Cleanup is idempotent and runs even if the child fails

#### Examples

```bash
# Basic usage
cli-replay exec scenario.yaml -- make deploy

# With security allowlist
cli-replay exec --allowed-commands kubectl,az scenario.yaml -- bash deploy.sh

# In CI (GitHub Actions)
- run: cli-replay exec test-scenario.yaml -- ./run-tests.sh

# Multiple args after --
cli-replay exec scenario.yaml -- bash -c 'echo hello && echo goodbye'
```

### cli-replay clean

Clean up an intercept session (remove wrappers and state):

```bash
cli-replay clean scenario.yaml
cli-replay clean              # uses CLI_REPLAY_SCENARIO from env
```

Clean is idempotent — it is safe to call even if the state file has already been removed.

#### TTL-Based Cleanup

Clean only sessions older than a given duration:

```bash
# Clean expired sessions for a single scenario
cli-replay clean --ttl 10m scenario.yaml

# Bulk cleanup: walk a directory tree for all .cli-replay/ dirs
cli-replay clean --ttl 1h --recursive .
cli-replay clean --ttl 30m --recursive /path/to/projects
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ttl` | string | `""` | Only clean sessions older than this Go duration (e.g., `10m`, `1h`) |
| `--recursive` | bool | `false` | Walk directory tree for `.cli-replay/` dirs (requires `--ttl`) |

**Safety guard**: `--recursive` requires `--ttl` to prevent accidental deletion of all sessions. Recursive walk skips `.git`, `node_modules`, `vendor`, and `.terraform` directories.

## Session Isolation

When running parallel CI jobs, each `cli-replay run` or `cli-replay exec` invocation generates a unique session ID (set via `CLI_REPLAY_SESSION`). State files are scoped to the session, so parallel test runs using the same scenario file do not interfere with each other.

```bash
# Two parallel CI jobs using the same scenario — no conflict
# Job A
eval "$(cli-replay run scenario.yaml)"   # gets session-A
kubectl get pods
cli-replay verify scenario.yaml          # checks session-A state only

# Job B (runs concurrently)
eval "$(cli-replay run scenario.yaml)"   # gets session-B
kubectl get pods
cli-replay verify scenario.yaml          # checks session-B state only
```

With `exec` mode, session isolation is automatic and requires no manual management.

## Trap Auto-Cleanup (eval pattern)

When using the `eval "$(cli-replay run ...)"` pattern with bash, zsh, or sh, cli-replay automatically emits a POSIX trap that cleans up the intercept session on shell exit or signals:

```bash
eval "$(cli-replay run scenario.yaml)"
# The emitted shell code includes:
#   _cli_replay_clean() { ... cli-replay clean "$CLI_REPLAY_SCENARIO" ...; }
#   trap '_cli_replay_clean' EXIT INT TERM
```

This ensures cleanup happens even if the script is interrupted (Ctrl+C) or terminated. The trap guard is idempotent — it only runs cleanup once regardless of how many signals fire.

> **Note**: Trap auto-cleanup is emitted for bash, zsh, and sh only. PowerShell and cmd sessions require manual cleanup via `cli-replay clean`.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CLI_REPLAY_SCENARIO` | Path to scenario file (required in intercept mode) |
| `CLI_REPLAY_SESSION` | Session ID for isolation (auto-set by `run`, or set manually) |
| `CLI_REPLAY_TRACE` | Set to "1" to enable trace output (includes denied env var logging) |
| `CLI_REPLAY_COLOR` | Force color output: `1` to enable, `0` to disable (overrides `NO_COLOR`) |
| `NO_COLOR` | Set to any value to disable colored output (see [no-color.org](https://no-color.org)) |

## Template Variables

Use Go text/template syntax in `respond.stdout` and `respond.stderr`:

```yaml
meta:
  vars:
    cluster: "prod"
    namespace: "default"

steps:
  - match:
      argv: ["kubectl", "config", "current-context"]
    respond:
      exit: 0
      stdout: "{{ .cluster }}"
```

Environment variables override `meta.vars`:

```bash
export cluster="staging"
# Now {{ .cluster }} renders as "staging"
```

### Denying Environment Variables

Prevent sensitive environment variables from leaking into template rendering using glob patterns in `meta.security.deny_env_vars`:

```yaml
meta:
  vars:
    cluster: "prod"              # safe default
  security:
    deny_env_vars:
      - "AWS_*"                   # block all AWS_ prefixed vars
      - "SECRET_*"                # block all SECRET_ prefixed vars
      - "TOKEN"                   # block exact name
      - "*"                       # block ALL env var overrides
```

**Behavior**:
- Denied env vars resolve to the `meta.vars` default (or empty string if no default)
- Patterns use `path.Match` glob syntax (`*` matches any sequence of non-separator characters)
- Internal variables (`CLI_REPLAY_*`) are always exempt from deny rules
- When `CLI_REPLAY_TRACE=1`, denied variables are logged: `cli-replay[trace]: denied env var SECRET_KEY`
- If no `deny_env_vars` is configured, all env vars pass through (backward compatible)

> 📖 See [SECURITY.md](SECURITY.md) for a complete list of security controls and known limitations.

## Dynamic Matching

Use wildcards and regex patterns in `match.argv` for flexible command matching:

```yaml
steps:
  # Match any value for a single argument
  - match:
      argv: ["az", "group", "list", "--subscription", "{{ .any }}"]
    respond:
      exit: 0
      stdout: "..."

  # Match an argument against a regex pattern
  - match:
      argv: ["kubectl", "get", "pods", "-n", '{{ .regex "^(prod|staging)-.*" }}']
    respond:
      exit: 0
      stdout: "..."
```

- `{{ .any }}` — matches any single argument value
- `{{ .regex "pattern" }}` — matches if the argument matches the given regex

## Dynamic Capture — Chaining Output Between Steps

Use `respond.capture` to store key-value pairs from a step's response, then reference them in later steps via `{{ .capture.<id> }}`:

```yaml
meta:
  name: capture-demo
  vars:
    region: eastus

steps:
  - match:
      argv: ["az", "group", "create", "--name", "demo-rg", "--location", "{{ .region }}"]
    respond:
      exit: 0
      stdout: '{"id": "/subscriptions/abc123/resourceGroups/demo-rg"}'
      capture:
        rg_id: "/subscriptions/abc123/resourceGroups/demo-rg"

  - match:
      argv: ["az", "vm", "create", "--resource-group", "demo-rg"]
    respond:
      exit: 0
      stdout: 'VM created in group {{ .capture.rg_id }}'
```

**Behavior**:
- Captures are accumulated across steps — each step can reference captures from any prior step
- Capture identifiers must match `[a-zA-Z_][a-zA-Z0-9_]*`
- A capture key cannot conflict with a `meta.vars` key (validated at load time)
- Forward references (referencing a capture before its defining step) are rejected at load time
- In unordered groups, sibling captures resolve to empty string (best-effort) if the defining step hasn't run yet
- Optional steps (`calls.min: 0`) that are never invoked do not add their captures

## Dry-Run Mode — Preview Without Side Effects

Use `--dry-run` on `run` or `exec` to preview a scenario's step sequence without creating intercepts, spawning child processes, or modifying state:

```bash
# Preview with run
cli-replay run --dry-run scenario.yaml

# Preview with exec
cli-replay exec --dry-run scenario.yaml -- make deploy
```

The dry-run output shows numbered steps with match patterns, exit codes, call bounds, group membership, captures, template variables, allowlist validation, and stdout previews. No files are created and no child processes are started.

## Call Count Bounds

By default, each step is consumed exactly once. Use `calls.min` and `calls.max` to support retry loops, polling, and optional steps:

```yaml
steps:
  # Polling step: can be called 1-10 times
  - match:
      argv: ["kubectl", "get", "pods", "-o", "json"]
    respond:
      exit: 0
      stdout: '{"items": []}'
    calls:
      min: 1
      max: 10

  # Optional step: can be skipped entirely
  - match:
      argv: ["kubectl", "delete", "pod", "{{ .any }}"]
    respond:
      exit: 0
      stdout: "pod deleted"
    calls:
      min: 0
      max: 1

  # Default behavior (no calls field): exactly once
  - match:
      argv: ["kubectl", "apply", "-f", "deploy.yaml"]
    respond:
      exit: 0
      stdout: "deployment created"
```

**Behavior**:
- When a step reaches its `max` count, cli-replay auto-advances to the next step
- When the current step doesn't match but its `min` is met, cli-replay soft-advances and tries the next step
- `verify` checks that all steps met their `min` count (not just that they were consumed)

## stdin Matching

Validate piped input content during replay. Useful for commands like `kubectl apply -f -` that read from stdin:

```yaml
steps:
  - match:
      argv: ["kubectl", "apply", "-f", "-"]
      stdin: |
        apiVersion: v1
        kind: Pod
        metadata:
          name: test-pod
    respond:
      exit: 0
      stdout: "pod/test-pod created"
```

**Behavior**:
- stdin is read up to 1 MB when `match.stdin` is set
- Trailing newlines are normalized (CRLF → LF)
- If `match.stdin` is not set, stdin content is ignored (backward compatible)
- During recording with `--command` flags, stdin is automatically captured when piped (non-TTY)

## JSON Schema for Scenario Files

cli-replay provides a JSON Schema for scenario YAML files, enabling IDE autocompletion, inline validation, and hover documentation.

### Per-File Modeline

Add this comment as the first line of any scenario YAML file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/cli-replay/cli-replay/main/schema/scenario.schema.json
meta:
  name: my-scenario
steps:
  - match:
      argv: [kubectl, get, pods]
    respond:
      exit: 0
```

### VS Code Workspace Settings

To apply the schema to all YAML files matching `*scenario*.yaml` or `*recording*.yaml`, add to `.vscode/settings.json`:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/cli-replay/cli-replay/main/schema/scenario.schema.json": [
      "**/scenarios/**/*.yaml",
      "**/recordings/**/*.yaml",
      "**/examples/**/*.yaml"
    ]
  }
}
```

Requires the [YAML extension](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml) (`redhat.vscode-yaml`).

## Mismatch Diagnostics

When a command doesn't match the expected step, cli-replay provides detailed per-element diff output:

```
Mismatch at step 2 of "deployment-test":

  Expected: ["kubectl", "get", "pods", "-n", "{{ .regex \"^prod-.*\" }}"]
  Received: ["kubectl", "get", "pods", "-n", "staging-app"]
                                              ^^^^^^^^^^^
  First difference at position 4:
    expected pattern: ^prod-.*
    received value:   staging-app
```

Color output is auto-detected from the terminal, and can be controlled via `CLI_REPLAY_COLOR` or `NO_COLOR` environment variables.

## Session TTL (Auto-Cleanup)

Configure automatic cleanup of stale replay sessions via `meta.session.ttl`:

```yaml
meta:
  name: my-scenario
  session:
    ttl: "10m"    # Clean sessions older than 10 minutes
```

**Behavior**:
- On `cli-replay run`, `exec`, or intercept startup, stale sessions (based on `last_updated` timestamp) are automatically removed
- Only the `.cli-replay/` directory next to the scenario file is scanned
- State files with `last_updated` in the future are treated as active (with a warning)
- Permission errors on individual files are logged and skipped
- Cleanup summary is emitted to stderr: `cli-replay: cleaned N expired sessions`
- If no `session.ttl` is configured, no automatic cleanup occurs (backward compatible)

For bulk cleanup across many projects, use [`cli-replay clean --ttl --recursive`](#ttl-based-cleanup).

## How It Works

1. **Symlink Interception**: Create symlinks to cli-replay named after commands you want to fake (e.g., `kubectl`, `az`)
2. **PATH Manipulation**: Prepend the symlink directory to PATH
3. **Command Detection**: When invoked via symlink, cli-replay reads `CLI_REPLAY_SCENARIO`
4. **Step Matching**: Compares incoming argv against the next expected step
5. **Response Replay**: Writes stdout/stderr and returns exit code
6. **State Persistence**: Tracks progress in `.cli-replay/` next to the scenario file (state files, intercept directories)

## Limitations

- **Strict ordering** — commands must match in exact sequence; no support for unordered or concurrent steps
- **No parallel execution** — state is per-scenario, not per-process. Session isolation (via `CLI_REPLAY_SESSION`) supports parallel *test runs*, but not parallel *steps within* a scenario
- **Fixed responses only** — no conditional or dynamic response logic based on runtime state
- **stdin size limit** — piped input is capped at 1 MB during both replay and recording

## Development

**Unix (macOS/Linux):**
```bash
# Build
make build

# Run tests
make test

# Lint
make lint

# Format
make fmt
```

**Windows (PowerShell):**
```powershell
# Build
go build -o cli-replay.exe .

# Run tests
go test ./...

# Lint (if golangci-lint installed)
golangci-lint run

# Or use the build script
.\build.ps1
.\build.ps1 -Test
.\build.ps1 -Lint
```

## Platform Support

| Feature | Unix (macOS/Linux) | Windows 10+ |
|---------|-------------------|-------------|
| Record commands | ✅ Bash shims | ✅ .cmd + .ps1 shims |
| Replay scenarios | ✅ Symlinks | ✅ .cmd wrappers |
| State persistence | ✅ `.cli-replay/` | ✅ `.cli-replay/` |
| Build | ✅ `make build` | ✅ `go build` / `build.ps1` |
| CI | ✅ GitHub Actions | ✅ GitHub Actions |

## Troubleshooting

### Windows: ExecutionPolicy Error

If PowerShell blocks script execution during recording:

```powershell
# Check current policy
Get-ExecutionPolicy -List

# Set policy for current user (one-time)
Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
```

cli-replay shims use `-ExecutionPolicy Bypass` per-process, but Group Policy may override this.

### Windows: Command Not Found During Recording

Ensure the target command is on PATH:

```powershell
Get-Command kubectl  # Should show the path to kubectl.exe
```

### Windows: Shim Not Intercepting

Ensure the intercept directory is **first** on PATH:

```powershell
$env:PATH = "$interceptDir;$env:PATH"  # Must be prepended, not appended
```

Also verify PATHEXT includes `.CMD` (it does by default on Windows 10+):

```powershell
$env:PATHEXT  # Should contain .CMD
```

### Windows: Signal Handling (Ctrl+C / Process Termination)

On Unix, cli-replay forwards `SIGINT` and `SIGTERM` to child processes during `exec` mode. On Windows, `SIGTERM` is not supported — cli-replay uses `Process.Kill()` instead when Ctrl+C is pressed.

**Known limitations on Windows:**
- `Process.Kill()` does not propagate to grandchild processes. If your child spawns sub-processes, they may be orphaned after Ctrl+C
- There is no graceful shutdown option — the child is killed immediately
- If cleanup fails due to orphaned grandchildren, run `cli-replay clean` manually to remove stale state and intercept directories

```powershell
# Manual cleanup after unexpected termination on Windows
cli-replay clean scenario.yaml
```

## License

MIT

## Security

See [SECURITY.md](SECURITY.md) for the security policy, vulnerability reporting process, threat model, and recommendations.
