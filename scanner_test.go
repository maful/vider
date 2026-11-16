package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestScanFindsAndSizes(t *testing.T) {
	root := t.TempDir()
	nm1 := filepath.Join(root, "proj-a", "node_modules")
	nm2 := filepath.Join(root, "proj-b", "nested", "node_modules")
	nested := filepath.Join(nm1, "pkg") // nested inside node_modules, should not double-count
	for _, d := range []string{nm1, nm2, nested, filepath.Join(root, ".git", "hooks")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(nm1, "a.js"), make([]byte, 3*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "b.js"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nm2, "c.js"), make([]byte, 2*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newScanner([]string{root})
	s.scan()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.walkers.Load() == 0 && s.pending.Load() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	dirs := s.sortedDirs()
	if len(dirs) != 2 {
		t.Fatalf("expected 2 node_modules, got %d: %v", len(dirs), dirs)
	}
	if dirs[0].Path != nm1 || dirs[0].Size != 3*1024*1024+1024 {
		t.Errorf("largest dir wrong: %+v", dirs[0])
	}
	if dirs[1].Path != nm2 || dirs[1].Size != 2*1024*1024 {
		t.Errorf("second dir wrong: %+v", dirs[1])
	}
	if dirs[0].ModTime.IsZero() {
		t.Error("modtime not set")
	}
}

func TestDeleteDirs(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a")
	b := filepath.Join(t.TempDir(), "b")
	for _, d := range []string{a, b} {
		os.MkdirAll(filepath.Join(d, "pkg"), 0o755)
		os.WriteFile(filepath.Join(d, "pkg", "x.js"), []byte("hi"), 0o644)
	}
	res := deleteDirs([]string{a, b})().(deletedMsg)
	if len(res) != 2 {
		t.Fatalf("want 2 results, got %d", len(res))
	}
	for _, r := range res {
		if r.err != nil {
			t.Errorf("delete %s: %v", r.path, r.err)
		}
		if _, err := os.Stat(r.path); !os.IsNotExist(err) {
			t.Errorf("%s still exists", r.path)
		}
	}
}

// waitIdle blocks until the scanner has no walkers and no size jobs left.
func waitIdle(t *testing.T, s *scanner) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.walkers.Load() != 0 || s.pending.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("scanner did not go idle: walkers=%d pending=%d", s.walkers.Load(), s.pending.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func scanTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, proj := range []string{"a", "b", "c"} {
		nm := filepath.Join(root, proj, "node_modules", "pkg")
		if err := os.MkdirAll(nm, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nm, "x.js"), make([]byte, 4096), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScanDoneNamesItsScanner(t *testing.T) {
	s := newScanner([]string{scanTree(t)})
	msg, ok := s.scan()().(scanDoneMsg)
	if !ok || msg.scanner != s {
		t.Fatalf("got %#v, want a scanDoneMsg for this scanner", msg)
	}
	if n := len(s.sortedDirs()); n != 3 {
		t.Errorf("found %d dirs, want 3", n)
	}
}

func TestCancelledScanStopsWalking(t *testing.T) {
	s := newScanner([]string{scanTree(t)})
	s.cancel()
	s.cancel() // a second call is harmless
	cmd := s.scan()
	waitIdle(t, s)
	if n := len(s.sortedDirs()); n != 0 {
		t.Errorf("cancelled scan still found %d dirs", n)
	}
	if msg := cmd(); msg != nil {
		t.Errorf("cancelled scan emitted %#v, want nothing", msg)
	}
}

// A size job that is queued or running when the scan is cancelled must still
// release its pending count, and must not record a partial size.
func TestCancelDropsSizeJobs(t *testing.T) {
	root := scanTree(t)
	s := newScanner([]string{root})
	for i := 0; i < cap(s.sem); i++ {
		s.sem <- struct{}{} // every size worker is busy
	}
	path := filepath.Join(root, "a", "node_modules")
	s.discover(path)
	if s.pending.Load() != 1 {
		t.Fatalf("pending=%d, want 1 while the job waits for a worker", s.pending.Load())
	}
	s.cancel()
	waitIdle(t, s)
	if s.sized.Load() != 0 || s.sortedDirs()[0].Size != 0 {
		t.Errorf("cancelled job recorded a size: sized=%d size=%d", s.sized.Load(), s.sortedDirs()[0].Size)
	}
}

func TestDirSizeStopsWhenCancelled(t *testing.T) {
	root := scanTree(t)
	path := filepath.Join(root, "a", "node_modules")
	if size, _ := dirSize(context.Background(), path); size != 4096 {
		t.Fatalf("size=%d, want 4096", size)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if size, _ := dirSize(ctx, path); size != 0 {
		t.Errorf("size=%d after cancel, want 0", size)
	}
}

// Rows of equal size used to swap places on every tick: the map iterates in a
// random order and the sort compared sizes only.
func TestSortedDirsOrderIsStable(t *testing.T) {
	s := newScanner(nil)
	s.dirs["/big/node_modules"] = &NodeModules{Path: "/big/node_modules", Size: 900}
	for _, p := range []string{"/d/node_modules", "/b/node_modules", "/a/node_modules", "/c/node_modules"} {
		s.dirs[p] = &NodeModules{Path: p, Size: 500} // four copies of one project
	}
	s.dirs["/new/node_modules"] = &NodeModules{Path: "/new/node_modules"} // not sized yet

	want := []string{
		"/big/node_modules",
		"/a/node_modules", "/b/node_modules", "/c/node_modules", "/d/node_modules",
		"/new/node_modules",
	}
	for i := 0; i < 50; i++ {
		got := paths(s.sortedDirs())
		if !slices.Equal(got, want) {
			t.Fatalf("call %d: order %v, want %v", i, got, want)
		}
	}
}
