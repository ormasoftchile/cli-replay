# cli-replay Quickstart Guide

## What is cli-replay?

cli-replay intercepts CLI commands and returns canned responses from YAML scenario files. You can:

1. **Record** real command executions into a YAML file
2. **Replay** those commands in tests — no network, no credentials, fully deterministic

---

## Prerequisites

- **Go 1.21+** — [go.dev/doc/install](https://go.dev/doc/install)

## Build

```powershell
# Windows
go build -o cli-replay.exe .

# macOS / Linux
go build -o cli-replay .
```

---

## The Two Modes

| Mode | How it's invoked | What it does |
|------|-----------------|--------------|
| **Management** | `cli-replay run`, `record`, `verify`, `clean` | Manage scenarios and sessions |
| **Intercept** | Symlink/copy named as another command (e.g., `kubectl`) | Return canned responses from a scenario |

---

## End-to-End Walkthrough

### Step 1: Write a scenario (or record one)

You can write YAML by hand:

```yaml
# azure-rg.yaml
meta:
  name: "list-resource-groups"
  description: "Get subscription then list its resource groups"

steps:
  - match:
      argv: ["az", "account", "show", "--query", "id", "-o", "tsv"]
    respond:
      exit: 0
      stdout: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

  - match:
      argv: ["az", "group", "list", "--subscription", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "-o", "table"]
    respond:
      exit: 0
      stdout: |
        Name              Location    Status
        ----------------  ----------  ---------
        my-rg-eastus      eastus      Succeeded
        my-rg-westus2     westus2     Succeeded
```

Or **record it** from a real session:

```powershell
.\cli-replay.exe record --output azure-rg.yaml --command az -- powershell ./my-script.ps1
```

### Step 2: Initialize and configure (one line)

`cli-replay run` creates intercept wrappers, sets up state, and emits shell
commands that configure PATH and CLI_REPLAY_SCENARIO in your current session.

**PowerShell:**
```powershell
.\cli-replay.exe run azure-rg.yaml | Invoke-Expression
```

**bash / zsh:**
```bash
eval "$(cli-replay run azure-rg.yaml)"
```

That's it. Behind the scenes `run`:
1. Reads the scenario and discovers which commands to intercept (`az`)
2. Creates a temp directory with a copy (Windows) or symlink (Unix) of cli-replay named `az`
3. Outputs `$env:PATH = '...'; $env:CLI_REPLAY_SCENARIO = '...'` to stdout
4. `Invoke-Expression` (or `eval`) applies those variables in your shell

### Step 3: Run your script

Every `az` call now hits the fake:

```powershell
# Your script thinks it's talking to real Azure
$sub = az account show --query id -o tsv
# → returns "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

az group list --subscription $sub -o table
# → returns the canned table
```

### Step 4: Verify all steps were consumed

```powershell
.\cli-replay.exe verify azure-rg.yaml
# cli-replay: scenario "list-resource-groups" complete (2/2 steps)
```

Exit code 0 = all steps matched. Exit code 1 = something's missing.

### Step 5: Clean up

```powershell
.\cli-replay.exe clean azure-rg.yaml
# cli-replay: removed intercept dir C:\Users\...\cli-replay-intercept-...
# cli-replay: state reset for azure-rg.yaml
```

`clean` removes the intercept directory and deletes the state file. PATH still
contains the (now removed) directory — harmless, since the files are gone and
`az` resolves to the real binary again.

---

## Recording Commands

### Direct capture (single command)

```powershell
.\cli-replay.exe record --output demo.yaml -- kubectl get pods
```

Runs the real command, captures stdout/stderr/exit, writes YAML.

### Shim-based recording (multi-step script)

```powershell
.\cli-replay.exe record --output workflow.yaml --command kubectl --command docker -- powershell ./deploy.ps1
```

Intercepts only `kubectl` and `docker` calls from the script. Each intercepted call becomes a separate step. The shim directory is auto-cleaned after recording.

### Recording flags

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--output` | `-o` | Yes | Output YAML file path |
| `--name` | `-n` | No | Scenario name (auto-generated if omitted) |
| `--description` | `-d` | No | Scenario description |
| `--command` | `-c` | No | Commands to intercept (repeatable) |

---

## All Commands

| Command | Description |
|---------|-------------|
| `cli-replay exec <scenario.yaml> -- <cmd>` | Full lifecycle: setup, run child, verify, cleanup (recommended) |
| `cli-replay run <scenario.yaml>` | Create intercepts, init state, emit env setup to stdout |
| `cli-replay run --shell bash <s>` | Force output format (powershell, bash, cmd) |
| `cli-replay verify <scenario.yaml>` | Check all steps consumed (exit 0 = complete) |
| `cli-replay validate <scenario.yaml>` | Check scenario for schema/semantic errors without executing |
| `cli-replay clean` | Remove intercept dir + delete state file |
| `cli-replay clean --ttl 10m --recursive .` | Bulk cleanup of expired sessions |
| `cli-replay record -o <file> -- <cmd>` | Record a real command into YAML |
| `cli-replay --version` | Print version |
| `cli-replay --help` | Show usage |

---

## How Intercept Mode Works

```
Your script calls:   az account show --query id -o tsv
                         │
                         ▼
          PATH finds "az.exe" in $shimDir (it's a copy of cli-replay.exe)
                         │
                         ▼
          cli-replay detects os.Args[0] = "az" (not "cli-replay")
                         │
                         ▼
          Reads CLI_REPLAY_SCENARIO → loads azure-rg.yaml
                         │
                         ▼
          Checks state → current step is 0
                         │
                         ▼
          Matches argv ["az","account","show","--query","id","-o","tsv"] ✓
                         │
                         ▼
          Writes canned stdout, exits with code 0, advances state to step 1
```

Steps are matched **strictly in order**. If your script calls commands in a different order or with different args, you'll get a mismatch error.

---

## Scenario YAML Reference

```yaml
meta:
  name: "scenario-name"            # Required
  description: "What this tests"   # Optional
  vars:                            # Optional: template variables
    namespace: "production"
  security:                        # Optional: restrict interceptable commands
    allowed_commands:
      - kubectl
    deny_env_vars:                 # Optional: block env vars from templates
      - "AWS_*"
  session:                         # Optional: auto-cleanup stale sessions
    ttl: "10m"

steps:
  - match:
      argv: ["kubectl", "get", "pods", "-n", "{{ .namespace }}"]
      stdin: |                     # Optional: expected piped input
        apiVersion: v1
    respond:
      exit: 0                      # Required: 0-255
      stdout: "inline output"      # Optional
      stderr: "error text"         # Optional
      stdout_file: "fixtures/out.txt"  # Optional: load from file
      stderr_file: "fixtures/err.txt"  # Optional: load from file
      delay: "500ms"               # Optional: artificial response delay
      capture:                     # Optional: capture values for later steps
        pod_name: "web-0"
    when: '{{ .capture.ready }}'   # Optional: conditional (reserved, not yet evaluated)
    calls:                         # Optional: call count bounds
      min: 1
      max: 5
```

- `stdout` and `stdout_file` are mutually exclusive
- `stderr` and `stderr_file` are mutually exclusive
- Template variables use Go `text/template` syntax: `{{ .varName }}`
- Environment variables override `meta.vars`
- Unknown YAML fields are rejected (strict parsing)
- Use `{{ .any }}` in `match.argv` to match any single argument
- Use `{{ .regex "pattern" }}` in `match.argv` for regex matching
- Use `respond.capture` to chain values between steps (reference via `{{ .capture.<id> }}`)
- Use `calls.min`/`calls.max` for retry loops and optional steps
- Use step groups with `mode: unordered` for order-independent matching

---

## Debugging

Enable trace mode to see what's happening:

```powershell
$env:CLI_REPLAY_TRACE = "1"
az account show --query id -o tsv
# stderr: [cli-replay] step=0 argv=[az account show --query id -o tsv] exit=0
```

---

## Limitations

- **Strict ordering** — steps must match in exact sequence (use step groups for unordered matching)
- **No parallel execution** — state is single-threaded per scenario (session isolation supports parallel test runs)
- **Fixed responses** — no conditional or dynamic response logic based on runtime state
