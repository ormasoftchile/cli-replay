# Data Model: Workflow Recorder

**Feature**: 002-workflow-recorder  
**Created**: February 6, 2026  
**Purpose**: Define entities and their relationships for command recording and YAML generation

## Core Entities

### RecordedCommand

Represents a single command execution captured during a recording session.

**Fields**:
- `Timestamp` (time.Time): UTC timestamp when command was executed
- `Argv` ([]string): Complete command-line arguments including program name
- `ExitCode` (int): Process exit code (0-255)
- `Stdout` (string): Standard output captured from command
- `Stderr` (string): Standard error output captured from command

**Validation Rules**:
- `Argv` must be non-empty
- `ExitCode` must be in range 0-255
- `Timestamp` must not be zero value
- `Stdout` and `Stderr` can be empty strings (valid for commands with no output)

**Relationships**:
- Belongs to one `RecordingSession`
- Maps 1:1 to `scenario.Step` during conversion

**State Transitions**: N/A (immutable once recorded)

---

### RecordingSession

Manages the lifecycle of a recording session, including shim setup, command execution tracking, and cleanup.

**Fields**:
- `ID` (string): Unique session identifier (UUID or timestamp-based)
- `StartTime` (time.Time): When recording session started
- `EndTime` (time.Time): When recording session completed
- `Commands` ([]RecordedCommand): Ordered list of captured commands
- `Filters` ([]string): Command names to record (empty = record all)
- `ShimDir` (string): Temporary directory path for shim executables
- `LogFile` (string): Path to JSONL log file
- `Metadata` (SessionMetadata): User-provided metadata for scenario

**Validation Rules**:
- `StartTime` must be before `EndTime` (when session is complete)
- `Commands` preserves insertion order (temporal sequence)
- `ShimDir` must be writable directory
- `LogFile` must be writable file path

**Relationships**:
- Contains multiple `RecordedCommand` instances
- Uses `SessionMetadata` for output YAML generation
- Produces one `scenario.Scenario` on conversion

**State Transitions**:
```
[Created] → [Recording] → [Completed] → [Converted]
    ↓          ↓              ↓             ↓
  Setup    Capture       Finalize      Generate YAML
```

**Lifecycle**:
1. **Created**: Session initialized, shim directory created
2. **Recording**: Commands being executed and logged
3. **Completed**: User command finished, session finalized
4. **Converted**: JSONL parsed, YAML generated

---

### SessionMetadata

User-provided metadata for generated scenario file.

**Fields**:
- `Name` (string): Scenario name (required, auto-generated if not provided)
- `Description` (string): Scenario description (optional)
- `RecordedAt` (time.Time): Timestamp when recording was created

**Validation Rules**:
- `Name` must be non-empty after trimming whitespace
- `Description` can be empty
- `RecordedAt` set automatically

**Relationships**:
- Embedded in `RecordingSession`
- Maps to `scenario.Meta` during conversion

**Default Values**:
- `Name`: `"recorded-session-{timestamp}"` format if not provided
- `Description`: Empty string if not provided
- `RecordedAt`: Set to session start time

---

### RecordingLog

Represents the JSONL log file structure for parsing recorded commands.

**Fields**:
- `Entries` ([]RecordingEntry): Parsed log entries in order
- `FilePath` (string): Path to JSONL file

**Validation Rules**:
- Each line must be valid JSON
- Required fields: `timestamp`, `argv`, `exit`
- `stdout` and `stderr` default to empty strings if missing

**Relationships**:
- Parsed from JSONL file on disk
- Converted to `[]RecordedCommand` for session

**Format** (per line):
```json
{
  "timestamp": "2026-02-06T14:30:22Z",
  "argv": ["kubectl", "get", "pods"],
  "exit": 0,
  "stdout": "NAME    READY   STATUS...\n",
  "stderr": ""
}
```

---

### RecordingEntry

Single entry in JSONL log (internal representation).

**Fields**:
- `Timestamp` (string): ISO 8601 timestamp
- `Argv` ([]string): Command arguments
- `Exit` (int): Exit code
- `Stdout` (string): Standard output
- `Stderr` (string): Standard error

**Validation Rules**:
- Must unmarshal from JSON
- `argv` must be array of strings
- `exit` must be integer

**Relationships**:
- Parsed from one line of JSONL
- Converts to `RecordedCommand` with type transformation

---

## Conversion Logic

### RecordedCommand → scenario.Step

```
RecordedCommand {
    Argv:     ["kubectl", "get", "pods"]
    ExitCode: 0
    Stdout:   "NAME    READY...\n"
    Stderr:   ""
}
↓
scenario.Step {
    Match: scenario.Match{
        Argv: ["kubectl", "get", "pods"]
    }
    Respond: scenario.Response{
        Exit:   0
        Stdout: "NAME    READY...\n"
        Stderr: ""
    }
}
```

### RecordingSession → scenario.Scenario

```
RecordingSession {
    Commands: [RecordedCommand1, RecordedCommand2, ...]
    Metadata: SessionMetadata{Name: "demo", Description: "..."}
}
↓
scenario.Scenario {
    Meta: scenario.Meta{
        Name:        "demo"
        Description: "..."
    }
    Steps: []scenario.Step{step1, step2, ...}
}
```

### JSONL → RecordingLog → RecordingSession

```
File: recording.jsonl
{"timestamp":"...","argv":[...],"exit":0,"stdout":"..."}
{"timestamp":"...","argv":[...],"exit":0,"stdout":"..."}

↓ Parse JSONL

RecordingLog {
    Entries: []RecordingEntry{...}
}

↓ Convert types

RecordingSession {
    Commands: []RecordedCommand{...}
}
```

## Entity Relationships Diagram

```
┌─────────────────────┐
│  RecordingSession   │
│  ─────────────────  │
│  - ID               │
│  - StartTime        │
│  - EndTime          │
│  - Filters          │
│  - ShimDir          │
│  - LogFile          │
│  ─────────────────  │
│  + Record()         │
│  + Finalize()       │
│  + Convert()        │
└──────────┬──────────┘
           │ contains
           │ 1..*
           ▼
┌─────────────────────┐      ┌─────────────────────┐
│  RecordedCommand    │      │  SessionMetadata    │
│  ─────────────────  │      │  ─────────────────  │
│  - Timestamp        │◄─────┤  - Name             │
│  - Argv             │ embeds  - Description      │
│  - ExitCode         │      │  - RecordedAt       │
│  - Stdout           │      └─────────────────────┘
│  - Stderr           │
└──────────┬──────────┘
           │ maps to
           │ 1:1
           ▼
┌─────────────────────┐
│   scenario.Step     │
│  ─────────────────  │
│  - Match            │
│  - Respond          │
└─────────────────────┘
```

## Validation Flow

```
User Input (--output, --name, --description, -- command)
    ↓
Validate flags (output path writable, command exists if filtered)
    ↓
Create RecordingSession
    ↓
Generate shims (validate commands in PATH)
    ↓
Execute user command (capture to JSONL)
    ↓
Parse JSONL → RecordingLog
    ↓
Validate RecordingEntries (JSON schema, exit codes)
    ↓
Convert to RecordingSession.Commands
    ↓
Convert to scenario.Scenario
    ↓
Validate scenario (scenario.Validate())
    ↓
Marshal to YAML
    ↓
Write to output file
```

## Storage Schema

### JSONL Log Format

```jsonl
{"timestamp":"2026-02-06T14:30:22Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"NAME    READY   STATUS             RESTARTS   AGE\nweb-0   0/1     CrashLoopBackOff   5          10m\n","stderr":""}
{"timestamp":"2026-02-06T14:30:25Z","argv":["kubectl","delete","pod","web-0"],"exit":0,"stdout":"pod \"web-0\" deleted\n","stderr":""}
{"timestamp":"2026-02-06T14:30:28Z","argv":["kubectl","get","pods"],"exit":0,"stdout":"NAME    READY   STATUS    RESTARTS   AGE\nweb-0   1/1     Running   0          30s\n","stderr":""}
```

**Constraints**:
- One JSON object per line
- No trailing commas
- Newlines in stdout/stderr must be escaped (`\n`)
- Quotes must be escaped (`\"`)

### YAML Output Format

```yaml
meta:
  name: "demo-session"
  description: "Recorded kubectl workflow"

steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS             RESTARTS   AGE
        web-0   0/1     CrashLoopBackOff   5          10m

  - match:
      argv: ["kubectl", "delete", "pod", "web-0"]
    respond:
      exit: 0
      stdout: "pod \"web-0\" deleted\n"

  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS    RESTARTS   AGE
        web-0   1/1     Running   0          30s
```

## Implementation Notes

### Immutability

- `RecordedCommand` instances are immutable after creation
- `RecordingSession.Commands` is append-only during recording phase
- JSONL log is append-only (no in-place updates)

### Error Handling

- Invalid JSONL entries are logged but don't stop conversion (best-effort recovery)
- Schema validation failures produce detailed error messages with line numbers
- Scenario validation failures include step index for debugging

### Concurrency

- Recording session is single-threaded (commands execute sequentially)
- No concurrent writes to JSONL log (enforced by sequential execution)
- Future: If parallel recording needed, use mutex or channel for log writes
