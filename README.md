# switchboard

Spread Claude Code sessions across several Claude subscriptions.

switchboard is a local proxy that sits between Claude Code and the Claude API. It uses each
account's quota right up to its limit, moves a session to another account when its account runs
out, and otherwise keeps every session on one account, so its prompt cache stays warm. It comes
with a terminal dashboard of every account's usage.

## How it works

Claude Code sends its requests to switchboard through `ANTHROPIC_BASE_URL`. For each message
request, switchboard picks an account, swaps in that account's token, and forwards the request
untouched; the rate-limit headers on every response tell it how much of each account's windows
(the 5-hour session, the week, and per-model weeks) is used, so it tracks usage without spending
requests. A session sticks to its account while its cache is warm. A new session, or one idle for
over an hour, goes to the account whose weekly quota is soonest lost unused. When an account hits
its limit, or refuses a request, switchboard replays the request on another account before Claude
Code sees anything, and the session moves there.

The full design is in [docs/design.md](docs/design.md).

## Requirements

- Go 1.27, to build it.
- Claude Code, and a long-lived setup token (`claude setup-token`) for each subscription.
- macOS, for the background service (a LaunchAgent). The rest runs anywhere Go does.

## Install

```sh
go install github.com/leeovery/switchboard/cmd/switchboard@latest
```

## Configure

Put your accounts in `~/.config/switchboard/config.toml` (or `$XDG_CONFIG_HOME/switchboard/`, or
wherever `$SWITCHBOARD_CONFIG` or `--config` says). Each names the environment variable that holds
its token; switchboard never stores a token itself.

```toml
[[account]]
id        = "work"
label     = "Work"
token_env = "CLAUDE_TOKEN_WORK"

[[account]]
id        = "personal"
label     = "Personal"
token_env = "CLAUDE_TOKEN_PERSONAL"

# Optional: which desktop notifications to post, shown with their defaults.
[notifications]
limits  = true   # an account hits a limit, and the sessions it moved
room    = true   # an account has room again
warning = 0.9    # a window passing this share of its limit; 0 turns it off
moves   = false  # every other session move
```

`switchboard accounts` lists them, and whether each token is set.

## Run the router

```sh
switchboard service install --env-file ~/path/to/tokens.env
```

This installs a LaunchAgent that starts the router now, at every login, and whenever it stops. A
LaunchAgent doesn't see your shell's environment, so `--env-file` names a file that sets the token
variables (`export CLAUDE_TOKEN_WORK=…`); zsh sources it each time the router starts. It must be
yours, and neither it nor its directory writable by anyone else. `install` reads the config
first, and refuses one the router couldn't serve. After a token changes, `switchboard service
restart`.
`switchboard service status` shows how it's doing, and `switchboard serve` runs the router in the
foreground instead.

## Shell integration

Add to `~/.zshrc`:

```sh
eval "$(switchboard init zsh)"
```

`claude` then starts Claude Code through the router, and `cxwork`, `cxpersonal` and so on start it
pinned to one account. When the router isn't running, Claude Code connects directly, and says so;
when switchboard can't take part at all, it starts as if switchboard weren't there.

## Commands

| Command | Does |
|---|---|
| `switchboard usage [-w [interval]] [--no-notify] [--probe]` | The dashboard; `-w` keeps it on screen |
| `switchboard status [--session <id>] [--json] [--probe]` | Every account's usage as text or JSON; `--session` for a statusline |
| `switchboard pin <account> [--move]`, `switchboard pin auto` | Send every new session to one account (and with `--move`, the running ones too), or route automatically again |
| `switchboard run [--account <id>] [--direct] [-- <claude args>]` | Start Claude Code through the router (what the `claude` function runs) |
| `switchboard init zsh [--prefix <prefix>]` | Print the shell integration |
| `switchboard service install\|uninstall\|restart\|status` | Manage the LaunchAgent |
| `switchboard serve [--log-level <level>]` | Run the router in the foreground |
| `switchboard accounts` | List the configured accounts |
| `switchboard logs [router\|cli] [-n N] [-f] [--path]` | Show or follow a log |

Every command takes `--config <file>`, and `--help`.

## The dashboard

`switchboard usage` draws a card per account: a bar for each window, with a marker where even use
would be, a projection ("on pace for 92%", "runs out ~Fri 19:40"), when it resets, and, for an
exhausted window, a countdown until it's back. The best account to use next is marked `▲ best`,
and the pinned one `● pinned`. With `-w` it stays on screen: while the router runs it reads the
router's view every few seconds, and the keys `1`–`9` pin new sessions to an account, `a` routes
automatically again, `m` moves running sessions to the pinned account, `r` refreshes and `q`
quits. Without the router, it probes each account itself every interval (30 minutes unless
given).

## Notifications

While it runs, the router posts desktop notifications (macOS): an account hitting a limit, and the
sessions it moved; an account with room again; a window passing the warning share; and, if asked,
every other move. `[notifications]` in the config says which. A dashboard watching without the
router posts its own room-again and warning notifications.

## Logs

The router logs to `~/.local/state/switchboard/logs/router.log` (or under `$XDG_STATE_HOME`),
every other command to `cli.log` beside it; `switchboard logs` shows them, and `-f` follows.
`SWITCHBOARD_LOG_LEVEL` (`debug`, `info`, `warn`, `error`) sets how much is logged, and
`serve --log-level`, or `service install --log-level`, overrides it for the router. Tokens are
never logged.

## Contributing

`CLAUDE.md` holds the house rules. Every change passes the gates:

```sh
gofmt -l .               # must print nothing
go vet ./...
scripts/test-isolated    # every test, race detector on, in a sandbox
golangci-lint run ./...
go build ./...
```

`scripts/test-isolated` runs the tests inside a macOS sandbox that denies them the network, the
real home directory, config and state, and the real `claude`, `launchctl`, `osascript`, `tmux`
and `open`; every package's tests also run through `internal/testguard`. No test may touch the
real system: a test that needs something a guard blocks injects it instead.
