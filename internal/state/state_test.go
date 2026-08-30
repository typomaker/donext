package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsLockedAndLifecycleLog(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	lock, _, err := store.Acquire("alpha")
	if err != nil {
		t.Fatal(err)
	}
	locked, err := store.IsLocked("alpha")
	if err != nil || !locked {
		t.Fatalf("locked=%v err=%v", locked, err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	locked, err = store.IsLocked("alpha")
	if err != nil || locked {
		t.Fatalf("locked=%v err=%v", locked, err)
	}
	if err := store.LogLifecycle("alpha", "turn", "completed", map[string]string{"thread": "t-1", "status": "completed"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "logs", "alpha.lifecycle.log"))
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	for _, want := range []string{"project=alpha", "component=turn", "event=completed", "thread=t-1", "status=completed"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log %q missing %q", line, want)
		}
	}
}

func TestWriteReadAndReplaceState(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Write(State{Project: "alpha", Status: "running", ThreadID: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Write(State{Project: "alpha", Status: "completed", ThreadID: "new", TurnID: "turn"}); err != nil {
		t.Fatal(err)
	}
	got, err := New(store.dir).Read("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" || got.ThreadID != "new" || got.TurnID != "turn" || got.UpdatedAt.IsZero() {
		t.Fatalf("state=%+v", got)
	}
	matches, err := filepath.Glob(filepath.Join(store.dir, "state", ".state-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files=%v err=%v", matches, err)
	}
	info, err := os.Stat(filepath.Join(store.dir, "state", "alpha.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("info=%v err=%v", info, err)
	}
}

func TestLocksArePerProject(t *testing.T) {
	store := New(t.TempDir())
	alpha, _, err := store.Acquire("alpha")
	if err != nil {
		t.Fatal(err)
	}
	defer alpha.Close()
	if _, _, err := store.Acquire("alpha"); !errors.Is(err, ErrLocked) {
		t.Fatalf("same project error=%v", err)
	}
	beta, _, err := store.Acquire("beta")
	if err != nil {
		t.Fatalf("different project: %v", err)
	}
	if err := beta.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireRecognizesStaleRunningState(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Write(State{Project: "alpha", Status: "running", ThreadID: "abandoned"}); err != nil {
		t.Fatal(err)
	}
	lock, recovered, err := store.Acquire("alpha")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if !recovered {
		t.Fatal("expected stale running state to be recognized")
	}
}

func TestLegacyFlatStateIsIgnored(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"project":"alpha","status":"running"}`
	if err := os.WriteFile(filepath.Join(dir, "alpha.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(dir)
	if _, err := store.Read("alpha"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy flat state must not be adopted, err=%v", err)
	}
	lock, recovered, err := store.Acquire("alpha")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if recovered {
		t.Fatal("legacy flat state must not trigger recovery")
	}
}

func TestProjectDirUsesRepository(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "project")
	if got := ProjectDir(repository); got != filepath.Join(repository, ".donext") {
		t.Fatalf("ProjectDir()=%q", got)
	}
}
