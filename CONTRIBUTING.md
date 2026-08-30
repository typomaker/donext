# Contributing

Thank you for your interest in `donext`.

## Before making a change

- Open an issue before implementing a substantial new feature.
- Read `AGENTS.md` and `ROADMAP.md`; they define the architecture constraints and
  Definition of Done.
- Do not add tests that run real Codex or consume user quota. Real App Server
  tests must be explicitly opt-in.

## Development checks

Run these commands before opening a pull request:

```sh
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build ./cmd/donext
git diff --check
```

A pull request should explain why the change is needed, describe behavior before
and after it, and list the checks that were run.
