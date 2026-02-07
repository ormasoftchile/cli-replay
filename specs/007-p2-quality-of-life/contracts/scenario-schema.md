# Contract: Scenario JSON Schema

**Feature**: 007-p2-quality-of-life (FR-009 through FR-014)  
**Date**: 2026-02-07

## File Location

```
schema/scenario.schema.json
```

## Schema Draft

JSON Schema **draft-07** (`http://json-schema.org/draft-07/schema#`)

## Schema Reference URL

```
https://raw.githubusercontent.com/<org>/cli-replay/main/schema/scenario.schema.json
```

## Usage in YAML Files

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/<org>/cli-replay/main/schema/scenario.schema.json
meta:
  name: my-scenario
steps:
  - match:
      argv: [kubectl, get, pods]
    respond:
      exit: 0
```

## Schema Structure

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://raw.githubusercontent.com/<org>/cli-replay/main/schema/scenario.schema.json",
  "title": "cli-replay Scenario",
  "description": "Schema for cli-replay scenario YAML files",
  "type": "object",
  "required": ["meta", "steps"],
  "additionalProperties": false,
  "definitions": { "..." },
  "properties": {
    "meta": { "$ref": "#/definitions/meta" },
    "steps": {
      "type": "array",
      "minItems": 1,
      "items": { "$ref": "#/definitions/step_element" }
    }
  }
}
```

## Definitions

### meta

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | yes | Scenario name (non-empty) |
| `description` | `string` | no | Human-readable description |
| `vars` | `object` (string→string) | no | Template variables |
| `security` | `$ref security` | no | Security constraints |

### security

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `allowed_commands` | `array` of `string` | no | Allowlist of interceptable commands |

### step_element

`oneOf`:
- `$ref step` — a leaf step (has `match` + `respond`)
- `$ref group_wrapper` — a group (has `group` key)

### step

| Field | Type | Required | Default | Constraints | Description |
|-------|------|----------|---------|-------------|-------------|
| `match` | `$ref match` | yes | — | — | Command matching criteria |
| `respond` | `$ref respond` | yes | — | — | Response to return |
| `calls` | `$ref calls` | no | `{min:1, max:1}` | — | Invocation bounds |
| `when` | `string` | no | — | — | Conditional expression |

### match

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| `argv` | `array` of `string` | yes | `minItems: 1` | Command + arguments to match |
| `stdin` | `string` | no | — | Expected stdin content |

### respond

| Field | Type | Required | Default | Constraints | Description |
|-------|------|----------|---------|-------------|-------------|
| `exit` | `integer` | no | `0` | `0 ≤ exit ≤ 255` | Exit code |
| `stdout` | `string` | no | — | mutex with `stdout_file` | Inline stdout |
| `stderr` | `string` | no | — | mutex with `stderr_file` | Inline stderr |
| `stdout_file` | `string` | no | — | mutex with `stdout` | File path for stdout |
| `stderr_file` | `string` | no | — | mutex with `stderr` | File path for stderr |
| `delay` | `string` | no | — | Go duration format | Response delay |

**Mutual exclusivity** (via `allOf` + `if/then`):
```json
"allOf": [
  {
    "if": { "required": ["stdout"] },
    "then": { "properties": { "stdout_file": false } }
  },
  {
    "if": { "required": ["stderr"] },
    "then": { "properties": { "stderr_file": false } }
  }
]
```

### calls

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| `min` | `integer` | no | `≥ 0` | Minimum invocations |
| `max` | `integer` | no | `≥ 1` | Maximum invocations |

**Note**: `min > max` is a semantic error caught by Go validation, not enforceable in JSON Schema (cross-field comparison requires `if/then` which is fragile for numeric ranges).

### group_wrapper

A wrapper object with a single `group` key:

```json
{
  "type": "object",
  "required": ["group"],
  "additionalProperties": false,
  "properties": {
    "group": { "$ref": "#/definitions/step_group" }
  }
}
```

### step_group

| Field | Type | Required | Default | Constraints | Description |
|-------|------|----------|---------|-------------|-------------|
| `mode` | `string` | yes | — | `enum: ["unordered"]` | Matching mode |
| `name` | `string` | no | `"group-N"` | — | Group name for labeling |
| `steps` | `array` of `$ref step` | yes | — | `minItems: 1`; items are leaf steps only | Child steps |

## Description Annotations

Every field includes both `description` (plain text) and `markdownDescription` (VS Code rich hover):

```json
{
  "exit": {
    "type": "integer",
    "minimum": 0,
    "maximum": 255,
    "default": 0,
    "description": "Process exit code (0-255). Defaults to 0.",
    "markdownDescription": "Process exit code (`0`–`255`). Defaults to `0`."
  }
}
```

## Validation Coverage

| Constraint | Schema Enforcement | Runtime Enforcement |
|-----------|-------------------|-------------------|
| Non-empty `meta.name` | `minLength: 1` | `Meta.Validate()` |
| Non-empty `argv` | `minItems: 1` | `Match.Validate()` |
| `exit` range 0–255 | `minimum`/`maximum` | `Response.Validate()` |
| `stdout`/`stdout_file` mutex | `if/then` | `Response.Validate()` |
| `stderr`/`stderr_file` mutex | `if/then` | `Response.Validate()` |
| `calls.min ≥ 0` | `minimum: 0` | `CallBounds.Validate()` |
| `calls.max ≥ 1` | `minimum: 1` | `CallBounds.Validate()` |
| `min ≤ max` | ❌ (cross-field) | `CallBounds.Validate()` |
| Unknown fields | `additionalProperties: false` | `KnownFields(true)` |
| Nested groups | ❌ (recursive `oneOf` limits) | `Scenario.Validate()` |
| Empty group | `minItems: 1` | `StepGroup.Validate()` |
| Unknown `mode` | `enum: ["unordered"]` | `StepGroup.Validate()` |
