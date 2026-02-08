# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | ✅ |
| Previous minor | ✅ (security fixes backported) |
| Older versions | ❌ (upgrade recommended) |

## Reporting a Vulnerability

If you discover a security vulnerability in cli-replay, please report it responsibly:

1. **GitHub Security Advisory** (preferred): [Create a private security advisory](https://github.com/ormasoftchile/cli-replay/security/advisories/new)
2. **Email**: Send details to the repository maintainers via the email listed on their GitHub profiles

**Response SLA**:
- Acknowledgment within 48 hours
- Initial assessment within 7 days
- Fix or mitigation within 30 days for confirmed vulnerabilities

Please include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact assessment
- Suggested fix (if any)

**Do not** open a public issue for security vulnerabilities.

## Trust Model

### Scenario Files Are Code

Scenario YAML files define what commands to intercept, what responses to return, and what exit codes to produce. **Treat scenario files with the same review rigor as source code.**

A malicious scenario file could:
- Intercept arbitrary commands on PATH
- Return misleading output to scripts
- Cause scripts to take incorrect actions based on fake command results

**Recommendations**:
- Review scenario files in pull requests like any other code change
- Store scenarios in version control alongside the code they test
- Do not download and execute scenarios from untrusted sources

### Shim Scripts Run As Current User

cli-replay intercepts commands by placing shim scripts (symlinks on Unix, `.cmd`/`.ps1` wrappers on Windows) first on PATH. These shims execute with the same permissions as the current user. No privilege escalation is involved, but the shims have full access to the user's environment.

## Threat Boundaries

### PATH Interception Mechanism

**Risk**: cli-replay prepends a directory containing shim scripts to `PATH`. Any command whose name matches a shim will be intercepted instead of running the real binary.

**Mitigations**:
- **`allowed_commands`** (YAML or `--allowed-commands` flag): Restricts which commands can be intercepted. If a scenario references a command not in the allowlist, cli-replay exits with an error *before* creating any intercepts.
- **Session isolation**: Each `cli-replay run` or `exec` invocation creates a unique session with its own intercept directory. Parallel sessions do not share interceptors.
- **Cleanup**: The `cli-replay clean` command and automatic trap cleanup (Unix) or defer cleanup (`exec` mode) remove intercept directories when sessions end.

### Environment Variable Leaking

**Risk**: Template variables in scenario files (e.g., `{{ .my_secret }}`) resolve against environment variables. A scenario could unintentionally expose sensitive values in intercepted command responses.

**Mitigations**:
- **`deny_env_vars`** (YAML `meta.security.deny_env_vars`): Glob-based patterns that block specific environment variables from template rendering. Denied variables resolve to their `meta.vars` default or empty string.
- **Internal variables exempt**: `CLI_REPLAY_*` variables are never blocked by deny rules.
- **Trace logging**: When `CLI_REPLAY_TRACE=1`, denied variable names are logged (but not their values).

```yaml
meta:
  security:
    deny_env_vars:
      - "AWS_*"        # Block all AWS credentials
      - "SECRET_*"     # Block all secret-prefixed vars
      - "TOKEN"        # Block exact match
```

### Session Isolation Boundaries

**Risk**: Parallel CI jobs using the same scenario file could interfere with each other's state.

**Mitigations**:
- Each session gets a unique ID (UUID-based) set via `CLI_REPLAY_SESSION`
- State files are session-scoped: `cli-replay-<hash>-<session>.state`
- No shared mutable state between sessions
- Session TTL (`meta.session.ttl`) automatically cleans up stale sessions

### Recording Mode Trust

**Risk**: `cli-replay record` executes real commands and captures their output. The recorded YAML contains real command responses.

**Mitigations**:
- Review recorded YAML before committing — it may contain sensitive output
- Use `--command` flag to limit which commands are intercepted during recording
- Recording shims capture stdout, stderr, and exit codes only — no memory or file system inspection

## Security Controls

| Control | Mechanism | Configuration |
|---------|-----------|---------------|
| Command allowlist | `allowed_commands` | YAML: `meta.security.allowed_commands`; CLI: `--allowed-commands` |
| Environment filtering | `deny_env_vars` | YAML: `meta.security.deny_env_vars` (glob patterns) |
| Strict YAML parsing | `KnownFields(true)` | Always enabled — unknown fields are rejected |
| Session TTL | Auto-cleanup | YAML: `meta.session.ttl` (Go duration) |
| Session isolation | Unique session IDs | Automatic via `CLI_REPLAY_SESSION` |
| Intercept cleanup | Automatic | Unix: shell trap; `exec` mode: defer; Manual: `cli-replay clean` |
| Process group cleanup (Unix) | `Setpgid` + group kill | Automatic in `exec` mode; SIGTERM → 100ms → SIGKILL to entire group |
| Regex safety (ReDoS) | Go RE2 engine | Inherent — no configuration needed |

## Known Limitations

1. **`deny_env_vars` is pattern-based, not content-scanning**: It blocks variables by name pattern, not by inspecting values. A variable named `MY_DATA` containing a secret would not be blocked unless the pattern matches `MY_DATA`.

2. **Shim scripts run as current user**: No sandboxing or privilege separation. The intercepted commands' environment is the same as the calling process.

3. **No signature verification on scenarios**: Scenario files are not cryptographically signed. Trust is based on source control and code review.

4. **Windows process tree termination**: On Windows versions older than Windows 8 (unsupported), job object assignment may fail, falling back to single-process kill. Grandchild processes may be orphaned.

5. **Recording captures real output**: `cli-replay record` captures real command output verbatim. Sensitive data in command output will appear in the recorded YAML.

6. **SIGKILL cannot be intercepted**: On Unix, `SIGKILL` (signal 9) is handled directly by the kernel and cannot be caught or forwarded by any user-space process, including cli-replay. If cli-replay is killed with `SIGKILL`, the process group cleanup logic cannot execute, and descendant processes may be orphaned. The TTL-based session cleanup (`meta.session.ttl`) mitigates this by removing stale session artifacts on the next `exec` invocation. To avoid orphans in CI, prefer `SIGTERM` (signal 15) which allows cli-replay to perform orderly group shutdown.

## Regex Safety (ReDoS Prevention)

cli-replay uses Go's built-in `regexp` package for pattern matching in `match.argv` fields (`{{ .regex "..." }}`). Go's `regexp` package implements the **RE2 algorithm** (Thompson NFA), which guarantees **linear-time matching** — O(n) in the length of the input string, regardless of pattern complexity.

This means cli-replay is **inherently immune to Regular Expression Denial of Service (ReDoS)** attacks. Pathological patterns like `^(a+)+$` that cause exponential backtracking in PCRE-based engines (Perl, JavaScript, Python `re`, Java) complete in microseconds in Go's RE2 engine.

**Key properties**:
- No exponential backtracking is possible
- Matching time is bounded by O(m×n) where m = pattern size, n = input length
- A benchmark test (`BenchmarkRegexPathological` in `internal/matcher/bench_test.go`) demonstrates safe performance on known-pathological patterns
- No additional configuration, timeouts, or complexity limits are needed

For more information, see [RE2: A Principled Approach to Regular Expression Matching](https://github.com/google/re2) and [Go regexp package documentation](https://pkg.go.dev/regexp).

## Recommendations

1. **Review scenarios in pull requests** — Scenario files define what commands your tests intercept and what responses they receive. Treat them as code.

2. **Restrict `allowed_commands` in production CI** — Always specify the minimum set of commands your scenario needs. This prevents accidental interception of unrelated tools.

3. **Use `deny_env_vars` for secrets** — If your CI environment has sensitive variables (AWS credentials, tokens, API keys), add deny patterns to prevent template leaking.

4. **Use `exec` mode in CI** — The `exec` command handles setup, execution, and cleanup in a single invocation with automatic defer cleanup. This reduces the risk of orphaned intercept directories.

5. **Pin cli-replay versions in CI** — Use a specific version tag (`v1.2.3`) rather than `latest` to ensure reproducible builds and prevent supply-chain attacks via compromised releases.

6. **Audit recording output** — After using `cli-replay record`, review the generated YAML for sensitive data before committing to version control.
