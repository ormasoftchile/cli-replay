# Quickstart: 010 P3/P4 Enhancements — Production Hardening, CI Ecosystem & Documentation

**Date**: 2026-02-07  
**Spec**: [spec.md](spec.md)

---

## 1. `cli-replay validate` — Pre-Flight Scenario Checks

### Validate a Single File

```bash
cli-replay validate my-scenario.yaml
# ✓ my-scenario.yaml: valid
# Exit code: 0
```

### Validate Multiple Files

```bash
cli-replay validate scenarios/deploy.yaml scenarios/rollback.yaml scenarios/monitor.yaml
# ✓ scenarios/deploy.yaml: valid
# ✗ scenarios/rollback.yaml:
#   - step 2: calls: min (5) must be <= max (3)
# ✓ scenarios/monitor.yaml: valid
# Result: 2/3 files valid
# Exit code: 1 (because one file has errors)
```

### JSON Output for CI Parsing

```bash
cli-replay validate --format json scenarios/*.yaml
```

**Output (stdout):**
```json
[
  {
    "file": "scenarios/deploy.yaml",
    "valid": true,
    "errors": []
  },
  {
    "file": "scenarios/rollback.yaml",
    "valid": false,
    "errors": [
      "step 2: calls: min (5) must be <= max (3)"
    ]
  }
]
```

### Use in Pre-Commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit
changed=$(git diff --cached --name-only --diff-filter=ACM -- '*.yaml' | grep -E 'scenario|test')
if [ -n "$changed" ]; then
  cli-replay validate $changed || exit 1
fi
```

### Error Categories

```bash
# File not found
cli-replay validate missing.yaml
# ✗ missing.yaml:
#   - failed to open scenario file: open missing.yaml: no such file or directory

# YAML syntax error
cli-replay validate malformed.yaml
# ✗ malformed.yaml:
#   - failed to parse scenario: yaml: line 5: did not find expected key

# Semantic error (forward capture reference)
cli-replay validate forward-ref.yaml
# ✗ forward-ref.yaml:
#   - step 1 references capture "rg_id" first defined at step 3 (forward reference)

# Missing stdout_file
cli-replay validate file-ref.yaml
# ✗ file-ref.yaml:
#   - step 2: stdout_file "data/output.txt" not found relative to scenario directory
```

---

## 2. Windows Job Objects — Reliable Process Tree Cleanup

### How It Works (No User Action Needed)

On Windows, `cli-replay exec` now automatically creates a Windows Job Object and assigns the child process to it. This ensures that when the child is terminated (Ctrl+C, timeout, or parent exit), **all descendant processes** are also killed — not just the direct child.

```powershell
# Windows PowerShell — exec mode now kills entire process tree
cli-replay exec scenario.yaml -- cmd /c "start /b ping -n 100 localhost & start /b ping -n 100 127.0.0.1"
# Press Ctrl+C → all pings terminated
```

### Fallback Behavior

If job object creation fails (restricted environments, containers), cli-replay falls back to the previous `Process.Kill()` behavior with a warning:

```
cli-replay: warning: job object unavailable, falling back to single-process kill
```

### Unix: No Changes

Unix signal handling (SIGINT/SIGTERM forwarding) is unchanged.

---

## 3. GitHub Action — One-Line CI Integration

### Basic Usage

```yaml
# .github/workflows/test.yml
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: ormasoftchile/cli-replay-action@v1
        with:
          scenario: test-scenario.yaml
          run: bash run-tests.sh
```

### With JUnit Reporting

```yaml
      - uses: ormasoftchile/cli-replay-action@v1
        with:
          scenario: test-scenario.yaml
          run: bash run-tests.sh
          format: junit
          report-file: test-results.xml

      - uses: dorny/test-reporter@v1
        if: always()
        with:
          name: CLI Replay Tests
          path: test-results.xml
          reporter: java-junit
```

### Cross-Platform Matrix

```yaml
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: ormasoftchile/cli-replay-action@v1
        with:
          scenario: test-scenario.yaml
          run: bash run-tests.sh
```

### Validate Only (PR Gate)

```yaml
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: ormasoftchile/cli-replay-action@v1
        with:
          scenario: scenarios/deploy.yaml
          validate-only: 'true'
```

### Pin to a Specific Version

```yaml
      - uses: ormasoftchile/cli-replay-action@v1
        with:
          scenario: test-scenario.yaml
          run: make test
          version: v1.2.3
```

---

## 4. SECURITY.md — What to Know

After implementation, the repository root will contain `SECURITY.md` covering:

- **Trust Model**: Scenario files are treated as code. Review them like any other source file.
- **PATH Interception**: cli-replay prepends a directory to PATH containing shim scripts. Use `allowed_commands` to restrict which commands are interceptible.
- **Environment Variables**: `deny_env_vars` prevents template-based leaking. Supports glob patterns.
- **Vulnerability Reporting**: Contact method for responsible disclosure.

---

## 5. Cookbook Examples

### Terraform Workflow

```bash
# Navigate to the cookbook
cd examples/cookbook/

# Validate the scenario
cli-replay validate terraform-workflow.yaml
# ✓ terraform-workflow.yaml: valid

# Run the full workflow
cli-replay exec terraform-workflow.yaml -- bash terraform-test.sh
# ✓ Scenario "terraform-workflow" completed: 3/3 steps consumed
```

**What's demonstrated**: Linear `terraform init` → `terraform plan` → `terraform apply` pipeline with realistic multi-line JSON output.

### Helm Deployment

```bash
cli-replay exec helm-deployment.yaml -- bash helm-test.sh
# ✓ Scenario "helm-deployment" completed: 3/3 steps consumed
```

**What's demonstrated**: `helm repo add` → `helm upgrade --install` → `helm status` with template variables and captures.

### kubectl Multi-Tool Pipeline

```bash
cli-replay exec kubectl-pipeline.yaml -- bash kubectl-test.sh
# ✓ Scenario "kubectl-pipeline" completed: 5/5 steps consumed
```

**What's demonstrated**: Step groups (unordered), `calls.min`/`calls.max`, captures, dynamic templates, `allowed_commands`.

### Deciding Which Example to Use

| Use Case | Example | Key Features |
|----------|---------|-------------|
| IaC provisioning pipeline | `terraform-workflow.yaml` | Linear steps, multi-line output, exit codes |
| Package/release management | `helm-deployment.yaml` | Captures, template variables |
| Kubernetes deployment + monitoring | `kubectl-pipeline.yaml` | Groups, call bounds, dynamic captures |

---

## 6. Performance Benchmarks — Contributor Guide

### Run All Benchmarks

```bash
go test -bench=. -benchmem ./internal/matcher/ ./internal/runner/ ./internal/verify/
```

### Compare Against Baseline

```bash
# Run benchmarks with multiple iterations for statistical significance
go test -bench=. -benchmem -count=5 ./internal/... > current.txt

# Compare with saved baseline
benchstat baseline.txt current.txt
```

### Expected Thresholds

| Benchmark | Expected | Regression Alert |
|-----------|----------|-----------------|
| `BenchmarkArgvMatch/100` | < 1ms | > 2ms |
| `BenchmarkArgvMatch/500` | < 5ms | > 10ms |
| `BenchmarkStateRoundTrip/500` | < 10ms | > 20ms |
| `BenchmarkReplayOrchestration_100` | < 500ms | > 1s |
| `BenchmarkFormatJSON_200` | < 50ms | > 100ms |
| `BenchmarkFormatJUnit_200` | < 50ms | > 100ms |

Thresholds are 2× the baseline — intended to catch regressions while tolerating CI environment variation.
