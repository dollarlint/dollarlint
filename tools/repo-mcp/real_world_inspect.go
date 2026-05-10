package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	realWorldInspectDefaultMaxMatches = 12
	realWorldInspectMaxFileBytes      = 512 * 1024
)

var realWorldSchemaRefPattern = regexp.MustCompile(`(?i)["']?\$schema["']?\s*[:=]\s*["']?([^"',\s#}]+)`)

type realWorldInspectArgs struct {
	RunID          string                `json:"runID"`
	CorpusDir      string                `json:"corpusDir"`
	CacheDir       string                `json:"cacheDir"`
	OutputArtifact string                `json:"outputArtifact"`
	ManifestPath   string                `json:"manifestPath"`
	Repositories   []realWorldRepository `json:"repositories"`
	MaxMatches     int                   `json:"maxMatches"`
}

type realWorldCorpusInspection struct {
	CorpusDir           string                        `json:"corpusDir,omitempty"`
	CacheDir            string                        `json:"cacheDir,omitempty"`
	OutputArtifact      string                        `json:"outputArtifact,omitempty"`
	ManifestPath        string                        `json:"manifestPath,omitempty"`
	Repositories        []realWorldDependencyPrepScan `json:"repositories"`
	DraftDependencyPrep []realWorldDependencyPrep     `json:"draftDependencyPrep"`
	PrepSecurityPolicy  map[string]any                `json:"prepSecurityPolicy"`
	Summary             string                        `json:"summary"`
	NeedsReview         bool                          `json:"needsReview"`
}

type realWorldDependencyPrepScan struct {
	Repository          string                `json:"repository"`
	Path                string                `json:"path,omitempty"`
	ScannedFiles        int                   `json:"scannedFiles"`
	Truncated           bool                  `json:"truncated,omitempty"`
	Lockfiles           []string              `json:"lockfiles,omitempty"`
	DependencyManifests []string              `json:"dependencyManifests,omitempty"`
	LocalSchemaRefs     []realWorldSchemaRef  `json:"localSchemaRefs,omitempty"`
	RemoteSchemaRefs    []realWorldSchemaRef  `json:"remoteSchemaRefs,omitempty"`
	TextSignals         []realWorldTextSignal `json:"textSignals,omitempty"`
	NeedsDependencyPrep bool                  `json:"needsDependencyPrep"`
	SuggestedCommands   []string              `json:"suggestedCommands,omitempty"`
	Notes               []string              `json:"notes,omitempty"`
	Error               string                `json:"error,omitempty"`
}

type realWorldSchemaRef struct {
	File  string `json:"file"`
	Line  int    `json:"line,omitempty"`
	Value string `json:"value"`
	Kind  string `json:"kind"`
}

type realWorldTextSignal struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Pattern string `json:"pattern"`
}

func (s *repoServer) handleRealWorldInspectCorpus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args realWorldInspectArgs
	_ = request.BindArguments(&args)
	if err := realWorldRejectManualPathArgsWithRunID(args.RunID, map[string]string{
		"corpusDir":      args.CorpusDir,
		"cacheDir":       args.CacheDir,
		"outputArtifact": args.OutputArtifact,
		"manifestPath":   args.ManifestPath,
	}); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if args.RunID != "" {
		if s.realWorldPrepareRuns == nil {
			return mcp.NewToolResultError(fmt.Sprintf("corpus preparation run %q was not found", args.RunID)), nil
		}
		run, ok := s.realWorldPrepareRuns.get(args.RunID)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("corpus preparation run %q was not found", args.RunID)), nil
		}
		if args.CorpusDir == "" {
			args.CorpusDir = run.CorpusDir
		}
		if args.CacheDir == "" {
			args.CacheDir = run.CacheDir
		}
		if args.OutputArtifact == "" {
			args.OutputArtifact = run.OutputArtifact
		}
		if args.ManifestPath == "" {
			args.ManifestPath = run.ManifestPath
		}
		if len(args.Repositories) == 0 {
			args.Repositories = run.repositories()
		}
	}
	inspection, err := realWorldInspectCorpus(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.realWorldStructured(ctx, map[string]any{
		"ok":                 true,
		"repositories":       realWorldPublicInspection(inspection.Repositories),
		"dependencyPrep":     inspection.DraftDependencyPrep,
		"prepSecurityPolicy": inspection.PrepSecurityPolicy,
		"summary":            inspection.Summary,
		"needsReview":        inspection.NeedsReview,
		"nextStep":           realWorldNextRunCorpus(args.RunID, inspection.CorpusDir, inspection.CacheDir, inspection.OutputArtifact, inspection.ManifestPath, inspection.DraftDependencyPrep),
	})
}

func realWorldInspectCorpus(args realWorldInspectArgs) (realWorldCorpusInspection, error) {
	manifestPath := args.ManifestPath
	if manifestPath == "" && args.CorpusDir != "" {
		manifestPath = filepath.Join(args.CorpusDir, realWorldManifestName)
	}
	repositories := append([]realWorldRepository{}, args.Repositories...)
	if manifestPath != "" {
		manifest, err := readRealWorldManifest(manifestPath)
		if err == nil {
			if args.CorpusDir == "" {
				args.CorpusDir = manifest.CorpusDir
			}
			if args.CacheDir == "" {
				args.CacheDir = manifest.CacheDir
			}
			if args.OutputArtifact == "" {
				args.OutputArtifact = manifest.OutputArtifact
			}
			if len(repositories) == 0 {
				repositories = manifest.Repositories
			}
		}
	}
	if args.CorpusDir == "" {
		return realWorldCorpusInspection{}, fmt.Errorf("corpusDir is required; call real_world_prepare_corpus first")
	}
	if args.MaxMatches <= 0 {
		args.MaxMatches = realWorldInspectDefaultMaxMatches
	}
	scans := make([]realWorldDependencyPrepScan, 0, len(repositories))
	for _, repo := range repositories {
		if repo.Path == "" {
			continue
		}
		scans = append(scans, realWorldInspectRepository(repo, args.MaxMatches))
	}
	draft := realWorldDraftDependencyPrep(scans)
	summary, needsReview := realWorldInspectionSummary(scans)
	return realWorldCorpusInspection{
		CorpusDir:           args.CorpusDir,
		CacheDir:            args.CacheDir,
		OutputArtifact:      args.OutputArtifact,
		ManifestPath:        manifestPath,
		Repositories:        scans,
		DraftDependencyPrep: draft,
		PrepSecurityPolicy:  realWorldDependencyPrepSecurityPolicy(),
		Summary:             summary,
		NeedsReview:         needsReview,
	}, nil
}

func realWorldInspectRepository(repo realWorldRepository, maxMatches int) realWorldDependencyPrepScan {
	name := nonEmpty(repo.Name, repoNameFromURL(repo.CloneURL), filepath.Base(repo.Path))
	scan := realWorldDependencyPrepScan{Repository: name, Path: repo.Path}
	info, err := os.Stat(repo.Path)
	if err != nil {
		scan.Error = fmt.Sprintf("repository path is not readable: %v", err)
		scan.Notes = append(scan.Notes, "Could not scan clone; record dependency prep manually.")
		return scan
	}
	if !info.IsDir() {
		scan.Error = "repository path is not a directory"
		scan.Notes = append(scan.Notes, "Could not scan clone; record dependency prep manually.")
		return scan
	}
	err = filepath.WalkDir(repo.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if realWorldShouldSkipInspectDir(name) && path != repo.Path {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(repo.Path, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		scan.ScannedFiles++
		lowerName := strings.ToLower(name)
		if realWorldIsLockfileName(lowerName) {
			scan.Lockfiles = appendUniqueLimit(scan.Lockfiles, rel, maxMatches)
		}
		if realWorldIsDependencyManifestName(lowerName) {
			scan.DependencyManifests = appendUniqueLimit(scan.DependencyManifests, rel, maxMatches)
		}
		if !realWorldShouldInspectTextFile(path, lowerName) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > realWorldInspectMaxFileBytes {
			return nil
		}
		realWorldInspectSchemaText(rel, string(data), maxMatches, &scan)
		return nil
	})
	if err != nil {
		scan.Error = err.Error()
	}
	scan.NeedsDependencyPrep = len(scan.LocalSchemaRefs) > 0
	if scan.NeedsDependencyPrep {
		scan.SuggestedCommands = realWorldSuggestedPrepCommands(scan)
	}
	scan.Notes = realWorldDependencyPrepScanNotes(scan)
	sort.Strings(scan.Lockfiles)
	sort.Strings(scan.DependencyManifests)
	return scan
}

func realWorldInspectSchemaText(rel, text string, maxMatches int, scan *realWorldDependencyPrepScan) {
	for i, line := range strings.Split(text, "\n") {
		lineNo := i + 1
		matches := realWorldSchemaRefPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			value := strings.Trim(match[1], `"',`)
			ref := realWorldSchemaRef{File: rel, Line: lineNo, Value: value, Kind: realWorldSchemaRefKind(value)}
			if realWorldSchemaRefIsLocal(ref.Kind) {
				scan.LocalSchemaRefs = appendSchemaRefLimit(scan.LocalSchemaRefs, ref, maxMatches)
			} else {
				scan.RemoteSchemaRefs = appendSchemaRefLimit(scan.RemoteSchemaRefs, ref, maxMatches)
			}
		}
		for _, pattern := range []string{"$schema", "node_modules", "file:"} {
			if strings.Contains(line, pattern) {
				scan.TextSignals = appendTextSignalLimit(scan.TextSignals, realWorldTextSignal{File: rel, Line: lineNo, Pattern: pattern}, maxMatches)
			}
		}
	}
}

func realWorldDraftDependencyPrep(scans []realWorldDependencyPrepScan) []realWorldDependencyPrep {
	prep := make([]realWorldDependencyPrep, 0, len(scans))
	for _, scan := range scans {
		entry := realWorldDependencyPrep{Repository: scan.Repository}
		switch {
		case scan.Error != "":
			entry.Status = "failed"
			entry.Notes = "MCP first-pass dependency scan failed; inspect this repository manually before interpreting validation fidelity."
			entry.Error = scan.Error
		case scan.NeedsDependencyPrep:
			entry.Status = "needs-review"
			entry.Command = strings.Join(scan.SuggestedCommands, " OR ")
			if len(scan.SuggestedCommands) == 0 {
				entry.Notes = fmt.Sprintf("MCP first-pass scan found %d local $schema refs, including %s, but did not infer a script-suppressed dependency prep command. Skip prep or inspect manually instead of running a general install.", len(scan.LocalSchemaRefs), firstSchemaRefSummary(scan.LocalSchemaRefs))
			} else {
				entry.Notes = fmt.Sprintf("MCP first-pass scan found %d local $schema refs, including %s. Run only a bounded dependency prep command that disables package lifecycle scripts if needed, or replace this with a skipped/not-needed note before recording.", len(scan.LocalSchemaRefs), firstSchemaRefSummary(scan.LocalSchemaRefs))
			}
		default:
			entry.Status = "not-needed"
			entry.Notes = fmt.Sprintf("MCP first-pass scan found no local $schema refs; dependency install is not needed for schema fidelity. Lockfiles: %s. Remote schema refs: %d.", summarizeStrings(scan.Lockfiles), len(scan.RemoteSchemaRefs))
		}
		prep = append(prep, entry)
	}
	return prep
}

func realWorldInspectionSummary(scans []realWorldDependencyPrepScan) (string, bool) {
	var repos, needsPrep, localRefs, remoteRefs, lockfiles int
	for _, scan := range scans {
		repos++
		if scan.NeedsDependencyPrep || scan.Error != "" {
			needsPrep++
		}
		localRefs += len(scan.LocalSchemaRefs)
		remoteRefs += len(scan.RemoteSchemaRefs)
		lockfiles += len(scan.Lockfiles)
	}
	return fmt.Sprintf("Scanned %d repositories: %d lockfiles, %d local schema refs, %d remote schema refs, %d repositories need dependency-prep review.", repos, lockfiles, localRefs, remoteRefs, needsPrep), needsPrep > 0
}

func realWorldDependencyPrepScanNotes(scan realWorldDependencyPrepScan) []string {
	if scan.Error != "" {
		return []string{"Scan failed; inspect this repository manually."}
	}
	var notes []string
	if len(scan.Lockfiles) > 0 {
		notes = append(notes, "Lockfiles detected: "+summarizeStrings(scan.Lockfiles)+".")
	}
	if len(scan.LocalSchemaRefs) > 0 && len(scan.SuggestedCommands) > 0 {
		notes = append(notes, "Suggested prep commands disable package lifecycle scripts; do not remove those protections.")
	}
	if len(scan.LocalSchemaRefs) > 0 {
		notes = append(notes, fmt.Sprintf("Local $schema refs detected: %s.", firstSchemaRefSummary(scan.LocalSchemaRefs)))
		if len(scan.SuggestedCommands) == 0 {
			notes = append(notes, "No script-suppressed dependency prep command was inferred; skip prep or inspect manually instead of running a general install.")
		}
	} else {
		notes = append(notes, "No local $schema refs detected.")
	}
	if len(scan.RemoteSchemaRefs) > 0 {
		notes = append(notes, fmt.Sprintf("Remote $schema refs detected: %d.", len(scan.RemoteSchemaRefs)))
	}
	return notes
}

func realWorldSuggestedPrepCommands(scan realWorldDependencyPrepScan) []string {
	var commands []string
	for _, lockfile := range scan.Lockfiles {
		switch strings.ToLower(filepath.Base(lockfile)) {
		case "package-lock.json", "npm-shrinkwrap.json":
			commands = appendUnique(commands, "npm_config_ignore_scripts=true npm ci --ignore-scripts --audit=false --fund=false")
		case "pnpm-lock.yaml":
			commands = appendUnique(commands, "npm_config_ignore_scripts=true pnpm install --frozen-lockfile --ignore-scripts")
		case "yarn.lock":
			commands = appendUnique(commands, "YARN_ENABLE_SCRIPTS=false yarn install --frozen-lockfile --ignore-scripts")
		case "bun.lock", "bun.lockb":
			commands = appendUnique(commands, "bun install --frozen-lockfile --ignore-scripts")
		case "composer.lock":
			commands = appendUnique(commands, "composer install --no-scripts --no-plugins")
		case "cargo.lock":
			commands = appendUnique(commands, "cargo fetch --locked")
		case "go.sum":
			commands = appendUnique(commands, "go mod download")
		}
	}
	return commands
}

func realWorldDependencyPrepSecurityPolicy() map[string]any {
	return map[string]any{
		"lifecycleScripts": "disabled",
		"principle":        "Dependency prep is only for local schema fidelity. Do not run package lifecycle hooks, build hooks, plugins, or repository install scripts.",
		"allowed": []string{
			"Run package-manager fetch/install commands only when they are needed to resolve local $schema refs.",
			"Use suggestedCommands as written, or an equivalent command with lifecycle scripts disabled.",
			"Use fetch-only commands such as go mod download or cargo fetch --locked when available.",
		},
		"forbidden": []string{
			"npm install, npm ci, pnpm install, yarn install, or bun install without script suppression.",
			"npm/pnpm/yarn/bun/composer lifecycle scripts, postinstall hooks, plugins, or repository-provided install scripts.",
			"Python, Ruby, Gradle, Maven, Mix, Dart, or other dependency installs unless the agent can prove lifecycle/build hooks are disabled or the command only fetches metadata.",
		},
		"whenUnsafe": "Mark dependencyPrep as skipped or needs-review with notes instead of running an unsafe install command.",
	}
}

func realWorldSchemaRefKind(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
		return "remote"
	case strings.HasPrefix(lower, "file:"):
		return "local-file"
	case strings.Contains(lower, "node_modules"):
		return "node-modules"
	case strings.HasPrefix(lower, "./") || strings.HasPrefix(lower, "../") || strings.HasPrefix(lower, "/"):
		return "local-path"
	default:
		return "unknown"
	}
}

func realWorldSchemaRefIsLocal(kind string) bool {
	return kind == "local-file" || kind == "node-modules" || kind == "local-path"
}

func realWorldShouldSkipInspectDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "vendor", "dist", "build", ".next", ".turbo", "target", ".venv", "venv", "__pycache__", ".cache":
		return true
	default:
		return false
	}
}

func realWorldShouldInspectTextFile(path, lowerName string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() > realWorldInspectMaxFileBytes {
		return false
	}
	ext := strings.ToLower(filepath.Ext(lowerName))
	switch ext {
	case ".json", ".jsonc", ".yaml", ".yml", ".toml":
		return true
	default:
		return strings.Contains(lowerName, "config") || strings.Contains(lowerName, "schema")
	}
}

func realWorldIsLockfileName(lowerName string) bool {
	switch lowerName {
	case "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb",
		"cargo.lock", "go.sum", "gemfile.lock", "poetry.lock", "uv.lock", "pipfile.lock", "composer.lock",
		"mix.lock", "pubspec.lock", "packages.lock.json", "gradle.lockfile":
		return true
	default:
		return false
	}
}

func realWorldIsDependencyManifestName(lowerName string) bool {
	switch lowerName {
	case "package.json", "pyproject.toml", "requirements.txt", "go.mod", "cargo.toml", "gemfile", "composer.json",
		"mix.exs", "pubspec.yaml", "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
		"settings.gradle.kts", "global.json", "nuget.config", "directory.packages.props":
		return true
	default:
		return false
	}
}

func appendSchemaRefLimit(values []realWorldSchemaRef, value realWorldSchemaRef, limit int) []realWorldSchemaRef {
	for _, existing := range values {
		if existing.File == value.File && existing.Line == value.Line && existing.Value == value.Value {
			return values
		}
	}
	if limit > 0 && len(values) >= limit {
		return values
	}
	return append(values, value)
}

func appendTextSignalLimit(values []realWorldTextSignal, value realWorldTextSignal, limit int) []realWorldTextSignal {
	for _, existing := range values {
		if existing.File == value.File && existing.Line == value.Line && existing.Pattern == value.Pattern {
			return values
		}
	}
	if limit > 0 && len(values) >= limit {
		return values
	}
	return append(values, value)
}

func firstSchemaRefSummary(refs []realWorldSchemaRef) string {
	if len(refs) == 0 {
		return "none"
	}
	ref := refs[0]
	return fmt.Sprintf("%s:%d -> %s", ref.File, ref.Line, ref.Value)
}

func summarizeStrings(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	if len(values) <= 3 {
		return strings.Join(values, ", ")
	}
	return strings.Join(values[:3], ", ") + fmt.Sprintf(", and %d more", len(values)-3)
}
