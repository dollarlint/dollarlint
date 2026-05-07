package dollarlint

import (
	"encoding/json"
	"fmt"
	"strings"
)

func FormatJSON(result Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

func FormatText(result Result, showSkipped bool) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "dollarlint: %d discovered, %d validated, %d skipped, %d issues",
		result.Summary.Discovered,
		result.Summary.Validated,
		result.Summary.Skipped,
		result.Summary.Issues,
	)
	if result.Summary.Ignored > 0 {
		fmt.Fprintf(&builder, " (%d ignored)", result.Summary.Ignored)
	}
	builder.WriteString("\n")
	for _, issue := range result.Issues {
		if issue.Ignored {
			continue
		}
		fmt.Fprintf(&builder, "\n%s: %s", issue.RelativePath, issue.Message)
		if issue.Keyword != "" {
			fmt.Fprintf(&builder, "\n  keyword: %s", issue.Keyword)
		}
		if issue.Property != "" {
			fmt.Fprintf(&builder, "\n  property: %s", issue.Property)
		}
		if issue.InstanceLocation != "" {
			fmt.Fprintf(&builder, "\n  location: %s", issue.InstanceLocation)
		}
		builder.WriteString("\n")
	}
	if showSkipped {
		for _, file := range result.Files {
			if file.Status == StatusSkipped {
				fmt.Fprintf(&builder, "\nskipped: %s (no schema)\n", file.RelativePath)
			}
		}
	}
	return builder.String()
}
