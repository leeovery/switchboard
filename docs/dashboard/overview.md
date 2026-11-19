# Overview: every account on one screen

The Overview shows how much of each window every account has used, and how the router stands with
each account. Look at it to see which account takes new sessions, which is near a limit, and what
resets next. The page fits on one screen: only USAGE scrolls, within itself, if its accounts don't
fit.

<div align="center">
<img src="../../art/overview.webp" alt="The Overview: every account's usage, routing, sessions, COMING UP and LATELY" width="840">
</div>

*The Overview: USAGE, ROUTING, COMING UP and LATELY, with SESSIONS hidden.*

The page has five sections, always in this order:

1. **USAGE:** the windows of every account.
2. **ROUTING:** how the router stands with each account.
3. **SESSIONS:** the sessions the router has routed in the last hour, hidden until you press `s`.
4. **COMING UP:** what happens next.
5. **LATELY:** what the router has done.

## USAGE: the windows of every account

USAGE is the block that [`usage`](../commands/usage.md) prints. It has a row for each account, and
a `Σ pool` row for the pool's accounts together. Each window is a bar, with the share used and a
few words on where it's heading, such as `out ~Sat` or `at cap · back 17:10`.

The key line under the block names the marks:

| Mark | Means |
|---|---|
| `┃` | even pace |
| `╎` | the cap |
| `◆` | the primary |
| `▲` | where new sessions go |

- **The marks** `◆` and `▲` stand at the end of the name column, so they line up down the list.
  ROUTING and SESSIONS line their marks up under them. Every list of accounts in a column does the same.
- **A pinned-out account,** one that a pin leaves out of the pool, is drawn faint.
  ROUTING may still show it as `● open`.
- **The `Σ pool` row** counts only the accounts that have the window, for example `1.2 of 3 left`.
  If fewer than two accounts have the window, and not all of them, the row shows nothing for it.
  An account with an ended subscription or a refused token adds no room, because the router sends
  it nothing.
- **With one account** there's no `Σ pool` row. The heading says the room left in each window
  instead, if the heading has room for it.

### Windows an account doesn't have

An account without a 5-hour window, such as a verified Ultra account, has no bar there. The words
in its place depend on where you look:

| Where | Says |
|---|---|
| USAGE, side by side | `no 5-hour limit`, or `no limit` if room is short or USAGE is by account |
| USAGE, stacked | nothing: the account is left out of the 5-HOUR section |
| Accounts' Detail and Sessions | `no limit`, beside the window name |
| The session page and the routing card | `no 5-hour limit` |

A window that no account has isn't shown. If only the week is left, USAGE stacks it at the full
width wherever it fits.

### Model windows

```text
FABLE

 work            ▌           ┃            ╎  12%  25%/day till Sun 21:00
 side       ▲  ▏           ┃                  4%  25%/day till Mon 10:00
```

Some models have a week of their own, for example Fable. USAGE shows the Fable week only if an
account has used it, the same way it shows the 5-hour window only if an account has one. Above,
work and side have used Fable this week, and personal hasn't.

| If | USAGE shows |
|---|---|
| An account has used the Fable week | Its bar, percentage and words. |
| A limit names an account's Fable week | The FABLE section, even at 0%, with that account's bar and words, such as `limit · back Sun`. |
| The FABLE section shows, and any limit holds an account | That account's words, such as `limit · back Sun`. |
| An account hasn't used the Fable week, side by side | `no Fable use`, or `no use` if room is short or USAGE is by account. |
| An account hasn't used the Fable week, stacked | Nothing: the account is left out of the FABLE section. |
| No account has used the Fable week, and no limit names it | No FABLE section at all. |

The section goes once the Fable week of every account resets to 0 and no limit names it. It
comes back with the first use. The same holds on these views:

- Detail on [Accounts](accounts.md);
- the Switchboard on [Sessions](sessions.md);
- [the session page](session-page.md).

The [`usage`](../commands/usage.md) command prints the Fable week the same way. If the
Switchboard's lanes sit side by side, they show only the week and the 5-hour window. Stacked, the
lanes show the Fable week too.

[The Log](log.md) and the JSON of every command keep every model window, used or not.

## ROUTING: how the router stands with each account

ROUTING says in words how each account stands with the router. For example:

- `● under pressure · out ~16:05`
- `■ limit reached · back 15:54`
- `○ idle · window starts at its prime, 16:20`
- `■ subscription lapsed · its subscription needs renewing`
- `✕ its token is another subscription's · switchboard accounts token b6s7`

`subscription lapsed` means the account's subscription has ended. `token refused` means Anthropic
refused the account's token. The router sends such an account nothing until a read comes back
clean.
To bring the account back, see
[Troubleshooting: an account's token or subscription](../troubleshooting/tokens.md).

`its token is another subscription's` means the token file for the account holds the token of
another subscription. USAGE and Runway call it `wrong token`. To fix it, see
[`switchboard accounts`](../commands/accounts.md).

Each account row also shows:

- the sessions on the account;
- a trace of its requests a minute over the last 30 minutes, with the rate in the last minute.

Every trace on the page shares one scale, including the trace of each session, so you can compare
any two.

The heading gives the routing mode. It says `auto`, or `pinned to personal since 14:02   p back to
auto`, or only `pinned to personal` if the rest doesn't fit.

### Pinned accounts

While a pin is in force, each account it names says `● pinned` beside its state, if there's
room. Room is tighter on some rows:

| If | ROUTING |
|---|---|
| The state says when the account is back or when its pressure runs out | Shows the mark only if that time fits beside it too. |
| The state gives a prime time | Drops the prime time for the mark. |
| The mark doesn't fit, but the state can say it | Says it in the state, for example `● at its cap · pinned`. |
| Neither fits | Names the pinned accounts only in the heading. |

## SESSIONS: what ran in the last hour

SESSIONS lists the sessions the router has routed in the last hour. The account whose sessions
send the most requests a minute comes first, with all its sessions together, the busiest first.
Each session shows:

- its short id, bright while the session is busy, and its project, such as `c61b mobile`;
- its model;
- when it was last seen;
- what it's doing, such as `↑ ask` or `↓ ~1.2k`;
- why it's on its account, such as `moved from work 14:31: work reached its cap`;
- its own trace, lined up with the traces in ROUTING.

To open the session page, click a session row.

### Rows that stay put

The sessions shown stay on their rows until the layout changes, on a key or a resize. Nothing
reorders as the rates change.

| When | SESSIONS |
|---|---|
| A session starts | Counts it in a row such as `+5 more`. |
| A session ends | Leaves its row blank. |
| Every row is taken | Gives the last session shown up to the `+N more` row. |
| Every session it kept has ended | Says `no sessions running`. |
| None ran as the layout was made, and one starts | Shows it on a row where it fits whole, or counts it in `+N more`. |

## COMING UP and LATELY: next and last

**COMING UP** is what happens next, soonest first, for example
`16:30  personal's 5-hour resets  in 1h 48m`. A run-out row is drawn in the attention colour of the
theme.

**LATELY** is what the router has done, newest first. Each event has a mark, for example
`14:31  ╎ work reached its cap: 3 sessions moved to side`.

- **A move** made by a limit, a cap, a refusal or a [hold](../glossary.md) folds into the line of
  the event that caused it.
- **A session** is named by the folder it runs in, as in `api moved personal → work`. If the folder
  isn't known, the short id names it.
- **A line that doesn't fit** takes a shorter form, such as `work at its cap: 3 moved to side`.
- **A wrong token** reads `work's token is another subscription's`, or `work: wrong token` if
  that doesn't fit.

## By account

Press `b` to make USAGE one block for each account, with its windows as rows. From 150 columns,
the account's ROUTING sits beside its block. The `Σ pool` block comes last. Until you press `b`,
the grouping follows `[usage] by` in [Configuration](../configuration.md).

## Showing and hiding sections

Each section key shows or hides its section, at any size. The size of the terminal never decides
which sections show. It decides only how much room each one gets:

- **When rows run short,** sections fold to their headings. A folded heading still says what the
  section holds.
- **Before any section folds,** the accounts in USAGE close up, losing the blank row between each
  two. They close up only if that keeps a section open.
- **Sections fold in this order:** SESSIONS, then COMING UP and LATELY, then ROUTING.
- **A folded section opens again** wherever it fits, the last folded first.
- **COMING UP and LATELY open short** if their heading and two lines fit, but not three lines.
  Opened short, a section shows its first two lines straight under its heading. Side by side, both
  open short together.
- **Spare rows** grow the open sections, up to their full size. SESSIONS takes them first, a whole
  session at a time.

USAGE never folds. With every other section folded, it shows as many accounts as it can. It closes
up its accounts first, then drops the blank rows under its headings, then its key. If the accounts
still don't fit, USAGE scrolls within itself, and `↑` and `↓` scroll it.

## Room by width

USAGE, ROUTING and SESSIONS lay out by the columns they have. The numbers are for names as short
as those in the stills.

| Part | Wide | Narrow |
|---|---|---|
| USAGE | Windows side by side from 84 columns with two windows, each bar at least 12 cells. With rows to spare, the windows stack instead, a section each at the full width. | The windows stack, as `usage` stacks them. |
| ROUTING and SESSIONS | One row an account and one row a session, from 83 columns with a name column of 16. | Two rows each. |
| COMING UP and LATELY | Side by side, from 95 columns. | LATELY goes under COMING UP. |

- **The name column** is as wide as the longest name, up to 24 cells of label, if the screen has
  room.
- **A bar grows** only after the words beside it have gone from their short form to their full one.
- **The project in SESSIONS** is the last part of the session folder, cut to 12 characters as the
  Log cuts it. Narrower, it's cut to 6 characters, then left out.
- **The reason in SESSIONS** is cut short if it doesn't fit, and hidden if fewer than 12
  characters would show. The sample reason reads whole from 155 columns, and hides under 102.
- **SESSIONS** shows as many sessions as fit, and a `+N more` row for the rest. It folds only if
  one session and that row don't fit.
- **The key line** says `▲ new` if there's no room for `▲ new sessions go here`.

On a phone, see [On a phone](phone.md).

## Keys on the Overview

| Key | Does |
|---|---|
| `u` `r` `s` `c` `l` | Shows or hides USAGE, ROUTING, SESSIONS, COMING UP or LATELY. |
| `b` | Groups USAGE by window or by account. |
| `↑` `↓` | Scroll USAGE a row, as `j` and `k` do, if its accounts don't fit. `?` lists them only then. |

The keys every page shares are on [Every page](every-page.md).
