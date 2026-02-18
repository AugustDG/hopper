package index

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AugustDG/hopper/internal/config"
	"github.com/AugustDG/hopper/internal/model"
)

type File struct {
	Version  int             `json:"version"`
	Updated  time.Time       `json:"updated"`
	Projects []model.Project `json:"projects"`
}

func Load() (File, string, error) {
	path, err := config.CachePath()
	if err != nil {
		return File{}, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{Version: 1, Projects: []model.Project{}}, path, nil
		}
		return File{}, path, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, path, err
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Projects == nil {
		f.Projects = []model.Project{}
	}
	return f, path, nil
}

func Save(path string, f File) error {
	if err := config.EnsureParent(path); err != nil {
		return err
	}
	f.Version = 1
	f.Updated = time.Now()
	sort.Slice(f.Projects, func(i, j int) bool {
		return f.Projects[i].Path < f.Projects[j].Path
	})
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func IsStale(updated time.Time, staleAfterMinutes int) bool {
	if updated.IsZero() {
		return true
	}
	return time.Since(updated) > time.Duration(staleAfterMinutes)*time.Minute
}

func MergeDiscovered(current []model.Project, discovered []model.Project) []model.Project {
	byPath := map[string]model.Project{}
	for _, p := range current {
		byPath[p.Path] = p
	}
	now := time.Now()
	for _, p := range discovered {
		ex := byPath[p.Path]
		ex.Path = p.Path
		ex.Name = p.Name
		ex.LastSeen = now
		byPath[p.Path] = ex
	}
	out := make([]model.Project, 0, len(byPath))
	for _, p := range byPath {
		if p.LastSeen.IsZero() {
			p.LastSeen = now
		}
		out = append(out, p)
	}
	return out
}

func MarkOpened(projects []model.Project, path string) []model.Project {
	now := time.Now()
	found := false
	for i := range projects {
		if projects[i].Path == path {
			projects[i].OpenCount++
			projects[i].LastOpened = now
			projects[i].LastSeen = now
			if projects[i].Name == "" {
				projects[i].Name = filepath.Base(path)
			}
			found = true
			break
		}
	}
	if !found {
		projects = append(projects, model.Project{
			Path:       path,
			Name:       filepath.Base(path),
			LastSeen:   now,
			LastOpened: now,
			OpenCount:  1,
		})
	}
	return projects
}

func SortByRecency(projects []model.Project) {
	sort.Slice(projects, func(i, j int) bool {
		a := projects[i]
		b := projects[j]
		if !a.LastOpened.Equal(b.LastOpened) {
			return a.LastOpened.After(b.LastOpened)
		}
		if a.OpenCount != b.OpenCount {
			return a.OpenCount > b.OpenCount
		}
		return a.Name < b.Name
	})
}

func PruneMissing(projects []model.Project) ([]model.Project, bool) {
	if len(projects) == 0 {
		return projects, false
	}

	out := make([]model.Project, 0, len(projects))
	changed := false
	for _, p := range projects {
		if strings.TrimSpace(p.Path) == "" {
			changed = true
			continue
		}
		info, err := os.Stat(p.Path)
		if err != nil || !info.IsDir() {
			changed = true
			continue
		}
		out = append(out, p)
	}

	if !changed {
		return projects, false
	}
	return out, true
}
