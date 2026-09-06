package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func rootsFromArgs(args []string) ([]string, string) {
	if len(args) > 0 {
		return args, strings.Join(args, ", ")
	}
	if env := os.Getenv("NKILL_ROOTS"); env != "" {
		return strings.Split(env, string(os.PathListSeparator)), env
	}
	return defaultRoots()
}

// defaultRoots returns $HOME plus common project subdirectories, and the cwd,
// along with a display label using ~ for home.
func defaultRoots() ([]string, string) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if st, err := os.Stat(p); err == nil && st.IsDir() && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		add(home)
		for _, sub := range []string{"Documents", "Desktop", "Downloads", "Projects", "Code", "dev", "src", "workspace", "repos", "Sites", "work"} {
			add(filepath.Join(home, sub))
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	// Display label: $HOME covers its subdirectories, so show just it;
	// mention cwd only when it lives outside home.
	var label string
	if home == "" {
		label = strings.Join(out, ", ")
	} else {
		label = home
		if cwd, err := os.Getwd(); err == nil && cwd != home && !strings.HasPrefix(cwd, home+string(os.PathSeparator)) {
			label += ", " + cwd
		}
	}
	return out, label
}

func main() {
	roots, rootsLabel := rootsFromArgs(os.Args[1:])
	if _, err := os.Stat(roots[0]); err != nil {
		fmt.Println("root not accessible:", err)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(initialModel(roots, rootsLabel), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
