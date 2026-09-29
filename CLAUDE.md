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
  expected ones: `spf13/cobra` for commands; `charm.land/bubbletea/v2` and
  `charm.land/lipgloss/v2` for the dashboard, with `charmbracelet/colorprofile` to bring its
  colour down to what the terminal shows, `charmbracelet/x/ansi` to measure and cut styled text,
  and `charmbracelet/x/term` to tell a terminal and its size; and `BurntSushi/toml` for config.
  No helper or utility libraries: no `samber/*`, no `testify`, no `lo`.
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
  emails or labels, tokens, token file paths or machine names. Use placeholders such as `work`,
  `personal` and `test-token-work`. The repo will go public, and its history goes with it.
- **Never log a token.** Redact `Authorization` and anything token-shaped in logs, errors and test
  output.

## Test isolation

No test touches the real network, environment, home directory, config, state, binaries or
notifications. Ever.

- **Inject** what a test needs: fake upstreams with `net/http/httptest`, and runners, clocks,
  probers, a `getenv`, a home directory, a state directory with its token files, switchboard's
  bin directory, Claude Code's config directory and a `PATH`, of its own. Write files under
  `t.TempDir()` or `os.MkdirTemp`; set variables with `t.Setenv`.
- **Every package with tests** runs them through `internal/testguard`, in a `TestMain` of that one
  statement: `func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }`. Before the tests, it:
  - points `HOME` and XDG's directories into a throwaway root;
  - clears the `SWITCHBOARD_`, `CLAUDE_` and `ANTHROPIC_` variables, tmux's and proxies';
  - puts only stubs of `claude`, `osascript`, `launchctl`, `tmux` and `open` on `PATH`;
  - lets `http.DefaultTransport`, and transports cloned from it, dial loopback, and unix
    sockets in the temporary directory, alone: never a live router's, in the real state
    directory.
- **testguard fails the run, even when every test passed,** when:
  - a stub ran, or a dial was blocked;
  - the real switchboard config changed, or its state directory appeared: in the home, or where
    `SWITCHBOARD_CONFIG`, `XDG_CONFIG_HOME` and `XDG_STATE_HOME` put them as the run began,
    links resolved;
  - a switchboard file in the real `~/Library/LaunchAgents` appeared or changed;
  - switchboard's skill in Claude Code's real config directory appeared or changed: in
    `~/.claude`, or where `CLAUDE_CONFIG_DIR` put it as the run began, links resolved;
  - switchboard's real bin directory, which holds its `claude` link, appeared or changed: in
    `~/.local/share/switchboard/bin`, or where `XDG_DATA_HOME` put it as the run began, links
    resolved;
  - anything named `claude`, a link or not, appeared or changed in a directory on the real
    `PATH` as the run began, links resolved.
- **testguard's own tests fail**, reading the module's source:
  - a package with tests but without that `TestMain`, or with anything else in it;
  - a change to the environment anywhere but `testguard`;
  - a process started outside the runners its allow-list names, an `exec.Cmd` made directly or
    `golang.org/x/sys/unix`'s `Exec` included;
  - an `http.Transport` made from scratch rather than cloned from `http.DefaultTransport`, as
    the guard sees no dial of its;
  - code, tests included but testguard's own, that imports `os/user`, as `HomeDir` is injected,
    or reads the environment as its package initialises, before `TestMain` runs.
- **`scripts/test-isolated` is the test gate:** every test, race detector on, inside a macOS
  sandbox (`scripts/isolation.sb`) that denies:
  - the network beyond loopback;
  - unix sockets outside the temporary directory, and in the real state directory;
  - the router's port, 4747, either way;
  - writes into the home directory, but Go's caches, and into the real config, state and bin
    directory, and Claude Code's config directory, wherever the environment puts them;
  - writes into the directories on `PATH` outside the home, such as `/opt/homebrew/bin`;
  - running the real `claude`, `osascript`, `launchctl`, `tmux` and `open`.
- **It unsets the variables testguard clears** before anything starts, as testguard clears them
  only once every package's init has run.
- **Each run first proves the sandbox holds**, and `--self-check` does only that:
  - it denies a dial off the machine, and a connect to a live unix socket outside the temporary
    directory;
  - it denies connecting to and binding 4747;
  - it denies a write into the home directory, and into the real config, state and bin
    directory, and Claude Code's config directory, where they exist;
  - it denies a write into each directory on `PATH` outside the home that the user can write to;
  - it denies running each of those programs that's installed, where `PATH` finds it, links
    resolved;
  - the tests start without those variables.
- **Without the sandbox:** on macOS, without `/usr/bin/sandbox-exec`, the tests don't run; off
  macOS, testguard alone guards them.
- **Never loosen a guard to make a test pass.** A test that needs what a guard blocks is a finding:
  inject it instead.
