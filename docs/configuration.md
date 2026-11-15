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
week_starts = "monday"      # optional: the day calendar weeks start on

[[account]]
id      = "b6s7"            # written by switchboard: don't change it
label   = "Work"            # what every screen shows
pool    = "d3wn"            # the pool the account is in
primary = true              # the account your browser and the Claude app are signed into
reserve = 0.1               # keep 10% of every window for the Claude app
plan    = "max"             # pro, max or ultra

[[account]]
id    = "q4tx"
label = "Acme 1"
pool  = "q7mt"
plan  = "ultra"

[[pool]]
id    = "d3wn"
label = "Default"

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["~/Code/acme"]     # sessions started here use only the accounts of Acme

[prime]
day = "08:00-23:00"         # spread the 5-hour resets over your day

[notifications]
warning = 0.8               # warn when a window passes 80% of its limit
```

Only `[[account]]` is required. Every other table and key falls back to the default in the tables
below.

## When a change takes effect

The router looks at the config every 3 seconds, so you never need to tell it about a change:

| If you change | Then |
|---|---|
| A pool: its folders, its primary, or which accounts it holds | The router takes it up within about 3 seconds. |
| Anything else, such as a new account | The router restarts to take it up. The service restarts it by itself once no request is in flight, and `switchboard service restart` does it at once. |
| The file, and it doesn't load | The router keeps the config it has, and logs a warning. |

## How switchboard reads the config

- Accounts keep the order they have in the file. Every screen shows them in that order, and
  priming gives them their slots in that order.
- An unknown key is an error. Switchboard reports every problem in the file at once.
- No error quotes a token pasted into the config by mistake. The token shows as `[redacted]`, or
  not at all.

## Top-level keys

| Key | Default | What it does |
|---|---|---|
| `listen` | `127.0.0.1:4747` | The address the router listens on, as `host:port`. The host must be a loopback IP address, `127.0.0.1` or `[::1]`, and never a name such as `localhost`. |
| `upstream` | `https://api.anthropic.com` | The API the router sends requests to. It must use `https`, unless its host is a loopback IP address. |
| `week_starts` | `monday` | The day calendar weeks start on in `history` and on the dashboard, named in English, in any case. The weeks of each account still reset when Anthropic resets them. |
| `when_router_down` | `stop` | What `claude` does when it can't reach the router. `stop` starts nothing and says why. `direct` connects straight to the API, unrouted and unrecorded. |

To choose for one launch, run `SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude`, or `=stop`. A direct
session can spend a reserve, which only the router honours. Every case is on
[`claude`](commands/claude.md).

## Accounts: `[[account]]`

The config has one `[[account]]` table for each Claude subscription, and at least one.
`switchboard setup` and `accounts add` write the table, `id` and all. Edit the other keys by hand
or with `accounts edit`.

| Key | Default | What it does |
|---|---|---|
| `id` | Written by switchboard | The id that commands and records name the account by. It is also the name of the token file. Don't change it. |
| `label` | The id | What the account is shown as, at most 100 characters. The `accounts` commands keep each label unique, ignoring case. |
| `pool` | None | The id of the pool the account is in. Every account needs one if the config has `[[pool]]` tables, and none can have one if it doesn't. |
| `primary` | The first account in the pool | `true` on the account your browser and the Claude app are signed into. A pool can have one primary at most. |
| `reserve` | `0` | The share of every window the router leaves unused: `0`, or more than `0` and less than `1`. See [Room for the Claude app](room-for-the-claude-app.md). |
| `plan` | None | `pro`, `max` or `ultra`. The old names, `max5x` for `max` and `max20x` for `ultra`, still load. |

### Why set the plan

Switchboard can't read the plan of an account from Anthropic, so `plan` is the only way it knows.
It uses the plan in three places:

- It sets the cost of the plan against what your use was worth at API prices.
- It sizes the account in the forecast of where a week runs out.
- It sizes the account when `history` replays a week.

A Max week counts as 5 Pro weeks, and an Ultra week as 10. An account without a `plan` shows its
plan as `-`, and has no cost to set its worth against. It counts as the plan most of your accounts
have, or, if none has a plan, every account counts the same.

### Ids written by hand

The ids switchboard makes are 4 lowercase letters and digits, starting with a letter. They run a
character longer if 4 would clash. An id you write yourself must follow these rules:

- It starts with a letter or digit, followed only by letters, digits, `-` and `_`.
- It differs from every other id, ignoring case, because macOS gives `b6s7` and `B6S7` one token
  file.
- It isn't `auto`, which `switchboard pin auto` uses.
- It holds nothing shaped like a token.

A pool id follows the same rules, and is never the id of an account. A config from before
switchboard made ids still loads, and the router gives the accounts ids as it first starts.

## Pools: `[[pool]]`

A pool ring-fences a set of accounts to the folders you choose: see [Pools](pools.md). The `pool`
commands write each table, `id` and all. Edit `label` and `dirs` by hand or with `pool edit`. The
router makes the first pool, labelled `Default`, as it moves your history onto pools.

| Key | Default | What it does |
|---|---|---|
| `id` | Written by switchboard | The id that commands and records name the pool by. Don't change it. |
| `label` | The id | What the pool is shown as, at most 100 characters. `pool add` and `pool edit` keep each label unique, ignoring case. |
| `dirs` | None (unfenced) | The folders the pool is fenced to, each an absolute path or one starting `~/`. A session started under them, or in a git worktree of a repository there, uses only this pool. |

Switchboard checks these rules as it loads the config:

- Only one pool can be unfenced.
- No folder can be the same as another, or inside or around it, in any pool. Folders compare a whole
  name at a time, ignoring case.
- Every account names one of the pools in `pool`.

Write `dirs` on one line. `pool edit --dirs` can't change an array written across lines.

## Priming: `[prime]`

| Key | Default | What it does |
|---|---|---|
| `day` | None (priming off) | Your day, as `HH:MM-HH:MM` in local time, such as `08:00-23:00`. Priming spreads the 5-hour resets over it. An end before the start runs past midnight, and the two must differ. |

What priming does, and what it costs, is on [Staggered resets](staggered-resets.md).

## Notifications: `[notifications]`

| Key | Default | What it turns on |
|---|---|---|
| `limits` | `true` | An account hits a limit, with the sessions the limit moved. |
| `room` | `true` | An account has room again. |
| `warning` | `0.9` | A window passes this share of its limit: more than `0` and less than `1`. `0` turns the warning off. |
| `moves` | `false` | Every other session move, such as after an hour idle or by a pin. |

What each notification says, and the app it comes from, are on
[Notifications](notifications.md).

## The record: `[history]` and `[ledger]`

| Key | Default | What it does |
|---|---|---|
| `[history] keep` | `400d` | How long to keep each day file of window readings, once the day has ended. |
| `[ledger] keep` | `400d` | How long to keep each day file of request lines and router events, once the day has ended. |

Each `keep` takes a whole number of days from `8d`, such as `30d`, or `forever`. The summary of
each day stays whatever `keep` says. What the record holds is on
[Your record](your-record.md).

## Export: `[export]`

| Key | Default | What it does |
|---|---|---|
| `dir` | None (no export) | The folder the router exports the history into, as it starts and every hour after. It is an absolute path, or one starting `~/`. Each Mac writes into a folder of its own inside it. Left empty, the router exports nothing. |

How to use the export for backups and a second Mac is on [Several Macs](several-macs.md).

## Usage: `[usage]`

| Key | Default | What it does |
|---|---|---|
| `by` | `window` | How `usage` and the dashboard Overview lay out the windows. `window` compares the accounts one window at a time. `account` shows the windows of each account together. |

`usage --by` overrides it for one printout. Pressing `b` on the dashboard overrides it from then on.

## Prices: `[prices]`

Switchboard carries its own prices, read from the Anthropic pricing page. These keys set your own
in their place, for `history`, `sessions` and the dashboard to price with.

```toml
[prices.plans]
max = 90                         # US dollars a month

[prices.models.claude-opus-5-5]  # US dollars a million tokens
input          = 3.2
output         = 16
cache_read     = 0.16
cache_write_5m = 4
cache_write_1h = 6.4
```

| Key | Default | What it does |
|---|---|---|
| `[prices.plans]` | Switchboard prices | The price of a plan in US dollars a month, under `pro`, `max` or `ultra`. An old name, `max5x` or `max20x`, loads, but not beside its new name. |
| `[prices.models.<id>]` | Switchboard prices | The prices of a model in US dollars a million tokens, one for each kind of token. Name the model by its API id, such as `claude-opus-5-5`. |

- Each price is a number from `0` to `1000000`.
- A price you give holds for every day, past days too.
- A price you leave out keeps the price switchboard carries.
- A plan name or a kind of token that switchboard doesn't know is an error.

For a model that switchboard doesn't know:

| If you give | Then |
|---|---|
| All five prices | Switchboard prices the model. Web searches get the price switchboard carries, and US-only inference stays unpriced. |
| Fewer than five | The model stays unpriced, and the log says so. |

## Not in the config

- **Tokens:** each account keeps its token in a file of its own, `<state dir>/tokens/<id>`. How
  switchboard reads and writes those files is on [Tokens](reference/tokens.md).
- **The session hook:** `setup` adds one entry to the `settings.json` of Claude Code, and leaves
  the rest of that file alone. There is nothing in it to set. What it does is on
  [How it works](how-it-works.md).
