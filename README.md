<div align="center">

# 🎛️ Switchboard

**One Claude Code, many subscriptions**

A local proxy that spreads your Claude Code sessions across several Claude subscriptions,
<br>keeps each session's prompt cache warm, and moves it when an account runs out, before Claude Code notices.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27+-00ADD8.svg)](https://go.dev)

[Install](#install) · [Quick Start](#quick-start) · [How It Works](#how-it-works) · [Commands](#commands) · [Configuration](#configuration) · [Dashboard](#the-dashboard)

</div>

---

Switchboard sits between Claude Code and the Claude API. You start Claude Code as you always do; switchboard decides which of your subscriptions each session runs on, swaps in that account's token, and forwards the request untouched. When an account hits its 5-hour or weekly limit mid-session, the request is replayed on another account and the session carries on: no exiting, no resuming.

`switchboard usage -w` keeps a live dashboard of every account on screen, with pace markers, projections and reset countdowns.

## Why Switchboard?

Run more than one Claude subscription and you know the routine: watch the limits, notice one has run out, exit Claude Code, switch accounts, resume. Meanwhile the weekly quota you didn't get round to before its reset is simply lost.

Switchboard automates the routine without paying for it in prompt cache:

- **Limits stop interrupting you.** A request that hits a limit is replayed on another account before Claude Code sees a byte. You get one slower turn while the cache rebuilds there, then it carries on.
- **Sessions stay put.** Prompt caches are per account, and the first turn after a move costs around 40× a warm one. A session stays on its account while its cache is warm, and moves only when it must, or once it has idled long enough that its cache is cold anyway.
- **No quota goes to waste.** New sessions go to the account whose weekly quota would be lost soonest unused: the share left, divided by the time until it resets.
- **Usage for free.** Every API response carries its account's usage headers, so switchboard reads usage off real traffic and probes only the accounts it hasn't heard from.
- **Never in the way.** When the router isn't running, or switchboard can't take part, `claude` starts exactly as it would without it.

## Install

**Homebrew**

```bash
brew install leeovery/tools/switchboard
```

**From source**

```bash
go install github.com/leeovery/switchboard/cmd/switchboard@latest
```

You'll need Claude Code and a long-lived token (`claude setup-token`) for each subscription. The background service is macOS-only; everything else runs wherever Go does.

## Quick Start

```bash
# 1. Describe your accounts. Tokens stay in your environment; the config only names them.
$EDITOR ~/.config/switchboard/config.toml
switchboard accounts                               # lists them, and whether each token is set

# 2. Run the router in the background, loading the tokens from a file.
switchboard service install --env-file ~/.tokens.env

# 3. Start Claude Code through it (put this in ~/.zshrc).
eval "$(switchboard init zsh)"
claude                                             # routed automatically
cxwork                                             # pinned to the account "work"

# 4. Watch it.
switchboard usage -w
```

## How It Works

```
Claude Code ──ANTHROPIC_BASE_URL──▶ switchboard ──▶ api.anthropic.com
                                      │
                                      ├─ chooses the account: sticky per session and model
                                      ├─ swaps in that account's token
                                      └─ reads every response's usage headers
```

- **Choosing.** A new session goes to the account with the most perishable quota that has room for the request's model. After that it's sticky while its cache is warm (an hour of Claude Code's cache life) and its account has room.
- **Limits.** A 429 that says an account's limit is reached is replayed on the next best account before the client sees it, and the session moves there. Throttling is retried on the same account instead, since moving would waste the cache for nothing.
- **Pins.** `cxwork` pins one session to an account. `switchboard pin work` sends every new session there, and `--move` moves the running ones too. Every pin yields at a limit, rather than failing.
- **State.** Which account each session is on is saved, so a restart doesn't scatter sessions and cost cache rebuilds.

The full design, including the cache facts it rests on, is in [docs/design.md](docs/design.md).

## Commands

| Command | Does |
|---|---|
| `switchboard usage [-w [interval]] [--no-notify] [--probe]` | The dashboard; `-w` keeps it on screen |
| `switchboard status [--session <id>] [--json] [--probe]` | Every account's usage as text or JSON; `--session` for a statusline |
| `switchboard pin <account> [--move]` · `switchboard pin auto` | Send new sessions (and with `--move`, running ones) to one account, or route automatically again |
| `switchboard run [--account <id>] [--direct] [-- <claude args>]` | Start Claude Code through the router: what the `claude` function runs |
| `switchboard init zsh [--prefix <prefix>]` | Print the shell integration |
| `switchboard service install\|uninstall\|restart\|status` | Manage the LaunchAgent that keeps the router running |
| `switchboard serve [--log-level <level>]` | Run the router in the foreground |
| `switchboard accounts` | List the configured accounts and their tokens' state |
| `switchboard logs [router\|cli] [-n N] [-f] [--path]` | Show or follow a log |

Every command takes `--config <file>` and `--help`.

## Configuration

`~/.config/switchboard/config.toml`, or `$XDG_CONFIG_HOME/switchboard/config.toml`, or wherever `$SWITCHBOARD_CONFIG` or `--config` points:

```toml
[[account]]
id        = "work"                # permanent name; also the launcher's: cxwork
label     = "Work"                # shown on the dashboard
token_env = "CLAUDE_TOKEN_WORK"   # the environment variable holding its token

[[account]]
id        = "personal"
label     = "Personal"
token_env = "CLAUDE_TOKEN_PERSONAL"

# Optional, shown with their defaults.
listen = "127.0.0.1:4747"         # the proxy's address: a loopback IP

[notifications]
limits  = true    # an account hits a limit, and the sessions it moved
room    = true    # an account has room again
warning = 0.9     # a window passing this share of its limit; 0 turns it off
moves   = false   # every other session move
```

Switchboard never stores a token. The router's LaunchAgent can't see your shell's environment, so `service install --env-file` names a file that exports the token variables; it must be yours, and neither it nor its directory writable by anyone else.

## The Dashboard

`switchboard usage` draws a card per account: a gradient bar for each window with a marker where even use would be, and on an account with a reserve a mark where the reserve starts, a projection ("on pace for 92%", "runs out ~Fri 19:40"), the reset, and for an exhausted window a countdown until it's back. The best account to use next is marked `▲ best`, the pinned one `● pinned`, and the primary `◆ primary`; an account held back by its reserve says so at the top of its card.

With `-w` it stays on screen. While the router runs it reads the router's live view every few seconds; the keys `1`–`9` pin new sessions to an account, `a` routes automatically again, `m` moves running sessions to the pin, `r` refreshes and `q` quits. Without the router it probes each account itself, every 30 minutes unless given another interval.

## Notifications

While it runs, the router posts desktop notifications (macOS) for:
- an account hitting a limit, and the sessions it moved
- an account with room again
- a window passing the warning share
- if asked, every other move

`[notifications]` says which. A dashboard watching without the router posts its own room-again and warning notifications instead.

## Logs

The router logs to `~/.local/state/switchboard/logs/router.log`, and every other command to `cli.log` beside it. `switchboard logs` shows them, and `-f` follows. `SWITCHBOARD_LOG_LEVEL` (`debug`, `info`, `warn`, `error`) sets the level; `serve --log-level` or `service install --log-level` overrides it for the router. Tokens are never logged.

## Contributing

`CLAUDE.md` holds the house rules. Every change passes the gates:

```bash
gofmt -l .               # must print nothing
go vet ./...
scripts/test-isolated    # every test, race detector on, sandboxed
golangci-lint run ./...
go build ./...
```

`scripts/test-isolated` runs the suite inside a macOS sandbox that denies it the network, the real home directory, config and state, and the real `claude`, `launchctl`, `osascript`, `tmux` and `open`. Every package's tests also run through `internal/testguard`, and a test that needs something the guards block injects it instead.

## License

MIT
