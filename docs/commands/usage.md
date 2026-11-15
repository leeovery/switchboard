# `switchboard usage`: every account's windows, as they stand

`switchboard usage` prints the windows of every account once, into the scrollback, with the time
they were read. You can hold it against one printed an hour before.

```bash
switchboard usage
```

```text
 SWITCHBOARD   Thu 1 Oct  14:42

● healthy  ·  6 sessions  ·  new sessions → ▲ side

WEEK

 work                     ▎      ┃              ╎   34%  out ~Sat

 personal   ◆                        ┃              66%  out ~Sat

 side       ▲                 ┃           ▏         78%  out ~Fri
────────────────────────────────────────────────────────────────────────────────
 Σ pool                           ┃▍                59%  1.2 of 3 left

┃ even pace: where you would be now, used evenly
╎ cap: work at 95%
```

The bars are drawn in colour, so their fill doesn't show in the text above. The 5-hour window and
the week of each model follow the week, cut here.

`usage` reads the status document of the router while the router answers. Otherwise it probes
each account itself. Off a terminal, such as in a pipe or the shell of an agent, it prints the status
document as JSON instead, the same as [`status --json`](status.md).

## Flags

```bash
switchboard usage [--5h] [--week] [--<model>] [--by window|account] [--pool <id>] [--probe] [-r] [--json | --pretty]
```

| Flag | Default | What it does |
|---|---|---|
| `--5h`, `--week` | every window | Prints only that window, with the header and the key line. Given together, they print in this order: the week, the 5-hour window, then the week of each model. |
| `--<model>` | | Prints the week of one model, such as `--fable` for the Fable week. There's a flag for each model with a window of its own. |
| `--by window\|account` | `window` | Lays the windows out one window at a time, comparing accounts, or one account at a time. Overrides `by` in `[usage]` in the config. |
| `--pool <id>` | the pool the verbs act on | Prints the accounts of that pool. See [Commands](../commands.md). |
| `--probe` | | Probes every account, even while the router runs. |
| `-r`, `--refresh` | | Has the router read every account it may first, as `R` does on the dashboard, and waits up to ten seconds. Without the router, or with `--probe`, every account is probed anyway. |
| `--json` | off a terminal | Prints the status document as JSON, even on a terminal. The window flags and `--by` don't change the JSON. |
| `--pretty` | on a terminal | Prints the bars, even off a terminal. |

A flag for a window that no account has fails, and names the windows there are. For example,
`--5h` fails if every account is on a verified Ultra plan. An Ultra account not yet verified keeps
its 5-hour window, and `--5h` works.

Off a terminal, `--pretty` draws the bars with no colour unless `CLICOLOR_FORCE` asks for it. They're
as wide as `COLUMNS` says, or else 80 columns.

## What it prints

From the top, `usage` prints:

1. A header, with when it read and how the router stands, such as
   `● healthy  ·  6 sessions  ·  new sessions → ▲ side`. While a pin holds, it adds
   `pinned to personal`.
2. A bar for every window of each account.
3. The `Σ pool` row, last in each window.
4. A key line saying what the marks mean.

Each row is an account, in the order the router uses, which is the order of the config once the
router has taken it up.

| Part | What it shows |
|---|---|
| Label | The label of the account. `◆` marks the primary, and `▲` the account new sessions go to. |
| Bar | How much of the window is used. `┃` marks where even use would have put it by now. `╎` marks the cap: 100% less the reserve. |
| Percentage | How much of the window is used. |
| Words | The first state in the next table that holds. |

### The words beside each bar

| Words | When they show |
|---|---|
| `limit · back 15:54` | The limit of Claude holds the window. |
| `at cap · back 17:10` | The window has reached the cap, so the router holds the account back. |
| `out ~16:05` | The window runs out before it resets. Within a day, the time shows. |
| `out ~Sat` | It runs out more than a day ahead, so the day shows only, because a forecast days out can miss by a day. |
| `out ~Thu 12 Nov` | It runs out on the same weekday as today, a week on, so the date shows. |
| `no 5-hour limit` | The account has no 5-hour window, such as a verified Ultra account. It shows in place of the bar, or as `no limit` where room is short. |
| `not started` | The 5-hour window hasn't started yet. |
| `19%/day till Sun 21:00` | Otherwise, the allowance: how much you can spend an hour or a day and still last to the reset. |
| `34% till 16:30` | In place of the allowance, under an hour from the reset of the 5-hour window, or under a day from the reset of a week: the room itself. |

A week runs out by its forecast, and the 5-hour window by its recent rate. See
[Limits and routing](../limits-and-routing.md).

### Accounts with no reading

| The account | What its row shows |
|---|---|
| Has no usable token | `no token`, with empty bars. |
| Has a token the router found to be for another subscription | `wrong token`, with empty bars. |
| Has never been read | `not read yet`, with empty bars. |
| Has a refused token, or its last read failed | Its last reading, faded, such as `not read since 14:20`. |

### The Σ pool row

The `Σ pool` row sums the room of the open accounts: every account in the pool, or only the pinned
ones while a pin holds. It shows wherever there's more than one account.

- **Its percentage** is the mean use of those accounts, each as its own row shows it. An account
  without a usable token, never read, or with a refused token, is left out.
- **Its words** say the room left before the caps, in accounts, such as `1.5 of 3 left`. While a pin
  holds, they add ` · pinned`.
- **A window held by fewer than two of the accounts,** but not all of them, has no Σ pool figure.
  Side by side, that part of the row is blank. Stacked, the section has no Σ pool row.
- **An account outside the open accounts** has its label and `◆` drawn faintly.

### Which windows show

Every window shows, used or not. The Fable week shows on every account, at 0% as at 40%.

`usage` learns which windows an account has from the answers of the API, never from its plan. A
verified Ultra account reports no 5-hour window. Stacked, a window leaves out an account that
doesn't have it. A window no account has doesn't show at all. An account not read yet may have
any window, so it keeps its rows.

### A token near the end of its year

Under the key line, after a blank line, each account whose token is up within 30 days, or is up
already, has a line:

```text
work  token expires 17 Nov 2026 · 21 days left — switchboard setup makes a new one
```

On the last day it says `· last day`, and once the year is up, `token expired 17 Nov 2026`. Other
accounts add nothing.

## In a pool

Once the config has pools, `usage` prints the accounts of one pool, and the `Σ pool` row sums only
them. Once there's a second pool, the text names the pool it read, such as `pool Acme`.

| If | `usage` |
|---|---|
| `--pool` names a pool the config no longer has | Refuses it, because the router routes nothing there. |
| The router still runs from before pools | Probes the accounts of the pool itself, and says `the router doesn't route by pools yet, as it started before them: restart it with switchboard service restart`. |

## How the width shapes it

| Part | Wide terminal | Narrow terminal |
|---|---|---|
| By window | The windows sit side by side, a column each. That needs bars of 12 cells and short words: from 86 columns with two windows and short names, or 122 with three. | Each window is a section of its own, stacked. |
| By account | The name of each account sits beside its first bar, from 60 columns with short names. | The name is on a line of its own, above the bars. |
| Header | One line, if the date and the whole status fit, every part said in full. That's from 84 columns for a healthy router with short names. | Two lines, so one line never says less than two would. |
| Key line | One line, if it fits beside the names. | Two lines. The second names the cap, if a bar draws one. |
| Words | In full. | Short, such as `back 15:54`, `out ~Sat` and `19%/day · Thu`. |
| Name column | As wide as the longest label, up to 24 cells. | A longer label is cut with `…`. |

As the terminal narrows, the bars shorten to 12 cells first, then the words go short.

In short words, a time on another day shows the day only. A run-out or `back` on the same weekday as
today, a week on, shows the date instead, such as `out ~12 Nov` or `back 12 Nov`. An allowance
keeps its day, because a date doesn't fit beside the rate.

The name column never moves as `▲` moves between accounts.

## Colour

`usage` draws in the theme of the dashboard, brought down to what the terminal shows, with no
background of its own. See [Dashboard](../dashboard.md) for themes. With `NO_COLOR` set, or on a
terminal that shows no colour, a bar fills with `█` and leaves its track as spaces. Names and
percentages stay bold.

## Examples

```bash
switchboard usage                  # every window, every account
switchboard usage --5h             # the 5-hour window only
switchboard usage --by account     # each account's windows together
switchboard usage --probe          # read every account from the API, whatever the router says
switchboard usage -r               # have the router read every account first, after a reset made by hand
```

## Related

- [status](status.md): the same reading as text or JSON, with the sessions and the pin.
- [Dashboard](../dashboard.md): the same windows, live, full screen.
- [Limits and routing](../limits-and-routing.md): what the windows, caps and forecasts mean.
