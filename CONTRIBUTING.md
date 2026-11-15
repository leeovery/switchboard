# Contributing

Switchboard is written in Go, for macOS. Before you change anything, read two files:

- [CLAUDE.md](CLAUDE.md): the house rules, test isolation, and the rules for writing the docs.
- [The design](docs/design.md): what's built, why, and the backlog.

## The gates

Every change passes these gates before it merges:

```bash
# Must print nothing
gofmt -l .
go vet ./...
# Every test, in a sandbox
scripts/test-isolated
# The linters in .golangci.yml
golangci-lint run
go build ./...
```

While you work, run only the tests of what you changed, for example
`scripts/test-isolated ./internal/views/...`. The whole suite costs hours of CPU. `scripts/gate`
runs every gate on the whole module, and records a clean tree that passes.

## Tests stay isolated

- **No test touches the real system.** `scripts/test-isolated` runs every test in a macOS sandbox.
- **`internal/testguard` fails a run** that reaches the real network, home directory, config or
  binaries, even if every test passed.
- **Without `sandbox-exec`,** the tests don't run.

CLAUDE.md lists what each guard watches, and how to inject what a test needs.

## Screens and demos

The stills and frames are drawn from the real dashboard with every seam faked. A change that
alters a screen on purpose regenerates its frames in the same change. CLAUDE.md says how, and
[demo/README.md](demo/README.md) says how to record the demos.
