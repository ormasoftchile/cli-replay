# Release Notes: cli-replay v0.2.0

**Date**: February 2026  
**Feature**: 002-workflow-recorder  
**Branch**: `002-workflow-recorder`

## What's New

### `record` Subcommand

Record real CLI command executions and automatically generate replay scenario YAML files. Instead of manually crafting YAML with command arguments and outputs, simply run your workflow once and let cli-replay capture it.

```bash
cli-replay record --output demo.yaml -- kubectl get pods
```

### Key Features

- **Single command recording**: Capture any CLI command's argv, stdout, stderr, and exit code
- **Multi-step workflow recording**: Record bash scripts with multiple commands
- **Custom metadata**: Set scenario name and description via flags
- **Selective command interception**: Use `--command` flags to record only specific commands from a complex script
- **Automatic YAML generation**: Produces valid scenario YAML files compatible with `cli-replay run`
- **Non-zero exit code capture**: Error scenarios are recorded faithfully

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--output` | `-o` | Output YAML file path (required) |
| `--name` | `-n` | Scenario name (auto-generated if omitted) |
| `--description` | `-d` | Scenario description |
| `--command` | `-c` | Commands to intercept (repeatable) |

### Examples

```bash
# Basic recording
cli-replay record -o simple.yaml -- echo "hello"

# With metadata
cli-replay record -o test.yaml --name "my-test" --description "Demo" -- kubectl get pods

# Multi-step workflow
cli-replay record -o workflow.yaml -- bash -c "cmd1 && cmd2 && cmd3"

# Selective recording (only intercept kubectl)
cli-replay record -o filtered.yaml --command kubectl -- bash deploy.sh
```

## Technical Details

### Architecture

- **Direct capture mode**: For recording without `--command` filters, stdout/stderr are captured via `io.MultiWriter`
- **Shim mode**: When `--command` flags are specified, bash shim scripts are generated in a temp directory and prepended to PATH for transparent command interception
- **JSONL logging**: Command executions are logged incrementally to a JSONL file for crash resilience
- **Type-safe conversion**: JSONL entries are parsed into `RecordedCommand` structs, then converted to `scenario.Scenario` types before YAML marshaling

### New Packages

- `internal/recorder` — Core recording functionality (session lifecycle, shim generation, JSONL parsing, YAML conversion)
- `cmd` — Cobra CLI command tree with `record` subcommand

### Test Coverage

- 37 unit tests in `internal/recorder/` (command validation, JSONL parsing, shim generation, session lifecycle, converter)
- 19 integration tests in `cmd/` (end-to-end recording, metadata, error handling, shim-based recording, YAML roundtrip)
- All tests pass on macOS (arm64) and Linux

### Dependencies

No new external dependencies. Uses existing:
- `cobra` v1.8.0 (CLI framework)
- `gopkg.in/yaml.v3` v3.0.1 (YAML generation)
- `testify` v1.8.4 (test assertions)

## Migration Notes

- No breaking changes to existing `cli-replay run`, `verify`, or `init` commands
- The `record` subcommand is available via the new root binary (`go build -o bin/cli-replay .`)
- The original `cmd/cli-replay` binary continues to provide `run`/`verify`/`init` with symlink intercept mode

## Known Limitations

- Recording executes real commands — it modifies your actual system
- Interactive commands (prompts for user input) are not supported
- macOS and Linux only (Windows not supported in this release)
- Strict ordering: multi-step recordings preserve execution order
- Environment variable capture is out of scope
