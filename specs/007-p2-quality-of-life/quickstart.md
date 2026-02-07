# Quickstart: P2 Quality of Life Enhancements

**Feature**: 007-p2-quality-of-life  
**Date**: 2026-02-07

## 1. Machine-Readable Verify Output

### JSON Output

```bash
# Run a scenario, then verify with JSON output
cli-replay run scenario.yaml
eval "$(cli-replay run scenario.yaml)"
./my-script.sh
cli-replay verify scenario.yaml --format json
```

Output:
```json
{
  "scenario": "deploy-app",
  "session": "default",
  "passed": true,
  "total_steps": 3,
  "consumed_steps": 3,
  "steps": [
    { "index": 0, "label": "git status", "call_count": 1, "min": 1, "max": 1, "passed": true },
    { "index": 1, "label": "kubectl get pods", "call_count": 1, "min": 1, "max": 1, "passed": true },
    { "index": 2, "label": "kubectl apply -f app.yaml", "call_count": 1, "min": 1, "max": 1, "passed": true }
  ]
}
```

### Pipe to jq

```bash
# Extract only failed steps
cli-replay verify scenario.yaml --format json | jq '.steps[] | select(.passed == false)'

# Get pass/fail status for CI
if cli-replay verify scenario.yaml --format json | jq -e '.passed' > /dev/null; then
  echo "All steps satisfied"
fi
```

### JUnit Output

```bash
# Produce JUnit XML for CI dashboard ingestion
cli-replay verify scenario.yaml --format junit > results.xml
```

Output:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="cli-replay" tests="3" failures="0" errors="0" time="0.000">
  <testsuite name="deploy-app" tests="3" failures="0" errors="0" skipped="0"
             time="0.000" timestamp="2026-02-07T10:30:00Z">
    <testcase name="step[0]: git status" classname="scenario.yaml" time="0.000"/>
    <testcase name="step[1]: kubectl get pods" classname="scenario.yaml" time="0.000"/>
    <testcase name="step[2]: kubectl apply -f app.yaml" classname="scenario.yaml" time="0.000"/>
  </testsuite>
</testsuites>
```

### Exec with Report File

```bash
# Run child process and write structured output to a file
cli-replay exec --report-file results.json --format json scenario.yaml -- ./deploy.sh

# Or write JUnit XML
cli-replay exec --report-file results.xml --format junit scenario.yaml -- ./deploy.sh

# Without --report-file, structured output goes to stderr
cli-replay exec --format json scenario.yaml -- ./deploy.sh 2> results.json
```

### GitHub Actions Integration

```yaml
- name: Run scenario
  run: cli-replay exec --report-file results.xml --format junit scenario.yaml -- ./test.sh

- name: Publish Test Results
  uses: EnricoMi/publish-unit-test-result-action@v2
  if: always()
  with:
    files: results.xml
```

## 2. JSON Schema for Scenario Files

### Add Schema Reference to Your Scenario

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/<org>/cli-replay/main/schema/scenario.schema.json
meta:
  name: my-scenario
  description: Demo scenario with schema validation
steps:
  - match:
      argv: [kubectl, get, pods]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS
        app-1   1/1     Running
```

### What You Get

With VS Code + [YAML extension](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml):

- **Autocompletion**: Type `meta.` and see `name`, `description`, `vars`, `security` suggestions
- **Hover documentation**: Hover over any field to see its description
- **Typo detection**: `stout` instead of `stdout` → red squiggly underline
- **Constraint validation**: `exit: 300` → error "value must be ≤ 255"
- **Mutual exclusivity**: `stdout` + `stdout_file` on same step → error

### Workspace-Level Configuration (Alternative)

Instead of per-file modeline, configure all scenarios at once in `.vscode/settings.json`:

```json
{
  "yaml.schemas": {
    "./schema/scenario.schema.json": ["testdata/scenarios/*.yaml", "examples/**/*.yaml"]
  }
}
```

## 3. Step Groups with Unordered Matching

### Scenario with an Unordered Group

```yaml
meta:
  name: deploy-with-preflight
  description: Deployment with order-independent pre-flight checks

steps:
  # Ordered: must happen first
  - match:
      argv: [git, status]
    respond:
      exit: 0
      stdout: "On branch main\nnothing to commit"

  # Unordered group: these 3 commands can happen in any order
  - group:
      mode: unordered
      name: pre-flight
      steps:
        - match:
            argv: [az, account, show]
          respond:
            exit: 0
            stdout: '{"name": "my-sub"}'
        - match:
            argv: [docker, info]
          respond:
            exit: 0
            stdout: "Server Version: 24.0"
        - match:
            argv: [kubectl, cluster-info]
          respond:
            exit: 0
            stdout: "Kubernetes control plane is running"

  # Ordered: must happen after all pre-flight checks pass
  - match:
      argv: [kubectl, apply, -f, app.yaml]
    respond:
      exit: 0
      stdout: "deployment.apps/app configured"
```

### How It Works

1. `git status` must be called first (ordered step)
2. `az account show`, `docker info`, `kubectl cluster-info` can be called in **any order** (unordered group)
3. `kubectl apply` is only eligible after all 3 pre-flight commands are called (barrier)
4. Verification checks all steps regardless of group membership

### Auto-Named Groups

```yaml
steps:
  - group:
      mode: unordered
      # name omitted → auto-generated as "group-1"
      steps:
        - match: { argv: [cmd-a] }
          respond: { exit: 0 }
        - match: { argv: [cmd-b] }
          respond: { exit: 0 }
```

In JSON/JUnit output, labels appear as `[group:group-1] cmd-a`.

### Groups with Call Bounds

```yaml
steps:
  - group:
      mode: unordered
      name: health-checks
      steps:
        - match:
            argv: [curl, http://localhost:8080/health]
          respond:
            exit: 0
          calls:
            min: 1
            max: 5    # may be polled multiple times
        - match:
            argv: [kubectl, get, pods]
          respond:
            exit: 0
          calls:
            min: 1
            max: 3
```

The group barrier lifts when both steps have met their `min` (1 each). Commands continue matching until all `max` budgets are hit or a non-matching command arrives.

### Verify Output with Groups

```bash
$ cli-replay verify scenario.yaml
✓ Scenario "deploy-with-preflight" completed: 4/4 steps consumed
  Step 1: git status — 1 call ✓
  [group:pre-flight]
    Step 2: az account show — 1 call ✓
    Step 3: docker info — 1 call ✓
    Step 4: kubectl cluster-info — 1 call ✓
  Step 5: kubectl apply -f app.yaml — 1 call ✓
```
