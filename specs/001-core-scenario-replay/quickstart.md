# Quickstart: cli-replay

**Feature**: 001-core-scenario-replay  
**Date**: 2026-02-05

## 30-Second Demo

```bash
# 1. Create a scenario file
cat > scenario.yaml << 'EOF'
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
EOF

# 2. Setup fake kubectl
ln -s $(which cli-replay) /tmp/kubectl
export PATH=/tmp:$PATH
export CLI_REPLAY_SCENARIO=$(pwd)/scenario.yaml

# 3. Initialize the session
cli-replay run scenario.yaml

# 4. Run your test (cli-replay intercepts transparently)
kubectl get pods
# Output:
# NAME    READY   STATUS    RESTARTS   AGE
# web-0   1/1     Running   0          5m

# 5. Verify all steps were consumed
cli-replay verify scenario.yaml
```

## Multi-Step Scenario

```yaml
meta:
  name: "incident-remediation"
  vars:
    namespace: "production"

steps:
  # Step 1: Check pods (unhealthy)
  - match:
      argv: ["kubectl", "get", "pods", "-n", "{{ .namespace }}"]
    respond:
      exit: 0
      stdout: |
        NAME      READY   STATUS             RESTARTS
        web-0     0/1     CrashLoopBackOff   5

  # Step 2: Restart deployment
  - match:
      argv: ["kubectl", "rollout", "restart", "deployment/web"]
    respond:
      exit: 0
      stdout: "deployment.apps/web restarted"

  # Step 3: Check pods (healthy)
  - match:
      argv: ["kubectl", "get", "pods", "-n", "{{ .namespace }}"]
    respond:
      exit: 0
      stdout: |
        NAME      READY   STATUS    RESTARTS
        web-0     1/1     Running   0
```

## Using External Files

```yaml
meta:
  name: "large-output"

steps:
  - match:
      argv: ["kubectl", "get", "pods", "-o", "json"]
    respond:
      exit: 0
      stdout_file: fixtures/pods.json
```

## Simulating Errors

```yaml
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 1
      stderr: "error: the server doesn't have a resource type \"pods\""
```

## Environment Variable Overrides

```bash
# Scenario uses {{ .namespace }}
# Override via environment:
export namespace=staging
kubectl get pods -n staging
```

## Debugging with Trace Mode

```bash
export CLI_REPLAY_TRACE=1
kubectl get pods
# stderr shows:
# cli-replay: [TRACE] step 1/3 matched
#   argv: ["kubectl", "get", "pods"]
#   stdout: "NAME..."
#   exit: 0
```

## Test Integration Example

```bash
#!/bin/bash
set -e

# Setup
export CLI_REPLAY_SCENARIO=$(pwd)/test-scenario.yaml
export PATH=/tmp/fakes:$PATH
cli-replay run $CLI_REPLAY_SCENARIO

# Run system under test
./my-automation-script.sh

# Verify all expected commands were called
cli-replay verify $CLI_REPLAY_SCENARIO
echo "✓ All CLI interactions verified"
```
