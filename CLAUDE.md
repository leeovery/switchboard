# CLAUDE.md

Guidance for Claude Code working in this repository.

Switchboard is a local proxy that spreads Claude Code sessions across several Claude subscriptions,
plus a terminal dashboard of their usage. The design, including what gets built when, is
`docs/design.md`.

## Gates

Run all of these before reporting work done. Each must pass clean.

```bash
gofmt -l .           # must print nothing
go vet ./...
go test -race ./...
golangci-lint run    # standard linters plus modernize (.golangci.yml)
go build ./...
```

## House rules

- **Go 1.27, standard library first.** A new dependency needs a stated reason in its PR. The
  expected ones: `spf13/cobra` for commands, `charm.land/bubbletea/v2` and
  `charm.land/lipgloss/v2` for the dashboard, and a TOML parser for config. No helper or utility
  libraries: no `samber/*`, no `testify`, no `lo`.
- **Layout:** `cmd/switchboard` holds `main`. Everything else lives under `internal/`, one package
  per concern.
- **Tests:** the standard `testing` package, table-driven where cases differ only by data. Unit
  tests never touch the network or the real environment: fake upstreams with `net/http/httptest`,
  set variables with `t.Setenv`, write files under `t.TempDir()`.
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
