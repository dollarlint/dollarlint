package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	ansiEscapeRE     = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

func FormatJSON(result Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

const BundleFormatVersion = 1

type BundleOutput struct {
	FormatVersion int                `json:"formatVersion"`
	JSON          Result             `json:"json"`
	SARIF         json.RawMessage    `json:"sarif"`
	Styled        BundleStyledOutput `json:"styled"`
}

type BundleStyledOutput struct {
	Plain     string       `json:"plain"`
	ANSI      string       `json:"ansi"`
	Options   OutputConfig `json:"options"`
	Truncated bool         `json:"truncated"`
}

func FormatBundle(result Result, output OutputConfig) ([]byte, error) {
	sarif, err := FormatSARIF(result)
	if err != nil {
		return nil, err
	}
	styled := FormatText(result, output)
	bundle := BundleOutput{
		FormatVersion: BundleFormatVersion,
		JSON:          result,
		SARIF:         json.RawMessage(sarif),
		Styled: BundleStyledOutput{
			Plain:   StripANSI(styled),
			ANSI:    styled,
			Options: output,
		},
	}
	return json.MarshalIndent(bundle, "", "  ")
}

func StripANSI(text string) string {
	return ansiEscapeRE.ReplaceAllString(text, "")
}

func FormatText(result Result, output OutputConfig) string {
	if output.Quiet && !result.HasIssues() {
		return textStyleSuccess.Render(passHeadline(result)) + "\n"
	}
	var builder strings.Builder
	if result.HasIssues() {
		headline := fmt.Sprintf("dollarlint found %s in %d file%s after %s",
			issueCountLabel(result),
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
	keyword := fallback(issue.Keyword, "issue")
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
	if issue.Hint != "" {
		fmt.Fprintf(builder, "    %s %s\n", textStyleMuted.Render("hint:"), issue.Hint)
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
	if issue.SchemaSource != "" {
		fmt.Fprintf(builder, "    %s %s\n", textStyleMuted.Render("schemaSource:"), issue.SchemaSource)
	}
	if issue.Schema != "" {
		fmt.Fprintf(builder, "    %s %s\n", textStyleMuted.Render("schema:"), issue.Schema)
	}
}

func writeSkipped(builder *strings.Builder, result Result, showSkipped bool) {
	if !showSkipped {
		return
	}
	groups := skippedFileGroups(result.Files)
	if len(groups) == 0 {
		return
	}
	builder.WriteString("\n")
	builder.WriteString(textStyleMuted.Render("skipped files"))
	builder.WriteString("\n")
	for _, group := range groups {
		fmt.Fprintf(builder, "  %s %s %s\n",
			textStyleMuted.Render(skipImportanceLabel(group.Importance)+":"),
			skipClassLabel(group.Class),
			textStyleMuted.Render("("+skipReasonLabel(group.Reason)+")"),
		)
		if group.Detail != "" {
			fmt.Fprintf(builder, "    %s %s\n", textStyleMuted.Render("why:"), group.Detail)
		}
		for _, file := range group.Files {
			fmt.Fprintf(builder, "    %s\n", file)
		}
	}
}

type skippedFileGroup struct {
	Importance string
	Class      string
	Reason     string
	Detail     string
	Files      []string
}

func skippedFileGroups(files []FileResult) []skippedFileGroup {
	indexes := map[string]int{}
	var groups []skippedFileGroup
	for _, file := range files {
		if file.Status == StatusSkipped {
			info := normalizedSkipInfo(file)
			key := strings.Join([]string{info.Importance, info.Class, info.Reason, info.Detail}, "\x00")
			index, ok := indexes[key]
			if !ok {
				index = len(groups)
				indexes[key] = index
				groups = append(groups, skippedFileGroup{
					Importance: info.Importance,
					Class:      info.Class,
					Reason:     info.Reason,
					Detail:     info.Detail,
				})
			}
			groups[index].Files = append(groups[index].Files, resultPath(file.RelativePath, file.Path))
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		left := groups[i]
		right := groups[j]
		if skipImportanceRank(left.Importance) != skipImportanceRank(right.Importance) {
			return skipImportanceRank(left.Importance) < skipImportanceRank(right.Importance)
		}
		if left.Class != right.Class {
			return left.Class < right.Class
		}
		if left.Reason != right.Reason {
			return left.Reason < right.Reason
		}
		return left.Detail < right.Detail
	})
	for i := range groups {
		sort.Strings(groups[i].Files)
	}
	return groups
}

func normalizedSkipInfo(file FileResult) skipClassification {
	info := skipClassification{
		Reason:     file.SkipReason,
		Class:      file.SkipClass,
		Importance: file.SkipImportance,
		Detail:     file.SkipDetail,
	}
	if info.Reason == "" {
		info.Reason = SkipReasonNoSchema
	}
	if info.Class == "" {
		info.Class = SkipClassUnknown
	}
	if info.Importance == "" {
		info.Importance = SkipImportanceMedium
	}
	if info.Detail == "" {
		info.Detail = "schema-less JSON/YAML/TOML file"
	}
	return info
}

func skipImportanceRank(value string) int {
	switch value {
	case SkipImportanceHigh:
		return 0
	case SkipImportanceMedium:
		return 1
	case SkipImportanceLow:
		return 2
	default:
		return 3
	}
}

func skipImportanceLabel(value string) string {
	switch value {
	case SkipImportanceHigh:
		return "high signal"
	case SkipImportanceMedium:
		return "medium signal"
	case SkipImportanceLow:
		return "low signal"
	default:
		return "unknown signal"
	}
}

func skipClassLabel(value string) string {
	switch value {
	case SkipClassApplicationData:
		return "application data"
	case SkipClassExternalCatalog:
		return "external catalog"
	case SkipClassLocaleData:
		return "locale data"
	case SkipClassLockfile:
		return "dependency lockfile"
	case SkipClassRepoManagement:
		return "repo-management config"
	case SkipClassTestData:
		return "test or fixture data"
	case SkipClassUnsupportedConfig:
		return "unsupported config"
	default:
		return "unknown/custom file"
	}
}

func skipReasonLabel(value string) string {
	switch value {
	case SkipReasonCatalogSchemaUnavailable:
		return "catalog schema unavailable"
	case SkipReasonNoSchema:
		return "no schema"
	default:
		return fallback(value, "unknown reason")
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
	summary := fmt.Sprintf("Summary: %d discovered, %d validated, %d skipped, %s",
		result.Summary.Discovered,
		result.Summary.Validated,
		result.Summary.Skipped,
		issueCountLabel(result),
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
		widths.Keyword = max(widths.Keyword, len(fallback(issue.Keyword, "issue")))
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

type issueCounts struct {
	Total      int
	Parsing    int
	Validation int
	Schema     int
	Coverage   int
}

func countIssues(result Result) issueCounts {
	counts := issueCounts{}
	for _, issue := range result.Issues {
		if issue.Ignored {
			continue
		}
		addIssueCounts(&counts, issue)
	}
	if counts.Total == 0 && result.Summary.Issues.Total > 0 {
		counts.Total = result.Summary.Issues.Total
		counts.Parsing = result.Summary.Issues.Parsing
		counts.Validation = result.Summary.Issues.Validation
		counts.Schema = result.Summary.Issues.Schema
		counts.Coverage = result.Summary.Issues.Coverage
	}
	return counts
}

func addIssueCounts(counts *issueCounts, issue Issue) {
	counts.Total++
	switch issue.Keyword {
	case issueKeywordParse:
		counts.Parsing++
	case issueKeywordSchema:
		counts.Schema++
	case "schemaCoverage":
		counts.Coverage++
	default:
		counts.Validation++
	}
}

func issueCountLabel(result Result) string {
	counts := countIssues(result)
	if counts.Total == 0 {
		return "0 issues"
	}
	parts := issueCategoryLabels(counts)
	if len(parts) == 1 {
		return singleIssueCategoryLabel(counts)
	}
	return fmt.Sprintf("%d issues (%s)", counts.Total, strings.Join(parts, ", "))
}

func singleIssueCategoryLabel(counts issueCounts) string {
	switch {
	case counts.Parsing > 0:
		return fmt.Sprintf("%d parsing issue%s", counts.Parsing, plural(counts.Parsing))
	case counts.Validation > 0:
		return fmt.Sprintf("%d validation issue%s", counts.Validation, plural(counts.Validation))
	case counts.Schema > 0:
		return fmt.Sprintf("%d schema issue%s", counts.Schema, plural(counts.Schema))
	case counts.Coverage > 0:
		return fmt.Sprintf("%d coverage issue%s", counts.Coverage, plural(counts.Coverage))
	default:
		return fmt.Sprintf("%d issue%s", counts.Total, plural(counts.Total))
	}
}

func issueCategoryLabels(counts issueCounts) []string {
	var parts []string
	if counts.Parsing > 0 {
		parts = append(parts, fmt.Sprintf("%d parsing", counts.Parsing))
	}
	if counts.Validation > 0 {
		parts = append(parts, fmt.Sprintf("%d validation", counts.Validation))
	}
	if counts.Schema > 0 {
		parts = append(parts, fmt.Sprintf("%d schema", counts.Schema))
	}
	if counts.Coverage > 0 {
		parts = append(parts, fmt.Sprintf("%d coverage", counts.Coverage))
	}
	return parts
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
