# donext

[![CI](https://github.com/typomaker/donext/actions/workflows/ci.yml/badge.svg)](https://github.com/typomaker/donext/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/typomaker/donext)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`donext` is a local, stateless CLI built on the installed Codex App Server. It
runs the roadmap in the current project sequentially, creating a new persisted
Codex thread for every step and waiting for its real terminal protocol event.
There is no user configuration, global project registry, or duplicate transcript.

## Requirements

- macOS or Linux;
- Go 1.23 or newer;
- a current, authenticated `codex` executable in `PATH`;
- a project with a canonical `ROADMAP.md` that defines how to select its next step;
- optionally, an `AGENTS.md` with project-specific instructions.

Windows is not currently supported because project locking uses Unix `flock`.

## Installation

### 1. Install Codex CLI

Follow the [official Codex CLI documentation](https://learn.chatgpt.com/docs/codex/cli).
The standalone installer for macOS and Linux is:

```sh
curl -fsSL https://chatgpt.com/codex/install.sh | sh
codex
```

On first launch, sign in with ChatGPT or another available authentication method.
Confirm that the executable is available:

```sh
codex --version
```

### 2. Install donext

The standard Go installation is sufficient; no separate installer is required:

```sh
go install github.com/typomaker/donext/cmd/donext@latest
donext --help
```

If your shell cannot find `donext`, add the Go binary directory to `PATH`:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

To install the current checkout instead of the published version:

```sh
git clone https://github.com/typomaker/donext.git
cd donext
go install ./cmd/donext
```

## Quick start

Run `donext` from the project you want it to manage:

```sh
cd /path/to/project
donext
```

`donext` does not parse the roadmap itself. It starts Codex with the canonical
Git root, or the canonical current directory for a non-Git project, and asks
Codex to complete exactly the first unfinished step. Codex discovers applicable
`AGENTS.md` files through its normal rules.

## Commands and modes

```text
donext [--once|--dry-run] [--prompt TEXT|@FILE|-]
       [--approval-policy POLICY] [--sandbox MODE]
       [--weekly-usage-budget N]
donext status
```

Examples:

```sh
donext                         # run until a stop condition is reached
donext --once                  # run no more than one roadmap step
donext --dry-run               # print the effective launch without starting Codex
donext --prompt 'Complete step ORCH-123'
donext --prompt @prompt.md
printf 'Complete step ORCH-123\n' | donext --prompt -
donext --approval-policy never --sandbox workspace-write
donext --weekly-usage-budget 5
donext status
```

Without `--once`, a new persisted thread is created after each successful goal.
Each thread is named with its project and local launch timestamp, for example
`donext my-project 2026-08-30 12:33:36 +03:00 next roadmap step`.
The loop stops on a standalone `ORCHESTRATOR_NO_WORK` final response, failure,
interruption, an interactive request, or the weekly usage budget. Failures and
interruptions return a nonzero exit status; `completed`, `no_work`, and
`weekly_usage_budget_reached` are successful terminal states.

`--dry-run` prints the canonical project path, exact App Server command,
effective approval policy, sandbox, and prompt. It does not create `.donext`,
start App Server, or read account rate limits. `--once` and `--dry-run` are
mutually exclusive.

### Prompt sources

`--prompt VALUE` has unambiguous source rules:

- any regular value is literal text, even if a file has the same name;
- `@PATH` reads a required file;
- `-` reads redirected standard input.

Empty prompts, missing or unreadable files, and terminal stdin are rejected
before App Server starts. Without the flag, the built-in roadmap prompt is used.
Prompts are never stored in `.donext` or lifecycle logs.

### Permissions and interactive requests

Every `thread/start` explicitly receives safe defaults:

- `--approval-policy never` (also: `on-request`, `untrusted`);
- `--sandbox workspace-write` (also: `read-only`, `danger-full-access`).

App Server always starts as `codex app-server --stdio` from `PATH`. Approval and
user-input requests are rejected, the active turn is interrupted, and unattended
orchestration exits with an error rather than answering on the user's behalf.

### Codex Desktop projects

After App Server starts, `donext` calls `project/list` and finds the single
Desktop project whose root matches the current canonical project root, including
symlink resolution. Its `projectId` is passed to each `thread/start`, so persisted
threads appear under the corresponding Codex Desktop project.

`donext` never creates or modifies Desktop projects. If no project matches, it
continues with a warning and leaves the thread unassigned. A `project/list` error
or multiple projects with the same root stops the run before creating a thread.

### Weekly usage budget

`--weekly-usage-budget N`, where `N` is 1 through 100, allows the current process
to consume at most `N` percentage points of the weekly quota. Before the first
goal, the CLI records the single 10,080-minute window as its baseline. Before
later goals, it compares current usage with that baseline. Reaching the budget
stops successfully before another thread is created.

The active turn is never interrupted, so a completed goal can overshoot the
budget. After each completed session, the CLI prints the baseline, current
weekly usage, consumed budget, configured budget, and remaining budget. Missing,
ambiguous, reset, or anomalous weekly-window data fails closed.

### Live session output and token usage

While a goal runs, `donext` reports new thread and turn IDs and prints each
completed model message to the terminal. When the turn ends, it prints input,
cached input, output, reasoning, and total token counts. When App Server supplies
a model context window, it also prints the latest request's context tokens and
percentage used. Model messages are terminal-only and are never copied into
state or lifecycle logs.

## Project state, locking, and recovery

Runtime metadata exists only under the canonical project root:

```text
.donext/
  state/
  logs/
  locks/
```

State is written atomically, and lifecycle logs contain metadata only—never the
prompt, transcript, reasoning, or command output. A project lock prevents two
runs for the same project while allowing different projects to run independently.
After a crash, the next run reports stale state and creates a new thread.

Dirty and non-Git projects are allowed with a warning. `.donext` is excluded from
the CLI's own dirty-worktree check.

`donext status` does not start Codex. It prints the current canonical repository,
stable local project ID, effective status, lock state, latest thread and turn IDs,
and update time. Persisted `running` state without a live lock is shown as `stale`.

## Development and opt-in smoke test

Normal checks do not run real Codex or consume user quota:

```sh
go test ./...
go vet ./...
go build ./cmd/donext
```

The real App Server smoke test is intentionally manual. It creates a persisted
Codex thread and consumes quota:

```sh
cd /path/to/disposable-smoke-project
/absolute/path/to/donext --once --approval-policy never --sandbox read-only
```

## Project status

`donext` does not yet have a tagged stable release. The App Server adapter is
verified against the installed Codex schema and isolated in `internal/codex`.
An incompatible protocol change in a future Codex release may require an update.

See [CHANGELOG.md](CHANGELOG.md) for notable changes,
[CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines, and
[SECURITY.md](SECURITY.md) for private vulnerability reporting.

## License

This project is available under the [MIT License](LICENSE).
