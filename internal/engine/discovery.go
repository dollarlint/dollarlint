package engine

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type DiscoveredFile struct {
	Path         string
	RelativePath string
}

func DiscoverFiles(root string, cfg DiscoveryConfig) ([]DiscoveredFile, error) {
	if root == "" {
		root = "."
	}
	root, _ = filepath.Abs(filepath.Clean(root))
	cfg = normalizedDiscoveryConfig(cfg)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat root %s: %w", root, err)
	}
	excluder, err := newDiscoveryExcluder(root, info.IsDir(), cfg)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return discoverSingleFile(root, filepath.Base(root), cfg, excluder)
	}
	var files []DiscoveredFile
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if excluder.ignored(rel, true) {
				return filepath.SkipDir
			}
			if discoveryRespectGitIgnore(cfg) {
				rules, err := loadGitIgnoreRulesAt(path, rel)
				if err != nil {
					return err
				}
				excluder.gitIgnoreRules = append(excluder.gitIgnoreRules, rules...)
			}
			return nil
		}
		if !cfg.FollowSymlinks && entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if excluder.ignored(rel, false) || !matchAny(cfg.Include, rel) {
			return nil
		}
		files = append(files, DiscoveredFile{Path: path, RelativePath: rel})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover files: %w", err)
	}
	return files, nil
}

func discoverSingleFile(path, rel string, cfg DiscoveryConfig, excluder discoveryExcluder) ([]DiscoveredFile, error) {
	rel = filepath.ToSlash(rel)
	if (cfg.ForceExclude && excluder.ignored(rel, false)) || !matchAny(cfg.Include, rel) {
		return nil, nil
	}
	return []DiscoveredFile{{Path: path, RelativePath: rel}}, nil
}

var defaultDiscoveryExcludes = []string{
	".bzr", "**/.bzr/**",
	".direnv", "**/.direnv/**",
	".eggs", "**/.eggs/**",
	".git", "**/.git/**",
	".git-rewrite", "**/.git-rewrite/**",
	".hg", "**/.hg/**",
	".ipynb_checkpoints", "**/.ipynb_checkpoints/**",
	".mypy_cache", "**/.mypy_cache/**",
	".nox", "**/.nox/**",
	".pants.d", "**/.pants.d/**",
	".pyenv", "**/.pyenv/**",
	".pytest_cache", "**/.pytest_cache/**",
	".pytype", "**/.pytype/**",
	".ruff_cache", "**/.ruff_cache/**",
	".svn", "**/.svn/**",
	".tox", "**/.tox/**",
	".venv", "**/.venv/**",
	".build", "**/.build/**",
	"__pypackages__", "**/__pypackages__/**",
	".next", "**/.next/**",
	".nuxt", "**/.nuxt/**",
	".turbo", "**/.turbo/**",
	".cache", "**/.cache/**",
	"buck-out", "**/buck-out/**",
	"build", "**/build/**",
	"coverage", "**/coverage/**",
	"DerivedData", "**/DerivedData/**",
	"dist", "**/dist/**",
	"Intermediates.noindex", "**/Intermediates.noindex/**",
	"node_modules", "**/node_modules/**",
	"SourcePackages/checkouts", "**/SourcePackages/checkouts", "**/SourcePackages/checkouts/**",
	"target", "**/target/**",
	"temp", "**/temp/**",
	"tmp", "**/tmp/**",
	"vendor", "**/vendor/**",
	"venv", "**/venv/**",
	"*.dSYM", "**/*.dSYM/**",
}

type discoveryExcluder struct {
	patterns       []string
	gitIgnoreRules []gitIgnoreRule
}

func newDiscoveryExcluder(root string, rootIsDir bool, cfg DiscoveryConfig) (discoveryExcluder, error) {
	excluder := discoveryExcluder{}
	if discoveryUseDefaultExcludes(cfg) {
		excluder.patterns = append(excluder.patterns, defaultDiscoveryExcludes...)
	}
	excluder.patterns = append(excluder.patterns, cfg.Exclude...)
	if discoveryRespectGitIgnore(cfg) && rootIsDir {
		rules, err := loadGitIgnoreRules(root)
		if err != nil {
			return excluder, err
		}
		excluder.gitIgnoreRules = rules
	}
	return excluder, nil
}

func normalizedDiscoveryConfig(cfg DiscoveryConfig) DiscoveryConfig {
	defaults := DefaultConfig().Discovery
	if cfg.Include == nil {
		cfg.Include = defaults.Include
	}
	if cfg.UseDefaultExcludes == nil {
		cfg.UseDefaultExcludes = defaults.UseDefaultExcludes
	}
	if cfg.RespectGitIgnore == nil {
		cfg.RespectGitIgnore = defaults.RespectGitIgnore
	}
	return cfg
}

func discoveryUseDefaultExcludes(cfg DiscoveryConfig) bool {
	return cfg.UseDefaultExcludes == nil || *cfg.UseDefaultExcludes
}

func discoveryRespectGitIgnore(cfg DiscoveryConfig) bool {
	return cfg.RespectGitIgnore == nil || *cfg.RespectGitIgnore
}

func (e discoveryExcluder) ignored(rel string, isDir bool) bool {
	ignored := matchAny(e.patterns, rel)
	for _, rule := range e.gitIgnoreRules {
		if rule.matches(rel, isDir) {
			ignored = !rule.Negated
		}
	}
	return ignored
}

type gitIgnoreRule struct {
	Base     string
	Pattern  string
	Negated  bool
	DirOnly  bool
	Anchored bool
}

func loadGitIgnoreRules(root string) ([]gitIgnoreRule, error) {
	return loadGitIgnoreRulesAt(root, "")
}

func loadGitIgnoreRulesAt(root, base string) ([]gitIgnoreRule, error) {
	path := filepath.Join(root, ".gitignore")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read gitignore %s: %w", path, err)
	}
	defer file.Close()
	var rules []gitIgnoreRule
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if rule, ok := parseGitIgnoreRule(base, scanner.Text()); ok {
			rules = append(rules, rule)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read gitignore %s: %w", path, err)
	}
	return rules, nil
}

func parseGitIgnoreRule(base, raw string) (gitIgnoreRule, bool) {
	line := strings.TrimRight(raw, " \t\r")
	if line == "" || strings.HasPrefix(line, "#") {
		return gitIgnoreRule{}, false
	}
	escapedPrefix := strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`)
	if escapedPrefix {
		line = strings.TrimPrefix(line, `\`)
	}
	rule := gitIgnoreRule{Base: cleanGlob(base)}
	if !escapedPrefix && strings.HasPrefix(line, "!") {
		rule.Negated = true
		line = strings.TrimPrefix(line, "!")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return gitIgnoreRule{}, false
	}
	if strings.HasSuffix(line, "/") {
		rule.DirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		rule.Anchored = true
		line = strings.TrimPrefix(line, "/")
	}
	rule.Pattern = cleanGlob(line)
	return rule, rule.Pattern != ""
}

func (r gitIgnoreRule) matches(rel string, isDir bool) bool {
	rel = cleanGlob(rel)
	if r.Base != "" {
		if rel != r.Base && !strings.HasPrefix(rel, r.Base+"/") {
			return false
		}
		rel = strings.TrimPrefix(strings.TrimPrefix(rel, r.Base), "/")
	}
	if rel == "" {
		return false
	}
	if r.Anchored && !strings.Contains(r.Pattern, "/") {
		if r.DirOnly {
			return rel == r.Pattern || strings.HasPrefix(rel, r.Pattern+"/")
		}
		return rel == r.Pattern
	}
	if !strings.Contains(r.Pattern, "/") {
		return gitIgnoreBasenameMatch(r.Pattern, rel, r.DirOnly, isDir)
	}
	if r.DirOnly {
		return rel == r.Pattern || strings.HasPrefix(rel, r.Pattern+"/") ||
			(!r.Anchored && matchPattern("**/"+r.Pattern+"/**", rel))
	}
	if matchPattern(r.Pattern, rel) {
		return true
	}
	return !r.Anchored && matchPattern("**/"+r.Pattern, rel)
}

func gitIgnoreBasenameMatch(pattern, rel string, dirOnly, isDir bool) bool {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if ok := matchPattern(pattern, part); !ok {
			continue
		}
		if !dirOnly {
			return i == len(parts)-1 || isDir
		}
		if i < len(parts)-1 || isDir {
			return true
		}
	}
	return false
}
