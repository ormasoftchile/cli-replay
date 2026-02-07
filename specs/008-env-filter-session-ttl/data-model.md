# Data Model: Environment Variable Filtering & Session TTL

**Feature**: `008-env-filter-session-ttl`  
**Date**: 2026-02-07

## Entity Changes

### Modified: `Security` (existing)

**Current** (model.go):
```
Security {
  AllowedCommands []string  yaml:"allowed_commands,omitempty"
}
```

**Proposed**:
```
Security {
  AllowedCommands []string  yaml:"allowed_commands,omitempty"
  DenyEnvVars     []string  yaml:"deny_env_vars,omitempty"     // NEW (FR-001)
}
```

**Fields**:
| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| `AllowedCommands` | `[]string` | `allowed_commands` | No | Existing: command allowlist for interception |
| `DenyEnvVars` | `[]string` | `deny_env_vars` | No | NEW: glob patterns of env vars to deny in templates |

**Validation** (FR-009):
- Each entry in `DenyEnvVars` must be a non-empty string
- Invalid: `deny_env_vars: [""]` → validation error

**Exemptions** (FR-007):
- `CLI_REPLAY_SESSION`, `CLI_REPLAY_SCENARIO`, `CLI_REPLAY_RECORDING_LOG`, `CLI_REPLAY_SHIM_DIR` are never filtered regardless of patterns

---

### New: `Session` (new section under Meta)

**Proposed**:
```
Session {
  TTL string  yaml:"ttl,omitempty"  // e.g., "5m", "1h"
}
```

**Fields**:
| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| `TTL` | `string` | `ttl` | No | Go duration string for session expiry (FR-011) |

**Validation** (FR-012):
- If non-empty, must be parseable by `time.ParseDuration`
- Must be positive (> 0)
- Invalid: `ttl: "never"` → validation error
- Invalid: `ttl: "-5m"` → validation error

---

### Modified: `Meta` (existing)

**Current**:
```
Meta {
  Name        string
  Description string
  Vars        map[string]string
  Security    *Security
}
```

**Proposed**:
```
Meta {
  Name        string
  Description string
  Vars        map[string]string
  Security    *Security
  Session     *Session             // NEW (FR-011)
}
```

---

### Unchanged: `State` (existing)

The `State` struct already contains all fields needed for TTL:
- `LastUpdated time.Time` — used for TTL expiry calculation
- `InterceptDir string` — used to locate associated intercept directory for cleanup

No changes required to the state model.

---

## YAML Examples

### Minimal (backward-compatible — no new fields)
```yaml
meta:
  name: basic-scenario
steps:
  - match:
      argv: [kubectl, get, pods]
    respond:
      stdout: "pod-1  Running\n"
```

### With deny_env_vars only
```yaml
meta:
  name: untrusted-tsg
  security:
    allowed_commands: [kubectl, az]
    deny_env_vars:
      - "AWS_*"
      - "GITHUB_TOKEN"
      - "*_SECRET"
  vars:
    cluster: "dev"
steps:
  - match:
      argv: [kubectl, get, pods]
    respond:
      stdout: "cluster={{ .cluster }}\n"
```

### With session TTL only
```yaml
meta:
  name: ci-pipeline-test
  session:
    ttl: "5m"
steps:
  - match:
      argv: [terraform, plan]
    respond:
      stdout: "No changes.\n"
```

### With both deny_env_vars and session TTL
```yaml
meta:
  name: secure-ci-test
  security:
    allowed_commands: [az]
    deny_env_vars: ["*"]
  session:
    ttl: "10m"
  vars:
    region: "eastus2"
steps:
  - match:
      argv: [az, account, show]
    respond:
      stdout: "region={{ .region }}\n"
```

## State Transitions

### TTL Cleanup Flow (at replay startup)

```
replay begins
  │
  ├─ meta.session.ttl configured?
  │   ├─ NO → skip TTL cleanup, proceed normally
  │   └─ YES → scan .cli-replay/ for cli-replay-*.state files
  │       │
  │       ├─ for each state file:
  │       │   ├─ parse JSON, read last_updated
  │       │   ├─ now - last_updated > ttl?
  │       │   │   ├─ NO → skip (active session)
  │       │   │   └─ YES → remove state file + intercept_dir
  │       │   └─ last_updated in future? → warn to stderr, skip
  │       │
  │       └─ log "cli-replay: cleaned N expired sessions" if N > 0
  │
  └─ proceed with normal replay (auto-initialize if needed)
```

### deny_env_vars Filter Flow (at template render time)

```
MergeVars(meta.vars, denyPatterns) called
  │
  ├─ copy meta.vars to result map
  │
  ├─ for each key in result:
  │   ├─ env override exists?
  │   │   ├─ NO → keep meta.vars value
  │   │   └─ YES → is key matched by any deny pattern?
  │   │       ├─ is key a CLI_REPLAY_* internal var? → allow (FR-007)
  │   │       ├─ path.Match(pattern, key) → matched?
  │   │       │   ├─ YES → suppress env override, keep meta.vars value
  │   │       │   │        (empty string if no meta.vars entry exists)
  │   │       │   │        emit trace if CLI_REPLAY_TRACE=1 (FR-010)
  │   │       │   └─ NO → allow env override
  │   │       └─ next pattern
  │   └─ next key
  │
  └─ return filtered map → Render(template, filteredMap)
```
