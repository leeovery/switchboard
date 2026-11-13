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
- [`[[pool]]` keys](#pool-keys)
- [`[prime]` keys](#prime-keys)
- [`[notifications]` keys](#notifications-keys)
- [`[history]` keys](#history-keys)
- [`[ledger]` keys](#ledger-keys)
- [`[export]` keys](#export-keys)
- [`[usage]` keys](#usage-keys)
- [`[prices]` keys](#prices-keys)
- [Tokens](#tokens)
- [Notifications](#notifications)

## Example

```toml
week_starts      = "monday"  # optional: the day calendar weeks start on
when_router_down = "stop"    # optional: with the router down, claude stops, or "direct"

[[account]]
id      = "b6s7"        # written by switchboard, from the account's subscription; its token is tokens/b6s7
label   = "Work"        # what every screen shows
pool    = "d3wn"        # the id of the pool it's in
primary = true          # its pool's primary: the account the browser and the Claude apps are signed into
reserve = 0.1           # optional, on any account: the share of every window the router leaves unused
plan    = "ultra"       # optional, on any account: its plan, pro, max or ultra

[[account]]
id    = "k2m9"
label = "Personal"
pool  = "d3wn"
plan  = "max"

[[account]]
id    = "q4tx"
label = "Side"
pool  = "q7mt"          # the only account in its pool, so its primary

[[pool]]
id    = "d3wn"          # written by switchboard, as it makes the pool
label = "Default"       # the first pool's label, until you change it

[[pool]]
id    = "q7mt"
label = "Client"
dirs  = ["~/Code/client"]   # sessions started in here use the pool's accounts alone

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

[export]                # optional: off unless it's set
dir = "~/Dropbox/Switchboard"   # where the router exports the history, hourly, each Mac into a folder of its own

[usage]                 # optional: this is the default
by = "window"

[prices.plans]          # optional: a plan's price, in US dollars a month, in place of switchboard's
max = 90

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
  With pools, a session that goes direct goes out on its pool's accounts alone, and with two pools
  or more, a router that doesn't route by the config's pools counts as down.

## `[[account]]` keys

The config has one `[[account]]` table for each subscription, and at least one. Add and remove
accounts with [`setup`](commands.md#setup) or [`accounts`](commands.md#accounts), which write the
table, its `id` among it: switchboard makes each account's id from its subscription (see
[Accounts and their ids](how-it-works.md#accounts-and-their-ids)). The other keys are yours to
edit, by hand or with `accounts edit`.

| Key | Default | Description |
|---|---|---|
| `id` | Required | The account's id, which switchboard writes as it adds the account, and every record and command names it by. It's also the name of the account's token file. Don't change it by hand. |
| `label` | Required | What the account is shown as, at most 100 characters, and no other account's, ignoring case. It can't hold anything shaped like a token. A config edited by hand without one still loads, the account shown by its id. |
| `pool` | Required where the config has pools | The id of the pool the account is in. `accounts edit <id> --pool <id>` moves it. A config without `[[pool]]` tables has none, and an account naming one there is an error. |
| `primary` | The pool's first account | Marks the account the browser and the Claude apps are signed into: [the primary](how-it-works.md#the-primary), one for each pool. At most one account in a pool can set it. |
| `reserve` | `0` | The share of every window the router leaves unused: `0`, or more than `0` and less than `1`. [`accounts edit <id> --reserve <percent>`](commands.md#accounts) sets it, as a percentage. See [Reserves](how-it-works.md#reserves). |
| `plan` | None (unknown) | The plan the account's subscription is on: `pro`, `max` or `ultra`. The names the plans had before Anthropic renamed them, `max5x` for `max` and `max20x` for `ultra`, still load. |

The ids switchboard makes are 4 lowercase letters and digits, a letter first, or a character
longer where 4 clash. A config from before switchboard made ids loads too, and the router moves its
accounts onto ids as it first starts (see
[Upgrading to account ids](how-it-works.md#upgrading-to-account-ids)). Any `id` must follow these
rules:

- It starts with a letter or digit, followed by letters, digits, `-` and `_`.
- It's unique, ignoring case, because macOS gives `b6s7` and `B6S7` one token file.
- It isn't `auto`.
- It holds nothing shaped like a token.

Switchboard can't read an account's plan from Anthropic, so `plan` is the only way it knows the
plan. [`accounts edit <id> --plan <plan>`](commands.md#accounts) sets it. Switchboard uses the plan
in three places:

- It compares what the plan costs with what the account's use was worth at API prices.
- It sizes the account in the forecast of
  [where a week runs out](how-it-works.md#where-a-week-runs-out).
- It sizes the account in [`history`](commands.md#history)'s replay of a week.

An account's size is its plan's week, in Pro weeks: Pro 1, Max 5 and Ultra 10.

Without a `plan`, the account's plan shows as `-`, and nothing is priced against it. It's sized as
the plan most of the accounts have, and where no account has one, every account is the same size.

## `[[pool]]` keys

A pool is a set of accounts that sessions are routed across, fenced to directories or not. See
[Pools](how-it-works.md#pools). Once the router has moved the history to pools, the config has a
`[[pool]]` table for each pool, and at least one: the router makes the first, labelled `Default`, as
it moves (see [Upgrading to pools](how-it-works.md#upgrading-to-pools)). Make, edit and remove
pools with [`pool`](commands.md#pool), which write the table, its `id` among it. The other keys are
yours to edit, by hand or with `pool edit`.

| Key | Default | Description |
|---|---|---|
| `id` | Required | The pool's id, which switchboard writes as it makes the pool, and every record and command names it by. Don't change it by hand. |
| `label` | The pool's id | What the pool is shown as, at most 100 characters. It can't hold anything shaped like a token. `pool add` and `pool edit` keep it no other pool's, ignoring case. |
| `dirs` | None (unfenced) | The directories the pool is fenced to, each an absolute path or one starting `~/`, covering everything beneath it. A session started inside them is routed across this pool's accounts and no others. Without any, it's the unfenced pool, which serves every session started outside every fence. |

The ids switchboard makes for pools are 4 lowercase letters and digits, a letter first, random, as
no subscription stands behind a pool, and never one the accounts record holds. The config is
checked as it's read:

- **Ids:** a pool's id follows the rules an account's does, is unique among the pools, ignoring
  case, and is never an account's.
- **One unfenced pool at most:** two would leave a session outside every fence with two pools to
  be in. Every pool can be fenced, the first among them: a session started outside every fence
  then doesn't start.
- **No overlap:** no directory may be the same as, inside or around another, the same pool's or
  another's. Directories are compared a whole folder name at a time, so `~/Code/acme` doesn't
  cover `~/Code/acme-api`, and without regard to case, as macOS compares them.
- **Membership:** where the config has pools, every account names one of them in `pool`, so no
  account can be in two.
- **A pool can have no accounts:** a session started in its directories then doesn't start, rather
  than falling into the unfenced pool.

Write `dirs` on one line: `pool edit --dirs` can't change an array written across lines, and says
to edit it by hand.

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

## `[export]` keys

This key sets a folder the router exports the history into, as it starts and every hour after, as
[`export`](commands.md#export) would. The router makes the folder only where the folder it goes in
is there, so a disk that isn't mounted fails the export. See
[An export every hour](how-it-works.md#an-export-every-hour).

| Key | Default | Description |
|---|---|---|
| `dir` | None (no export) | The folder to export into: an absolute path, or one starting `~/`, your home folder. Each Mac exports into a folder of its own in it. Left empty, as left out, the router exports nothing. |

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

- Each price is a number from `0` to `1000000`.
- Each price holds for every day alike.
- A price you leave out keeps switchboard's.

| Key | Default | Description |
|---|---|---|
| `[prices.plans]` | Switchboard's prices | A plan's price in US dollars a month, under the plan's name: `pro`, `max` or `ultra`. A plan's old name, `max5x` or `max20x`, still loads, but not beside its new. |
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
`~/.local/state/switchboard/tokens/b6s7`. The file holds the token alone, and whitespace around the
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
  `b6s7 · Work hit its Session limit, back at Mon 18:10 — 3 sessions moved to q4tx · Side`.
- **Room again:** An account has room again, such as `b6s7 · Work has room again`.
- **Warnings:** A window passes the share set in `warning`. It's posted once until the
  window resets, such as
  `b6s7 · Work: Week at 91%`.
- **Moves:** Every other move, such as
  `session 18bb978f moved from b6s7 · Work to q4tx · Side (rescored after 1h 2m idle)`. These are
  off unless you turn them on.

It also tells you when an account's token nears the end of its year, whatever the keys say: once
when 30 days or fewer are left, and again at 7, such as
`b6s7 · Work: token expires 17 Nov 2026 · 21 days left — switchboard setup makes a new one`, or
`token expired …` once it's past. A router started with a token already within either tells only
the nearest. A restart doesn't tell either again, and a new token starts afresh.

Once there's a second [pool](how-it-works.md#pools), each notification is led by the label of the
pool its account is in, as `Acme · k3f9 · Acme 1 has room again`, the dashboard's own too. With one
pool, they read as above.

### Switchboard's own notifications

Notifications come from switchboard's own app, `Switchboard`, with switchboard's icon. A click on
one dismisses it. A release carries the app inside the switchboard binary, and writes it out to
`$XDG_DATA_HOME/switchboard/Switchboard.app`, else `~/.local/share/switchboard/Switchboard.app`,
again after each upgrade. A switchboard built from source, as `go install` builds it, has no app,
and posts through `osascript`, whose notifications are Script Editor's.

macOS asks once whether `Switchboard` may send notifications, when the router first starts on a
release with the app: after `setup` installs the service, or after `brew upgrade` brings that
release. The dashboard, posting without the router, asks too, before its first notification.
Notifications wait for your answer, up to 5 minutes, then go on through the app. The router never
waits on the question: it asks on the side, routing all the while, and stopping or restarting it
ends the asking. Turned off, they're turned on again in System Settings → Notifications →
Switchboard. [`service uninstall`](commands.md#service-uninstall-and-service-status)
removes the app. See
[Notifications that don't show](troubleshooting.md#notifications-that-dont-show).

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
