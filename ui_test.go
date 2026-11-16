package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func update(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(model)
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// Jumping to the end of an empty list used to leave cursor and offset at -1,
// and View panicked on m.dirs[-1] as soon as the first result arrived.
func TestJumpToEndOnEmptyList(t *testing.T) {
	keys := map[string]tea.Msg{
		"G":   runeKey('G'),
		"end": tea.KeyMsg{Type: tea.KeyEnd},
	}
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			m := initialModel(nil, "")
			m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

			m = update(t, m, key)
			if m.cursor != 0 || m.offset != 0 {
				t.Fatalf("empty list: cursor=%d offset=%d, want 0 and 0", m.cursor, m.offset)
			}

			// the first result arrives on the next tick
			path := "/tmp/project/node_modules"
			m.scanner.dirs[path] = &NodeModules{Path: path}
			m = update(t, m, tickMsg{})
			if m.cursor != 0 || m.offset != 0 {
				t.Fatalf("first result: cursor=%d offset=%d, want 0 and 0", m.cursor, m.offset)
			}
			_ = m.View() // must not panic
		})
	}
}

func TestJumpToEndMovesToLastRow(t *testing.T) {
	m := initialModel(nil, "")
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	for _, p := range []string{"/a/node_modules", "/b/node_modules", "/c/node_modules"} {
		m.scanner.dirs[p] = &NodeModules{Path: p}
	}
	m = update(t, m, tickMsg{})

	m = update(t, m, runeKey('G'))
	if m.cursor != 2 {
		t.Fatalf("cursor=%d, want 2", m.cursor)
	}
	_ = m.View()
}

// Deleted rows used to come back on the next tick, because only the model's
// list was filtered and the scanner still held the paths.
func TestDeletedRowsStayGone(t *testing.T) {
	gone1, gone2, failed := "/a/node_modules", "/b/node_modules", "/c/node_modules"
	m := initialModel(nil, "")
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	for i, p := range []string{gone1, gone2, failed} {
		m.scanner.dirs[p] = &NodeModules{Path: p, Size: int64(i + 1)}
	}
	m = update(t, m, tickMsg{})
	m = update(t, m, runeKey('a'))
	m = update(t, m, runeKey('d'))
	m = update(t, m, runeKey('y'))

	m = update(t, m, deletedMsg{
		{path: gone1},
		{path: gone2},
		{path: failed, err: errors.New("permission denied")},
	})

	check := func(when string) {
		t.Helper()
		if len(m.dirs) != 1 || m.dirs[0].Path != failed {
			t.Fatalf("%s: rows=%v, want only %s", when, paths(m.dirs), failed)
		}
	}
	check("after delete")
	m = update(t, m, tickMsg{})
	check("after next tick")

	if len(m.selected) != 0 {
		t.Errorf("selected=%v, want empty", m.selected)
	}
	if m.err == nil {
		t.Error("failed delete did not set err")
	}
	if m.mode != modeBrowse || m.pending != nil {
		t.Errorf("mode=%d pending=%v, want browse mode and no pending paths", m.mode, m.pending)
	}
	_ = m.View()
}

func paths(dirs []*NodeModules) []string {
	out := make([]string, len(dirs))
	for i, d := range dirs {
		out[i] = d.Path
	}
	return out
}

// ---------- mode machine ----------

func step(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(model), cmd
}

// listModel returns a model in browse mode showing three sized rows, largest
// first: /c, /b, /a.
func listModel(t *testing.T) model {
	t.Helper()
	m := initialModel(nil, "")
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	for i, p := range []string{"/a/node_modules", "/b/node_modules", "/c/node_modules"} {
		m.scanner.dirs[p] = &NodeModules{Path: p, Size: int64(i+1) << 20}
	}
	return update(t, m, tickMsg{})
}

func selectedCount(m model) int {
	n := 0
	for _, on := range m.selected {
		if on {
			n++
		}
	}
	return n
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestDeleteKeyNeedsSelection(t *testing.T) {
	m := listModel(t)
	for _, key := range []tea.Msg{runeKey('d'), tea.KeyMsg{Type: tea.KeyEnter}, runeKey('y')} {
		var cmd tea.Cmd
		m, cmd = step(t, m, key)
		if m.mode != modeBrowse || cmd != nil {
			t.Fatalf("key %v with nothing selected: mode=%d cmd=%v, want browse and no command", key, m.mode, cmd != nil)
		}
	}
}

// While the prompt is up, only y, n and esc act. The old handler let space
// and a change the selection behind the prompt.
func TestConfirmIgnoresOtherKeys(t *testing.T) {
	m := listModel(t)
	m = update(t, m, tea.KeyMsg{Type: tea.KeySpace}) // selects /c
	m = update(t, m, runeKey('d'))
	if m.mode != modeConfirm || len(m.pending) != 1 || m.pending[0] != "/c/node_modules" {
		t.Fatalf("after d: mode=%d pending=%v", m.mode, m.pending)
	}
	if !strings.Contains(m.View(), "Delete 1 directory") {
		t.Errorf("prompt missing from view:\n%s", m.View())
	}

	sc, cursor := m.scanner, m.cursor
	ignored := []tea.Msg{
		tea.KeyMsg{Type: tea.KeySpace}, runeKey('a'), runeKey('d'), runeKey('r'),
		runeKey('j'), runeKey('G'), tea.KeyMsg{Type: tea.KeyEnter},
	}
	for _, key := range ignored {
		var cmd tea.Cmd
		m, cmd = step(t, m, key)
		if cmd != nil {
			t.Errorf("key %v returned a command while confirming", key)
		}
	}
	if m.mode != modeConfirm || selectedCount(m) != 1 || len(m.pending) != 1 {
		t.Errorf("mode=%d selected=%d pending=%v, want confirm, 1, 1 path", m.mode, selectedCount(m), m.pending)
	}
	if m.scanner != sc || m.cursor != cursor {
		t.Errorf("r or a cursor key acted while confirming")
	}
}

func TestConfirmCancel(t *testing.T) {
	for name, key := range map[string]tea.Msg{"n": runeKey('n'), "esc": tea.KeyMsg{Type: tea.KeyEsc}} {
		t.Run(name, func(t *testing.T) {
			m := listModel(t)
			m = update(t, m, runeKey('a'))
			m = update(t, m, runeKey('d'))
			m, cmd := step(t, m, key)
			if m.mode != modeBrowse || m.pending != nil || cmd != nil {
				t.Fatalf("mode=%d pending=%v cmd=%v, want browse, nil, no command", m.mode, m.pending, cmd != nil)
			}
			if selectedCount(m) != 3 {
				t.Errorf("selected=%d, want the selection kept (3)", selectedCount(m))
			}
		})
	}
}

// One delete at a time: the old handler started a second deleteDirs on the
// same paths when d and y were pressed during a delete.
func TestDeleteModeAllowsOnlyMoving(t *testing.T) {
	m := listModel(t)
	m = update(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = update(t, m, runeKey('d'))
	m, cmd := step(t, m, runeKey('y'))
	if m.mode != modeDelete || cmd == nil || len(m.pending) != 1 {
		t.Fatalf("after y: mode=%d cmd=%v pending=%v", m.mode, cmd != nil, m.pending)
	}
	if !strings.Contains(m.View(), "deleting 1 directory…") {
		t.Errorf("progress line missing from view:\n%s", m.View())
	}

	sc := m.scanner
	blocked := []tea.Msg{
		runeKey('d'), runeKey('y'), tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeySpace}, runeKey('a'), runeKey('r'), runeKey('n'),
	}
	for _, key := range blocked {
		m, cmd = step(t, m, key)
		if cmd != nil {
			t.Errorf("key %v returned a command during a delete", key)
		}
	}
	if m.mode != modeDelete || selectedCount(m) != 1 || m.scanner != sc {
		t.Errorf("mode=%d selected=%d rescanned=%v, want delete, 1, false", m.mode, selectedCount(m), m.scanner != sc)
	}

	m = update(t, m, runeKey('j'))
	if m.cursor != 1 {
		t.Errorf("cursor=%d, want 1: moving is allowed during a delete", m.cursor)
	}
}

func TestQuitWorksInEveryMode(t *testing.T) {
	m := listModel(t)
	m = update(t, m, runeKey('a'))
	for _, next := range []tea.Msg{runeKey('d'), runeKey('y'), nil} {
		for _, quit := range []tea.Msg{runeKey('q'), tea.KeyMsg{Type: tea.KeyCtrlC}} {
			if _, cmd := step(t, m, quit); !isQuit(cmd) {
				t.Errorf("mode %d: %v did not quit", m.mode, quit)
			}
		}
		if next != nil {
			m = update(t, m, next)
		}
	}
	if m.mode != modeDelete {
		t.Fatalf("test did not reach delete mode, mode=%d", m.mode)
	}
}

// The error banner used to stay until a rescan, and showed only the last failure.
func TestDeleteErrorLifecycle(t *testing.T) {
	m := listModel(t)
	m = update(t, m, runeKey('a'))
	m = update(t, m, runeKey('d'))
	m = update(t, m, runeKey('y'))
	m = update(t, m, deletedMsg{
		{path: "/c/node_modules", err: errors.New("permission denied")},
		{path: "/b/node_modules", err: errors.New("busy")},
		{path: "/a/node_modules"},
	})
	if m.err == nil || !strings.Contains(m.err.Error(), "2 directories were not deleted") {
		t.Fatalf("err=%v, want a count of both failures", m.err)
	}

	// the next attempt clears it, and a clean result keeps it clear
	m = update(t, m, runeKey('a'))
	m = update(t, m, runeKey('d'))
	m = update(t, m, runeKey('y'))
	if m.err != nil {
		t.Fatalf("err=%v after starting a new delete, want nil", m.err)
	}
	m = update(t, m, deletedMsg{{path: "/c/node_modules"}, {path: "/b/node_modules"}})
	if m.err != nil || len(m.dirs) != 0 {
		t.Fatalf("err=%v rows=%v, want no error and an empty list", m.err, paths(m.dirs))
	}
}

// ---------- scan generations ----------

// Pressing r mid-scan used to leave the old scan running, and its
// scanDoneMsg marked the new scan as finished.
func TestRescanCancelsOldScanAndIgnoresItsDone(t *testing.T) {
	m := initialModel(nil, "")
	old := m.scanner

	m = update(t, m, runeKey('r'))
	if m.scanner == old {
		t.Fatal("r did not start a new scanner")
	}
	if old.ctx.Err() == nil {
		t.Error("r did not cancel the old scanner")
	}
	if m.scanner.ctx.Err() != nil {
		t.Error("the new scanner is cancelled")
	}

	m = update(t, m, scanDoneMsg{old})
	if m.scanDone {
		t.Fatal("the old scanner's scanDoneMsg finished the new scan")
	}
	m = update(t, m, scanDoneMsg{m.scanner})
	if !m.scanDone {
		t.Fatal("the current scanner's scanDoneMsg was ignored")
	}
}
