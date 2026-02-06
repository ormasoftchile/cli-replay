# Feature Request: Built-in Workflow Recorder for cli-replay

## Overview

Add a `record` subcommand to cli-replay that captures real command executions and automatically generates replay scenario YAML files. This eliminates the manual work of crafting scenarios by recording actual workflows.

## User Stories

1. **As a developer**, I want to record my actual kubectl troubleshooting session so I can replay it in demos without manual YAML crafting.

2. **As a trainer**, I want to capture a multi-step CLI workflow once and use it to create consistent, reproducible training scenarios.

3. **As a QA engineer**, I want to record CLI interactions during bug reproduction so I can create automated replay scenarios for testing.

## Proposed Usage

```bash
# Basic recording - execute commands and generate scenario
cli-replay record --output kubectl-workflow.yaml -- bash -c '
  kubectl get pods
  kubectl delete pod web-0
  kubectl get pods
'

# Interactive recording mode
cli-replay record --interactive --output scenario.yaml

# Record specific command only
cli-replay record --command kubectl --output kubectl-demo.yaml

# Record with metadata
cli-replay record \
  --name "pod-restart-demo" \
  --description "Demonstrates pod restart workflow" \
  --output demo.yaml \
  -- bash workflow.sh
```

## Technical Requirements

### Core Functionality

1. **Command Interception**
   - Use PATH shim approach to intercept specified commands
   - Capture full argv, exit code, stdout, stderr
   - Preserve command execution order
   - Support both single commands and shell scripts

2. **YAML Generation**
   - Convert recorded executions to scenario format
   - Auto-generate meta section with name/description
   - Create properly formatted steps with match/respond blocks
   - Preserve output formatting (multiline strings, escaping)

3. **Filtering & Refinement**
   - Allow filtering which commands to record (e.g., only kubectl)
   - Option to exclude commands (e.g., skip `ls`, `cd`)
   - Post-recording editing workflow
   - Merge duplicate commands or keep separate steps

### Implementation Approach

1. **Recorder Architecture**
   ```
   cli-replay record
     ├─ Create temporary bin directory
     ├─ Generate shim executables for target commands
     ├─ Set PATH to prioritize shims
     ├─ Execute user's workflow
     ├─ Collect execution records (JSON format)
     └─ Convert to YAML scenario format
   ```

2. **Shim Template**
   - Transparent wrapper that calls real command
   - Records to structured log (JSONL or similar)
   - Minimal performance overhead
   - Handles edge cases (signals, pipes, etc.)

3. **Conversion Logic**
   - Parse recorded JSONL/JSON
   - Match recorded data to scenario schema
   - Generate proper YAML with:
     - Correct argv arrays
     - Properly escaped/formatted stdout/stderr
     - Exit codes
     - Step ordering

### Advanced Features (Future)

- **Interactive mode**: Start recording session, manually control when to capture
- **Edit-on-the-fly**: Modify responses during recording
- **Template variables**: Auto-detect repeated values, suggest templating
- **Merge scenarios**: Combine multiple recording sessions
- **Smart deduplication**: Detect similar commands, ask to merge

## Success Criteria

- [ ] Can record simple single-command execution
- [ ] Can record multi-step shell script workflows
- [ ] Generated YAML is valid and can be replayed
- [ ] Handles stdout/stderr correctly
- [ ] Preserves command ordering
- [ ] Works with common CLIs (kubectl, docker, aws, git)
- [ ] Documentation includes examples and workflow

## Design Constraints

1. Must work on macOS and Linux
2. Should not require root/sudo
3. Must handle commands with arguments containing spaces, quotes, special chars
4. Generated scenarios should be human-readable and editable
5. Recording should be transparent (minimal impact on command execution)

## Example Workflow

```bash
# User records actual session
$ cli-replay record --output demo.yaml --command kubectl -- bash -c '
  kubectl get pods
  kubectl delete pod web-0
  kubectl get pods
'
Recording session...
✓ Recorded: kubectl get pods (exit: 0)
✓ Recorded: kubectl delete pod web-0 (exit: 0)
✓ Recorded: kubectl get pods (exit: 0)
Scenario written to demo.yaml

# Generated demo.yaml
meta:
  name: "recorded-session-20260206-143022"
  description: "Recorded workflow session"
  recorded_at: "2026-02-06T14:30:22Z"

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

## Related Components

- Uses existing scenario YAML schema (internal/scenario/model.go)
- Integrates with existing replay runner (internal/runner/replay.go)
- May share code with matcher logic (internal/matcher/argv.go)

## Questions to Address

1. How to handle timing-dependent outputs (timestamps, random IDs)?
2. Should we support recording environment variables?
3. How to handle interactive commands (prompts, stdin)?
4. Should recording capture working directory changes?
5. How to handle command aliases and shell functions?
