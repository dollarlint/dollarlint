package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRealWorldHistoryQueriesRepositories(t *testing.T) {
	history := realWorldHistory{
		SchemaVersion: 1,
		Entries: []realWorldEntry{
			{
				ID:    "first",
				Date:  "2026-05-01",
				Title: "First",
				Repositories: []realWorldRepository{
					{Name: "cargo", Ecosystem: "Rust", CloneURL: "https://github.com/rust-lang/cargo.git", Commit: "abc"},
				},
			},
			{
				ID:    "second",
				Date:  "2026-05-09",
				Title: "Second",
				Repositories: []realWorldRepository{
					{Name: "cargo", Ecosystem: "Rust", CloneURL: "https://github.com/rust-lang/cargo.git", Commit: "def"},
					{Name: "django", Ecosystem: "Python", CloneURL: "https://github.com/django/django.git", Commit: "123"},
				},
			},
		},
	}

	tested := realWorldTestedRepos(history)
	if len(tested) != 2 {
		t.Fatalf("tested repos = %+v", tested)
	}
	matches := realWorldRepoMatches(history, "rust-lang/cargo")
	if len(matches) != 1 || matches[0].TestCount != 2 || matches[0].LatestCommit != "def" {
		t.Fatalf("cargo matches = %+v", matches)
	}
	previous := realWorldPreviousEntries(history, realWorldRepository{CloneURL: "https://github.com/django/django"})
	if len(previous) != 1 || previous[0] != "second" {
		t.Fatalf("django previous entries = %+v", previous)
	}
}

func TestRealWorldStartTestingOmitsFullHistoryByDefault(t *testing.T) {
	root := t.TempDir()
	var entries []realWorldEntry
	for i := 0; i < 40; i++ {
		repoName := fmt.Sprintf("repo-%02d", i)
		entries = append(entries, realWorldEntry{
			ID:    fmt.Sprintf("entry-%02d", i),
			Date:  "2026-05-01",
			Title: "Entry",
			Repositories: []realWorldRepository{{
				Name:     repoName,
				CloneURL: "https://github.com/example/" + repoName + ".git",
			}},
		})
	}
	if err := saveRealWorldHistory(root, realWorldHistory{Entries: entries}); err != nil {
		t.Fatal(err)
	}
	server := &repoServer{root: root}
	out, err := server.realWorldStartTesting(context.Background(), realWorldStartTestingArgs{Title: "Compact"})
	if err != nil {
		t.Fatalf("start testing: %v", err)
	}
	if _, ok := out["testedRepos"]; ok {
		t.Fatalf("testedRepos should be omitted by default: %+v", out)
	}
	if out["repoCount"] != 40 {
		t.Fatalf("repoCount = %v", out["repoCount"])
	}
	if _, ok := out["testedReposOmitted"]; !ok {
		t.Fatalf("missing testedReposOmitted hint: %+v", out)
	}
	if out["ok"].(bool) {
		t.Fatalf("start without candidates should route to discovery before reporting ok: %+v", out)
	}
	next := out["nextStep"].(map[string]any)
	if next["tool"] != "real_world_discover_candidates" {
		t.Fatalf("start without candidates next step = %+v", next)
	}

	withRepos, err := server.realWorldStartTesting(context.Background(), realWorldStartTestingArgs{Title: "Compact", IncludeTestedRepos: true, TestedRepoLimit: 3})
	if err != nil {
		t.Fatalf("start testing with tested repos: %v", err)
	}
	testedRepos := withRepos["testedRepos"].([]realWorldTestedRepo)
	if len(testedRepos) != 3 || withRepos["testedReposTruncated"] != true {
		t.Fatalf("tested repo sample len/truncated = %d/%v", len(testedRepos), withRepos["testedReposTruncated"])
	}
}

func TestRealWorldHistoryDefaultsToCompactQueries(t *testing.T) {
	root := t.TempDir()
	history := realWorldHistory{Entries: []realWorldEntry{{
		ID:    "first",
		Date:  "2026-05-01",
		Title: "First",
		Repositories: []realWorldRepository{
			{Name: "cargo", CloneURL: "https://github.com/rust-lang/cargo.git"},
			{Name: "django", CloneURL: "https://github.com/django/django.git"},
		},
	}}}
	if err := saveRealWorldHistory(root, history); err != nil {
		t.Fatal(err)
	}
	server := &repoServer{root: root}
	out, err := server.realWorldHistory(realWorldHistoryArgs{Repositories: []string{"rust-lang/cargo"}})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if _, ok := out["testedRepos"]; ok {
		t.Fatalf("testedRepos should be omitted by default: %+v", out)
	}
	queries := out["queries"].([]map[string]any)
	if len(queries) != 1 || queries[0]["alreadyTested"] != true {
		t.Fatalf("queries = %+v", queries)
	}
	if _, ok := out["testedReposOmitted"]; !ok {
		t.Fatalf("missing omission hint: %+v", out)
	}

	withRepos, err := server.realWorldHistory(realWorldHistoryArgs{IncludeTestedRepos: true, Limit: 1})
	if err != nil {
		t.Fatalf("history with tested repos: %v", err)
	}
	testedRepos := withRepos["testedRepos"].([]realWorldTestedRepo)
	if len(testedRepos) != 1 || withRepos["testedReposTruncated"] != true {
		t.Fatalf("tested repo sample len/truncated = %d/%v", len(testedRepos), withRepos["testedReposTruncated"])
	}
}

func TestRealWorldSelectDiscoveredCandidatesSkipsHistoryAndBatchDuplicates(t *testing.T) {
	history := realWorldHistory{Entries: []realWorldEntry{{
		ID:    "old-run",
		Date:  "2026-05-01",
		Title: "Old",
		Repositories: []realWorldRepository{{
			Name:     "cargo",
			CloneURL: "https://github.com/rust-lang/cargo.git",
		}},
	}}}
	groups := [][]realWorldRepository{
		{
			{Name: "cargo", Ecosystem: "Rust", CloneURL: "https://github.com/rust-lang/cargo.git"},
			{Name: "django", Ecosystem: "Python", CloneURL: "https://github.com/django/django.git"},
		},
		{
			{Name: "django-copy", Ecosystem: "Python", CloneURL: "https://github.com/django/django.git"},
			{Name: "pnpm", Ecosystem: "TypeScript", CloneURL: "https://github.com/pnpm/pnpm.git"},
		},
	}

	selected, omittedHistory, omittedBatchDuplicates := realWorldSelectDiscoveredCandidates(history, groups, 2, false)
	if len(selected) != 2 || normalizedRepoKey(selected[0]) != "django/django" || normalizedRepoKey(selected[1]) != "pnpm/pnpm" {
		t.Fatalf("selected = %+v", selected)
	}
	if len(omittedHistory) != 1 || omittedHistory[0].Name != "cargo" || !omittedHistory[0].AlreadyTested {
		t.Fatalf("omitted history = %+v", omittedHistory)
	}
	if omittedBatchDuplicates != 1 {
		t.Fatalf("omitted batch duplicates = %d", omittedBatchDuplicates)
	}
}

func TestRealWorldSelectDiscoveredCandidatesAllowsIntentionalReruns(t *testing.T) {
	history := realWorldHistory{Entries: []realWorldEntry{{
		ID:    "old-run",
		Date:  "2026-05-01",
		Title: "Old",
		Repositories: []realWorldRepository{{
			Name:     "cargo",
			CloneURL: "https://github.com/rust-lang/cargo.git",
		}},
	}}}
	groups := [][]realWorldRepository{{
		{Name: "cargo", Ecosystem: "Rust", CloneURL: "https://github.com/rust-lang/cargo.git"},
	}}

	selected, omittedHistory, _ := realWorldSelectDiscoveredCandidates(history, groups, 1, true)
	if len(selected) != 1 || selected[0].Name != "cargo" || !selected[0].AlreadyTested {
		t.Fatalf("selected rerun = %+v", selected)
	}
	if len(omittedHistory) != 0 {
		t.Fatalf("omitted history with reruns allowed = %+v", omittedHistory)
	}
}

func TestRealWorldCandidateDiffReplacesDuplicateWithoutFullResubmit(t *testing.T) {
	root := t.TempDir()
	history := realWorldHistory{
		Entries: []realWorldEntry{
			{
				ID:    "first",
				Date:  "2026-05-01",
				Title: "First",
				Repositories: []realWorldRepository{
					{Name: "cargo", Ecosystem: "Rust", CloneURL: "https://github.com/rust-lang/cargo.git"},
				},
			},
		},
	}
	if err := saveRealWorldHistory(root, history); err != nil {
		t.Fatal(err)
	}
	server := &repoServer{root: root}
	ctx := context.Background()
	start, err := server.realWorldStartTesting(ctx, realWorldStartTestingArgs{
		Title: "Diff Candidates",
		Repositories: []realWorldRepository{
			{Name: "cargo", Ecosystem: "Rust", CloneURL: "https://github.com/rust-lang/cargo.git"},
			{Name: "redis", Ecosystem: "C", CloneURL: "https://github.com/redis/redis.git"},
		},
	})
	if err != nil {
		t.Fatalf("start testing: %v", err)
	}
	if start["ok"].(bool) {
		t.Fatalf("start should reject duplicate candidate: %+v", start)
	}
	candidateSetID := start["candidateSetID"].(string)
	if candidateSetID == "" {
		t.Fatalf("missing candidateSetID: %+v", start)
	}
	if path, err := realWorldCandidateSetPath(candidateSetID); err == nil {
		defer os.Remove(path)
	}
	next := start["nextStep"].(map[string]any)
	if next["tool"] != "real_world_update_candidates" {
		t.Fatalf("duplicate next step = %+v", next)
	}

	updated, err := server.realWorldUpdateCandidates(ctx, realWorldUpdateCandidatesArgs{
		CandidateSetID: candidateSetID,
		ExpectedCount:  2,
		Diff: realWorldCandidateDiff{Replace: []realWorldCandidateReplacement{{
			Match: "cargo",
			Repository: realWorldRepository{
				Name:      "django",
				Ecosystem: "Python",
				CloneURL:  "https://github.com/django/django.git",
			},
		}}},
	})
	if err != nil {
		t.Fatalf("update candidates: %v", err)
	}
	if !updated["ok"].(bool) {
		t.Fatalf("updated candidates should be ready: %+v", updated)
	}
	candidates := updated["candidateRepositories"].([]realWorldRepository)
	if len(candidates) != 2 || candidates[0].Name != "django" || candidates[1].Name != "redis" {
		t.Fatalf("updated candidates = %+v", candidates)
	}
	next = updated["nextStep"].(map[string]any)
	if next["tool"] != "real_world_prepare_corpus" {
		t.Fatalf("ready next step = %+v", next)
	}
	suggested := next["suggestedArgs"].(map[string]any)
	if suggested["candidateSetID"] != candidateSetID {
		t.Fatalf("prepare suggested args should use candidateSetID, got %+v", suggested)
	}
	if _, hasRepositories := suggested["repositories"]; hasRepositories {
		t.Fatalf("prepare suggested args should not resubmit repositories: %+v", suggested)
	}

	prepared, err := server.realWorldPrepareCorpus(ctx, mcp.CallToolRequest{}, realWorldPrepareCorpusArgs{
		CandidateSetID: candidateSetID,
		ExpectedCount:  2,
		Clone:          false,
	})
	if err != nil {
		t.Fatalf("prepare corpus: %v", err)
	}
	if !prepared["ok"].(bool) {
		t.Fatalf("prepare from candidate set failed: %+v", prepared)
	}
	defer os.RemoveAll(prepared["corpusDir"].(string))
	defer os.RemoveAll(prepared["cacheDir"].(string))
	repositories := prepared["repositories"].([]realWorldRepository)
	if len(repositories) != 2 || repositories[0].Name != "django" || repositories[1].Name != "redis" {
		t.Fatalf("prepared repositories = %+v", repositories)
	}
	manifest, err := readRealWorldManifest(prepared["manifestPath"].(string))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(manifest.Repositories) != 2 || manifest.Repositories[0].Name != "django" || manifest.Repositories[1].Name != "redis" {
		t.Fatalf("manifest repositories = %+v", manifest.Repositories)
	}
}

func TestReadRealWorldOutputSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	data := `{
  "summary": {
    "discovered": 3,
    "validated": 2,
    "skipped": 1,
    "failed": 1,
    "issues": {"total": 4, "parsing": 1, "validation": 2, "schema": 1, "coverage": 0},
    "ignored": 1,
    "warnings": 2,
    "durationNanos": 99
  },
  "warnings": [{"kind": "schemaCatalogSchemaUnavailable"}]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, warnings, err := readRealWorldOutputSummary(path)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if summary.Discovered != 3 || summary.Issues.Validation != 2 || summary.Ignored != 1 || summary.Duration.Nanos != 99 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(warnings) != 1 || warnings[0]["kind"] != "schemaCatalogSchemaUnavailable" {
		t.Fatalf("warnings = %+v", warnings)
	}

	bundlePath := filepath.Join(dir, "bundle.json")
	bundle := `{
  "formatVersion": 1,
  "json": {
    "summary": {
      "discovered": 1,
      "validated": 1,
      "skipped": 0,
      "failed": 0,
      "issues": {"total": 0, "parsing": 0, "validation": 0, "schema": 0, "coverage": 0},
      "ignored": 0,
      "warnings": 0,
      "durationNanos": 12
    },
    "files": [{"path": "ok.json", "format": "json", "status": "validated"}],
    "issues": [],
    "warnings": []
  },
  "sarif": {"version": "2.1.0", "runs": []},
  "styled": {"plain": "dollarlint passed", "ansi": "\u001b[32mdollarlint passed\u001b[0m", "options": {"locations": true, "showSkipped": true}}
}`
	if err := os.WriteFile(bundlePath, []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, warnings, err = readRealWorldOutputSummary(bundlePath)
	if err != nil {
		t.Fatalf("read bundle summary: %v", err)
	}
	if summary.Discovered != 1 || summary.Duration.Nanos != 12 || len(warnings) != 0 {
		t.Fatalf("bundle summary = %+v warnings=%+v", summary, warnings)
	}
	details, err := readRealWorldOutputDetails(bundlePath)
	if err != nil {
		t.Fatalf("read bundle details: %v", err)
	}
	if details.Styled == nil || !strings.Contains(details.Styled.Plain, "passed") {
		t.Fatalf("bundle styled output = %+v", details.Styled)
	}
}

func TestRealWorldValidationEvidenceIncludesBundleUXCollateral(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "pyproject.toml"), []byte("[tool.ruff.lint]\nignore = [\n    \"UP038\",\n]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "bundle.json")
	longMessage := "value must be one of '" + strings.Repeat("A', '", 600) + "Z'"
	bundle := map[string]any{
		"formatVersion": 1,
		"json": map[string]any{
			"root": repo,
			"summary": map[string]any{
				"discovered": 2,
				"validated":  1,
				"skipped":    1,
				"failed":     0,
				"issues":     map[string]any{"total": 1, "parsing": 0, "validation": 1, "schema": 0, "coverage": 0},
				"ignored":    0,
				"warnings":   0,
			},
			"files": []map[string]any{
				{"path": "pyproject.toml", "format": "toml", "status": "validated", "issues": 1},
				{"path": ".rubocop.yml", "format": "yaml", "status": "skipped"},
			},
			"issues": []map[string]any{{
				"path":             "pyproject.toml",
				"category":         "validation",
				"keyword":          "enum",
				"schemaSource":     "catalog:schemastore:PyProject",
				"instanceLocation": "/tool/ruff/lint/ignore/0",
				"line":             3,
				"column":           5,
				"message":          longMessage,
			}},
			"warnings": []map[string]any{},
		},
		"sarif":  map[string]any{"version": "2.1.0", "runs": []any{}},
		"styled": map[string]any{"plain": "dollarlint found 1 validation issue\npyproject.toml\n", "ansi": "\u001b[31mdollarlint found 1 validation issue\u001b[0m\n", "options": map[string]any{"locations": true, "showSkipped": true}},
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	evidence, err := realWorldValidationEvidence("repo", path)
	if err != nil {
		t.Fatalf("validation evidence: %v", err)
	}
	examples := evidence["exampleIssues"].([]realWorldIssueExample)
	if len(examples) != 1 || !examples[0].MessageTruncated || examples[0].SourceExcerpt == nil {
		t.Fatalf("issue examples = %+v", examples)
	}
	groups := evidence["skippedGroups"].([]realWorldSkippedFileGroup)
	if len(groups) == 0 || groups[0].Class != "tooling-config" || !groups[0].ProductSignal {
		t.Fatalf("skipped groups = %+v", groups)
	}
	preview := evidence["cliPreview"].(map[string]any)
	if !strings.Contains(preview["plain"].(string), "validation issue") {
		t.Fatalf("cli preview = %+v", preview)
	}
	signals := evidence["uxSignals"].([]string)
	if !containsString(signals, "large-enum-message") {
		t.Fatalf("ux signals = %+v", signals)
	}
}

func TestRealWorldValidationEvidenceCapsSkippedSignals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.json")
	files := make([]map[string]any, 0, 30)
	for i := 0; i < 30; i++ {
		files = append(files, map[string]any{
			"path":   fmt.Sprintf("config-%02d.yml", i),
			"format": "yaml",
			"status": "skipped",
		})
	}
	bundle := map[string]any{
		"json": map[string]any{
			"summary": map[string]any{
				"discovered": 30,
				"validated":  0,
				"skipped":    30,
				"failed":     0,
				"issues":     map[string]any{"total": 0, "parsing": 0, "validation": 0, "schema": 0, "coverage": 0},
				"ignored":    0,
				"warnings":   0,
			},
			"files":    files,
			"issues":   []map[string]any{},
			"warnings": []map[string]any{},
		},
		"sarif":  map[string]any{"version": "2.1.0", "runs": []any{}},
		"styled": map[string]any{"plain": "dollarlint skipped 30 files\n"},
	}
	if err := writeJSONFile(path, bundle); err != nil {
		t.Fatal(err)
	}
	evidence, err := realWorldValidationEvidence("repo", path)
	if err != nil {
		t.Fatalf("validation evidence: %v", err)
	}
	signals := evidence["skippedSignals"].([]realWorldSkippedFileSignal)
	if len(signals) != 20 || evidence["skippedSignalsTotal"].(int) != 30 || evidence["skippedSignalsTruncated"].(bool) != true {
		t.Fatalf("skipped signal cap = len %d total %+v truncated %+v", len(signals), evidence["skippedSignalsTotal"], evidence["skippedSignalsTruncated"])
	}
}

func TestTriageRealWorldOutputGroupsIssuesAndWarnings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	data := `{
  "summary": {
    "discovered": 5,
    "validated": 2,
    "skipped": 2,
    "failed": 1,
    "issues": {"total": 4, "parsing": 1, "validation": 3, "schema": 0, "coverage": 0},
    "ignored": 0,
    "warnings": 1,
    "durationNanos": 99
  },
  "files": [
    {"path": "hugo/hugolib/testdata/fakejson.json", "format": "json", "status": "error", "issues": 1},
    {"path": "hugo/docs/.github/workflows/markdownlint.yml", "format": "yaml", "status": "validated", "issues": 1},
    {"path": "hugo/docs/hugo.toml", "format": "toml", "status": "validated", "issues": 2},
    {"path": "nestjs/.circleci/config.yml", "format": "yaml", "status": "skipped"},
    {"path": "real-world-manifest.json", "format": "json", "status": "skipped"}
  ],
  "issues": [
    {"path": "hugo/hugolib/testdata/fakejson.json", "category": "parsing", "keyword": "parse", "message": "invalid GIF"},
    {"path": "hugo/docs/.github/workflows/markdownlint.yml", "category": "validation", "schemaSource": "catalog:schemastore:GitHub Workflow", "keyword": "type", "message": "expected string"},
    {"path": "hugo/docs/hugo.toml", "category": "validation", "schemaSource": "catalog:schemastore:Hugo", "keyword": "additionalProperties", "message": "must not have additional property sites"},
    {"path": "hugo/docs/hugo.toml", "category": "validation", "schemaSource": "catalog:schemastore:Hugo", "keyword": "additionalProperties", "message": "must not have additional property mediatype"}
  ],
  "warnings": [
    {"kind": "schemaCatalogSchemaUnavailable", "path": "nestjs/.circleci/config.yml", "schema": "https://www.schemastore.org/circleciconfig.json", "schemaSource": "catalog:schemastore:CircleCI config.yml", "message": "catalog schema could not be used"}
  ]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	triage, err := triageRealWorldOutput(path, []realWorldRepository{
		{Name: "hugo", CloneURL: "https://github.com/gohugoio/hugo.git", Path: filepath.Join(dir, "hugo")},
		{Name: "nestjs", CloneURL: "https://github.com/nestjs/nest.git", Path: filepath.Join(dir, "nestjs")},
	})
	if err != nil {
		t.Fatalf("triage output: %v", err)
	}
	if triage.Summary == nil || triage.Summary.Issues.Total != 4 {
		t.Fatalf("summary = %+v", triage.Summary)
	}
	if len(triage.WarningGroups) != 1 || triage.WarningGroups[0].Count != 1 {
		t.Fatalf("warning groups = %+v", triage.WarningGroups)
	}
	var fixtureGroup, hugoCatalogGroup bool
	for _, group := range triage.IssueGroups {
		if group.Repository == "hugo" && group.Signal == "fixture-data" && group.Count == 1 && group.FixtureSignal {
			fixtureGroup = true
		}
		if group.Repository == "hugo" && group.Signal == "catalog-schema-validation" && group.SchemaSource == "catalog:schemastore:Hugo" && group.Count == 2 {
			hugoCatalogGroup = true
		}
	}
	if !fixtureGroup || !hugoCatalogGroup {
		t.Fatalf("issue groups = %+v", triage.IssueGroups)
	}
	var hugoRepo *realWorldRepositoryTriage
	for i := range triage.PerRepository {
		if triage.PerRepository[i].Repository == "hugo" {
			hugoRepo = &triage.PerRepository[i]
			break
		}
	}
	if hugoRepo == nil || hugoRepo.IssueCount != 4 || hugoRepo.FixtureIssueCount != 1 || hugoRepo.ProductSignalIssueCount != 3 {
		t.Fatalf("hugo repo triage = %+v", hugoRepo)
	}
	if len(triage.Findings) == 0 || !strings.Contains(triage.Findings[0], "discovered 5 files") {
		t.Fatalf("findings = %+v", triage.Findings)
	}
	if len(triage.ProductRecommendations) == 0 || triage.FinalResponseContract["required"] == "" {
		t.Fatalf("recommendations=%+v finalResponseContract=%+v", triage.ProductRecommendations, triage.FinalResponseContract)
	}
	discussionRepos, ok := triage.DiscussionPacket["repositories"].([]map[string]string)
	if !ok {
		t.Fatalf("discussion repositories = %#v", triage.DiscussionPacket["repositories"])
	}
	if len(discussionRepos) != 2 {
		t.Fatalf("discussion repositories = %+v", discussionRepos)
	}
	if discussionRepos[0]["markdown"] != "[hugo](https://github.com/gohugoio/hugo)" {
		t.Fatalf("hugo discussion repo = %+v", discussionRepos[0])
	}
}

func TestTriageRealWorldOutputRejectsCountMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	data := `{
  "summary": {
    "discovered": 1,
    "validated": 1,
    "skipped": 0,
    "failed": 0,
    "issues": {"total": 2, "parsing": 0, "validation": 2, "schema": 0, "coverage": 0},
    "warnings": 0
  },
  "files": [
    {"path": "repo/config.json", "format": "json", "status": "validated", "issues": 1}
  ],
  "issues": [
    {"path": "repo/config.json", "category": "validation", "keyword": "type", "message": "expected string"}
  ],
  "warnings": []
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := triageRealWorldOutput(path, nil); err == nil || !strings.Contains(err.Error(), "summary.issues.total=2") {
		t.Fatalf("expected count mismatch error, got %v", err)
	}
}

func TestRealWorldArtifactQueryReturnsRecordedCollateral(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "corpus")
	watchPath := filepath.Join(corpus, "redis", "src", "commands", "watch.json")
	if err := os.MkdirAll(filepath.Dir(watchPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(watchPath, []byte("{\"summary\":\"WATCH command\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifactRel := "reports/agentic-product-testing/sample/dollarlint.json"
	artifactPath := filepath.Join(root, artifactRel)
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := `{
  "formatVersion": 1,
  "json": {
    "root": "` + filepath.ToSlash(corpus) + `",
    "summary": {
      "discovered": 4,
      "validated": 2,
      "skipped": 1,
      "failed": 1,
      "issues": {"total": 3, "parsing": 1, "validation": 2, "schema": 0, "coverage": 0},
      "ignored": 0,
      "warnings": 1,
      "durationNanos": 99
    },
    "files": [
      {"path": "redis/src/commands/watch.json", "format": "json", "status": "validated", "issues": 1},
      {"path": "redis/deps/jemalloc/.travis.yml", "format": "yaml", "status": "validated", "issues": 1},
      {"path": "redis/data/domain.json", "format": "json", "status": "skipped", "skipClass": "application-data", "skipReason": "noSchema"},
      {"path": "helm/chart/templates/deploy.yaml", "format": "yaml", "status": "error", "issues": 1}
    ],
    "issues": [
      {"path": "redis/src/commands/watch.json", "category": "validation", "schemaSource": "catalog:schemastore:Grunt Watch", "keyword": "required", "line": 1, "column": 2, "message": "must have required property tasks"},
      {"path": "redis/deps/jemalloc/.travis.yml", "category": "validation", "schemaSource": "catalog:schemastore:Travis", "keyword": "enum", "message": "value must be one of linux, osx"},
      {"path": "helm/chart/templates/deploy.yaml", "category": "parsing", "keyword": "parse", "message": "yaml: invalid map key from Helm template"}
    ],
    "warnings": [
      {"kind": "schemaCatalogSchemaUnavailable", "path": "redis/.circleci/config.yml", "schemaSource": "catalog:schemastore:CircleCI config.yml", "message": "catalog schema could not be used"}
    ]
  },
  "sarif": {"version": "2.1.0", "runs": []},
  "styled": {"plain": "dollarlint found 3 issues\nredis/src/commands/watch.json\n", "ansi": "\u001b[31mdollarlint found 3 issues\u001b[0m\n", "options": {"locations": true}}
}`
	if err := os.WriteFile(artifactPath, []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	history := realWorldHistory{Entries: []realWorldEntry{{
		ID:                      "sample",
		Date:                    "2026-05-09",
		Title:                   "Sample",
		DollarLintRevision:      "abc123",
		Corpus:                  corpus,
		Command:                 "bin/dollarlint validate",
		OutputArtifact:          "/tmp/out.json",
		PersistedOutputArtifact: artifactRel,
		Repositories: []realWorldRepository{{
			Name:     "redis",
			CloneURL: "https://github.com/redis/redis.git",
			Path:     filepath.Join(corpus, "redis"),
		}},
	}}}
	if err := saveRealWorldHistory(root, history); err != nil {
		t.Fatal(err)
	}

	server := &repoServer{root: root}
	out, err := server.queryRealWorldArtifact(realWorldArtifactQueryArgs{
		EntryID:        "sample",
		Repository:     "redis",
		Recommendation: "Tighten generic JSON basename catalog inference for watch.json",
		Limit:          4,
	})
	if err != nil {
		t.Fatalf("query artifact: %v", err)
	}
	summary := out["summary"].(*realWorldResult)
	if summary.Discovered != 3 || summary.Issues.Total != 2 || summary.Issues.Parsing != 0 {
		t.Fatalf("filtered summary = %+v", summary)
	}
	examples := out["recommendationExamples"].(map[string]any)
	issueGroups := examples["issueGroups"].([]realWorldIssueGroup)
	if len(issueGroups) == 0 || !strings.Contains(strings.Join(issueGroups[0].Paths, " "), "watch.json") {
		t.Fatalf("recommendation issue groups = %+v", issueGroups)
	}
	coverage := out["skippedCoverageByRepository"].([]map[string]any)
	if len(coverage) != 1 || coverage[0]["repository"] != "redis" || coverage[0]["skipped"] != 1 {
		t.Fatalf("skipped coverage = %+v", coverage)
	}
	preview := out["cliPreview"].(map[string]any)
	if !strings.Contains(preview["plain"].(string), "dollarlint found") {
		t.Fatalf("cli preview = %+v", preview)
	}
}

func TestRealWorldRecommendationBacklogClustersRecordedRecommendations(t *testing.T) {
	root := t.TempDir()
	history := realWorldHistory{Entries: []realWorldEntry{
		{
			ID:    "first",
			Date:  "2026-05-09",
			Title: "First",
			ProductRecommendations: []realWorldProductRecommendation{
				{
					Strength:       "high",
					Recommendation: "Tighten SchemaStore catalog inference for generic JSON basenames such as watch.json and cluster.json.",
					Rationale:      "Redis command metadata files were validated against unrelated schemas solely because of generic names.",
				},
				{
					Strength:       "low",
					Recommendation: "No product change is recommended from this sweep.",
					Rationale:      "The run behaved reasonably.",
				},
			},
			ValidationFeedback: []realWorldValidationFeedback{{
				Repository: "caddy",
				Outcome:    realWorldFeedbackProductSignal,
				ProductRecommendations: []realWorldProductRecommendation{{
					Strength:       "med",
					Recommendation: "Group repeated `.github/FUNDING.yml` blank-provider findings.",
					Rationale:      "Caddy produced repetitive null provider findings from one GitHub Funding template shape.",
				}},
			}},
		},
		{
			ID:    "second",
			Date:  "2026-05-10",
			Title: "Second",
			ValidationFeedback: []realWorldValidationFeedback{{
				Repository: "scikit-learn",
				Outcome:    realWorldFeedbackProductSignal,
				ProductRecommendations: []realWorldProductRecommendation{{
					Strength:       "low",
					Recommendation: "Group catalog-backed GitHub Funding null provider findings in `.github/FUNDING.yml`.",
					Rationale:      "Eight near-identical null provider errors dominated the result.",
				}},
			}},
		},
	}}
	if err := saveRealWorldHistory(root, history); err != nil {
		t.Fatal(err)
	}
	server := &repoServer{root: root}
	out, err := server.realWorldRecommendationBacklog(realWorldRecommendationBacklogArgs{Limit: 10, MinOccurrences: 1})
	if err != nil {
		t.Fatalf("recommendation backlog: %v", err)
	}
	clusters := out["clusters"].([]realWorldRecommendationBacklogCluster)
	var funding, generic *realWorldRecommendationBacklogCluster
	for i := range clusters {
		switch clusters[i].Key {
		case "github-funding-blank-providers":
			funding = &clusters[i]
		case "schemastore-generic-basename":
			generic = &clusters[i]
		case "no-product-change-recommended-from-sweep-run":
			t.Fatalf("no-change recommendation should be excluded by default: %+v", clusters[i])
		}
	}
	if funding == nil || funding.Count != 2 || funding.HighestStrength != "med" || !containsString(funding.Repositories, "caddy") {
		t.Fatalf("funding cluster = %+v", funding)
	}
	if generic == nil || generic.HighestStrength != "high" || len(generic.SuggestedRegressionFixtures) == 0 {
		t.Fatalf("generic cluster = %+v", generic)
	}

	filtered, err := server.realWorldRecommendationBacklog(realWorldRecommendationBacklogArgs{Limit: 10, MinOccurrences: 2})
	if err != nil {
		t.Fatalf("filtered backlog: %v", err)
	}
	filteredClusters := filtered["clusters"].([]realWorldRecommendationBacklogCluster)
	if len(filteredClusters) != 1 || filteredClusters[0].Key != "github-funding-blank-providers" {
		t.Fatalf("filtered clusters = %+v", filteredClusters)
	}
}

func TestRealWorldInspectCorpusDraftsDependencyPrep(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "corpus")
	cache := filepath.Join(root, "cache")
	output := filepath.Join(root, "out.json")
	remoteRepo := filepath.Join(corpus, "remote")
	localRepo := filepath.Join(corpus, "local")
	for _, dir := range []string{
		filepath.Join(remoteRepo, ".github", "workflows"),
		filepath.Join(localRepo, "config"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(remoteRepo, "package-lock.json"):              `{"lockfileVersion": 3}`,
		filepath.Join(remoteRepo, "package.json"):                   `{"name":"remote"}`,
		filepath.Join(remoteRepo, ".github", "workflows", "ci.yml"): `"$schema": "https://json.schemastore.org/github-workflow.json"`,
		filepath.Join(localRepo, "pnpm-lock.yaml"):                  `lockfileVersion: '9.0'`,
		filepath.Join(localRepo, "package.json"):                    `{"name":"local"}`,
		filepath.Join(localRepo, "config", "tool.json"):             `{"$schema":"./schema.json","enabled":true}`,
		filepath.Join(localRepo, "config", "node-schema.json"):      `{"$schema":"../node_modules/tool/schema.json"}`,
		filepath.Join(localRepo, "config", "schema.json"):           `{"type":"object"}`,
	}
	for path, text := range files {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(corpus, realWorldManifestName)
	if err := writeJSONFile(manifestPath, realWorldManifest{
		SchemaVersion:  1,
		CorpusDir:      corpus,
		CacheDir:       cache,
		OutputArtifact: output,
		Repositories: []realWorldRepository{
			{Name: "remote", CloneURL: "https://github.com/example/remote.git", Path: remoteRepo, Status: "cloned"},
			{Name: "local", CloneURL: "https://github.com/example/local.git", Path: localRepo, Status: "cloned"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	inspection, err := realWorldInspectCorpus(realWorldInspectArgs{CorpusDir: corpus, ManifestPath: manifestPath})
	if err != nil {
		t.Fatalf("inspect corpus: %v", err)
	}
	if inspection.CacheDir != cache || inspection.OutputArtifact != output {
		t.Fatalf("carried paths = cache %q output %q", inspection.CacheDir, inspection.OutputArtifact)
	}
	if len(inspection.Repositories) != 2 || len(inspection.DraftDependencyPrep) != 2 {
		t.Fatalf("inspection = %+v", inspection)
	}
	byRepo := map[string]realWorldDependencyPrepScan{}
	for _, scan := range inspection.Repositories {
		byRepo[scan.Repository] = scan
	}
	if byRepo["remote"].NeedsDependencyPrep || len(byRepo["remote"].RemoteSchemaRefs) != 1 || len(byRepo["remote"].Lockfiles) != 1 {
		t.Fatalf("remote scan = %+v", byRepo["remote"])
	}
	if !byRepo["local"].NeedsDependencyPrep || len(byRepo["local"].LocalSchemaRefs) != 2 {
		t.Fatalf("local scan = %+v", byRepo["local"])
	}
	var remotePrep, localPrep realWorldDependencyPrep
	for _, prep := range inspection.DraftDependencyPrep {
		if prep.Repository == "remote" {
			remotePrep = prep
		}
		if prep.Repository == "local" {
			localPrep = prep
		}
	}
	if remotePrep.Status != "not-needed" || !strings.Contains(remotePrep.Notes, "no local $schema refs") {
		t.Fatalf("remote prep = %+v", remotePrep)
	}
	if localPrep.Status != "needs-review" || !strings.Contains(localPrep.Command, "pnpm install") || !strings.Contains(localPrep.Command, "--ignore-scripts") {
		t.Fatalf("local prep = %+v", localPrep)
	}
	if inspection.PrepSecurityPolicy["lifecycleScripts"] != "disabled" {
		t.Fatalf("prep security policy = %+v", inspection.PrepSecurityPolicy)
	}
	if !inspection.NeedsReview || !strings.Contains(inspection.Summary, "2 local schema refs") {
		t.Fatalf("summary=%q needsReview=%v", inspection.Summary, inspection.NeedsReview)
	}
}

func TestRealWorldPrepareRunStreamsManifestAndDependencyPrep(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGit := filepath.Join(fakeBin, "git")
	fakeGitScript := `#!/bin/sh
if [ "$1" = "clone" ]; then
  target=""
  for arg in "$@"; do target="$arg"; done
  mkdir -p "$target/config"
  printf '{"name":"fake"}' > "$target/package.json"
  printf '{"$schema":"./schema.json"}' > "$target/config/tool.json"
  printf '{"type":"object"}' > "$target/config/schema.json"
  exit 0
fi
if [ "$1" = "rev-parse" ]; then
  echo deadbeef
  exit 0
fi
exit 1
`
	if runtime.GOOS == "windows" {
		fakeGit = filepath.Join(fakeBin, "git.cmd")
		fakeGitScript = `@echo off
setlocal enabledelayedexpansion
if "%1"=="clone" (
  set "target="
  for %%A in (%*) do set "target=%%~A"
  mkdir "!target!\config"
  > "!target!\package.json" echo {"name":"fake"}
  > "!target!\config\tool.json" echo {"$schema":"./schema.json"}
  > "!target!\config\schema.json" echo {"type":"object"}
  exit /b 0
)
if "%1"=="rev-parse" (
  echo deadbeef
  exit /b 0
)
exit /b 1
`
	}
	if err := os.WriteFile(fakeGit, []byte(fakeGitScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	corpus := filepath.Join(root, "corpus")
	cache := filepath.Join(root, "cache")
	output := filepath.Join(root, "out.json")
	server := &repoServer{root: root, realWorldPrepareRuns: newRealWorldPrepareRegistry()}
	run, err := server.startRealWorldPrepareRun(realWorldStartPrepareArgs{
		Title:          "Streaming",
		CorpusDir:      corpus,
		CacheDir:       cache,
		OutputArtifact: output,
		Repositories: []realWorldRepository{
			{Name: "one", CloneURL: "https://github.com/example/one.git"},
			{Name: "two", CloneURL: "https://github.com/example/two.git"},
		},
		Concurrency: 2,
	})
	if err != nil {
		t.Fatalf("start prepare run: %v", err)
	}
	for i := 0; i < 2; i++ {
		select {
		case result := <-run.results:
			if !result.Succeeded || result.DependencyPrep == nil || result.DependencyPrep.Status != "needs-review" {
				t.Fatalf("prepare result = %+v", result)
			}
			run.markDelivered()
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for prepare result")
		}
	}
	select {
	case <-run.done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for prepare completion")
	}
	manifest, err := readRealWorldManifest(run.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !manifest.PreparationManaged || !manifest.PreparationComplete || manifest.PreparationRunID != run.ID {
		t.Fatalf("manifest preparation fields = %+v", manifest)
	}
	if len(manifest.Repositories) != 2 || manifest.Repositories[0].Status != "cloned" || manifest.Repositories[0].Commit != "deadbeef" {
		t.Fatalf("manifest repositories = %+v", manifest.Repositories)
	}
	if len(manifest.DependencyPrep) != 2 || len(manifest.DependencyPrepInspection) != 2 || !manifest.DependencyPrepNeedsReview {
		t.Fatalf("manifest dependency prep = prep %+v inspection %+v", manifest.DependencyPrep, manifest.DependencyPrepInspection)
	}
}

func TestRealWorldValidationRefreshesDependencyPrepFromManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, realWorldManifestName)
	if err := writeJSONFile(manifestPath, realWorldManifest{
		SchemaVersion:  1,
		CorpusDir:      dir,
		CacheDir:       filepath.Join(dir, "cache"),
		OutputArtifact: filepath.Join(dir, "out.json"),
		Repositories: []realWorldRepository{{
			Name:     "example",
			CloneURL: "https://github.com/example/example.git",
			Path:     filepath.Join(dir, "example"),
			Status:   "cloned",
		}},
		DependencyPrep: []realWorldDependencyPrep{{
			Repository: "example",
			Status:     "not-needed",
			Notes:      "No local schema refs.",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	run := &realWorldValidationRun{
		ManifestPath: manifestPath,
		ctx:          context.Background(),
	}
	run.refreshFromManifest()
	if len(run.Repositories) != 1 || len(run.DependencyPrep) != 1 {
		t.Fatalf("run after refresh = repos %+v prep %+v", run.Repositories, run.DependencyPrep)
	}
}

func TestRealWorldSuggestedPrepCommandsSuppressScripts(t *testing.T) {
	commands := realWorldSuggestedPrepCommands(realWorldDependencyPrepScan{Lockfiles: []string{
		"package-lock.json",
		"pnpm-lock.yaml",
		"yarn.lock",
		"bun.lock",
		"composer.lock",
		"cargo.lock",
		"go.sum",
		"poetry.lock",
		"Pipfile.lock",
		"Gemfile.lock",
	}})
	joined := strings.Join(commands, "\n")
	for _, want := range []string{
		"npm_config_ignore_scripts=true npm ci --ignore-scripts",
		"npm_config_ignore_scripts=true pnpm install --frozen-lockfile --ignore-scripts",
		"YARN_ENABLE_SCRIPTS=false yarn install --frozen-lockfile --ignore-scripts",
		"bun install --frozen-lockfile --ignore-scripts",
		"composer install --no-scripts --no-plugins",
		"cargo fetch --locked",
		"go mod download",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing safe prep command %q in %q", want, joined)
		}
	}
	for _, forbidden := range []string{"poetry install", "pipenv sync", "bundle install"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unsafe prep command %q in %q", forbidden, joined)
		}
	}
}

func TestSaveRealWorldHistoryAddsSchema(t *testing.T) {
	root := t.TempDir()
	history := realWorldHistory{
		Entries: []realWorldEntry{{
			ID:                      "sample",
			Date:                    "2026-05-09",
			Title:                   "Sample",
			DollarLintRevision:      "abc123",
			Corpus:                  "/tmp/corpus",
			Command:                 "bin/dollarlint validate /tmp/corpus",
			OutputArtifact:          "/tmp/out.json",
			PersistedOutputArtifact: "reports/agentic-product-testing/sample/dollarlint.json",
			DependencyPrep: []realWorldDependencyPrep{{
				Repository: "example",
				Command:    "npm ci --ignore-scripts",
				Status:     "skipped",
				Notes:      "No lockfile present.",
			}},
			ValidationFeedback: []realWorldValidationFeedback{{
				Repository: "example",
				Outcome:    realWorldFeedbackBehavedReasonably,
				Notes:      "DollarLint handled the repo reasonably.",
			}},
			Repositories: []realWorldRepository{{
				Name:     "example",
				CloneURL: "https://github.com/example/example.git",
			}},
			ProductRecommendations: []realWorldProductRecommendation{{
				Strength:       "low",
				Recommendation: "Keep observing this fixture class.",
				Rationale:      "The sweep produced a low-volume signal.",
			}},
		}},
	}
	if err := saveRealWorldHistory(root, history); err != nil {
		t.Fatal(err)
	}
	entryRelPath := realWorldEntryRelPath(history.Entries[0])
	if entryRelPath != "reports/agentic-product-testing/sample/metadata.json" {
		t.Fatalf("entry path = %q", entryRelPath)
	}
	entryData, err := os.ReadFile(filepath.Join(root, entryRelPath))
	if err != nil {
		t.Fatal(err)
	}
	var entryFile realWorldEntryFile
	if err := json.Unmarshal(entryData, &entryFile); err != nil {
		t.Fatal(err)
	}
	if entryFile.Schema != realWorldEntrySchema || entryFile.SchemaVersion != realWorldHistorySchemaVersion || entryFile.ID != "sample" {
		t.Fatalf("entry file = %+v", entryFile)
	}
	if entryFile.PersistedOutputArtifact != "reports/agentic-product-testing/sample/dollarlint.json" {
		t.Fatalf("persisted output artifact = %q", entryFile.PersistedOutputArtifact)
	}
	if len(entryFile.DependencyPrep) != 1 || entryFile.DependencyPrep[0].Status != "skipped" {
		t.Fatalf("dependency prep = %+v", entryFile.DependencyPrep)
	}
	if len(entryFile.ValidationFeedback) != 1 || entryFile.ValidationFeedback[0].Outcome != realWorldFeedbackBehavedReasonably {
		t.Fatalf("validation feedback = %+v", entryFile.ValidationFeedback)
	}
	if len(entryFile.ProductRecommendations) != 1 || entryFile.ProductRecommendations[0].Strength != "low" {
		t.Fatalf("product recommendations = %+v", entryFile.ProductRecommendations)
	}
	loaded, err := loadRealWorldHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schema != realWorldEntrySchema || loaded.SchemaVersion != realWorldHistorySchemaVersion {
		t.Fatalf("loaded history = %+v", loaded)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].Repositories[0].Name != "example" {
		t.Fatalf("loaded entries = %+v", loaded.Entries)
	}
}

func TestValidateRealWorldEntryForRecordRequiresStructuredFields(t *testing.T) {
	err := validateRealWorldEntryForRecord(realWorldEntry{
		Title:              "Incomplete",
		DollarLintRevision: "abc123",
		WorkingTreeNote:    "clean working tree",
		Corpus:             "/tmp/corpus",
		CacheDir:           "/tmp/cache",
		Command:            "bin/dollarlint validate /tmp/corpus --format json --output /tmp/out.json",
		OutputArtifact:     "/tmp/out.json",
		Repositories: []realWorldRepository{{
			Name:     "example",
			CloneURL: "https://github.com/example/example.git",
		}},
		DependencyPrep: []realWorldDependencyPrep{{
			Repository: "example",
			Status:     "not-needed",
			Notes:      "No local dependency schemas referenced.",
		}},
		Findings: []string{"No crashes or output contract issues."},
	})
	if err == nil {
		t.Fatal("expected incomplete entry error")
	}
	if !strings.Contains(err.Error(), "validationFeedback") || !strings.Contains(err.Error(), "productRecommendations") || !strings.Contains(err.Error(), "productDecisions") || !strings.Contains(err.Error(), "followUp") {
		t.Fatalf("error did not name missing structured fields: %v", err)
	}
}

func TestValidateRealWorldEntryForRecordAcceptsCompleteEntry(t *testing.T) {
	entry := realWorldEntry{
		Title:              "Complete",
		DollarLintRevision: "abc123",
		WorkingTreeNote:    "clean working tree",
		Corpus:             "/tmp/corpus",
		CacheDir:           "/tmp/cache",
		Command:            "bin/dollarlint validate /tmp/corpus --format json --output /tmp/out.json",
		OutputArtifact:     "/tmp/out.json",
		Repositories: []realWorldRepository{{
			Name:     "example",
			CloneURL: "https://github.com/example/example.git",
		}},
		DependencyPrep: []realWorldDependencyPrep{{
			Repository: "example",
			Status:     "not-needed",
			Notes:      "No local dependency schemas referenced.",
		}},
		ValidationFeedback: []realWorldValidationFeedback{{
			Repository: "example",
			Outcome:    realWorldFeedbackBehavedReasonably,
			Findings:   []string{"The repo produced no issues or warnings and skipped files were expected fixtures."},
			Notes:      "DollarLint handled the repo reasonably from a developer-experience perspective.",
		}},
		Findings: []string{"No crashes or output contract issues."},
		ProductRecommendations: []realWorldProductRecommendation{{
			Strength:       "low",
			Recommendation: "Keep observing this fixture class.",
			Rationale:      "The sweep produced a low-volume signal.",
		}},
		ProductDecisions: []string{"No product changes made from this sweep."},
		FollowUp:         []string{"Run another diverse corpus next week."},
	}
	if err := validateRealWorldEntryForRecord(entry); err != nil {
		t.Fatalf("validate complete entry: %v", err)
	}
}

func TestValidateRealWorldFeedbackRequiresEvidenceForBehavedReasonably(t *testing.T) {
	err := validateRealWorldValidationFeedback(realWorldValidationFeedback{
		Repository: "example",
		Outcome:    realWorldFeedbackBehavedReasonably,
		Notes:      "Accepted; see merged artifact for details.",
	})
	if err == nil {
		t.Fatal("expected behaved-reasonably feedback without evidence to be rejected")
	}
	if !strings.Contains(err.Error(), "behaved-reasonably feedback") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRealWorldFeedbackContractFramesDeveloperExperience(t *testing.T) {
	contract := realWorldValidationFeedbackContract()
	if _, ok := contract["assessmentPerspective"]; !ok {
		t.Fatalf("contract missing developer-experience guidance: %+v", contract)
	}
	evidence, ok := contract["evidence"].(string)
	if !ok || !strings.Contains(evidence, "notes-only boilerplate is rejected") {
		t.Fatalf("contract evidence guidance = %+v", contract["evidence"])
	}

	next := realWorldNextValidationResultWithFeedback("run", "example")
	if _, ok := next["feedbackTemplate"]; !ok {
		t.Fatalf("next step should provide a feedback template instead of a copyable default: %+v", next)
	}
	suggested := next["suggestedArgs"].(map[string]any)
	if _, ok := suggested["feedback"]; ok {
		t.Fatalf("suggested args should not default feedback to behaved-reasonably: %+v", suggested)
	}
}

func TestRealWorldManagedNextStepHidesPaths(t *testing.T) {
	run := &realWorldPrepareRun{
		ID:             "prepare-run",
		CorpusDir:      "/tmp/dollarlint-corpus.secret",
		CacheDir:       "/tmp/dollarlint-cache.secret",
		OutputArtifact: "/tmp/dollarlint-output.secret.json",
		ManifestPath:   "/tmp/dollarlint-corpus.secret/real-world-manifest.json",
		Repositories:   []realWorldRepository{{Name: "example", CloneURL: "https://github.com/example/example.git", Path: "/tmp/dollarlint-corpus.secret/example"}},
		prepByID:       map[string]realWorldDependencyPrep{},
	}
	step := realWorldNextRunCorpusDuringPrepare(run)
	suggested := step["suggestedArgs"].(map[string]any)
	if suggested["runID"] != "prepare-run" {
		t.Fatalf("managed next step should carry runID: %+v", suggested)
	}
	for _, key := range []string{"corpusDir", "cacheDir", "outputArtifact", "manifestPath"} {
		if _, ok := suggested[key]; ok {
			t.Fatalf("managed next step leaked %s: %+v", key, suggested)
		}
	}
}

func TestRealWorldPublicResultsHideManagedPaths(t *testing.T) {
	prep := realWorldPublicPrepareResult(realWorldRepoPrepareResult{
		Repository: "example",
		Path:       "/tmp/dollarlint-corpus.secret/example",
		Command:    "git clone https://github.com/example/example.git /tmp/dollarlint-corpus.secret/example",
		RepositoryRecord: realWorldRepository{
			Name:     "example",
			CloneURL: "https://github.com/example/example.git",
			Path:     "/tmp/dollarlint-corpus.secret/example",
		},
		DependencyPrepInspection: &realWorldDependencyPrepScan{
			Repository: "example",
			Path:       "/tmp/dollarlint-corpus.secret/example",
		},
	})
	validation := realWorldPublicValidationResult(nil, realWorldRepoValidationResult{
		Repository:     "example",
		Path:           "/tmp/dollarlint-corpus.secret/example",
		CacheDir:       "/tmp/dollarlint-cache.secret/repo-example",
		OutputArtifact: "/tmp/dollarlint-validation.secret/example.json",
		Command:        "XDG_CACHE_HOME=/tmp/dollarlint-cache.secret/repo-example bin/dollarlint validate /tmp/dollarlint-corpus.secret/example",
	})
	data, err := json.Marshal(map[string]any{"prep": prep, "validation": validation})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, leaked := range []string{"dollarlint-corpus.secret", "dollarlint-cache.secret", "dollarlint-validation.secret", "git clone"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("public result leaked %q in %s", leaked, text)
		}
	}
}

func TestRealWorldValidationFeedbackLedger(t *testing.T) {
	result := realWorldRepoValidationResult{Repository: "example", Accepted: true}
	run := &realWorldValidationRun{
		ID:            "run",
		Repositories:  []realWorldRepository{{Name: "example"}},
		resultsByID:   map[string]realWorldRepoValidationResult{"example": result},
		deliveredByID: map[string]realWorldRepoValidationResult{},
		feedbackByID:  map[string]realWorldValidationFeedback{},
	}
	if err := run.recordFeedback(realWorldValidationFeedback{
		Repository: "example",
		Outcome:    realWorldFeedbackBehavedReasonably,
		Notes:      "Looks fine.",
	}); err == nil {
		t.Fatal("expected feedback before delivery to be rejected")
	}

	run.markDelivered(result)
	snapshot := run.snapshot()
	if snapshot.FeedbackMissing != 1 || snapshot.FeedbackRecorded != 0 || len(snapshot.FeedbackMissingRepositories) != 1 {
		t.Fatalf("snapshot before feedback = %+v", snapshot)
	}
	next := realWorldValidationNextStep(run)
	if next["tool"] != "real_world_next_validation_result" {
		t.Fatalf("next step before feedback = %+v", next)
	}

	err := run.recordFeedback(realWorldValidationFeedback{
		Repository: "example",
		Outcome:    realWorldFeedbackProductSignal,
		Findings:   []string{"The repository result exposed a concrete product signal."},
		ProductRecommendations: []realWorldProductRecommendation{{
			Strength:       "med",
			Recommendation: "Improve this behavior.",
			Rationale:      "The product signal was reproducible in this repository.",
		}},
	})
	if err != nil {
		t.Fatalf("record feedback: %v", err)
	}
	if missing := run.missingFeedback(); len(missing) != 0 {
		t.Fatalf("missing feedback = %+v", missing)
	}
	feedback := run.validationFeedback()
	if len(feedback) != 1 || feedback[0].Repository != "example" || feedback[0].Outcome != realWorldFeedbackProductSignal {
		t.Fatalf("feedback = %+v", feedback)
	}
	snapshot = run.snapshot()
	if snapshot.FeedbackMissing != 0 || snapshot.FeedbackRecorded != 1 || !snapshot.FeedbackComplete {
		t.Fatalf("snapshot after feedback = %+v", snapshot)
	}
}

func TestRealWorldNextAfterRecordOnlyMentionsDiscussionInAgenticWorkflow(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	next := realWorldNextAfterRecord(realWorldEntry{ID: "sample"})
	if _, ok := next["discussion"]; ok {
		t.Fatalf("generic GitHub Actions run should not include discussion guidance: %+v", next)
	}

	t.Setenv("GH_AW_WORKFLOW_ID", "agentic-product-testing")
	next = realWorldNextAfterRecord(realWorldEntry{ID: "sample"})
	if _, ok := next["discussion"]; !ok {
		t.Fatalf("GitHub Agentic Workflow run should include discussion guidance: %+v", next)
	}
}

func TestCreateRealWorldOutputPath(t *testing.T) {
	path, err := createRealWorldOutputPath("My Sweep", "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) == path || filepath.Ext(path) != ".json" {
		t.Fatalf("generated path = %q", path)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("output path should be reserved but not created, stat err=%v", err)
	}
}

func TestPersistRealWorldOutputArtifactCopiesRawJSON(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "out.json")
	raw := []byte(`{"summary":{"discovered":1},"files":[{"path":"repo/config.json"}]}`)
	if err := os.WriteFile(src, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := persistRealWorldOutputArtifact(root, realWorldEntry{
		ID:             "2026-05-09 Sample Sweep",
		OutputArtifact: src,
	})
	if err != nil {
		t.Fatalf("persist output artifact: %v", err)
	}
	expectedRel := "reports/agentic-product-testing/2026-05-09-sample-sweep/dollarlint.json"
	if rel != expectedRel {
		t.Fatalf("rel path = %q, want %q", rel, expectedRel)
	}
	got, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("persisted output changed: %s", got)
	}
}

func TestPersistRealWorldOutputArtifactRejectsInvalidJSON(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "out.json")
	if err := os.WriteFile(src, []byte(`not-json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := persistRealWorldOutputArtifact(root, realWorldEntry{ID: "bad", OutputArtifact: src}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestRealWorldCleanupRecordTempsRemovesManagedTempDirs(t *testing.T) {
	corpus, err := os.MkdirTemp("", realWorldCorpusTempPrefix)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := os.MkdirTemp("", realWorldCacheTempPrefix)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(corpus, "repo"), filepath.Join(cache, "schemas")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	results := realWorldCleanupRecordTemps(realWorldEntry{Corpus: corpus, CacheDir: cache})
	if !realWorldCleanupOK(results) {
		t.Fatalf("cleanup should be ok: %+v", results)
	}
	for _, result := range results {
		if result.Status != "removed" {
			t.Fatalf("cleanup result = %+v", result)
		}
		if _, err := os.Stat(result.Path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected %s to be removed, stat err=%v", result.Path, err)
		}
	}
}

func TestRealWorldCleanupRecordTempsSkipsUnmanagedDirs(t *testing.T) {
	unmanaged := t.TempDir()
	results := realWorldCleanupRecordTemps(realWorldEntry{Corpus: unmanaged, CacheDir: unmanaged})
	for _, result := range results {
		if result.Status != "skipped" {
			t.Fatalf("cleanup result = %+v", result)
		}
	}
	if _, err := os.Stat(unmanaged); err != nil {
		t.Fatalf("unmanaged dir should remain, stat err=%v", err)
	}
}

func TestRealWorldMergeValidationArtifactsPrefixesRepoPaths(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	if err := os.MkdirAll(repoA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoB, 0o755); err != nil {
		t.Fatal(err)
	}
	outA := filepath.Join(root, "a.json")
	outB := filepath.Join(root, "b.json")
	for path, payload := range map[string]realWorldMergedOutput{
		outA: {
			Schema:        "https://example.test/result.schema.json",
			FormatVersion: 1,
			Root:          repoA,
			Summary: realWorldMergedSummary{
				Discovered: 1,
				Validated:  1,
				Issues:     realWorldIssueSummary{Total: 1, Validation: 1},
			},
			Files:  []map[string]any{{"path": "config/a.json", "format": "json", "status": "validated", "issues": float64(1)}},
			Issues: []map[string]any{{"path": "config/a.json", "category": "validation", "message": "bad"}},
		},
		outB: {
			Schema:        "https://example.test/result.schema.json",
			FormatVersion: 1,
			Root:          repoB,
			Summary: realWorldMergedSummary{
				Discovered: 1,
				Failed:     1,
				Issues:     realWorldIssueSummary{Total: 1, Parsing: 1},
				Warnings:   1,
			},
			Files:    []map[string]any{{"path": "broken.yaml", "format": "yaml", "status": "error", "issues": float64(1)}},
			Issues:   []map[string]any{{"path": "broken.yaml", "category": "parsing", "message": "parse failed"}},
			Warnings: []map[string]any{{"path": "broken.yaml", "kind": "schemaCatalogSchemaUnavailable", "message": "warn"}},
		},
	} {
		if err := writeJSONFile(path, payload); err != nil {
			t.Fatal(err)
		}
	}
	run := &realWorldValidationRun{
		CorpusDir: root,
		resultsByID: map[string]realWorldRepoValidationResult{
			"a": {Repository: "repo-a", Path: repoA, OutputArtifact: outA, Accepted: true},
			"b": {Repository: "repo-b", Path: repoB, OutputArtifact: outB, Accepted: true},
		},
	}
	mergedPath := filepath.Join(root, "merged.json")
	summary, err := realWorldMergeValidationArtifacts(run, mergedPath)
	if err != nil {
		t.Fatalf("merge validation artifacts: %v", err)
	}
	if summary.Discovered != 2 || summary.Issues.Total != 2 || summary.Issues.Parsing != 1 || summary.Warnings != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	details, err := readRealWorldOutputDetails(mergedPath)
	if err != nil {
		t.Fatalf("read merged details: %v", err)
	}
	paths := []string{details.Files[0].Path, details.Files[1].Path, details.Issues[0].Path, details.Issues[1].Path}
	sort.Strings(paths)
	want := []string{"repo-a/config/a.json", "repo-a/config/a.json", "repo-b/broken.yaml", "repo-b/broken.yaml"}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %+v, want %+v", paths, want)
		}
	}
}

func TestRealWorldEnsureManagedOutputArtifactRegeneratesMissingMerge(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo-a")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	perRepoOutput := filepath.Join(root, "repo-a.json")
	if err := writeJSONFile(perRepoOutput, realWorldMergedOutput{
		Schema:        "https://example.test/result.schema.json",
		FormatVersion: 1,
		Root:          repo,
		Summary: realWorldMergedSummary{
			Discovered: 1,
			Validated:  1,
			Issues:     realWorldIssueSummary{},
		},
		Files: []map[string]any{{"path": "dollarlint.json", "format": "json", "status": "validated"}},
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	mergedPath := filepath.Join(root, "missing-merged.json")
	result := realWorldRepoValidationResult{Repository: "repo-a", Path: repo, OutputArtifact: perRepoOutput, Accepted: true}
	run := &realWorldValidationRun{
		ID:             "run",
		CorpusDir:      root,
		OutputArtifact: mergedPath,
		Repositories:   []realWorldRepository{{Name: "repo-a", Path: repo}},
		StartedAt:      now,
		completedAt:    &now,
		completed:      1,
		results:        make(chan realWorldRepoValidationResult, 1),
		resultsByID:    map[string]realWorldRepoValidationResult{"repo-a": result},
		deliveredByID:  map[string]realWorldRepoValidationResult{"repo-a": result},
		feedbackByID: map[string]realWorldValidationFeedback{"repo-a": {
			Repository: "repo-a",
			Outcome:    realWorldFeedbackBehavedReasonably,
			Findings:   []string{"Validated cleanly with clear output."},
		}},
	}
	if err := realWorldEnsureManagedOutputArtifact(run); err != nil {
		t.Fatalf("ensure managed output artifact: %v", err)
	}
	summary, _, err := readRealWorldOutputSummary(mergedPath)
	if err != nil {
		t.Fatalf("read regenerated artifact: %v", err)
	}
	if summary.Discovered != 1 || summary.Validated != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}
