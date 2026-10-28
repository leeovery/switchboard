# Configuration

Switchboard reads its settings from one TOML file, the config. `setup` writes it when you set
switchboard up, and the `accounts` commands edit it. You can edit it by hand too.

Switchboard looks for the config in the first of these places that's set:

1. `$SWITCHBOARD_CONFIG`
2. `$XDG_CONFIG_HOME/switchboard/config.toml`
3. `~/.config/switchboard/config.toml`

To use another file for one command, pass `--config <file>`.

Of Claude Code's settings, `setup` writes one entry in its `settings.json`, the session hook, and
leaves the rest of the file as it is. There's nothing in it to set. See
[A session that isn't going through switchboard](how-it-works.md#a-session-that-isnt-going-through-switchboard).

## Contents

- [Example](#example)
- [How switchboard reads the config](#how-switchboard-reads-the-config)
- [Top-level keys](#top-level-keys)
- [`[[account]]` keys](#account-keys)
- [`[prime]` keys](#prime-keys)
- [`[notifications]` keys](#notifications-keys)
- [`[history]` keys](#history-keys)
- [`[ledger]` keys](#ledger-keys)
- [`[usage]` keys](#usage-keys)
- [`[prices]` keys](#prices-keys)
- [Tokens](#tokens)
- [Notifications](#notifications)

## Example

```toml
week_starts      = "monday"  # optional: the day calendar weeks start on
when_router_down = "stop"    # optional: with the router down, claude stops, or "direct"

[[account]]
id      = "work"        # for good: letters, digits, '-' and '_'; its token is tokens/work
label   = "Work"
primary = true          # the account the browser and the Claude apps are signed into
reserve = 0.1           # optional, on any account: the share of every window the router leaves unused
plan    = "max20x"      # optional, on any account: its plan, pro, max5x or max20x

[[account]]
id    = "personal"
label = "Personal"
plan  = "max5x"

[[account]]
id    = "side"
label = "Side"

[prime]
day = "08:00-23:00"     # your day, in local time: priming spreads the 5-hour resets over it

[notifications]         # optional: these are the defaults
limits  = true
room    = true
warning = 0.9
moves   = false

[history]               # optional: this is the default
keep = "400d"

[ledger]                # optional: this is the default
keep = "400d"

[usage]                 # optional: this is the default
by = "window"

[prices.plans]          # optional: a plan's price, in US dollars a month, in place of switchboard's
max5x = 90

[prices.models.claude-opus-5-5]   # optional: a model's prices, in US dollars a million tokens
input          = 3.2
output         = 16
cache_read     = 0.16
cache_write_5m = 4
cache_write_1h = 6.4
```

## How switchboard reads the config

- **Account order:** The accounts keep the order they have in the file. Switchboard shows them in
  that order everywhere, and [priming](how-it-works.md#priming) gives them their slots in that
  order.
- **Errors:** An unknown key is an error. When the file parses, switchboard reports every problem
  in it at once.
- **Tokens in errors:** No error quotes a token pasted into the config, even when the file fails to
  parse. The token shows as `[redacted]`, or not at all.

## Top-level keys

| Key | Default | Description |
|---|---|---|
| `listen` | `127.0.0.1:4747` | The proxy's address, as `host:port`. |
| `upstream` | `https://api.anthropic.com` | The API's base URL. |
| `week_starts` | `monday` | The day calendar weeks start on, as [`history`](commands.md#history) counts them. |
| `when_router_down` | `stop` | What `claude` does when the router can't be routed through, as when it isn't running: `stop` or `direct`. |

- **`listen`:** The host must be a loopback IP address: `127.0.0.1`, or `::1` written as
  `[::1]:4747`. It can never be a name, such as `localhost`.
- **`upstream`:** The URL must use `https`, unless its host is a loopback IP address.
- **`week_starts`:** Give a day of the week by its name in English, in any case, such as `monday`
  or `Sunday`. An account's own week resets when Anthropic resets it, and never moves with
  `week_starts`.
- **`when_router_down`:** With `stop`, `claude` starts nothing while the router is down, and says
  why and how to go on. A router that answers but isn't healthy isn't down: `claude` goes through it
  all the same, and says so, as its failures are upstream, where a direct connection would meet them
  too. With `direct`, Claude Code connects straight to the API on an account's token, unrouted and
  unrecorded, and it can spend the primary's reserve, which only the router honours.
  `SWITCHBOARD_WHEN_ROUTER_DOWN=direct` or `=stop` overrides the key for the one launch it's given
  to, as in `SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude`. See [`claude`](commands.md#claude).

## `[[account]]` keys

The config has one `[[account]]` table for each subscription, and at least one.

| Key | Default | Description |
|---|---|---|
| `id` | Required | The account's permanent name. It's also the name of the account's token file. |
| `label` | The id | What the account is shown as, at most 100 characters. It can't hold anything shaped like a token. |
| `primary` | The first account | Marks the account the browser and the Claude apps are signed into: [the primary](how-it-works.md#the-primary). At most one account can set it. |
| `reserve` | `0` | The share of every window the router leaves unused: `0`, or more than `0` and less than `1`. [`accounts edit <id> --reserve <percent>`](commands.md#accounts) sets it, as a percentage. See [Reserves](how-it-works.md#reserves). |
| `plan` | None (unknown) | The plan the account's subscription is on: `pro`, `max5x` or `max20x`. |

An `id` must follow these rules:

- It starts with a letter or digit, followed by letters, digits, `-` and `_`.
- It's unique, ignoring case, because macOS gives `work` and `Work` one token file.
- It isn't `auto`.
- It holds nothing shaped like a token.

Switchboard can't read an account's plan from Anthropic, so `plan` is the only way it knows the
plan. [`accounts edit <id> --plan <plan>`](commands.md#accounts) sets it. Switchboard uses the plan
in two places:

- It compares what the plan costs with what the account's use was worth at API prices.
- It sizes the account in [`history`](commands.md#history)'s replay of a week.

Without a `plan`, the account's plan shows as `-`, and nothing is priced against it.

## `[prime]` keys

| Key | Default | Description |
|---|---|---|
| `day` | None (priming off) | Your day, as `HH:MM-HH:MM` in local time. [Priming](how-it-works.md#priming) spreads the 5-hour resets over it. |

An end time before the start time means the day runs past midnight.

## `[notifications]` keys

These keys choose which desktop notifications the router posts. See
[Notifications](#notifications).

| Key | Default | Description |
|---|---|---|
| `limits` | `true` | An account hits a limit, and the sessions the limit moved. |
| `room` | `true` | An account has room again. |
| `warning` | `0.9` | A window passes this share of its limit. `0` turns the warning off. |
| `moves` | `false` | Every other session move, such as after an idle hour or by a pin. |

## `[history]` keys

This key sets how long the router keeps the readings history. See
[What it records](how-it-works.md#what-it-records).

| Key | Default | Description |
|---|---|---|
| `keep` | `400d` | How long to keep a day's file once the day has ended. |

`keep` takes one of these values:

- A whole number of days, written `<n>d`, of `8d` (a week and a day) or more.
- `forever`, which never removes a file.

## `[ledger]` keys

This key sets how long the router keeps the request ledger's lines. See
[What it records](how-it-works.md#what-it-records).

| Key | Default | Description |
|---|---|---|
| `keep` | `400d` | How long to keep a day's file of lines once the day has ended. |

`keep` takes one of these values:

- A whole number of days, written `<n>d`, of `8d` (a week and a day) or more.
- `forever`, which never removes a file.

The day's summary is kept for good, whatever `keep` says.

## `[usage]` keys

This key sets how [`usage`](commands.md#usage) and the dashboard's [**Overview**](dashboard.md#overview)
lay out the windows.

| Key | Default | Description |
|---|---|---|
| `by` | `window` | `window` or `account`. |

- **`window`:** Compares the accounts one window at a time.
- **`account`:** Shows each account's windows together.

To override `by` for one printout, run `usage --by`. Pressing `b` in the dashboard overrides it for
good.

## `[prices]` keys

Switchboard carries its own prices, read from Anthropic's pricing page. These keys set prices in
their place, for [`history`](commands.md#history), [`sessions`](commands.md#sessions) and the
[dashboard](dashboard.md) to price with.

- Each price is a number, `0` or more.
- Each price holds for every day alike.
- A price you leave out keeps switchboard's.

| Key | Default | Description |
|---|---|---|
| `[prices.plans]` | Switchboard's prices | A plan's price in US dollars a month, under the plan's name: `pro`, `max5x` or `max20x`. |
| `[prices.models.<id>]` | Switchboard's prices | A model's prices in US dollars a million tokens, one for each kind of token. |

Name a model by its id as the API names it, such as `claude-opus-5-5`. The kinds of token are
`input`, `output`, `cache_read`, `cache_write_5m` and `cache_write_1h`.

For a model that switchboard doesn't know:

- Switchboard prices it only when you give all five prices. It then prices the model's web
  searches at switchboard's price, and leaves the model's US-only inference unpriced.
- When you give fewer than five, the model stays unpriced, and the log says so.

A plan name or a kind of token that switchboard doesn't know is an error, as any unknown key is.

## Tokens

Each account's token lives in a file of its own, `<state dir>/tokens/<id>`, such as
`~/.local/state/switchboard/tokens/work`. The file holds the token alone, and whitespace around the
token is ignored. Switchboard reads no token from the environment.

### Token file permissions

The token file must be yours, and no one else can read or write it (`chmod 600`). Otherwise, the
account counts as having no token, and `accounts` and `status` say why and how to fix it.

Switchboard keeps the `tokens` directory at `0700` when it writes there. The router also sets the
directory to `0700` as it starts.

### Writing token files

`setup`, `accounts add` and `accounts token` write token files. Anything else can write them too,
such as a secrets manager's file export or a dotfiles step. The best way is to write a temporary
file beside the token file, then rename it into place, as switchboard does.

A token file can be a link to a file kept elsewhere. Switchboard writes a token through the link,
to the file it leads to.

If an account's token file already holds a usable token, switchboard adds the account without
asking for a token.

### When each token was taken

A setup token is good for a year from when it's made. When switchboard writes a token, it also
writes when it took it, in `tokens/<id>.taken` beside the token file. The record holds a pair of
lines for each of the account's latest 8 tokens, the latest first:

```text
taken 2026-10-27T09:30:00Z
sha256 <the token's SHA-256 hash, in lowercase hex>
taken 2026-03-02T14:05:00Z
sha256 <an earlier token's hash>
```

A token's year counts from when switchboard took it. Nothing says when a pasted token was made, so
its year counts from the day you gave it. Giving a token again, even after another, keeps the
earlier of its two dates. The record keeps the latest 8 tokens alone, so a token given again after
8 others counts afresh.

The hash ties each date to its token, never holding the token itself. A token written by anything
else has no known date, unless the record has a date for that same token, as when you restore both
files together. A missing or damaged record only leaves the date unknown: the token works as
before.

The record must be yours, and no one else can read or write it, as with the token file. A record
restored readable by others, but not writable, as a dotfiles checkout often leaves it, counts again
once it's private. A plain file is made private as the router starts. A record that's a second name
of a file, or a link to one, keeps its dates when switchboard next writes a token beside it, which
writes them afresh, private, but only where the file is yours and no one else can write it. A link
to anything else, such as nothing at all, another account's token file or a file that isn't a
record, is left as it is, with what it leads to, and a warning in the log: the date stays unknown,
and the token works as before. So is a record that's another account's token file, or its record,
however it's reached, as a link from one account's file to another's leaves it, and so is a record's
path where another account's token file or record is a link leading there.

`accounts remove` deletes the record with the token file, a link alone where either is one. Where
the token file is a link to the record's own path, and the file there has no other name, it's the
token's only copy, and it's left; while it's so, a token written is written there, and no record
over it, with a warning in the log. Once the account's removed, that file still holds the token, and
switchboard never writes a record over a file holding anything shaped like a token, whatever its
permissions or owner, nor over one it can't read or one over 1 MiB: a token written for an account
of that id later leaves it, with a warning, and its date stays unknown until you move the file.
`accounts remove` leaves such a file too, with a warning, rather than deleting it, unless it's a
second name of the very token file it removes, or of the file it leads to, as a hard link of it,
which goes with it: where the token file is a link to the record's own path and the file there has
another name, the record's path goes with the token, the file's kept under its other name, and
`accounts remove` says so. Writing a token replaces the token file, so a second name made by hand
stops being one at the next write, and is then left as any file holding a token at the record's path
is. `status --json` gives each token's `token_taken`, and its `token_good_until`, 365 days on.

### When switchboard reads token files

A changed token file needs no restart. The router reads the token files at these times:

- When it starts.
- Every 3 seconds, for every account's file.
- When the API refuses an account's token, for that account's file.

A file found without a usable token at one look, such as while it's being rewritten, keeps the
account's token. Only two looks in a row, 3 seconds apart, take the token away.

`claude` also looks twice. If it finds the primary's token file, or a pinned account's, present but
empty, it looks again a moment later before it counts the file as holding no token.

### Replaced and removed tokens

Running sessions keep the token they started with:

- Sessions started before a token was replaced still carry the old token. The router routes them as
  that account's sessions for 7 days.
- Sessions started on the token of an account you've since removed carry that token. The router
  routes them as the primary's sessions for 7 days.

The router keeps none of these tokens, only their SHA-256 hashes.

## Notifications

While the router runs, it posts desktop notifications, whether or not a dashboard is open. The
[`[notifications]` keys](#notifications-keys) choose which kinds it posts:

- **Limits:** An account hits a limit. The notification waits 5 seconds for the sessions the limit
  moves, then tells of them together, such as
  `work · Work hit its Session limit, back at Mon 18:10 — 3 sessions moved to side · Side`.
- **Room again:** An account has room again, such as `work · Work has room again`.
- **Warnings:** A window passes the share set in `warning`. It's posted once until the
  window resets, such as
  `work · Work: Week at 91%`.
- **Moves:** Every other move, such as
  `session 18bb978f moved from work · Work to side · Side (rescored after 1h 2m idle)`. These are
  off unless you turn them on.

It also tells you when an account's token nears the end of its year, whatever the keys say: once
when 30 days or fewer are left, and again at 7, such as
`work · Work: token expires 17 Nov 2026 · 21 days left — switchboard setup makes a new one`, or
`token expired …` once it's past. A router started with a token already within either tells only
the nearest. A restart doesn't tell either again, and a new token starts afresh.

### How often notifications go out

A limit's notification and a token's year's always go out. Any other notification goes out only
when a minute or more has passed since the last one posted about its account. Otherwise, it's
dropped.

Notifications never hold a request up.

### Notifications from the dashboard

When the dashboard probes without the router, it posts its own notifications for these:

- An account with room again.
- A window passing the warning.

To stop them, pass `--no-notify` to `dashboard`. While the router runs, the dashboard posts none,
even when it probes with `--probe`, so you never get the same notification twice.
