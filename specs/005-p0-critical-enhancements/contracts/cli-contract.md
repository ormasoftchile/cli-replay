# CLI Contract: P0 Critical Enhancements

**Feature**: `005-p0-critical-enhancements`

---

## Command Changes

### `cli-replay run` (modified)

**New Flag**: `--allowed-commands`

```
cli-replay run [--allowed-commands cmd1,cmd2,...] <scenario-file> -- <command> [args...]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--allowed-commands` | `string` (comma-separated) | `""` (no restriction) | Restrict which commands may be intercepted. Overrides / intersects with YAML `meta.security.allowed_commands`. |

**Behavior Changes**:
- Before creating intercepts, validate all `step[*].match.argv[0]` base names against the effective allowlist
- If YAML `allowed_commands` AND `--allowed-commands` are both set → use intersection
- If only one is set → use that list
- If neither is set → allow all (backward compatible)
- On violation: exit 1 with `Error: command "docker" is not in the allowed commands list: [kubectl, az]`

**Exit Codes** (unchanged + new):
| Code | Meaning |
|------|---------|
| 0 | Success (existing) |
| 1 | Error — scenario load failed, validation failed, or **allowlist violation** (new) |

---

### `cli-replay verify` (modified)

**Behavior Changes**:
- In addition to checking `state.AllStepsConsumed()`, check per-step minimum counts via `state.AllStepsMetMin(scenario.Steps)`
- Report per-step counts in verification output

**Output Format** (enhanced):

Success case (no call bounds):
```
✓ Scenario "my-test" completed: 3/3 steps consumed
```

Success case (with call bounds):
```
✓ Scenario "my-test" completed: 3/3 steps consumed
  Step 1: kubectl get pods — 4 calls (min: 1, max: 5) ✓
  Step 2: kubectl apply — 1 call (min: 1, max: 1) ✓
  Step 3: kubectl rollout status — 2 calls (min: 1, max: 3) ✓
```

Failure case (min not met):
```
✗ Scenario "my-test" incomplete: step 2 not satisfied
  Step 1: kubectl get pods — 3 calls (min: 1, max: 5) ✓
  Step 2: kubectl apply — 0 calls (min: 1, max: 1) ✗ needs 1 more
  Step 3: kubectl rollout status — not reached
```

**Exit Codes** (unchanged):
| Code | Meaning |
|------|---------|
| 0 | All steps satisfied |
| 1 | Steps incomplete or min counts not met |

---

### `cli-replay record` (modified)

**Behavior Changes**:
- Shim captures stdin when piped (non-TTY)
- Recorded JSONL entries include `stdin` field when captured
- YAML converter includes `match.stdin` when stdin is non-empty

No new flags.

---

### Intercept Mode (symlink invocation — modified)

**Behavior Changes**:
- Reads `os.Stdin` when `match.stdin` is set on the current step
- Compares actual stdin vs expected (trailing-newline normalized)
- Increments `StepCounts[idx]` instead of flipping `ConsumedSteps[idx]`
- Budget check: if `StepCounts[idx] >= max`, advance to next step before matching
- Soft advance: if current step doesn't match AND `StepCounts[idx] >= min`, try next step

**New Environment Variable** (record path only):
| Variable | Set By | Used By | Description |
|----------|--------|---------|-------------|
| `CLI_REPLAY_STDIN_FILE` | Shim | Recorder | Path to temp file containing captured stdin |

---

## Error Messages (new/changed)

### Mismatch Error (enhanced format)

```
Mismatch at step 2 of "deployment-test":

  Expected: ["kubectl", "get", "pods", "-n", "{{ .regex \"^prod-.*\" }}"]
  Received: ["kubectl", "get", "pods", "-n", "staging-app"]
                                              ^^^^^^^^^^^
  First difference at position 4:
    expected pattern: ^prod-.*
    received value:   staging-app

```

### Length Mismatch Error

```
Mismatch at step 1 of "deployment-test":

  Expected: ["kubectl", "get", "pods", "-n", "{{ .any }}"]  (5 args)
  Received: ["kubectl", "get", "pods"]                       (3 args)

  Missing arguments starting at position 3:
    [3]: "-n"
    [4]: "{{ .any }}"

```

### Call Count Error (verify)

```
Step 2 "kubectl apply" not satisfied:
  received 0 calls, minimum required: 1
```

### Allowlist Violation Error

```
Error: command "docker" is not in the allowed commands list: [kubectl, az]
  Scenario: deployment-test
  Step 3: ["docker", "build", "-t", "myapp"]
```

### Stdin Mismatch Error

```
Mismatch at step 1 of "pipe-test":

  argv matched: ["kubectl", "apply", "-f", "-"]
  stdin mismatch:
    expected (first 200 chars):
      apiVersion: v1
      kind: Pod
      ...
    received (first 200 chars):
      apiVersion: v1
      kind: Deployment
      ...
```
