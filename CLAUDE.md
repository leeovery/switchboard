# CLAUDE.md

Guidance for Claude Code working in this repository.

Switchboard is a local router for Claude Code. It spreads sessions across several Claude
subscriptions, records every request, and shows where the usage stands, in a terminal dashboard, in
verbs that print JSON for agents, and through a skill that tells Claude about it.

- `docs/design.md` is the design: what's built, why, the decided calls, and the backlog's index.
- `README.md` is the front page, and `docs/` the user's guide beside the design: the dashboard, how it works, commands, configuration, reference and troubleshooting. `ideas/` is the backlog, one idea a file.

## How it fits together

- **The request path:** `claude`, linked to switchboard, starts the real Claude Code pointed at the
  router (`launch`). The `router` picks each request's account (`score`), swaps in its token
  (`tokens`), replays a request a limit refuses on another account, and reads every answer's usage.
  What's Claude's own, the paths, headers, model families and probes, sits in `claude`, behind the
  router's seams. The router restarts in place across upgrades (`handover`).
- **What it records,** each a file a day (`dayfile`) in the state directory: every window reading
  that changed (`readings`), every request without its content, and each day's summary
  (`ledger`), and the router's events (`events`). Logs go to `router.log` and `cli.log` (`logs`).
- **Reading it back:** `status` builds the status document and its `Moment`, the one judgement of
  every account at a moment. `views` builds each view's data from that, the router's sessions and
  request stream, and the recorded files: one function a view, read alike by the dashboard and the
  verbs. `prose` words every plain value.
- **Showing it:** `dashboard` draws (Lip Gloss), `dashboard/watch` runs it (Bubble Tea), and
  `theme` colours it. `cli` holds the commands, kept thin. `setup`, `accounts`, `service` and
  `skill` install and manage.
- **One owner per fact.** Each fact is worked out once, in `status` or `views`, and every tab and
  verb reads it there, never again in a drawing package or a command. The guards hold it
  (`status/guard_test.go`, `views/guard_test.go`, `prose/guard_test.go`, built on
  `internal/sourcecheck`). A guard that fails is telling you where the fact belongs.

## Gates

Run all of these before reporting work done. Each must pass clean. `scripts/gate` runs them all, and
records a clean tree that passes: run it before merging or pushing to main, so a release finds the
record and doesn't run them again (`scripts/gated`, mint's `pre_tag`).

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
  `charm.land/lipgloss/v2` for the dashboard, with `charmbracelet/colorprofile`,
  `charmbracelet/x/ansi` and `charmbracelet/x/term`; and `BurntSushi/toml` for config. No helper
  or utility libraries: no `samber/*`, no `testify`, no `lo`.
- **Layout:** `cmd/switchboard` holds `main`. Everything else lives under `internal/`, one package
  per concern.
- **Tests:** the standard `testing` package, table-driven where cases differ only by data, and
  isolated as Test isolation says.
- **Errors:** wrap with `fmt.Errorf("…: %w", err)`. Handle an error once: log it or return it,
  never both.
- **Logging:** `log/slog`, through `internal/logs`.
- **Comments are rare and earned:** only what the code can't say, such as a non-obvious constraint,
  or something that looks wrong but was proven right (say so). Never narration.
- **Nothing personal, ever**, in code, tests, fixtures, docs or commit messages: no real account
  emails or labels, tokens, token file paths or machine names. Use placeholders such as `work`,
  `personal` and `test-token-work`. The repo is public, and its history with it.
- **Never log a token.** Redact `Authorization` and anything token-shaped in logs, errors and test
  output (`internal/redact`).
- **Hands off the real install.** Working on the repo, never read, copy or write the real config,
  state directory, ledger, readings or tokens, and never run the real router, dashboard or
  `claude` to see what changed: use the capture harness below.

## Test isolation

No test touches the real network, environment, home directory, config, state, binaries or
notifications. Ever.

- **Inject** what a test needs: fake upstreams with `net/http/httptest`, and runners, clocks,
  probers, a `getenv`, a home directory, a state directory with its token files, switchboard's
  bin directory, Claude Code's config directory and a `PATH`, of its own. Write files under
  `t.TempDir()`; set variables with `t.Setenv`.
- **Every package with tests** runs them through `internal/testguard`, in a `TestMain` of that one
  statement: `func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }`. It gives the tests a
  throwaway home, clears the `SWITCHBOARD_`, `CLAUDE_` and `ANTHROPIC_` variables, puts only stubs
  of `claude`, `osascript`, `launchctl`, `tmux` and `open` on `PATH`, and lets transports dial
  loopback alone. It fails the run, even when every test passed, where a stub ran, a dial was
  blocked, or anything real of switchboard's or Claude Code's changed.
- **testguard's own tests read the module's source** and fail on: a package with tests but without
  that `TestMain`; a change to the environment outside `testguard`; a process started outside its
  allow-list; an `http.Transport` not cloned from `http.DefaultTransport`; `os/user`; or the
  environment read as a package initialises.
- **`scripts/test-isolated` is the test gate:** every test, race detector on, inside a macOS sandbox
  (`scripts/isolation.sb`) that denies the network beyond loopback, the router's port 4747, writes
  into the home and the real install, and running the real programs. Each run first proves the
  sandbox holds (`--self-check` does only that). Without `sandbox-exec` on macOS, the tests don't
  run.
- **The full lists** of what each guard watches and denies are in `internal/testguard` and
  `scripts/`: read them before changing either.
- **A known false failure:** Claude Code's updater re-creates `~/.local/bin/claude` during long
  runs, and testguard then fails a run whose tests all passed. If that link is its only complaint
  and there's no `--- FAIL`, the run passed.
- **Never loosen a guard to make a test pass.** A test that needs what a guard blocks is a finding:
  inject it instead.

## The dashboard, by fixture

`cmd/capturetool`, a program of its own and never a switchboard command, draws a named fixture of
the real dashboard, built with every seam faked: it never dials the router, probes, or touches the
real install. `go run ./cmd/capturetool --fixture <name> --print --size 160x37` prints one as text
(`--ansi` with its colour, `--theme` in another theme); `--help` lists the fixtures.

- **The frames** in `testdata/vhs/reference/` are the live view as the owner last signed it off.
  capture's tests hold each fixture named after a frame to it, cell for cell, passing over nothing,
  and hold every view to its rows at every size; the dashboard package's tests hold its rules,
  never the frames.
- **A change that alters a screen** shows the owner its prints first (`--print`), then, once he
  signs them off, regenerates its frames in the same change:
  `go run ./cmd/capturetool --frames testdata/vhs/reference [names…]`, every frame without names.
  Never edit a frame by hand.
- **The README's demos:** `--scenario` plays a scripted scenario in real time; their tapes are in
  `demo/` and their art in `art/`, as `demo/README.md` says.
- **Scaffolding:** the `vhs` tapes and captures in `testdata/vhs/` are for judging a milestone by
  eye, and are cleared once it's signed off.
