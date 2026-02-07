package runner

import (
	"fmt"
	"io"
	"strings"

	"github.com/cli-replay/cli-replay/internal/scenario"
)

// DryRunReport contains the information needed to display a dry-run preview
// of a scenario. Not persisted — used only for formatting output.
type DryRunReport struct {
	ScenarioName    string
	Description     string
	TotalSteps      int
	Steps           []DryRunStep
	Groups          []scenario.GroupRange
	Commands        []string
	Allowlist       []string
	AllowlistIssues []string
	TemplateVars    []string
	SessionTTL      string
}

// DryRunStep contains per-step information for dry-run display.
type DryRunStep struct {
	Index         int
	MatchArgv     string
	Exit          int
	StdoutPreview string
	CallsMin      int
	CallsMax      int
	GroupName     string
	GroupMode     string
	Captures      []string
}

// BuildDryRunReport constructs a DryRunReport from a loaded scenario.
func BuildDryRunReport(scn *scenario.Scenario) *DryRunReport {
	flatSteps := scn.FlatSteps()
	groupRanges := scn.GroupRanges()

	// Build group membership lookup: flat index → GroupRange + Mode
	type groupInfo struct {
		Name string
		Mode string
	}
	groupLookup := make(map[int]*groupInfo)
	for i := range groupRanges {
		gr := &groupRanges[i]
		// Resolve mode from the original group in top-level steps
		mode := "unordered" // default
		if gr.TopIndex < len(scn.Steps) && scn.Steps[gr.TopIndex].Group != nil {
			mode = scn.Steps[gr.TopIndex].Group.Mode
		}
		for j := gr.Start; j < gr.End; j++ {
			groupLookup[j] = &groupInfo{Name: gr.Name, Mode: mode}
		}
	}

	// Extract unique commands
	seen := make(map[string]bool)
	var commands []string
	for _, step := range flatSteps {
		if len(step.Match.Argv) > 0 {
			cmd := step.Match.Argv[0]
			if !seen[cmd] {
				seen[cmd] = true
				commands = append(commands, cmd)
			}
		}
	}

	// Build template vars list
	var templateVars []string
	for k := range scn.Meta.Vars {
		templateVars = append(templateVars, k)
	}

	// Build allowlist
	var allowlist []string
	if scn.Meta.Security != nil {
		allowlist = scn.Meta.Security.AllowedCommands
	}

	// Check allowlist issues: commands not in the allowlist
	var allowlistIssues []string
	if len(allowlist) > 0 {
		allowSet := make(map[string]bool)
		for _, a := range allowlist {
			allowSet[a] = true
		}
		for _, cmd := range commands {
			if !allowSet[cmd] {
				allowlistIssues = append(allowlistIssues, fmt.Sprintf("command %q not in allowlist", cmd))
			}
		}
	}

	// Build per-step info
	steps := make([]DryRunStep, len(flatSteps))
	for i, step := range flatSteps {
		bounds := step.EffectiveCalls()

		// Stdout preview
		preview := stdoutPreview(step)

		// Group membership
		var groupName, groupMode string
		if gi, ok := groupLookup[i]; ok {
			groupName = gi.Name
			groupMode = gi.Mode
		}

		// Capture identifiers
		var captures []string
		for k := range step.Respond.Capture {
			captures = append(captures, k)
		}

		steps[i] = DryRunStep{
			Index:         i,
			MatchArgv:     strings.Join(step.Match.Argv, " "),
			Exit:          step.Respond.Exit,
			StdoutPreview: preview,
			CallsMin:      bounds.Min,
			CallsMax:      bounds.Max,
			GroupName:     groupName,
			GroupMode:     groupMode,
			Captures:      captures,
		}
	}

	// Session TTL
	var sessionTTL string
	if scn.Meta.Session != nil && scn.Meta.Session.TTL != "" {
		sessionTTL = scn.Meta.Session.TTL
	}

	return &DryRunReport{
		ScenarioName:    scn.Meta.Name,
		Description:     scn.Meta.Description,
		TotalSteps:      len(flatSteps),
		Steps:           steps,
		Groups:          groupRanges,
		Commands:        commands,
		Allowlist:       allowlist,
		AllowlistIssues: allowlistIssues,
		TemplateVars:    templateVars,
		SessionTTL:      sessionTTL,
	}
}

// stdoutPreview returns a preview string for dry-run display.
// If stdout_file is set, returns "[file: path]".
// Otherwise, returns first 80 chars of stdout (or empty).
func stdoutPreview(step scenario.Step) string {
	if step.Respond.StdoutFile != "" {
		return fmt.Sprintf("[file: %s]", step.Respond.StdoutFile)
	}
	s := step.Respond.Stdout
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

// FormatDryRunReport writes a human-readable dry-run report to the writer.
func FormatDryRunReport(report *DryRunReport, w io.Writer) error {
	// Header
	_, _ = fmt.Fprintf(w, "Scenario: %s\n", report.ScenarioName)
	if report.Description != "" {
		_, _ = fmt.Fprintf(w, "Description: %s\n", report.Description)
	}
	_, _ = fmt.Fprintf(w, "Steps: %d | Groups: %d | Commands: %d\n",
		report.TotalSteps, len(report.Groups), len(report.Commands))

	// Template vars
	if len(report.TemplateVars) > 0 {
		_, _ = fmt.Fprintf(w, "\nTemplate Variables: %s\n", strings.Join(report.TemplateVars, ", "))
	}

	// Session TTL
	if report.SessionTTL != "" {
		_, _ = fmt.Fprintf(w, "Session TTL: %s\n", report.SessionTTL)
	}

	// Allowlist
	if len(report.Allowlist) > 0 {
		_, _ = fmt.Fprintf(w, "Allowlist: %s\n", strings.Join(report.Allowlist, ", "))
	}

	// Separator
	sep := strings.Repeat("\u2500", 60)
	_, _ = fmt.Fprintf(w, "\n%s\n", sep)
	_, _ = fmt.Fprintf(w, " %-4s %-44s %-7s %s\n", "#", "Command / Match Pattern", "Calls", "Group")
	_, _ = fmt.Fprintf(w, "%s\n", sep)

	// Steps
	for _, step := range report.Steps {
		// Format calls bounds
		callsStr := formatCallBounds(step.CallsMin, step.CallsMax)

		// Format group
		groupStr := "\u2014"
		if step.GroupName != "" {
			groupStr = fmt.Sprintf("%s (%s)", step.GroupName, step.GroupMode)
		}

		_, _ = fmt.Fprintf(w, " %-4d %-44s %-7s %s\n",
			step.Index+1, truncate(step.MatchArgv, 44), callsStr, groupStr)

		// Detail line
		detailParts := []string{fmt.Sprintf("exit %d", step.Exit)}
		if step.StdoutPreview != "" {
			detailParts = append(detailParts, fmt.Sprintf("stdout: %s", step.StdoutPreview))
		}
		_, _ = fmt.Fprintf(w, "     \u2192 %s\n", strings.Join(detailParts, " | "))

		// Capture line
		if len(step.Captures) > 0 {
			_, _ = fmt.Fprintf(w, "     \u21b3 captures: %s\n", strings.Join(step.Captures, ", "))
		}
	}

	_, _ = fmt.Fprintf(w, "%s\n", sep)

	// Allowlist validation
	if len(report.Allowlist) > 0 {
		_, _ = fmt.Fprintln(w)
		if len(report.AllowlistIssues) == 0 {
			_, _ = fmt.Fprintln(w, "\u2713 All commands match allowlist")
		} else {
			for _, issue := range report.AllowlistIssues {
				_, _ = fmt.Fprintf(w, "\u2717 %s\n", issue)
			}
		}
	}

	_, _ = fmt.Fprintln(w, "\u2713 No validation errors")

	return nil
}

// formatCallBounds formats the call bounds for display.
func formatCallBounds(min, max int) string {
	maxStr := "\u221e"
	if max > 0 {
		maxStr = fmt.Sprintf("%d", max)
	}
	return fmt.Sprintf("[%d,%s)", min, maxStr)
}

// truncate truncates a string to maxLen, adding "..." if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
