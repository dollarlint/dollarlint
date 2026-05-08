package dollarlint

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat root %s: %w", root, err)
	}
	if !info.IsDir() {
		return discoverSingleFile(root, filepath.Base(root), cfg)
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
			if matchAny(cfg.Exclude, rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !cfg.FollowSymlinks && entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if matchAny(cfg.Exclude, rel) || !matchAny(cfg.Include, rel) {
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

func discoverSingleFile(path, rel string, cfg DiscoveryConfig) ([]DiscoveredFile, error) {
	rel = filepath.ToSlash(rel)
	if matchAny(cfg.Exclude, rel) || !matchAny(cfg.Include, rel) {
		return nil, nil
	}
	return []DiscoveredFile{{Path: path, RelativePath: rel}}, nil
}
