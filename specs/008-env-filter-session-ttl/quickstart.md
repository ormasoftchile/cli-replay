# Quickstart: Environment Variable Filtering & Session TTL

**Feature**: `008-env-filter-session-ttl`  
**Date**: 2026-02-07

## Prerequisites

- `cli-replay` binary built and on PATH
- Existing scenario file (e.g., from quickstart-guide.md)

---

## Feature 1: Environment Variable Filtering

### 1. Create a scenario with deny_env_vars

Create `secure-scenario.yaml`:

```yaml
meta:
  name: secure-test
  security:
    allowed_commands: [kubectl]
    deny_env_vars:
      - "AWS_*"
      - "GITHUB_TOKEN"
      - "*_SECRET"
  vars:
    cluster: "dev-cluster"
    region: "eastus2"
steps:
  - match:
      argv: [kubectl, get, pods]
    respond:
      stdout: "cluster={{ .cluster }} region={{ .region }}\n"
```

### 2. Set some environment variables

```powershell
$env:AWS_SECRET_ACCESS_KEY = "super-secret-key-12345"
$env:GITHUB_TOKEN = "ghp_abc123"
$env:cluster = "prod-override"   # This will be allowed (not in deny list)
```

### 3. Run with deny filtering active

```powershell
cli-replay run secure-scenario.yaml | Invoke-Expression
kubectl get pods
```

**Expected output**:
```
cluster=prod-override region=eastus2
```

Note: `cluster` was overridden by the env var (it's not in the deny list). `AWS_SECRET_ACCESS_KEY` and `GITHUB_TOKEN` would be blocked if referenced in templates.

### 4. Enable trace to see filtering in action

```powershell
$env:CLI_REPLAY_TRACE = "1"
kubectl get pods
```

**Expected stderr trace output**:
```
cli-replay[trace]: denied env var AWS_SECRET_ACCESS_KEY
[cli-replay] step=0 argv=[kubectl get pods] exit=0
```

### 5. Clean up

```powershell
cli-replay clean
```

---

## Feature 2: Session TTL

### 1. Create a scenario with TTL

Create `ci-scenario.yaml`:

```yaml
meta:
  name: ci-pipeline-test
  session:
    ttl: "5m"
steps:
  - match:
      argv: [terraform, plan]
    respond:
      stdout: "No changes. Infrastructure is up-to-date.\n"
  - match:
      argv: [terraform, apply]
    respond:
      stdout: "Apply complete! Resources: 0 added, 0 changed, 0 destroyed.\n"
```

### 2. Start a session

```powershell
cli-replay run ci-scenario.yaml | Invoke-Expression
terraform plan
```

### 3. Simulate a stale session

Wait 6+ minutes (or manually backdate the state file for testing), then start a new session:

```powershell
cli-replay run ci-scenario.yaml | Invoke-Expression
```

**Expected stderr output** (if previous session was stale):
```
cli-replay: cleaned 1 expired sessions
cli-replay: session initialized for "ci-pipeline-test" (2 steps, 1 commands)
```

### 4. Manual TTL cleanup

```powershell
# Clean only sessions older than 10 minutes for a specific scenario
cli-replay clean ci-scenario.yaml --ttl 10m

# Recursively clean all expired sessions under current directory
cli-replay clean --ttl 10m --recursive .
```

---

## Validation

### Verify backward compatibility

Scenarios without `deny_env_vars` or `session.ttl` should work exactly as before:

```yaml
meta:
  name: basic-test
steps:
  - match:
      argv: [echo, hello]
    respond:
      stdout: "hello\n"
```

```powershell
cli-replay run basic-test.yaml | Invoke-Expression
echo hello
# Output: hello
cli-replay clean
```

No TTL cleanup occurs. No env var filtering occurs. Identical behavior to pre-008 versions.
