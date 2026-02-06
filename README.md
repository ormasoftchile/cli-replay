# cli-replay

A scenario-driven CLI replay tool for testing systems that orchestrate external CLI tools.

## Prerequisites

- **Go 1.21+** - [Install Go](https://go.dev/doc/install)
  ```bash
  # macOS (Homebrew)
  brew install go
  
  # Verify installation
  go version
  ```

- **golangci-lint** (optional, for development)
  ```bash
  brew install golangci-lint
  ```

## Problem Statement

When testing systems that execute external CLI commands (like `kubectl`, `az`, `aws`, `docker`), you need predictable, reproducible responses. Real CLI tools make network calls, require credentials, and produce non-deterministic output.

**cli-replay** solves this by:
- Intercepting CLI calls via PATH manipulation
- Matching commands against predefined scenarios
- Returning predetermined stdout, stderr, and exit codes
- Tracking step order across multiple invocations

## Installation

```bash
# From source
go install github.com/cli-replay/cli-replay/cmd/cli-replay@latest

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

```bash
# Create symlink to cli-replay
ln -s $(which cli-replay) /tmp/kubectl

# Prepend to PATH
export PATH=/tmp:$PATH

# Point to scenario
export CLI_REPLAY_SCENARIO=$(pwd)/scenario.yaml

# Initialize session
cli-replay run scenario.yaml
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

steps:
  - match:
      argv: ["kubectl", "get", "pods", "-n", "{{ .namespace }}"]
    respond:
      exit: 0                      # Required: exit code (0-255)
      stdout: "inline output"      # Optional: literal stdout
      stderr: "error message"      # Optional: literal stderr
      stdout_file: "fixtures/out.txt"  # Optional: file-based stdout
      stderr_file: "fixtures/err.txt"  # Optional: file-based stderr
```

### Validation Rules

- `meta.name` is required and must be non-empty
- `steps` must contain at least one step
- `match.argv` must be non-empty
- `exit` must be 0-255
- `stdout` and `stdout_file` are mutually exclusive
- `stderr` and `stderr_file` are mutually exclusive
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

### cli-replay verify

Check all steps were consumed:

```bash
cli-replay verify scenario.yaml
```

Exit code 0 if complete, 1 if steps remain.

### cli-replay init

Reset scenario state (start over):

```bash
cli-replay init scenario.yaml
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CLI_REPLAY_SCENARIO` | Path to scenario file (required in intercept mode) |
| `CLI_REPLAY_TRACE` | Set to "1" to enable trace output |

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

## How It Works

1. **Symlink Interception**: Create symlinks to cli-replay named after commands you want to fake (e.g., `kubectl`, `az`)
2. **PATH Manipulation**: Prepend the symlink directory to PATH
3. **Command Detection**: When invoked via symlink, cli-replay reads `CLI_REPLAY_SCENARIO`
4. **Step Matching**: Compares incoming argv against the next expected step
5. **Response Replay**: Writes stdout/stderr and returns exit code
6. **State Persistence**: Tracks progress in `/tmp/cli-replay-<hash>.state`

## Limitations

- **Strict ordering**: Commands must match in exact sequence
- **No regex matching**: argv comparison is literal (no wildcards)
- **No parallel execution**: State is per-scenario, not per-process
- **Template in responses only**: `match.argv` values are literal

## Development

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

## License

MIT
