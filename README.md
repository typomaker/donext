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
Git root, or the canonical current directory for a non-Git project. Goal
selection and execution behavior come from the user and the project's own
instructions and documentation. Codex discovers applicable `AGENTS.md` files
through its normal rules.

## Commands and modes

```text
donext [--once|--dry-run] [-v|--verbose] [--prompt TEXT|@FILE|-]
       [--approval-policy POLICY] [--sandbox MODE]
       [--weekly-usage-budget N]
donext status
```

Run `donext --help` to see all accepted approval policies and sandbox modes,
including their defaults.

Examples:

```sh
donext                         # run until a stop condition is reached
donext --once                  # run no more than one Codex session
donext --dry-run               # print the effective launch without starting Codex
donext --prompt 'Complete step ORCH-123'
donext --prompt @prompt.md
printf 'Complete step ORCH-123\n' | donext --prompt -
donext --approval-policy never --sandbox workspace-write
donext --weekly-usage-budget 5
donext -v                      # show labeled model requests and responses
donext status
```

Without `--once`, a new persisted thread is created after each successful goal.
Each thread starts with a temporary local discovery title, for example
`30 Aug 14:08 · discovering task`. After Codex identifies the task from project
context, the thread is renamed to a concise task-aware title such as
`ORCH-039 · Context-aware session titles · 30 Aug 14:08`. The hidden title
marker is normalized and bounded to 72 characters; a rename failure is only a
warning and does not stop the task. On macOS, `donext` sends the thread's
`codex://threads/<id>` deep link to Codex Desktop in the background. This lets
Desktop navigate to the persisted thread when the GUI can load it. A missing
Desktop URL handler produces a warning but does not stop the roadmap run.
Codex Desktop may not list a thread while the separate `donext` App Server still
has it loaded. `donext` therefore closes that server after every goal and starts
a fresh one for the next goal, releasing completed sessions to Desktop during a
continuous run. The currently active session can still remain absent until its
turn finishes and that goal's server closes.
In continuous mode, a standalone `DONEXT_NO_WORK` final response starts one
fresh independent session with the normal prompt. The loop stops only after two
consecutive sessions report `DONEXT_NO_WORK`; any completed goal resets that
confirmation. `--once` still stops after its single session. The loop also stops
on failure, interruption, an interactive request, a standalone `DONEXT_BLOCKED`
final response, or the weekly usage budget. Task selection, scope, verification,
recovery, Git, commit, and working-directory policies belong to each project's
own instructions; donext does not inject them. A blocked response is reserved
for work that needs external intervention after local options are exhausted.
It records `blocked`, returns a nonzero exit code, and never starts another
goal. Failures and
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
before App Server starts. Without the flag, the built-in English session
exit-code prompt is used. The same exit-code contract is appended to every
custom prompt. It does not select goals or impose task, verification, recovery,
Git, path, or commit policy. `DONEXT_NO_WORK` is a successful session exit only
after the current task is complete and the available project plan has no actionable work.
`DONEXT_BLOCKED` is a blocked session exit only when progress is impossible
without external intervention. The marker must be the final line of the final
response; ordinary failures must be investigated while reasonable local action
remains. In continuous mode, `DONEXT_NO_WORK` is independently confirmed in a
fresh thread before termination. Prompts are never stored in `.donext` or
lifecycle logs.

### Permissions and interactive requests

Every `thread/start` explicitly receives safe defaults:

- `--approval-policy never` (also: `on-request`, `untrusted`);
- `--sandbox workspace-write` (also: `read-only`, `danger-full-access`).

Each goal uses a fresh `codex app-server --stdio` process started from `PATH`.
Approval and user-input requests are rejected, the active turn is interrupted,
and unattended orchestration exits with an error rather than answering on the
user's behalf.

### Codex Desktop projects

Every `thread/start` receives the canonical repository path as `cwd`. Current
Codex Desktop uses that persisted working directory to group the chat with its
project; `donext` does not query or modify Desktop's separate project list and
does not send the obsolete experimental `projectId` field.

Codex Desktop owns a private stdio App Server process rather than publishing its
transport socket. Consequently, `donext` cannot attach to that exact process.
It runs its own documented App Server client and uses the Desktop deep link to
request navigation to each new thread. Because Desktop may defer loading a
thread owned by another App Server, `donext` restarts its server between goals;
completed sessions can then appear without waiting for the whole continuous run
to exit. The separately managed `codex app-server daemon` control socket is not
the App Server process owned by the GUI.

### Weekly usage budget

`--weekly-usage-budget N`, where `N` is 1 through 100, allows the current process
to consume at most `N` percentage points of the weekly quota. Before the first
goal, the CLI records the single 10,080-minute window as its baseline. Before
later goals, it compares current usage with that baseline. Reaching the budget
stops successfully before another thread is created.

The active turn is never interrupted, so a completed goal can overshoot the
budget. After each completed session, normal output prints only the percentage
remaining from the budget configured at process launch, for example
`16:34:24.259 % run budget remaining: 60.0% of configured 10% weekly quota`.
The dedicated `%` marker denotes a session boundary budget update. `-v` or
`--verbose` additionally prints the
baseline, current weekly usage, consumed budget, configured budget, raw remaining
points, and terminal session metadata. Missing, ambiguous, reset, or anomalous
weekly-window data fails closed.

### Live session output and token usage

While a goal runs, `donext` prints each completed model message to the terminal
with every line prefixed by compact local time and `>`, for example
`15:27:41.083 >`. The `HH:MM:SS.mmm` timestamp includes milliseconds without
repeating the date on every live line. The prompt and session lifecycle are hidden and
responses have no label by default. Live activity always uses distinct markers:
`?` for available reasoning summaries, `$` for command launches, and `~` for
files changed by patch operations. Responses use `>` lines. Requests are visible
only with `-v` or `--verbose` and use `<` lines; neither direction has a text
label. Verbose session lifecycle uses `#`. App Server
does not expose the model's private chain of thought; the reasoning output is
the summary supplied by the protocol. Terminal session metadata and token/context
diagnostics are hidden in normal output and available with `-v` or `--verbose`;
verbose system statistics use the `=` marker. A configured weekly budget is the
only normal completion summary and uses a timestamped `%` line showing the
remaining percentage of that launch budget. Raw App Server diagnostics are also
suppressed in normal output and available in verbose mode; actionable errors
reported by `donext` remain visible in both modes. Model messages and live
activity are terminal-only and are never copied into state or lifecycle logs.
Standalone orchestrator control markers are hidden from the terminal model
output and are consumed only by the final-response handler.
If removing those markers leaves no visible response, the CLI prints
`> [response completed with no visible output]` to confirm that the model turn
did finish.

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
