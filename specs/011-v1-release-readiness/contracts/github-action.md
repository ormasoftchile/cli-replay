# Contract: GitHub Action (Setup cli-replay)

**File**: `action.yml` (repo root)  
**Purpose**: Reusable composite action to install cli-replay on GitHub Actions runners

## Action Manifest Schema

```yaml
name: 'Setup cli-replay'
description: 'Download and install cli-replay from GitHub Releases'
branding:
  icon: 'terminal'
  color: 'blue'

inputs:
  version:
    description: 'Version to install (e.g. v1.0.0, latest)'
    required: false
    default: 'latest'
  github-token:
    description: 'GitHub token for API rate limits on version resolution'
    required: false
    default: ${{ github.token }}

outputs:
  version:
    description: 'Actual installed version'
    value: ${{ steps.install.outputs.version }}

runs:
  using: "composite"
  steps:
    - name: Install cli-replay
      id: install
      shell: bash
      env:
        GH_TOKEN: ${{ inputs.github-token }}
        INPUT_VERSION: ${{ inputs.version }}
      run: |
        set -euo pipefail

        # Map runner.os to GOOS
        case "${{ runner.os }}" in
          Linux)   GOOS="linux"   ;;
          macOS)   GOOS="darwin"  ;;
          Windows) GOOS="windows" ;;
          *)       echo "::error::Unsupported OS: ${{ runner.os }}"; exit 1 ;;
        esac

        # Map runner.arch to GOARCH
        case "${{ runner.arch }}" in
          X64)   GOARCH="amd64" ;;
          ARM64) GOARCH="arm64" ;;
          *)     echo "::error::Unsupported arch: ${{ runner.arch }}"; exit 1 ;;
        esac

        # Resolve version
        VERSION="${INPUT_VERSION}"
        if [ "$VERSION" = "latest" ]; then
          VERSION=$(curl -fsSL \
            -H "Authorization: token ${GH_TOKEN}" \
            "https://api.github.com/repos/cli-replay/cli-replay/releases/latest" \
            | grep '"tag_name"' | head -1 | cut -d '"' -f 4)
          if [ -z "$VERSION" ]; then
            echo "::error::Failed to resolve latest version"
            exit 1
          fi
        fi

        # Normalize version prefix
        [[ "$VERSION" =~ ^v ]] || VERSION="v${VERSION}"

        # Determine archive format
        EXT="tar.gz"
        [ "$GOOS" = "windows" ] && EXT="zip"

        # Strip v prefix for archive name (GoReleaser uses version without v)
        VER_NUM="${VERSION#v}"
        ARCHIVE="cli-replay_${VER_NUM}_${GOOS}_${GOARCH}.${EXT}"

        # Download
        URL="https://github.com/cli-replay/cli-replay/releases/download/${VERSION}/${ARCHIVE}"
        echo "Downloading ${URL}"
        curl -fsSL "${URL}" -o "${ARCHIVE}"

        # Extract
        INSTALL_DIR="${{ runner.temp }}/cli-replay"
        mkdir -p "${INSTALL_DIR}"
        if [ "$EXT" = "zip" ]; then
          unzip -q "${ARCHIVE}" -d "${INSTALL_DIR}"
        else
          tar xzf "${ARCHIVE}" -C "${INSTALL_DIR}"
        fi
        rm -f "${ARCHIVE}"

        # Add to PATH
        echo "${INSTALL_DIR}" >> "$GITHUB_PATH"
        echo "version=${VERSION}" >> "$GITHUB_OUTPUT"
        echo "Installed cli-replay ${VERSION} for ${GOOS}/${GOARCH}"

    - name: Verify installation
      shell: bash
      run: cli-replay --version
```

## Consumer Usage

```yaml
# In a workflow:
steps:
  - uses: cli-replay/cli-replay@v1
    with:
      version: 'v1.0.0'  # or 'latest'

  - name: Run scenario
    run: cli-replay exec scenario.yaml -- ./my-script.sh
```

## Contract Guarantees

- `version: latest` resolves to the most recent non-prerelease, non-draft release
- Binary is available on PATH for all subsequent steps
- Supports `ubuntu-latest`, `macos-latest`, `windows-latest`
- Returns actual installed version as action output
- No Go toolchain installation required (downloads prebuilt binary)
