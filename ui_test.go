package main

import (
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
