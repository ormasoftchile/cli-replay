package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/spf13/cobra"
)

// ValidationResult represents the validation outcome for a single scenario file.
type ValidationResult struct {
	File   string   `json:"file"`
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

var validateFormatFlag string

var validateCmd = &cobra.Command{
	Use:   "validate <file>...",
	Short: "Validate scenario files for schema and semantic correctness",
	Long: `Validate one or more scenario YAML files without executing them.

Checks schema compliance (required fields, types, ranges) and semantic rules
(no forward capture references, no min > max, no duplicate capture keys,
allowlist consistency, group structure).

Does not create any files, directories, or modify any environment state.

Exit code 0 if all files are valid, 1 if any file has errors.

Formats:
  text   Human-readable output to stderr (default)
  json   Structured JSON to stdout

Examples:
  cli-replay validate scenario.yaml
  cli-replay validate a.yaml b.yaml c.yaml
  cli-replay validate --format json scenario.yaml`,
	Args: cobra.MinimumNArgs(1),
	RunE: runValidate,
}

func init() { //nolint:gochecknoinits // Standard cobra pattern
	validateCmd.Flags().StringVar(&validateFormatFlag, "format", "text",
		"Output format: text, json")
	rootCmd.AddCommand(validateCmd)
}

// runValidate implements the validate command: iterates over file args,
// validates each independently, and outputs results in the chosen format.
func runValidate(_ *cobra.Command, args []string) error {
	// Validate --format flag
	format := strings.ToLower(validateFormatFlag)
	switch format {
	case "text", "json":
		// valid
	default:
		return fmt.Errorf("invalid format %q: valid values are text, json", validateFormatFlag)
	}

	var results []ValidationResult
	hasErrors := false

	for _, path := range args {
		result := validateFile(path)
		results = append(results, result)
		if !result.Valid {
			hasErrors = true
		}
	}

	// Output based on format
	switch format {
	case "text":
		formatValidateText(results)
	case "json":
		if err := formatValidateJSON(results); err != nil {
			return fmt.Errorf("failed to encode JSON output: %w", err)
		}
	}

	if hasErrors {
		os.Exit(1)
	}

	return nil
}

// validateFile validates a single scenario file and returns a ValidationResult.
// It calls scenario.LoadFile() which performs strict YAML parsing and all
// semantic validations. Additionally, it checks that stdout_file and
// stderr_file references exist relative to the scenario directory.
func validateFile(path string) ValidationResult {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ValidationResult{
			File:   path,
			Valid:  false,
			Errors: []string{fmt.Sprintf("failed to resolve path: %v", err)},
		}
	}

	scn, err := scenario.LoadFile(absPath)
	if err != nil {
		return ValidationResult{
			File:   path,
			Valid:  false,
			Errors: []string{err.Error()},
		}
	}

	// Additional validation: check stdout_file/stderr_file existence
	var errs []string
	scenarioDir := filepath.Dir(absPath)
	for i, step := range scn.FlatSteps() {
		if step.Respond.StdoutFile != "" {
			refPath := filepath.Join(scenarioDir, step.Respond.StdoutFile)
			if _, statErr := os.Stat(refPath); errors.Is(statErr, fs.ErrNotExist) {
				errs = append(errs, fmt.Sprintf("step %d: stdout_file %q not found relative to scenario directory",
					i+1, step.Respond.StdoutFile))
			}
		}
		if step.Respond.StderrFile != "" {
			refPath := filepath.Join(scenarioDir, step.Respond.StderrFile)
			if _, statErr := os.Stat(refPath); errors.Is(statErr, fs.ErrNotExist) {
				errs = append(errs, fmt.Sprintf("step %d: stderr_file %q not found relative to scenario directory",
					i+1, step.Respond.StderrFile))
			}
		}
	}

	if len(errs) > 0 {
		return ValidationResult{
			File:   path,
			Valid:  false,
			Errors: errs,
		}
	}

	return ValidationResult{
		File:   path,
		Valid:  true,
		Errors: []string{},
	}
}

// formatValidateText writes human-readable validation results to stderr.
func formatValidateText(results []ValidationResult) {
	validCount := 0
	for _, r := range results {
		if r.Valid {
			validCount++
			fmt.Fprintf(os.Stderr, "✓ %s: valid\n", r.File)
		} else {
			fmt.Fprintf(os.Stderr, "✗ %s:\n", r.File)
			for _, e := range r.Errors {
				fmt.Fprintf(os.Stderr, "  - %s\n", e)
			}
		}
	}

	if len(results) > 1 {
		fmt.Fprintf(os.Stderr, "\nResult: %d/%d files valid\n", validCount, len(results))
	}
}

// formatValidateJSON writes JSON-encoded validation results to stdout.
func formatValidateJSON(results []ValidationResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}
