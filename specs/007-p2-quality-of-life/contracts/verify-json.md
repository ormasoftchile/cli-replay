# Contract: Verify JSON Output

**Feature**: 007-p2-quality-of-life (FR-001, FR-002, FR-003, FR-007)  
**Date**: 2026-02-07

## Trigger

```bash
cli-replay verify scenario.yaml --format json
```

## Output Channel

**stdout** — structured JSON, suitable for piping to `jq` or file redirection.

## Schema

```json
{
  "scenario": "string — meta.name from scenario file",
  "session": "string — session ID or 'default'",
  "passed": "boolean — true if all steps met min bounds",
  "total_steps": "integer — count of flat leaf steps",
  "consumed_steps": "integer — steps invoked ≥ 1 time",
  "error": "string (optional) — present only on error (e.g., 'no state found')",
  "steps": [
    {
      "index": "integer — 0-based flat index",
      "label": "string — argv summary, prefixed with [group:<name>] for group steps",
      "group": "string (optional) — group name, only for group children",
      "call_count": "integer — times matched",
      "min": "integer — minimum required",
      "max": "integer — maximum allowed",
      "passed": "boolean — call_count >= min"
    }
  ]
}
```

## Example: All Steps Passed

```json
{
  "scenario": "deploy-app",
  "session": "default",
  "passed": true,
  "total_steps": 4,
  "consumed_steps": 4,
  "steps": [
    { "index": 0, "label": "git status", "call_count": 1, "min": 1, "max": 1, "passed": true },
    { "index": 1, "label": "[group:pre-flight] az account show", "group": "pre-flight", "call_count": 1, "min": 1, "max": 1, "passed": true },
    { "index": 2, "label": "[group:pre-flight] docker info", "group": "pre-flight", "call_count": 1, "min": 1, "max": 1, "passed": true },
    { "index": 3, "label": "kubectl apply -f app.yaml", "call_count": 1, "min": 1, "max": 1, "passed": true }
  ]
}
```

## Example: Incomplete Steps

```json
{
  "scenario": "deploy-app",
  "session": "default",
  "passed": false,
  "total_steps": 4,
  "consumed_steps": 2,
  "steps": [
    { "index": 0, "label": "git status", "call_count": 1, "min": 1, "max": 1, "passed": true },
    { "index": 1, "label": "[group:pre-flight] az account show", "group": "pre-flight", "call_count": 0, "min": 1, "max": 1, "passed": false },
    { "index": 2, "label": "[group:pre-flight] docker info", "group": "pre-flight", "call_count": 1, "min": 1, "max": 1, "passed": true },
    { "index": 3, "label": "kubectl apply -f app.yaml", "call_count": 0, "min": 1, "max": 1, "passed": false }
  ]
}
```

## Example: No State File

```json
{
  "scenario": "deploy-app",
  "session": "default",
  "passed": false,
  "total_steps": 0,
  "consumed_steps": 0,
  "error": "no state found",
  "steps": []
}
```

## Exit Codes

| Condition | Exit Code |
|-----------|-----------|
| All steps passed | 0 |
| Steps incomplete | 1 |
| No state file | 1 |
| Invalid format flag | non-zero (cobra error) |

## Behavior Notes

- JSON is compact (no pretty-print) by default. Users pipe to `jq .` for formatting.
- Single session only. `--session` selects which session; omitting uses default/latest.
- `--format` is case-insensitive: `json`, `JSON`, `Json` all work.
- When `--format json` is set, human-readable output to stderr is suppressed.
