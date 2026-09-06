package main

import (
	"os"
	"path/filepath"
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
