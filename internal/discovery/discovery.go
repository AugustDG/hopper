package discovery

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AugustDG/hopper/internal/config"
	"github.com/AugustDG/hopper/internal/model"
	"github.com/bmatcuk/doublestar/v4"
)

func Discover(cfg config.Config, only []string) ([]model.Project, error) {
	hidden := makeHiddenSet(cfg.HiddenProjects)
	if len(only) > 0 {
		return projectsFromOnly(only, hidden)
	}
	seen := map[string]struct{}{}
	results := make([]model.Project, 0, 256)

	for _, root := range cfg.Roots {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		if err := walkRoot(root, cfg, seen, &results); err != nil {
			return nil, err
		}
	}

	for _, p := range cfg.Pinned {
		if _, blocked := hidden[p]; blocked {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			continue
		}
		results = append(results, projectFromPath(p))
		seen[p] = struct{}{}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	return results, nil
}

func projectsFromOnly(paths []string, hidden map[string]struct{}) ([]model.Project, error) {
	seen := map[string]struct{}{}
	out := make([]model.Project, 0, len(paths))
	for _, raw := range paths {
		p := config.ExpandPath(raw)
		if p == "" {
			continue
		}
		if _, blocked := hidden[p]; blocked {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			continue
		}
		out = append(out, projectFromPath(p))
		seen[p] = struct{}{}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func walkRoot(root string, cfg config.Config, seen map[string]struct{}, results *[]model.Project) error {
	rootDepth := strings.Count(root, string(os.PathSeparator))
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		relSlash := filepath.ToSlash(rel)
		if excluded(relSlash, cfg.Exclude) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !cfg.ScanHidden && strings.HasPrefix(d.Name(), ".") && d.IsDir() {
			return filepath.SkipDir
		}

		depth := strings.Count(path, string(os.PathSeparator)) - rootDepth
		if depth > cfg.MaxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if !cfg.FollowSymlinks {
			if info, err := d.Info(); err == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return filepath.SkipDir
				}
			}
		}

		if hasMarker(path, cfg.ProjectMarkers) {
			abs := config.ExpandPath(path)
			if _, ok := seen[abs]; !ok {
				*results = append(*results, projectFromPath(abs))
				seen[abs] = struct{}{}
			}
			return filepath.SkipDir
		}
		return nil
	})
}

func excluded(relPath string, patterns []string) bool {
	if relPath == "." || relPath == "" {
		return false
	}
	for _, p := range patterns {
		if ok, _ := doublestar.Match(p, relPath); ok {
			return true
		}
	}
	return false
}

func hasMarker(path string, markers []string) bool {
	for _, marker := range markers {
		markerPath := filepath.Join(path, marker)
		if isGlob(marker) {
			matches, err := filepath.Glob(markerPath)
			if err == nil && len(matches) > 0 {
				return true
			}
			continue
		}
		if _, err := os.Stat(markerPath); err == nil {
			return true
		}
	}
	return false
}

func isGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func projectFromPath(path string) model.Project {
	return model.Project{
		Path: path,
		Name: filepath.Base(path),
	}
}

func makeHiddenSet(paths []string) map[string]struct{} {
	out := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		out[p] = struct{}{}
	}
	return out
}
