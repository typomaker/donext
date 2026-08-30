# Changelog

All notable changes to this project will be documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project intends to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
after its first release.

## [Unreleased]

### Changed

- Make stop markers a last resort: Codex now consults all applicable project
  instructions before using `ORCHESTRATOR_NO_WORK`, and uses
  `ORCHESTRATOR_BLOCKED` only for a known goal with an unresolvable problem. The
  built-in task and orchestration contract are now entirely in English.
- Hide model requests and remove the `model response:` label in normal output;
  prefix response lines with `>` and add `-v`/`--verbose` labeled request and
  response blocks.

### Fixed

- Keep recovery of clearly unfinished prior-step work as the thread's sole goal
  and stop after its verified commit instead of continuing into the next item.
- Keep repository commands rooted consistently and forbid duplicating a chosen
  working-directory prefix in command paths.
- Shorten managed Desktop thread titles to the readable local format
  `30 Aug 14:08 · next roadmap step` without redundant project prefixes.
- Reveal each newly created thread in Codex Desktop on macOS before its turn
  starts, and rely on canonical `cwd` grouping instead of the now-empty
  experimental `project/list`/`projectId` path.
- Append a repair-and-retry completion contract to every prompt so fixable
  linter, test, coverage, profiler, and log failures do not stop as blocked.

### Added

- Complete approval-policy and sandbox value lists in `donext --help`.
- Live thread/turn progress, terminal-only model responses, and post-session
  token, context-window, and weekly-budget statistics.
- Local launch timestamps with UTC offsets in managed Codex thread names.
- Explicit `ORCHESTRATOR_BLOCKED` handling that records blocked state and stops
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
