package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func FormatJSON(result Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

func FormatText(result Result, output OutputConfig) string {
	if output.Quiet && !result.HasIssues() {
		return fmt.Sprintf("dollarlint passed in %s\n", formatElapsed(result.Summary.Duration.Duration))
	}
	var builder strings.Builder
	if result.HasIssues() {
		fmt.Fprintf(&builder, "dollarlint found %d issue%s in %d file%s after %s",
			result.Summary.Issues,
			plural(result.Summary.Issues),
			filesWithIssues(result),
			plural(filesWithIssues(result)),
			formatElapsed(result.Summary.Duration.Duration),
		)
		if result.Summary.Ignored > 0 {
			fmt.Fprintf(&builder, " (%d ignored)", result.Summary.Ignored)
		}
		builder.WriteString("\n")
		writeGroupedIssues(&builder, result, output)
	} else {
		fmt.Fprintf(&builder, "dollarlint passed in %s: %d discovered, %d validated, %d skipped\n",
			formatElapsed(result.Summary.Duration.Duration),
			result.Summary.Discovered,
			result.Summary.Validated,
			result.Summary.Skipped,
		)
	}
	if !output.Quiet {
		writeSkipped(&builder, result, output.ShowSkipped)
		writeSummary(&builder, result)
	}
	return builder.String()
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
		fmt.Fprintf(builder, "\n%s\n", file)
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
	fmt.Fprintf(builder, "  %-*s  %-*s  %-*s",
		widths.Location,
		location,
		widths.Keyword,
		keyword,
		messageWidth,
		issue.Message,
	)
	if !output.Verbose && issue.InstanceLocation != "" && output.Locations {
		fmt.Fprintf(builder, "  %s", issue.InstanceLocation)
	}
	builder.WriteString("\n")
	if output.Verbose {
		writeVerboseIssueDetails(builder, issue)
	}
}

func writeVerboseIssueDetails(builder *strings.Builder, issue Issue) {
	if issue.InstanceLocation != "" {
		fmt.Fprintf(builder, "    location: %s\n", issue.InstanceLocation)
	}
	if issue.Property != "" {
		fmt.Fprintf(builder, "    property: %s\n", issue.Property)
	}
	if issue.KeywordLocation != "" {
		fmt.Fprintf(builder, "    keywordLocation: %s\n", issue.KeywordLocation)
	}
	if issue.Schema != "" {
		fmt.Fprintf(builder, "    schema: %s\n", issue.Schema)
	}
}

func writeSkipped(builder *strings.Builder, result Result, showSkipped bool) {
	if !showSkipped {
		return
	}
	for _, file := range result.Files {
		if file.Status == StatusSkipped {
			fmt.Fprintf(builder, "\nskipped: %s (no schema)\n", file.RelativePath)
		}
	}
}

func writeSummary(builder *strings.Builder, result Result) {
	fmt.Fprintf(builder, "\nSummary: %d discovered, %d validated, %d skipped, %d issue%s",
		result.Summary.Discovered,
		result.Summary.Validated,
		result.Summary.Skipped,
		result.Summary.Issues,
		plural(result.Summary.Issues),
	)
	if result.Summary.Ignored > 0 {
		fmt.Fprintf(builder, ", %d ignored", result.Summary.Ignored)
	}
	fmt.Fprintf(builder, " in %s", formatElapsed(result.Summary.Duration.Duration))
	builder.WriteString("\n")
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
