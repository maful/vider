package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------- styling ----------

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Padding(0, 1)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	checkedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Padding(0, 1)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	sizeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	pathStyle     = lipgloss.NewStyle()
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Padding(0, 1)
)

// ---------- messages ----------

type deletedMsg []delResult

type delResult struct {
	path string
	err  error
}

type tickMsg struct{}

// ---------- model ----------

// mode says what the keyboard does right now. Every key is routed through it
// in handleKey, so a key can only act in the modes that list it.
//
// Scanning is not a mode: it runs alongside all three and is tracked by
// scanDone.
//
//	browse  --d/enter (something selected)--> confirm
//	confirm --y--> delete      confirm --n/esc--> browse
//	delete  --deletedMsg--> browse
type mode int

const (
	modeBrowse  mode = iota // move, select, rescan, open the prompt
	modeConfirm             // y/n prompt about model.pending
	modeDelete              // deleteDirs is running on model.pending
)

type model struct {
	scanner     *scanner
	spinner     spinner.Model
	cursor      int
	offset      int
	dirs        []*NodeModules
	selected    map[string]bool
	scanDone    bool
	mode        mode
	pending     []string // set on entering modeConfirm, cleared on returning to modeBrowse
	err         error
	roots       []string
	rootsLabel  string
	width       int
	height      int
	startTime   time.Time
	scanElapsed time.Duration
}

func initialModel(roots []string, rootsLabel string) model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	sc := newScanner(roots)
	return model{
		scanner:    sc,
		spinner:    s,
		selected:   map[string]bool{},
		roots:      roots,
		rootsLabel: rootsLabel,
		startTime:  time.Now(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.scanner.scan(), m.spinner.Tick, tickEvery())
}

func tickEvery() tea.Cmd {
	return tea.Every(time.Millisecond*100, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.handleKey(msg.String())
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	case tickMsg:
		m.refreshDirs()
		cmds = append(cmds, tickEvery())
	case scanDoneMsg:
		// a rescan replaces the scanner; the old one's message can still be
		// in flight, and it says nothing about the scan on screen
		if msg.scanner == m.scanner {
			m.scanDone = true
			m.scanElapsed = time.Since(m.startTime)
			m.refreshDirs()
		}
	case deletedMsg:
		var failed []delResult
		for _, r := range []delResult(msg) {
			delete(m.selected, r.path)
			if r.err == nil {
				// drop it from the scanner too, or the next tick's
				// refreshDirs puts the row back
				m.scanner.remove(r.path)
			} else {
				failed = append(failed, r)
			}
		}
		m.err = deleteError(failed)
		m.pending = nil
		m.mode = modeBrowse
		m.cursor = 0
		m.refreshDirs()
	}
	return m, tea.Batch(cmds...)
}

// handleKey routes a key to the current mode. Quit works in every mode.
func (m model) handleKey(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" || key == "q" {
		return m, tea.Quit
	}
	switch m.mode {
	case modeConfirm:
		return m.confirmKey(key)
	case modeDelete:
		// the rows in m.pending are going away: look, but do not touch
		m.moveKey(key)
		return m, nil
	default:
		return m.browseKey(key)
	}
}

// moveKey handles the cursor keys and reports whether key was one of them.
func (m *model) moveKey(key string) bool {
	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.dirs)-1 {
			m.cursor++
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		if len(m.dirs) > 0 {
			m.cursor = len(m.dirs) - 1
		}
	default:
		return false
	}
	m.clampScroll()
	return true
}

func (m model) browseKey(key string) (tea.Model, tea.Cmd) {
	if m.moveKey(key) {
		return m, nil
	}
	switch key {
	case " ", "space":
		if m.cursor < len(m.dirs) {
			p := m.dirs[m.cursor].Path
			m.selected[p] = !m.selected[p]
		}
	case "a":
		all := len(m.dirs) > 0
		for _, d := range m.dirs {
			if !m.selected[d.Path] {
				all = false
				break
			}
		}
		for _, d := range m.dirs {
			m.selected[d.Path] = !all
		}
	case "r":
		m = m.rescan()
		return m, tea.Batch(m.scanner.scan(), m.spinner.Tick)
	case "d", "enter":
		if paths := m.selectedPaths(); len(paths) > 0 {
			m.pending = paths
			m.mode = modeConfirm
		}
	}
	return m, nil
}

// confirmKey answers the prompt. Every other key is ignored, so the set of
// paths the prompt showed is exactly the set that y deletes.
func (m model) confirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		m.mode = modeDelete
		m.err = nil // a new attempt replaces the last attempt's error
		return m, deleteDirs(m.pending)
	case "n", "esc":
		m.pending = nil
		m.mode = modeBrowse
	}
	return m, nil
}

// selectedPaths returns the selected rows in list order.
func (m model) selectedPaths() []string {
	var paths []string
	for _, d := range m.dirs {
		if m.selected[d.Path] {
			paths = append(paths, d.Path)
		}
	}
	return paths
}

// sizeOf sums the current sizes of the given paths.
func (m model) sizeOf(paths []string) int64 {
	want := make(map[string]bool, len(paths))
	for _, p := range paths {
		want[p] = true
	}
	var total int64
	for _, d := range m.dirs {
		if want[d.Path] {
			total += d.Size
		}
	}
	return total
}

// deleteError folds the failed deletes into one line for the error banner.
func deleteError(failed []delResult) error {
	switch len(failed) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("%s: %v", failed[0].path, failed[0].err)
	default:
		return fmt.Errorf("%d directories were not deleted, first: %s: %v",
			len(failed), failed[0].path, failed[0].err)
	}
}

func (m model) rescan() model {
	m.scanner.cancel() // or every r adds another full scan of the same roots
	nm := initialModel(m.roots, m.rootsLabel)
	nm.width, nm.height = m.width, m.height
	return nm
}

func (m *model) refreshDirs() {
	m.dirs = m.scanner.sortedDirs()
	if m.cursor >= len(m.dirs) {
		m.cursor = len(m.dirs) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.clampScroll()
}

const rowHeight = 5 // per-dir block height in the UI

func (m *model) clampScroll() {
	// View indexes m.dirs from offset, so neither value may go negative,
	// whatever the caller did to the cursor.
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if m.height == 0 {
		return
	}
	visible := (m.height - 8) / rowHeight
	if visible < 1 {
		visible = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

// ---------- view ----------

func shortenPath(p string, home string, max int) string {
	if home != "" && strings.HasPrefix(p, home) {
		p = "~" + strings.TrimPrefix(p, home)
	}
	if len(p) > max {
		p = "…" + p[len(p)-max+1:]
	}
	return p
}

func mb(size int64) string {
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func (m model) View() string {
	var b strings.Builder

	totalSel := int64(0)
	nSel := 0
	for _, d := range m.dirs {
		if m.selected[d.Path] {
			totalSel += d.Size
			nSel++
		}
	}

	b.WriteString(titleStyle.Render("🗑  vider — node_modules killer"))
	b.WriteString("\n")

	if !m.scanDone {
		b.WriteString(statusStyle.Render(fmt.Sprintf("%s scanning %s · found %d, sized %d",
			m.spinner.View(), m.rootsLabel, m.scanner.found.Load(), m.scanner.sized.Load())))
	} else {
		elapsed := m.scanElapsed.Round(time.Millisecond)
		b.WriteString(statusStyle.Render(fmt.Sprintf("scanned %s in %s — %d found, %d selected (%s reclaimable)",
			m.rootsLabel, elapsed, len(m.dirs), nSel, mb(totalSel))))
	}
	b.WriteString("\n\n")

	if len(m.dirs) == 0 {
		if m.scanDone {
			b.WriteString(statusStyle.Render("No node_modules directories found. 🎉\n"))
		}
		return b.String()
	}

	// header
	b.WriteString(headerStyle.Render(fmt.Sprintf("   %-6s %-9s %-17s %s", "SEL", "SIZE", "LAST MODIFIED", "DIRECTORY")))
	b.WriteString("\n")

	home, _ := os.UserHomeDir()
	visible := (m.height - 8) / rowHeight
	if m.height == 0 || visible < 5 {
		visible = 5
	}
	end := m.offset + visible
	if end > len(m.dirs) {
		end = len(m.dirs)
	}

	for i := m.offset; i < end; i++ {
		d := m.dirs[i]
		cursor := "  "
		if i == m.cursor && m.mode != modeConfirm {
			cursor = selectedStyle.Render("→ ")
		}
		check := "[ ]"
		if m.selected[d.Path] {
			check = checkedStyle.Render("[x]")
		}
		rowStyle := lipgloss.NewStyle()
		if i == m.cursor && m.mode != modeConfirm {
			rowStyle = rowStyle.Background(lipgloss.Color("236"))
		}
		line := rowStyle.Render(fmt.Sprintf("%s%-6s %s %-17s %s",
			cursor, check,
			sizeStyle.Render(mb(d.Size)),
			d.ModTime.Format("2006-01-02 15:04"),
			pathStyle.Render(shortenPath(d.Path, home, 70))))
		b.WriteString(line)
		b.WriteString("\n\n")
	}

	// footer
	total := int64(0)
	for _, d := range m.dirs {
		total += d.Size
	}
	footer := helpStyle.Render(fmt.Sprintf("%d/%d  total %s", m.cursor+1, len(m.dirs), mb(total)))
	b.WriteString("\n")
	b.WriteString(footer)
	b.WriteString("\n")

	switch m.mode {
	case modeConfirm:
		b.WriteString(errStyle.Render(fmt.Sprintf("\n⚠  Delete %d director%s (%s)? This cannot be undone. [y]es / [n]o",
			len(m.pending), plural(len(m.pending)), mb(m.sizeOf(m.pending)))))
	case modeDelete:
		b.WriteString(statusStyle.Render(fmt.Sprintf("\n%s deleting %d director%s…",
			m.spinner.View(), len(m.pending), plural(len(m.pending)))))
	}
	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(errStyle.Render("error: " + m.err.Error()))
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(m.helpText()))
	return b.String()
}

// helpText lists only the keys that act in the current mode.
func (m model) helpText() string {
	switch m.mode {
	case modeConfirm:
		return "y delete · n cancel · q quit"
	case modeDelete:
		return "↑/↓ move · q quit"
	default:
		return "↑/↓ move · space select · a toggle all · d delete selected · r rescan · q quit"
	}
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// deleteDirs removes the selected paths concurrently (bounded) and reports
// each result in one message.
func deleteDirs(paths []string) tea.Cmd {
	return func() tea.Msg {
		results := make([]delResult, len(paths))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		for i, p := range paths {
			wg.Add(1)
			go func(i int, p string) {
				defer wg.Done()
				sem <- struct{}{}
				results[i] = delResult{p, os.RemoveAll(p)}
				<-sem
			}(i, p)
		}
		wg.Wait()
		return deletedMsg(results)
	}
}
