# Where the files are

Every file and folder switchboard keeps, where it lives, and how long it stays. The formats of the
files you'd read are on pages of their own, linked from each row.

```bash
ls ~/.local/state/switchboard
```

```
accounts.json  control.sock  history  ledger
logs  prefs.json  state.json  tokens
```

## Where each thing lives

| What | Default path | Moved by |
|---|---|---|
| The config | `~/.config/switchboard/config.toml` | `$SWITCHBOARD_CONFIG`, then `$XDG_CONFIG_HOME` |
| Dashboard themes | `~/.config/switchboard/themes/` | `$SWITCHBOARD_THEMES_DIR`, then `$XDG_CONFIG_HOME` |
| The state directory | `~/.local/state/switchboard/` | `$XDG_STATE_HOME` |
| The `claude` link | `~/.local/share/switchboard/bin/claude` | `$XDG_DATA_HOME` |
| The shell files | `~/.local/share/switchboard/shell/` | `$XDG_DATA_HOME` |
| The router copy | `~/.local/share/switchboard/router/switchboard` | `$XDG_DATA_HOME` |
| The notifications app | `~/.local/share/switchboard/Switchboard.app` | `$XDG_DATA_HOME` |
| The LaunchAgent | `~/Library/LaunchAgents/io.github.leeovery.switchboard.plist` | Nothing |
| The skill | `~/.claude/skills/switchboard/SKILL.md` | `$CLAUDE_CONFIG_DIR` |
| The session hook | `~/.claude/settings.json`, under `hooks.SessionStart` | `$CLAUDE_CONFIG_DIR` |

- `$SWITCHBOARD_CONFIG` names the config file itself, and `$SWITCHBOARD_THEMES_DIR` the themes
  folder itself.
- An `XDG_` variable replaces the start of the path. For example, `$XDG_DATA_HOME/switchboard/bin/claude`
  takes the place of `~/.local/share/switchboard/bin/claude`.
- `$CLAUDE_CONFIG_DIR` takes the place of `~/.claude`.

## The files beside them

- **The shell files** are `init.zsh`, `init.bash`, `completion.bash` and `init.fish`.
- **The router copy** is the switchboard binary the LaunchAgent runs. `service install` makes it.
  A router started from a LaunchAgent written before the copy existed moves over to it.
- **The notifications app** is written out by each release, and the router's notifications come
  from it. See [Notifications](../notifications.md).
- **`.Switchboard.app.sum`** sits beside the app. It holds the SHA-256 of the zip the app was
  written from, which says whether the app is current.
- **`.Switchboard.app.lock`** sits beside the app too. Two switchboards writing the app out take
  turns under it.
- **The session hook** has a page of its own: [The session hook](session-hook.md).

## Your shell startup file

`setup` adds one line to your shell's startup file, marked `# switchboard`, as the last line.
The file is the one your shell says it reads:

| Shell | The file |
|---|---|
| zsh | `.zshrc` in `$ZDOTDIR`, as zsh reports it, else `~/.zshrc` |
| bash | The first that exists of the three below, else `~/.bash_profile` |
| fish | `config.fish` in `$__fish_config_dir`, as fish reports it, by default `~/.config/fish` |

For bash, the three are `~/.bash_profile`, `~/.bash_login` and `~/.profile`, in that order. That's
the file a login bash reads, and terminals on macOS start a login bash.

## In the state directory

| File | What it holds |
|---|---|
| `state.json` | What the router keeps across a restart. See below. |
| `control.sock` | The socket of the [control API](control-api.md). |
| `prefs.json` | What the dashboard remembers. Switchboard's own: its form can change in any release. |
| `tokens/<id>` | Each account's token. See [Tokens](tokens.md). |
| `tokens/<id>.taken` | When switchboard took each of the account's latest tokens, with a hash of each. See [Tokens](tokens.md). |
| `accounts.json` | The [accounts record](accounts-record.md): every account and pool this Mac has had, kept for good. |
| `accounts.lock` | The lock a change to the accounts record holds. |
| `accounts.moved` | Marks the move onto account ids as done, with when it was. |
| `pools.moved` | Marks the move to pools as done, with when it was. |
| `logs/` | `router.log`, `cli.log` and `launchd.log`. See [The logs](../troubleshooting/logs.md). |
| `history/readings-<date>.jsonl` | The [readings history](readings.md), a file a day. |
| `ledger/requests-<date>.jsonl` | The [request ledger](ledger.md), a file a day. |
| `ledger/day-<date>.json` | The [day summary](day-summary.md) of each day. |
| `ledger/events-<date>.jsonl` | The [router events](events.md), a file a day. |
| `install-id` | The id of this install: 16 lowercase hex characters. |
| `import/` | The folders an import copied in, for the router to take in. |
| `import.lock` | The lock an import holds while it runs. |

`state.json` holds these, and they outlast a restart:

- the account each session is on;
- the pins;
- the last readings of each account;
- how long use stays on its account, which the week forecast goes by;
- the SHA-256 hash of each account token, and of the tokens it replaced.

## Locks and markers

- **`accounts.lock` and `import.lock` stay** after their process ends, because the lock goes with
  the process that held it. A file left there holds nothing.
- **A second import is refused** while `import.lock` is held.
- **`pools.moved`** means the move gave the first pool to everything from before pools. That's
  every request line, every day summary, and every event that names a pool: `started`, `moved`,
  `pin` and `auto`.
- **`install-id`** names the folder an export writes for this Mac. The first export makes it, and
  nothing replaces it. See [`export`](../commands/export.md).
- **`import/`** holds one folder for each Mac imported, as `1/`, `2/` and on. Each holds its
  files in the same folders as the state directory. The router removes `import/` once it has taken it in.
  The next import removes one that an import cut short left behind.

How the move onto account ids works is in [Upgrades and restarts](../upgrades-and-restarts.md).

## Indexes the router builds

The router keeps four kinds of index beside the lines of each day. They let the dashboard and the
verbs read a day without reading all its lines. The form of each is switchboard's own, and can
change in any release.

| File | What it holds |
|---|---|
| `ledger/digest-<date>.json.gz` | Each session the day summary names, as it stood at the end of the day, with its page. |
| `ledger/parts-<date>.gz` | What each session page gained that day: its turns, the subagents it first saw, and when its requests went upstream. |
| `ledger/logday-<date>.json.gz` | Each session in the lines of the day, with the directory it ran in, for the find of the Log. |
| `ledger/rows-<date>.gz` | A row for each line of the day, as the Log shows it, without a price. |

- Each index is kept as long as the lines of its day.
- Deleting one is safe. Until the router writes it again, the lines of the day are read instead.
- If a parts file is missing or damaged, a session page rests on an older digest, or reads the
  lines of the session whole.

## Compressed and damaged files

Two days after a day ends, its files of lines are compressed to `<name>.jsonl.gz`. Read one with
`gzip -dc`.

A compressed file can be found damaged as it's rewritten, for example by the move onto account
ids. Then its whole lines before the damage stay in its place. The damaged file is set aside
beside it as `<name>.jsonl.gz.damaged`, then `.damaged-2` and on, for anyone who'd recover more of
it. It's removed with its day.

## How long each file stays

| Files | Kept for |
|---|---|
| `history/readings-*` | As long as `[history] keep` says |
| `ledger/requests-*` | As long as `[ledger] keep` says, and past it until the day summary stands |
| `ledger/events-*` | As long as `[ledger] keep` says, with no wait for the summary, so they can go first |
| `ledger/day-*` | For good |
| The indexes | As long as the request lines of their day |

The `keep` keys are in [Configuration](../configuration.md).
