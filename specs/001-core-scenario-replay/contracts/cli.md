# CLI Contract: cli-replay

**Feature**: 001-core-scenario-replay  
**Date**: 2026-02-05  
**Type**: Command-Line Interface

## Overview

cli-replay operates in two modes:
1. **Management mode**: Invoked as `cli-replay` with subcommands
2. **Intercept mode**: Invoked via symlink (e.g., `kubectl`) to replay scenarios

## Commands

### cli-replay run

Start or resume a replay session for a scenario.

```
cli-replay run <scenario.yaml>
```

**Arguments**:
| Argument | Required | Description |
|----------|----------|-------------|
| scenario.yaml | Yes | Path to YAML scenario file |

**Environment Variables**:
| Variable | Description |
|----------|-------------|
| CLI_REPLAY_SCENARIO | Alternative to CLI argument |
| CLI_REPLAY_TRACE | Set to "1" to enable trace output |

**Exit Codes**:
| Code | Meaning |
|------|---------|
| 0 | Session initialized successfully |
| 1 | Error (file not found, invalid YAML, etc.) |

**Behavior**:
1. Load and validate scenario file
2. Initialize state file if not exists
3. Print confirmation message to stderr

---

### cli-replay verify

Verify all scenario steps have been consumed.

```
cli-replay verify <scenario.yaml>
```

**Arguments**:
| Argument | Required | Description |
|----------|----------|-------------|
| scenario.yaml | Yes | Path to YAML scenario file |

**Exit Codes**:
| Code | Meaning |
|------|---------|
| 0 | All steps consumed |
| 1 | Unused steps remain or state not found |

**Output (stderr)**:
```
# Success
cli-replay: scenario "incident-remediation" complete (3/3 steps)

# Failure
cli-replay: scenario "incident-remediation" incomplete
  consumed: 2/3 steps
  remaining:
    step 3: ["kubectl", "get", "pods", "-n", "sql"]
```

---

### cli-replay init

Reset state file for a scenario (clear progress).

```
cli-replay init <scenario.yaml>
```

**Arguments**:
| Argument | Required | Description |
|----------|----------|-------------|
| scenario.yaml | Yes | Path to YAML scenario file |

**Exit Codes**:
| Code | Meaning |
|------|---------|
| 0 | State reset successfully |
| 1 | Error (file not found, invalid YAML) |

---

### cli-replay --version

Print version information.

```
cli-replay --version
```

**Output (stdout)**:
```
cli-replay version 0.1.0
```

---

### cli-replay --help

Print usage information.

```
cli-replay --help
```

**Output (stdout)**:
```
cli-replay - Scenario-driven CLI replay for testing

Usage:
  cli-replay [command]

Available Commands:
  run      Start or resume a replay session
  verify   Verify all scenario steps consumed
  init     Reset scenario state
  help     Help about any command

Flags:
  -h, --help      help for cli-replay
  -v, --version   version for cli-replay

Use "cli-replay [command] --help" for more information about a command.
```

---

## Intercept Mode

When cli-replay is symlinked as another command (e.g., `kubectl`), it operates in intercept mode.

### Detection

Intercept mode activates when `filepath.Base(os.Args[0])` is NOT `cli-replay`.

### Behavior

1. Determine scenario path from `CLI_REPLAY_SCENARIO` environment variable
2. Load scenario and state
3. Compare `os.Args` against expected step's `match.argv`
4. If match:
   - Render response through templates
   - Write stdout to stdout, stderr to stderr
   - Exit with response's exit code
   - Advance state to next step
5. If no match:
   - Write error to stderr
   - Exit with code 1

### Example

```bash
# Setup
ln -s /usr/local/bin/cli-replay /tmp/bin/kubectl
export PATH=/tmp/bin:$PATH
export CLI_REPLAY_SCENARIO=/path/to/scenario.yaml

# Initialize
cli-replay run /path/to/scenario.yaml

# Intercept (transparent to calling code)
kubectl get pods -n sql
# → Returns scripted stdout, exit 0

# Verify
cli-replay verify /path/to/scenario.yaml
```

---

## Error Messages

All errors written to stderr with structured format:

### File Not Found
```
cli-replay: scenario file not found: /path/to/scenario.yaml
```

### Invalid YAML
```
cli-replay: invalid scenario YAML at line 15: unknown field "matchh"
```

### Mismatch
```
cli-replay: mismatch at step 2 of "incident-remediation"
  expected: ["kubectl", "rollout", "restart", "deployment/sql-agent"]
  received: ["kubectl", "get", "pods", "-n", "sql"]
  scenario: /path/to/scenario.yaml
```

### Missing Variable
```
cli-replay: template error in step 1 stdout: variable "clustr" not defined
```

### No Scenario
```
cli-replay: no scenario specified
  use: cli-replay run <scenario.yaml>
  or:  export CLI_REPLAY_SCENARIO=/path/to/scenario.yaml
```

---

## Trace Mode

When `CLI_REPLAY_TRACE=1`:

```
cli-replay: [TRACE] step 1/3 matched
  argv: ["kubectl", "get", "pods", "-n", "sql"]
  stdout: "NAME    READY   STATUS\nsql-0   0/1     CrashLoopBackOff"
  stderr: ""
  exit: 0
```

Trace output goes to stderr to avoid polluting stdout.
