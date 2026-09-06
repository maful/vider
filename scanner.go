package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// NodeModules is one discovered node_modules directory.
type NodeModules struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// scanDoneMsg is emitted when scanning finishes.
type scanDoneMsg struct{}

type scanner struct {
	roots []string

	walkers atomic.Int64 // in-flight walk goroutines
	pending atomic.Int64 // size jobs not yet finished
	found   atomic.Int64
	sized   atomic.Int64

	sem  chan struct{} // bounds concurrent size jobs
	mu   sync.Mutex
	dirs map[string]*NodeModules
}

func newScanner(roots []string) *scanner {
	s := &scanner{
		roots: roots,
		sem:   make(chan struct{}, workers()),
		dirs:  map[string]*NodeModules{},
	}
	return s
}

func workers() int {
	n := 32
	if v := os.Getenv("VIDER_WORKERS"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
			n = 32
		}
	}
	return n
}

// skipDirs are pruned during traversal for speed.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"Library": true, "Trash": true, ".Trash": true,
	"node_modules": true, "proc": true, "sys": true, "dev": true,
	".cache": true, ".cargo": true, "target": true, "dist": true, "build": true,
	"venv": true, ".venv": true, "__pycache__": true,
	".npm": true, ".pnpm-store": true, ".yarn": true, "DerivedData": true,
}

// scan starts the walk/sizing pipeline and returns a tea.Cmd that emits
// scanDoneMsg once every walker has finished and all discovered dirs are sized.
func (s *scanner) scan() tea.Cmd {
	s.start()
	return func() tea.Msg {
		for {
			time.Sleep(25 * time.Millisecond)
			if s.walkers.Load() == 0 && s.pending.Load() == 0 {
				return scanDoneMsg{}
			}
		}
	}
}

// start launches one walk goroutine per root. Sizing of discovered dirs runs
// concurrently with the walk itself on a bounded worker pool, so results
// stream in as fast as the disk allows.
func (s *scanner) start() {
	for _, root := range s.roots {
		// increment synchronously so the done-watcher can never observe a
		// spurious walkers==0 before the goroutines get scheduled
		s.walkers.Add(1)
		go s.walk(root)
	}
}

func (s *scanner) walk(root string) {
	defer s.walkers.Add(-1)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil // skip unreadable entries
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == "node_modules" {
			s.discover(path)
			return fs.SkipDir // nested node_modules are irrelevant
		}
		if path != root && (skipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && d.Name() != ".")) {
			return fs.SkipDir
		}
		return nil
	})
}

func (s *scanner) discover(path string) {
	s.found.Add(1)
	s.pending.Add(1)
	s.mu.Lock()
	s.dirs[path] = &NodeModules{Path: path}
	s.mu.Unlock()

	go func() {
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		size, mt := dirSize(path)
		s.mu.Lock()
		if nm, ok := s.dirs[path]; ok {
			nm.Size = size
			nm.ModTime = mt
		}
		s.mu.Unlock()
		s.sized.Add(1)
		s.pending.Add(-1)
	}()
}

// dirSize sums file sizes under path, walking in parallel for speed on
// wide trees: subdirectories are handed off to the same bounded pool.
func dirSize(path string) (int64, time.Time) {
	var total int64
	var newest time.Time
	var mu sync.Mutex
	sem := s_sizeSem
	var wg sync.WaitGroup
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				select {
				case sem <- struct{}{}:
					wg.Add(1)
					go func(p string) {
						defer func() { wg.Done(); <-sem }()
						walk(p)
					}(p)
				default:
					walk(p)
				}
				continue
			}
			if info, err := e.Info(); err == nil {
				mu.Lock()
				total += info.Size()
				if mt := info.ModTime(); mt.After(newest) {
					newest = mt
				}
				mu.Unlock()
			}
		}
	}
	walk(path)
	wg.Wait()
	return total, newest
}

// s_sizeSem bounds the parallel size walk; sized to workers().
var s_sizeSem = make(chan struct{}, workers())

// sortedDirs returns dirs ordered by size (largest first); unsized ones last.
func (s *scanner) sortedDirs() []*NodeModules {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*NodeModules, 0, len(s.dirs))
	for _, nm := range s.dirs {
		out = append(out, nm)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Size > out[j].Size
	})
	return out
}
