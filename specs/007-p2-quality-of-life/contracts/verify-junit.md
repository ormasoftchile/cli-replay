# Contract: Verify JUnit XML Output

**Feature**: 007-p2-quality-of-life (FR-004, FR-005, FR-007)  
**Date**: 2026-02-07

## Trigger

```bash
cli-replay verify scenario.yaml --format junit
```

## Output Channel

**stdout** — valid JUnit XML document, suitable for CI test report ingestion.

## XML Structure

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="cli-replay" tests="{N}" failures="{F}" errors="0" time="{T}">
  <testsuite name="{scenario.meta.name}" tests="{N}" failures="{F}" errors="0"
             skipped="{S}" time="{T}" timestamp="{ISO8601}">
    <testcase name="step[{i}]: {argv_label}" classname="{scenario_file}" time="0.000">
      <!-- present only for failed steps -->
      <failure message="{description}" type="VerificationFailure">
        called {count} times, minimum {min} required
      </failure>
    </testcase>
    <testcase name="step[{i}]: {argv_label}" classname="{scenario_file}" time="0.000">
      <!-- present only for optional (min=0) uncalled steps -->
      <skipped message="optional step (min=0), not called"/>
    </testcase>
  </testsuite>
</testsuites>
```

## Attribute Mapping

| XML Attribute | Source |
|--------------|--------|
| `testsuites.name` | `"cli-replay"` (constant) |
| `testsuites.tests` | `len(FlatSteps())` |
| `testsuites.failures` | count of steps where `call_count < min` |
| `testsuite.name` | `scenario.Meta.Name` |
| `testsuite.timestamp` | `state.LastUpdated` in ISO 8601 |
| `testcase.name` | `step[{i}]: {argv_label}` — for group steps: `[group:{name}] step[{i}]: {argv_label}` |
| `testcase.classname` | Scenario file path (relative or absolute as provided) |
| `testcase.time` | `"0.000"` (cli-replay doesn't track per-step timing) |
| `failure.message` | Short description: `"called {n} times, minimum {min} required"` |
| `failure.type` | `"VerificationFailure"` |
| `skipped.message` | `"optional step (min=0), not called"` |

## Example: All Steps Passed

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="cli-replay" tests="3" failures="0" errors="0" time="0.000">
  <testsuite name="deploy-app" tests="3" failures="0" errors="0" skipped="0"
             time="0.000" timestamp="2026-02-07T10:30:00Z">
    <testcase name="step[0]: git status" classname="scenario.yaml" time="0.000"/>
    <testcase name="step[1]: kubectl get pods" classname="scenario.yaml" time="0.000"/>
    <testcase name="step[2]: kubectl apply -f app.yaml" classname="scenario.yaml" time="0.000"/>
  </testsuite>
</testsuites>
```

## Example: With Groups and Failures

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="cli-replay" tests="4" failures="1" errors="0" time="0.000">
  <testsuite name="deploy-app" tests="4" failures="1" errors="0" skipped="0"
             time="0.000" timestamp="2026-02-07T10:30:00Z">
    <testcase name="step[0]: git status" classname="scenario.yaml" time="0.000"/>
    <testcase name="[group:pre-flight] step[1]: az account show" classname="scenario.yaml" time="0.000"/>
    <testcase name="[group:pre-flight] step[2]: docker info" classname="scenario.yaml" time="0.000">
      <failure message="called 0 times, minimum 1 required" type="VerificationFailure">called 0 times, minimum 1 required</failure>
    </testcase>
    <testcase name="step[3]: kubectl apply -f app.yaml" classname="scenario.yaml" time="0.000"/>
  </testsuite>
</testsuites>
```

## Example: No State File

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="cli-replay" tests="0" failures="0" errors="1" time="0.000">
  <testsuite name="{scenario.meta.name}" tests="0" failures="0" errors="1" skipped="0"
             time="0.000" timestamp="{now}">
    <testcase name="state" classname="{scenario_file}" time="0.000">
      <error message="no state found" type="StateError">no state file found for scenario</error>
    </testcase>
  </testsuite>
</testsuites>
```

## Go Struct Mapping

```go
type JUnitTestSuites struct {
    XMLName  xml.Name         `xml:"testsuites"`
    Name     string           `xml:"name,attr"`
    Tests    int              `xml:"tests,attr"`
    Failures int              `xml:"failures,attr"`
    Errors   int              `xml:"errors,attr"`
    Time     string           `xml:"time,attr"`
    Suites   []JUnitTestSuite `xml:"testsuite"`
}

type JUnitTestSuite struct {
    Name      string          `xml:"name,attr"`
    Tests     int             `xml:"tests,attr"`
    Failures  int             `xml:"failures,attr"`
    Errors    int             `xml:"errors,attr"`
    Skipped   int             `xml:"skipped,attr"`
    Time      string          `xml:"time,attr"`
    Timestamp string          `xml:"timestamp,attr"`
    Cases     []JUnitTestCase `xml:"testcase"`
}

type JUnitTestCase struct {
    Name      string        `xml:"name,attr"`
    Classname string        `xml:"classname,attr"`
    Time      string        `xml:"time,attr"`
    Failure   *JUnitFailure `xml:"failure,omitempty"`
    Skipped   *JUnitSkipped `xml:"skipped,omitempty"`
}

type JUnitFailure struct {
    Message string `xml:"message,attr"`
    Type    string `xml:"type,attr"`
    Content string `xml:",chardata"`
}

type JUnitSkipped struct {
    Message string `xml:"message,attr"`
}
```

## Compatibility

Validated against:
- Jenkins JUnit plugin XSD
- Azure DevOps `PublishTestResults@2` with `testResultsFormat: 'JUnit'`
- GitHub Actions `publish-unit-test-result-action`
- gotestsum / go-junit-report output patterns

## Exit Codes

Same as JSON format (see [verify-json.md](verify-json.md)).
