# Data Model: Core Scenario Replay

**Feature**: 001-core-scenario-replay  
**Date**: 2026-02-05  
**Source**: spec.md Key Entities + research.md decisions

## Entities

### Scenario

A complete test definition loaded from a YAML file.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| meta | Meta | Yes | Scenario metadata |
| steps | []Step | Yes | Ordered list of command-response pairs |

**Validation Rules**:
- `steps` must contain at least one step
- Unknown fields rejected (strict YAML parsing)

**State Transitions**: N/A (immutable after load)

---

### Meta

Scenario metadata including identification and template variables.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Human-readable scenario identifier |
| description | string | No | Optional description of scenario purpose |
| vars | map[string]string | No | Template variables for response rendering |

**Validation Rules**:
- `name` must be non-empty
- `vars` keys must be valid Go template identifiers

---

### Step

A single command-response pair within a scenario.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| match | Match | Yes | Criteria for matching incoming commands |
| respond | Response | Yes | Output to return when matched |

**Validation Rules**:
- Both `match` and `respond` are required

---

### Match

Criteria for identifying an incoming CLI command.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| argv | []string | Yes | Exact argument vector to match |

**Validation Rules**:
- `argv` must be non-empty (at least the command name)
- Matching is exact: same length, same order, string equality

**Matching Algorithm**:
```
MATCH(expected, received):
  IF len(expected.argv) != len(received.argv):
    RETURN false
  FOR i := 0; i < len(expected.argv); i++:
    IF expected.argv[i] != received.argv[i]:
      RETURN false
  RETURN true
```

---

### Response

Output definition for a matched command.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| exit | int | Yes | Exit code to return (0-255) |
| stdout | string | No | Literal stdout content |
| stderr | string | No | Literal stderr content |
| stdout_file | string | No | Path to file containing stdout (relative to scenario) |
| stderr_file | string | No | Path to file containing stderr (relative to scenario) |

**Validation Rules**:
- `exit` must be in range 0-255
- `stdout` and `stdout_file` are mutually exclusive
- `stderr` and `stderr_file` are mutually exclusive
- File paths validated at scenario load time (fail early)

**Template Rendering**:
- `stdout`, `stderr` (and file contents) are rendered through text/template
- Variables sourced from: Meta.vars (base) + environment (override)
- Missing variable → error

---

### State

Persistent state tracking scenario progress across CLI invocations.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| scenario_path | string | Yes | Absolute path to scenario file |
| scenario_hash | string | Yes | SHA256 hash of scenario content (for staleness detection) |
| current_step | int | Yes | Index of next step to match (0-based) |
| total_steps | int | Yes | Total number of steps in scenario |
| last_updated | time.Time | Yes | Timestamp of last state update |

**Persistence**:
- Location: `{os.TempDir()}/cli-replay-{hash(scenario_path)[:16]}.state`
- Format: JSON
- Write: Atomic (write temp file, rename)

**State Transitions**:
```
[No State] --init/first-run--> [current_step=0]
[current_step=N] --match--> [current_step=N+1]
[current_step=total_steps] --verify--> [COMPLETE]
```

---

## Entity Relationships

```
Scenario
├── Meta
│   └── vars: map[string]string
└── Steps[]
    ├── Match
    │   └── argv: []string
    └── Response
        ├── exit: int
        ├── stdout/stdout_file: string
        └── stderr/stderr_file: string

State (separate file)
└── tracks: Scenario progress
```

## YAML Schema Example

```yaml
meta:
  name: "incident-remediation"
  description: "Simulates cluster recovery flow"
  vars:
    cluster: "prod-eus2"
    namespace: "sql"

steps:
  - match:
      argv: ["kubectl", "get", "pods", "-n", "sql"]
    respond:
      exit: 0
      stdout_file: fixtures/pods_unhealthy.txt

  - match:
      argv: ["kubectl", "rollout", "restart", "deployment/sql-agent"]
    respond:
      exit: 0
      stdout: "deployment.apps/sql-agent restarted"

  - match:
      argv: ["kubectl", "get", "pods", "-n", "sql"]
    respond:
      exit: 0
      stdout: "NAME    READY   STATUS\nsql-0   1/1     Running"
```

> **Note**: Template variables (e.g., `{{ .namespace }}`) are supported in `respond.stdout` and `respond.stderr` only. Match patterns (`match.argv`) are literal strings for deterministic matching.
