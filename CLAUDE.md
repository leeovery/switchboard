# CLAUDE.md

Guidance for Claude Code working in this repository.

Switchboard is a local proxy that spreads Claude Code sessions across several Claude subscriptions,
plus a terminal dashboard of their usage. The design, including what gets built when, is
`docs/design.md`.

## Gates

Run all of these before reporting work done. Each must pass clean.

```bash
gofmt -l .               # must print nothing
go vet ./...
scripts/test-isolated    # every test, race detector on, isolated: see Test isolation
golangci-lint run        # standard linters plus modernize (.golangci.yml)
go build ./...
```

## House rules

- **Go 1.27, standard library first.** A new dependency needs a stated reason in its PR. The
  expected ones: `spf13/cobra` for commands, `charm.land/bubbletea/v2` and
  `charm.land/lipgloss/v2` for the dashboard, and a TOML parser for config. No helper or utility
  libraries: no `samber/*`, no `testify`, no `lo`.
- **Layout:** `cmd/switchboard` holds `main`. Everything else lives under `internal/`, one package
  per concern.
- **Tests:** the standard `testing` package, table-driven where cases differ only by data, and
  isolated as Test isolation says.
- **Errors:** wrap with `fmt.Errorf("…: %w", err)`. Handle an error once: log it or return it,
  never both.
- **Logging:** `log/slog`.
- **Comments are rare and earned:** only what the code can't say, such as a non-obvious constraint,
  or something that looks wrong but was proven right (say so). Never narration.
- **Nothing personal, ever**, in code, tests, fixtures, docs or commit messages: no real account
  emails or labels, tokens, env-file paths or machine names. Use placeholders such as `work`,
  `personal` and `CLAUDE_TOKEN_WORK`. The repo will go public, and its history goes with it.
- **Never log a token.** Redact `Authorization` and anything token-shaped in logs, errors and test
  output.

## Test isolation

No test touches the real network, environment, home directory, config, state, binaries or
notifications. Ever.

- **Inject** what a test needs: fake upstreams with `net/http/httptest`, and runners, clocks,
  probers, a `getenv` and a home directory of its own. Write files under `t.TempDir()` or
  `os.MkdirTemp`; set variables with `t.Setenv`.
- **Every package with tests** runs them through `internal/testguard`:
  `func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }`. It points `HOME` and XDG's
  directories into a throwaway root; clears the `SWITCHBOARD_`, `CLAUDE_` and `ANTHROPIC_`
  variables, tmux's and proxies'; puts only stubs of `claude`, `osascript`, `launchctl`, `tmux`
  and `open` on `PATH`; and lets `http.DefaultTransport`, and transports cloned from it, dial
  loopback alone. A run in which a stub ran, a dial was blocked, the real switchboard config
  changed, or its state directory appeared fails, even when every test passed. Its own tests fail
  a package without that `TestMain`, a change to the environment anywhere but `testguard`, a
  process started outside the runners its allow-list names, and production code that imports
  `os/user`, as `HomeDir` is injected, or reads the environment as its package initialises, before
  `TestMain` runs.
- **`scripts/test-isolated` is the test gate:** every test, race detector on, inside a macOS
  sandbox (`scripts/isolation.sb`) that denies the network beyond loopback, writes into the home
  directory but Go's caches, and running the real `claude`, `osascript`, `launchctl`, `tmux` and
  `open`. Each run first proves the sandbox denies a dial off the machine, a write into the home
  directory and running `osascript`; `--self-check` does only that.
- **Never loosen a guard to make a test pass.** A test that needs what a guard blocks is a finding:
  inject it instead.
