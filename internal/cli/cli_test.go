package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/typomaker/donext/internal/codex"
	"github.com/typomaker/donext/internal/project"
	"github.com/typomaker/donext/internal/state"
)

type fakeCodex struct {
	events           chan codex.Event
	threadStarts     int
	turnStarts       int
	closed           int
	interrupted      int
	rejected         int
	forced           int
	interruptNoEvent bool
	threadOpts       codex.ThreadOptions
	name             string
	prompt           string
	outcomes         []string
	currentThread    string
	rateLimits       []codex.RateLimits
	rateLimitErr     error
	rateLimitReads   int
	projects         []codex.Project
	projectsErr      error
	projectListReads int
}

func (f *fakeCodex) ListProjects(context.Context) ([]codex.Project, error) {
	f.projectListReads++
	return f.projects, f.projectsErr
}

func (f *fakeCodex) ReadRateLimits(context.Context) (codex.RateLimits, error) {
	f.rateLimitReads++
	if f.rateLimitErr != nil {
		return codex.RateLimits{}, f.rateLimitErr
	}
	if len(f.rateLimits) == 0 {
		return codex.RateLimits{}, nil
	}
	index := f.rateLimitReads - 1
	if index >= len(f.rateLimits) {
		index = len(f.rateLimits) - 1
	}
	return f.rateLimits[index], nil
}

func weeklyLimits(used int, resetsAt int64) codex.RateLimits {
	duration := int64(10080)
	return codex.RateLimits{Secondary: &codex.RateLimitWindow{UsedPercent: used, WindowDurationMins: &duration, ResetsAt: &resetsAt}}
}

func (f *fakeCodex) StartThread(_ context.Context, opts codex.ThreadOptions) (string, error) {
	f.threadStarts++
	f.threadOpts = opts
	if len(f.outcomes) > 0 {
		f.currentThread = fmt.Sprintf("thread-%d", f.threadStarts)
		return f.currentThread, nil
	}
	return "thread-123", nil
}
func (f *fakeCodex) NameThread(_ context.Context, _ string, name string) error {
	f.name = name
	return nil
}
func (f *fakeCodex) StartTurn(_ context.Context, _, prompt string) (string, error) {
	f.turnStarts++
	f.prompt = prompt
	if len(f.outcomes) > 0 {
		turnID := fmt.Sprintf("turn-%d", f.turnStarts)
		status := f.outcomes[f.turnStarts-1]
		if status == "no_work" {
			f.events <- codex.Event{Kind: codex.AgentMessageCompleted, ThreadID: f.currentThread, TurnID: turnID, Text: "ORCHESTRATOR_NO_WORK"}
			status = "completed"
		}
		f.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: f.currentThread, TurnID: turnID, Status: status}
		return turnID, nil
	}
	return "turn-456", nil
}
func (f *fakeCodex) InterruptTurn(_ context.Context, threadID, turnID string) error {
	f.interrupted++
	if len(f.outcomes) == 0 && !f.interruptNoEvent {
		f.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: threadID, TurnID: turnID, Status: "interrupted"}
	}
	return nil
}
func (f *fakeCodex) RejectRequest(context.Context, int64, string) error { f.rejected++; return nil }
func (f *fakeCodex) Events() <-chan codex.Event                         { return f.events }
func (f *fakeCodex) ForceClose() error                                  { f.forced++; return nil }
func (f *fakeCodex) Close() error {
	f.closed++
	return nil
}

func withSignals(t *testing.T, ch <-chan os.Signal) {
	t.Helper()
	old := subscribeSignals
	subscribeSignals = func() (<-chan os.Signal, func()) { return ch, func() {} }
	t.Cleanup(func() { subscribeSignals = old })
}

func withFakeCodex(t *testing.T, fake *fakeCodex) {
	t.Helper()
	old := startCodex
	startCodex = func(context.Context, string, io.Writer) (codex.Client, error) { return fake, nil }
	t.Cleanup(func() { startCodex = old })
}

func withPromptStdin(t *testing.T, input io.Reader, terminal bool) {
	t.Helper()
	oldInput, oldTerminal := promptStdin, stdinIsTerminal
	promptStdin = input
	stdinIsTerminal = func() bool { return terminal }
	t.Cleanup(func() { promptStdin, stdinIsTerminal = oldInput, oldTerminal })
}

func fixtureProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldStateDirectory := stateDirectory
	stateDirectory = func(repository string) (string, error) { return state.ProjectDir(repository), nil }
	t.Cleanup(func() { stateDirectory = oldStateDirectory })
	repository := filepath.Join(dir, "alpha")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	return repository
}

func fixtureIdentity(t *testing.T, repository string) project.Identity {
	t.Helper()
	identity, err := project.Resolve(repository)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func runArgs(t *testing.T, args ...string) []string {
	return runArgsFor(t, fixtureProject(t), args...)
}

func runArgsFor(t *testing.T, repository string, args ...string) []string {
	t.Helper()
	chdirForTest(t, repository)
	return args
}

func chdirForTest(t *testing.T, path string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestRuntimeDirectoryUsesCanonicalRootFromNestedSymlink(t *testing.T) {
	root := t.TempDir()
	if err := exec.Command("git", "-C", root, "init", "--quiet").Run(); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(nested, link); err != nil {
		t.Fatal(err)
	}
	identity, err := project.Resolve(link)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, _ := filepath.EvalSymlinks(root)
	if got := state.ProjectDir(identity.Repository); got != filepath.Join(canonicalRoot, ".donext") {
		t.Fatalf("runtime directory=%q", got)
	}
}

func TestRejectsRemovedProjectsCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"projects"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), `unexpected argument "projects"`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestHelpReturnsSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--help"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stderr.String(), "usage: donext") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestStatusAllProjectsAndDetailedStaleState(t *testing.T) {
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)
	chdirForTest(t, identity.Repository)
	dir, err := stateDirectory(identity.Repository)
	if err != nil {
		t.Fatal(err)
	}
	store := state.New(dir)
	if err := store.Write(state.State{Project: identity.ID, Status: "completed", ThreadID: "thread-old", TurnID: "turn-old"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"status"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "project: alpha\nproject_id: "+identity.ID) || !strings.Contains(stdout.String(), "status: completed\nlock: free") || !strings.Contains(stdout.String(), "thread: thread-old") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	if err := store.Write(state.State{Project: identity.ID, Status: "running", ThreadID: "abandoned", TurnID: "abandoned-turn"}); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"status"}, &stdout, &stderr)
	for _, want := range []string{"status: stale", "lock: free", "persisted_status: running", "thread: abandoned", "turn: abandoned-turn"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("code=%d stdout=%q stderr=%q missing=%q", code, stdout.String(), stderr.String(), want)
		}
	}
}

func TestStatusReportsRunningOnlyWithLiveLock(t *testing.T) {
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)
	chdirForTest(t, identity.Repository)
	dir, err := stateDirectory(identity.Repository)
	if err != nil {
		t.Fatal(err)
	}
	store := state.New(dir)
	if err := store.Write(state.State{Project: identity.ID, Status: "running", ThreadID: "live"}); err != nil {
		t.Fatal(err)
	}
	lock, _, err := store.Acquire(identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"status"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "status: running") || !strings.Contains(stdout.String(), "lock: held") || strings.Contains(stdout.String(), "stale") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestLifecycleLogContainsMetadataButNotPrompt(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 1)}
	fake.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: "thread-123", TurnID: "turn-456", Status: "completed"}
	close(fake.events)
	withFakeCodex(t, fake)
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)
	var stdout, stderr bytes.Buffer
	if code := Run(append(runArgsFor(t, repository), "--once"), &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	dir, _ := stateDirectory(identity.Repository)
	data, err := os.ReadFile(filepath.Join(dir, "logs", identity.ID+".lifecycle.log"))
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, want := range []string{"component=app-server event=started", "component=thread event=started", "component=turn event=started", "component=turn event=completed", "thread=thread-123", "turn=turn-456"} {
		if !strings.Contains(log, want) {
			t.Fatalf("log=%q missing=%q", log, want)
		}
	}
	if strings.Contains(log, "next goal") {
		t.Fatalf("prompt leaked into lifecycle log: %q", log)
	}
}

func TestDryRunAfterProject(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t, "--dry-run"), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "project: alpha") || !strings.Contains(stdout.String(), "command: codex app-server --stdio") || !strings.Contains(stdout.String(), "approval_policy: never\nsandbox: workspace-write") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestDryRunShowsExplicitSafetyOptions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t, "--dry-run", "--approval-policy", "on-request", "--sandbox", "read-only"), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "approval_policy: on-request\nsandbox: read-only") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestPromptSources(t *testing.T) {
	t.Run("literal does not infer existing path", func(t *testing.T) {
		repository := fixtureProject(t)
		path := filepath.Join(repository, "prompt.md")
		if err := os.WriteFile(path, []byte("file prompt"), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := Run(runArgsFor(t, repository, "--dry-run", "--prompt", "prompt.md"), &stdout, &stderr)
		if code != 0 || !strings.Contains(stdout.String(), "prompt:\nprompt.md") || strings.Contains(stdout.String(), "file prompt") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})

	t.Run("file", func(t *testing.T) {
		repository := fixtureProject(t)
		path := filepath.Join(repository, "prompt.md")
		if err := os.WriteFile(path, []byte("prompt from file\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := Run(runArgsFor(t, repository, "--dry-run", "--prompt", "@"+path), &stdout, &stderr)
		if code != 0 || !strings.Contains(stdout.String(), "prompt:\nprompt from file\n") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})

	t.Run("stdin", func(t *testing.T) {
		withPromptStdin(t, strings.NewReader("prompt from stdin\n"), false)
		var stdout, stderr bytes.Buffer
		code := Run(runArgs(t, "--dry-run", "--prompt", "-"), &stdout, &stderr)
		if code != 0 || !strings.Contains(stdout.String(), "prompt:\nprompt from stdin\n") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})
}

func TestPromptErrors(t *testing.T) {
	tests := []struct {
		name, value, want string
	}{
		{"empty literal", "", "prompt is empty"},
		{"whitespace literal", "  ", "prompt is empty"},
		{"missing file", "@missing.md", "read \"missing.md\""},
		{"missing file path", "@", "@FILE requires a path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(runArgs(t, "--dry-run", "--prompt", tt.value), &stdout, &stderr)
			if code != 2 || !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("empty file", func(t *testing.T) {
		repository := fixtureProject(t)
		path := filepath.Join(repository, "empty.md")
		if err := os.WriteFile(path, []byte("\n\t"), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := Run(runArgsFor(t, repository, "--dry-run", "--prompt", "@"+path), &stdout, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "prompt is empty") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("unreadable file", func(t *testing.T) {
		repository := fixtureProject(t)
		var stdout, stderr bytes.Buffer
		code := Run(runArgsFor(t, repository, "--dry-run", "--prompt", "@"+repository), &stdout, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "read \"") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("terminal stdin", func(t *testing.T) {
		withPromptStdin(t, strings.NewReader("ignored"), true)
		var stdout, stderr bytes.Buffer
		code := Run(runArgs(t, "--dry-run", "--prompt", "-"), &stdout, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "stdin is a terminal") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("empty stdin", func(t *testing.T) {
		withPromptStdin(t, strings.NewReader(""), false)
		var stdout, stderr bytes.Buffer
		code := Run(runArgs(t, "--dry-run", "--prompt", "-"), &stdout, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "prompt is empty") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})
}

func TestCustomPromptReachesTurnButNotLifecycleLog(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 1)}
	fake.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: "thread-123", TurnID: "turn-456", Status: "completed"}
	close(fake.events)
	withFakeCodex(t, fake)
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)
	const secretPrompt = "CUSTOM_PROMPT_DO_NOT_LOG"
	var stdout, stderr bytes.Buffer
	if code := Run(runArgsFor(t, repository, "--once", "--prompt", secretPrompt), &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if fake.prompt != secretPrompt {
		t.Fatalf("prompt=%q", fake.prompt)
	}
	dir, _ := stateDirectory(identity.Repository)
	data, err := os.ReadFile(filepath.Join(dir, "logs", identity.ID+".lifecycle.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secretPrompt) {
		t.Fatalf("prompt leaked into lifecycle log: %q", data)
	}
}

func TestRejectsInvalidSafetyOptions(t *testing.T) {
	for _, args := range [][]string{{"--approval-policy", "always"}, {"--sandbox", "host"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestRejectsRemovedRunCommandAndConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--config", "old.yaml"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunOnceCompleted(t *testing.T) {
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)
	fake := &fakeCodex{events: make(chan codex.Event, 1), projects: []codex.Project{{ID: "desktop-alpha", Name: "Alpha", Roots: []string{identity.Repository}}}}
	fake.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: "thread-123", TurnID: "turn-456", Status: "completed"}
	close(fake.events)
	withFakeCodex(t, fake)

	var stdout, stderr bytes.Buffer
	code := Run(append(runArgsFor(t, repository), "--once"), &stdout, &stderr)
	if code != 0 || stdout.String() != "project: alpha\nthread: thread-123\nstatus: completed\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if fake.threadStarts != 1 || fake.turnStarts != 1 || fake.closed != 1 || fake.prompt != defaultPrompt {
		t.Fatalf("fake=%+v", fake)
	}
	if fake.threadOpts.CWD == "" || fake.threadOpts.ProjectID != "desktop-alpha" || fake.projectListReads != 1 || fake.name != "donext alpha next roadmap step" {
		t.Fatalf("opts=%+v name=%q", fake.threadOpts, fake.name)
	}
}

func TestRunWithoutDesktopProjectWarnsAndStartsUnassigned(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 1)}
	fake.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: "thread-123", TurnID: "turn-456", Status: "completed"}
	close(fake.events)
	withFakeCodex(t, fake)

	var stdout, stderr bytes.Buffer
	if code := Run(runArgs(t, "--once"), &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if fake.threadOpts.ProjectID != "" || !strings.Contains(stderr.String(), "thread will remain unassigned") {
		t.Fatalf("opts=%+v stderr=%q", fake.threadOpts, stderr.String())
	}
}

func TestRunRejectsAmbiguousDesktopProjectBeforeThread(t *testing.T) {
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)
	fake := &fakeCodex{events: make(chan codex.Event), projects: []codex.Project{
		{ID: "p1", Name: "One", Roots: []string{identity.Repository}},
		{ID: "p2", Name: "Two", Roots: []string{identity.Repository}},
	}}
	withFakeCodex(t, fake)

	var stdout, stderr bytes.Buffer
	code := Run(append(runArgsFor(t, repository), "--once"), &stdout, &stderr)
	if code != 1 || fake.threadStarts != 0 || !strings.Contains(stderr.String(), "belongs to multiple projects") {
		t.Fatalf("code=%d starts=%d stdout=%q stderr=%q", code, fake.threadStarts, stdout.String(), stderr.String())
	}
}

func TestRunStopsWhenDesktopProjectsCannotBeRead(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event), projectsErr: errors.New("project API unavailable")}
	withFakeCodex(t, fake)

	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t, "--once"), &stdout, &stderr)
	if code != 1 || fake.threadStarts != 0 || !strings.Contains(stderr.String(), "project API unavailable") {
		t.Fatalf("code=%d starts=%d stdout=%q stderr=%q", code, fake.threadStarts, stdout.String(), stderr.String())
	}
}

func TestRunOncePersistsTerminalState(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 1)}
	fake.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: "thread-123", TurnID: "turn-456", Status: "completed"}
	close(fake.events)
	withFakeCodex(t, fake)
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)

	var stdout, stderr bytes.Buffer
	if code := Run(append(runArgsFor(t, repository), "--once"), &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	dir, err := stateDirectory(identity.Repository)
	if err != nil {
		t.Fatal(err)
	}
	got, err := state.New(dir).Read(identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" || got.ThreadID != "thread-123" || got.TurnID != "turn-456" {
		t.Fatalf("state=%+v", got)
	}
}

func TestRunRejectsConcurrentSameProject(t *testing.T) {
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)
	dir, err := stateDirectory(identity.Repository)
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := state.New(dir).Acquire(identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	var stdout, stderr bytes.Buffer
	code := Run(append(runArgsFor(t, repository), "--once"), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "project alpha is already running") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRecoversStaleStateWithNewThread(t *testing.T) {
	repository := fixtureProject(t)
	identity := fixtureIdentity(t, repository)
	dir, err := stateDirectory(identity.Repository)
	if err != nil {
		t.Fatal(err)
	}
	store := state.New(dir)
	if err := store.Write(state.State{Project: identity.ID, Status: "running", ThreadID: "abandoned-thread", TurnID: "abandoned-turn"}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeCodex{events: make(chan codex.Event, 1)}
	fake.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: "thread-123", TurnID: "turn-456", Status: "completed"}
	close(fake.events)
	withFakeCodex(t, fake)

	var stdout, stderr bytes.Buffer
	code := Run(append(runArgsFor(t, repository), "--once"), &stdout, &stderr)
	if code != 0 || fake.threadStarts != 1 || !strings.Contains(stderr.String(), "recovering project alpha from stale running state") {
		t.Fatalf("code=%d fake=%+v stdout=%q stderr=%q", code, fake, stdout.String(), stderr.String())
	}
	got, err := store.Read(identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThreadID != "thread-123" || got.Status != "completed" {
		t.Fatalf("state=%+v", got)
	}
}

func TestRunContinuousCompletedCompletedNoWork(t *testing.T) {
	fake := &fakeCodex{
		events:   make(chan codex.Event, 5),
		outcomes: []string{"completed", "completed", "no_work"},
	}
	starts := 0
	old := startCodex
	startCodex = func(context.Context, string, io.Writer) (codex.Client, error) {
		starts++
		return fake, nil
	}
	t.Cleanup(func() { startCodex = old })

	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if starts != 1 || fake.threadStarts != 3 || fake.turnStarts != 3 || fake.closed != 1 {
		t.Fatalf("starts=%d fake=%+v", starts, fake)
	}
	want := "project: alpha\nthread: thread-1\nstatus: completed\n" +
		"project: alpha\nthread: thread-2\nstatus: completed\n" +
		"project: alpha\nthread: thread-3\nstatus: no_work\n"
	if stdout.String() != want {
		t.Fatalf("stdout=%q want=%q", stdout.String(), want)
	}
}

func TestRunContinuousStopsAfterTerminalFailure(t *testing.T) {
	for _, status := range []string{"failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			fake := &fakeCodex{
				events:   make(chan codex.Event, 2),
				outcomes: []string{status, "completed"},
			}
			withFakeCodex(t, fake)

			var stdout, stderr bytes.Buffer
			code := Run(runArgs(t), &stdout, &stderr)
			if code != 1 || fake.threadStarts != 1 || fake.turnStarts != 1 {
				t.Fatalf("code=%d fake=%+v stdout=%q stderr=%q", code, fake, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunContinuousStopsBeforeNextGoalAtWeeklyBudget(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 2), outcomes: []string{"completed", "completed"}, rateLimits: []codex.RateLimits{weeklyLimits(19, 2000), weeklyLimits(20, 2000)}}
	withFakeCodex(t, fake)
	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t, "--weekly-usage-budget", "1"), &stdout, &stderr)
	want := "status: weekly_usage_budget_reached\nbaseline: 19\ncurrent_usage: 20\nconsumed_delta: 1\nbudget: 1\n"
	if code != 0 || fake.threadStarts != 1 || fake.rateLimitReads != 2 || !strings.Contains(stdout.String(), want) {
		t.Fatalf("code=%d fake=%+v stdout=%q stderr=%q", code, fake, stdout.String(), stderr.String())
	}
}

func TestRunWeeklyBudgetAllowsCompletedGoalToExceedBudget(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 2), outcomes: []string{"completed", "completed"}, rateLimits: []codex.RateLimits{weeklyLimits(10, 2000), weeklyLimits(13, 2000)}}
	withFakeCodex(t, fake)
	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t, "--weekly-usage-budget", "2"), &stdout, &stderr)
	if code != 0 || fake.threadStarts != 1 || !strings.Contains(stdout.String(), "consumed_delta: 3") {
		t.Fatalf("code=%d fake=%+v stdout=%q stderr=%q", code, fake, stdout.String(), stderr.String())
	}
}

func TestRunWeeklyBudgetFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		limits []codex.RateLimits
	}{
		{name: "unavailable", limits: []codex.RateLimits{{}}},
		{name: "rollover", limits: []codex.RateLimits{weeklyLimits(20, 2000), weeklyLimits(1, 3000)}},
		{name: "decreased", limits: []codex.RateLimits{weeklyLimits(20, 2000), weeklyLimits(19, 2000)}},
		{name: "invalid", limits: []codex.RateLimits{weeklyLimits(101, 2000)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcomes := []string{"completed", "completed"}
			fake := &fakeCodex{events: make(chan codex.Event, 2), outcomes: outcomes, rateLimits: tt.limits}
			withFakeCodex(t, fake)
			var stdout, stderr bytes.Buffer
			code := Run(runArgs(t, "--weekly-usage-budget", "5"), &stdout, &stderr)
			if code != 1 || !strings.Contains(stderr.String(), "cannot enforce --weekly-usage-budget") {
				t.Fatalf("code=%d fake=%+v stdout=%q stderr=%q", code, fake, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunRejectsInvalidWeeklyUsageBudget(t *testing.T) {
	for _, value := range []string{"-1", "0", "101"} {
		var stdout, stderr bytes.Buffer
		code := Run(runArgs(t, "--weekly-usage-budget", value), &stdout, &stderr)
		if code != 2 || !strings.Contains(stderr.String(), "integer from 1 to 100") {
			t.Fatalf("value=%s code=%d stderr=%q", value, code, stderr.String())
		}
	}
}

func TestRunOnceRecognizesNoWorkOnlyInFinalAgentOutput(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 3)}
	fake.events <- codex.Event{Kind: codex.AgentMessageCompleted, ThreadID: "other", TurnID: "turn-456", Text: "ORCHESTRATOR_NO_WORK"}
	fake.events <- codex.Event{Kind: codex.AgentMessageCompleted, ThreadID: "thread-123", TurnID: "turn-456", Text: "ORCHESTRATOR_NO_WORK\n"}
	fake.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: "thread-123", TurnID: "turn-456", Status: "completed"}
	close(fake.events)
	withFakeCodex(t, fake)

	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t, "--once"), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "status: no_work\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunOnceFailureAndInterruption(t *testing.T) {
	for _, status := range []string{"failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			fake := &fakeCodex{events: make(chan codex.Event, 1)}
			fake.events <- codex.Event{Kind: codex.TurnCompleted, ThreadID: "thread-123", TurnID: "turn-456", Status: status}
			close(fake.events)
			withFakeCodex(t, fake)
			var stdout, stderr bytes.Buffer
			code := Run(runArgs(t, "--once"), &stdout, &stderr)
			if code != 1 || !strings.Contains(stdout.String(), "status: "+status+"\n") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestSignalInterruptsActiveTurnAndDoesNotStartNextThread(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 2)}
	withFakeCodex(t, fake)
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	withSignals(t, signals)

	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t), &stdout, &stderr)
	if code != 1 || fake.interrupted != 1 || fake.threadStarts != 1 || fake.turnStarts != 1 {
		t.Fatalf("code=%d fake=%+v stdout=%q stderr=%q", code, fake, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "thread: thread-123\nstatus: interrupted") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestServerRequestIsRejectedAndFailsWithThreadID(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event, 2)}
	fake.events <- codex.Event{Kind: codex.ServerRequest, Method: "item/tool/requestUserInput", RequestID: 17}
	withFakeCodex(t, fake)
	withSignals(t, make(chan os.Signal))

	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t, "--once"), &stdout, &stderr)
	if code != 1 || fake.rejected != 1 || fake.interrupted != 1 || fake.threadStarts != 1 {
		t.Fatalf("code=%d fake=%+v stdout=%q stderr=%q", code, fake, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "thread: thread-123\nstatus: failed") || !strings.Contains(stderr.String(), "requestUserInput") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSecondSignalForceClosesImmediately(t *testing.T) {
	fake := &fakeCodex{events: make(chan codex.Event), interruptNoEvent: true}
	withFakeCodex(t, fake)
	signals := make(chan os.Signal, 2)
	signals <- os.Interrupt
	signals <- os.Interrupt
	withSignals(t, signals)

	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t), &stdout, &stderr)
	if code != 1 || fake.interrupted != 1 || fake.forced != 1 || fake.threadStarts != 1 {
		t.Fatalf("code=%d fake=%+v stdout=%q stderr=%q", code, fake, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "second shutdown signal") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRunOnceStartFailureStillPrintsTerminalFields(t *testing.T) {
	old := startCodex
	startCodex = func(context.Context, string, io.Writer) (codex.Client, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { startCodex = old })
	var stdout, stderr bytes.Buffer
	code := Run(runArgs(t, "--once"), &stdout, &stderr)
	if code != 1 || stdout.String() != "project: alpha\nthread: -\nstatus: failed\n" || !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestWarnGitStateReportsDirtyRepository(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "--quiet").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("dirty"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	warnGitState(dir, &stderr)
	if !strings.Contains(stderr.String(), "has uncommitted changes") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestWarnGitStateIgnoresRuntimeMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "--quiet").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".donext"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".donext", "state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	warnGitState(dir, &stderr)
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%q", stderr.String())
	}
}
