# YAML Schema Contract: P0 Critical Enhancements

**Feature**: `005-p0-critical-enhancements`

---

## Schema Changes

All changes are additive. Existing scenario files remain valid without modification.

### Step: `calls` field (new)

```yaml
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "NAME   READY   STATUS\npod-1  1/1     Running"
    calls:          # NEW — optional
      min: 1        # minimum required invocations (default: 1)
      max: 5        # maximum allowed invocations (default: 1)
```

**Field Specification**:

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `calls` | object | No | `{min: 1, max: 1}` | — |
| `calls.min` | integer | No | `1` (if `calls` absent); `0` (if only `max` given) | `>= 0` |
| `calls.max` | integer | No | `1` (if `calls` absent); equals `min` (if only `min` given) | `>= 1`, `>= min` |

**Defaulting Logic** (applied after YAML parse):
```
If calls is nil:
  effective = {min: 1, max: 1}        # backward compatible
Else:
  If max == 0 and min > 0:
    max = min                          # "calls: {min: 3}" means exactly 3
  If max == 0 and min == 0:
    REJECT: "calls.max must be >= 1"
  If min > max:
    REJECT: "calls.min (N) must be <= calls.max (M)"
```

**Examples**:
```yaml
# Exactly 1 call (current default, backward compatible)
calls:
  min: 1
  max: 1

# Polling step: 1-5 calls
calls:
  min: 1
  max: 5

# Optional step (0 or 1 calls)
calls:
  min: 0
  max: 1

# Shorthand: exactly 3 calls
calls:
  min: 3

# Shorthand: up to 5 calls (0 minimum)
calls:
  max: 5
```

---

### Match: `stdin` field (new)

```yaml
steps:
  - match:
      argv: ["kubectl", "apply", "-f", "-"]
      stdin: |                   # NEW — optional
        apiVersion: v1
        kind: Pod
        metadata:
          name: test-pod
    respond:
      exit: 0
      stdout: "pod/test-pod created"
```

**Field Specification**:

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `match.stdin` | string | No | `""` (ignored) | Max 1 MB. Text only. |

**Matching Behavior**:
- When absent or empty: stdin is ignored (backward compatible)
- When set: actual stdin is compared after `strings.TrimRight(s, "\n")` normalization on both sides
- Comparison is exact (no template patterns in stdin for v1)

---

### Meta: `security` section (new)

```yaml
meta:
  name: deployment-test
  description: Tests a Kubernetes deployment workflow
  security:                      # NEW — optional
    allowed_commands:
      - kubectl
      - az
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
```

**Field Specification**:

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `meta.security` | object | No | `nil` (no restrictions) | — |
| `meta.security.allowed_commands` | `[]string` | No | `[]` (no restrictions) | Non-empty strings, base command names only |

**Validation Behavior**:
- When absent: all commands allowed (backward compatible)
- When set: `run` command validates each `step[*].match.argv[0]` base name against the list
- Base name extraction: `filepath.Base(argv[0])` — handles both `kubectl` and `/usr/bin/kubectl`
- Matching: case-sensitive on Unix, case-insensitive on Windows (via `runtime.GOOS`)
- Empty `allowed_commands` list: treated as no restriction (same as absent)

---

## Full Example

```yaml
meta:
  name: k8s-deploy-with-polling
  description: Deployment with status polling and stdin pipe
  security:
    allowed_commands:
      - kubectl

steps:
  # Step 1: Apply manifest via stdin pipe (exactly once)
  - match:
      argv: ["kubectl", "apply", "-f", "-"]
      stdin: |
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: myapp
    respond:
      exit: 0
      stdout: "deployment.apps/myapp created"

  # Step 2: Poll rollout status (1-10 times)
  - match:
      argv: ["kubectl", "rollout", "status", "deployment/myapp"]
    respond:
      exit: 0
      stdout: "Waiting for deployment rollout..."
    calls:
      min: 1
      max: 10

  # Step 3: Verify pods (exactly once)
  - match:
      argv: ["kubectl", "get", "pods", "-l", "app=myapp"]
    respond:
      exit: 0
      stdout: "NAME        READY   STATUS    RESTARTS   AGE\nmyapp-abc   1/1     Running   0          30s"
```

---

## State File Changes

**Format**: JSON in `$TMPDIR/cli-replay-state-<hash>.json`

### Before (current)

```json
{
  "scenario_path": "/path/to/scenario.yaml",
  "scenario_hash": "abc123",
  "current_step": 2,
  "total_steps": 3,
  "consumed_steps": [true, true, false],
  "intercept_dir": "/tmp/cli-replay-xxx",
  "last_updated": "2026-02-07T10:00:00Z"
}
```

### After (new)

```json
{
  "scenario_path": "/path/to/scenario.yaml",
  "scenario_hash": "abc123",
  "current_step": 2,
  "total_steps": 3,
  "step_counts": [1, 4, 0],
  "intercept_dir": "/tmp/cli-replay-xxx",
  "last_updated": "2026-02-07T10:00:00Z"
}
```

**Migration**: When `step_counts` is nil and `consumed_steps` is present → convert `true` → `1`, `false` → `0`. The `consumed_steps` field is never written in new code.

---

## Recording JSONL Changes

### Before (current)

```json
{"timestamp":"2026-02-07T10:00:00Z","argv":["kubectl","apply","-f","-"],"exit":0,"stdout":"created","stderr":""}
```

### After (new)

```json
{"timestamp":"2026-02-07T10:00:00Z","argv":["kubectl","apply","-f","-"],"exit":0,"stdout":"created","stderr":"","stdin":"apiVersion: v1\nkind: Pod\n"}
```

The `stdin` field is omitted when empty (consistent with `omitempty` tag).
