# cli-replay GitHub Action

Install [cli-replay](https://github.com/ormasoftchile/cli-replay) and optionally run or validate a scenario in your CI workflow.

## Usage

### Basic: Run a scenario

```yaml
- name: Test with cli-replay
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: test-scenario.yaml
    run: bash test.sh
```

### With JUnit Reporting

```yaml
- name: Test with cli-replay
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: test-scenario.yaml
    run: bash test.sh
    format: junit
    report-file: results.xml

- name: Publish Test Results
  uses: dorny/test-reporter@v1
  if: always()
  with:
    name: CLI Replay Tests
    path: results.xml
    reporter: java-junit
```

### Cross-Platform Matrix

```yaml
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: ormasoftchile/cli-replay-action@v1
        with:
          scenario: test-scenario.yaml
          run: bash test.sh
```

### Validate Only (PR Gate)

```yaml
- name: Validate scenarios
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: scenarios/deploy.yaml
    validate-only: 'true'
```

### Pin to Specific Version

```yaml
- name: Test with cli-replay
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: test-scenario.yaml
    run: make test
    version: v1.2.3
```

### With Allowed Commands

```yaml
- name: Test with cli-replay
  uses: ormasoftchile/cli-replay-action@v1
  with:
    scenario: kubectl-test.yaml
    run: bash deploy.sh
    allowed-commands: kubectl,helm
```

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `scenario` | Yes | — | Path to scenario YAML file |
| `run` | No | — | Command to run under interception |
| `version` | No | `latest` | cli-replay version (e.g., `v1.2.3`) |
| `format` | No | — | Output format: `json` or `junit` |
| `report-file` | No | — | Path to write verification report |
| `validate-only` | No | `false` | Run `validate` instead of `exec` |
| `allowed-commands` | No | — | Comma-separated command allowlist |

## Outputs

| Output | Description |
|--------|-------------|
| `cli-replay-version` | The installed cli-replay version string |

## Supported Platforms

| Runner | OS | Arch | Status |
|--------|----|------|--------|
| `ubuntu-latest` | Linux | X64 | ✅ Supported |
| `ubuntu-24.04-arm` | Linux | ARM64 | ✅ Supported |
| `macos-latest` | macOS | ARM64 | ✅ Supported |
| `macos-13` | macOS | X64 | ✅ Supported |
| `windows-latest` | Windows | X64 | ✅ Supported |

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Unsupported runner OS | `::error::Unsupported runner OS`, exit 1 |
| Unsupported CPU architecture | `::error::Unsupported architecture`, exit 1 |
| Download failure | curl fails, step exits non-zero |
| Version not found | 404 from GitHub Releases, step fails |
| Scenario validation errors | cli-replay returns exit 1, step fails |
| Exec verification failure | cli-replay returns exit 1, step fails |

## Versioning

- Use `@v1` (recommended) — floating tag pointing to latest `v1.x.y`
- Use `@v1.2.3` — pin to exact release
- Use `@main` — latest commit (not recommended for production)

## License

MIT
