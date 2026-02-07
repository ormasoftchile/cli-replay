# Data Model: P0 Critical Enhancements

**Feature Branch**: `005-p0-critical-enhancements`
**Date**: 2026-02-07

---

## Entity Changes

### 1. Match (extended)

**Package**: `internal/scenario`
**File**: `model.go`

| Field | Type | YAML Tag | Required | Description |
|-------|------|----------|----------|-------------|
| Argv | `[]string` | `argv` | Yes | Command argument vector to match (existing) |
| **Stdin** | `string` | `stdin,omitempty` | No | Expected stdin content. When set, actual stdin must match after trailing-newline normalization. When absent, stdin is ignored (backward compatible). |

**Validation Rules**:
- `Argv` must be non-empty (existing)
- `Stdin` has no validation constraints (empty string and absent are both valid)

---

### 2. Step (extended)

**Package**: `internal/scenario`
**File**: `model.go`

| Field | Type | YAML Tag | Required | Description |
|-------|------|----------|----------|-------------|
| Match | `Match` | `match` | Yes | Matching criteria (existing) |
| Respond | `Response` | `respond` | Yes | Canned response (existing) |
| **Calls** | `*CallBounds` | `calls,omitempty` | No | Invocation bounds. `nil` defaults to `{Min: 1, Max: 1}`. |
| When | `string` | `when,omitempty` | No | Conditional expression (existing) |

**Validation Rules**:
- Existing: match and respond are validated
- New: if `Calls` is non-nil, validate `Min >= 0`, `Max >= 1`, `Max >= Min`

**Helper Method**:
```
EffectiveCalls() → CallBounds
  If Calls is nil → return {Min: 1, Max: 1}
  Else → return *Calls
```

---

### 3. CallBounds (new)

**Package**: `internal/scenario`
**File**: `model.go`

| Field | Type | YAML Tag | Description |
|-------|------|----------|-------------|
| Min | `int` | `min` | Minimum required invocations. Default: 1 when omitted from YAML (via pointer nil check). |
| Max | `int` | `max` | Maximum allowed invocations. Default: 1 when omitted from YAML. |

**Validation Rules**:
- `Min >= 0`
- `Max >= 1` (a step that can never be called is invalid)
- `Max >= Min`

**YAML Defaults**: When only `min` or only `max` is specified in YAML:
- `calls: { min: 2 }` → `Min: 2, Max: 2` (min and max are the same if only min is given; enforced in validation or defaulting logic)
- `calls: { max: 5 }` → `Min: 0, Max: 5` (Go zero-value for int is 0; `min: 0` means "optional step")
- `calls: { min: 1, max: 5 }` → as specified

> **Design note**: When only `min` is provided and `max` is 0 (Go zero-value), default `max` to `min`. When only `max` is provided, `min` stays at 0. This provides intuitive YAML ergonomics.

---

### 4. Security (new)

**Package**: `internal/scenario`
**File**: `model.go`

| Field | Type | YAML Tag | Description |
|-------|------|----------|-------------|
| AllowedCommands | `[]string` | `allowed_commands,omitempty` | List of command base names that may be intercepted. Empty = no restriction. |

---

### 5. Meta (extended)

**Package**: `internal/scenario`
**File**: `model.go`

| Field | Type | YAML Tag | Required | Description |
|-------|------|----------|----------|-------------|
| Name | `string` | `name` | Yes | Scenario name (existing) |
| Description | `string` | `description,omitempty` | No | (existing) |
| Vars | `map[string]string` | `vars,omitempty` | No | Template variables (existing) |
| **Security** | `*Security` | `security,omitempty` | No | Security constraints. `nil` = no restrictions (backward compatible). |

---

### 6. State (extended)

**Package**: `internal/runner`
**File**: `state.go`

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| ScenarioPath | `string` | `scenario_path` | (existing) |
| ScenarioHash | `string` | `scenario_hash` | (existing) |
| CurrentStep | `int` | `current_step` | Index of the step currently accepting calls (existing) |
| TotalSteps | `int` | `total_steps` | (existing) |
| **StepCounts** | `[]int` | `step_counts,omitempty` | Per-step invocation counts. Replaces `ConsumedSteps`. |
| ConsumedSteps | `[]bool` | `consumed_steps,omitempty` | **Deprecated**: read-only for migration from older state files. Never written. |
| InterceptDir | `string` | `intercept_dir,omitempty` | (existing) |
| LastUpdated | `time.Time` | `last_updated` | (existing) |

**Migration**: On `ReadState`, if `StepCounts` is nil but `ConsumedSteps` is present, convert `true` → `1`, `false` → `0`, nil out `ConsumedSteps`.

**New Methods**:
- `IncrementStep(idx int)` — increments `StepCounts[idx]`; replaces `Advance()` for the call-count path
- `StepBudgetRemaining(idx, maxCalls int) int` — returns `max(0, maxCalls - StepCounts[idx])`
- `AllStepsMetMin(steps []scenario.Step) bool` — checks each step's count against `EffectiveCalls().Min`
- `IsStepConsumed(idx int) bool` — updated to check `StepCounts[idx] >= 1`

---

### 7. MismatchError (extended)

**Package**: `internal/runner`
**File**: `replay.go`

| Field | Type | Description |
|-------|------|-------------|
| Scenario | `string` | (existing) |
| StepIndex | `int` | (existing) |
| Expected | `[]string` | (existing) |
| Received | `[]string` | (existing) |
| **SoftAdvanced** | `bool` | True if soft-advance was attempted (current step met min, tried next step) |
| **NextStepIndex** | `int` | Index of the next step tried (only set when SoftAdvanced) |
| **NextExpected** | `[]string` | Expected argv of the next step (only set when SoftAdvanced) |

---

### 8. MatchDetail (new)

**Package**: `internal/matcher`
**File**: `argv.go`

| Field | Type | Description |
|-------|------|-------------|
| Matched | `bool` | Whether the element matched |
| Kind | `string` | `"literal"`, `"wildcard"`, or `"regex"` |
| Pattern | `string` | The regex pattern string (for regex kind), `"{{ .any }}"` (for wildcard), or empty (for literal) |
| FailReason | `string` | Human-readable explanation (e.g., `regex "^prod-.*" did not match "staging-app"`) |

**Function**: `ElementMatchDetail(pattern, value string) MatchDetail` — called only on mismatch for the divergence position. Not on the hot path.

---

### 9. RecordingEntry (extended)

**Package**: `internal/recorder`
**File**: `log.go`

| Field | Type | JSON Tag | Description |
|-------|------|----------|-------------|
| Timestamp | `string` | `timestamp` | (existing) |
| Argv | `[]string` | `argv` | (existing) |
| Exit | `int` | `exit` | (existing) |
| Stdout | `string` | `stdout` | (existing) |
| Stderr | `string` | `stderr` | (existing) |
| Encoding | `string` | `encoding,omitempty` | (existing) |
| **Stdin** | `string` | `stdin,omitempty` | Captured stdin content. Empty if no stdin was piped. |

---

## Relationship Diagram

```
Scenario
├── Meta
│   ├── Name, Description, Vars (existing)
│   └── *Security (new)
│       └── AllowedCommands []string
└── []Step
    ├── Match
    │   ├── Argv []string (existing)
    │   └── Stdin string (new)
    ├── Response (existing, unchanged)
    ├── *CallBounds (new)
    │   ├── Min int
    │   └── Max int
    └── When string (existing)

State
├── CurrentStep, TotalSteps (existing)
├── StepCounts []int (new, replaces ConsumedSteps)
├── ConsumedSteps []bool (deprecated, read-only migration)
└── InterceptDir (existing)

MismatchError
├── Scenario, StepIndex, Expected, Received (existing)
└── SoftAdvanced, NextStepIndex, NextExpected (new)

MatchDetail (new)
├── Matched, Kind, Pattern, FailReason
```

## State Transitions

### Step Lifecycle (with Call Counts)

```
                ┌─────────────┐
                │  WAITING     │  count = 0, count < max
                │  (current)   │◄──────────┐
                └──────┬───────┘           │
                       │ match             │ match (count < max)
                       ▼                   │
                ┌──────────────┐           │
                │  ACTIVE      │───────────┘
                │  count++     │
                └──────┬───────┘
                       │ count == max
                       ▼
                ┌──────────────┐
                │  EXHAUSTED   │  auto-advance to next step
                │              │
                └──────────────┘
```

### Soft Advance (Mismatch with Min Met)

```
  Command arrives → doesn't match step[N]
                         │
              ┌──────────┴──────────┐
              │ count < min          │ count >= min
              │                      │
              ▼                      ▼
        HARD MISMATCH          TRY step[N+1]
        (error: step N              │
         needs more calls)   ┌──────┴──────┐
                             │ match       │ no match
                             ▼             ▼
                        ADVANCE TO     MISMATCH
                        step[N+1]      (report both
                                        steps tried)
```
