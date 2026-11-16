# Trends: the shape of your use over time

Trends charts your use over time, across every account. Look at it to see how each week is
burning down, when in the day you work, and how the last 12 months went. It's a page of sections
you turn on and off, with a burn-down card for each account at the top.

<div align="center">
<img src="../../art/graphs.webp" alt="Trends: a burn-down card for each account, its room over the week, two to a row, work capped till 17:10, personal's and side's weeks running out at their pace, each heading to its ✕, and the room of the pool under them" width="840">
</div>

*Trends at 160 columns: a card for each account, each heading for its `✕`.*

## Sections on Trends

Trends has seven sections, always in this order. Press a section's key to show or hide it, at any
size. The dashboard remembers which show.

| Section | Key | At first | What it is |
|---|---|---|---|
| WEEK | `w` | shown | A burn-down card for each account, over its week |
| POOL | `o` | hidden | The room of the pool over Runway's week, each account stacked |
| GLANCE | `g` | hidden | A row for each account and for `Σ pool`: the week's room as a sparkline, with the words `usage` prints |
| WHEN YOU WORK | `h` | hidden | Requests by the hour of the day and the day of the week, over the last four weeks |
| YEAR | `y` | hidden | The last 12 months, a square a day and a column a week, with their totals |
| BY MONTH | `m` | hidden | A row a month: its requests, worth, worth against what the plans cost, limits, and worth by model family |
| MODELS BY DAY | `d` | hidden | The worth a day of each model family over the last eight weeks, a strip a family |

- Trends has no period. The cards show this week, YEAR and BY MONTH the last 12 months, and
  MODELS BY DAY the last eight weeks.
- If you hide every section, the first row says which keys bring them back.
- YEAR, BY MONTH and MODELS BY DAY came from the History tab, which is gone. Its weeks are now in
  Detail on [Accounts](accounts.md), and its tokens in Tokens there. The verdict on how many
  accounts you need is in [`switchboard history`](../commands/history.md).

## The burn-down cards

Each card charts one account's room over its week, from the week's start to its reset. Over
the chart are the account's name, its marks and its words, such as
`66% left  ·  capped 14:31  ·  back 17:10`.

The room left comes first. If the reading is stale, why comes next, as `usage` says it:
`subscription lapsed` or `token refused` in red, or `not read since 14:20`. The rest are the words
[Runway](runway.md) gives the account at the same width. A hold shorter than one column of Runway
can go unsaid, on a card as on Runway. A wider terminal can bring it out, and so can collapsing the
labels on Runway with `a`.

| Mark | Means |
|---|---|
| Solid fill, `▆` | The room the window had, up to now. |
| Dotted line, `⠂⠄⡀` | Where the week is heading, from now to the reset. |
| `✕` | Where the window runs out. |
| Faint dotted line, `⠠ ⠠` | The reserve: the room the cap leaves. It's not drawn while a pin spends the reserve. |
| Faint `│` | Now. |

The dotted line goes straight to where the week is forecast to be, if the router spreads your use
as it would. That's the pace Runway goes by, and [Routing rules](../reference/routing.md) explains
it. The line falls to where the week runs out, then runs along the floor it meets, its cap or
empty, to the reset. Or it ends at the room the week will have left when it resets. If a cap or a
limit already holds the account, `✕` marks the column where that hold began, if it's known.

A card takes its colour from its week's state, as a lane on Runway does:

| If the week | The card is drawn in |
|---|---|
| lasts to its reset | the ramp's first colour |
| runs out at its pace, or its cap holds it | the attention colour |
| is held by a limit, or its token is refused | red |

- An account not read yet shows its words from Runway and an empty chart, with only now drawn.
- An account the router doesn't have yet, just added to the config, shows its label only.
- A stale account is faded, and its words say why. One that a pin leaves out of the pool is faint.

The cards follow the config's order, two side by side if both fit at 35 cells or more. Every
card is as wide as the others, so the same burn draws the same slope on each. With three accounts,
the third sits under the first, as wide. Each card keeps its own clock: its days and its now fall
where that week puts them, not where the card beside it does.

Under the cards, `Σ pool` gives the pool's room in the week, such as `week 1.2 of 3 left`.
At the foot, over the key line, a legend names the marks.

Trends charts the week that all models share, as Runway does. A model's own week has no card, and
neither has the 5-hour window, which the Overview shows.

<div align="center">
<img src="../../art/dashboard.webp" alt="One account's dashboard: work alone on the Overview, its usage, routing trace and LATELY, then its week's card on Trends, then Runway, its lane across the week, with COMING UP and LATELY under it" width="840">
</div>

*With one account, its card takes the width, and there's no `Σ pool` line. Here `work` alone, on
the Overview, Trends and Runway.*

## POOL

POOL charts the room of the pool over the week Runway shows, from a day before now.

- Each account's room stacks from the foot, in its own colour, so the top edge is their total.
- Before now, each account shows its room as it stood.
- From now, drawn faint, each shows where it's heading: to where it runs out, along its floor to
  its reset, then full.
- The axis from Runway runs under it, and a key names each account's colour.

Without colour, POOL draws only the total, because the accounts can't be told apart.

## GLANCE

GLANCE lists every account in a few rows, as [`usage`](../commands/usage.md) does:

- The week of each account is a sparkline from its start to its reset. It's faint from now, where
  it's heading, with `✕` where it runs out.
- Then come the room left now, such as `66%`, and the words `usage` gives the week. For example,
  `out ~Sat` or `19%/day till Sun 21:00`.
- The `Σ pool` row gives the sparkline and room of the pool.

## WHEN YOU WORK

WHEN YOU WORK shows when your requests come. It has a row for each weekday and the 24 hours
across. The rows start on the first day of the week that
[`week_starts`](../configuration.md) sets.

- Each hour is shaded by its requests, averaged over the four whole weeks before today.
- It counts the requests that spend quota and went upstream, each in the local hour it arrived.
- The heading names the weeks counted, such as `the 4 weeks to Wed 30 Sep`. If the ledger holds
  fewer, it counts fewer.
- With no weeks recorded, the section says `nothing recorded yet`.

The shades are those of YEAR: one for an hour with nothing on it, and four for the quarters of the
active hours. The key under the grid gives their bounds.

## YEAR

YEAR shows the last 12 months, a square a day and a column a week. Each day is shaded by how many
requests it made. Worth never sets the shade, because worth shifts with the models used and their
prices.

- A day with nothing on it has a shade of its own.
- The active days are split into quarters, and the key under the grid gives their bounds.
- The grid shows as many of the latest weeks as the width holds.

Its totals, LAST 12 MONTHS, sit beside the grid if the width holds both, and under it if not:

- the requests, sessions and limits hit;
- the worth, what the plans cost, and the worth against it;
- the busiest day, and the longest run of days in a row.

### One version or one account

Press `v` to cycle what YEAR and BY MONTH show: all use, then each model version, then each
account.

- One version shows in four steps of its own colour.
- On one account, `←` and `→` step between the accounts.
- The shades, their key and the totals scale to what's shown, and BY MONTH with them.
- The key line says what's shown, such as `v split: all`, if it has room.

For one version, YEAR counts each session once. There's one exception. A day may have a summary
written before the ledger kept sessions by their ids, and its lines may have gone since. For such
a day, YEAR adds the count of the day, because nothing tells those sessions from another day's.

## BY MONTH

BY MONTH has a row for each of the last 12 months, with:

- its requests;
- its worth at API prices;
- its worth against what the plans cost that month;
- the limits reached;
- a bar of its worth against the largest month, split by model family, each in its colour.

The bars take the width left. The month still running says `so far` after its bar. The month the
ledger began in names the day it began, such as `from the 16th`. If some accounts have no `plan`,
the worth is set against the plans that are known, as the heading says. For one version, its
share of the month stands in place of the worth against the plans.

## MODELS BY DAY

MODELS BY DAY covers eight weeks: this week so far and the seven before it, two cells a day.

- Each model family has a strip of its worth a day at API prices, across every account.
- A strip has four steps of its family's colour, and every family shares one scale.
- Each strip ends with the share of the eight weeks' worth that family took.
- The strips show as many of the latest weeks as the width holds.

The days of each account on its own are in Detail on [Accounts](accounts.md).

## Keys on Trends

| Key | Does |
|---|---|
| `w` `o` `g` `h` `y` `m` `d` | Shows or hides a section: WEEK, POOL, GLANCE, WHEN YOU WORK, YEAR, BY MONTH and MODELS BY DAY. |
| `v` | While YEAR or BY MONTH shows, cycles what they show: all use, each model version, then each account. The key line lists it if it has room. |
| `←` `→` | While YEAR or BY MONTH shows one account, steps to the account before or after. It works with BY MONTH shown alone too. |
| `j` `k`, `PgDn` `PgUp`, the wheel | Scrolls the page, if its sections don't fit. |

## Room on Trends

The terminal's size never decides which sections show, only how much room each gets.

- At their least, each card's words share its label's row, and each chart is 5 rows.
- Spare rows go first to the cards' words, a row of their own.
- Then they go to the charts on the cards and in POOL, every one the same height, up to 19 rows.
- The layout changes only on a key, a resize, a change to the config, or a new read of the
  ledger's days. It never changes as the numbers move.
- The charts move on with the clock, a column at a time.

If the sections don't fit even at their least, the page scrolls whole, a row at a time. The row
over the key line says what's out of view, such as `▲ 3 more above · ▼ 12 more below`. The legend
stays at the foot. A heading shows only with the first row under it.

| Screen | The cards | Each chart |
|---|---|---|
| 160 × 37, three accounts | Two a row, their words on a row of their own | 8 rows |
| 108 × 44, three accounts | Two a row | 12 rows |
| A phone | One a row | 11 rows, or 7 with four accounts |

### Narrow and wide screens

- **The cards:** if two don't fit side by side, for example on a phone, there's one card a row.
- **GLANCE:** the sparklines grow with the width, from 16 cells. The room left and the words keep
  the room they need, lined up down the rows. Narrower, the words take their short forms, then the
  names narrow.
- **GLANCE beside WHEN YOU WORK:** the two sit side by side if both fit whole with 10 cells
  between. With names such as `personal`, that's from 123 columns. GLANCE takes the spare width that
  the whole hours leave. Narrower, WHEN YOU WORK comes under GLANCE.
- **WHEN YOU WORK:** each hour square grows with the width, all as wide as each other. Any cells
  over are left blank at the end of the row. The hours over the grid follow their columns.
  On a narrow screen, an hour is a square with no gap.
- **YEAR:** under the grid, the totals spread to end where the grid ends. On a phone, the totals
  the width leaves off go on a row under the rest, so none goes unsaid.
- **MODELS BY DAY:** on a narrow screen, the name of each family goes on the row over its strip.

## Related

- [Runway](runway.md): the same weeks as lanes, with what comes next.
- [Accounts](accounts.md): each account in depth over the period, its weeks among it.
- [`switchboard history`](../commands/history.md): how many accounts your use needs.
