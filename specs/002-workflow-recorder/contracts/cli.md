# CLI Contract: record subcommand

**Feature**: 002-workflow-recorder  
**Created**: February 6, 2026  
**Purpose**: Define the command-line interface for the record subcommand

## Command Syntax

```bash
cli-replay record [flags] -- <command> [args...]
```

## Flags

### Required Flags

| Flag | Type | Description | Example |
|------|------|-------------|---------|
| `--output`, `-o` | string | Output file path for generated YAML scenario | `--output demo.yaml` |

### Optional Flags

| Flag | Type | Description | Default | Example |
|------|------|-------------|---------|---------|
| `--name`, `-n` | string | Scenario name in meta section | `recorded-session-{timestamp}` | `--name "pod-restart"` |
| `--description`, `-d` | string | Scenario description in meta section | `""` | `--description "Restart workflow"` |
| `--command`, `-c` | []string | Command names to record (can be specified multiple times) | `[]` (record all) | `--command kubectl --command docker` |

### Inherited Flags

| Flag | Type | Description | Default |
|------|------|-------------|---------|
| `--help`, `-h` | bool | Display help for record command | - |

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `-- <command> [args...]` | Yes | Command to execute while recording |

**Notes**:
- The `--` separator is mandatory to distinguish flags from the user command
- All arguments after `--` are passed to the user command verbatim
- User command can be a single command (`-- kubectl get pods`) or shell script (`-- bash script.sh`)

## Usage Examples

### Example 1: Basic Recording

```bash
cli-replay record --output simple.yaml -- kubectl get pods
```

**Effect**:
- Execute `kubectl get pods`
- Capture argv, exit code, stdout, stderr
- Generate `simple.yaml` with auto-generated name

**Generated YAML**:
```yaml
meta:
  name: "recorded-session-20260206-143022"
  description: ""

steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS    RESTARTS   AGE
        web-0   1/1     Running   0          5m
```

---

### Example 2: Multi-Step Workflow

```bash
cli-replay record \
  --output workflow.yaml \
  --name "pod-restart-demo" \
  --description "Demonstrates pod restart workflow" \
  -- bash -c 'kubectl get pods && kubectl delete pod web-0 && kubectl get pods'
```

**Effect**:
- Execute bash script with three kubectl commands
- Capture all three executions
- Generate `workflow.yaml` with custom metadata

**Generated YAML**:
```yaml
meta:
  name: "pod-restart-demo"
  description: "Demonstrates pod restart workflow"

steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS             RESTARTS   AGE
        web-0   0/1     CrashLoopBackOff   5          10m

  - match:
      argv: ["kubectl", "delete", "pod", "web-0"]
    respond:
      exit: 0
      stdout: "pod \"web-0\" deleted\n"

  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS    RESTARTS   AGE
        web-0   1/1     Running   0          30s
```

---

### Example 3: Selective Command Recording

```bash
cli-replay record \
  --output filtered.yaml \
  --command kubectl \
  --command docker \
  -- bash deploy.sh
```

**Effect**:
- Execute `bash deploy.sh`
- Only record `kubectl` and `docker` commands
- Ignore other commands (e.g., `echo`, `ls`, `grep`)
- Generate `filtered.yaml` with only kubectl/docker steps

**Generated YAML** (only shows intercepted commands):
```yaml
meta:
  name: "recorded-session-20260206-143522"
  description: ""

steps:
  - match:
      argv: ["docker", "build", "-t", "app:latest", "."]
    respond:
      exit: 0
      stdout: "Successfully built abc123\n"

  - match:
      argv: ["kubectl", "apply", "-f", "deployment.yaml"]
    respond:
      exit: 0
      stdout: "deployment.apps/app created\n"
```

---

### Example 4: Recording Failed Commands

```bash
cli-replay record --output errors.yaml -- kubectl get pods --invalid-flag
```

**Effect**:
- Execute command that fails with non-zero exit code
- Capture error output in stderr field
- Generate YAML with exit code and stderr

**Generated YAML**:
```yaml
meta:
  name: "recorded-session-20260206-144022"
  description: ""

steps:
  - match:
      argv: ["kubectl", "get", "pods", "--invalid-flag"]
    respond:
      exit: 1
      stdout: ""
      stderr: |
        Error: unknown flag: --invalid-flag
        See 'kubectl get --help' for usage.
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Recording successful, YAML generated |
| `1` | Recording setup failed (e.g., output path not writable, command not found) |
| `2` | User command execution failed (propagated from user command exit code) |
| `3` | YAML generation/validation failed |

**Note**: The recorder propagates the user command's exit code. If the user command fails with exit code 5, the recorder exits with code 2 but still generates YAML capturing that failure.

## Error Messages

### Invalid Flags

```bash
$ cli-replay record --output demo.yaml
Error: user command is required (use '-- <command>' after flags)

$ cli-replay record -- kubectl get pods
Error: required flag --output not specified
```

### Setup Failures

```bash
$ cli-replay record --output /readonly/path.yaml -- kubectl get pods
Error: output path not writable: /readonly/path.yaml

$ cli-replay record --output demo.yaml --command nonexistent -- bash script.sh
Error: command 'nonexistent' not found in PATH
```

### Execution Failures

```bash
$ cli-replay record --output demo.yaml -- nonexistent-command
Error: failed to execute user command: exec: "nonexistent-command": executable file not found in $PATH
```

### Generation Failures

```bash
$ cli-replay record --output demo.yaml -- kubectl get pods
[command executes]
Error: failed to generate YAML: invalid scenario: step 0: match: argv must be non-empty
```

## Help Output

```bash
$ cli-replay record --help

Record command executions and generate replay scenario YAML files.

The record subcommand intercepts CLI command executions, captures their
arguments, exit codes, and output, then generates a scenario YAML file
that can be replayed using 'cli-replay run'.

Usage:
  cli-replay record [flags] -- <command> [args...]

Examples:
  # Record a single command
  cli-replay record --output demo.yaml -- kubectl get pods

  # Record a multi-step workflow
  cli-replay record --output workflow.yaml -- bash script.sh

  # Record only specific commands
  cli-replay record -o filtered.yaml -c kubectl -c docker -- bash deploy.sh

  # Record with custom metadata
  cli-replay record \
    --output demo.yaml \
    --name "pod-restart" \
    --description "Pod restart workflow" \
    -- bash -c 'kubectl get pods && kubectl delete pod web-0'

Flags:
  -o, --output string        Output file path for generated YAML (required)
  -n, --name string          Scenario name in meta section
  -d, --description string   Scenario description in meta section
  -c, --command strings      Command names to record (can be repeated)
  -h, --help                 Help for record

Global Flags:
      --config string   Config file (default is $HOME/.cli-replay.yaml)
```

## Behavioral Specifications

### Command Interception

1. **Shim generation**: Recorder creates temporary directory with shim executables
2. **PATH modification**: Prepends shim directory to PATH for user command
3. **Execution**: User command runs in subprocess with modified PATH
4. **Cleanup**: Shim directory removed after user command completes

### Recording Process

1. **Before execution**: Create JSONL log file, generate shims
2. **During execution**: Each intercepted command appends to JSONL log
3. **After execution**: Parse JSONL, convert to scenario, validate, write YAML

### Filtering Logic

- **No `--command` flags**: Record all commands executed
- **One or more `--command` flags**: Only generate shims for specified commands
- Commands not in filter list execute normally without interception

### Metadata Defaults

- **Name**: If not provided, use `recorded-session-{ISO8601-timestamp}`
- **Description**: If not provided, use empty string
- **RecordedAt**: Always set to session start time (not included in output YAML)

## Compatibility

### Existing cli-replay Commands

The `record` subcommand does not conflict with existing commands:
- `cli-replay run <scenario>` - Replay existing scenario
- `cli-replay verify <scenario>` - Verify scenario step order

### Future Commands

Reserved namespace for recorder-related commands:
- `cli-replay record` - Current feature
- `cli-replay record list` - Future: List recorded sessions (if persistence added)
- `cli-replay record edit` - Future: Interactive YAML editing (if needed)

## Testing Requirements

### Unit Tests

- Flag parsing (required/optional flags, multiple `--command` values)
- Error messages (invalid flags, missing output, command not found)
- Help output formatting

### Integration Tests

- End-to-end recording (execute simple command, verify YAML)
- Multi-step recording (bash script with multiple commands)
- Filtered recording (only specified commands captured)
- Failed command recording (non-zero exit code, stderr captured)
- YAML validation (generated scenario passes `scenario.Validate()`)

### Edge Cases

- Commands with arguments containing spaces (`kubectl get pods --field-selector="status.phase=Running"`)
- Commands with quotes in arguments (`echo "hello world"`)
- Commands that produce no output (empty stdout/stderr)
- Commands that fail immediately (exit before producing output)
