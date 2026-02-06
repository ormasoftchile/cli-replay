# Quickstart Guide: Workflow Recorder

**Feature**: 002-workflow-recorder  
**Created**: February 6, 2026  
**Audience**: Users who want to record command executions and generate replay scenarios

## What is the Workflow Recorder?

The workflow recorder captures real CLI command executions and automatically generates scenario YAML files. Instead of manually crafting YAML with command arguments and outputs, you simply run your actual workflow once and let cli-replay record it.

## Prerequisites

- cli-replay installed (with `record` subcommand support)
- Target commands available in PATH (e.g., `kubectl`, `docker`, `aws`)
- Write permissions for output directory

## Build from Source

```bash
# Build the record-enabled binary
go build -o bin/cli-replay .

# Verify record subcommand is available
./bin/cli-replay record --help
```

## Quick Start: 30 Second Tutorial

### 1. Record a Single Command

```bash
cli-replay record --output my-first-recording.yaml -- kubectl get pods
```

**What happens**:
- cli-replay intercepts `kubectl get pods`
- Captures the actual output from your cluster
- Generates `my-first-recording.yaml` with the captured data

**Generated file**:
```yaml
meta:
  name: "recorded-session-20260206-143022"

steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS    RESTARTS   AGE
        web-0   1/1     Running   0          5m
```

### 2. Replay the Recording

```bash
cli-replay run my-first-recording.yaml
```

Now when you run `kubectl get pods`, you'll get the exact same output that was recorded, without hitting your actual cluster!

---

## Common Workflows

### Workflow 1: Recording Multi-Step Processes

**Use case**: You want to record a sequence of commands (e.g., build → push → deploy)

```bash
# Create a script with your workflow
cat > deploy-workflow.sh <<'EOF'
#!/bin/bash
docker build -t myapp:v1 .
docker push myapp:v1
kubectl apply -f deployment.yaml
kubectl get deployments
EOF

chmod +x deploy-workflow.sh

# Record the entire workflow
cli-replay record \
  --output deploy-scenario.yaml \
  --name "deploy-workflow" \
  --description "Build, push, and deploy application" \
  -- bash deploy-workflow.sh
```

**Result**: All four commands (docker build, docker push, kubectl apply, kubectl get) are captured in one scenario file.

---

### Workflow 2: Recording Only Specific Commands

**Use case**: Your script has many commands, but you only want to record specific ones (e.g., kubectl, ignoring echo/cd/ls)

```bash
# Record only kubectl commands from a complex script
cli-replay record \
  --output kubectl-only.yaml \
  --command kubectl \
  -- bash deployment-script.sh
```

**Result**: Only `kubectl` invocations are recorded. Other commands run normally but aren't captured.

---

### Workflow 3: Recording Error Scenarios

**Use case**: You want to test error handling by recording failed commands

```bash
# Record a command that fails
cli-replay record \
  --output error-scenario.yaml \
  --name "invalid-flag-error" \
  -- kubectl get pods --invalid-flag
```

**Generated YAML** (includes stderr and non-zero exit code):
```yaml
meta:
  name: "invalid-flag-error"

steps:
  - match:
      argv: ["kubectl", "get", "pods", "--invalid-flag"]
    respond:
      exit: 1
      stderr: |
        Error: unknown flag: --invalid-flag
        See 'kubectl get --help' for usage.
```

---

### Workflow 4: Recording State Changes

**Use case**: You want to show how output changes over time (e.g., pod status transitions)

```bash
# Record the same command multiple times
cli-replay record \
  --output pod-lifecycle.yaml \
  --name "pod-restart-lifecycle" \
  -- bash -c '
    kubectl get pods
    kubectl delete pod web-0
    sleep 5
    kubectl get pods
  '
```

**Result**: Three separate steps showing pod before deletion, deletion confirmation, and pod after restart.

---

## Tips & Best Practices

### Naming Conventions

Use descriptive names for better organization:
```bash
# ✅ Good: Descriptive name
cli-replay record -o kubectl-deployment.yaml --name "k8s-deployment-flow"

# ❌ Less helpful: Auto-generated name
cli-replay record -o scenario.yaml
```

### Command Filtering

Filter commands when recording complex scripts:
```bash
# Record only kubectl and docker, ignore bash builtins
cli-replay record \
  -o ci-pipeline.yaml \
  -c kubectl \
  -c docker \
  -- bash ci-script.sh
```

### Verifying Recordings

Always test your recording immediately:
```bash
# Record
cli-replay record -o test.yaml -- kubectl get pods

# Verify it replays correctly
cli-replay run test.yaml
kubectl get pods  # Should output the recorded response
```

### Editing Recordings

Recordings generate standard YAML - you can edit them manually:
```bash
# Record initial scenario
cli-replay record -o base.yaml -- kubectl get pods

# Edit the YAML to modify outputs or add variations
vim base.yaml

# Verify edited scenario
cli-replay run base.yaml
```

---

## Troubleshooting

### Problem: "command not found in PATH"

```bash
$ cli-replay record -o test.yaml -c kubectl -- bash script.sh
Error: command 'kubectl' not found in PATH
```

**Solution**: Ensure the command you're filtering for exists in your PATH:
```bash
which kubectl  # Verify command exists
export PATH=$PATH:/path/to/kubectl  # Add to PATH if needed
```

---

### Problem: "output path not writable"

```bash
$ cli-replay record -o /root/test.yaml -- kubectl get pods
Error: output path not writable: /root/test.yaml
```

**Solution**: Use a path you have write permissions for:
```bash
cli-replay record -o ~/test.yaml -- kubectl get pods
```

---

### Problem: Recording captures no commands

```bash
$ cli-replay record -o test.yaml -c kubectl -- echo "hello"
# test.yaml has no steps
```

**Solution**: The command filter didn't match any executions. Either:
- Remove the filter to record all commands: `cli-replay record -o test.yaml -- echo "hello"`
- Ensure the filtered command is actually executed in your script

---

### Problem: Special characters in command arguments

```bash
# Command with spaces and quotes
kubectl get pods --field-selector="status.phase=Running"
```

**Solution**: The recorder handles this automatically. Just run it:
```bash
cli-replay record -o test.yaml -- kubectl get pods --field-selector="status.phase=Running"
```

---

## Real-World Examples

### Example 1: CI/CD Pipeline Testing

Record your deployment pipeline once, then replay it in tests without actual infrastructure:

```bash
# Record production deployment
cli-replay record \
  --output prod-deploy.yaml \
  --name "production-deployment" \
  --description "Full production deployment workflow" \
  -- bash -c '
    docker build -t app:v1.2.3 .
    docker push app:v1.2.3
    kubectl set image deployment/app app=app:v1.2.3
    kubectl rollout status deployment/app
  '

# Use in tests
export CLI_REPLAY_SCENARIO=prod-deploy.yaml
# Your test suite now sees fake docker/kubectl responses
npm test
```

---

### Example 2: Training & Demos

Create consistent demo scenarios without live clusters:

```bash
# Record demo scenario
cli-replay record \
  --output k8s-troubleshooting-demo.yaml \
  --name "troubleshoot-crashloop" \
  --description "Demo: Debugging CrashLoopBackOff pods" \
  -- bash -c '
    kubectl get pods
    kubectl describe pod web-0
    kubectl logs web-0
    kubectl delete pod web-0
    kubectl get pods
  '

# Present demo (no cluster required)
cli-replay run k8s-troubleshooting-demo.yaml
# Demo commands now work offline
```

---

### Example 3: Bug Reproduction

Capture the exact command sequence that triggers a bug:

```bash
# Record bug scenario
cli-replay record \
  --output bug-123-repro.yaml \
  --name "bug-123-auth-failure" \
  --description "Reproduces authentication failure in kubectl" \
  -- bash reproduce-bug.sh

# Share with team - they can replay without your environment
# Attach bug-123-repro.yaml to bug report
```

---

## Next Steps

- **Learn more**: Read [CLI Contract](contracts/cli.md) for complete flag reference
- **Advanced usage**: See [Data Model](data-model.md) for YAML structure details
- **Contribute**: Check [Implementation Plan](plan.md) for development guidelines

## FAQ

**Q: Can I record interactive commands (prompts for user input)?**  
A: No. The recorder only supports non-interactive commands. Interactive prompts are out of scope.

**Q: Will recording modify my actual system?**  
A: Yes! The recorder executes real commands to capture their output. It's not a simulation - it runs the actual tools.

**Q: Can I edit the YAML after recording?**  
A: Absolutely. The generated YAML is standard cli-replay scenario format. Edit it as needed.

**Q: Does recording work on Windows?**  
A: Not in the initial release. macOS and Linux only for now.

**Q: How do I record environment variables?**  
A: Environment variable capture is out of scope. The recorder only captures command argv and outputs.

**Q: What if my command takes a long time?**  
A: The recorder waits for the command to complete. Long-running commands are supported (e.g., `sleep 60` will wait 60 seconds before generating YAML).

**Q: Can I merge multiple recordings?**  
A: Not automatically. You can manually combine YAML files by copying steps between scenarios.
