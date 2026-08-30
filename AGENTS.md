# AGENTS.md

## Project purpose

This repository contains a local Codex orchestrator: a small CLI that executes
project roadmaps sequentially through Codex App Server. Every task must run in a
separate persisted Codex thread.

The orchestrator only manages project selection and the thread/turn lifecycle.
The project files, its `AGENTS.md`, its roadmap, and the standard Codex session
history remain the sources of truth.

## Task source

- Current tasks are listed in `ROADMAP.md`.
- Read all of `ROADMAP.md`, including its maintenance rules, before starting.
- Unless the user specifies another task, select the first unfinished item under
  "Current steps."
- Complete exactly one roadmap step per work cycle.
- Do not start the next step after completing the selected one.
- If a task is too large or reveals mandatory follow-up work, record its
  decomposition or a new roadmap item first.

## Completion criteria

A task is complete only when implementation, tests, relevant checks, required
documentation, and repository consistency are all complete, and the item has
been moved from "Current steps" to "Step history" in `ROADMAP.md`.

If work is blocked or checks fail, leave the step under "Current steps" and add
a concise nested `Status` entry describing the state and blocker.

## Architecture rules

- Do not implement a custom LLM client or call the Responses API directly
  without a separate architecture decision.
- Use the installed `codex app-server` and the existing Codex authentication.
- Never assume App Server protocol details from memory. Verify the installed
  version and isolate protocol-specific code in one adapter.
- Start every managed roadmap goal in a new thread; never reuse a completed one.
- Do not parse managed-project roadmaps unnecessarily. Codex determines the next
  task from the canonical project roadmap.
- Do not store transcripts or duplicate conversation history. Persist only the
  minimum lifecycle metadata.
- Do not run destructive Git operations such as `reset`, `clean`, or reverting
  user changes.
- A dirty worktree is not an automatic blocker, but it must be reported.
- Always create a Git commit after successfully completing a task. Do not commit
  partial or blocked work unless the user explicitly requests it.
- Keep the architecture small; do not add a daemon, Web UI, scheduler, or custom
  task database to the MVP.

## Implementation quality

- The App Server client must be replaceable with a fake or mock.
- Determine turn completion from protocol events, not a timeout or CLI stdout.
- Ignore or briefly log unknown protocol notifications without terminating.
- Write state atomically.
- Locks must be independent between projects.
- Errors, interruptions, and interactive requests must never start the next goal.
- Orchestrator logs must not copy the complete Codex transcript.

## Checks

- Run focused tests for changed packages first, then all relevant tests.
- Normal unit and integration tests must not run real Codex or consume user quota.
- Tests using a real App Server must be explicitly opt-in and documented.
