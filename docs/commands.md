# Commands

Every switchboard command has a page of its own, with an example, its flags and the details. This
page lists them, and holds what every command shares.

## Every command

| Group | Command | What it does |
|---|---|---|
| Everyday | [`usage`](commands/usage.md) | Prints the windows of every account as they stand. |
| | [`dashboard`](commands/dashboard.md) | Shows the usage of the accounts in a pool, full screen, on seven tabs. |
| | [`pin`](commands/pin.md) | Tells the router where to send sessions. |
| | [`claude`](commands/claude-command.md) | Starts the real Claude Code through the router. |
| Setting up | [`setup`](commands/setup.md) | Walks you through setting switchboard up. |
| | [`accounts`](commands/accounts.md) | Lists, adds, edits and removes accounts, and replaces their tokens. |
| | [`pool`](commands/pool.md) | Lists the pools, and makes, edits and removes them. |
| | [`service`](commands/service.md) | Manages the LaunchAgent that keeps the router running. |
| | [`uninstall`](commands/uninstall.md) | Takes away what `setup` put on this Mac, before `brew uninstall`. |
| Checking in | [`status`](commands/status.md) | Prints the usage of every account as text, or the status document as JSON. |
| | [`logs`](commands/logs.md) | Shows the last lines of the router log or the CLI log. |
| Looking back | [`requests`](commands/requests.md) | Lists the requests in the ledger. |
| | [`history`](commands/history.md) | Sums up the days in the ledger, priced, and judges whether you have enough accounts. |
| | [`sessions`](commands/sessions.md) | Lists the sessions of today, or shows the page of one session. |
| | [`events`](commands/events.md) | Lists what the router decided and noticed. |
| Keeping history | [`export`](commands/export.md) | Copies the history of this Mac into a folder of its own. |
| | [`import`](commands/import.md) | Brings the history of other Macs into this one. |
| Plumbing | [`run`](commands/run.md) | Starts Claude Code through the router. It's what `claude` runs. |
| | [`serve`](commands/serve.md) | Runs the router in the foreground. |
| Help and version | `help` | `switchboard help` lists the commands. `switchboard help <command>` shows the help of one. |
| | `version` | Prints the version of switchboard. `--version` and `-v` do the same. |

## Flags every command takes

| Flag | What it does |
|---|---|
| `--config <file>` | Uses `<file>` as the config file, in place of the one switchboard finds. See [Configuration](configuration.md). |
| `-h`, `--help` | Shows the help of the command. `claude --help` is the help of Claude Code. |

A command that needs the router fails without it, and says how to start it.

If you paste a token by mistake, what you see depends on where it went:

| You pasted it | What shows |
|---|---|
| Where an account id or a session id goes, or as the value of a flag | `[redacted]`. No command repeats it, even in refusing it. |
| Where a command goes, as in `switchboard <token>` or `switchboard help <token>` | The token, as given, in saying there's no such command or topic. |

## Exit status

| Status | When |
|---|---|
| `0` | The command did what it was asked. |
| `1` | It failed, for any reason, and said why on stderr. |
| `130` | You interrupted it, for example with `ctrl+c`, while it asked you for something, made a token, or installed the service. |
| `78` | `serve --check` found that the router couldn't start. Routers read it across versions, so it never changes. |

A command with nothing to finish ends at the interrupt itself, which a shell reports as 130 too.
Some commands differ:

| Command | Exit status |
|---|---|
| `run`, and so `claude` | The status of Claude Code, once it has started. `1` only if switchboard couldn't start Claude Code. |
| `status` | `1` if the config can't be read, even after printing the document of the router. |
| `status --session` | `1` if the router isn't running or hasn't seen the session, so a statusline can tell. |
| `service status` | `0` whatever it finds, a router that's down included. `1` only if it can't look. |
| `events -f`, `logs -f`, `dashboard` and `serve` | `0` when interrupted, and for `serve` when terminated, once it has stopped. |
| `hook session-start` | Always `0`. |

## Reading a pool

These verbs read or act on one pool, and take `--pool <id>`: `status`, `usage`, `sessions`,
`requests`, `history`, `events`, `dashboard` and `pin`. Name the pool by the id that
`switchboard pool` lists. `--pool ""` is the same as leaving it out. See [Pools](pools.md).

| Without `--pool`, if | The verb reads |
|---|---|
| There's one pool | That pool. |
| There's an unfenced pool | The unfenced pool. |
| Every pool is fenced | None. It fails: `every pool is fenced: name a pool, q7mt (Acme) or r2d4 (Client), with --pool`. |
| The config has no pools yet, from before the move to pools | Every account. `--pool` is ignored. |
| The config names no pools after the move, or can't be read | The pools of the router, or else of the accounts record. If none names a pool, it refuses: `can't tell which pool to read`. |

A pool that isn't configured fails too: `there's no pool "x7": name …`. Removed pools are the
exception, below.

The folder you run a verb in never chooses its pool, because that could act on a pool you didn't
know you were in. Inside a session, the skill has Claude pass `--pool "$SWITCHBOARD_POOL"`, the
pool of that session.

- **A verb given accounts or a session** finds them in whatever pool they're in. With `--pool`, it
  looks only in that pool, so passing `--pool` is always safe.
- **Once there's a second pool,** the text of each verb names the pool it read, such as `pool Acme`.
  The JSON gives `pool` and `pool_label`.
- **A removed pool keeps its history.** `requests`, `history`, `sessions` and `events` read it with
  `--pool <id>`, marked `pool Acme (removed 6 Oct)`. A pool brought in from another Mac shows as
  `pool Client (not configured here)`. `status`, `usage` and `pin` refuse such a pool.
