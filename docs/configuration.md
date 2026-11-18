# Configuration

Switchboard reads its settings from one TOML file, the config. `switchboard setup` writes it, and
the `accounts` and `pool` commands edit it. You can edit it by hand too. This page lists every key,
with its default and the values it takes.

## Where the config lives

Switchboard uses the first of these that is set:

1. `$SWITCHBOARD_CONFIG`
2. `$XDG_CONFIG_HOME/switchboard/config.toml`
3. `~/.config/switchboard/config.toml`

To use another file for one command, pass `--config <file>`.

## An example config

```toml
# optional: the day calendar weeks start on
week_starts = "monday"

[[account]]
# do not edit: links the account to its history
id      = "b6s7"
# what every screen shows
label   = "Work"
pool    = "d3wn"
# Claude Code holds this account's token
primary = true
# keep 10% of every window for the Claude app
reserve = 0.1
# pro, max or ultra
plan    = "max"

[[account]]
id    = "m2kp"
label = "Acme 1"
pool  = "q7mt"
plan  = "ultra"

[[pool]]
id    = "d3wn"
label = "Default"

[[pool]]
id    = "q7mt"
label = "Acme"
# sessions started here use only Acme's accounts
dirs  = ["~/Code/acme"]

[prime]
# spread the 5-hour resets over your day
day = "08:00-23:00"

[notifications]
# warn when a window passes 80% of its limit
warning = 0.8
```

Only `[[account]]` is required. Every other table and key falls back to the default in the tables
below.

## When a change takes effect

The router looks at the config every 3 seconds:

| If you change | Then |
|---|---|
| A pool, or which pool an account is in | Taken up within about 3 seconds. |
| `[export] dir` | Taken up within about 3 seconds. The next export goes to the new folder. |
| Removing an account | Within about 3 seconds, sessions still on its token count as the primary's. |
| Anything else, such as a new account | Taken up when the router restarts. |
| The file, and it doesn't load | The router keeps the config it has, and logs a warning. |

The router restarts itself once no request is in flight. `switchboard service restart` does it
now. A router you run by hand with `serve` only logs that a restart is due. How restarts work is on
[Upgrades and restarts](upgrades-and-restarts.md).

## How switchboard reads the config

- Accounts keep the order they have in the file. Every screen shows them in that order, and
  priming gives them their slots in that order.
- An unknown key is an error. Switchboard reports every problem in the file at once.
- No error quotes a token pasted into the config by mistake. The token shows as `[redacted]`, or
  not at all.

## Top-level keys

| Key | Default | What it does |
|---|---|---|
| `listen` | `127.0.0.1:4747` | The address the router listens on. |
| `upstream` | `https://api.anthropic.com` | The API the router sends requests to. |
| `week_starts` | `monday` | The day calendar weeks start on. |
| `when_router_down` | `stop` | What `claude` does if it can't reach the router. |

- **`listen`** is `host:port`. The host must be a loopback IP address, `127.0.0.1` or `[::1]`,
  never a name such as `localhost`.
- **`upstream`** must use `https`, unless its host is a loopback IP address.
- **`week_starts`** names a day in English, in any case. It sets the calendar weeks in `history`
  and on the dashboard. Each account still resets its week when Anthropic does.
- **`when_router_down`** is `stop` or `direct`. With `stop`, `claude` starts nothing and says why.
  With `direct`, it connects straight to the API, unrouted and unrecorded. A direct session can
  spend a reserve, which only the router honours.

To choose for one launch, run `SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude`, or `=stop`. Every case
is on [`claude`](commands/claude-command.md).

## Accounts: `[[account]]`

The config has one `[[account]]` table for each Claude subscription, and at least one.
`switchboard setup` and `accounts add` write the table, `id` and all. Edit the other keys by hand
or with `accounts edit`.

| Key | Default | What it does |
|---|---|---|
| `id` | Written by switchboard | Names the account in commands and records. Don't edit it. |
| `label` | The id | What screens show, at most 100 characters. |
| `pool` | None | The id of the pool the account is in. |
| `primary` | The first account in the pool | `true` on the primary of the pool. |
| `reserve` | `0` | The share of every window the router leaves unused. |
| `plan` | None | `pro`, `max` or `ultra`. |

- **`id`** is also the name of the token file. Switchboard writes `# do not edit` beside it,
  because a changed id cuts the account off from its token and its history.
- **`label`** is kept unique by the `accounts` commands, ignoring case.
- **`pool`** is needed on every account if the config has `[[pool]]` tables, and allowed on none if
  it doesn't.
- **`primary`** marks the account whose token Claude Code holds in the pool: see the
  [glossary](glossary.md). A pool has one primary at most. Make the primary of your first pool the
  account your browser and the Claude app are signed into, because artifacts open there.
- **`reserve`** is `0`, or more than `0` and less than `1`, such as `0.1` for 10%. See
  [Room for the Claude app](room-for-the-claude-app.md).
- **`plan`** also loads the old names, `max5x` for `max` and `max20x` for `ultra`.

### Why set the plan

Switchboard can't read an account's plan from Anthropic, so `plan` is the only way it knows. It
uses the plan in three places:

- It sets the plan's cost against what your use was worth at API prices.
- It sizes the account in the forecast of where a week runs out.
- It sizes the account when `history` replays a week.

A Max week counts as 5 Pro weeks, and an Ultra week as 10. An account without a `plan` shows its
plan as `-`, and has no cost to set its worth against. It counts as the plan most of your accounts
have, or, if none has a plan, every account counts the same.

### Ids written by hand

Switchboard makes ids of 4 lowercase letters and digits, starting with a letter. They run a
character longer if 4 would clash. An id you write yourself must follow these rules:

- It starts with a letter or digit, followed only by letters, digits, `-` and `_`.
- It differs from every other id, ignoring case, because macOS gives `b6s7` and `B6S7` one token
  file.
- It isn't `auto`, which `switchboard pin auto` uses.
- It holds nothing shaped like a token.

A pool id follows the same rules, and is never an account id. A config from before switchboard made
ids still loads, and the router gives the accounts ids as it first starts. See
[Upgrades and restarts](upgrades-and-restarts.md).

## Pools: `[[pool]]`

A pool ring-fences a set of accounts to the folders you choose: see [Pools](pools.md). The `pool`
commands write each table, `id` and all. Edit `label` and `dirs` by hand or with `pool edit`. The
router makes the first pool, labelled `Default`, as it moves your record onto pools.

| Key | Default | What it does |
|---|---|---|
| `id` | Written by switchboard | Names the pool in commands and records. Don't edit it. |
| `label` | The id | What screens show, at most 100 characters. |
| `dirs` | None (unfenced) | The folders the pool is fenced to. |

- **`id`** has `# do not edit` written beside it, the same as an account's.
- **`label`** is kept unique by `pool add` and `pool edit`, ignoring case.
- **`dirs`** holds absolute paths, or paths starting `~/`. A session started under one of them, or
  in a git worktree of a repository there, uses only this pool.
- Write `dirs` on one line. `pool edit --dirs` can't change an array written across lines.

Switchboard checks these rules as it loads the config:

- Only one pool can be unfenced.
- No folder can be the same as another, or inside or around it, in any pool. Folders compare a whole
  name at a time, ignoring case.
- Every account names one of the pools in `pool`.

## Priming: `[prime]`

| Key | Default | What it does |
|---|---|---|
| `day` | None (priming off) | Your day, as `HH:MM-HH:MM` in local time. |

Priming spreads the 5-hour resets over your day, such as `08:00-23:00`. An end before the start
runs past midnight, and the two must differ. What priming does, and what it costs, is on
[Staggered resets](staggered-resets.md).

## Notifications: `[notifications]`

| Key | Default | What it turns on |
|---|---|---|
| `limits` | `true` | An account hits a limit, or goes on a [hold](glossary.md). |
| `room` | `true` | An account has room again. |
| `warning` | `0.9` | A window passes this share of its limit. |
| `moves` | `false` | Every other session move. |

`warning` is more than `0` and less than `1`, and `0` turns the warning off. What each
notification says, and the app it comes from, are on [Notifications](notifications.md).

## The record: `[history]` and `[ledger]`

| Key | Default | What it keeps |
|---|---|---|
| `[history] keep` | `400d` | Day files of window readings. |
| `[ledger] keep` | `400d` | Day files of request lines and router events. |

Each `keep` says how long to keep a day file once the day has ended. It takes a whole number of
days from `8d`, such as `30d`, or `forever`. The summary of each day stays whatever `keep` says.
What the record holds is on [Your record](your-record.md).

## Export: `[export]`

| Key | Default | What it does |
|---|---|---|
| `dir` | None (no export) | The folder the router exports your record into. |

- The router exports as it starts, and every hour after.
- `dir` is an absolute path, or one starting `~/`. Left empty, the router exports nothing.
- Each Mac writes into a folder of its own inside `dir`.

How to use the export for backups and a second Mac is on [Backups and a new Mac](several-macs.md).

## Usage: `[usage]`

| Key | Default | What it does |
|---|---|---|
| `by` | `window` | How `usage` and the Overview lay out the windows. |

`window` compares the accounts one window at a time. `account` shows each account's windows
together. `usage --by` overrides it for one printout. Pressing `b` on the dashboard overrides it
from then on.

## Prices: `[prices]`

Switchboard carries its own prices, read from Anthropic's pricing page. These keys set your own in
their place, for `history`, `sessions` and the dashboard to price with.

```toml
# US dollars a month
[prices.plans]
max = 90

# US dollars a million tokens
[prices.models.claude-opus-5-5]
input          = 3.2
output         = 16
cache_read     = 0.16
cache_write_5m = 4
cache_write_1h = 6.4
```

| Key | Default | What it does |
|---|---|---|
| `[prices.plans]` | Switchboard prices | A plan price, in US dollars a month. |
| `[prices.models.<id>]` | Switchboard prices | Model prices, in US dollars a million tokens. |

- Plans go by `pro`, `max` or `ultra`. An old name, `max5x` or `max20x`, loads, but not beside its
  new name.
- Models go by their API id, such as `claude-opus-5-5`. The five kinds of token are `input`,
  `output`, `cache_read`, `cache_write_5m` and `cache_write_1h`.
- Each price is a number from `0` to `1000000`.
- A price you give holds for every day, past days too.
- A price you leave out keeps the price switchboard carries.
- A plan name or a kind of token that switchboard doesn't know is an error.

For a model that switchboard doesn't know:

| If you give | Then |
|---|---|
| All five prices | Switchboard prices the model. |
| Fewer than five | The model stays unpriced, and the log says so. |

A model priced this way gets the web search price switchboard carries. Its US-only inference stays
unpriced.

## Not in the config

- **Tokens:** each account keeps its token in a file of its own, `<state dir>/tokens/<id>`. How
  switchboard reads and writes those files is on [Tokens](reference/tokens.md).
- **The session hook:** `setup` adds one entry to Claude Code's `settings.json`, and leaves the
  rest of that file alone. There is nothing in it to set. What it does is on
  [The session hook](reference/session-hook.md).
