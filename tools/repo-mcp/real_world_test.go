package main

import (
	"os"
	"path/filepath"
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
