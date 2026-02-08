# GitHub Action Contract: ormasoftchile/cli-replay-action

**Date**: 2026-02-07  
**Spec**: [spec.md](../spec.md)

---

## action.yml — Composite Action Definition

### Metadata

```yaml
name: 'Setup cli-replay'
description: 'Install cli-replay and optionally run/validate a scenario'
branding:
  icon: 'terminal'
  color: 'blue'
```

### Inputs

```yaml
inputs:
  scenario:
    description: 'Path to scenario YAML file'
    required: true
  run:
    description: 'Command to run under interception (passed after -- to cli-replay exec)'
    required: false
  version:
    description: 'cli-replay version to install (e.g., v1.2.3). Default: latest'
    required: false
    default: 'latest'
  format:
    description: 'Output format for verification report: json or junit'
    required: false
  report-file:
    description: 'Path to write verification report file'
    required: false
  validate-only:
    description: 'Run cli-replay validate instead of exec (true/false)'
    required: false
    default: 'false'
  allowed-commands:
    description: 'Comma-separated list of commands allowed to be intercepted'
    required: false
```

### Outputs

```yaml
outputs:
  cli-replay-version:
    description: 'The installed cli-replay version'
    value: ${{ steps.install.outputs.version }}
```

### Steps

```yaml
runs:
  using: "composite"
  steps:
    # Step 1: Detect platform and install cli-replay
    - name: Install cli-replay
      id: install
      shell: bash
      run: |
        # Map runner.os → GOOS
        case "${{ runner.os }}" in
          Linux)   GOOS="linux" ;;
          macOS)   GOOS="darwin" ;;
          Windows) GOOS="windows" ;;
          *) echo "::error::Unsupported runner OS: ${{ runner.os }}"; exit 1 ;;
        esac

        # Map runner.arch → GOARCH
        case "${{ runner.arch }}" in
          X64)   GOARCH="amd64" ;;
          ARM64) GOARCH="arm64" ;;
          *) echo "::error::Unsupported architecture: ${{ runner.arch }}"; exit 1 ;;
        esac

        # Resolve version
        VERSION="${{ inputs.version }}"
        if [ "$VERSION" = "latest" ]; then
          VERSION=$(curl -sL https://api.github.com/repos/ormasoftchile/cli-replay/releases/latest | grep '"tag_name"' | head -1 | cut -d '"' -f 4)
        fi

        # Download
        EXT="tar.gz"
        [ "$GOOS" = "windows" ] && EXT="zip"
        ARCHIVE="cli-replay-${VERSION}-${GOOS}-${GOARCH}.${EXT}"
        URL="https://github.com/ormasoftchile/cli-replay/releases/download/${VERSION}/${ARCHIVE}"
        echo "Downloading cli-replay ${VERSION} for ${GOOS}/${GOARCH}..."
        curl -sL "${URL}" -o "${ARCHIVE}"

        # Extract
        mkdir -p "${{ github.action_path }}/bin"
        if [ "$EXT" = "zip" ]; then
          unzip -q "${ARCHIVE}" -d "${{ github.action_path }}/bin"
        else
          tar xzf "${ARCHIVE}" -C "${{ github.action_path }}/bin"
        fi
        rm -f "${ARCHIVE}"

        # Add to PATH
        echo "${{ github.action_path }}/bin" >> "$GITHUB_PATH"

        # Output version
        echo "version=${VERSION}" >> "$GITHUB_OUTPUT"
        echo "Installed cli-replay ${VERSION}"

    # Step 2: Run validate or exec
    - name: Run cli-replay
      id: run
      shell: bash
      run: |
        if [ "${{ inputs.validate-only }}" = "true" ]; then
          # Validate mode
          VALIDATE_CMD="cli-replay validate"
          [ -n "${{ inputs.format }}" ] && VALIDATE_CMD="${VALIDATE_CMD} --format ${{ inputs.format }}"
          ${VALIDATE_CMD} ${{ inputs.scenario }}
        elif [ -n "${{ inputs.run }}" ]; then
          # Exec mode
          EXEC_CMD="cli-replay exec"
          [ -n "${{ inputs.format }}" ] && EXEC_CMD="${EXEC_CMD} --format ${{ inputs.format }}"
          [ -n "${{ inputs.report-file }}" ] && EXEC_CMD="${EXEC_CMD} --report-file ${{ inputs.report-file }}"
          [ -n "${{ inputs.allowed-commands }}" ] && EXEC_CMD="${EXEC_CMD} --allowed-commands ${{ inputs.allowed-commands }}"
          ${EXEC_CMD} ${{ inputs.scenario }} -- ${{ inputs.run }}
        else
          # Setup-only mode: just validate
          cli-replay validate ${{ inputs.scenario }}
        fi
```

---

## Usage Examples

### Basic: Run a scenario in CI

```yaml
- name: Test with cli-replay
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: test-scenario.yaml
    run: bash test.sh
```

### With JUnit reporting

```yaml
- name: Test with cli-replay
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: test-scenario.yaml
    run: bash test.sh
    format: junit
    report-file: results.xml
```

### Validate only (pre-commit / PR check)

```yaml
- name: Validate scenarios
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: scenarios/*.yaml
    validate-only: 'true'
```

### Pin to specific version

```yaml
- name: Test with cli-replay
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: test-scenario.yaml
    run: make test
    version: v1.2.3
```

### With allowed commands

```yaml
- name: Test with cli-replay
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: kubectl-test.yaml
    run: bash deploy.sh
    allowed-commands: kubectl,helm
```

---

## Supported Platforms

| Runner | OS | Arch | Binary | Status |
|--------|-----|------|--------|--------|
| `ubuntu-latest` | Linux | X64 | `cli-replay` | Supported |
| `ubuntu-24.04-arm` | Linux | ARM64 | `cli-replay` | Supported |
| `macos-latest` | macOS | ARM64 | `cli-replay` | Supported |
| `macos-13` | macOS | X64 | `cli-replay` | Supported |
| `windows-latest` | Windows | X64 | `cli-replay.exe` | Supported |

---

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Unsupported runner.os | `::error::Unsupported runner OS: {os}`, exit 1 |
| Unsupported runner.arch | `::error::Unsupported architecture: {arch}`, exit 1 |
| Download failure | curl returns non-zero, step fails |
| Version not found | 404 from GitHub API, step fails |
| Scenario failure | cli-replay returns non-zero, step fails |
| Validation errors | cli-replay validate returns 1, step fails |

---

## Versioning

| Tag | Meaning |
|-----|---------|
| `v1.0.0` | Initial release |
| `v1.x.y` | Patch/minor updates (backward compatible) |
| `v1` | Floating tag → latest `v1.x.y` (recommended for users) |
| `v2` | Future breaking changes (new major tag) |

Update floating tag on each release:
```bash
git tag -fa v1 -m "Update v1 tag to v1.x.y"
git push origin v1 --force
```

---

## Release Binary Naming Convention

The action expects binaries published to GitHub Releases with this naming:

```
cli-replay-{version}-{goos}-{goarch}.{ext}

Examples:
  cli-replay-v1.2.3-linux-amd64.tar.gz
  cli-replay-v1.2.3-linux-arm64.tar.gz
  cli-replay-v1.2.3-darwin-amd64.tar.gz
  cli-replay-v1.2.3-darwin-arm64.tar.gz
  cli-replay-v1.2.3-windows-amd64.zip
```

This convention must be established in `build.ps1` / `Makefile` release targets.
