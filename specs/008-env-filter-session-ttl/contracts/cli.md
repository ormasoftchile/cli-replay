# CLI Contract: Environment Variable Filtering & Session TTL

**Feature**: `008-env-filter-session-ttl`  
**Date**: 2026-02-07

## Modified Commands

### `cli-replay run <scenario.yaml>` — No CLI changes

TTL cleanup runs transparently at session startup. No new flags.

**New behavior**: If `meta.session.ttl` is configured, stale sessions are auto-cleaned before initializing the new session.  
**stderr output** (when cleanup occurs):  
```
cli-replay: cleaned 2 expired sessions
cli-replay: session initialized for "my-scenario" (3 steps, 2 commands)
```

---

### `cli-replay exec <scenario.yaml> -- <command>` — No CLI changes

Same TTL cleanup behavior as `run`.

---

### `cli-replay clean [scenario.yaml]` — New flags

**Current usage** (unchanged):
```
cli-replay clean                    # uses CLI_REPLAY_SCENARIO env
cli-replay clean scenario.yaml      # explicit path
```

**New flags**:

| Flag | Type | Default | Description | FR |
|------|------|---------|-------------|-----|
| `--ttl` | `duration` | `""` (none) | Only clean sessions older than this duration | FR-016 |
| `--recursive` | `bool` | `false` | Walk directory tree for `.cli-replay/` dirs | FR-020 |

**New usage patterns**:

```bash
# Clean only expired sessions for a specific scenario
cli-replay clean scenario.yaml --ttl 10m

# Clean all expired sessions recursively under current directory
cli-replay clean --ttl 10m --recursive .

# Clean all expired sessions recursively under a specific path
cli-replay clean --ttl 1h --recursive /path/to/scenarios
```

**Argument rules**:
- Without `--recursive`: Scenario path (positional or env) is required (existing behavior)
- With `--recursive`: Positional argument is the root path to walk (default: `.`)
- `--ttl` without `--recursive`: Applies to the single scenario's sessions
- `--recursive` without `--ttl`: ERROR — `--recursive` requires `--ttl` to prevent accidental deletion of all sessions

**Exit codes**:
| Code | Meaning |
|------|---------|
| 0 | Success (including "0 sessions cleaned") |
| 1 | Error (invalid args, path not found, etc.) |

**stderr output**:
```
# With --ttl, single scenario
cli-replay: cleaned 3 expired sessions for scenario.yaml

# With --recursive
cli-replay: scanned 12 directories, cleaned 5 expired sessions

# Nothing to clean
cli-replay: no expired sessions found
```

---

## Intercept Mode (shim invocation) — No CLI changes

When cli-replay is invoked as an intercept shim (e.g., as `kubectl`), TTL cleanup also runs before matching. The shim reads the scenario from `CLI_REPLAY_SCENARIO` env and applies the same TTL logic.

---

## YAML Schema Changes

### `meta.security.deny_env_vars` (new field)

```yaml
meta:
  security:
    deny_env_vars:             # NEW: list of glob patterns
      - "AWS_*"
      - "GITHUB_TOKEN"
      - "*_SECRET"
```

**JSON Schema addition** (in `security` definition):
```json
"deny_env_vars": {
  "type": "array",
  "description": "Deny-list of environment variable name patterns. Matching vars are replaced with empty strings in template rendering. Supports glob patterns (* wildcard).",
  "items": {
    "type": "string",
    "minLength": 1
  }
}
```

### `meta.session` (new section)

```yaml
meta:
  session:
    ttl: "5m"                  # NEW: Go duration string
```

**JSON Schema addition** (in `meta` definition):
```json
"session": {
  "type": "object",
  "description": "Session lifecycle configuration.",
  "additionalProperties": false,
  "properties": {
    "ttl": {
      "type": "string",
      "description": "Time-to-live for replay sessions. Sessions older than this are auto-cleaned. Go duration format (e.g., '5m', '1h', '30s').",
      "pattern": "^[0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h)+$"
    }
  }
}
```

---

## Environment Variables — No changes

No new environment variables are introduced. Existing variables:
- `CLI_REPLAY_SESSION` — session isolation (unchanged)
- `CLI_REPLAY_SCENARIO` — scenario path (unchanged)  
- `CLI_REPLAY_TRACE` — trace output (extended: now also traces denied env var substitutions per FR-010)
