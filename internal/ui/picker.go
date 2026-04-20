package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AugustDG/hopper/internal/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

var (
	pickerRenderer = lipgloss.NewRenderer(os.Stderr)
	pinnedStyle    = pickerRenderer.NewStyle().Foreground(lipgloss.Color("214"))
	missingStyle   = pickerRenderer.NewStyle().Foreground(lipgloss.Color("196"))
	selectedStyle  = pickerRenderer.NewStyle().Reverse(true)
)

type PickResult struct {
	SelectedPath string
	RemovedPath  string
	Cancelled    bool
}

func Pick(projects []model.Project, size int, pinned []string, missingPinned []string) (string, error) {
	res, err := pick(projects, size, pinned, missingPinned, false)
	if err != nil {
		return "", err
	}
	if res.Cancelled {
		return "", nil
	}
	return res.SelectedPath, nil
}

func PickWithActions(projects []model.Project, size int, pinned []string, missingPinned []string) (PickResult, error) {
	return pick(projects, size, pinned, missingPinned, true)
}

func pick(projects []model.Project, size int, pinned []string, missingPinned []string, allowRemove bool) (PickResult, error) {
	if len(projects) == 0 {
		if len(missingPinned) == 0 {
			return PickResult{}, fmt.Errorf("no projects available")
		}
	}
	if size <= 0 {
		size = 12
	}
	pinnedSet := makePathSet(pinned)
	missingSet := makePathSet(missingPinned)
	seen := make(map[string]struct{}, len(projects)+len(missingPinned))

	items := make([]entry, 0, len(projects))
	for _, p := range projects {
		items = append(items, newEntry(p, pinnedSet, missingSet))
		seen[p.Path] = struct{}{}
	}
	for _, p := range missingPinned {
		if _, ok := seen[p]; ok {
			continue
		}
		items = append(items, newEntry(model.Project{Name: filepath.Base(p), Path: p}, pinnedSet, missingSet))
	}

	m := newPickerModel(items, size, allowRemove)
	p := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(os.Stderr))
	finalModel, err := p.Run()
	if err != nil {
		return PickResult{}, err
	}
	out := finalModel.(pickerModel)
	if out.cancelled || (out.selectedPath == "" && out.removedPath == "") {
		return PickResult{Cancelled: true}, nil
	}
	return PickResult{SelectedPath: out.selectedPath, RemovedPath: out.removedPath}, nil
}

type entry struct {
	Project model.Project
	label   string
	search  string
	pinned  bool
	missing bool
}

type pickerModel struct {
	all          []entry
	filtered     []entry
	query        string
	cursor       int
	height       int
	cancelled    bool
	selectedPath string
	removedPath  string
	pendingG     bool
	allowRemove  bool
	confirming   bool
	confirmItem  entry
}

func newPickerModel(items []entry, height int, allowRemove bool) pickerModel {
	m := pickerModel{
		all:         append([]entry(nil), items...),
		filtered:    append([]entry(nil), items...),
		height:      height,
		allowRemove: allowRemove,
	}
	return m
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.confirming {
			switch msg.Type {
			case tea.KeyEnter:
				m.removedPath = m.confirmItem.Project.Path
				m.confirming = false
				return m, tea.Quit
			case tea.KeyEsc:
				m.confirming = false
				return m, nil
			default:
				k := strings.ToLower(msg.String())
				switch k {
				case "y":
					m.removedPath = m.confirmItem.Project.Path
					m.confirming = false
					return m, tea.Quit
				case "n", "q":
					m.confirming = false
					return m, nil
				}
				return m, nil
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEnter:
			return m.selectCurrent()
		case tea.KeyUp:
			m.pendingG = false
			m.moveCursor(-1)
			return m, nil
		case tea.KeyDown:
			m.pendingG = false
			m.moveCursor(1)
			return m, nil
		case tea.KeyCtrlU:
			m.pendingG = false
			m.moveCursor(-m.pageStep())
			return m, nil
		case tea.KeyCtrlD:
			m.pendingG = false
			m.moveCursor(m.pageStep())
			return m, nil
		case tea.KeyDelete:
			m.pendingG = false
			if m.allowRemove && strings.TrimSpace(m.query) == "" {
				return m.requestRemove()
			}
			if len(m.query) > 0 {
				m.query = m.query[:len(m.query)-1]
				m.applyFilter()
			}
			return m, nil
		case tea.KeyBackspace:
			m.pendingG = false
			if len(m.query) > 0 {
				m.query = m.query[:len(m.query)-1]
				m.applyFilter()
			}
			return m, nil
		default:
			if m.handleVimKey(msg.String()) {
				if m.cancelled || m.selectedPath != "" || m.removedPath != "" {
					return m, tea.Quit
				}
				return m, nil
			}
			m.pendingG = false
			r := msg.Runes
			if len(r) > 0 {
				m.query += string(r)
				m.applyFilter()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString("hopper > ")
	b.WriteString(m.query)
	b.WriteString("\n")

	if len(m.filtered) == 0 {
		b.WriteString("  no matches\n")
		b.WriteString("  enter/l=select  esc/q=cancel  arrows/jk=move  ctrl-u/d=page")
		if m.allowRemove {
			b.WriteString("  d/Delete=remove")
		}
		return b.String()
	}

	limit := m.height
	if limit <= 0 {
		limit = 12
	}
	if limit > len(m.filtered) {
		limit = len(m.filtered)
	}

	start := 0
	if m.cursor >= limit {
		start = m.cursor - limit + 1
	}
	end := start + limit
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := start; i < end; i++ {
		prefix := "  "
		line := styledLabel(m.filtered[i])
		if i == m.cursor {
			prefix = "> "
			line = selectedStyle.Render(line)
		}
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("  enter/l=select  esc/q=cancel  arrows/jk=move  gg/G=top/bottom")
	if m.allowRemove {
		b.WriteString("  d/Delete=remove")
	}
	b.WriteString("\n")
	b.WriteString("  colors: pinned=gold, missing pinned=red")
	if m.confirming {
		b.WriteString("\n")
		b.WriteString("  remove ")
		b.WriteString(m.confirmItem.Project.Path)
		b.WriteString(" ? [y/N]")
	}
	return b.String()
}

func (m *pickerModel) handleVimKey(key string) bool {
	if m.query != "" {
		return false
	}
	switch key {
	case "j":
		m.pendingG = false
		m.moveCursor(1)
		return true
	case "k":
		m.pendingG = false
		m.moveCursor(-1)
		return true
	case "q":
		m.pendingG = false
		m.cancelled = true
		return true
	case "l":
		m.pendingG = false
		if p := m.selectedCandidatePath(); p != "" {
			m.selectedPath = p
		}
		return true
	case "d":
		if !m.allowRemove || strings.TrimSpace(m.query) != "" {
			return false
		}
		m.pendingG = false
		_, _ = m.requestRemove()
		return true
	case "g":
		if m.pendingG {
			m.cursor = 0
			m.pendingG = false
			return true
		}
		m.pendingG = true
		return true
	case "G":
		m.pendingG = false
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
		return true
	}
	return false
}

func (m *pickerModel) selectCurrent() (tea.Model, tea.Cmd) {
	path := m.selectedCandidatePath()
	if path == "" {
		return *m, nil
	}
	m.selectedPath = path
	return *m, tea.Quit
}

func (m *pickerModel) selectedCandidatePath() string {
	if len(m.filtered) == 0 {
		return ""
	}
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
	if !m.filtered[m.cursor].missing {
		return m.filtered[m.cursor].Project.Path
	}
	for _, item := range m.filtered {
		if !item.missing {
			return item.Project.Path
		}
	}
	return ""
}

func (m *pickerModel) requestRemove() (tea.Model, tea.Cmd) {
	if !m.allowRemove || len(m.filtered) == 0 {
		return *m, nil
	}
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
	item := m.filtered[m.cursor]
	if strings.TrimSpace(item.Project.Path) == "" {
		return *m, nil
	}
	if item.missing {
		m.removedPath = item.Project.Path
		return *m, tea.Quit
	}
	m.confirming = true
	m.confirmItem = item
	return *m, nil
}

func (m *pickerModel) currentPath() string {
	if len(m.filtered) == 0 {
		return ""
	}
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
	return m.filtered[m.cursor].Project.Path
}

func (m *pickerModel) moveCursor(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	n := len(m.filtered)
	m.cursor = (m.cursor + delta) % n
	if m.cursor < 0 {
		m.cursor += n
	}
}

func (m *pickerModel) pageStep() int {
	if m.height <= 1 {
		return 5
	}
	return maxInt(1, m.height-2)
}

func (m *pickerModel) applyFilter() {
	q := strings.TrimSpace(m.query)
	if q == "" {
		m.filtered = append([]entry(nil), m.all...)
		m.cursor = 0
		return
	}

	qLower := strings.ToLower(q)
	targets := make([]string, 0, len(m.all))
	for _, item := range m.all {
		targets = append(targets, item.search)
	}
	matches := fuzzy.Find(qLower, targets)
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].Score > matches[j].Score
		})
		filtered := make([]entry, 0, len(matches))
		for _, match := range matches {
			filtered = append(filtered, m.all[match.Index])
		}
		m.filtered = filtered
	} else {
		type fallback struct {
			entry entry
			pos   int
		}
		fbs := make([]fallback, 0, len(m.all))
		for _, item := range m.all {
			if pos := strings.Index(item.search, qLower); pos >= 0 {
				fbs = append(fbs, fallback{item, pos})
			}
		}
		sort.SliceStable(fbs, func(i, j int) bool {
			if fbs[i].pos != fbs[j].pos {
				return fbs[i].pos < fbs[j].pos
			}
			return len(fbs[i].entry.search) < len(fbs[j].entry.search)
		})
		filtered := make([]entry, 0, len(fbs))
		for _, f := range fbs {
			filtered = append(filtered, f.entry)
		}
		m.filtered = filtered
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func makePathSet(paths []string) map[string]struct{} {
	out := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		out[p] = struct{}{}
	}
	return out
}

func newEntry(p model.Project, pinnedSet map[string]struct{}, missingSet map[string]struct{}) entry {
	_, pinned := pinnedSet[p.Path]
	_, missing := missingSet[p.Path]
	label := fmt.Sprintf("%s  -  %s", p.Name, p.Path)
	return entry{
		Project: p,
		label:   label,
		search:  strings.ToLower(p.Name + " " + p.Path),
		pinned:  pinned,
		missing: missing,
	}
}

func styledLabel(item entry) string {
	if item.missing {
		return missingStyle.Render(item.label + " (missing)")
	}
	if item.pinned {
		return pinnedStyle.Render(item.label)
	}
	return item.label
}
