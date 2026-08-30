# ROADMAP

## Goal

Build a local, stateless `donext` CLI on top of Codex App Server. From the
current directory, it executes a project's roadmap sequentially. Every goal uses
a new persisted Codex thread, and the next goal starts only after the previous
one succeeds.

## Roadmap maintenance rules

1. "Current steps" is the source of the next task.
2. By default, select the first unfinished step from top to bottom.
3. Complete exactly one step in each Codex thread.
4. A step is complete only after implementation, testing, and required
   documentation updates.
5. Remove a completed step from "Current steps" and append it to "Step history."
6. History entries preserve the original ID, title, completion date, concise
   result, and checks performed.
7. Keep unfinished, partial, or blocked steps under "Current steps" and add a
   nested `Status` entry.
8. Insert mandatory new work in the logically correct position, not
   automatically at the end.
9. Step IDs remain stable after implementation begins.
10. Never move a step to history while relevant checks fail or remain unknown.
11. Read `AGENTS.md` and this entire file before starting a step.

## MVP scope

The MVP includes `donext` from the current directory; `--once`, `--dry-run`,
`--prompt TEXT|@FILE|-`, and `--weekly-usage-budget N`; `donext status`; one App
Server per process; one persisted thread per goal; real terminal-event waiting;
stopping on no-work, failure, or interruption; independent project locks;
state/recovery; tests; and documentation.

The MVP excludes a Web UI, daemon/service, scheduler, remote orchestration,
custom task database, and forced Git commit management.

## Current steps

## Deferred improvements

These are outside the MVP until explicitly moved into "Current steps":

- interactive project selector;
- rename a managed thread after goal discovery to a task-aware title such as
  `BC-02 · Funding authority · 30 Aug 14:08`;
- defensive `--max-goals`;
- additional transport implementations;
- packaging and release automation.

## Step history

### ORCH-027 — Shorten managed thread names

- Completed: 2026-08-30.
- Result: managed Desktop titles now use the compact local format
  `30 Aug 14:08 · next roadmap step`, omitting redundant CLI/project prefixes,
  year, seconds, and UTC offset. Task-aware post-discovery naming remains a
  documented deferred candidate. Updated README and changelog. Checks: focused
  CLI race tests; full race tests; full tests; vet; build; and diff check.

### ORCH-026 — Reveal managed threads live in Codex Desktop

- Completed: 2026-08-30.
- Result: removed the obsolete empty `project/list`/`projectId` path and now
  relies on canonical `cwd`, which current Desktop uses for project grouping.
  On macOS each named thread is revealed through `codex://threads/<id>` before
  its turn starts; Desktop failures remain non-blocking. Verified that the GUI's
  private stdio App Server has no attachable control socket and documented the
  supported deep-link integration. Checks: focused Codex/CLI race tests; full
  race tests; full tests; vet; build; diff check; installed-schema and live
  protocol probes; real no-work smoke; and Desktop list confirmation while the
  smoke turn was active.

### ORCH-025 — Require repair after failed verification

- Completed: 2026-08-30.
- Result: every built-in and custom prompt now includes a completion contract
  requiring Codex to diagnose, repair, and rerun fixable linter, test, coverage,
  profiler, and log failures. `ORCHESTRATOR_BLOCKED` is reserved for external
  blockers the active thread cannot resolve. Updated README and changelog.
  Checks: focused CLI race tests; full race tests; full tests; vet; build; manual
  custom-prompt dry run; and diff check.

### ORCH-024 — Confirm marker-only model completion in live output

- Completed: 2026-08-30.
- Result: marker-only final responses now print an explicit no-visible-output
  completion line while control markers stay hidden and normal model responses
  remain unchanged. Updated README and changelog. Checks: focused CLI race tests;
  full race tests; full tests; vet; build; and diff check.

### ORCH-023 — List permission values in CLI help

- Completed: 2026-08-30.
- Result: `donext --help` now lists every supported approval policy and sandbox
  mode with their defaults, and the README and changelog point users to the
  discoverable CLI reference. Checks: focused CLI race tests; manual help
  output; full race tests; full tests; vet; build; and diff check.

### ORCH-022 — Hide control markers from live model output

- Completed: 2026-08-30.
- Result: standalone no-work and blocked markers remain available to the final
  response handler but are removed from terminal model output. Marker-only
  responses no longer produce empty response blocks; prose containing marker
  text remains unchanged. Updated README and changelog. Checks: focused CLI race
  tests; full race tests; full tests; vet; build; and diff check.

### ORCH-021 — Stop cleanly when a managed goal is blocked

- Completed: 2026-08-30.
- Result: added the standalone `ORCHESTRATOR_BLOCKED` final-response contract;
  blocked state and terminal output; a metadata-only `event=blocked` lifecycle
  record; nonzero exit; and continuous-loop termination. The default prompt,
  README, and changelog document environment, permission, gate, and user-action
  blockers. Checks: focused CLI race tests; full race tests; full tests; vet;
  build; and diff check.

### ORCH-020 — Timestamp managed thread names

- Completed: 2026-08-30.
- Result: managed Codex thread names now include the local launch date, time,
  and UTC offset, with deterministic coverage and a documented example. Checks:
  focused CLI race tests; full race tests; full tests; vet; build; and diff check.

### ORCH-019 — Show live session progress and usage statistics

- Completed: 2026-08-30.
- Result: added live thread/turn start messages and terminal-only completed model
  responses; routed `thread/tokenUsage/updated`; and printed post-session token,
  context-window, and configured weekly-budget statistics. Lifecycle files remain
  metadata-only. Updated README, protocol notes, and changelog. Checks: focused
  Codex/CLI race tests; full race tests; full tests; vet; build; and diff check.

### ORCH-018 — Negotiate the experimental project API capability

- Completed: 2026-08-30.
- Result: added `capabilities.experimentalApi: true` to the App Server
  initialization handshake so current Codex versions accept `project/list`.
  Updated the protocol documentation and changelog. Checks: focused adapter
  race test; full race tests; full tests; vet; build; and diff check.

### ORCH-014 — Replace an absolute threshold with a per-run weekly budget

- Completed: 2026-08-30.
- Result: replaced `--max-used-percent` with `--weekly-usage-budget N`. The
  adapter retains duration/reset metadata, selects the single 10,080-minute
  window, records a baseline, and stops before a new thread when consumed delta
  reaches the budget. Missing, ambiguous, reset, and anomalous data fail closed.
  Checks: repeated race tests for Codex/CLI; full tests; vet; build; diff check.

### ORCH-001 — Verify the installed Codex protocol and persistence

- Completed: 2026-08-30.
- Result: verified NDJSON framing, handshake, persisted thread/turn lifecycle,
  naming, terminal events, restart persistence, and Desktop navigation against
  `codex-cli 0.149.0-alpha.4.3`. Documented the spike and GUI limitations.

### ORCH-002 — Initialize the Go CLI and project configuration

- Completed: 2026-08-30.
- Result: created the Go module, initial orchestrator command, strict YAML
  configuration, project validation, prompt loading, and dry-run. Checks: package
  and full tests, vet, build, and manual dry-run. Later superseded by ORCH-012.

### ORCH-003 — Implement the minimal App Server adapter

- Completed: 2026-08-30.
- Result: added a mockable domain client and isolated stdio adapter with framing,
  handshake, concurrent response correlation, thread/turn lifecycle, interrupt,
  server requests, unknown-notification compatibility, and process errors.
  Checks: repeated race tests, full tests, vet, and build.

### ORCH-004 — Implement one complete `--once` run

- Completed: 2026-08-30.
- Result: implemented Git-state reporting, one App Server, a new named persisted
  thread, one prompt, terminal status classification, and final-message-only
  no-work recognition. Checks: repeated race tests, full tests, vet, and build.

### ORCH-005 — Add continuous execution

- Completed: 2026-08-30.
- Result: added a loop that reuses one App Server but creates a new thread per
  successful goal, stopping on no-work, failure, or interruption. Checks:
  repeated race tests, full tests, vet, and build.

### ORCH-006 — Implement state, locking, and recovery

- Completed: 2026-08-30.
- Result: added minimal transcript-free JSON state, atomic writes, independent
  OS locks, stale-run recovery, and new-thread restart behavior. Checks: repeated
  race tests for state/CLI, full tests, vet, and build.

### ORCH-007 — Handle interrupts and interactive requests correctly

- Completed: 2026-08-30.
- Result: the first signal interrupts and waits for a terminal event; a second
  signal or grace timeout force-closes App Server. Approval and user-input
  requests are rejected and stop unattended execution. Checks: generated schema,
  repeated race tests, full tests, vet, and build.

### ORCH-008 — Implement status and lifecycle logging

- Completed: 2026-08-30.
- Result: added status output with repository, lock, turn, and timestamp; stale
  state detection; and metadata-only lifecycle logs without prompts, reasoning,
  transcripts, or command output. Checks: repeated race tests, full tests, vet,
  and build.

### ORCH-009 — Complete MVP documentation and verification

- Completed: 2026-08-30.
- Result: documented requirements, installation, configuration, commands, stop
  conditions, state, locks, recovery, dirty/non-Git behavior, sessions, GUI
  limits, and opt-in smoke testing. A real isolated App Server run completed and
  persisted successfully. Checks: full tests, vet, build, and real smoke test.

### ORCH-010 — Limit continuous runs by account usage

- Completed: 2026-08-30.
- Result: added the initial `--max-used-percent` account rate-limit guard with
  fail-closed behavior and no active-turn interruption. Checks: repeated race
  tests, full tests, vet, build, and diff check. Superseded by ORCH-014.

### ORCH-011 — Identify projects and store runtime metadata by current root

- Completed: 2026-08-30.
- Result: added canonical Git/non-Git root resolution with symlink handling,
  display names, stable path hashes, current-directory status, and isolated
  state/log/lock layout. Checks: repeated race tests, full tests, vet, build, and
  diff check.

### ORCH-011A — Store runtime metadata inside each project

- Completed: 2026-08-30.
- Result: moved state, logs, and locks to `<canonical-root>/.donext`, ignored the
  previous global layout, and excluded `.donext` from dirty-state checks. Checks:
  repeated race tests, full tests, vet, build, and diff check.

### ORCH-012 — Make `donext` stateless and remove user configuration

- Completed: 2026-08-30.
- Result: renamed the binary to `donext`, switched to current-directory execution,
  retained only `status`, removed YAML/project registries, and added validated
  approval/sandbox flags and concrete dry-run output. Checks: repeated race tests,
  full tests, vet, build, diff check, and manual CLI checks.

### ORCH-013 — Unify prompt sources under `--prompt`

- Completed: 2026-08-30.
- Result: added literal, `@FILE`, and redirected-stdin prompt sources with early
  validation and a built-in roadmap prompt. Prompts remain absent from state and
  logs. Checks: repeated race tests, full tests, vet, build, and diff check.

### ORCH-015 — Complete documentation and end-to-end checks for the new CLI

- Completed: 2026-08-30.
- Result: rewrote documentation for `cd project && donext`, stateless behavior,
  `AGENTS.md`, prompt sources, project-local state, status, stopping, and weekly
  budget. Fixed `--help` exit status and manually checked the CLI matrix. Checks:
  repeated race tests, full tests, vet, build, and diff check.

### ORCH-016 — Associate threads with Codex Desktop projects

- Completed: 2026-08-30.
- Result: added paginated `project/list`; canonical-root matching with symlink
  resolution; and `projectId` in each `thread/start`. No-match runs remain
  unassigned with a warning; protocol errors and ambiguity stop before thread
  creation. Updated README and protocol spike. Checks: repeated race tests for
  Codex/CLI, full tests, vet, build, and diff check.

### ORCH-017 — Prepare the repository for GitHub publication

- Completed: 2026-08-30.
- Result: added the MIT License, Keep a Changelog-compatible changelog,
  contribution and security policies, GitHub Actions CI, a pull request template,
  and Dependabot configuration. Rewrote all Markdown documentation in English
  and expanded README with verified Codex prerequisites, `go install`, PATH
  troubleshooting, quick start, limitations, and project links. An isolated
  install produced a working standalone binary, so no redundant `install.sh`
  was added. Checks: isolated `go install`; tests, vet, and build under Go
  1.23.12; `go test -race ./...`; full tests; vet; build; diff check; and a
  repository-wide Cyrillic scan of Markdown files.
