# Quickstart: v1.0 Release Readiness

**Feature Branch**: `011-v1-release-readiness`  
**Prerequisite**: Go 1.21+, goreleaser (v2), access to GitHub Actions

## What This Feature Delivers

1. **Unix process group signal forwarding** — `exec` mode creates a process group and signals the entire tree, not just the direct child
2. **CI badges** — CI status, Go version, and license badges at top of README
3. **ReDoS safety documentation** — SECURITY.md section + benchmark proving RE2 safety
4. **GitHub Action** — `uses: cli-replay/cli-replay@v1` installs the binary on any runner
5. **Release automation** — tag `v*` → GoReleaser builds 5 platform binaries + checksums

## Quick Verification

### 1. Process group cleanup (Unix)

```bash
# Build from this branch
go build -o cli-replay .

# Create a test scenario that runs a script spawning background children
cat > /tmp/test-pgid.yaml << 'EOF'
meta:
  name: pgid-test
steps:
  - argv: ["bash", "-c", "echo hello"]
    respond:
      stdout: "hello\n"
EOF

# Run with a child that spawns a background grandchild
cli-replay exec /tmp/test-pgid.yaml -- bash -c '
  sleep 100 &   # background grandchild
  echo "parent running"
  sleep 1
'
# After exit, verify no orphan sleep process
ps aux | grep "sleep 100" | grep -v grep  # should return nothing
```

### 2. Version output

```bash
# Dev build (no ldflags)
go build -o cli-replay .
./cli-replay --version
# Output: cli-replay version dev

# Build with version embedding
go build -ldflags "-X github.com/cli-replay/cli-replay/cmd.Version=1.0.0-test" -o cli-replay .
./cli-replay --version
# Output: cli-replay version 1.0.0-test
```

### 3. ReDoS benchmark

```bash
go test -bench=BenchmarkRegexPathological -benchtime=5s ./internal/matcher/
# Should complete in microseconds, not seconds
```

### 4. CI badges

After merging to main, visit the repository README on GitHub — three badges should render at the top.

### 5. GitHub Action (after release)

```yaml
# In any workflow:
steps:
  - uses: cli-replay/cli-replay@v1
  - run: cli-replay --version
```

### 6. Release (after goreleaser setup)

```bash
git tag v1.0.0
git push origin v1.0.0
# Release workflow triggers → check GitHub Releases page for artifacts
```

## Files Changed in This Feature

| File | Change |
|---|---|
| `cmd/exec_unix.go` | Process group creation + group signal forwarding |
| `cmd/root.go` | Add `Commit`, `Date` vars; enrich version template |
| `.goreleaser.yaml` | New — release configuration |
| `.github/workflows/release.yml` | New — tag-triggered release pipeline |
| `action.yml` | New — reusable GitHub Action |
| `README.md` | Add CI/Go/License badges at top |
| `SECURITY.md` | Add SIGKILL documentation + RE2 regex safety section |
| `internal/matcher/bench_test.go` | Add `BenchmarkRegexPathological` |
