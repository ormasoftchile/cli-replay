# Research: v1.0 Release Readiness

**Feature Branch**: `011-v1-release-readiness`  
**Date**: 2026-02-08

## R1: Unix Process Group Signal Forwarding

### Decision
Use `Setpgid: true` with `Pgid: 0` on `exec.Cmd.SysProcAttr`, then signal the group via `syscall.Kill(-cmd.Process.Pid, sig)`. Fall back to single-process `Process.Signal()` if `Start()` fails.

### Rationale
This is the standard POSIX pattern for process tree management and the Unix equivalent of what the Windows code already does with Job Objects (`cmd/exec_windows.go`). The current code in `cmd/exec_unix.go` only calls `childCmd.Process.Signal(sig)` which misses grandchildren entirely — despite the README claiming "SIGTERM to process group."

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| `Setsid: true` (new session) | Detaches from controlling terminal, breaks interactive use and TTY tools. Overkill for CI cleanup |
| `/proc` filesystem walking | Fragile, Linux-only, race-prone. Not viable cross-platform |
| `Pdeathsig: SIGKILL` on Linux | Supplemental only — not available on macOS, doesn't cover grandchildren directly |
| `exec.CommandContext` with `Cancel` | Go's built-in context cancellation only calls `Process.Kill()` on the direct child |

### Key Technical Details

- When `Setpgid: true` and `Pgid: 0`, the child's PGID equals its PID (guaranteed by POSIX `setpgid(0, 0)`)
- `syscall.Kill(-pgid, sig)` sends the signal to every process in the group. Returns `ESRCH` if group is already gone (safe to ignore)
- `cmd.Wait()` only waits for the direct child — we must actively kill the group during cleanup
- Works identically on Linux and macOS (Darwin). The `syscall.SysProcAttr.Setpgid` field exists on both
- `Setpgid` failure in practice is almost impossible (standard POSIX call since 1988), but if it fails, `cmd.Start()` returns the error
- macOS lacks `Pdeathsig` (parent death signal) — orthogonal to process groups

### Implementation Pattern

```go
// exec_unix.go - upgraded setupSignalForwarding
childCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}  // FR-001

// Signal forwarding: signal the entire process group
pgid := childCmd.Process.Pid
_ = syscall.Kill(-pgid, sig.(syscall.Signal))  // FR-002

// Cleanup: best-effort kill of process group
_ = syscall.Kill(-pgid, syscall.SIGTERM)  // FR-003
time.Sleep(100 * time.Millisecond)
_ = syscall.Kill(-pgid, syscall.SIGKILL)

// Fallback: if Setpgid fails, cmd.Start() returns error → handled by existing exit code logic (FR-004)
```

---

## R2: GoReleaser Configuration

### Decision
Use GoReleaser v2 with a `.goreleaser.yaml` at repo root, targeting 5 platform/arch combos. Trigger via a separate `.github/workflows/release.yml` on `v*` tags. Embed version in `cmd.Version` via `-ldflags`.

### Rationale
GoReleaser is the de facto standard for Go project releases. The project already has `Version = "dev"` in `cmd/root.go`. GoReleaser handles multi-platform cross-compilation, archive creation, checksum generation, and changelog — all in one tool.

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| Manual `go build` + scripts | Error-prone, no changelog, no checksums, no archive naming convention |
| `Makefile` + `build.ps1` | Already exists for dev builds, but not suitable for release automation |
| Ko (container image builder) | This is a CLI tool, not a container service |

### Key Technical Details

- **ldflags path**: `-X github.com/cli-replay/cli-replay/cmd.Version={{.Version}}` (must use full import path because `Version` lives in `cmd` package, not `main`)
- **Additional vars**: Add `Commit` and `Date` companion vars to `cmd/root.go`
- **Archive format**: `.tar.gz` for Linux/macOS, `.zip` for Windows (GoReleaser convention)
- **Asset naming**: `cli-replay_<version>_<os>_<arch>.<ext>` (GoReleaser default template)
- **Checksums**: Single `cli-replay_<version>_checksums.txt` with SHA256 (GoReleaser's built-in)
- **Changelog**: `git`-based, groups by conventional commit prefix (`feat:`, `fix:`) — no API call needed
- **GitHub Actions**: `goreleaser/goreleaser-action@v6` with `fetch-depth: 0` (full history for changelog), `contents: write` permission
- **Prerelease tags**: GoReleaser's `prerelease: auto` detects tags like `v1.0.0-rc1` and marks them as pre-release

### Files to Create/Modify

| File | Action |
|---|---|
| `.goreleaser.yaml` | Create — full release config |
| `.github/workflows/release.yml` | Create — tag-triggered release pipeline |
| `cmd/root.go` | Modify — add `Commit`, `Date` vars; enrich version template |

---

## R3: Reusable GitHub Action (Composite)

### Decision
Create a composite action (`action.yml` at repo root) that downloads prebuilt binaries from GitHub Releases using `curl`, with OS/arch detection via `runner.os`/`runner.arch`.

### Rationale
Composite actions are the simplest and most portable option — no Node.js runtime or Docker needed. Placing `action.yml` at the repo root gives the cleanest consumer UX: `uses: cli-replay/cli-replay@v1`.

### Alternatives Considered

| Alternative | Why Rejected |
|---|---|
| Node.js action | Requires `dist/` bundle, `node20` runtime, build step. Overkill |
| Docker action | Linux-only, slow cold start, not cross-platform |
| `.github/actions/setup-cli-replay/` | Internal-only path; consumers would need `uses: cli-replay/cli-replay/.github/actions/setup-cli-replay@v1` |
| Separate repo `cli-replay/setup-cli-replay` | Extra repo to maintain; unnecessary for a single CLI tool |

### Key Technical Details

- **OS detection**: `runner.os` returns `Linux`, `macOS`, `Windows` (title-case); must map to `GOOS`
- **Arch detection**: `runner.arch` returns `X64`, `ARM64` (caps); must map to `GOARCH`
- **Version resolution**: GitHub API `/releases/latest` for `latest`; direct URL for pinned versions
- **Download URL**: `https://github.com/cli-replay/cli-replay/releases/download/{tag}/{asset}`
- **PATH**: `echo "${INSTALL_DIR}" >> "$GITHUB_PATH"` (official, cross-platform, persistent)
- **Rate limits**: Pass `github-token` input for API calls (unauthenticated limit is 60/hr shared per runner IP)
- **Install location**: `${{ runner.temp }}/cli-replay/` — temporary, auto-cleaned

### Action Inputs/Outputs

| Input | Default | Purpose |
|---|---|---|
| `version` | `latest` | Version tag to install |
| `github-token` | `${{ github.token }}` | API auth for `latest` resolution |

| Output | Purpose |
|---|---|
| `version` | Actual installed version string |

---

## R4: CI Badges

### Decision
Use GitHub's native badge URLs and shields.io for Go version. Place at top of README immediately after the `# cli-replay` heading.

### Rationale
GitHub's native CI badge URL (`github.com/{owner}/{repo}/actions/workflows/{file}/badge.svg?branch=main`) is the most reliable — no third-party dependency. shields.io is standard for Go version and license badges.

### Key Technical Details

- **CI badge**: `![CI](https://github.com/cli-replay/cli-replay/actions/workflows/ci.yml/badge.svg?branch=main)`
- **Go version badge**: `![Go](https://img.shields.io/badge/Go-1.21-blue?logo=go)`
- **License badge**: `![License](https://img.shields.io/badge/License-MIT-green)`
- All three badges can be on a single line at the top of README
- GitHub's badge SVG is cached with a ~5-minute delay

---

## R5: ReDoS Safety Documentation

### Decision
Add an "RE2 Engine" section to SECURITY.md and create a benchmark test in `internal/matcher/` with a pathological pattern.

### Rationale
Go's `regexp` package implements RE2 (guaranteed O(n) matching — no backtracking). This is an inherent language-level safety property. The work is purely documentation + a benchmark proving the claim.

### Key Technical Details

- Go's `regexp` package uses a Thompson NFA / RE2 algorithm — it **cannot** exhibit exponential backtracking
- Pathological pattern: `^(a+)+$` against `"aaaaaaa...b"` (50 chars) — completes in microseconds in RE2, would timeout in PCRE
- Existing `internal/matcher/bench_test.go` has `BenchmarkArgvMatch` and `BenchmarkGroupMatch_50` — add a `BenchmarkRegexPathological` in the same file
- SECURITY.md already has a "Known Limitations" section — add a "Regex Safety" control in the "Security Controls" table

---

## Summary of NEEDS CLARIFICATION Resolution

| Item | Resolution |
|---|---|
| Process group API | `Setpgid: true` + `syscall.Kill(-pgid, sig)` — standard POSIX, cross-platform Unix |
| GoReleaser config | v2, `.goreleaser.yaml` at root, 5 targets, `cmd.Version` ldflags |
| Release workflow | Separate `release.yml`, `goreleaser-action@v6`, `v*` tag trigger |
| Action location | Repo root `action.yml`, composite action, `curl`-based download |
| Asset naming | `cli-replay_{version}_{os}_{arch}.{tar.gz\|zip}` |
| Version resolution | API for `latest`, direct URL for pinned |
| Badge URLs | GitHub native + shields.io |
| ReDoS | Go RE2 inherent safety, document + benchmark |

All NEEDS CLARIFICATION items resolved. No blockers for Phase 1.
