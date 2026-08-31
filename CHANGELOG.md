# Changelog

All notable changes to this project will be documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project intends to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
after its first release.

## [Unreleased]

### Changed

- Retry `serverOverloaded` and compatible model-capacity turn failures in the
  same persisted session after 10 seconds, with a timestamped system log,
  instead of stopping the managed loop.
- Continue managed loops across weekly account rate-limit rollovers by resetting
  the per-run usage baseline to the new window; other anomalous window changes
  still fail closed.
- Focus normal session-boundary output on one timestamped `%` line containing
  the percentage remaining from the weekly budget configured at launch and the
  configured share of weekly quota itself. Move session metadata and
  token/context diagnostics to `-v`/`--verbose`.
- Start sessions with a temporary discovery name, then rename them from a
  hidden, normalized task-title marker emitted after Codex identifies the task.
- Suppress raw App Server stderr in normal output while retaining it with
  `-v`/`--verbose`; actionable errors emitted by `donext` remain visible.
- Replace verbose request/response text labels with directional `<` and `>`
  markers. Mark system statistics with `=` and compact token/context and weekly
  usage into one line outside verbose mode.
- Show reasoning summaries, command launches, and changed paths in normal output
  as well as verbose output; lifecycle progress remains verbose-only.
- Hide session/thread/turn lifecycle progress by default. Verbose output now
  marks lifecycle (`#`), reasoning summaries (`?`), command launches (`$`),
  file changes (`~`), and model text (`>`) while keeping activity terminal-only.
- Present terminal markers to Codex as session exit codes, require them on the
  final line only, and explicitly treat fixable execution failures as nonterminal.
- Limit the built-in and appended prompt to exceptional terminal-marker rules;
  goal selection, scope, checks, recovery, Git, path, and commit policies now
  come exclusively from the user and managed project.
- Rename stop markers to `DONEXT_NO_WORK` and `DONEXT_BLOCKED`; the former means
  that no further project plan exists, while the latter is reserved for goals
  that require external intervention after local options are exhausted.
- Make stop markers a last resort: Codex now consults all applicable project
  instructions before using `DONEXT_NO_WORK`, and uses
  `DONEXT_BLOCKED` only when a known goal requires external intervention. The
  built-in task and orchestration contract are entirely in English.
- Hide model requests and remove the `model response:` label in normal output;
  prefix response lines with `>` and add `-v`/`--verbose` labeled request and
  response blocks.

### Fixed

- Require two consecutive `DONEXT_NO_WORK` results from independent sessions
  before a continuous run stops, and reset the confirmation after completed
  work. This prevents one contradictory no-work marker from ending a roadmap.
- Restart App Server between continuous-run goals so completed persisted
  sessions are released to Codex Desktop before the entire `donext` run exits.
- Shorten managed Desktop thread titles to the readable local format
  `30 Aug 14:08 · next roadmap step` without redundant project prefixes.
- Reveal each newly created thread in Codex Desktop on macOS before its turn
  starts, and rely on canonical `cwd` grouping instead of the now-empty
  experimental `project/list`/`projectId` path.

### Added

- A timestamped `=` system-status line explaining why the managed goal loop
  stopped.
- Compact local `HH:MM:SS.mmm` timestamps on every marked live activity line.
- Complete approval-policy and sandbox value lists in `donext --help`.
- Live thread/turn progress, terminal-only model responses, and post-session
  token, context-window, and weekly-budget statistics.
- Local launch timestamps with UTC offsets in managed Codex thread names.
- Explicit `DONEXT_BLOCKED` handling that records blocked state and stops
  continuous execution after an incomplete goal.
- Control markers are consumed internally instead of appearing in live model
  output.
- Explicit terminal confirmation when a completed model response contains only
  hidden control markers.
- Stateless `donext` CLI that runs the current project's roadmap through Codex
  App Server.
- Continuous and single-step execution, dry-run mode, custom prompts, and a
  weekly usage budget.
- Persisted thread lifecycle handling, project-local state, independent locking,
  interruption, recovery, metadata-only logs, and `donext status`.
- Automated tests using fake and in-memory App Server implementations.
- GitHub Actions CI, MIT License, contribution guidelines, and security policy.

[Unreleased]: https://github.com/typomaker/donext/commits/main
