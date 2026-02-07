# Quickstart: 009 P1/P2 Enhancements

**Date**: 2026-02-07  
**Spec**: [spec.md](spec.md)

---

## 1. Dynamic Capture — Chain Output Between Steps

### Scenario File

```yaml
meta:
  name: capture-demo
  description: Capture a resource ID from step 1 and use it in step 2
  vars:
    region: eastus

steps:
  - match:
      argv: ["az", "group", "create", "--name", "demo-rg", "--location", "{{ .region }}"]
    respond:
      exit: 0
      stdout: '{"id": "/subscriptions/abc123/resourceGroups/demo-rg", "name": "demo-rg"}'
      capture:
        rg_id: "/subscriptions/abc123/resourceGroups/demo-rg"

  - match:
      argv: ["az", "vm", "create", "--resource-group", "demo-rg"]
    respond:
      exit: 0
      stdout: 'VM created in group {{ .capture.rg_id }}'
```

### Run It

```bash
# Start the replay session
eval "$(cli-replay run capture-demo.yaml)"

# Step 1 — produces capture
az group create --name demo-rg --location eastus
# Output: {"id": "/subscriptions/abc123/resourceGroups/demo-rg", "name": "demo-rg"}

# Step 2 — uses captured value
az vm create --resource-group demo-rg
# Output: VM created in group /subscriptions/abc123/resourceGroups/demo-rg
```

### Captures in Unordered Groups

```yaml
steps:
  - group:
      name: monitoring
      mode: unordered
    children:
      - match:
          argv: ["kubectl", "get", "pods"]
        respond:
          exit: 0
          stdout: "web-pod-1"
          capture:
            first_pod: "web-pod-1"

      - match:
          argv: ["kubectl", "get", "svc"]
        respond:
          exit: 0
          # If this step runs before the pods step, {{ .capture.first_pod }}
          # resolves to empty string (best-effort semantics)
          stdout: 'Service for {{ .capture.first_pod }}'
```

---

## 2. Dry-Run Mode — Preview Without Side Effects

### Preview a Scenario

```bash
cli-replay run --dry-run deploy.yaml
```

**Example Output:**

```
Scenario: deploy-to-aks
Description: Full AKS deployment pipeline
Steps: 4 | Groups: 1 | Commands: 3

Template Variables: cluster, namespace
Session TTL: 30m
Allowlist: kubectl, az

──────────────────────────────────────────────────────────
 #   Command / Match Pattern                   Calls  Group
──────────────────────────────────────────────────────────
 1   az group create --name {{.rg}}            [1,1]  —
     → exit 0 | stdout: {"id": "/subscriptions/...
     ↳ captures: rg_id

 2   kubectl get pods -n {{.namespace}}        [1,∞)  monitor (unordered)
     → exit 0 | stdout: NAME         READY ...

 3   kubectl get svc -n {{.namespace}}         [1,∞)  monitor (unordered)
     → exit 0 | stdout: NAME         TYPE  ...

 4   az vm create --resource-group demo-rg     [1,1]  —
     → exit 0 | stdout: VM created in group...
──────────────────────────────────────────────────────────

✓ All commands match allowlist
✓ No validation errors
```

### Dry-Run with exec

```bash
cli-replay exec --dry-run deploy.yaml -- az group create --name demo-rg
```

### Dry-Run Catches Errors

```bash
# Scenario with missing meta.name
cli-replay run --dry-run bad-scenario.yaml
# stderr: validation error: meta.name is required
# exit code: 1
```

---

## 3. Windows Signal Handling

### No User-Facing Changes

Signal handling improvements are internal. Ctrl+C in `exec` mode now correctly terminates child processes on Windows.

```powershell
# Windows PowerShell — exec mode works the same
cli-replay exec deploy.yaml -- az group create --name demo-rg
# Press Ctrl+C → child terminated, intercept dir cleaned up
```

### Known Limitation

On Windows, grandchild processes (processes spawned by the child process) may not be terminated when the parent is killed. Use `cli-replay clean` to remove any stale state.

```powershell
cli-replay clean
```

---

## 4. Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem ./internal/matcher/ ./internal/runner/ ./internal/verify/

# Run only 100+ step benchmarks
go test -bench='100|200|500' -benchmem ./internal/...

# Compare with baseline (using benchstat)
go test -bench=. -benchmem -count=5 ./internal/... > new.txt
benchstat old.txt new.txt
```
