package main

import "testing"

func TestNormalizeGitHubRunID(t *testing.T) {
	cases := map[string]string{
		"25714454702": "25714454702",
		"https://github.com/dollarlint/dollarlint/actions/runs/25714454702": "25714454702",
	}
	for input, want := range cases {
		got, err := normalizeGitHubRunID(input)
		if err != nil {
			t.Fatalf("normalizeGitHubRunID(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeGitHubRunID(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeGitHubRunID("not-a-run"); err == nil {
		t.Fatalf("expected invalid run id to fail")
	}
}

func TestGitHubRepoSlugFromRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:dollarlint/dollarlint.git":       "dollarlint/dollarlint",
		"https://github.com/dollarlint/dollarlint.git":   "dollarlint/dollarlint",
		"ssh://git@github.com/dollarlint/dollarlint.git": "dollarlint/dollarlint",
		"github.com:dollarlint/dollarlint":               "dollarlint/dollarlint",
		"https://example.com/dollarlint/dollarlint.git":  "",
		"git@example.com:dollarlint/dollarlint.git":      "",
		"https://github.com/dollarlint":                  "",
	}
	for input, want := range cases {
		if got := githubRepoSlugFromRemote(input); got != want {
			t.Fatalf("githubRepoSlugFromRemote(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExtractRealWorldEntryIDs(t *testing.T) {
	text := "Artifact `reports/agentic-product-testing/2026-05-12-d10a1227c97a91c7/dollarlint.json`\n- Real-world entry: `2026-05-12-d10a1227c97a91c7`"
	ids := extractRealWorldEntryIDs(text)
	if len(ids) != 1 || ids[0] != "2026-05-12-d10a1227c97a91c7" {
		t.Fatalf("ids = %v, want single entry id", ids)
	}
}

func TestRealWorldRemoteResultSummary(t *testing.T) {
	body := `
- **Result counts:** 20 repositories tested; 10 product-signal, 8 behaved reasonably, 2 blocked
- **Coverage:** 996 discovered, 322 validated, 671 skipped, 13 issues, 0 warnings
- **Persisted artifact path:** ` + "`reports/agentic-product-testing/2026-05-12-d10a1227c97a91c7/dollarlint.json`" + `
- **Metadata path:** ` + "`reports/agentic-product-testing/2026-05-12-d10a1227c97a91c7/metadata.json`" + `
- **DollarLint commit:** ` + "`b4d3614`" + `

## Product recommendations
- **high** Group schema-resolution failures by root cause.
- **med** Improve large-repository skip summarization.
`
	summary := realWorldRemoteResultSummary(body)
	if summary["resultCounts"] == "" || summary["coverage"] == "" {
		t.Fatalf("summary missing result counts or coverage: %#v", summary)
	}
	if got := summary["persistedArtifact"]; got != "reports/agentic-product-testing/2026-05-12-d10a1227c97a91c7/dollarlint.json" {
		t.Fatalf("persistedArtifact = %v", got)
	}
	recommendations, ok := summary["productRecommendations"].([]string)
	if !ok || len(recommendations) != 2 {
		t.Fatalf("recommendations = %#v", summary["productRecommendations"])
	}
}
