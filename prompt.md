# Project Specification: cli-replay

## Overview

Build **cli-replay**, an open-source, scenario-driven CLI replay tool that acts as a **fake command-line executable** for testing systems that orchestrate external CLI tools.

cli-replay allows developers to define deterministic CLI behavior using **YAML scenarios**, enabling fast, safe, and reproducible tests for:

* runbooks
* DevOps and SRE automation
* incident tooling
* VS Code extensions that invoke CLIs
* any software that shells out to external commands

The tool works by placing a fake executable earlier in PATH, intercepting CLI invocations, matching them against a scripted scenario, and returning predefined outputs.

Conceptually: **VCR for CLI commands**.

---

## Goals

* Deterministically replay CLI interactions without invoking real tools
* Enable scenario-based testing of multi-step CLI workflows
* Fail fast on unexpected or out-of-order commands
* Be cross-platform, fast, and enterprise-friendly
* Ship as a single static binary

---

## Non-Goals (v0)

* No interactive TUI
* No real CLI execution fallback
* No network mocking
* No daemon or long-running service
* No concurrency guarantees (single-process, sequential execution)

---

## Language & Platform

* Language: Go
* Distribution: single static binary
* Supported OS: Windows, macOS, Linux
* License: MIT or Apache-2.0

---

## Primary Usage Model

cli-replay runs in **fake executable mode**.

During tests:

* cli-replay is symlinked or copied under the name of the real CLI (e.g. kubectl, az, icm)
* its directory is prepended to PATH
* the system under test invokes the CLI normally
* cli-replay intercepts the call and replays scripted behavior

---

## Command Line Interface

Main command:

cli-replay run <scenario.yaml>

Environment variables:

* CLI_REPLAY_SCENARIO: path to scenario file (alternative to CLI arg)
* CLI_REPLAY_TRACE=1: enable verbose trace logging

---

## Scenario File (YAML)

Top-level structure:

meta:
name: "incident remediation - missing replicas"
description: "Simulates unhealthy cluster and recovery flow"
vars:
cluster: prod-eus2

steps:

* match:
  argv: ["kubectl", "get", "pods", "-n", "sql"]
  respond:
  exit: 0
  stdout_file: fixtures/pods_unhealthy.txt

* match:
  argv: ["kubectl", "rollout", "restart", "deployment/sql-agent"]
  respond:
  exit: 0
  stdout: "deployment restarted"

* match:
  argv: ["kubectl", "get", "pods", "-n", "sql"]
  respond:
  exit: 0
  stdout_file: fixtures/pods_healthy.txt

---

## Matching Rules (v0)

* Matching is **strict and ordered**
* Only the **next step** in the scenario is eligible for matching
* argv must match exactly:

  * same length
  * same ordering
  * string equality after templating
* If no match is found, execution fails immediately

---

## Response Behavior

Each step may define:

* exit: integer exit code (required)
* stdout: literal string
* stderr: literal string
* stdout_file: load stdout from file (relative to scenario)
* stderr_file: load stderr from file

Rules:

* stdout/stderr are written exactly as specified
* exit code is returned to caller
* after response, scenario advances to next step

---

## Templating

* Use Go text/template
* Template variables are sourced from:

  * meta.vars
  * environment variables

Example:
stdout: "Cluster {{ .cluster }} is unhealthy"

---

## Error Handling (Strict by Default)

cli-replay MUST fail with a clear error when:

* an unexpected command is invoked
* commands are invoked out of order
* scenario file is malformed
* scenario steps remain unused at process exit

Error messages must include:

* received argv
* expected argv
* scenario name
* step index

---why ha

## Observability

When CLI_REPLAY_TRACE=1 is set:

* print matched step index
* print received argv
* print rendered stdout/stderr
* print exit code

Trace output goes to stderr.

---

## Repository Structure

cli-replay/
├── cmd/
│   └── cli-replay/
│       └── main.go
├── internal/
│   ├── scenario/
│   │   ├── loader.go
│   │   ├── model.go
│   │   └── validate.go
│   ├── matcher/
│   │   └── argv.go
│   ├── runner/
│   │   └── replay.go
│   └── template/
│       └── render.go
├── examples/
│   └── simple.yaml
├── go.mod
├── README.md
└── LICENSE

---

## README Requirements

The README must include:

1. Problem statement
2. Concept explanation (PATH interception)
3. 30-second quickstart example
4. Scenario YAML explanation
5. Limitations
6. Roadmap

---

## Roadmap (Documented but Not Implemented in v0)

* Regex and glob argv matching
* Variable capture from argv
* Scenario state and conditional branching
* Simulated latency
* Unordered / parallel steps
* Record mode (generate scenarios from real CLI execution)
* VS Code extension for authoring scenarios

---

## Quality Bar

* Clear, minimal public API
* Deterministic behavior
* No panics on user error
* Human-readable error messages
* Unit tests for matcher and scenario runner

---

## Success Criteria

* A developer can test a multi-step CLI orchestration without installing the real CLI
* Tests are fast, deterministic, and safe
* cli-replay can be dropped into CI pipelines easily
* The project is understandable from the README alone

---

If you want, next I can:

* compress this into an ultra-tight MVP spec,
* generate the initial Go code skeleton,
* or tailor a version explicitly branded around TSG / ICM while keeping cli-replay generic.

This is **very** solid.
