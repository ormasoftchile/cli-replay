# Quickstart: P0 Critical Enhancements

**Feature**: `005-p0-critical-enhancements`

This guide walks through each new capability introduced in the P0 enhancements.

---

## 1. Enhanced Mismatch Diagnostics

No YAML changes required — diagnostics are automatic when a mismatch occurs.

### Try It

Create a scenario that expects `kubectl get pods`:

```yaml
# scenario.yaml
meta:
  name: diag-demo
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "No resources found"
```

Run it and trigger a mismatch:

```bash
eval "$(cli-replay run scenario.yaml)"
kubectl get deployments   # wrong — expects "pods"
```

You'll see an enhanced error:

```
Mismatch at step 1 of "diag-demo":

  Expected: ["kubectl", "get", "pods"]
  Received: ["kubectl", "get", "deployments"]
                              ^^^^^^^^^^^
  First difference at position 2:
    expected: "pods"
    received: "deployments"
```

### With Regex Patterns

```yaml
steps:
  - match:
      argv: ["kubectl", "get", "pods", "-n", '{{ .regex "^prod-.*" }}']
    respond:
      exit: 0
      stdout: "..."
```

When called with `kubectl get pods -n staging-app`:

```
  First difference at position 4:
    expected pattern: ^prod-.*
    received value:   staging-app
```

---

## 2. Call Count Bounds

### Basic Polling Step

```yaml
meta:
  name: polling-demo
steps:
  # This step accepts 1-5 invocations
  - match:
      argv: ["kubectl", "rollout", "status", "deployment/myapp"]
    respond:
      exit: 0
      stdout: "Waiting for rollout..."
    calls:
      min: 1
      max: 5

  # Then exactly one verification
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "myapp-abc   1/1   Running"
```

### Try It

```bash
eval "$(cli-replay run polling-demo.yaml)"

# Poll 3 times (all accepted)
kubectl rollout status deployment/myapp
kubectl rollout status deployment/myapp
kubectl rollout status deployment/myapp

# Move to next step
kubectl get pods

# Verify
cli-replay verify
```

Output:
```
✓ Scenario "polling-demo" completed: 2/2 steps consumed
  Step 1: kubectl rollout status — 3 calls (min: 1, max: 5) ✓
  Step 2: kubectl get pods — 1 call (min: 1, max: 1) ✓
```

### Optional Step

```yaml
- match:
    argv: ["kubectl", "get", "events"]
  respond:
    exit: 0
    stdout: ""
  calls:
    min: 0
    max: 1
```

This step is satisfied whether called 0 or 1 times.

---

## 3. stdin Matching

### Pipe Content Validation

```yaml
meta:
  name: stdin-demo
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

### Try It

```bash
eval "$(cli-replay run stdin-demo.yaml)"

# Pipe matching content
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
EOF
```

The step matches both the argv and the piped stdin.

### Backward Compatible

If you omit `match.stdin`, stdin is ignored as before:

```yaml
# Works exactly like today — stdin not checked
- match:
    argv: ["kubectl", "apply", "-f", "-"]
  respond:
    exit: 0
    stdout: "created"
```

### Recording stdin

When recording a command that receives piped input:

```bash
eval "$(cli-replay record)"
echo '{"key":"val"}' | mycommand --input -
cli-replay stop
```

The generated YAML includes the stdin field automatically.

---

## 4. Security Allowlist

### In YAML

```yaml
meta:
  name: secure-deploy
  security:
    allowed_commands:
      - kubectl
      - az
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
  - match:
      argv: ["az", "account", "show"]
    respond:
      exit: 0
```

If any step references a command not in the list, `cli-replay run` rejects it immediately:

```
Error: command "docker" is not in the allowed commands list: [kubectl, az]
```

### Via CLI Flag

```bash
cli-replay run --allowed-commands kubectl,az scenario.yaml -- ./deploy.sh
```

### Both (Intersection)

```yaml
# YAML allows: kubectl, az, docker
meta:
  security:
    allowed_commands: ["kubectl", "az", "docker"]
```

```bash
# CLI restricts to: kubectl, az
cli-replay run --allowed-commands kubectl,az scenario.yaml -- ./deploy.sh
```

Effective allowlist: `[kubectl, az]` (intersection — stricter set wins).

---

## Migration Guide

### Existing Scenarios

No changes needed. All new fields are optional with backward-compatible defaults:

| Feature | Default when absent |
|---------|-------------------|
| `calls` | `{min: 1, max: 1}` (exactly once, same as today) |
| `match.stdin` | ignored (same as today) |
| `meta.security` | no restrictions (same as today) |
| Mismatch diagnostics | automatic (no opt-in) |

### State File Migration

State files from previous versions use `consumed_steps: [true, false, ...]`. The new version reads these and converts to `step_counts: [1, 0, ...]` automatically. No manual intervention needed.
