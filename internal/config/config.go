package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Roots                   []string `toml:"roots"`
	Exclude                 []string `toml:"exclude"`
	ProjectMarkers          []string `toml:"project_markers"`
	Pinned                  []string `toml:"pinned"`
	HiddenProjects          []string `toml:"hidden_projects"`
	FuzzyConfidenceMinScore int      `toml:"fuzzy_confidence_min_score"`
	FuzzyConfidenceMinDelta int      `toml:"fuzzy_confidence_min_delta"`
	MaxDepth                int      `toml:"max_depth"`
	FollowSymlinks          bool     `toml:"follow_symlinks"`
	ScanHidden              bool     `toml:"scan_hidden"`
	PickerHeight            int      `toml:"picker_height"`
	StaleAfterMins          int      `toml:"stale_after_mins"`
}

func Default() Config {
	return Config{
		Roots: []string{"~/projects"},
		Exclude: []string{
			"**/.git",
			"**/node_modules",
			"**/dist",
			"**/build",
			"**/target",
		},
		ProjectMarkers: []string{
			".git",
			"go.mod",
			"package.json",
			"pyproject.toml",
			"requirements.txt",
			"Pipfile",
			"poetry.lock",
			"setup.py",
			"Cargo.toml",
			"pom.xml",
			"build.gradle",
			"build.gradle.kts",
			"settings.gradle",
			"settings.gradle.kts",
			"CMakeLists.txt",
			"Makefile",
			"composer.json",
			"Gemfile",
			"mix.exs",
			"stack.yaml",
			"*.sln",
			"*.csproj",
			"*.fsproj",
			"*.xcodeproj",
			"*.xcworkspace",
			"deno.json",
			"tsconfig.json",
			"vite.config.ts",
			"vite.config.js",
			"next.config.js",
			"next.config.mjs",
			"nuxt.config.ts",
			"angular.json",
			"pubspec.yaml",
			"Project.toml",
			"Manifest.toml",
			"Dockerfile",
			"docker-compose.yml",
			"*.go",
			"*.rs",
			"*.py",
			"*.js",
			"*.ts",
			"*.tsx",
			"*.java",
			"*.kt",
			"*.swift",
			"*.rb",
			"*.php",
			"*.c",
			"*.cpp",
			"*.cc",
			"*.cs",
			"*.scala",
			"*.lua",
			"*.zig",
		},
		Pinned:                  []string{},
		HiddenProjects:          []string{},
		FuzzyConfidenceMinScore: 25,
		FuzzyConfidenceMinDelta: 10,
		MaxDepth:                4,
		FollowSymlinks:          false,
		ScanHidden:              false,
		PickerHeight:            12,
		StaleAfterMins:          30,
	}
}

func Load() (Config, string, error) {
	cfg := Default()
	path, err := configPath()
	if err != nil {
		return cfg, "", err
	}
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &cfg); err != nil {
			return cfg, path, err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		normalize(&cfg)
		if err := Save(path, cfg); err != nil {
			return cfg, path, err
		}
		return cfg, path, nil
	} else if err != nil {
		return cfg, path, err
	}
	normalize(&cfg)
	return cfg, path, nil
}

func normalize(cfg *Config) {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 4
	}
	if cfg.PickerHeight <= 0 {
		cfg.PickerHeight = 12
	}
	if cfg.StaleAfterMins <= 0 {
		cfg.StaleAfterMins = 30
	}
	if len(cfg.ProjectMarkers) == 0 {
		cfg.ProjectMarkers = Default().ProjectMarkers
	}
	if cfg.FuzzyConfidenceMinScore <= 0 {
		cfg.FuzzyConfidenceMinScore = Default().FuzzyConfidenceMinScore
	}
	if cfg.FuzzyConfidenceMinDelta <= 0 {
		cfg.FuzzyConfidenceMinDelta = Default().FuzzyConfidenceMinDelta
	}

	allRoots := append([]string{"~/projects"}, cfg.Roots...)
	rootSet := map[string]struct{}{}
	normalized := make([]string, 0, len(allRoots))
	for _, r := range allRoots {
		ex := ExpandPath(r)
		if ex == "" {
			continue
		}
		if _, ok := rootSet[ex]; ok {
			continue
		}
		rootSet[ex] = struct{}{}
		normalized = append(normalized, ex)
	}
	sort.Strings(normalized)
	cfg.Roots = normalized

	pins := make([]string, 0, len(cfg.Pinned))
	for _, p := range cfg.Pinned {
		ex := ExpandPath(p)
		if ex == "" {
			continue
		}
		pins = append(pins, ex)
	}
	sort.Strings(pins)
	cfg.Pinned = pins

	hidden := make([]string, 0, len(cfg.HiddenProjects))
	seenHidden := map[string]struct{}{}
	for _, p := range cfg.HiddenProjects {
		ex := ExpandPath(p)
		if ex == "" {
			continue
		}
		if _, ok := seenHidden[ex]; ok {
			continue
		}
		seenHidden[ex] = struct{}{}
		hidden = append(hidden, ex)
	}
	sort.Strings(hidden)
	cfg.HiddenProjects = hidden
}

func Save(path string, cfg Config) error {
	normalize(&cfg)
	if err := EnsureParent(path); err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	if err := toml.NewEncoder(buf).Encode(cfg); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func configPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); p != "" {
		return filepath.Join(p, "hopper", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "hopper", "config.toml"), nil
}

func CachePath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); p != "" {
		return filepath.Join(p, "hopper", "index-v1.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "hopper", "index-v1.json"), nil
}

func ExpandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				path = home
			} else {
				path = filepath.Join(home, path[2:])
			}
		}
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func EnsureParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
