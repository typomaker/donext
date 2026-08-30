package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveUsesGitRootFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := exec.Command("git", "-C", root, "init", "--quiet").Run(); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(nested)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	if got.Repository != want || got.Name != filepath.Base(want) {
		t.Fatalf("identity=%+v want root=%q", got, want)
	}
}

func TestResolveNonGitSymlinkAndSameBasename(t *testing.T) {
	base := t.TempDir()
	one := filepath.Join(base, "one", "same")
	two := filepath.Join(base, "two", "same")
	if err := os.MkdirAll(one, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(two, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(one, link); err != nil {
		t.Fatal(err)
	}
	a, err := Resolve(link)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Resolve(two)
	if err != nil {
		t.Fatal(err)
	}
	wantOne, _ := filepath.EvalSymlinks(one)
	if a.Repository != wantOne || a.Name != "same" || b.Name != "same" || a.ID == b.ID {
		t.Fatalf("first=%+v second=%+v", a, b)
	}
}
