# Data Model: v1.0 Release Readiness

**Feature Branch**: `011-v1-release-readiness`  
**Date**: 2026-02-08

This feature primarily modifies behavior (signal forwarding) and adds infrastructure (CI/release). The data model changes are minimal — limited to version metadata and release artifacts.

## Entities

### ProcessGroup (runtime, Unix only)

Represents the Unix process group created for a child process.

| Attribute | Type | Description |
|---|---|---|
| pgid | int | Process group ID, equals child PID when using `Setpgid: true, Pgid: 0` |
| members | []Process | All processes sharing this PGID (child + descendants) |
| state | enum | `active`, `terminating`, `terminated` |

**Relationships**:
- 1 `exec` session → 1 `ProcessGroup`
- 1 `ProcessGroup` → many processes (child + grandchildren)

**State Transitions**:
```
active → terminating (SIGTERM sent to group)
terminating → terminated (all members exited or SIGKILL sent)
active → terminated (SIGKILL — instant, no grace period)
```

**Validation Rules**:
- PGID must be > 0
- SIGTERM → SIGKILL escalation timeout: 100ms (hardcoded, not configurable)
- `ESRCH` from `syscall.Kill` is not an error (group already gone)

### VersionInfo (build time)

Embedded metadata set via `-ldflags` at build time.

| Attribute | Type | Default | Description |
|---|---|---|---|
| Version | string | `"dev"` | Semantic version from Git tag (e.g., `1.0.0`) |
| Commit | string | `"none"` | Git commit SHA |
| Date | string | `"unknown"` | Build timestamp (RFC3339) |

**Validation Rules**:
- Version should match semver pattern when built by GoReleaser
- `"dev"` is valid for local builds

### ReleaseArtifact (build output)

Produced by GoReleaser, published to GitHub Releases.

| Attribute | Type | Description |
|---|---|---|
| name | string | Archive filename, e.g., `cli-replay_1.0.0_linux_amd64.tar.gz` |
| os | string | Target OS: `linux`, `darwin`, `windows` |
| arch | string | Target arch: `amd64`, `arm64` |
| format | string | `tar.gz` (Unix) or `zip` (Windows) |
| checksum | string | SHA256 hash of the archive |

**Naming Template**: `cli-replay_{version}_{os}_{arch}.{format}`

**Target Matrix** (5 artifacts):

| OS | Arch | Format |
|---|---|---|
| linux | amd64 | tar.gz |
| linux | arm64 | tar.gz |
| darwin | amd64 | tar.gz |
| darwin | arm64 | tar.gz |
| windows | amd64 | zip |

### ActionInput (GitHub Action)

Inputs accepted by the composite GitHub Action.

| Input | Type | Required | Default | Description |
|---|---|---|---|---|
| version | string | no | `latest` | Version to install (e.g., `v1.0.0`, `latest`) |
| github-token | string | no | `${{ github.token }}` | Token for API rate limit avoidance |

### ActionOutput (GitHub Action)

Outputs produced by the composite GitHub Action.

| Output | Type | Description |
|---|---|---|
| version | string | Actual installed version (e.g., `v1.0.0`) |

## Entity Relationships

```
exec session ──creates──> ProcessGroup ──contains──> Process(es)
                              │
                    signals via Kill(-pgid, sig)

GoReleaser ──produces──> ReleaseArtifact[]
               │                │
               └── checksums ───┘

GitHub Action ──downloads──> ReleaseArtifact
                  │
                  └── resolves version via Releases API
```

## No Database/Storage Changes

This feature does not introduce new file formats, state files, or persistent storage. All changes are:
- Runtime behavior (signal forwarding in `exec_unix.go`)
- Build configuration (`.goreleaser.yaml`, `release.yml`)
- Infrastructure (GitHub Action, badges)
- Documentation (SECURITY.md, README.md)
