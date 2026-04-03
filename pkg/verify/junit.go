package verify

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// JUnit XML struct types per contracts/verify-junit.md

// JUnitTestSuites is the root element of JUnit XML output.
type JUnitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a single test suite within the JUnit output.
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

// JUnitTestCase represents a single test case within a test suite.
type JUnitTestCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
	Skipped   *JUnitSkipped `xml:"skipped,omitempty"`
}

// JUnitFailure represents a test case failure.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitSkipped represents a skipped test case.
type JUnitSkipped struct {
	Message string `xml:"message,attr"`
}

// FormatJUnit writes the VerifyResult as JUnit XML to the given writer.
// The scenarioFile parameter is used as the classname attribute.
// The timestamp parameter provides the timestamp for the test suite;
// if zero, the current time is used.
func FormatJUnit(w io.Writer, result *VerifyResult, scenarioFile string, timestamp time.Time) error {
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	// Handle error case (e.g., no state file)
	if result.Error != "" {
		return formatJUnitError(w, result, scenarioFile, timestamp)
	}

	failures := 0
	skipped := 0
	cases := make([]JUnitTestCase, len(result.Steps))

	for i, step := range result.Steps {
		tc := JUnitTestCase{
			Name:      stepTestCaseName(step),
			Classname: scenarioFile,
			Time:      "0.000",
		}

		if !step.Passed {
			failures++
			msg := fmt.Sprintf("called %d times, minimum %d required", step.CallCount, step.Min)
			tc.Failure = &JUnitFailure{
				Message: msg,
				Type:    "VerificationFailure",
				Content: msg,
			}
		} else if step.Min == 0 && step.CallCount == 0 {
			skipped++
			tc.Skipped = &JUnitSkipped{
				Message: "optional step (min=0), not called",
			}
		}

		cases[i] = tc
	}

	suites := JUnitTestSuites{
		Name:     "cli-replay",
		Tests:    result.TotalSteps,
		Failures: failures,
		Errors:   0,
		Time:     "0.000",
		Suites: []JUnitTestSuite{
			{
				Name:      result.Scenario,
				Tests:     result.TotalSteps,
				Failures:  failures,
				Errors:    0,
				Skipped:   skipped,
				Time:      "0.000",
				Timestamp: timestamp.Format(time.RFC3339),
				Cases:     cases,
			},
		},
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(suites); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// formatJUnitError writes JUnit XML for an error state (e.g., no state file).
func formatJUnitError(w io.Writer, result *VerifyResult, scenarioFile string, timestamp time.Time) error {
	suites := JUnitTestSuites{
		Name:     "cli-replay",
		Tests:    0,
		Failures: 0,
		Errors:   1,
		Time:     "0.000",
		Suites: []JUnitTestSuite{
			{
				Name:      result.Scenario,
				Tests:     0,
				Failures:  0,
				Errors:    1,
				Skipped:   0,
				Time:      "0.000",
				Timestamp: timestamp.Format(time.RFC3339),
				Cases: []JUnitTestCase{
					{
						Name:      "state",
						Classname: scenarioFile,
						Time:      "0.000",
						Failure: &JUnitFailure{
							Message: result.Error,
							Type:    "StateError",
							Content: "no state file found for scenario",
						},
					},
				},
			},
		},
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(suites); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// stepTestCaseName builds the JUnit test case name from a StepResult.
// Format: "step[{i}]: {label}" or "[group:{name}] step[{i}]: {label}"
// Note: step.Label already contains the [group:...] prefix for group steps,
// so we strip it to avoid duplication when building the JUnit name format.
func stepTestCaseName(step StepResult) string {
	label := step.Label
	if step.Group != "" {
		// Strip the "[group:xxx] " prefix from label since we add our own
		prefix := fmt.Sprintf("[group:%s] ", step.Group)
		label = strings.TrimPrefix(label, prefix)
		return fmt.Sprintf("[group:%s] step[%d]: %s", step.Group, step.Index, label)
	}
	return fmt.Sprintf("step[%d]: %s", step.Index, label)
}
