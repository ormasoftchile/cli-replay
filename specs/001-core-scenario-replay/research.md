# Research: Core Scenario Replay

**Feature**: 001-core-scenario-replay  
**Date**: 2026-02-05  
**Purpose**: Resolve technical unknowns before Phase 1 design

## Research Tasks

### 1. State File Format and Locking

**Question**: How should state be persisted between CLI invocations to track scenario progress?

**Decision**: JSON file with atomic write via rename

**Rationale**:
- JSON is human-readable for debugging
- Atomic write (write to temp, rename) prevents corruption on crash
- File locking not needed for v0 (single-process, sequential execution assumption)
- Path: `/tmp/cli-replay-<sha256(scenario-path)[:16]>.state`

**Alternatives Considered**:
- SQLite: Overkill for simple index tracking; adds CGO dependency
- Binary format: Harder to debug; no meaningful size benefit
- Environment variable: Cannot persist across separate process invocations

**State File Schema**:
```json
{
  "scenario_path": "/path/to/scenario.yaml",
  "scenario_hash": "a1b2c3d4...",
  "current_step": 2,
  "total_steps": 5,
  "last_updated": "2026-02-05T10:30:00Z"
}
```

---

### 2. YAML Parsing with Strict Validation

**Question**: How to parse YAML while rejecting unknown fields?

**Decision**: Use `gopkg.in/yaml.v3` with `KnownFields(true)` decoder option

**Rationale**:
- yaml.v3 supports strict mode via `decoder.KnownFields(true)`
- Rejects unknown fields at parse time with clear error messages
- Well-maintained, widely used in Go ecosystem
- No CGO required

**Code Pattern**:
```go
decoder := yaml.NewDecoder(reader)
decoder.KnownFields(true)
err := decoder.Decode(&scenario)
```

**Alternatives Considered**:
- `sigs.k8s.io/yaml`: Kubernetes wrapper, heavier dependency
- `github.com/goccy/go-yaml`: Less mature, fewer users

---

### 3. Cobra CLI Structure

**Question**: How to structure CLI commands for both "fake executable" mode and management commands?

**Decision**: Single binary with behavior determined by invocation name (argv[0])

**Rationale**:
- When invoked as symlink (e.g., `kubectl`), detect via `os.Args[0]` and run replay mode
- When invoked as `cli-replay`, use cobra subcommands (`run`, `verify`, `init`)
- Follows busybox/toybox pattern for multi-call binaries
- No separate binaries needed

**Command Structure**:
```
cli-replay run <scenario.yaml>     # Start replay session (optional if using symlink)
cli-replay verify <scenario.yaml>  # Check all steps consumed
cli-replay init <scenario.yaml>    # Reset state file for scenario
cli-replay --help                  # Show usage
cli-replay --version               # Show version

# When symlinked as "kubectl":
kubectl get pods                   # Intercepts and replays from scenario
```

**Alternatives Considered**:
- Separate `cli-replay-shim` binary: More complex distribution
- Always require explicit `cli-replay run`: Breaks transparent interception

---

### 4. Template Rendering

**Question**: How to safely render Go templates in stdout/stderr responses?

**Decision**: Use `text/template` with merged variable context (env overrides vars)

**Rationale**:
- `text/template` is stdlib, no dependency
- `missingkey=error` option catches undefined variables at render time
- Merge order: `meta.vars` as base, environment variables overlay

**Code Pattern**:
```go
tmpl, err := template.New("response").Option("missingkey=error").Parse(text)
context := mergeVarsWithEnv(scenario.Meta.Vars)
var buf bytes.Buffer
err = tmpl.Execute(&buf, context)
```

**Alternatives Considered**:
- `html/template`: Adds HTML escaping, wrong for CLI output
- Handlebars/Mustache libraries: External dependency, Go syntax is fine

---

### 5. Cross-Platform Temp Directory

**Question**: Where to store state files across Windows/macOS/Linux?

**Decision**: Use `os.TempDir()` for platform-appropriate temp location

**Rationale**:
- `os.TempDir()` returns platform-specific temp directory
- Windows: `%TEMP%` or `%TMP%`
- Unix: `$TMPDIR` or `/tmp`
- Consistent behavior without platform checks

**State File Path**:
```go
hash := sha256.Sum256([]byte(absScenarioPath))
filename := fmt.Sprintf("cli-replay-%x.state", hash[:8])
statePath := filepath.Join(os.TempDir(), filename)
```

---

### 6. Error Message Format

**Question**: What format for error messages to maximize debuggability?

**Decision**: Structured stderr with labeled fields

**Rationale**:
- Human-readable format with clear labels
- Machine-parseable for test frameworks that capture stderr
- Include all context needed to diagnose without additional debugging

**Format**:
```
cli-replay: mismatch at step 2 of "incident-remediation"
  expected: ["kubectl", "rollout", "restart", "deployment/sql-agent"]
  received: ["kubectl", "get", "pods", "-n", "sql"]
  scenario: /path/to/scenario.yaml
```

**Alternatives Considered**:
- JSON errors: Harder to read in terminal
- Single-line errors: Insufficient context

---

### 7. Build and Release Strategy

**Question**: How to produce static binaries for all platforms?

**Decision**: Makefile + goreleaser for releases

**Rationale**:
- `CGO_ENABLED=0 go build` produces static binary
- goreleaser automates cross-compilation and release artifacts
- Makefile for local development (`make build`, `make test`, `make lint`)

**Build Commands**:
```makefile
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/cli-replay ./cmd/cli-replay

test:
	go test -race -cover ./...

lint:
	golangci-lint run
```

---

## Summary of Decisions

| Topic | Decision |
|-------|----------|
| State persistence | JSON file in temp dir, atomic write via rename |
| YAML parsing | yaml.v3 with KnownFields(true) |
| CLI structure | Multi-call binary pattern (argv[0] detection) |
| Template rendering | text/template with missingkey=error |
| Temp directory | os.TempDir() for cross-platform |
| Error format | Structured stderr with labeled fields |
| Build strategy | Makefile + goreleaser, CGO_ENABLED=0 |

## Open Questions (None)

All technical unknowns have been resolved.
