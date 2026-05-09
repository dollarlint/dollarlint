package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
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
