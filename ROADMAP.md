# cli-replay — Roadmap

Ideas extracted from external review analysis, categorized by priority and feasibility.

> **Status key:** 🔴 Not started · 🟡 Partially exists · 🟢 Already implemented

---

## P0 — Critical: Blockers for Real-World Adoption

### 1. stdin Support 🔴

**Problem:** Many CLI workflows pipe data via stdin (`kubectl apply -f -`, `terraform plan -`). Without stdin matching, these workflows can't be tested.

**Proposed model:**
```yaml
steps:
  - match:
      argv: ["kubectl", "apply", "-f", "-"]
      stdin: |
        apiVersion: v1
        kind: Pod
    respond:
      exit: 0
      stdout: "pod/my-pod created"
```

**Analysis:**
- Requires changes to `scenario.Match` (add `Stdin` field), the shim/symlink interception layer, and `matcher.ArgvMatch`
- Stdin matching could be exact, contains, or regex — start with exact match
- The shim must capture stdin *before* the intercepted binary consumes it
- **Complexity: Medium** — shim changes are platform-specific (bash shim vs `.cmd` wrapper)
- **Impact: High** — unblocks a large class of pipe-based workflows

---

### 2. Call Count Bounds (Repeat Steps) 🔴

**Problem:** Strict one-shot consumption breaks for retry/polling patterns (e.g., `kubectl get pods` in a loop until status changes). Full unordered mode is overkill; bounded repetition is the right primitive.

**Proposed model:**
```yaml
steps:
  - match:
      argv: ["kubectl", "get", "pods", "-n", "prod"]
    respond:
      exit: 0
      stdout: "..."
    calls:
      min: 1
      max: 10  # Allow up to 10 invocations of this step
```

**Analysis:**
- Add `Calls` struct to `scenario.Step` with `Min`/`Max` fields
- State tracking needs per-step invocation counts (not just `ConsumedSteps []bool`)
- A step with remaining call budget stays "current" instead of advancing
- Verify must check `min` was reached for each step
- **Complexity: Medium** — state model changes ripple through replay, verify, and state
- **Impact: High** — enables testing of polling, retry, and convergence-loop patterns

---

### 3. Enhanced Mismatch Diagnostics 🟡

**Problem:** When a command doesn't match, the current error is terse (`argv mismatch at step N`). Debugging requires manually comparing expected vs. actual.

**What exists:** `MismatchError` in [internal/runner/replay.go](internal/runner/replay.go) captures `Expected` and `Received` slices but formats minimally.

**Proposed improvement:**
```
cli-replay: step 2 mismatch
  expected: ["kubectl", "get", "pods"]
  received: ["kubectl", "get", "deployments"]
                                ^^^^^^^^^^^
  diff: argv[2] — expected "pods", got "deployments"
```

**Analysis:**
- Extend `MismatchError.Error()` to produce per-element diff
- Highlight first divergence point
- Show which argument template patterns were evaluated
- **Complexity: Low** — isolated to error formatting
- **Impact: High** — dramatically improves debugging experience

---

### 4. Security Allowlist for CI 🔴

**Problem:** Scenarios manipulate PATH, which is a trust boundary. In CI pipelines, a malicious scenario could intercept arbitrary binaries.

**Proposed model:**
```yaml
meta:
  security:
    allowed_commands: ["kubectl", "az", "aws"]
```

**Analysis:**
- Add optional `Security` struct to `scenario.Meta`
- `run` command validates that all intercepted commands (from `argv[0]`) are in the allowlist
- If `allowed_commands` is empty/absent, no restriction (backward compatible)
- Could also support a CLI flag `--allowed-commands` for org-level enforcement
- **Complexity: Low** — validation-only, no runtime behavior changes
- **Impact: Medium** — important for enterprise/CI trust, but not blocking basic usage

---

## P1 — High Priority: CI/CD Requirements

### 5. Sub-process Execution Mode (`exec`) 🔴

**Problem:** The `eval "$(cli-replay run ...)"` pattern modifies the caller's environment and risks pollution if the user forgets `cli-replay clean`. An `exec` mode would run the target command in an isolated child process with automatic cleanup.

**Proposed UX:**
```bash
cli-replay exec scenario.yaml -- ./deploy.sh
```

**Analysis:**
- New `exec` command that: sets up intercept dir, spawns child process with modified env, waits for exit, auto-cleans (intercept dir + state), auto-verifies completion
- Eliminates the need for separate `run` / `verify` / `clean` steps
- Most CI users would prefer this over the eval pattern
- **Complexity: Medium** — subprocess management, signal forwarding, cleanup-on-exit
- **Impact: High** — dramatically simplifies CI usage and prevents leaked state

---

### 6. Session Isolation for Parallel Tests 🟢

**What exists:** Already implemented. `CLI_REPLAY_SESSION` env var + `StateFilePathWithSession()` produce unique state files per session. The `run` command auto-generates a random session ID.

**Remaining gap:** The `verify` and `clean` commands use `StateFilePath()` (no session), so they won't find session-specific state unless `CLI_REPLAY_SESSION` is set in the env.

**Fix:** Ensure `verify` and `clean` respect `CLI_REPLAY_SESSION` from the environment (should already work since they inherit the eval'd env, but verify in tests).

- **Complexity: Low** — testing/validation only
- **Impact: Medium** — confidence for parallel CI

---

### 7. Signal-Trap Auto-Cleanup 🔴

**Problem:** If a script using `eval "$(cli-replay run ...)"` is interrupted (Ctrl+C, SIGTERM), the intercept directory and state file are leaked.

**Proposed approach:**
- The `run` command's shell output could include a trap:
  ```bash
  trap 'cli-replay clean scenario.yaml' EXIT INT TERM
  ```
- Or, better: the `exec` mode (idea #5) handles this internally

**Analysis:**
- Adding a trap to the eval output is simple but shell-specific
- The exec mode is the clean long-term solution
- **Complexity: Low** (trap) / **Medium** (exec mode handles it)
- **Impact: Medium** — prevents CI resource leaks

---

## P2 — Medium Priority: Quality of Life

### 8. Step Groups (Unordered Blocks) 🔴

**Problem:** Some workflows have a set of pre-checks that can run in any order (e.g., checking node status and account status before deploying). Strict ordering forces a single order.

**Proposed model:**
```yaml
steps:
  - group:
      mode: unordered
      steps:
        - match: { argv: ["kubectl", "get", "nodes"] }
          respond: { exit: 0 }
        - match: { argv: ["az", "account", "show"] }
          respond: { exit: 0 }
  - match: { argv: ["kubectl", "apply", "-f", "app.yaml"] }
    respond: { exit: 0 }
```

**Analysis:**
- Significant model change: `Step` becomes a union type (single step OR group)
- Replay engine needs to try all unconsumed steps within a group
- State tracking becomes per-step within groups
- The `When` field (conditional steps) already hints at this direction
- **Complexity: High** — touches model, loader, matcher, state, replay, and verify
- **Impact: Medium** — useful but strict ordering is the stated design philosophy and covers most TSG use cases

---

### 9. Machine-Readable Output (JSON / JUnit) 🔴

**Problem:** CI systems need structured output. The current `verify` command writes human-readable text to stderr.

**Proposed UX:**
```bash
cli-replay verify scenario.yaml --format json
cli-replay verify scenario.yaml --format junit > results.xml
```

**Analysis:**
- Add `--format` flag to `verify` (default: `text`)
- JSON output: structured object with scenario name, step statuses, pass/fail
- JUnit XML: standard format consumed by GitHub Actions, Azure DevOps, Jenkins
- **Complexity: Low-Medium** — formatting only, no logic changes
- **Impact: Medium** — valuable for CI integration and test dashboards

---

### 10. JSON Schema for Scenario Files 🔴

**Problem:** No IDE autocompletion or inline validation for scenario YAML files.

**Proposed approach:**
- Publish a JSON Schema for the scenario YAML format
- Users add a schema comment or configure VS Code `yaml.schemas` setting
- Schema validates field names, types, mutual exclusivity (`stdout` vs `stdout_file`), and ranges (`exit: 0-255`)

**Analysis:**
- Generate schema from the Go structs or write manually
- Publish alongside releases or in the repo
- **Complexity: Low** — one-time authoring effort
- **Impact: Medium** — great for onboarding and preventing typos in scenarios

---

## Summary Matrix

| # | Idea | Priority | Complexity | Impact | Status |
|---|------|----------|------------|--------|--------|
| 1 | stdin support | P0 | Medium | High | 🔴 |
| 2 | Call count bounds | P0 | Medium | High | 🔴 |
| 3 | Mismatch diagnostics | P0 | Low | High | 🟡 |
| 4 | Security allowlist | P0 | Low | Medium | 🔴 |
| 5 | `exec` sub-process mode | P1 | Medium | High | 🔴 |
| 6 | Session isolation | P1 | Low | Medium | 🟢 |
| 7 | Signal-trap cleanup | P1 | Low | Medium | 🔴 |
| 8 | Step groups (unordered) | P2 | High | Medium | 🔴 |
| 9 | JSON / JUnit output | P2 | Low-Med | Medium | 🔴 |
| 10 | JSON Schema | P2 | Low | Medium | 🔴 |

### Recommended implementation order

1. **Mismatch diagnostics** (#3) — low effort, high debugging value, good first win
2. **Security allowlist** (#4) — low effort, good for CI trust story
3. **Signal-trap cleanup** (#7) — low effort, prevents leaked state
4. **`exec` mode** (#5) — medium effort, biggest UX improvement for CI
5. **Call count bounds** (#2) — medium effort, unlocks retry/polling patterns
6. **stdin support** (#1) — medium effort, unlocks pipe-based workflows
7. **JSON/JUnit output** (#9) — low-medium effort, CI integration
8. **JSON Schema** (#10) — low effort, developer experience
9. **Step groups** (#8) — high effort, defer unless demand is strong
