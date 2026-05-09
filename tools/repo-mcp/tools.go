package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *repoServer) addTools() {
	s.addTool("repo_status", "Summarize branch, dirty files, recent commits, tags, generated artifacts, and configured remotes.", schemaObject(nil), s.handleRepoStatus, true)
	s.addTool("project_map", "Return a concise map of important dollarlint paths, commands, and development surfaces.", schemaObject(nil), s.handleProjectMap, true)
	s.addTool("verify", "Run a named repo verification profile. Profiles: quick, full, docs, release, examples, ci.", schemaObject(map[string]any{
		"profile": enumSchema([]string{"quick", "full", "docs", "release", "examples", "ci"}, "Verification profile to run."),
	}), s.handleVerify, false)
	s.addTool("run_example", "Run dollarlint against a named example suite and return structured validation output.", schemaObject(map[string]any{
		"suite":     enumSchema([]string{"basics", "nested-configs", "schemastore", "azure", "repo-config", "all"}, "Example suite to validate."),
		"format":    enumSchema([]string{"text", "json", "sarif"}, "Output format. Defaults to text."),
		"locations": map[string]any{"type": "boolean", "description": "Include source locations. Defaults to true."},
	}), s.handleRunExample, true)
	s.addTool("diagnose_validation", "Validate files and classify common dollarlint states such as skipped files, fetch/config errors, branch error mode, and Azure pruning.", schemaObject(map[string]any{
		"include":      arrayStringSchema("File or glob patterns to validate."),
		"branchErrors": enumSchema([]string{"best", "all"}, "Temporarily override output.branchErrors."),
	}), s.handleDiagnoseValidation, true)
	s.addTool("real_world_history", "List and query structured real-world corpus history, including repositories already tested.", schemaObject(map[string]any{
		"repo":           map[string]any{"type": "string", "description": "Single repository name or clone URL to check."},
		"repositories":   arrayStringSchema("Repository names or clone URLs to check."),
		"includeEntries": map[string]any{"type": "boolean", "description": "Include full structured history entries. Defaults to false."},
	}), s.handleRealWorldHistory, true)
	s.addTool("real_world_start_testing", "Start a guided real-world validation sweep, check structured history and candidate duplicates, and return the next MCP tool to call.", schemaObject(map[string]any{
		"title":                 map[string]any{"type": "string", "description": "Short sweep title."},
		"repositories":          realWorldRepositoryArraySchema("Candidate repositories to check before preparing the corpus."),
		"allowPreviouslyTested": map[string]any{"type": "boolean", "description": "Allow intentional reruns of repositories already present in real-world history."},
	}), s.handleRealWorldStartTesting, true)
	s.addToolWithHints("real_world_prepare_corpus", "Create real-world testing temp dirs, flag previously tested repositories, optionally clone repos, and write a corpus manifest.", schemaObject(map[string]any{
		"title":                 map[string]any{"type": "string", "description": "Short sweep title used for generated paths and manifests."},
		"repositories":          realWorldRepositoryArraySchema("Repositories planned for the corpus."),
		"clone":                 map[string]any{"type": "boolean", "description": "When true, run shallow git clones into the prepared corpus directory. Defaults to false."},
		"allowPreviouslyTested": map[string]any{"type": "boolean", "description": "Allow intentional reruns of repositories already present in real-world history."},
		"outputName":            map[string]any{"type": "string", "description": "Optional output JSON filename or absolute path."},
	}), s.handleRealWorldPrepareCorpus, toolHints{ReadOnly: false, OpenWorld: true})
	s.addTool("real_world_inspect_corpus", "Scan prepared real-world clones for lockfiles, dependency manifests, and local or remote $schema refs, then draft script-suppressed dependency-prep notes.", schemaObject(map[string]any{
		"corpusDir":      map[string]any{"type": "string", "description": "Prepared corpus directory."},
		"cacheDir":       map[string]any{"type": "string", "description": "Cache directory to carry forward to real_world_start_validation. Defaults from manifestPath when available."},
		"outputArtifact": map[string]any{"type": "string", "description": "Output artifact path to carry forward to real_world_start_validation. Defaults from manifestPath when available."},
		"manifestPath":   map[string]any{"type": "string", "description": "Prepared corpus manifest path. Defaults to <corpusDir>/real-world-manifest.json."},
		"repositories":   realWorldRepositoryArraySchema("Repositories included in the sweep. Defaults from manifestPath when available."),
		"maxMatches":     map[string]any{"type": "integer", "description": "Maximum matches to retain per category per repository. Defaults to 12."},
	}), s.handleRealWorldInspectCorpus, true)
	s.addToolWithHints("real_world_start_validation", "Start managed per-repository DollarLint validation jobs and optionally wait with progress notifications until the first repo result is ready.", realWorldValidationToolSchema(map[string]any{
		"concurrency":        map[string]any{"type": "integer", "description": "Maximum repository validations to run at once. Defaults to 4; capped at 16."},
		"waitForFirstResult": map[string]any{"type": "boolean", "description": "When true, keep this tool call open with progress notifications until the first repository result is ready. Defaults to true."},
	}), s.handleRealWorldStartValidation, toolHints{ReadOnly: false, OpenWorld: true})
	s.addTool("real_world_next_validation_result", "Wait with progress notifications until the next per-repository real-world validation result is ready.", schemaObject(map[string]any{
		"runID": map[string]any{"type": "string", "description": "Managed validation run id returned by real_world_start_validation."},
	}), s.handleRealWorldNextValidationResult, false)
	s.addTool("real_world_validation_status", "Return the current status of a managed per-repository real-world validation run.", schemaObject(map[string]any{
		"runID": map[string]any{"type": "string", "description": "Managed validation run id returned by real_world_start_validation."},
	}), s.handleRealWorldValidationStatus, true)
	s.addToolWithHints("real_world_finish_validation", "Merge completed per-repository validation artifacts into the standard real-world JSON artifact for triage.", schemaObject(map[string]any{
		"runID":          map[string]any{"type": "string", "description": "Managed validation run id returned by real_world_start_validation."},
		"outputArtifact": map[string]any{"type": "string", "description": "Optional merged JSON artifact path. Defaults to the run outputArtifact."},
	}), s.handleRealWorldFinishValidation, toolHints{ReadOnly: false})
	s.addToolWithHints("real_world_cancel_validation", "Cancel a managed per-repository real-world validation run.", schemaObject(map[string]any{
		"runID": map[string]any{"type": "string", "description": "Managed validation run id returned by real_world_start_validation."},
	}), s.handleRealWorldCancelValidation, toolHints{ReadOnly: false})
	s.addToolWithHints("real_world_run_corpus", "Legacy blocking runner for the standard schema-backed real-world corpus validation command. Prefer real_world_start_validation for long runs.", schemaObject(map[string]any{
		"corpusDir":          map[string]any{"type": "string", "description": "Prepared corpus directory to validate."},
		"cacheDir":           map[string]any{"type": "string", "description": "Isolated XDG cache directory. Created automatically when omitted."},
		"outputArtifact":     map[string]any{"type": "string", "description": "JSON output artifact path. Created automatically when omitted."},
		"manifestPath":       map[string]any{"type": "string", "description": "Prepared corpus manifest path. Defaults to <corpusDir>/real-world-manifest.json."},
		"repositories":       realWorldRepositoryArraySchema("Repositories included in the sweep, carried forward to triage."),
		"dependencyPrep":     realWorldDependencyPrepArraySchema("Dependency-prep notes from real_world_inspect_corpus, carried forward to triage. Do not run dependency lifecycle scripts during prep."),
		"build":              map[string]any{"type": "boolean", "description": "Build bin/dollarlint before validating. Defaults to true."},
		"schemaStore":        map[string]any{"type": "boolean", "description": "Enable SchemaStore catalog matching. Defaults to true."},
		"schemaStoreFailure": enumSchema([]string{"warn", "error", "skip"}, "SchemaStore catalog failure policy. Defaults to warn."),
		"fetchRetries":       map[string]any{"type": "integer", "description": "Remote schema fetch retries. Defaults to 1."},
		"fetchRetryMinWait":  map[string]any{"type": "string", "description": "Minimum retry wait. Defaults to 1ms."},
		"fetchRetryMaxWait":  map[string]any{"type": "string", "description": "Maximum retry wait. Defaults to 1ms."},
		"extraArgs":          arrayStringSchema("Additional dollarlint validate arguments."),
	}), s.handleRealWorldRunCorpus, toolHints{ReadOnly: false, OpenWorld: true})
	s.addTool("real_world_triage_output", "Sanity-check and triage a DollarLint real-world JSON output artifact, then return grouped findings, draft record fields, and final-response guidance.", schemaObject(map[string]any{
		"title":                  map[string]any{"type": "string", "description": "Short sweep title. Defaults from manifestPath when available."},
		"corpusDir":              map[string]any{"type": "string", "description": "Prepared corpus directory."},
		"cacheDir":               map[string]any{"type": "string", "description": "Cache directory used by the run."},
		"command":                map[string]any{"type": "string", "description": "Reproducible validation command. Defaults to the standard command when corpus/cache/output are available."},
		"outputArtifact":         map[string]any{"type": "string", "description": "DollarLint JSON output artifact to triage."},
		"manifestPath":           map[string]any{"type": "string", "description": "Prepared corpus manifest path. Defaults to <corpusDir>/real-world-manifest.json."},
		"repositories":           realWorldRepositoryArraySchema("Repositories included in the sweep. Defaults from manifestPath when available."),
		"dependencyPrep":         realWorldDependencyPrepArraySchema("Dependency preparation commands, skips, failures, and their validation impact."),
		"productRecommendations": realWorldProductRecommendationArraySchema("Optional product recommendations to use instead of the triage draft."),
		"productDecisions":       arrayStringSchema("Optional product changes or decisions to use instead of the triage draft."),
		"followUp":               arrayStringSchema("Optional follow-up notes to use instead of the triage draft."),
	}), s.handleRealWorldTriageOutput, true)
	s.addTool("real_world_record_result", "Persist a real-world sweep result, copy the DollarLint JSON output into reports/real-world-artifacts, and clean managed temp corpus/cache dirs.", schemaObject(map[string]any{
		"id":                     map[string]any{"type": "string", "description": "Stable entry id. Defaults to a slug from date and title."},
		"date":                   map[string]any{"type": "string", "description": "Entry date in YYYY-MM-DD. Defaults to today."},
		"title":                  map[string]any{"type": "string", "description": "Short sweep title."},
		"dollarlintRevision":     map[string]any{"type": "string", "description": "DollarLint commit under test. Defaults to git rev-parse HEAD."},
		"workingTreeNote":        map[string]any{"type": "string", "description": "Working tree note. Defaults to current git status summary."},
		"corpus":                 map[string]any{"type": "string", "description": "Corpus directory."},
		"cacheDir":               map[string]any{"type": "string", "description": "Cache directory used by the run."},
		"command":                map[string]any{"type": "string", "description": "Reproducible validation command."},
		"outputArtifact":         map[string]any{"type": "string", "description": "DollarLint JSON output artifact to summarize."},
		"manifestPath":           map[string]any{"type": "string", "description": "Prepared corpus manifest path. Defaults to <corpus>/real-world-manifest.json."},
		"repositories":           realWorldRepositoryArraySchema("Repositories included in the sweep."),
		"dependencyPrep":         realWorldDependencyPrepArraySchema("Dependency preparation commands, skips, failures, and their validation impact."),
		"findings":               arrayStringSchema("Triaged findings."),
		"productRecommendations": realWorldProductRecommendationArraySchema("Product recommendations from the sweep, with strength and rationale."),
		"productDecisions":       arrayStringSchema("Product changes or decisions made after the sweep."),
		"followUp":               arrayStringSchema("Follow-up notes."),
		"replace":                map[string]any{"type": "boolean", "description": "Replace an existing entry with the same id."},
	}), s.handleRealWorldRecordResult, false)
	s.addTool("azure_pruning_report", "Inspect an Azure ARM template and report detected resource refs, pruning config, branch error mode, and validation summary.", schemaObject(map[string]any{
		"file":         map[string]any{"type": "string", "description": "ARM template path relative to the repo root."},
		"branchErrors": enumSchema([]string{"best", "all"}, "Temporarily override output.branchErrors."),
	}), s.handleAzurePruningReport, true)
	s.addTool("release_readiness", "Run release-readiness checks without publishing anything.", schemaObject(map[string]any{
		"snapshot": map[string]any{"type": "boolean", "description": "Deprecated; GoReleaser snapshot validation always runs for the Pro config."},
	}), s.handleReleaseReadiness, false)
	s.addTool("release_status", "Inspect recent release-related workflow runs and tags.", schemaObject(nil), s.handleReleaseStatus, true)
	s.addTool("prepare_release", "Dry-run or trigger the Prepare Release workflow. dryRun defaults to true; dryRun=false triggers an externally visible GitHub workflow.", schemaObject(map[string]any{
		"version": map[string]any{"type": "string", "description": "Release tag to create, for example v0.1.2."},
		"dryRun":  map[string]any{"type": "boolean", "description": "When true, validate readiness without triggering GitHub Actions. Defaults to true."},
	}), s.handlePrepareRelease, false)
}

type toolHints struct {
	ReadOnly  bool
	OpenWorld bool
}

func (s *repoServer) addTool(name, description string, input map[string]any, handler server.ToolHandlerFunc, readOnly bool) {
	s.addToolWithHints(name, description, input, handler, toolHints{ReadOnly: readOnly})
}

func (s *repoServer) addToolWithHints(name, description string, input map[string]any, handler server.ToolHandlerFunc, hints toolHints) {
	tool := mcp.NewToolWithRawSchema(name, description, mustRawJSON(input))
	tool.RawOutputSchema = mustRawJSON(map[string]any{"type": "object", "additionalProperties": true})
	tool.Annotations = mcp.ToolAnnotation{
		Title:           name,
		ReadOnlyHint:    mcp.ToBoolPtr(hints.ReadOnly),
		IdempotentHint:  mcp.ToBoolPtr(hints.ReadOnly),
		DestructiveHint: mcp.ToBoolPtr(!hints.ReadOnly),
		OpenWorldHint:   mcp.ToBoolPtr(hints.OpenWorld),
	}
	s.mcp.AddTool(tool, handler)
}

func realWorldValidationToolSchema(extra map[string]any) map[string]any {
	properties := map[string]any{
		"corpusDir":          map[string]any{"type": "string", "description": "Prepared corpus directory to validate."},
		"cacheDir":           map[string]any{"type": "string", "description": "Isolated XDG cache directory. Created automatically when omitted."},
		"outputArtifact":     map[string]any{"type": "string", "description": "Merged JSON output artifact path. Created automatically when omitted."},
		"manifestPath":       map[string]any{"type": "string", "description": "Prepared corpus manifest path. Defaults to <corpusDir>/real-world-manifest.json."},
		"repositories":       realWorldRepositoryArraySchema("Repositories included in the sweep. Defaults from manifestPath when available."),
		"dependencyPrep":     realWorldDependencyPrepArraySchema("Dependency-prep notes from real_world_inspect_corpus, carried forward to triage. Do not run dependency lifecycle scripts during prep."),
		"build":              map[string]any{"type": "boolean", "description": "Build bin/dollarlint before validating. Defaults to true."},
		"schemaStore":        map[string]any{"type": "boolean", "description": "Enable SchemaStore catalog matching. Defaults to true."},
		"schemaStoreFailure": enumSchema([]string{"warn", "error", "skip"}, "SchemaStore catalog failure policy. Defaults to warn."),
		"fetchRetries":       map[string]any{"type": "integer", "description": "Remote schema fetch retries. Defaults to 1."},
		"fetchRetryMinWait":  map[string]any{"type": "string", "description": "Minimum retry wait. Defaults to 1ms."},
		"fetchRetryMaxWait":  map[string]any{"type": "string", "description": "Maximum retry wait. Defaults to 1ms."},
		"extraArgs":          arrayStringSchema("Additional dollarlint validate arguments."),
	}
	for key, value := range extra {
		properties[key] = value
	}
	return schemaObject(properties)
}

func realWorldRepositoryArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name":      map[string]any{"type": "string", "description": "Short local repository name."},
				"ecosystem": map[string]any{"type": "string", "description": "Primary ecosystem or language."},
				"cloneURL":  map[string]any{"type": "string", "description": "Public clone URL."},
				"commit":    map[string]any{"type": "string", "description": "Checked-out commit SHA."},
				"notes":     map[string]any{"type": "string", "description": "Repo-specific notes."},
				"path":      map[string]any{"type": "string", "description": "Local clone path, usually filled by prepare_corpus."},
				"status":    map[string]any{"type": "string", "description": "Preparation status, usually filled by prepare_corpus."},
				"error":     map[string]any{"type": "string", "description": "Preparation error, when any."},
				"alreadyTested": map[string]any{
					"type":        "boolean",
					"description": "Whether history already contains this repository.",
				},
				"previousEntries": arrayStringSchema("Prior real-world entry ids for this repository."),
			},
		},
	}
}

func realWorldDependencyPrepArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"repository": map[string]any{"type": "string", "description": "Repository name this prep entry applies to, when scoped to one repo."},
				"command":    map[string]any{"type": "string", "description": "Dependency-prep command that was run or intentionally skipped."},
				"status":     map[string]any{"type": "string", "description": "Outcome such as run, skipped, failed, timed-out, narrowed, or not-needed."},
				"notes":      map[string]any{"type": "string", "description": "Reason, result, and expected validation impact."},
				"error":      map[string]any{"type": "string", "description": "Failure or timeout detail, when any."},
				"output":     map[string]any{"type": "string", "description": "Relevant trimmed command output, when useful."},
			},
		},
	}
}

func realWorldProductRecommendationArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"strength", "recommendation", "rationale"},
			"properties": map[string]any{
				"strength":       enumSchema([]string{"high", "med", "low"}, "Recommendation strength based on frequency, severity, reproducibility, and user impact."),
				"recommendation": map[string]any{"type": "string", "description": "Recommended product action."},
				"rationale":      map[string]any{"type": "string", "description": "Why this recommendation follows from the sweep."},
			},
		},
	}
}
