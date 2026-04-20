package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AugustDG/hopper/internal/config"
	"github.com/AugustDG/hopper/internal/discovery"
	"github.com/AugustDG/hopper/internal/index"
	"github.com/AugustDG/hopper/internal/model"
	"github.com/AugustDG/hopper/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

var (
	pinnedPathStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	missingPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func Run(args []string) error {
	if _, _, err := config.Load(); err != nil {
		return err
	}

	if len(args) == 0 {
		return runPick(nil)
	}

	switch args[0] {
	case "pick":
		return runPick(args[1:])
	case "add":
		return runAdd(args[1:])
	case "remove":
		return runRemove(args[1:])
	case "list":
		return runList(args[1:])
	case "query":
		return runQuery(args[1:])
	case "recent":
		return runRecent(args[1:])
	case "index":
		return runIndex(args[1:])
	case "init":
		return runInit(args[1:])
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		return runPick(args)
	}
}

func runPick(args []string) error {
	fs := flag.NewFlagSet("pick", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	only := multiValue{}
	fs.Var(&only, "project", "project directory to include (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	for {
		cfg, projects, idx, cachePath, err := getProjects(only)
		if err != nil {
			return err
		}
		missingPinned := missingPinnedPaths(cfg)
		if len(projects) == 0 && len(missingPinned) == 0 {
			return errors.New("no projects found; add roots in ~/.config/hopper/config.toml or pass --project")
		}

		result, err := ui.PickWithActions(projects, cfg.PickerHeight, cfg.Pinned, missingPinned)
		if err != nil {
			return err
		}
		if result.Cancelled {
			return nil
		}

		if strings.TrimSpace(result.RemovedPath) != "" {
			if err := removeEntryByPath(result.RemovedPath); err != nil {
				return err
			}
			continue
		}

		path := strings.TrimSpace(result.SelectedPath)
		if path == "" {
			return nil
		}

		idx.Projects = index.MarkOpened(idx.Projects, path)
		if err := index.Save(cachePath, idx); err != nil {
			return err
		}

		fmt.Println(path)
		return nil
	}
}

func runAdd(args []string) error {
	if len(args) < 1 {
		return errors.New("path is required")
	}
	target := config.ExpandPath(strings.Join(args, " "))
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("not a directory: %s", target)
	}

	cfg, cfgPath, err := config.Load()
	if err != nil {
		return err
	}

	cfg.Pinned = appendUniquePath(cfg.Pinned, target)
	cfg.HiddenProjects = removePath(cfg.HiddenProjects, target)
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}

	if err := rebuildIndexFromConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("added %s\n", target)
	return nil
}

func runRemove(args []string) error {
	if len(args) < 1 {
		return errors.New("path or query is required")
	}
	q := strings.TrimSpace(strings.Join(args, " "))
	if q == "" {
		return errors.New("path or query is required")
	}

	cfg, cfgPath, err := config.Load()
	if err != nil {
		return err
	}
	_, projects, _, _, err := getProjects(nil)
	if err != nil {
		return err
	}

	target, err := resolveRemoveTarget(cfg, q, projects)
	if err != nil {
		return err
	}

	cfg.Pinned = removePath(cfg.Pinned, target)
	cfg.HiddenProjects = appendUniquePath(cfg.HiddenProjects, target)
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}

	if err := rebuildIndexFromConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("removed %s\n", target)
	return nil
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	only := multiValue{}
	fs.Var(&only, "project", "project directory to include (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, projects, _, _, err := getProjects(only)
	if err != nil {
		return err
	}
	pinnedSet := makePathSet(cfg.Pinned)
	for _, p := range projects {
		_, pinned := pinnedSet[p.Path]
		fmt.Println(colorizePath(p.Path, pinned, false))
	}

	seen := makePathSet(nil)
	for _, p := range projects {
		seen[p.Path] = struct{}{}
	}
	for _, p := range missingPinnedPaths(cfg) {
		if _, ok := seen[p]; ok {
			continue
		}
		fmt.Println(colorizePath(p, true, true))
	}
	return nil
}

func runQuery(args []string) error {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	only := multiValue{}
	fs.Var(&only, "project", "project directory to include (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("query text is required")
	}
	q := strings.TrimSpace(strings.Join(fs.Args(), " "))
	cfg, projects, idx, cachePath, err := getProjects(only)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return errors.New("no projects found")
	}

	best, err := resolveQueryTarget(cfg, q, projects)
	if err != nil {
		return err
	}

	idx.Projects = index.MarkOpened(idx.Projects, best.Path)
	if err := index.Save(cachePath, idx); err != nil {
		return err
	}

	fmt.Println(best.Path)
	return nil
}

func runRecent(args []string) error {
	fs := flag.NewFlagSet("recent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	limit := fs.Int("n", 20, "number of results")
	if err := fs.Parse(args); err != nil {
		return err
	}

	idx, cachePath, err := index.Load()
	if err != nil {
		return err
	}
	pruned, changed := index.PruneMissing(idx.Projects)
	if changed {
		idx.Projects = pruned
		if err := index.Save(cachePath, idx); err != nil {
			return err
		}
	}
	projects := append([]model.Project(nil), idx.Projects...)
	index.SortByRecency(projects)
	if *limit > 0 && *limit < len(projects) {
		projects = projects[:*limit]
	}
	for _, p := range projects {
		if p.LastOpened.IsZero() {
			continue
		}
		fmt.Printf("%s\t%s\n", p.LastOpened.Format("2006-01-02 15:04"), p.Path)
	}
	return nil
}

func runIndex(args []string) error {
	if len(args) == 0 || args[0] == "rebuild" {
		cfg, err := mustConfig()
		if err != nil {
			return err
		}
		if err := rebuildIndexFromConfig(cfg); err != nil {
			return err
		}
		idx, _, err := index.Load()
		if err != nil {
			return err
		}
		fmt.Printf("indexed %d projects\n", len(idx.Projects))
		return nil
	}
	return fmt.Errorf("unknown index command: %s", args[0])
}

func runInit(args []string) error {
	if len(args) == 0 {
		return errors.New("shell required (zsh|bash)")
	}
	switch args[0] {
	case "zsh":
		fmt.Println(zshInitScript())
		return nil
	case "bash":
		fmt.Println(bashInitScript())
		return nil
	default:
		return fmt.Errorf("unsupported shell: %s", args[0])
	}
}

func getProjects(only []string) (config.Config, []model.Project, index.File, string, error) {
	cfg, err := mustConfig()
	if err != nil {
		return config.Config{}, nil, index.File{}, "", err
	}
	idx, cachePath, err := index.Load()
	if err != nil {
		return cfg, nil, index.File{}, "", err
	}
	pruned, changed := index.PruneMissing(idx.Projects)
	if changed {
		idx.Projects = pruned
		if err := index.Save(cachePath, idx); err != nil {
			return cfg, nil, idx, cachePath, err
		}
	}

	if len(only) > 0 {
		d, err := discovery.Discover(cfg, only)
		if err != nil {
			return cfg, nil, idx, cachePath, err
		}
		index.SortByRecency(d)
		return cfg, d, idx, cachePath, nil
	}

	if len(idx.Projects) == 0 || index.IsStale(idx.Updated, cfg.StaleAfterMins) {
		discovered, err := discovery.Discover(cfg, nil)
		if err != nil {
			return cfg, nil, idx, cachePath, err
		}
		idx.Projects = index.MergeDiscovered(idx.Projects, discovered)
		if err := index.Save(cachePath, idx); err != nil {
			return cfg, nil, idx, cachePath, err
		}
	}

	projects := append([]model.Project(nil), idx.Projects...)
	index.SortByRecency(projects)
	return cfg, projects, idx, cachePath, nil
}

func bestMatchDetailed(query string, projects []model.Project) (model.Project, []model.Project, bool) {
	q := strings.ToLower(query)
	targets := make([]string, 0, len(projects))
	for _, p := range projects {
		targets = append(targets, strings.ToLower(p.Name+" "+p.Path))
	}
	matches := fuzzy.Find(q, targets)
	if len(matches) > 0 {
		candidates := make([]model.Project, 0, minInt(8, len(matches)))
		for i := 0; i < len(matches) && i < 8; i++ {
			candidates = append(candidates, projects[matches[i].Index])
		}
		return projects[matches[0].Index], candidates, true
	}
	for _, p := range projects {
		if strings.Contains(strings.ToLower(p.Path), q) || strings.Contains(strings.ToLower(p.Name), q) {
			return p, []model.Project{p}, true
		}
	}
	return model.Project{}, nil, false
}

func bestMatch(query string, projects []model.Project) (model.Project, []model.Project, bool) {
	return bestMatchDetailed(query, projects)
}

func mustConfig() (config.Config, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func printHelp() {
	fmt.Print(`hopper - jump between projects fast

Usage:
  hopper [pick] [--project PATH ...]
  hopper add <path>
  hopper remove <path-or-query>
  hopper list [--project PATH ...]
  hopper query <text> [--project PATH ...]
  hopper recent [-n N]
  hopper index rebuild
  hopper init zsh
  hopper init bash
`)
}

func zshInitScript() string {
	return `# hopper zsh integration
_h_cd_pick() {
  local target
  if (( $# > 0 )); then
    target="$(command hopper query "$*" </dev/tty 2>/dev/tty)" || return
  else
    target="$(command hopper pick </dev/tty 2>/dev/tty)" || return
  fi
  target="${target//$'\r'/}"
  target="${target##*$'\n'}"
  [[ "$target" == /* ]] || return
  [[ -n "$target" ]] && cd "$target"
}

h() {
  case "$1" in
    add|remove|list|query|recent|index|init|pick|help|-h|--help)
      command hopper "$@"
      ;;
    "")
      _h_cd_pick
      ;;
    *)
      # If args do not match subcommands, treat them as pick filters.
      _h_cd_pick "$@"
      ;;
  esac
}

ha() {
  command hopper add "$@"
}

hr() {
  command hopper remove "$@"
}

h_widget() {
  local target
  target="$(command hopper pick </dev/tty 2>/dev/tty)" || return
  target="${target//$'\r'/}"
  target="${target##*$'\n'}"
  [[ "$target" == /* ]] || return
  zle reset-prompt
  [[ -n "$target" ]] || return
  BUFFER="cd ${(q)target}"
  zle accept-line
}
zle -N h_widget
bindkey '^G' h_widget
`
}

func bashInitScript() string {
	return `# hopper bash integration
_h_cd_pick() {
  local target
  if [[ $# -gt 0 ]]; then
    target="$(command hopper query "$*" </dev/tty 2>/dev/tty)" || return
  else
    target="$(command hopper pick </dev/tty 2>/dev/tty)" || return
  fi
  target="${target//$'\r'/}"
  target="${target##*$'\n'}"
  [[ "$target" == /* ]] || return
  [[ -n "$target" ]] && cd "$target"
}

h() {
  case "$1" in
    add|remove|list|query|recent|index|init|pick|help|-h|--help)
      command hopper "$@"
      ;;
    "")
      _h_cd_pick
      ;;
    *)
      _h_cd_pick "$@"
      ;;
  esac
}

ha() {
  command hopper add "$@"
}

hr() {
  command hopper remove "$@"
}
`
}

type multiValue []string

func (m *multiValue) String() string {
	if m == nil {
		return ""
	}
	return strings.Join(*m, ",")
}

func (m *multiValue) Set(value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	if strings.HasPrefix(v, ".") {
		abs, err := filepath.Abs(v)
		if err == nil {
			v = abs
		}
	}
	*m = append(*m, v)
	return nil
}

func appendUniquePath(paths []string, path string) []string {
	for _, p := range paths {
		if p == path {
			return paths
		}
	}
	return append(paths, path)
}

func removePath(paths []string, target string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == target {
			continue
		}
		out = append(out, p)
	}
	return out
}

func rebuildIndexFromConfig(cfg config.Config) error {
	discovered, err := discovery.Discover(cfg, nil)
	if err != nil {
		return err
	}
	idx, cachePath, err := index.Load()
	if err != nil {
		return err
	}
	pruned, changed := index.PruneMissing(idx.Projects)
	if changed {
		idx.Projects = pruned
	}
	idx.Projects = index.MergeDiscovered(idx.Projects, discovered)
	hidden := map[string]struct{}{}
	for _, p := range cfg.HiddenProjects {
		hidden[p] = struct{}{}
	}
	filtered := idx.Projects[:0]
	for _, p := range idx.Projects {
		if _, blocked := hidden[p.Path]; blocked {
			continue
		}
		filtered = append(filtered, p)
	}
	idx.Projects = filtered
	return index.Save(cachePath, idx)
}

func missingPinnedPaths(cfg config.Config) []string {
	out := make([]string, 0, len(cfg.Pinned))
	for _, p := range cfg.Pinned {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func makePathSet(paths []string) map[string]struct{} {
	out := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		out[p] = struct{}{}
	}
	return out
}

func colorizePath(path string, pinned bool, missing bool) string {
	if missing {
		return missingPathStyle.Render(path + " (missing)")
	}
	if pinned {
		return pinnedPathStyle.Render(path)
	}
	return path
}

func removeEntryByPath(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	cfg, cfgPath, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Pinned = removePath(cfg.Pinned, target)
	cfg.HiddenProjects = appendUniquePath(cfg.HiddenProjects, target)
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	return rebuildIndexFromConfig(cfg)
}

func resolveRemoveTarget(cfg config.Config, input string, projects []model.Project) (string, error) {
	asPath := config.ExpandPath(input)
	if info, err := os.Stat(asPath); err == nil && info.IsDir() {
		return asPath, nil
	}

	best, candidates, ok := bestMatchDetailed(input, projects)
	if !ok {
		return "", errors.New("no match found")
	}

	if len(candidates) == 1 {
		return candidates[0].Path, nil
	}

	targets := make([]string, 0, len(projects))
	for _, p := range projects {
		targets = append(targets, strings.ToLower(p.Name+" "+p.Path))
	}
	matches := fuzzy.Find(strings.ToLower(input), targets)
	if len(matches) < 2 {
		return best.Path, nil
	}
	if !isConfident(matches, cfg) {
		picked, err := ui.Pick(candidates, cfg.PickerHeight, cfg.Pinned, missingPinnedPaths(cfg))
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(picked) == "" {
			return "", errors.New("remove cancelled")
		}
		return picked, nil
	}

	return best.Path, nil
}

func isConfident(matches fuzzy.Matches, cfg config.Config) bool {
	if len(matches) == 0 {
		return false
	}
	if matches[0].Score < cfg.FuzzyConfidenceMinScore {
		return false
	}
	if len(matches) == 1 {
		return true
	}
	delta := matches[0].Score - matches[1].Score
	return delta >= cfg.FuzzyConfidenceMinDelta
}

func resolveQueryTarget(cfg config.Config, query string, projects []model.Project) (model.Project, error) {
	best, candidates, ok := bestMatchDetailed(query, projects)
	if !ok {
		return model.Project{}, errors.New("no match found")
	}

	targets := make([]string, 0, len(projects))
	for _, p := range projects {
		targets = append(targets, p.Name+" "+p.Path)
	}
	matches := fuzzy.Find(query, targets)
	if len(matches) < 2 || isConfident(matches, cfg) {
		return best, nil
	}

	if !isInteractiveSession() {
		return best, nil
	}

	picked, err := ui.Pick(candidates, cfg.PickerHeight, cfg.Pinned, missingPinnedPaths(cfg))
	if err != nil {
		return model.Project{}, err
	}
	if strings.TrimSpace(picked) == "" {
		return model.Project{}, errors.New("query cancelled")
	}
	for _, p := range projects {
		if p.Path == picked {
			return p, nil
		}
	}
	return model.Project{}, errors.New("selected project not found")
}

func isInteractiveSession() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	stdinInfo, err := os.Stdin.Stat()
	if err != nil || (stdinInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	stderrInfo, err := os.Stderr.Stat()
	if err != nil || (stderrInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	return true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
