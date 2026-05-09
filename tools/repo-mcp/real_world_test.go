package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			PersistedOutputArtifact: "reports/real-world-artifacts/sample.dollarlint.json",
			DependencyPrep: []realWorldDependencyPrep{{
				Repository: "example",
				Command:    "npm ci --ignore-scripts",
				Status:     "skipped",
				Notes:      "No lockfile present.",
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
	indexData, err := os.ReadFile(filepath.Join(root, realWorldResultsRelPath))
	if err != nil {
		t.Fatal(err)
	}
	var index realWorldHistoryIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatal(err)
	}
	if index.Schema != realWorldResultsSchema || index.SchemaVersion != realWorldHistorySchemaVersion || len(index.Entries) != 1 {
		t.Fatalf("index = %+v", index)
	}
	if index.Entries[0].Path == "" || filepath.Dir(index.Entries[0].Path) != realWorldResultsDirRelPath {
		t.Fatalf("index entry = %+v", index.Entries[0])
	}
	entryData, err := os.ReadFile(filepath.Join(root, index.Entries[0].Path))
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
	if entryFile.PersistedOutputArtifact != "reports/real-world-artifacts/sample.dollarlint.json" {
		t.Fatalf("persisted output artifact = %q", entryFile.PersistedOutputArtifact)
	}
	if len(entryFile.DependencyPrep) != 1 || entryFile.DependencyPrep[0].Status != "skipped" {
		t.Fatalf("dependency prep = %+v", entryFile.DependencyPrep)
	}
	if len(entryFile.ProductRecommendations) != 1 || entryFile.ProductRecommendations[0].Strength != "low" {
		t.Fatalf("product recommendations = %+v", entryFile.ProductRecommendations)
	}
	loaded, err := loadRealWorldHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schema != realWorldResultsSchema || loaded.SchemaVersion != realWorldHistorySchemaVersion {
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
	if !strings.Contains(err.Error(), "productRecommendations") || !strings.Contains(err.Error(), "productDecisions") || !strings.Contains(err.Error(), "followUp") {
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

func TestRealWorldNextAfterRecordOnlyMentionsDiscussionInAgenticWorkflow(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	next := realWorldNextAfterRecord(realWorldEntry{ID: "sample"})
	if _, ok := next["discussion"]; ok {
		t.Fatalf("generic GitHub Actions run should not include discussion guidance: %+v", next)
	}

	t.Setenv("GH_AW_WORKFLOW_ID", "weekly-real-world-testing")
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
	expectedRel := "reports/real-world-artifacts/2026-05-09-sample-sweep.dollarlint.json"
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
