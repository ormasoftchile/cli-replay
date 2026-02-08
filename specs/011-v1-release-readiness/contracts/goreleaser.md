# Contract: GoReleaser Configuration

**File**: `.goreleaser.yaml` (repo root)  
**Purpose**: Multi-platform release build configuration

## Schema

```yaml
version: 2
project_name: cli-replay

before:
  hooks:
    - go mod tidy

builds:
  - id: cli-replay
    main: .
    binary: cli-replay
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X github.com/cli-replay/cli-replay/cmd.Version={{.Version}}
      - -X github.com/cli-replay/cli-replay/cmd.Commit={{.Commit}}
      - -X github.com/cli-replay/cli-replay/cmd.Date={{.Date}}

archives:
  - id: default
    format: tar.gz
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "{{ .ProjectName }}_{{ .Version }}_checksums.txt"
  algorithm: sha256

changelog:
  sort: asc
  use: git
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"
      - "^chore:"
      - "(?i)merge pull request"
      - "(?i)merge branch"
  groups:
    - title: "Features"
      regexp: '^.*?feat(\([[:word:]]+\))??!?:.+$'
      order: 0
    - title: "Bug Fixes"
      regexp: '^.*?fix(\([[:word:]]+\))??!?:.+$'
      order: 1
    - title: "Other Changes"
      order: 999

release:
  github:
    owner: cli-replay
    name: cli-replay
  draft: false
  prerelease: auto
  name_template: "v{{.Version}}"
```

## Output Artifacts (per release)

| Artifact | Example |
|---|---|
| Linux amd64 | `cli-replay_1.0.0_linux_amd64.tar.gz` |
| Linux arm64 | `cli-replay_1.0.0_linux_arm64.tar.gz` |
| Darwin amd64 | `cli-replay_1.0.0_darwin_amd64.tar.gz` |
| Darwin arm64 | `cli-replay_1.0.0_darwin_arm64.tar.gz` |
| Windows amd64 | `cli-replay_1.0.0_windows_amd64.zip` |
| Checksums | `cli-replay_1.0.0_checksums.txt` |
