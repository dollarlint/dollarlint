package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	textStyleError   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true)
	textStyleSuccess = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)).Bold(true)
	textStyleFile    = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6)).Bold(true)
	textStyleKeyword = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))
	textStyleMuted   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
	textStylePointer = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(7))
	textStyleSummary = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
)

func FormatJSON(result Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

func FormatText(result Result, output OutputConfig) string {
	if output.Quiet && !result.HasIssues() {
		return textStyleSuccess.Render(passHeadline(result)) + "\n"
	}
	var builder strings.Builder
	if result.HasIssues() {
		headline := fmt.Sprintf("dollarlint found %d issue%s in %d file%s after %s",
			result.Summary.Issues,
			plural(result.Summary.Issues),
			filesWithIssues(result),
			plural(filesWithIssues(result)),
			formatElapsed(result.Summary.Duration.Duration),
		)
		if result.Summary.Ignored > 0 {
			headline += fmt.Sprintf(" (%d ignored)", result.Summary.Ignored)
		}
		if result.HasWarnings() {
			headline += fmt.Sprintf(" (%d warning%s)", result.Summary.Warnings, plural(result.Summary.Warnings))
		}
		builder.WriteString(textStyleError.Render(headline))
		builder.WriteString("\n")
		writeGroupedIssues(&builder, result, output)
	} else {
		builder.WriteString(textStyleSuccess.Render(passHeadline(result)))
		builder.WriteString("\n")
	}
	if !output.Quiet {
		writeWarnings(&builder, result)
		writeSkipped(&builder, result, output.ShowSkipped)
		writeSummary(&builder, result)
	}
	return builder.String()
}

func passHeadline(result Result) string {
	headline := "dollarlint passed"
	if result.HasWarnings() {
		headline += fmt.Sprintf(" with %d warning%s", result.Summary.Warnings, plural(result.Summary.Warnings))
	}
	headline += fmt.Sprintf(" in %s", formatElapsed(result.Summary.Duration.Duration))
	if result.Summary.Discovered > 0 || result.Summary.Validated > 0 || result.Summary.Skipped > 0 {
		headline += fmt.Sprintf(": %d discovered, %d validated, %d skipped",
			result.Summary.Discovered,
			result.Summary.Validated,
			result.Summary.Skipped,
		)
	}
	return headline
}

func writeGroupedIssues(builder *strings.Builder, result Result, output OutputConfig) {
	grouped := map[string][]Issue{}
	for _, issue := range result.Issues {
		if issue.Ignored {
			continue
		}
		grouped[issue.RelativePath] = append(grouped[issue.RelativePath], issue)
	}
	files := make([]string, 0, len(grouped))
	for file := range grouped {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		issues := grouped[file]
		sortIssues(issues, output)
		widths := issueColumnWidths(issues, output)
		fmt.Fprintf(builder, "\n%s\n", textStyleFile.Render(file))
		for _, issue := range issues {
			writeIssueRow(builder, issue, output, widths)
		}
	}
}

func writeIssueRow(builder *strings.Builder, issue Issue, output OutputConfig, widths textWidths) {
	location := issueLocation(issue, output)
	keyword := fallback(issue.Keyword, "dollarlint")
	messageWidth := 0
	if output.Locations && !output.Verbose {
		messageWidth = widths.Message
	}
	locationCell := styledLocation(location, widths.Location)
	keywordCell := styledCell(textStyleKeyword, keyword, widths.Keyword)
	messageCell := fmt.Sprintf("%-*s", messageWidth, issue.Message)
	fmt.Fprintf(builder, "  %s  %s  %s", locationCell, keywordCell, messageCell)
	if !output.Verbose && issue.InstanceLocation != "" && output.Locations {
		fmt.Fprintf(builder, "  %s", textStylePointer.Render(issue.InstanceLocation))
	}
	builder.WriteString("\n")
	if output.Verbose {
		writeVerboseIssueDetails(builder, issue)
	}
}

func writeVerboseIssueDetails(builder *strings.Builder, issue Issue) {
	if issue.InstanceLocation != "" {
		fmt.Fprintf(builder, "    %s %s\n", textStyleMuted.Render("location:"), textStylePointer.Render(issue.InstanceLocation))
	}
	if issue.Property != "" {
		fmt.Fprintf(builder, "    %s %s\n", textStyleMuted.Render("property:"), issue.Property)
	}
	if issue.KeywordLocation != "" {
		fmt.Fprintf(builder, "    %s %s\n", textStyleMuted.Render("keywordLocation:"), textStylePointer.Render(issue.KeywordLocation))
	}
	if issue.Schema != "" {
		fmt.Fprintf(builder, "    %s %s\n", textStyleMuted.Render("schema:"), issue.Schema)
	}
}

func writeSkipped(builder *strings.Builder, result Result, showSkipped bool) {
	if !showSkipped {
		return
	}
	for _, file := range result.Files {
		if file.Status == StatusSkipped {
			fmt.Fprintf(builder, "\n%s %s %s\n", textStyleMuted.Render("skipped:"), file.RelativePath, textStyleMuted.Render("(no schema)"))
		}
	}
}

func writeWarnings(builder *strings.Builder, result Result) {
	if !result.HasWarnings() {
		return
	}
	builder.WriteString("\nwarnings\n")
	for _, warning := range result.Warnings {
		kind := warning.Kind
		if warning.Source != "" {
			kind = warning.Source
		}
		fmt.Fprintf(builder, "  %s  %s\n", textStyleKeyword.Render(kind), warning.Message)
	}
}

func writeSummary(builder *strings.Builder, result Result) {
	summary := fmt.Sprintf("Summary: %d discovered, %d validated, %d skipped, %d issue%s",
		result.Summary.Discovered,
		result.Summary.Validated,
		result.Summary.Skipped,
		result.Summary.Issues,
		plural(result.Summary.Issues),
	)
	if result.Summary.Ignored > 0 {
		summary += fmt.Sprintf(", %d ignored", result.Summary.Ignored)
	}
	if result.Summary.Warnings > 0 {
		summary += fmt.Sprintf(", %d warning%s", result.Summary.Warnings, plural(result.Summary.Warnings))
	}
	summary += fmt.Sprintf(" in %s", formatElapsed(result.Summary.Duration.Duration))
	builder.WriteString("\n")
	builder.WriteString(textStyleSummary.Render(summary))
	builder.WriteString("\n")
}

func styledCell(style lipgloss.Style, value string, width int) string {
	if width <= 0 {
		return style.Render(value)
	}
	return style.Render(fmt.Sprintf("%-*s", width, value))
}

func styledLocation(location string, width int) string {
	if strings.HasPrefix(location, "/") {
		return styledCell(textStylePointer, location, width)
	}
	return styledCell(textStyleMuted, location, width)
}

type textWidths struct {
	Location int
	Keyword  int
	Message  int
}

func issueColumnWidths(issues []Issue, output OutputConfig) textWidths {
	widths := textWidths{Location: len("location"), Keyword: len("keyword")}
	for _, issue := range issues {
		widths.Location = max(widths.Location, len(issueLocation(issue, output)))
		widths.Keyword = max(widths.Keyword, len(fallback(issue.Keyword, "dollarlint")))
		widths.Message = max(widths.Message, len(issue.Message))
	}
	return widths
}

func issueLocation(issue Issue, output OutputConfig) string {
	if output.Locations && issue.Line > 0 {
		if issue.Column > 0 {
			return fmt.Sprintf("%d:%d", issue.Line, issue.Column)
		}
		return fmt.Sprintf("%d", issue.Line)
	}
	if issue.InstanceLocation != "" {
		return issue.InstanceLocation
	}
	return "/"
}

func sortIssues(issues []Issue, output OutputConfig) {
	if !output.Locations {
		return
	}
	sort.SliceStable(issues, func(i, j int) bool {
		left := issues[i]
		right := issues[j]
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		return left.InstanceLocation < right.InstanceLocation
	})
}

func filesWithIssues(result Result) int {
	seen := map[string]bool{}
	for _, issue := range result.Issues {
		if !issue.Ignored {
			seen[issue.RelativePath] = true
		}
	}
	return len(seen)
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func fallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func formatElapsed(duration time.Duration) string {
	if duration <= 0 {
		return "0s"
	}
	if duration < time.Millisecond {
		return duration.String()
	}
	if duration < time.Second {
		return duration.Round(time.Millisecond).String()
	}
	if duration < time.Minute {
		return duration.Round(10 * time.Millisecond).String()
	}
	return duration.Round(100 * time.Millisecond).String()
}
