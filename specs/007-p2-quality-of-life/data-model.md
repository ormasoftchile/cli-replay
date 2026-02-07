# Data Model: P2 Quality of Life Enhancements

**Feature**: 007-p2-quality-of-life  
**Date**: 2026-02-07  
**Status**: Complete

## Entity Diagram

```
Scenario
├── Meta { Name, Description, Vars, Security }
└── Steps []StepElement
        ├── Step { Match, Respond, Calls, When }    ← leaf step
        └── StepGroup { Mode, Name, Steps }          ← group container
               └── Steps []StepElement               ← nested (no groups allowed)

VerifyResult
├── Scenario (string)
├── Session (string)
├── Passed (bool)
├── TotalSteps (int)
├── ConsumedSteps (int)
├── Error (string, optional)
└── Steps []StepResult
        ├── Index (int)
        ├── Label (string)
        ├── Group (string, optional)
        ├── CallCount (int)
        ├── Min (int)
        ├── Max (int)
        └── Passed (bool)
```

## New & Modified Types

### StepElement (NEW)

Union type — exactly one field is non-nil after unmarshaling.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Step` | `*Step` | mutex | Leaf step (set when YAML has `match`) |
| `Group` | `*StepGroup` | mutex | Group container (set when YAML has `group`) |

**Invariant**: Exactly one of `Step` or `Group` must be non-nil. Validated in `StepElement.Validate()`.

**YAML dispatch**: Custom `UnmarshalYAML(*yaml.Node)` scans mapping keys. Presence of `group` key → decode as group. Otherwise → decode as step.

### StepGroup (NEW)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `Mode` | `string` | yes | — | Matching mode. Only `"unordered"` accepted. |
| `Name` | `string` | no | `"group-N"` | Human-readable name. Auto-generated as `group-1`, `group-2`, etc. (1-based, by declaration order) when omitted. |
| `Steps` | `[]StepElement` | yes | — | Child steps. Must have ≥ 1 element. Children must all be leaf steps (no nested groups). |

**Validation rules**:
- `Mode` must be `"unordered"` — unknown values rejected
- `Steps` must be non-empty — empty group rejected
- Children must all have `Group == nil` — nested groups rejected
- Each child step validated normally (match, respond, calls, when)

### VerifyResult (NEW)

Structured output of a verification run. Serialized to JSON or JUnit XML.

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| `Scenario` | `string` | `scenario` | Scenario `meta.name` |
| `Session` | `string` | `session` | Session identifier, or `"default"` |
| `Passed` | `bool` | `passed` | Overall pass/fail |
| `TotalSteps` | `int` | `total_steps` | Count of flat leaf steps |
| `ConsumedSteps` | `int` | `consumed_steps` | Steps invoked ≥ 1 time |
| `Error` | `string` | `error,omitempty` | Error message (e.g., "no state found") |
| `Steps` | `[]StepResult` | `steps` | Per-step detail |

### StepResult (NEW)

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| `Index` | `int` | `index` | 0-based flat index |
| `Label` | `string` | `label` | Argv summary, with `[group:<name>]` prefix for group steps |
| `Group` | `string` | `group,omitempty` | Group name (only for group children) |
| `CallCount` | `int` | `call_count` | Times this step was matched |
| `Min` | `int` | `min` | Minimum required calls |
| `Max` | `int` | `max` | Maximum allowed calls |
| `Passed` | `bool` | `passed` | `call_count >= min` |

### GroupRange (NEW, internal helper)

Computed from scenario structure, not serialized.

| Field | Type | Description |
|-------|------|-------------|
| `Start` | `int` | Inclusive flat index of first group child |
| `End` | `int` | Exclusive flat index (`Start + len(group.Steps)`) |
| `Name` | `string` | Group name (resolved, never empty) |
| `TopIndex` | `int` | Index of the group in the top-level `Steps` array |

## Modified Types

### Scenario

| Field | Before | After | Notes |
|-------|--------|-------|-------|
| `Steps` | `[]Step` | `[]StepElement` | Union type; existing YAML with only leaf steps still works |

**New methods**:
- `FlatSteps() []Step` — expands groups inline, returns all leaf steps
- `GroupRanges() []GroupRange` — returns flat-index ranges for all groups

### State

| Field | Before | After | Notes |
|-------|--------|-------|-------|
| `ActiveGroup` | (absent) | `*int` (optional) | Index into `GroupRanges()`; `nil` when outside a group |

`StepCounts`, `TotalSteps`, `CurrentStep` semantics unchanged. Group children occupy contiguous flat indices in `StepCounts`.

### GroupMismatchError (NEW)

| Field | Type | Description |
|-------|------|-------------|
| `Scenario` | `string` | Scenario name |
| `GroupName` | `string` | Group name |
| `GroupIndex` | `int` | Index in `GroupRanges()` |
| `Candidates` | `[]int` | Flat indices of unconsumed group steps |
| `CandidateArgv` | `[][]string` | Argv of each candidate |
| `Received` | `[]string` | The unmatched command |

## Flat Index Mapping Example

```yaml
steps:
  - match: { argv: [git, status] }           # flat index 0
    respond: { exit: 0 }
  - group:                                     # group-1 (auto-named)
      mode: unordered
      steps:
        - match: { argv: [az, account, show] } # flat index 1
          respond: { exit: 0 }
        - match: { argv: [docker, info] }      # flat index 2
          respond: { exit: 0 }
  - match: { argv: [kubectl, apply, -f, app.yaml] }  # flat index 3
    respond: { exit: 0 }
```

- `TotalSteps = 4` (4 leaf steps)
- `StepCounts = [0, 0, 0, 0]` (flat)
- `GroupRanges = [{Start: 1, End: 3, Name: "group-1", TopIndex: 1}]`
- Barrier: steps at indices 1 and 2 must both meet min bounds before index 3 is eligible

## JSON Schema Entity

The schema file (`schema/scenario.schema.json`) is a static artifact, not a Go type. Its structure mirrors the data model:

```
scenario.schema.json
├── $schema: "http://json-schema.org/draft-07/schema#"
├── definitions/
│   ├── meta_block          → Meta type fields
│   ├── security_block      → Security type fields
│   ├── step_block          → Step type fields (match, respond, calls, when)
│   ├── match_block         → Match type fields (argv, stdin)
│   ├── respond_block       → Response type fields (exit, stdout, stderr, stdout_file, stderr_file, delay)
│   ├── calls_block         → CallBounds type fields (min, max)
│   └── step_group_block    → StepGroup type fields (mode, name, steps)
├── properties/
│   ├── meta: $ref meta_block
│   └── steps: array of oneOf [step_block, group_wrapper]
└── allOf/
    └── mutual exclusivity rules (stdout/stdout_file, stderr/stderr_file)
```

## Relationships

```
Scenario 1──* StepElement
StepElement ──1 Step         (XOR)
StepElement ──1 StepGroup    (XOR)
StepGroup 1──* StepElement   (leaf only, no nesting)

VerifyResult 1──* StepResult

State tracks flat indices ←→ FlatSteps() mapping
GroupRange is computed from Scenario, not persisted in State
```
