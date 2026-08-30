# Changelog

All notable changes to this project will be documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project intends to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
after its first release.

## [Unreleased]

### Added

- Stateless `donext` CLI that runs the current project's roadmap through Codex
  App Server.
- Continuous and single-step execution, dry-run mode, custom prompts, and a
  weekly usage budget.
- Persisted thread lifecycle handling, project-local state, independent locking,
  interruption, recovery, metadata-only logs, and `donext status`.
- Codex Desktop project association through `project/list` and
  `thread/start.projectId`.
- Automated tests using fake and in-memory App Server implementations.
- GitHub Actions CI, MIT License, contribution guidelines, and security policy.

[Unreleased]: https://github.com/albertsultanov/donext/commits/main
