package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/typomaker/donext/internal/codex"
	projectid "github.com/typomaker/donext/internal/project"
	"github.com/typomaker/donext/internal/state"
)

type codexStarter func(context.Context, string, io.Writer) (codex.Client, error)

var startCodex codexStarter = func(ctx context.Context, command string, stderr io.Writer) (codex.Client, error) {
	return codex.StartAppServer(ctx, command, stderr)
}

var stateDirectory = func(repository string) (string, error) {
	return state.ProjectDir(repository), nil
}
var shutdownGracePeriod = 3 * time.Second
var currentTime = time.Now
var revealDesktopThread = revealThreadInDesktop
var promptStdin io.Reader = os.Stdin
var stdinIsTerminal = func() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
var subscribeSignals = func() (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch, func() { signal.Stop(ch) }
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "status" {
		return statusCommand(args[1:], stdout, stderr)
	}
	root := flag.NewFlagSet("donext", flag.ContinueOnError)
	root.SetOutput(stderr)
	once := root.Bool("once", false, "run exactly one Codex session")
	dryRun := root.Bool("dry-run", false, "show the concrete launch without starting Codex")
	var verbose bool
	root.BoolVar(&verbose, "v", false, "show session lifecycle, requests, and expanded system status")
	root.BoolVar(&verbose, "verbose", false, "show session lifecycle, requests, and expanded system status")
	approvalPolicy := root.String("approval-policy", "never", "Codex approval policy: never, on-request, or untrusted")
	sandbox := root.String("sandbox", "workspace-write", "Codex sandbox: read-only, workspace-write, or danger-full-access")
	weeklyUsageBudget := root.Int("weekly-usage-budget", 0, "weekly quota percentage points available to this run")
	promptValue := root.String("prompt", "", "prompt text, @FILE, or - for stdin")
	root.Usage = func() { usage(stderr) }
	if err := root.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if root.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected argument %q\n", root.Arg(0))
		return 2
	}
	if *dryRun && *once {
		fmt.Fprintln(stderr, "--once and --dry-run are mutually exclusive")
		return 2
	}
	if !validApprovalPolicy(*approvalPolicy) {
		fmt.Fprintln(stderr, "--approval-policy must be one of: never, on-request, untrusted")
		return 2
	}
	if !validSandbox(*sandbox) {
		fmt.Fprintln(stderr, "--sandbox must be one of: read-only, workspace-write, danger-full-access")
		return 2
	}
	if flagWasSet(root, "weekly-usage-budget") && (*weeklyUsageBudget < 1 || *weeklyUsageBudget > 100) {
		fmt.Fprintln(stderr, "--weekly-usage-budget requires an integer from 1 to 100")
		return 2
	}
	prompt, err := resolvePrompt(*promptValue, flagWasSet(root, "prompt"))
	if err != nil {
		fmt.Fprintf(stderr, "--prompt: %v\n", err)
		return 2
	}
	return runCommand(runOptions{once: *once, dryRun: *dryRun, verbose: verbose, approvalPolicy: *approvalPolicy, sandbox: *sandbox, weeklyUsageBudget: *weeklyUsageBudget, prompt: prompt}, stdout, stderr)
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

func resolvePrompt(value string, explicitlySet bool) (string, error) {
	if !explicitlySet {
		return defaultPrompt, nil
	}
	var prompt string
	switch {
	case value == "-":
		if stdinIsTerminal() {
			return "", errors.New("stdin is a terminal; pipe or redirect prompt input")
		}
		data, err := io.ReadAll(promptStdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		prompt = string(data)
	case strings.HasPrefix(value, "@"):
		path := strings.TrimPrefix(value, "@")
		if path == "" {
			return "", errors.New("@FILE requires a path")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %q: %w", path, err)
		}
		prompt = string(data)
	default:
		prompt = value
	}
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("prompt is empty")
	}
	return composePrompt(prompt), nil
}

func composePrompt(task string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		return orchestrationContract
	}
	return task + "\n\n" + orchestrationContract
}

func statusCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: donext status")
		return 2
	}
	identity, err := projectid.Resolve(".")
	if err != nil {
		fmt.Fprintf(stderr, "identify current project: %v\n", err)
		return 1
	}
	stateDir, err := stateDirectory(identity.Repository)
	if err != nil {
		fmt.Fprintf(stderr, "locate state directory: %v\n", err)
		return 1
	}
	store := state.New(stateDir)
	current, readErr := store.Read(identity.ID)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		fmt.Fprintln(stderr, readErr)
		return 1
	}
	locked, err := store.IsLocked(identity.ID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	actual := "idle"
	if readErr == nil {
		actual = current.Status
		if current.Status == "running" && !locked {
			actual = "stale"
		}
	}
	fmt.Fprintf(stdout, "project: %s\nproject_id: %s\nrepository: %s\nstatus: %s\nlock: %s\n", identity.Name, identity.ID, identity.Repository, actual, lockLabel(locked))
	if actual == "stale" {
		fmt.Fprintln(stdout, "persisted_status: running")
	}
	threadID, turnID, updated := "-", "-", "-"
	if readErr == nil {
		if current.ThreadID != "" {
			threadID = current.ThreadID
		}
		if current.TurnID != "" {
			turnID = current.TurnID
		}
		updated = current.UpdatedAt.Format(time.RFC3339)
	}
	fmt.Fprintf(stdout, "thread: %s\nturn: %s\nupdated: %s\n", threadID, turnID, updated)
	return 0
}

func lockLabel(locked bool) string {
	if locked {
		return "held"
	}
	return "free"
}

type runOptions struct {
	once, dryRun, verbose   bool
	approvalPolicy, sandbox string
	weeklyUsageBudget       int
	prompt                  string
}

type projectSpec struct {
	ID, Name, Repository, Prompt, ApprovalPolicy, Sandbox string
	Verbose                                               bool
}

const orchestrationContract = `## Exit codes

Emit a session exit code only when terminating the session.

- ` + "`DONEXT_NO_WORK`" + ` — exit 0: the current task is complete and no actionable work remains in the available project plan.
- ` + "`DONEXT_BLOCKED`" + ` — blocked exit: progress is impossible without external input, access, credentials, or unavailable infrastructure.

Failures during execution are not exit conditions. Failed tests, builds, commands, or implementation attempts must be investigated and fixed when possible.

If you can still take any reasonable action yourself, continue working.

The exit code must appear only as the final line of the final response.
`

var defaultPrompt = composePrompt("")

func runCommand(options runOptions, stdout, stderr io.Writer) int {
	identity, err := projectid.Resolve(".")
	if err != nil {
		fmt.Fprintf(stderr, "identify project: %v\n", err)
		return 1
	}
	project := projectSpec{ID: identity.ID, Name: identity.Name, Repository: identity.Repository, Prompt: options.prompt, ApprovalPolicy: options.approvalPolicy, Sandbox: options.sandbox, Verbose: options.verbose}
	if options.dryRun {
		fmt.Fprintf(stdout, "project: %s\nrepository: %s\ncommand: codex app-server --stdio\napproval_policy: %s\nsandbox: %s\nonce: %t\nprompt:\n%s", project.Name, project.Repository, project.ApprovalPolicy, project.Sandbox, options.once, project.Prompt)
		return 0
	}
	signals, stopSignals := subscribeSignals()
	defer stopSignals()
	return runGoals(context.Background(), "codex", project, options.once, options.weeklyUsageBudget, signals, stdout, stderr)
}

func runGoals(ctx context.Context, command string, project projectSpec, once bool, weeklyUsageBudget int, signals <-chan os.Signal, stdout, stderr io.Writer) int {
	projectID, projectName := project.ID, project.Name
	stateDir, err := stateDirectory(project.Repository)
	if err != nil {
		fmt.Fprintf(stderr, "locate state directory: %v\n", err)
		return 1
	}
	store := state.New(stateDir)
	lock, recovered, err := store.Acquire(projectID)
	if err != nil {
		printTerminal(stdout, projectName, "-", "failed")
		if errors.Is(err, state.ErrLocked) {
			fmt.Fprintf(stderr, "project %s is already running\n", projectName)
		} else {
			fmt.Fprintf(stderr, "acquire project lock: %v\n", err)
		}
		return 1
	}
	defer lock.Close()
	if recovered {
		fmt.Fprintf(stderr, "warning: recovering project %s from stale running state; a new thread will be created\n", projectName)
	}
	logLifecycle(store, stderr, projectID, "donext", "run_started", nil)
	if err := store.Write(state.State{Project: projectID, Status: "running"}); err != nil {
		printTerminal(stdout, projectName, "-", "failed")
		fmt.Fprintf(stderr, "write running state: %v\n", err)
		return 1
	}
	warnGitState(project.Repository, stderr)
	appServerStderr := io.Discard
	if project.Verbose {
		appServerStderr = stderr
	}
	client, err := startCodex(ctx, command, appServerStderr)
	if err != nil {
		_ = store.Write(state.State{Project: projectID, Status: "failed"})
		logLifecycle(store, stderr, projectID, "app-server", "start_failed", nil)
		printTerminal(stdout, projectName, "-", "failed")
		fmt.Fprintf(stderr, "start app-server: %v\n", err)
		return 1
	}
	logLifecycle(store, stderr, projectID, "app-server", "started", nil)
	defer func() {
		if err := client.Close(); err != nil {
			fmt.Fprintf(stderr, "close app-server: %v\n", err)
		}
		logLifecycle(store, stderr, projectID, "app-server", "stopped", nil)
	}()
	var baseline *codex.RateLimitWindow
	for {
		if weeklyUsageBudget > 0 {
			limits, err := client.ReadRateLimits(ctx)
			if err != nil {
				_ = store.Write(state.State{Project: projectID, Status: "failed"})
				fmt.Fprintf(stderr, "read account rate limits: %v\n", err)
				return 1
			}
			current, err := weeklyWindow(limits)
			if err != nil {
				_ = store.Write(state.State{Project: projectID, Status: "failed"})
				fmt.Fprintf(stderr, "weekly account rate limit is unavailable; cannot enforce --weekly-usage-budget: %v\n", err)
				return 1
			}
			if baseline == nil {
				baseline = current
			} else if err := validateSameWeeklyWindow(*baseline, *current); err != nil {
				_ = store.Write(state.State{Project: projectID, Status: "failed"})
				fmt.Fprintf(stderr, "weekly account rate limit changed unexpectedly; cannot enforce --weekly-usage-budget: %v\n", err)
				return 1
			}
			delta := current.UsedPercent - baseline.UsedPercent
			if delta >= weeklyUsageBudget {
				_ = store.Write(state.State{Project: projectID, Status: "weekly_usage_budget_reached"})
				fields := map[string]string{"baseline": fmt.Sprint(baseline.UsedPercent), "current_usage": fmt.Sprint(current.UsedPercent), "consumed_delta": fmt.Sprint(delta), "budget": fmt.Sprint(weeklyUsageBudget)}
				logLifecycle(store, stderr, projectID, "donext", "weekly_usage_budget_reached", fields)
				fmt.Fprintf(stdout, "project: %s\nthread: -\nstatus: weekly_usage_budget_reached\nbaseline: %d\ncurrent_usage: %d\nconsumed_delta: %d\nbudget: %d\n", projectName, baseline.UsedPercent, current.UsedPercent, delta, weeklyUsageBudget)
				return 0
			}
		}
		status, ok := runGoal(ctx, client, project, store, baseline, weeklyUsageBudget, signals, stdout, stderr)
		if !ok {
			return 1
		}
		if once || status == "no_work" {
			return 0
		}
	}
}

const weeklyWindowMinutes int64 = 7 * 24 * 60

func weeklyWindow(limits codex.RateLimits) (*codex.RateLimitWindow, error) {
	var weekly []*codex.RateLimitWindow
	for _, window := range []*codex.RateLimitWindow{limits.Primary, limits.Secondary} {
		if window != nil && window.WindowDurationMins != nil && *window.WindowDurationMins == weeklyWindowMinutes {
			weekly = append(weekly, window)
		}
	}
	if len(weekly) != 1 {
		return nil, fmt.Errorf("expected exactly one 10080-minute window, got %d", len(weekly))
	}
	if weekly[0].UsedPercent < 0 || weekly[0].UsedPercent > 100 {
		return nil, fmt.Errorf("invalid usedPercent %d", weekly[0].UsedPercent)
	}
	return weekly[0], nil
}

func validateSameWeeklyWindow(baseline, current codex.RateLimitWindow) error {
	if baseline.ResetsAt != nil && current.ResetsAt != nil && *baseline.ResetsAt != *current.ResetsAt {
		return errors.New("weekly window rolled over")
	}
	if current.UsedPercent < baseline.UsedPercent {
		return fmt.Errorf("usage decreased from %d to %d", baseline.UsedPercent, current.UsedPercent)
	}
	return nil
}

func runGoal(ctx context.Context, client codex.Client, project projectSpec, store *state.Store, weeklyBaseline *codex.RateLimitWindow, weeklyBudget int, signals <-chan os.Signal, stdout, stderr io.Writer) (string, bool) {
	projectID, projectName := project.ID, project.Name
	if project.Verbose {
		printMarkedLine(stderr, '#', "starting new Codex session")
	}
	threadID, err := client.StartThread(ctx, codex.ThreadOptions{CWD: project.Repository, ApprovalPolicy: project.ApprovalPolicy, Sandbox: project.Sandbox})
	if err != nil {
		_ = store.Write(state.State{Project: projectID, Status: "failed"})
		logLifecycle(store, stderr, projectID, "thread", "start_failed", nil)
		printTerminal(stdout, projectName, "-", "failed")
		fmt.Fprintf(stderr, "start thread: %v\n", err)
		return "failed", false
	}
	if project.Verbose {
		printMarkedLine(stderr, '#', "session started: thread="+threadID)
	}
	logLifecycle(store, stderr, projectID, "thread", "started", map[string]string{"thread": threadID})
	if err := store.Write(state.State{Project: projectID, Status: "running", ThreadID: threadID}); err != nil {
		printTerminal(stdout, projectName, threadID, "failed")
		fmt.Fprintf(stderr, "write running state: %v\n", err)
		return "failed", false
	}
	startedAt := currentTime()
	threadName := startedAt.Format("02 Jan 15:04") + " · next roadmap step"
	if err := client.NameThread(ctx, threadID, threadName); err != nil {
		_ = store.Write(state.State{Project: projectID, Status: "failed", ThreadID: threadID})
		logLifecycle(store, stderr, projectID, "thread", "name_failed", map[string]string{"thread": threadID})
		printTerminal(stdout, projectName, threadID, "failed")
		fmt.Fprintf(stderr, "name thread: %v\n", err)
		return "failed", false
	}
	if err := revealDesktopThread(threadID); err != nil {
		fmt.Fprintf(stderr, "warning: reveal thread %s in Codex Desktop: %v\n", threadID, err)
	}
	if project.Verbose {
		printMarkedMessage(stderr, '<', project.Prompt)
	}
	turnID, err := client.StartTurn(ctx, threadID, project.Prompt)
	if err != nil {
		_ = store.Write(state.State{Project: projectID, Status: "failed", ThreadID: threadID})
		logLifecycle(store, stderr, projectID, "turn", "start_failed", map[string]string{"thread": threadID})
		printTerminal(stdout, projectName, threadID, "failed")
		fmt.Fprintf(stderr, "start turn: %v\n", err)
		return "failed", false
	}
	if project.Verbose {
		printMarkedLine(stderr, '#', "model turn started: turn="+turnID)
	}
	logLifecycle(store, stderr, projectID, "turn", "started", map[string]string{"thread": threadID, "turn": turnID})
	if err := store.Write(state.State{Project: projectID, Status: "running", ThreadID: threadID, TurnID: turnID}); err != nil {
		printTerminal(stdout, projectName, threadID, "failed")
		fmt.Fprintf(stderr, "write running state: %v\n", err)
		return "failed", false
	}

	finalOutput := ""
	var tokenUsage *codex.Event
	var grace <-chan time.Time
	desiredStatus := ""
	stopReason := ""
	for {
		select {
		case <-signals:
			if desiredStatus != "" {
				fmt.Fprintln(stderr, "second shutdown signal received; terminating app-server immediately")
				_ = client.ForceClose()
				return finishStopped(store, projectID, projectName, threadID, turnID, desiredStatus, stopReason, stdout, stderr)
			}
			desiredStatus, stopReason = "interrupted", "shutdown signal received"
			fmt.Fprintf(stderr, "interrupting active turn %s in thread %s\n", turnID, threadID)
			if err := client.InterruptTurn(ctx, threadID, turnID); err != nil {
				fmt.Fprintf(stderr, "interrupt turn: %v\n", err)
			}
			grace = time.After(shutdownGracePeriod)
		case <-grace:
			fmt.Fprintln(stderr, "shutdown grace period expired; terminating app-server")
			_ = client.ForceClose()
			return finishStopped(store, projectID, projectName, threadID, turnID, desiredStatus, stopReason, stdout, stderr)
		case event, open := <-client.Events():
			if !open {
				if desiredStatus != "" {
					return finishStopped(store, projectID, projectName, threadID, turnID, desiredStatus, stopReason, stdout, stderr)
				}
				_ = store.Write(state.State{Project: projectID, Status: "failed", ThreadID: threadID, TurnID: turnID})
				logLifecycle(store, stderr, projectID, "turn", "stream_closed", map[string]string{"thread": threadID, "turn": turnID, "status": "failed"})
				printTerminal(stdout, projectName, threadID, "failed")
				fmt.Fprintln(stderr, "app-server event stream closed before turn/completed")
				return "failed", false
			}
			if event.Kind == codex.ServerRequest {
				if (event.ThreadID != "" && event.ThreadID != threadID) || (event.TurnID != "" && event.TurnID != turnID) {
					continue
				}
			} else if event.ThreadID != threadID || event.TurnID != turnID {
				continue
			}
			if event.Kind == codex.ServerRequest {
				if err := client.RejectRequest(ctx, event.RequestID, "interactive requests are disabled for unattended orchestration"); err != nil {
					fmt.Fprintf(stderr, "reject server request %s: %v\n", event.Method, err)
				}
				if desiredStatus == "" {
					desiredStatus = "failed"
					stopReason = fmt.Sprintf("interactive server request %s cannot be handled (thread %s)", event.Method, threadID)
					fmt.Fprintln(stderr, stopReason)
					if err := client.InterruptTurn(ctx, threadID, turnID); err != nil {
						fmt.Fprintf(stderr, "interrupt turn: %v\n", err)
					}
					grace = time.After(shutdownGracePeriod)
				}
				continue
			}
			if event.Kind == codex.AgentMessageCompleted {
				finalOutput = event.Text
				if visible := visibleModelOutput(event.Text); visible != "" {
					printMarkedMessage(stderr, '>', visible)
				} else {
					printMarkedMessage(stderr, '>', "[response completed with no visible output]")
				}
				continue
			}
			if event.Kind == codex.ReasoningCompleted {
				printMarkedMessage(stderr, '?', event.Text)
				continue
			}
			if event.Kind == codex.CommandStarted {
				printMarkedMessage(stderr, '$', event.Text)
				continue
			}
			if event.Kind == codex.FileChangeCompleted {
				for _, path := range event.Paths {
					printMarkedLine(stderr, '~', path)
				}
				continue
			}
			if event.Kind == codex.TokenUsageUpdated {
				usage := event
				tokenUsage = &usage
				continue
			}
			if event.Kind != codex.TurnCompleted {
				continue
			}
			status := event.Status
			if desiredStatus != "" {
				status = desiredStatus
			}
			if status == "completed" {
				if containsMarkerLine(finalOutput, "DONEXT_BLOCKED") {
					status = "blocked"
				} else if containsMarkerLine(finalOutput, "DONEXT_NO_WORK") {
					status = "no_work"
				}
			}
			if err := store.Write(state.State{Project: projectID, Status: status, ThreadID: threadID, TurnID: turnID}); err != nil {
				printTerminal(stdout, projectName, threadID, "failed")
				fmt.Fprintf(stderr, "write terminal state: %v\n", err)
				return "failed", false
			}
			lifecycleEvent := "completed"
			if status == "blocked" {
				lifecycleEvent = "blocked"
			}
			logLifecycle(store, stderr, projectID, "turn", lifecycleEvent, map[string]string{"thread": threadID, "turn": turnID, "status": status})
			printTerminal(stdout, projectName, threadID, status)
			printUsageSummary(ctx, client, tokenUsage, weeklyBaseline, weeklyBudget, project.Verbose, stdout, stderr)
			if status == "completed" || status == "no_work" {
				return status, true
			}
			return status, false
		}
	}
}

func printMarkedMessage(w io.Writer, marker byte, message string) {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	message = strings.TrimSuffix(message, "\n")
	for _, line := range strings.Split(message, "\n") {
		printMarkedLine(w, marker, line)
	}
}

func printMarkedLine(w io.Writer, marker byte, line string) {
	timestamp := currentTime().Format("15:04:05.000")
	if line == "" {
		fmt.Fprintf(w, "%s %c\n", timestamp, marker)
		return
	}
	fmt.Fprintf(w, "%s %c %s\n", timestamp, marker, line)
}

func revealThreadInDesktop(threadID string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return exec.Command("open", "-g", "codex://threads/"+threadID).Run()
}

func printUsageSummary(ctx context.Context, client codex.Client, usage *codex.Event, weeklyBaseline *codex.RateLimitWindow, weeklyBudget int, verbose bool, stdout, stderr io.Writer) {
	if usage != nil {
		contextTokens := usage.LastUsage.TotalTokens
		if verbose {
			fmt.Fprintf(stdout, "= input_tokens: %d\n= cached_input_tokens: %d\n= output_tokens: %d\n= reasoning_output_tokens: %d\n= total_tokens: %d\n", usage.TotalUsage.InputTokens, usage.TotalUsage.CachedInputTokens, usage.TotalUsage.OutputTokens, usage.TotalUsage.ReasoningOutputTokens, usage.TotalUsage.TotalTokens)
			if usage.ContextWindow != nil && *usage.ContextWindow > 0 {
				fmt.Fprintf(stdout, "= context_tokens: %d\n= context_window: %d\n= context_used_percent: %.1f\n", contextTokens, *usage.ContextWindow, float64(contextTokens)*100/float64(*usage.ContextWindow))
			}
		} else if usage.ContextWindow != nil && *usage.ContextWindow > 0 {
			fmt.Fprintf(stdout, "= tokens input=%d cached=%d output=%d reasoning=%d total=%d context=%d/%d (%.1f%%)\n", usage.TotalUsage.InputTokens, usage.TotalUsage.CachedInputTokens, usage.TotalUsage.OutputTokens, usage.TotalUsage.ReasoningOutputTokens, usage.TotalUsage.TotalTokens, contextTokens, *usage.ContextWindow, float64(contextTokens)*100/float64(*usage.ContextWindow))
		} else {
			fmt.Fprintf(stdout, "= tokens input=%d cached=%d output=%d reasoning=%d total=%d context=unavailable\n", usage.TotalUsage.InputTokens, usage.TotalUsage.CachedInputTokens, usage.TotalUsage.OutputTokens, usage.TotalUsage.ReasoningOutputTokens, usage.TotalUsage.TotalTokens)
		}
	} else {
		fmt.Fprintln(stdout, "= token_usage: unavailable")
	}
	if weeklyBaseline == nil || weeklyBudget == 0 {
		return
	}
	limits, err := client.ReadRateLimits(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "warning: read post-session weekly usage: %v\n", err)
		return
	}
	current, err := weeklyWindow(limits)
	if err != nil || validateSameWeeklyWindow(*weeklyBaseline, *current) != nil {
		fmt.Fprintln(stderr, "warning: post-session weekly usage is unavailable")
		return
	}
	delta := current.UsedPercent - weeklyBaseline.UsedPercent
	remaining := weeklyBudget - delta
	if remaining < 0 {
		remaining = 0
	}
	if verbose {
		fmt.Fprintf(stdout, "= weekly_usage_baseline: %d\n= weekly_usage_current: %d\n= weekly_budget_consumed: %d\n= weekly_budget: %d\n= weekly_budget_remaining: %d\n", weeklyBaseline.UsedPercent, current.UsedPercent, delta, weeklyBudget, remaining)
	} else {
		fmt.Fprintf(stdout, "= weekly_usage baseline=%d current=%d consumed=%d budget=%d remaining=%d\n", weeklyBaseline.UsedPercent, current.UsedPercent, delta, weeklyBudget, remaining)
	}
}

func logLifecycle(store *state.Store, stderr io.Writer, project, component, event string, fields map[string]string) {
	if err := store.LogLifecycle(project, component, event, fields); err != nil {
		fmt.Fprintf(stderr, "warning: write lifecycle log: %v\n", err)
	}
}

func finishStopped(store *state.Store, projectID, projectName, threadID, turnID, status, reason string, stdout, stderr io.Writer) (string, bool) {
	if status == "" {
		status = "failed"
	}
	if err := store.Write(state.State{Project: projectID, Status: status, ThreadID: threadID, TurnID: turnID}); err != nil {
		fmt.Fprintf(stderr, "write terminal state: %v\n", err)
	}
	logLifecycle(store, stderr, projectID, "turn", "stopped", map[string]string{"thread": threadID, "turn": turnID, "status": status})
	printTerminal(stdout, projectName, threadID, status)
	if reason != "" && status != "failed" {
		fmt.Fprintln(stderr, reason)
	}
	return status, false
}

func containsMarkerLine(output, marker string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == marker {
			return true
		}
	}
	return false
}

func visibleModelOutput(output string) string {
	markers := map[string]bool{
		"DONEXT_NO_WORK": true,
		"DONEXT_BLOCKED": true,
	}
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	visible := lines[:0]
	for _, line := range lines {
		if !markers[strings.TrimSpace(line)] {
			visible = append(visible, line)
		}
	}
	return strings.TrimSpace(strings.Join(visible, "\n"))
}

func printTerminal(w io.Writer, project, threadID, status string) {
	fmt.Fprintf(w, "project: %s\nthread: %s\nstatus: %s\n", project, threadID, status)
}

func warnGitState(repository string, stderr io.Writer) {
	cmd := exec.Command("git", "-C", repository, "status", "--porcelain", "--", ".", ":(exclude).donext")
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not inspect Git state for %s: %v\n", repository, err)
		return
	}
	if len(out) != 0 {
		fmt.Fprintf(stderr, "warning: project %s has uncommitted changes\n", repository)
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: donext [--once|--dry-run] [-v|--verbose] [--prompt TEXT|@FILE|-] [--approval-policy POLICY] [--sandbox MODE] [--weekly-usage-budget N]")
	fmt.Fprintln(w, "       donext status")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "options:")
	fmt.Fprintln(w, "  -v, --verbose")
	fmt.Fprintln(w, "      show session lifecycle, requests, and expanded system status; activity lines use local HH:MM:SS.mmm time")
	fmt.Fprintln(w, "  --approval-policy POLICY")
	fmt.Fprintln(w, "      never (default), on-request, untrusted")
	fmt.Fprintln(w, "  --sandbox MODE")
	fmt.Fprintln(w, "      workspace-write (default), read-only, danger-full-access")
}

func validApprovalPolicy(value string) bool {
	return value == "never" || value == "on-request" || value == "untrusted"
}
func validSandbox(value string) bool {
	return value == "read-only" || value == "workspace-write" || value == "danger-full-access"
}
