# Data Model: 009 P1/P2 Enhancements

**Date**: 2026-02-07  
**Spec**: [spec.md](spec.md) | **Research**: [research.md](research.md)

---

## Entity: Capture (on Response)

**Location**: `internal/scenario/model.go` — `Response` struct

| Field   | Type               | YAML Tag                  | Description                                              |
|---------|--------------------|---------------------------|----------------------------------------------------------|
| Capture | `map[string]string` | `yaml:"capture,omitempty"` | Named key-value pairs produced by this step's response.  |

### Validation Rules

| Rule                            | Level                | Error                                                                                |
|---------------------------------|----------------------|--------------------------------------------------------------------------------------|
| Identifier format               | `Response.Validate`  | Capture identifier must match `^[a-zA-Z_][a-zA-Z0-9_]*$`                            |
| No conflict with meta.vars keys | `Scenario.Validate`  | Capture identifier `"X"` conflicts with `meta.vars` key `"X"`                       |
| No forward references            | `Scenario.Validate`  | Step N references `capture.X` but `X` is first defined at step M where M > N        |
| Group sibling semantics          | Runtime (no error)   | In unordered groups, unexecuted sibling captures resolve to empty string silently    |
| Optional step semantics          | Runtime (no error)   | If producing step has `calls.min: 0` and was never invoked, capture resolves to `""` |

### YAML Example

```yaml
steps:
  - match:
      argv: ["az", "group", "create", "--name", "mygroup"]
    respond:
      exit: 0
      stdout: '{"id": "/subscriptions/.../mygroup", "name": "mygroup"}'
      capture:
        resource_group_id: "/subscriptions/.../mygroup"
        group_name: "mygroup"
```

---

## Entity: State.Captures

**Location**: `internal/runner/state.go` — `State` struct

| Field    | Type               | JSON Tag                   | Description                                              |
|----------|--------------------|----------------------------|----------------------------------------------------------|
| Captures | `map[string]string` | `json:"captures,omitempty"` | Accumulated capture values from all executed steps so far. |

### Lifecycle

1. **Initialization**: `NewState()` sets `Captures` to `make(map[string]string)`.
2. **Accumulation**: After each step response is served, if `step.Respond.Capture` is non-empty, merge all key-value pairs into `state.Captures`.
3. **Persistence**: `WriteState()` serializes `Captures` as part of the atomic JSON write (existing pattern — no new I/O).
4. **Recovery**: `ReadState()` deserializes `Captures`. Old state files without `"captures"` produce a nil map (safe — treated as empty).
5. **Consumption**: `ReplayResponseWithTemplate()` reads `state.Captures` and injects them into the template variable map under the `capture` namespace.

### State Transitions

```
NewState() → Captures = {}
  ↓
Step 1 executes, has capture {a: "1"}
  → state.Captures = {a: "1"}, WriteState()
  ↓
Step 2 executes, has capture {b: "2"}
  → state.Captures = {a: "1", b: "2"}, WriteState()
  ↓
Step 3 references {{ .capture.a }} and {{ .capture.b }}
  → template renders with a="1", b="2"
```

### JSON Example (persisted state file)

```json
{
  "scenario_path": "/path/to/scenario.yaml",
  "scenario_hash": "abc123",
  "current_step": 2,
  "total_steps": 3,
  "step_counts": [1, 1, 0],
  "intercept_dir": "/tmp/.cli-replay-xyz",
  "last_updated": "2026-02-07T10:00:00Z",
  "captures": {
    "resource_group_id": "/subscriptions/.../mygroup",
    "group_name": "mygroup"
  }
}
```

---

## Entity: DryRunReport

**Location**: New — `internal/runner/dryrun.go` (or `cmd/dryrun.go`)

This is an internal struct for formatting dry-run output. Not persisted.

| Field            | Type              | Description                                                    |
|------------------|-------------------|----------------------------------------------------------------|
| ScenarioName     | `string`          | From `scn.Meta.Name`                                          |
| Description      | `string`          | From `scn.Meta.Description`                                   |
| TotalSteps       | `int`             | `len(scn.FlatSteps())`                                        |
| Steps            | `[]DryRunStep`    | One entry per flat step                                        |
| Groups           | `[]GroupRange`    | From `scn.GroupRanges()`                                       |
| Commands         | `[]string`        | Unique commands extracted from scenario                        |
| Allowlist        | `[]string`        | From `scn.Meta.Security.Allowlist`                             |
| AllowlistIssues  | `[]string`        | Steps referencing commands not in allowlist                    |
| TemplateVars     | `[]string`        | Keys from `scn.Meta.Vars`                                     |
| SessionTTL       | `string`          | From `scn.Meta.Session.TTL` (if set)                          |

### DryRunStep

| Field       | Type     | Description                                                    |
|-------------|----------|----------------------------------------------------------------|
| Index       | `int`    | Flat step index (0-based)                                      |
| MatchArgv   | `string` | Joined argv pattern (e.g., `kubectl get pods -n {{.ns}}`)     |
| Exit        | `int`    | Response exit code                                             |
| StdoutPreview | `string` | First 80 chars of stdout, or `[file: path]` for stdout_file |
| CallsMin    | `int`    | Minimum expected call count                                    |
| CallsMax    | `int`    | Maximum expected call count (0 = unlimited)                    |
| GroupName   | `string` | Group name if step is in a group, else empty                   |
| GroupMode   | `string` | Group mode if step is in a group, else empty                   |
| Captures    | `[]string` | Capture identifiers defined by this step                     |

### Output Format (stdout)

```
Scenario: deploy-to-aks
Description: Multi-step AKS deployment with resource chaining
Steps: 5 | Groups: 1 | Commands: 3

Template Variables: cluster, namespace, image
Session TTL: 30m
Allowlist: kubectl, az, helm

──────────────────────────────────────────────────────────
 #   Command / Match Pattern                   Calls  Group
──────────────────────────────────────────────────────────
 1   az group create --name {{.rg}}            [1,1]  —
     → exit 0 | stdout: {"id": "/subscriptions/...
     ↳ captures: resource_group_id, group_name

 2   az aks create --resource-group {{.rg}}... [1,1]  —
     → exit 0 | stdout: {"id": "/subscriptions/...

 3   kubectl get pods -n {{.ns}}               [1,∞)  monitoring (unordered)
     → exit 0 | stdout: NAME         READY  STATUS...

 4   kubectl get svc -n {{.ns}}                [1,∞)  monitoring (unordered)
     → exit 0 | stdout: NAME         TYPE   CLUSTER-IP...

 5   helm status {{.release}}                  [1,1]  —
     → exit 0 | stdout: [file: testdata/helm_status.txt]
──────────────────────────────────────────────────────────

✓ All commands match allowlist
✓ No validation errors
```

---

## Entity: Signal Handler (Windows)

**Location**: `cmd/exec_windows.go` (new, build-tagged)

Not a data entity — behavioral change with build tags.

| Aspect              | Unix (`exec_unix.go`)         | Windows (`exec_windows.go`)          |
|---------------------|-------------------------------|--------------------------------------|
| Signal notification | `SIGINT`, `SIGTERM`           | `os.Interrupt` only                  |
| Termination action  | `Process.Signal(sig)`         | `Process.Kill()` on interrupt        |
| Graceful stop       | SIGTERM first, then SIGKILL   | Immediate kill (no graceful option)  |
| Grandchild cleanup  | Signal propagates to group    | Not guaranteed (documented limitation)|

---

## Relationships

```
Scenario
 └── Steps[]
      └── Response
           └── Capture map[string]string  ←── NEW
                    │
                    ▼
State
 └── Captures map[string]string  ←── NEW (accumulated across steps)
          │
          ▼
ReplayResponseWithTemplate()
 └── vars["capture"] = state.Captures  ←── injection point
          │
          ▼
template.Render(content, vars)
 └── {{ .capture.<id> }}  ←── template access
```
