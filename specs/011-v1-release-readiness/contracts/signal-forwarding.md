# Contract: Unix Signal Forwarding (exec_unix.go)

**File**: `cmd/exec_unix.go`  
**Purpose**: Process group creation and group-wide signal forwarding on Unix

## Current Behavior (BROKEN)

```go
// Sends signal only to the direct child — grandchildren are NOT signaled
_ = childCmd.Process.Signal(sig)
```

## New Behavior

```go
// FR-001: Create new process group
childCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

// FR-002: Signal the entire process group
pgid := childCmd.Process.Pid
_ = syscall.Kill(-pgid, sig.(syscall.Signal))

// FR-003: Cleanup on exit — terminate remaining group members
_ = syscall.Kill(-pgid, syscall.SIGTERM)
time.Sleep(100 * time.Millisecond)
_ = syscall.Kill(-pgid, syscall.SIGKILL)  // ESRCH if already gone

// FR-004: Fallback — if cmd.Start() fails due to Setpgid,
//         emit warning and retry without Setpgid (single-process mode)
```

## Function Signature (unchanged)

```go
func setupSignalForwarding(childCmd *exec.Cmd) (postStart func(), cleanup func())
```

## Contract Guarantees

- Process group is always created before child exec (no race window)
- Signals (SIGINT, SIGTERM) are forwarded to the entire group, not just the direct child
- Cleanup sends SIGTERM → waits 100ms → SIGKILL to the group
- `ESRCH` errors are silently ignored (group already terminated)
- Fallback emits a warning to stderr when Setpgid is unavailable
- `postStart` remains a no-op on Unix (symmetry with Windows API)
- No change to the function signature — backwards compatible with `exec.go`
