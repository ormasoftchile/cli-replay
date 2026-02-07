# cli-replay

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
- Unknown fields are rejected (strict YAML parsing)

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

#### Security Allowlist

Restrict which commands can be intercepted before any PATH manipulation occurs:

```bash
# Via CLI flag
cli-replay run --allowed-commands kubectl,az scenario.yaml

# Via YAML (meta.security.allowed_commands)
# If both are set, the intersection is used
```

If a scenario step references a command not in the allowlist, `cli-replay run` exits with an error before creating any intercepts.

### cli-replay verify

Check all steps were satisfied:

```bash
cli-replay verify scenario.yaml
```

Exit code 0 if all steps met their minimum call counts, 1 if steps remain unsatisfied.

When call count bounds are used, verify reports per-step invocation counts:

```
✓ Scenario "my-test" completed: 3/3 steps consumed
  Step 1: kubectl get pods — 4 calls (min: 1, max: 5) ✓
  Step 2: kubectl apply — 1 call (min: 1, max: 1) ✓
  Step 3: kubectl rollout status — 2 calls (min: 1, max: 3) ✓
```

### cli-replay clean

Clean up an intercept session (remove wrappers and state):

```bash
cli-replay clean scenario.yaml
cli-replay clean              # uses CLI_REPLAY_SCENARIO from env
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CLI_REPLAY_SCENARIO` | Path to scenario file (required in intercept mode) |
| `CLI_REPLAY_SESSION` | Session ID for isolation (auto-set by `run`, or set manually) |
| `CLI_REPLAY_TRACE` | Set to "1" to enable trace output |
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

## How It Works

1. **Symlink Interception**: Create symlinks to cli-replay named after commands you want to fake (e.g., `kubectl`, `az`)
2. **PATH Manipulation**: Prepend the symlink directory to PATH
3. **Command Detection**: When invoked via symlink, cli-replay reads `CLI_REPLAY_SCENARIO`
4. **Step Matching**: Compares incoming argv against the next expected step
5. **Response Replay**: Writes stdout/stderr and returns exit code
6. **State Persistence**: Tracks progress in the system temp directory (`os.TempDir()` → `/tmp` on Unix, `%TEMP%` on Windows)

## Limitations

- **Strict ordering** — commands must match in exact sequence; no support for unordered or concurrent steps
- **No parallel execution** — state is per-scenario, not per-process (session isolation helps for parallel *test runs*, but not parallel *steps within* a scenario)
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
| State persistence | ✅ `/tmp/` | ✅ `%TEMP%` |
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

## License

MIT
