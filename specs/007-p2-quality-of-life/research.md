# Research: P2 Quality of Life Enhancements

**Feature**: 007-p2-quality-of-life  
**Date**: 2026-02-07  
**Status**: Complete

## R1: JUnit XML Output Format

**Decision**: Use `<testsuites>` as root, one `<testsuite>` per scenario, one `<testcase>` per flat step. Implement with Go's `encoding/xml` stdlib.

**Rationale**: The `<testsuites>` wrapper is used by both Go test tools (gotestsum, go-junit-report) and is consumed by every major CI platform (Jenkins, Azure DevOps, GitHub Actions). Using `encoding/xml` avoids external dependencies and handles XML escaping automatically.

**Key Facts**:

| Element | Key Attributes | Notes |
|---------|---------------|-------|
| `<testsuites>` (root) | `name`, `tests`, `failures`, `errors`, `time` | Always emit even for single scenario |
| `<testsuite>` | `name` (req), `tests` (req), `failures` (req), `errors` (req), `time`, `timestamp` | One per scenario |
| `<testcase>` | `name` (req), `classname` (req), `time` | One per flat step |
| `<failure>` | `message`, `type` | Child of `<testcase>`; text content has detail |
| `<skipped>` | `message` | Used for optional steps (`min: 0`) that weren't called |

**cli-replay Mapping**:
- `testsuite.name` → scenario `meta.name`
- `testcase.classname` → scenario file path (grouping key for CI dashboards)
- `testcase.name` → step label, e.g., `step[0]: kubectl get pods` or `[group:pre-flight] step[1]: az account show`
- `failure.message` → "called 0 times, minimum 1 required"
- Time format: `%.3f` seconds (matches go-junit-report convention)

**Encoding Considerations**:
- `encoding/xml.Marshal` auto-escapes `<`, `>`, `&`, `"`, `'`
- Filter illegal XML characters (`\x00`, `\x0B`, `\x0C`) before marshaling — replace with U+FFFD
- Emit `<?xml version="1.0" encoding="UTF-8"?>` header

**Alternatives Considered**:
- Template-based XML generation → Rejected: fragile, no auto-escaping, harder to test
- Third-party JUnit library → Rejected: no deps policy; `encoding/xml` is sufficient

## R2: JSON Schema for Scenario Files

**Decision**: Use JSON Schema draft-07 with `definitions`/`$ref` for reusable blocks. All properties defined at top level for autocompletion; mutual exclusivity enforced via `if/then` in `allOf`.

**Rationale**: Draft-07 is the sweet spot — supports `if/then/else` (needed for mutual exclusivity), has widest IDE support (VS Code, JetBrains, Neovim), and is what the JSON Schema Store predominantly uses. Newer drafts (2019-09, 2020-12) have incomplete support in redhat.vscode-yaml.

**Key Design Decisions**:

| Concern | Approach | Why |
|---------|----------|-----|
| Draft version | draft-07 | Widest IDE support; `if/then/else` available |
| Mutual exclusivity | `if/then` with `properties: false` in `allOf` | Good error messages; doesn't confuse autocompletion |
| Autocompletion | All properties in top-level `properties` block | `oneOf`/`if/then` branches alone don't drive completions |
| Reusable definitions | `definitions` + `$ref` | Fully supported by yaml-language-server |
| Descriptions | `description` + `markdownDescription` | `description` for compatibility; `markdownDescription` for rich hover |
| Schema reference | `# yaml-language-server: $schema=<url>` modeline | Highest priority in yaml-language-server resolution |

**Mutual Exclusivity Pattern** (stdout vs stdout_file):
```json
{
  "properties": {
    "stdout": { "type": "string", "description": "..." },
    "stdout_file": { "type": "string", "description": "..." }
  },
  "allOf": [
    {
      "if": { "required": ["stdout"] },
      "then": { "properties": { "stdout_file": false } }
    }
  ]
}
```

**Alternatives Considered**:
- Draft 2019-09 → Rejected: brand-new support in yaml-language-server, too risky
- `oneOf` for exclusivity → Rejected: produces duplicate autocompletion suggestions
- External schema hosting (Schema Store) → Rejected: raw GitHub URL is zero-infrastructure

## R3: YAML Union Type for Steps

**Decision**: Wrapper struct `StepElement` with custom `UnmarshalYAML(*yaml.Node)` that dispatches on presence of `group` key vs `match` key. Provide `FlatSteps()` bridge for migration.

**Rationale**: This approach is type-safe at compile time, works naturally with yaml.v3's Node-based custom unmarshaling, supports recursive groups (even though nesting is rejected by validation), and has the lowest migration cost via the `FlatSteps()` helper.

**Model Design**:
```go
type Scenario struct {
    Meta  Meta          `yaml:"meta"`
    Steps []StepElement `yaml:"steps"`
}

type StepElement struct {
    Step  *Step      // set when YAML has "match"
    Group *StepGroup // set when YAML has "group"
}

type StepGroup struct {
    Mode  string        `yaml:"mode"`
    Name  string        `yaml:"name,omitempty"`
    Steps []StepElement `yaml:"steps"`
}
```

**UnmarshalYAML Strategy**: Scan `yaml.Node.Content` for a key named `group`. If found, decode as `StepGroup` wrapper. Otherwise, decode as `Step`.

**Migration via FlatSteps()**:
```go
func (s *Scenario) FlatSteps() []Step { /* expand groups inline */ }
```

Existing consumers that iterate `scn.Steps` change to `scn.FlatSteps()` — one-line change per call site.

**Gotchas**:
- `KnownFields(true)` does NOT propagate into custom `UnmarshalYAML` — inner decode creates fresh context. Rely on `Validate()` methods instead.
- Need custom `MarshalYAML` on `StepElement` for round-trip (recorder's `yaml.Encoder.Encode` uses Marshal)
- `StepElement.Validate()` must enforce exactly one of `Step`/`Group` is non-nil

**Alternatives Considered**:
- Raw `yaml.Node` field + manual dispatch → Rejected: more code, same result, loses `KnownFields` on outer struct
- Interface type with type switch → Rejected: yaml.v3 cannot unmarshal into interface types
- Single `Step` struct with optional `Group *StepGroup` field → Simpler but couples step and group in one type; unclear `match`/`respond` when `group` is set. Considered but rejected for semantic clarity.

## R4: Unordered Group Replay Engine

**Decision**: Keep `StepCounts []int` flat (group children get contiguous indices). Add `GroupRange{Start, End}` helper derived from scenario structure. Match within a group via linear scan of all unconsumed group steps.

**Rationale**: Flat StepCounts preserves backward compatibility, simplifies state serialization, and makes verification trivially correct (existing `AllStepsMetMin` loop works unchanged). Group ranges are computed from the scenario, not stored in state.

**State Changes**:
- `State.ActiveGroup *int` — NEW: index into `GroupRanges()`. `nil` when outside a group. Optional/derived field for clarity.
- `State.TotalSteps` = `len(FlatSteps())` — unchanged semantic

**Flat Index Mapping**:
```
YAML structure:              Flat index:
  step 0  (leaf)               0
  step 1  (group, 2 children)
    group-child 0              1
    group-child 1              2
  step 2  (leaf)               3
```

**Matching Algorithm**:

1. Compute `groupRange := findGroupContaining(groupRanges, state.CurrentStep)`
2. If `groupRange == nil` → **ordered path** (existing logic, unchanged)
3. If inside group:
   - Scan all steps in `[groupRange.Start, groupRange.End)` with remaining budget
   - First match wins (documented; deterministic by YAML declaration order)
   - On match: increment `StepCounts[matchedIdx]`; if all maxes hit → advance past group
   - On no match + all mins met: soft-advance past group, retry at `groupRange.End`
   - On no match + mins NOT met: `GroupMismatchError` listing all unconsumed candidates

**Group Exhaustion**:

| Condition | Action |
|-----------|--------|
| All min bounds met | Barrier cleared; non-matching command advances past group |
| All max bounds hit | Force-advance past group; no more commands can match here |
| Some mins not met, no match | `GroupMismatchError` with candidate list |

**Verification**: No changes to `AllStepsMetMin` logic — flat iteration already covers group children. Callers pass `scn.FlatSteps()`.

**Error Reporting**: New `GroupMismatchError` type lists all unconsumed group steps as potential matches per FR-023.

**Edge Cases Resolved**:

| Edge Case | Behavior |
|-----------|----------|
| All group steps `min: 0` | Group barrier immediately satisfiable; first non-match advances past |
| Group as first step | `CurrentStep = 0`, engine enters group path immediately |
| Group as last step | After exhaustion `CurrentStep = TotalSteps`, `IsComplete()` true |
| Adjacent groups | Each group has separate `GroupRange`; advancing past one enters next |
| Soft-advance from ordered INTO group | Lands on `GroupRange.Start`, switches to group-scan logic |
| Duplicate argv in group | First-match-wins; first step fills up, then second starts matching |

**Performance**: O(N) scan per command within a group. Groups are 3–10 steps typically. Negligible vs process spawn overhead.

**Alternatives Considered**:
- Nested StepCounts (map-based tracking) → Rejected: breaks backward compat, complicates serialization
- Hash-based matching within groups → Rejected: wildcards/regex patterns can't be hashed; linear scan is fast enough
- Best-match heuristic (fewest wildcards) → Rejected: adds complexity with no clear benefit; YAML order is deterministic
