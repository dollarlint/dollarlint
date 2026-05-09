package engine

import (
	"fmt"
	"os"
	"path/filepath"
)

type configuredFile struct {
	file       DiscoveredFile
	config     Config
	configPath string
}

type configScope struct {
	config     Config
	configPath string
	excluder   discoveryExcluder
}

func discoverConfiguredFiles(root string, cfg Config, configPath string, overlay ConfigOverlay, output OutputConfig) ([]configuredFile, error) {
	if root == "" {
		root = "."
	}
	root, _ = filepath.Abs(filepath.Clean(root))
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat root %s: %w", root, err)
	}
	if !info.IsDir() {
		excluder := newDiscoveryPatternExcluder(cfg.Discovery)
		if discoveryRespectGitIgnore(cfg.Discovery) {
			rules, err := loadGitIgnoreRules(filepath.Dir(root))
			if err != nil {
				return nil, err
			}
			excluder.gitIgnoreRules = rules
		}
		files, err := discoverSingleFile(root, filepath.Base(root), cfg.Discovery, excluder)
		if err != nil {
			return nil, err
		}
		configured := make([]configuredFile, 0, len(files))
		for _, file := range files {
			configured = append(configured, configuredFile{file: file, config: cfg, configPath: configPath})
		}
		return configured, nil
	}
	scope := configScope{
		config:     cfg,
		configPath: configPath,
		excluder:   newDiscoveryPatternExcluder(cfg.Discovery),
	}
	var files []configuredFile
	if err := walkConfiguredDir(root, root, scope, overlay, output, &files); err != nil {
		return nil, fmt.Errorf("discover files: %w", err)
	}
	return files, nil
}

func walkConfiguredDir(root, dir string, scope configScope, overlay ConfigOverlay, output OutputConfig, files *[]configuredFile) error {
	rel := ""
	if dir != root {
		relPath, _ := filepath.Rel(root, dir)
		rel = filepath.ToSlash(relPath)
		if scope.excluder.ignored(rel, true) {
			return nil
		}
	}
	scope, err := scopeForDirectory(root, dir, rel, scope, overlay, output)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		childRel, _ := filepath.Rel(root, path)
		childRel = filepath.ToSlash(childRel)
		if entry.IsDir() {
			if err := walkConfiguredDir(root, path, scope, overlay, output, files); err != nil {
				return err
			}
			continue
		}
		if !scope.config.Discovery.FollowSymlinks && entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if scope.excluder.ignored(childRel, false) || !matchAny(scope.config.Discovery.Include, childRel) {
			continue
		}
		*files = append(*files, configuredFile{
			file: DiscoveredFile{
				Path:         path,
				RelativePath: childRel,
			},
			config:     scope.config,
			configPath: scope.configPath,
		})
	}
	return nil
}

func scopeForDirectory(root, dir, rel string, scope configScope, overlay ConfigOverlay, output OutputConfig) (configScope, error) {
	configPath := filepath.Join(dir, ".dollarlint.toml")
	if configPath != scope.configPath {
		if _, err := os.Stat(configPath); err == nil {
			loaded, err := loadResolvedConfig(root, configPath, nil)
			if err != nil {
				return scope, err
			}
			loaded.ApplyDefaults()
			if overlay != nil {
				if err := overlay(&loaded); err != nil {
					return scope, err
				}
				loaded.ApplyDefaults()
			}
			loaded.Output = output
			excluder := newDiscoveryPatternExcluder(loaded.Discovery)
			if discoveryRespectGitIgnore(loaded.Discovery) && discoveryRespectGitIgnore(scope.config.Discovery) {
				excluder.gitIgnoreRules = append([]gitIgnoreRule(nil), scope.excluder.gitIgnoreRules...)
			}
			scope = configScope{
				config:     loaded,
				configPath: configPath,
				excluder:   excluder,
			}
		} else if err != nil && !os.IsNotExist(err) {
			return scope, fmt.Errorf("config %s: %w", configPath, err)
		}
	}
	if discoveryRespectGitIgnore(scope.config.Discovery) {
		rules, err := loadGitIgnoreRulesAt(dir, rel)
		if err != nil {
			return scope, err
		}
		scope.excluder.gitIgnoreRules = append(scope.excluder.gitIgnoreRules, rules...)
	}
	return scope, nil
}

func newDiscoveryPatternExcluder(cfg DiscoveryConfig) discoveryExcluder {
	excluder := discoveryExcluder{}
	if discoveryUseDefaultExcludes(cfg) {
		excluder.patterns = append(excluder.patterns, defaultDiscoveryExcludes...)
	}
	excluder.patterns = append(excluder.patterns, cfg.Exclude...)
	return excluder
}
