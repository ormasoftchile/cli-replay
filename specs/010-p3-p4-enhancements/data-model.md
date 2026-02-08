# Data Model: 010 P3/P4 Enhancements — Production Hardening, CI Ecosystem & Documentation

**Date**: 2026-02-07  
**Spec**: [spec.md](spec.md) | **Research**: [research.md](research.md)

---

## Entity: JobObject (Windows Process Tree Manager)

**Location**: New — `internal/platform/jobobject_windows.go` (build-tagged `//go:build windows`)

This entity represents a Windows Job Object used to group a child process and all its descendants for collective lifecycle management. It is not persisted — it exists only for the duration of a single `cli-replay exec` invocation.

### Structure

| Field     | Type                    | Description                                                                   |
|-----------|-------------------------|-------------------------------------------------------------------------------|
| handle    | `windows.Handle`        | Win32 job object handle from `CreateJobObject`                                |
| assigned  | `bool`                  | Whether a process has been successfully assigned to this job                  |

### API Surface

| Method                 | Signature                                              | Description                                                              |
|------------------------|--------------------------------------------------------|--------------------------------------------------------------------------|
| `NewJobObject()`       | `func NewJobObject() (*JobObject, error)`              | Creates an anonymous job with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` set. |
| `AssignProcess(pid)`   | `func (j *JobObject) AssignProcess(pid int) error`     | Opens process handle by PID and assigns it to the job.                   |
| `Terminate(exitCode)`  | `func (j *JobObject) Terminate(exitCode uint32) error` | Explicitly terminates all processes in the job.                          |
| `Close()`              | `func (j *JobObject) Close() error`                    | Closes the job handle; kills remaining processes via `KILL_ON_JOB_CLOSE`.|

### Lifecycle

```
NewJobObject()
    → CreateJobObject(nil, nil)
    → SetInformationJobObject(job, ExtendedLimitInfo, KILL_ON_JOB_CLOSE)
    ↓
childCmd.Start()  // child spawned
    ↓
AssignProcess(childCmd.Process.Pid)
    → OpenProcess(PROCESS_SET_QUOTA | PROCESS_TERMINATE, false, pid)
    → AssignProcessToJobObject(job, processHandle)
    → CloseHandle(processHandle)
    ↓
[child runs, may spawn grandchildren — all tracked by job]
    ↓
Signal received (Ctrl+C / timeout)
    → Terminate(1)  // kills entire process tree
    ↓
defer Close()
    → CloseHandle(job)  // safety net: KILL_ON_JOB_CLOSE activates if any remain
```

### State Transitions

| State | Trigger | Next State | Behavior |
|-------|---------|------------|----------|
| Created | `NewJobObject()` succeeds | Active | Handle valid, KILL_ON_JOB_CLOSE configured |
| Created | `NewJobObject()` fails | Fallback | Log warning, use `Process.Kill()` |
| Active | `AssignProcess()` succeeds | Assigned | Process tree tracked |
| Active | `AssignProcess()` fails (nested job on Win7-) | Fallback | Log warning, use `Process.Kill()` |
| Assigned | Signal received | Terminating | `Terminate()` called, all descendants killed |
| Assigned | Child exits normally | Closing | No explicit terminate needed |
| Terminating/Closing | `Close()` | Closed | Handle released, safety net activates |

### Fallback Behavior

When job object creation or assignment fails:
1. Emit warning to stderr: `cli-replay: warning: job object unavailable, falling back to single-process kill`
2. Fall back to current `Process.Kill()` behavior (exact behavior preserved from existing `exec_windows.go`)
3. No error propagation — this is a graceful degradation, not a fatal error

### Win32 API Mapping

| Go Function | Win32 API | Constants Used |
|-------------|-----------|---------------|
| `windows.CreateJobObject(nil, nil)` | `CreateJobObjectW` | Anonymous (nil name) |
| `windows.SetInformationJobObject(...)` | `SetInformationJobObject` | `JobObjectExtendedLimitInformation` (9), `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (0x2000) |
| `windows.AssignProcessToJobObject(job, proc)` | `AssignProcessToJobObject` | — |
| `windows.TerminateJobObject(job, code)` | `TerminateJobObject` | Exit code = 1 |
| `windows.OpenProcess(access, inherit, pid)` | `OpenProcess` | `PROCESS_SET_QUOTA \| PROCESS_TERMINATE` |
| `windows.CloseHandle(handle)` | `CloseHandle` | — |

---

## Entity: ValidationResult

**Location**: New — `cmd/validate.go`

This entity represents the output of `cli-replay validate` for a single file. It is not persisted — it is serialized to stdout (JSON) or stderr (text) and discarded.

### Structure

| Field  | Type       | JSON Tag        | Description                                                  |
|--------|------------|-----------------|--------------------------------------------------------------|
| File   | `string`   | `json:"file"`   | Path to the validated scenario file (as provided by user)    |
| Valid  | `bool`     | `json:"valid"`  | `true` if no errors found                                    |
| Errors | `[]string` | `json:"errors"` | List of error descriptions; empty when `Valid` is `true`     |

### Validation Pipeline

```
Input: file path(s) from CLI args
    ↓
For each file:
    1. filepath.Abs(path) → resolve to absolute path
    2. scenario.LoadFile(absPath)
       → YAML parse (strict: KnownFields=true)
       → Scenario.Validate()
           → Meta.Validate()
           → StepElement.Validate() × N
           → validateCaptures()
    3. If LoadFile returns error → ValidationResult{Valid: false, Errors: [error.Error()]}
    4. If LoadFile succeeds → check stdout_file/stderr_file existence
       → For each step with stdout_file or stderr_file:
           → Resolve path relative to scenario file directory
           → If file doesn't exist → append warning to Errors
    5. Build ValidationResult
    ↓
Output: []ValidationResult
```

### Output Formats

**Text format** (to stderr):
```
✓ scenario-a.yaml: valid
✗ scenario-b.yaml:
  - meta: name must be non-empty
  - step 2: calls: min (5) must be <= max (3)
  - step 3: respond: stdout and stdout_file are mutually exclusive
✗ scenario-c.yaml:
  - failed to parse scenario: yaml: line 5: did not find expected key
```

**JSON format** (to stdout):
```json
[
  {
    "file": "scenario-a.yaml",
    "valid": true,
    "errors": []
  },
  {
    "file": "scenario-b.yaml",
    "valid": false,
    "errors": [
      "meta: name must be non-empty",
      "step 2: calls: min (5) must be <= max (3)",
      "step 3: respond: stdout and stdout_file are mutually exclusive"
    ]
  }
]
```

### Error Categories

| Category | Source | Example |
|----------|--------|---------|
| File I/O | `os.Open` failure | `failed to open scenario file: no such file or directory` |
| YAML Parse | `yaml.Decoder.Decode` | `failed to parse scenario: yaml: line 5: ...` |
| Schema | `Scenario.Validate()` → field-level | `meta: name must be non-empty` |
| Semantic | `Scenario.validateCaptures()` | `step 2 references capture "x" first defined at step 4 (forward reference)` |
| File Reference | New in `validate` | `step 3: stdout_file "data/output.txt" not found relative to scenario directory` |

---

## Entity: GitHub Action Definition (action.yml)

**Location**: Separate repository — `ormasoftchile/cli-replay-action/action.yml`

This is a composite GitHub Action manifest. It is not a Go data structure — it's a YAML configuration file that lives in a separate repository.

### Inputs

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `scenario` | string | yes | — | Path to scenario YAML file |
| `run` | string | no | — | Command to run under interception |
| `version` | string | no | `latest` | cli-replay version (e.g., `v1.2.3`) |
| `format` | string | no | — | Output format: `json` or `junit` |
| `report-file` | string | no | — | Path to write verification report |
| `validate-only` | string | no | `false` | Run `validate` instead of `exec` |
| `allowed-commands` | string | no | — | Comma-separated command allowlist |

### Outputs

| Name | Type | Description |
|------|------|-------------|
| `cli-replay-version` | string | Installed cli-replay version |
| `result` | string | `pass` or `fail` |

### Execution Flow

```
Step 1: Detect platform
    → runner.os (Linux/macOS/Windows) → GOOS (linux/darwin/windows)
    → runner.arch (X64/ARM64) → GOARCH (amd64/arm64)

Step 2: Resolve version
    → If "latest": query GitHub Releases API
    → Else: use specified version tag

Step 3: Download binary
    → URL: https://github.com/ormasoftchile/cli-replay/releases/download/{version}/cli-replay-{version}-{goos}-{goarch}.{tar.gz|zip}
    → Extract to ${{ github.action_path }}/bin/

Step 4: Add to PATH
    → echo "${{ github.action_path }}/bin" >> "$GITHUB_PATH"

Step 5: Execute (conditional)
    → If validate-only=true: cli-replay validate {scenario}
    → Else if run is set: cli-replay exec [flags] {scenario} -- {run}
    → Else: cli-replay validate {scenario} (setup-only mode)
```

### Platform Binary Matrix

| runner.os | runner.arch | GOOS | GOARCH | Binary Name | Archive Format |
|-----------|------------|------|--------|-------------|----------------|
| Linux | X64 | linux | amd64 | cli-replay | tar.gz |
| Linux | ARM64 | linux | arm64 | cli-replay | tar.gz |
| macOS | X64 | darwin | amd64 | cli-replay | tar.gz |
| macOS | ARM64 | darwin | arm64 | cli-replay | tar.gz |
| Windows | X64 | windows | amd64 | cli-replay.exe | zip |

---

## Entity: SECURITY.md Document Structure

**Location**: Repository root — `SECURITY.md`

This is a documentation entity, not a code struct. It follows the GitHub security advisory convention.

### Sections

| Section | Content |
|---------|---------|
| Security Policy | Supported versions table |
| Reporting a Vulnerability | Disclosure process (email, GitHub Security Advisory, response SLA) |
| Trust Model | Scenarios are trusted code; scenario files receive same review as source |
| Threat Boundaries | PATH interception risks, environment variable leaking, session isolation |
| Security Controls | `allowed_commands`, `deny_env_vars`, `KnownFields(true)`, session TTL |
| Known Limitations | `deny_env_vars` is pattern-based (not content scanning); shim scripts run as current user |
| Recommendations | Scenario review in PRs; restrict `allowed_commands`; use `deny_env_vars` for secrets |

---

## Entity: Cookbook Scenario

**Location**: `examples/cookbook/` — 3 scenario YAML files + companion scripts + README

This is a documentation entity. Each cookbook scenario is a self-contained, annotated YAML file that passes `cli-replay validate`.

### Scenario Structure (Common Pattern)

| Field | Purpose | Example |
|-------|---------|---------|
| `meta.name` | Descriptive name | `terraform-workflow` |
| `meta.description` | What the scenario demonstrates | `Terraform init/plan/apply pipeline with realistic output` |
| `meta.vars` | Template variables for customization | `{project: "my-project", region: "us-east-1"}` |
| `meta.security.allowed_commands` | Restrict to relevant tools | `["terraform"]` |
| `steps[]` | Annotated step chain | See individual examples |

### Cookbook Index

| Scenario | File | Commands Intercepted | Features Demonstrated |
|----------|------|---------------------|----------------------|
| Terraform Workflow | `terraform-workflow.yaml` | `terraform` | Linear steps, exit codes, multi-line stdout |
| Helm Deployment | `helm-deployment.yaml` | `helm` | Captures, template variables, realistic output |
| kubectl Pipeline | `kubectl-pipeline.yaml` | `kubectl` | Step groups (unordered), `calls.min/max`, captures, dynamic templates |

---

## Entity: BENCHMARKS.md Document Structure

**Location**: Repository root — `BENCHMARKS.md`

Documentation entity documenting existing benchmark baselines.

### Sections

| Section | Content |
|---------|---------|
| Overview | Purpose, how to run, reference system specs |
| Benchmark Results | Table of each benchmark with baseline ns/op, B/op, allocs/op |
| Threshold Policy | 2× baseline = warning, 4× baseline = regression |
| Contributing | How to add benchmarks, format, naming conventions |

### Benchmark Inventory (existing)

| Benchmark | Package | Scale | Expected Threshold |
|-----------|---------|-------|--------------------|
| `BenchmarkArgvMatch/100` | `internal/matcher` | 100 steps | < 1ms |
| `BenchmarkArgvMatch/500` | `internal/matcher` | 500 steps | < 5ms |
| `BenchmarkGroupMatch_50` | `internal/matcher` | 50 groups | < 2ms |
| `BenchmarkStateRoundTrip/100` | `internal/runner` | 100 steps | < 2ms |
| `BenchmarkStateRoundTrip/500` | `internal/runner` | 500 steps | < 10ms |
| `BenchmarkReplayOrchestration_100` | `internal/runner` | 100 steps | < 500ms |
| `BenchmarkFormatJSON_200` | `internal/verify` | 200 steps | < 50ms |
| `BenchmarkFormatJUnit_200` | `internal/verify` | 200 steps | < 50ms |

---

## Relationships

```
exec command (cmd/exec.go)
    │
    ├── [Windows only] ──→ JobObject (internal/platform/jobobject_windows.go)
    │       ├── NewJobObject() → handle
    │       ├── AssignProcess(child.Pid)
    │       ├── Terminate(1) on signal
    │       └── Close() on defer
    │
    ├── [Unix unchanged] ──→ setupSignalForwarding (cmd/exec_unix.go)
    │
    └── [All platforms] ──→ scenario.LoadFile() + Scenario.Validate()
                               │
                               └── Reused by validate command
                                     │
                                     └── ValidationResult[]
                                           ├── text → stderr
                                           └── json → stdout

GitHub Action (separate repo)
    │
    ├── Downloads cli-replay binary
    ├── Invokes: cli-replay exec ... | cli-replay validate ...
    └── Reports: pass/fail step outcome

Documentation entities (no code dependencies):
    ├── SECURITY.md (repo root)
    ├── BENCHMARKS.md (repo root)
    └── examples/cookbook/ (self-contained)
```
