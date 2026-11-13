# Dashboard

The dashboard keeps every account's usage on screen, live. It has seven tabs and a page for each
session, and shows the router's sessions and its record of every request. To open it, run
`switchboard dashboard`.

- [Terms](#terms)
- [Overview](#overview)
- [Accounts](#accounts)
- [History](#history)
- [Sessions](#sessions-1)
- [Runway](#runway)
- [Graphs](#graphs)
- [The Log](#the-log)
- [A session's page](#a-sessions-page)
- [On a phone](#on-a-phone)
- [Every page](#every-page)
- [Running it](#running-it)
- [Keys](#keys)
- [The routing card](#the-routing-card)
- [The key line](#the-key-line)
- [Clicks](#clicks)
- [What's remembered](#whats-remembered)
- [Themes](#themes)

## Terms

This page uses these terms throughout:

- **Label:** what every page calls an account, such as `work`. The accounts aren't numbered: they
  keep the config's order. Commands name an account by its id instead. See
  [Accounts and their ids](how-it-works.md#accounts-and-their-ids).
- **Window:** a span Claude counts an account's usage over. Each account has a week that every model
  shares, a 5-hour window, and a model's own week where the model has one. A verified Ultra account
  has no 5-hour window: switchboard learns that from its answers, never from its plan. An account
  not read yet, or without a usable token, counts as having it.
- **Limit:** Claude's own limit on a window. An account at a limit can't take the requests that
  limit counts until it lifts.
- **Cap:** the point where an account's reserve starts, 1 less its `reserve`. The router passes
  over an account at its cap. See [Reserves](how-it-works.md#reserves).
- **Pool:** the accounts new sessions can go to now. That's every account on auto, or only the
  pinned accounts while the pin holds. Where the config has [pools](how-it-works.md#pools), the
  dashboard shows one, and its pool is that pool's.
- **Pin:** your choice of where sessions go, set with the routing card or with
  [`pin`](commands.md#pin).
- **The primary:** the account your browser and the Claude apps are signed into. See
  [The primary](how-it-works.md#the-primary).
- **Worth at API prices:** what the same tokens would have cost through the API.

## Overview

The **Overview** shows every account's windows and how the router stands with each. It fits on
one screen that never scrolls.

<div align="center">
<img src="../art/overview.webp" alt="The Overview: every account's usage, routing, sessions, COMING UP and LATELY" width="840">
</div>

### Sections

The Overview has five sections, always in this order. Each key shows or hides its section, at any
size.

| Section | Key | At first |
|---|---|---|
| USAGE | `u` | shown |
| ROUTING | `r` | shown |
| SESSIONS | `s` | hidden |
| COMING UP | `c` | shown |
| LATELY | `l` | shown |

The terminal's size never decides which sections show. It decides only how much room each section
gets:

- When rows run short, sections fold to their headings. Each folded heading still says what the
  section holds.
- Before a section folds, USAGE's accounts close up, the blank row between each two giving way.
  They close up only where that keeps a section open.
- Sections fold in this order: SESSIONS first, then COMING UP and LATELY, then ROUTING.
- USAGE never folds. Short of its rows, with every other section folded, it shows as many
  accounts as it can: its accounts close up, then the blank rows under its headings give way,
  then its key, as far as shows the most, and where they still don't fit, it scrolls within
  itself.
- A folded section opens again wherever it fits, the last folded first.
- Spare rows grow the sections that aren't folded, up to their full size, even while others are
  folded. SESSIONS takes them first, a whole session at a time.

### USAGE

USAGE is the block that [`usage`](commands.md#usage) prints: every account's windows, and the
pool's.

- Wherever its windows fit side by side, it's the matrix: from 86 columns with two windows and
  short names, its bars at least 12 cells. Given the room, it stacks the windows at the full width
  instead, their bars as long.
- Narrower, as on a phone, it stacks the windows as `usage` stacks them.
- Its name column is as wide as the longest name, up to 24 cells of label, wherever the screen has
  room. A bar only grows as the screen widens: the words go from short to full first.
- Each name's marks, `◆` and `▲`, stand at the end of the name column, so they line up down the
  list, and under them ROUTING's and SESSIONS'. Every list of accounts in a column does the same,
  Compare's rows on a phone among them.
- An account out of the pool, under a pin, is drawn faint.
- An account without a 5-hour window reads `no 5-hour limit` in place of its bar, or `no limit`
  where the room is short, and by account, where the window's label beside it names the window. In a
  stacked 5-HOUR section, it's left out. A window no account has isn't shown, and with only the week
  left, USAGE stacks it at the full width wherever it fits. Accounts' Detail and Sessions' lines say
  `no limit` beside the window's name, and a session's page and the routing card `no 5-hour limit`.
- The pool's room counts only the accounts that have the window, such as `1.2 of 3 left`. Where
  fewer than two of them have it, and not all, the pool shows nothing for it.

Each model's own window, such as Fable's week, is hidden until you press `w`. That goes for every
view that shows the windows as they stand now: USAGE, Accounts' Detail, Sessions' Switchboard and
a session's page. A hidden window still shows while it nears its limit on any
account: at the limit, at the cap, running out, or within 10 points of running out. `usage`,
History and the Log always show every window.

The key line under the block names the marks:

| Mark | Means |
|---|---|
| `┃` | even pace |
| `╎` | the cap |
| `◆` | the primary |
| `▲` | where new sessions go |

Where the key line has no room for `▲ new sessions go here`, it says `▲ new`. With one account,
which has no pool, USAGE's heading says the room it has left in each window, wherever the heading
has room for it.

### ROUTING

ROUTING says, in words, how each account stands with the router. For example:

- `● under pressure · out ~16:05`
- `■ limit reached · back 15:54`
- `○ idle · window starts at its prime, 16:20`
- `✕ its token is another subscription's · switchboard accounts token b6s7`, where the account's
  token file holds another subscription's token, which USAGE and Runway call `wrong token` (see
  [A token that's another subscription's](how-it-works.md#a-token-thats-another-subscriptions))
- `● pinned`, on each account the pin names, where it has room beside the account's state, and,
  where the state says when it's back or when its pressure runs out, only where that time fits
  beside the mark too. A prime's time gives way to the mark. Where the mark doesn't fit, a
  state that says the pin says it in its words, as `● at its cap · pinned`; otherwise only the
  heading names the pinned accounts.

For each account, ROUTING also shows:

- the account's sessions
- its requests a minute over the last 30 minutes, drawn as a trace, with the last minute's rate.

The section's heading gives the routing mode: `auto`, or
`pinned to personal since 14:02   p back to auto`, or `pinned to personal` alone where the rest
doesn't fit.

Every trace on the page shares one scale, a session's trace included, so you can compare any two.

### SESSIONS

SESSIONS lists the sessions the router has routed in the last hour, busiest first. The account
whose sessions send the most requests a minute comes first, with all its sessions together, the
busiest of them first. Each session shows:

- its id, bright while the session is busy, and its project, the last part of its directory, cut
  to 12 characters as the Log cuts it, `c61b mobile`; on narrower screens the project narrows to
  6 characters, then leaves
- its model
- when it was last seen
- what it's doing, such as `↑ ask` or `↓ ~1.2k`
- why it's on its account, such as `moved from work 14:31: work reached its cap`, cut short where
  the screen hasn't room for it all (that one reads whole from 155 columns with names as short as
  the sample's, and later with longer ones), and hidden where it would have fewer than 12
  characters, under 102 columns with the sample's names
- its own trace, lined up with ROUTING's.

SESSIONS shows as many sessions as there's room for, and a row such as `+5 more` for the rest. It
folds only when there isn't room for one session and that row. The sessions it shows stay put
until the layout changes, on a key or a resize: a session that starts joins `+5 more`, one that
ends leaves its row blank, and nothing reorders as the rates change. When every row is already
taken, the last session shown gives way to the `+N more` row. Once every session it kept has
ended, it says `no sessions running`. When none were running as the layout was made, a session
that starts shows on the row where it fits whole, and otherwise is counted in `+N more`.

ROUTING takes a row for each account, and SESSIONS a row for each session, wherever an account's
name, its state in at least 20 cells, its sessions and a trace of at least 24 fit on one row, from
83 columns with a name column of 16. Narrower, each takes two.

### COMING UP and LATELY

COMING UP and LATELY sit side by side where both fit, from 95 columns, and LATELY goes under
COMING UP where they don't.

- **COMING UP** is what happens next, soonest first, such as
  `16:30  personal's 5-hour resets  in 1h 48m`. A run-out's row is in the theme's attention colour.
- **LATELY** is what the router has done, newest first. Each event is marked, such as
  `14:31  ╎ work reached its cap: 3 sessions moved to side`. When a limit, a cap or a refusal moves
  sessions, those sessions are folded into that event's line. A session is named by the folder
  it runs in, as in `api moved personal → work`, or by its short id where that isn't known.
  Where a line doesn't fit whole, it takes a shorter form, `work at its cap: 3 moved to side`. An
  account found holding another subscription's token is told as `work's token is another
  subscription's`, or `work: wrong token` where that doesn't fit.

### By account

`b` makes USAGE one block for each account, with the account's windows as rows and, from 150
columns, its ROUTING beside it. The pool's block comes last.

### Keys on the Overview

| Key | Does |
|---|---|
| `u` `r` `s` `c` `l` | Shows or hides a section: USAGE, ROUTING, SESSIONS, COMING UP and LATELY. |
| `w` | Shows or hides each model's own window, such as Fable's week. They're hidden until you press `w`. `?` lists it; the key line leaves it out. |
| `b` | Groups the windows by window or by account. Until you press `b`, the grouping follows [`[usage] by`](configuration.md#usage-keys). |
| `↑` `↓` | Scroll USAGE a row, as `j` and `k` do, where its accounts don't fit. Like the other scrolling keys, it's left to `?`, which lists it only then. |

## Accounts

**Accounts** shows each account in depth, and every account together. It has two views on
sub-tabs, Compare and Detail, and `[` and `]` move between them. With one account, Accounts shows
Detail alone, and `[` and `]` do nothing there.

<div align="center">
<img src="../art/accounts-compare.webp" alt="Accounts' Compare over this week: a column each for work, personal and side and one for all accounts, each with its week used, its use by model version, its requests, sessions and moves, its limits and time at its cap, and what its plan costs against the worth of its use at API prices, 4.0× to 5.2×" width="840">
</div>

### Compare

Compare has a column for each account, and one for `all accounts`. Each column shows:

- the account's week, as a bar
- its share of use by model version
- its requests, and the sessions it served and moved
- its limits hit
- the time it spent at its cap or a limit, and what it had left at its last reset
- its plan and its extra usage
- what the plan costs against what its use was worth at API prices. Where an account has no
  `plan`, `all accounts` says how many of the plans it knows, as its worth is every account's.

An account whose token expires within 30 days, or has expired, says so under its state,
`token: 21 days left`.

`←` and `→` move a focus along the columns, scrolling them where they don't all fit. `enter`
opens the focused account in Detail.

Where two accounts' columns and `all accounts`' don't fit side by side, as on a phone, Compare
turns: a group at a time, an account a row. Turned, it has no focus.

### Detail

Detail shows one account, or all of them, chosen from a list at its left. It shows:

- the account's windows now
- the period's requests, limits, plan, worth and extra usage
- its use by model
- the last weeks, six where they fit, as its rule names them, a day and a week at a time
- its verdict, as History's Weeks gives it
- its SESSIONS now
- its LATELY.

An account whose token expires within 30 days, or has expired, says so on a line under its state,
with the command that makes a new one: `token expires 17 Nov 2026 · 21 days left — switchboard
setup makes a new one`.

In Detail:

- `↑` and `↓` choose the account in the list, `all accounts` first.
- `→` moves into the chosen account's sessions, and `←` goes back to the list.
- In the sessions, `↑` and `↓` choose a session, and `enter` opens its page.

The chosen row is lit only where `↑` and `↓` act, in the list or in the sessions. The other's chosen
row stays bold, marked `▸`, and `SESSIONS` is bright while you're in them. The list widens to fit
the accounts' names where the screen has room, as USAGE's name column does.

### The period

`<` and `>` step to a shorter or a longer period, which both views count over. The periods are
today, this week, 30 days and 12 weeks. They're rolling, never billing months.

## History

**History** shows how use has gone over time. It has four views on sub-tabs: Year, Weeks, Days
and Tokens. `[` and `]` move between them.

<div align="center">
<img src="../art/history-year.webp" alt="History's Year: a square a day over the last 12 months, shaded by its requests; beside it, 212,885 requests worth $24,736 at API prices against $7,200 of plans; and BY MONTH under it, each month's requests, worth, ratio, limits and use by model family" width="840">
</div>

### Year

Year is the default view. It draws a square for each day over the last 12 months, with a column
for each week.

- Each square is shaded by how many requests the day made, never by its worth.
- A day with nothing on it has a shade of its own. The active days fall into quarters by how much
  each did.

Beside the grid where the width allows, and under the key otherwise, Year shows the 12 months':

- requests, sessions and limits hit
- worth, what the plans cost, and the worth against it
- busiest day and longest run.

Under the squares, BY MONTH has a row for each month. Each row's bar is the month's worth, split by
model family.

Year's keys:

- `v` chooses what Year shows: all use, one model version alone in four steps of its own colour,
  or one account.
- `←` and `→` step to the account before or after, round, while Year shows one account.
- `<` and `>` step back through each calendar year the ledger holds, and forward again to the last
  12 months.

Shown for one version, Year counts each of the version's sessions once. There's one exception. A
day's summary may have been written before the ledger kept a model's sessions by their ids, and the
day's lines may have gone since. For such a day, Year adds the day's count, as nothing tells those
sessions from another day's.

### Weeks

Weeks shows how full each account's own week got over eight weeks. It also says whether there are
as many accounts as the use needs.

- Its verdict is worded as [`history`](commands.md#history) words it.
- The verdict comes from replaying the last 7 whole weeks with one account fewer or one more.
- `v` draws the weeks as columns or as a table, where both fit.
- As many weeks show as fit. On a narrower screen the oldest go first, so each account's verdict
  still reads whole.
- Where the table doesn't fit, under 79 columns, the verdict comes first, with the weeks under
  it and no verdicts beside them.

### Days

Days shows the eight weeks a day at a time:

- each model family's worth, a day at a time
- how much of its week each account used each day, with the days it hit a 5-hour limit marked.

An account you've removed keeps its row while the eight weeks hold its days, by the label it had,
with `removed 4 Sep` where its days end, so it's told apart from an account you've given the same
label since. The Log, a session's page and the verbs name it by that label too.

### Tokens

Tokens is ccusage's report inside switchboard. It shows each period's tokens by kind, and its worth
at API prices against what its plans cost.

- `<` and `>` set the period each row is: a day, a week or a month.
- `v` splits each period by account, by model, by both, or not at all.
- On a narrower screen, the columns give way one at a time: the model count first, then cache
  write, input, output, the worth against the price and cache read. Total and worth always show,
  and the names stay whole until every other column has gone.

## Sessions

**Sessions** shows where each session's requests are going now. It has two views on sub-tabs, the
Switchboard and the List, and `[` and `]` move between them.

<div align="center">
<img src="../art/repatch.webp" alt="Sessions' Switchboard as personal reaches its limit: the 429 back red along 41e0's cord, its old cord left hanging, and personal's sessions re-patched to side" width="840">
</div>

### The Switchboard

The Switchboard is the default view. It's drawn as a telephone switchboard:

- The sessions run down the left, as calls.
- The accounts run down the right, as lines.
- A cord runs from each call to its account.

Each session's requests travel its cord, out as they go and back as their answers stream. A
refusal bounces back red, `✕ 429`.

When a limit moves a session, the session joins its new account's group. A short cord hangs red
from the old jack, one for each session the limit moved, while the limit holds. A move made for any
other reason hangs faint for 3 seconds.

Where the cords don't fit, as on a phone, each account's line carries its sessions under it. Its
windows are stacked as the board's lines are, a row each, where the screen is tall enough, and go
side by side on one row where it's short. The pool has no lane, as
no session runs on it: the Overview, Runway and `usage` show it.

While the router isn't answering, the Switchboard and the List show the sessions it last listed.
What one was doing then, such as `↓ answering`, stays as it was but faint, here and on every other
tab, as it may have ended since.

### The List

The List is one table of today's sessions: those running first, then those ended. Each row shows
the session's:

- account, model and state
- start time
- requests, and their worth at API prices
- how it came to its account.

### Keys on Sessions

- `↑` and `↓` choose a session: a call on the Switchboard, or a row on the List. Where it's out of
  view, the view scrolls to it.
- `enter` opens the chosen session's page.
- `esc` deselects it.

## Runway

**Runway** shows when each account has room, with a lane for each account.

<div align="center">
<img src="../art/runway-week.webp" alt="Runway over the week: a lane an account to next Wednesday, work at its cap till 17:10, personal's and side's weeks running out at their pace before they reset, and the pool's room under them" width="840">
</div>

Runway shows the week, from a day before now. Every window every model shares counts, the 5-hour
among them, so a lane is without room wherever any of them holds its account back.

Each column of a lane is one cell, showing room, at its cap, or at its limit. The past is dimmed.

Until the router has a day of your sessions, as on a new install, its forecast takes the use on
each account to stay there, so a run-out may come earlier than it will. Runway says so under its
heading, `run-outs firm up once switchboard has a day of your sessions`, where the screen has room.

Words under a lane say where a stretch without room starts and when the account is back. For
example, `capped 14:31  ·  back 17:10` at its cap, `out 13:58  ·  back 15:30` at its limit, or,
ahead, `out ~16:05` where it runs out and `caps ~15:45` where it reaches its cap, `~` marking a
time projected at its pace: more than a day ahead, its day alone, `out ~Sat`, or dated on today's
weekday, `out ~Thu 12 Nov`. Where a cap or a run-out ends as the window resets, room returning
then, the words say `resets Sun 01:00` in place of `back`; a limit always says `back`, as it lifts
whenever it does. Where one hold runs straight into another, the first says
when it lifts, and only the last says when the account is back:
`out 14:00  ·  lifts 19:00  capped 14:00  ·  back Mon 21:00`. Words that don't fit from where a
stretch starts shift left until they do, never over the words before, and drop parts from the end
where even that can't fit them.

With more than one account, the pool's lane sits under the others. It shows how many accounts have
room, a column at a time, and draws a low red bar along its foot where none has. `Σ pool` names it,
as an account's name heads its lane, with the pool's room in the week under it, such as
`week 1.2 of 3 left`, out of the accounts that have the window. Where fewer than two have it, and
not all, there's no pool's lane.

COMING UP lists what happens next, soonest first, and LATELY what befell each account over the
week gone, newest first: a cap, a limit, a refused token, coming under pressure, and room again,
the sessions each moved said in its line, and how long ago at its end, such as `11m ago`. It
reads the whole week off the router's files of events once, as Runway first shows, and keeps it
as the router restarts; till then it shows the router's newest events. Where the screen is wide
enough for both, from 159 columns, and has the rows for COMING UP's every line in one column,
LATELY sits at the left and COMING UP at the right, the past and what's coming as the lanes lie,
each down to the legend. Else COMING UP sits under the pool's lane with LATELY under it, wherever
LATELY's heading and two lines fit: COMING UP in two columns where they fit, from 148 columns,
and LATELY across the screen under them; else the two in one column, as wide as their lines need
to read whole, from 72 columns to the screen's width. Shorter still, or narrower, COMING UP shows
alone. LATELY only takes the rows the lanes and COMING UP leave empty, so it never squeezes them.
Where COMING UP's lines don't all fit, its last says how many more, such as `▼ 3 more`. The legend sits at the foot,
over the key line. On a short screen, as with a phone's keyboard up, LATELY under COMING UP goes
first; then COMING UP is cut to the lines that fit; then the legend gives way, then COMING UP, then the
blank row under the axis; only then do the lanes scroll with `j` and `k`, down to one lane over
the pool's lane at the shortest.

The labels beside the lanes take as many columns as the widest needs, up to 24 cells of label,
and the lanes take the rest. `a` collapses the labels, to give the lanes more room: the lanes
start at the left edge, with no names and no marks, as the status line already says where new
sessions go. Under the start of each lane is its use, such as `66%`, faint for an account out of
the pool, and under the pool's lane its room, short, such as `1.2/3`. The words under a lane
still start under their stretch, but where that would run into the use, they start two cells
after it, as in `100%  out Tue 17:54  ·  back Sun 01:00`. COMING UP stays as it is, each line
naming its account. `a` again expands them; the key line says `a collapse`, its word faint while
the labels are collapsed. Runway remembers which you left it on.

To look back, `←` goes back to this week, then a week at a time, and `→` steps forward, on to the
live view. A week stepped back to has no COMING UP and no LATELY. A step back is never kept.

## Graphs

**Graphs** draws every account's room at once, as charts. It's a page of sections you turn on and
off: first a burn-down card for every account over its week, then whichever other charts you
show.

<div align="center">
<img src="../art/graphs.webp" alt="Graphs: a burn-down card for each account, its room over the week, two to a row, work capped till 17:10, personal's and side's weeks running out at their pace, each heading to its ✕, and the pool's room under them" width="840">
</div>

### Sections on Graphs

Graphs has four sections, always in this order. Each key shows or hides its section, at any size,
and the dashboard remembers which show.

| Section | Key | At first | What it is |
|---|---|---|---|
| WEEK | `w` | shown | A burn-down card for each account, over its week |
| POOL | `o` | hidden | The pool's room over Runway's week, each account's stacked |
| GLANCE | `g` | hidden | A row for each account and the pool: its week's room as a sparkline, with `usage`'s words |
| WHEN YOU WORK | `h` | hidden | Requests by the hour of the day and the day of the week, over the last four weeks |

Graphs charts the week every model shares, as on Runway: a model's own week has no card, and the
5-hour window, which the Overview shows and Runway counts, has none here. With every section hidden, the
page's first row says which keys bring them back.

### The burn-down cards

Each card charts one account's room over its own week, from the week's start to its reset, a
column a cell. Each card keeps its own clock: a card's days, and its now, fall where its own week
puts them, not where the card beside it does. The cards follow the config's order, two side by side
where both fit at 35 cells or more, the rest on the rows under. Every card is as wide as the
others, so the same burn draws the same slope on each: with three accounts, the third sits under
the first, as wide. Where two don't fit side by side, as on a phone, there's a card to a row; with
one account, its card takes the width.

Over the chart, a card has the account's name and marks, and its words: the room left,
such as `66% left`, then what Runway says under the account's lane in its week view, such as
`capped 14:31  ·  back 17:10` or `out ~Sat`. The words come from
Runway's live view as Runway would draw it at the same width. So a hold shorter than one of
Runway's columns can go unsaid on a card, as on Runway, and a wider terminal, or Runway's labels
collapsed with `a`, can bring it out.

The chart:

- **The room left** is filled solid up to now, each column to the room the window had then.
- **Where it's heading** is a dotted line from now to the reset, straight to where the week is
  forecast to be, as the router would spread the pool's use (see
  [Where a week runs out](how-it-works.md#where-a-week-runs-out)). That's the pace Runway's words
  go by. It falls to where the week runs out, then runs along the floor it meets, its cap or
  empty, to the reset; or it ends at the room the week will have left at its reset.
- **`✕`** marks where the window runs out. Where its cap or its limit already holds it, `✕` marks
  the column the hold began in, where that's known.
- **The reserve** is a faint dotted line at the room the account's cap leaves, where the reserve
  holds. It's not drawn while a pin spends the reserve.
- **Now** is a faint `│`.

A week's colour is its state's, as Runway colours a lane: the ramp's first colour where its room
lasts to its reset, the attention colour where it runs out at its pace or its cap holds it, and red
where its limit holds it or its token is refused.

An account not read yet has Runway's words for it and an empty chart but for now. An account the
router doesn't have yet, just added to the config, has its label alone. A stale account is faded,
and one out of the pool faint.

Under the cards, `Σ pool` gives the pool's room in the week, such as `week 1.2 of 3 left`. With
one account there's no pool, and no line. At the foot, over the key line, a legend names the marks.

<div align="center">
<img src="../art/dashboard.webp" alt="One account's dashboard: work alone on the Overview, its usage, routing trace and LATELY, then its week's card on Graphs, then Runway, its lane across the week, with COMING UP and LATELY under it" width="840">
</div>

The animation shows one account, `work`, alone: the Overview, with its usage, its routing trace
and LATELY; then Graphs, its week's card; then Runway, its lane across the week, with COMING UP
and LATELY under it.

### POOL

POOL charts the pool's room over Runway's week view, from a day before now, for a week. Each
account's room is stacked from the foot, in its own colour, so the stack's top edge is the pool's
total. Before now, each account's room as it stood; from now, drawn faint, where it's heading as
forecast: to where it runs out, along its floor to its reset, then full. Runway's axis runs
under it, and a key names each account's colour. Without colour, the accounts can't be told apart,
so POOL draws the pool's total alone.

### GLANCE

GLANCE lists every account and the pool in a few rows, as `usage` does. Each account's week is a
sparkline, its room from its start to its reset, faint from now where it's heading, with `✕` where
it runs out. Then come the room left now, such as `66%`, and `usage`'s words for the week, such as
`19%/day till Sun 21:00`. `Σ pool` gives the pool's sparkline and its room.

GLANCE takes the width: its sparklines grow with it, from 16 cells, while the room left and the
words keep the room they need, lined up down the rows. Narrower, its words take their short forms,
then its names narrow. While WHEN YOU WORK shows too, the two sit side by side wherever both fit
whole with 10 cells between, from 123 columns with the sample's names, sharing what's
spare, GLANCE taking what WHEN YOU WORK's whole hours leave; elsewhere WHEN YOU WORK comes under
GLANCE.

### WHEN YOU WORK

WHEN YOU WORK shows when your requests come: a row for each weekday, from the first day of the
week as [`week_starts`](configuration.md#top-level-keys) begins weeks, and the day's 24 hours across. Each hour is
shaded by its requests, averaged over the four whole weeks before today. The heading says the weeks
counted, such as `the 4 weeks to Wed 30 Sep`, or fewer, where the ledger holds fewer. With none,
the heading says no weeks, and the section says `nothing recorded yet`.

The grid takes the width: every hour's square grows with it, each as wide as the others, any
cells over left blank at the row's end, and the hours over the grid follow their columns. On a
narrow screen, an hour is a square alone, with no gap.

It counts the requests that spend quota and went upstream, each in the local hour it arrived in.
The shades are History's: one for an hour with nothing on it, and four for the quarters of the
active hours, their bounds in the key under the grid.

### Room on Graphs

The terminal's size never decides which sections show, only how much room each gets. At their
least, each card's words share its label's row, and each chart is 5 rows. Spare rows go first to
the cards' words, a row of their own, then to the charts, the cards' and POOL's, every one the same
height, up to 19 rows. Where the sections don't fit even at their least, the page scrolls whole, a
row at a time, and the row over the key line says what's out of view, such as
`▲ 3 more above · ▼ 12 more below`. The legend stays at the foot. The layout changes only on a key,
a resize or the config, never as the numbers move. The charts move on with the clock, a column at
a time.

So at 160 by 37, with three accounts, the cards lie two to a row, each card's words have a row of
their own and the charts take 8 rows; at 108 by 44, 12. On a phone, the cards stack, one to a row,
with charts of 11 rows, or 7 with four accounts.

### Keys on Graphs

| Key | Does |
|---|---|
| `w` `o` `g` `h` | Shows or hides a section: WEEK, POOL, GLANCE and WHEN YOU WORK. |
| `j` `k`, `PgDn` `PgUp`, the wheel | Scrolls the page, where its sections don't fit. |

## The Log

**The Log** shows the router's traffic, as it happens and back through the days. It has two
tables under one tab, Requests and Events, and `[` and `]` move between them. The newest line is at
the foot.

<div align="center">
<img src="../art/log-requests.webp" alt="The Log's Requests, following the newest: a line a request, with its time, session and directory, model, account, status, time to first byte and in all, tokens read from the cache, written to it and put out, why its answer stopped, and a new session's routing, the requests in flight at the foot" width="840">
</div>

### Requests

Requests has a line for each request the router routes. Each line shows:

- when the request was made
- its session, model and account
- its status
- how long it took
- its tokens by kind
- why its answer stopped
- how the router chose its account, with a move's cost.

`f` can show more columns:

- worth at API prices
- input tokens
- attempts
- size, messages, tools and thinking budget
- the client that sent the request
- the router's id for the request.

`f` can also show Claude Code's quota checks and counts of tokens. These stay hidden until you
choose them, as neither is work done for you.

### Events

Events has a line for each thing the router decides or notices. Every move the router makes is
there, with why.

### Keys on the Log

| Key | Does |
|---|---|
| `↑` `↓` | Chooses a line. `↑` while following chooses the newest line and pauses the table. With a line's details open, moves to the line before or after. |
| `end` | Follows again: the newest line at the foot, and nothing chosen. |
| `enter` | Opens the chosen line's details. |
| `s` | Opens the page of the chosen line's session. Does nothing on an account's event. |
| `g` | On the details of an event, shows the Requests, paused at the event's moment. |
| `f` | Opens a panel that sets what the Log shows: its columns, and which lines. |
| `/` | Finds a session by the start of its id or its directory's name. |
| `esc` | With a filter on and nothing open, clears the filter. |

In the `f` panel:

- `←` and `→` move between its sides.
- `↑` and `↓` move through a side's ticks.
- `space` ticks the one the cursor is on.
- `enter` applies the ticks, and `esc` closes the panel unchanged.
- `/` closes the panel for the find.

The find looks at today's sessions first, then back a day at a time. You type in it as in setup's
fields: `←`, `→`, `home` and `end` move the cursor, `backspace` and `delete` take out a character,
and a paste goes in at the cursor. `↑`, `↓` and `enter` pick a session, filtering both tables to it.

## A session's page

**A session's page** follows one session through its whole life.

<div align="center">
<img src="../art/session.webp" alt="A session's page for d28c in ~/Code/switchboard: its STORY of requests a minute, its move from work to side at 14:31 marked with the $1.82 its cache cost, its time and worth on each account, and its turns, the one that moved it opened into its requests" width="840">
</div>

The page shows:

- what the session is doing now, and where it runs
- its STORY: its requests a minute, from its first to now, with each move marked and costed
- the accounts it ran on, and what it took of each, their worth adding up to its turns', to the
  cent
- its turns, newest first.

To open a session's page, do one of these:

- Press `enter` on a session's row on Sessions, or on Accounts' Detail.
- Press `s` on a line of the Log.
- Click a row of the Overview's SESSIONS.

A session's page opens over a tab. `esc` goes back to where you opened it from, with the tab as you
left it.

A session that has run for weeks takes a moment to read. While it does, the page shows its heading
with `loading…` after it, and `esc` goes back.

A turn's requests are read from the ledger as you open it. If that takes a moment, the turn's foot
says `loading…` until they're read. An open turn's requests fill the rows there are as you open
it: those left under the turns, or where the turns fill them, those under the turn, up to five;
and down to one on a short screen. That holds as new turns arrive, until you open it again or
resize the screen. An account joining takes a row from the turns, and the turn shows one fewer
only where it had no row to spare. The request you've chosen stays in view through all of it.

On a short screen, the page keeps its rows for its turns. The STORY's bars give up a row first,
then the turns, from eight down to three, though a turn you've opened keeps its eight rows there,
then the STORY folds to its heading (on a phone it's left out), then the accounts give way to a
count, `+2 more accounts`, then the blank rows, then the turns, down to one. With a turn open, it
keeps five of the turn's requests until the blank rows have gone, then shows fewer, down to one.
Last, that count, the STORY's heading and the two rows saying what the session is doing give way,
one at a time. From 10 rows tall and 11 on a phone, it always shows a turn, and from 13 and 14 a
turn you've opened, whole.

Keys on a session's page:

| Key | Does |
|---|---|
| `↑` `↓` | Chooses a turn, or with a turn open, a request of it. |
| `enter` | Opens the chosen turn into its requests, in place. With a turn open, opens the chosen request's details. |
| `esc` | Closes the open turn, or goes back. See [Closing and going back](#closing-and-going-back). |
| `p` | While the session runs, opens the session's own routing card to pin it. See [The routing card](#the-routing-card). |
| `tab` `shift-tab` | Moves to the tab after or before the one the page was opened from. |

## On a phone

Every view lays itself out by what fits, at every width: each part narrows as far as it still
reads, and a view takes a narrower arrangement only where its wider one's parts don't fit. So
what a phone shows follows from its width, as it would in any narrow window. Only the page's frame
switches at one width: under 100 columns, it takes its phone form.

<div align="center">
<img src="../art/phone.webp" alt="The Overview in a phone's terminal, 52 columns wide" width="260">
</div>

- The tabs sit on a row of their own, under the clock, or on as many rows as their names need
  where one can't hold them, split as evenly as their order allows; and the status line, the
  heading row and the key line say less.
- The Overview's USAGE sets its windows side by side wherever every part of a row fits at its
  least, from 86 columns with two windows and names of the sample's length, and stacks them where
  they don't. As the room runs short, its bars give way first, to 12 cells, then its words, to their
  short form, then the name column.
- ROUTING takes two rows for each account, and SESSIONS two rows for each session, as they do
  wherever one row doesn't fit, under 83 columns with a name column of 16.
- LATELY comes under COMING UP, as it does wherever the two don't fit side by side, under 95
  columns.
- Graphs' cards stack, one to a row, each card's words beside its label where rows run short. See
  [Room on Graphs](#room-on-graphs).
- Accounts' `against it` and `against` give `all accounts`' ratio alone, but where an account has
  no `plan`, they add how many of the plans it's over, compact: `4.4× · 2/3`.
- A session's page keeps each turn's number, start, length, requests, worth and accounts, as it
  does at every width: its tools, then `↑ asking`, then read, written and out, leave as the width
  runs short. An account's requests, then its time there, leave before its windows do, and the now
  rows take their short form.

## Every page

Every page is one frame. From top to bottom, it has:

1. The title row, with the tabs and the clock, and once there's a second pool, the pool shown's
   label after the badge.
2. The status line.
3. On every tab but the Overview and Graphs, which head their own sections, the tab's heading row:
   its name and a rule.
4. The view.
5. The key line. See [The key line](#the-key-line).

When a tab reads a lot, as Sessions or the Log first showing a long ledger, its heading row says
`loading…` until the read lands. On the Overview, which has no heading row, SESSIONS' heading says
it, and on Graphs, its first section's heading. A quick read says nothing. Once the Log, Sessions' List or a session's page has read the
ledger, it keeps reading what the ledger gains while another tab shows, so it's up to date when you
come back to it. The Log keeps only the days it shows: once today's lines fill it, it lets go of
yesterday's, and scrolling back reads them in again.

### The tabs

`tab` and `shift-tab` move between the seven tabs, in the title row's order: the Overview, Accounts,
History, Sessions, Runway, Graphs and the Log. The order goes round, from the Log back to the Overview. The
next dashboard opens on the tab last shown.

### The status line

The status line always says how the router stands, whatever a tab shows. It starts with one of
these:

- `● healthy`
- `■ the router isn't answering  ·  since 14:40:55`, while the router's last view stays on screen
- `○ probed · router not running`
- `○ probed · router started before pools`, where the config has pools and the router, still
  running from before them, doesn't route by them: restart it.

Then it says, but of a pool with no accounts, which says `no accounts` alone:

- how the router routes: `auto` or `pinned to personal`
- the sessions it has routed in the last hour
- where new sessions go: `new sessions → ▲ side`.

### Help

To see every key that works on a page, press `?`. The help card shows:

- every key that works on the page
- what each part of the page shows, and how to read it
- what the page's marks mean.

The card sits over the page. `j` and `k` scroll it a row, and `PgDn` and `PgUp` a page. `?` or
`esc` closes it.

<div align="center">
<img src="../art/help.webp" alt="The Overview's help: every key that works there, and what its marks mean" width="840">
</div>

## Running it

`switchboard dashboard` shows every account's usage, full screen, until you press `q`. It needs a
terminal, and refuses to run without one. It reads the router while the router answers, and
otherwise probes each account. Just after the upgrade that made account ids, while the router
moves the history onto them, it says what the move waits on, a line each time that changes, and
opens once it's done (see [Upgrading to account ids](how-it-works.md#upgrading-to-account-ids)),
and the same while it moves the history to pools.

### In a pool

Where the config has [pools](how-it-works.md#pools), the dashboard shows one pool: the one
`--pool` names, else the one the verbs act on, the only pool, else the unfenced one (see
[Reading a pool](commands.md#reading-a-pool)). A pool the config doesn't have is refused, naming
those it does. What it reads from the router is that pool's: its accounts, windows, room, sessions
running and pin, and the router's events, the Overview's and Runway's LATELY and the Log's Events
among them. The tabs it builds from the request ledger, the Log's requests, Sessions, a session's
page, History and Graphs, still tell of every pool's work together, each account by its label,
until a later release keeps them to the pool shown.

Once there's a second pool:

- The title row names the pool shown after the badge, by its label, at every size.
- `P` turns to the next pool, in the config's order, the first after the last, on every tab. It
  reads the pools afresh, so a pool added or removed while the dashboard is open shows at the next
  `P`. The view starts afresh, as when another router answers: nothing of the pool before carries
  over, its pages, what's chosen, a session's page, the routing card or its notifications. The tab
  shown stays, and what each tab shows as the dashboard remembers it (see
  [What's remembered](#whats-remembered)).
- The dashboard's own notifications lead with the pool's label, as `Acme · k3f9 · Acme 1 has room
  again`.

A pool with no accounts draws the same page on every tab: its status line says `no accounts`, and
the page says that a session started in its directories doesn't start, how to add an account to it
or move one in, and that its directories stay fenced until the pool is removed. The key line keeps
`tab`, `P`, `?` and `q` alone.

Where every pool is fenced and no `--pool` names one, there's no pool the verbs act on: the
dashboard opens on none. Its status line says it couldn't read usage, as every pool is fenced,
naming the pools, and tries again at each read; the pages drawn from the router's document stay
empty, and it reads none of the router's events. `P` turns to the first pool.

```bash
switchboard dashboard [interval] [--no-notify] [--probe] [--pool ID]
```

The interval, given after the command, is how often the dashboard probes every account when it
probes for itself, without the router or with `--probe`:

- It's `30m` unless you give one, and `5m` at the least.
- Give it as a duration, such as `15m` or `1h`, or as a number of minutes.

| Flag | Description |
|---|---|
| `--no-notify` | Posts none of the desktop notifications the dashboard posts while it probes without the router. See [Notifications](configuration.md#notifications). |
| `--probe` | Probes every account each interval, even while the router runs. With no router read, there's no routing card, and `p` does nothing. |
| `--pool` | Shows the pool with the id given, as `switchboard pool` lists them, else the one the verbs act on. See [In a pool](#in-a-pool). |

```bash
switchboard dashboard           # probing every 30 minutes without the router
switchboard dashboard 15m       # every 15 minutes
switchboard dashboard --probe   # every account from the API each interval, whatever the router says
switchboard dashboard --pool q7mt   # pool q7mt, whatever pool the verbs act on
```

### How it reads the router

While the router answers:

- The dashboard looks at the router's view every 5 seconds. This costs nothing upstream.
- It doesn't have the router probe each interval. The router reads the idle accounts itself, on its
  own [rounds](how-it-works.md#the-routers-rounds), each one nothing has read in the last 5 minutes.
- `R` has the router read them at once.

If the router stops answering, or a read of it fails, the router's last view stays on screen. The
status line says `■ the router isn't answering  ·  since 14:40:55`. This lasts until the router
answers again, or until the interval's read, or `R`, probes the accounts directly.

Without the router, the dashboard probes every account every interval. It probes sooner after a
window on screen resets, or after an account couldn't be read. It goes back to the router once the
router answers.

## Keys

These keys work on every page. Each page's own keys are under the page:
[Overview](#keys-on-the-overview), [Accounts](#accounts), [History](#history),
[Sessions](#keys-on-sessions), [Runway](#runway), [Graphs](#keys-on-graphs),
[the Log](#keys-on-the-log) and
[a session's page](#a-sessions-page).

| Key | Does |
|---|---|
| `tab` | Moves to the next tab, in the title row's order and round: the Overview, Accounts, History, Sessions, Runway, Graphs and the Log. From a session's page, moves to the tab after the one the page was opened from. |
| `shift-tab` | Moves to the tab before. From a session's page, moves to the tab before the one the page was opened from. |
| `[` `]` | On Accounts, History, Sessions and the Log, moves to the sub-tab before or after, round. |
| `p` | Opens the routing card. See [The routing card](#the-routing-card). |
| `P` | Turns to the next pool, where there's more than one. See [In a pool](#in-a-pool). |
| `R` | Refreshes now. See [Refreshing](#refreshing). |
| `t` | Opens the theme picker. See [Themes](#themes). |
| `?` | Opens the page's help. See [Help](#help). |
| `esc` | Closes what's open, newest first. See [Closing and going back](#closing-and-going-back). |
| `q` | Quits, except while a card, the Log's panel or the Log's find is open: then only their keys act. |
| `ctrl-c` | Quits, wherever you are. |

### Refreshing

`R` has the router probe:

- the accounts it hasn't read in the last minute
- the accounts that can take no request, however lately it read them.

It doesn't probe an account whose 5-hour window has lapsed and that can take a request. It probes
no account twice in a minute. Without the router, `R` probes every account.

### Closing and going back

`esc` closes what's open, the newest first:

- a card
- the theme picker
- the Log's panel
- the Log's find
- a turn opened on a session's page.

With nothing open, `esc` ends a choice. On the Log, with a filter on and nothing open, it clears
the filter. Otherwise, on a session's page, it goes back to where the page was opened from.

### Scrolling

Where a view doesn't fit, these keys scroll it:

| Key | Scrolls |
|---|---|
| `j` `k` | a row |
| `PgDn` `PgUp` | a page |
| the wheel | three rows a turn |

What scrolls on each page:

- **Overview:** USAGE, where its accounts don't fit. It scrolls whole, a row at a time, keeping
  each window's heading on its first row while that window's rows pass under it, and its last row
  says how many rows are out of view above and below. `↑` and `↓` scroll it too.
- **Accounts:** its view.
- **History:** the Year, where the screen is shorter than it; Weeks' accounts; Days' BY ACCOUNT,
  past six accounts; and Tokens' periods.
- **Sessions:** the List, and the Switchboard's accounts where they don't fit.
- **Runway:** its lanes.
- **Graphs:** the whole page, where its sections don't fit. `?` lists its scrolling keys always.
- **The Log:** its table.
- **A session's page:** its turns, or with a turn open, the turn's requests.

Weeks, Days, the Switchboard on Sessions, and the lanes on Runway scroll an account at a time.
Tokens scrolls a period at a time. A turn of the wheel scrolls the same.

Where one account or one period doesn't fit, as on a short screen, the view scrolls whole, a row at
a time. It does the same on History's Weeks and Days below 37 rows, and the Year always scrolls
so. It then says how many rows are out of view above and below.

Scrolled a row at a time, a page down starts at the first row the page didn't show, and a page up
ends just above the first row it showed, so paging either way passes over nothing. The Switchboard
on Sessions and Tokens on History follow the same rule when they scroll an account or a period at a
time: a page down starts at the first account or period the page didn't show, and a page up ends
at the one just above the first it showed.

When a resize or what's read turns a view from one kind of scrolling to the other, the view goes
back to its top. So does USAGE where `b` groups it the other way, a resize changes how it's laid
out, or a read changes its rows over the key line, as a model's window appearing does.

## The routing card

The **routing card** is the only way the dashboard changes routing. It changes nothing until you
press `enter`.

<div align="center">
<img src="../art/routing.webp" alt="Switchboard's dashboard routing three accounts: the Overview's usage, routing and LATELY, a new session starting on side, then the routing card pinning new and running sessions to side, personal's two moving there" width="840">
</div>

`p` opens the card over the page. It sets where new sessions go. Nothing is picked but what's set
now. The card has these rows:

- `auto`
- a row for each account, by its label, in the config's order.

The accounts aren't numbered, here or on any tab: each shows by its label. After you add or remove
an account, the router keeps its old list until it restarts, and every tab keeps its order till
then: an account you've removed stays while the router still holds it, and one you've added comes
after the router's last.

Each account's row shows:

- whether it's open
- its week and 5-hour window, or `no 5-hour limit` for an account without one
- why it's closed, such as `at its cap till 17:10`, or its sessions running.

### Picking

- `↑` and `↓` move between the rows, and `space` picks the row the cursor is on.
- Picking auto clears the accounts, and picking an account clears auto.
- `m` ticks moving the sessions running elsewhere to the picks too. The card then says how many
  sessions would move, and roughly what rebuilding their caches costs, such as `≈ $6.20`.
- Below, a sentence says what the picks do.

An account that can't take a session can't be picked. That's an account at a limit, one whose
token is refused, one with no token, or one whose token is another subscription's, which reads
`wrong token`.

### Applying

- `enter` applies the picks, as [`pin`](commands.md#pin) does. The router's event names the
  dashboard as what set them.
- `esc` closes the card with nothing changed.

### When `p` does nothing

- Outside the card, `m` does nothing, so a stray key moves no session.
- While the router doesn't answer, `p pin` is drawn faint, and `p` says why instead of opening the
  card.
- With `--probe`, `p pin` isn't listed, and `p` opens nothing, even while the router answers.
- With one account, there's nothing to pin to, and `p` does nothing. The exception is a pin set
  with [`pin`](commands.md#pin) that still stands: then `p` opens the card to clear it.

### A session's own card

On the page of a session that's running, `p` opens that session's own card. Use it to pin the
session to an account, or back to auto. It has no `m`.

### Cards over a page

A card over a page, the routing card or help, is drawn with the page faded behind it. The page
stands still while the card is open, but for the clock, which keeps ticking. The card's own
figures follow each read.

## The key line

The key line, the last row, lists the keys that work on the page. In order:

1. `tab views`, or on a session's page, `esc back`.
2. The page's own keys.
3. `p pin`, where a pin can be set, and `P pool`, where there's more than one pool.
4. `? help  q quit`.

A key that can't act now is drawn faint. If you press it, the key line says why, in the line's
place, for 4 seconds.

`t`, `R` and the scrolling keys work everywhere, and are left to `?`.

The Overview's key line lists its section toggles, each word faint while its section is hidden.
Then come `b by: window` and `p pin  ? help  q quit`.

Graphs' key line lists its toggles the same way, `w week  o pool  g glance  h when`,
then `p pin  ? help  q quit`.

On a phone, or wherever the line doesn't fit whole:

- `? keys` and `q quit` end it. On the Overview and Graphs, it's `? keys and toggles`.
- `tab views` leads it, while it fits.
- `P pool` stays, where there's more than one pool; `p pin` is left to `?`.

## Clicks

The mouse is on while the dashboard runs. To select text, hold the terminal's modifier: Option in
iTerm2 and Terminal.app, and Shift in most others.

| Click on | Does |
|---|---|
| a tab or a sub-tab | Shows it. |
| `<` or `>` by a period | Steps it. |
| a key on the key line | Does what the key does. For a pair, such as `[ ]` or `< >`, it does what the second key does. A click doesn't act on `p pin` or `q quit`. |
| a row that names an account, a session, a line or a turn, or a Compare column's head | Chooses it, as the arrows would. On the one already chosen, opens it, as `enter` does. |
| a section's heading, on the Overview or Graphs | Hides the section. |
| `▼ 214 new`, on the Log | Follows again. |
| a tick in the Log's `f` panel | Ticks it. |

The wheel scrolls what's under the pointer.

Some rows act differently:

- A click on a row of the Overview's SESSIONS opens the session's page at once.
- A click on an account in Detail's list opens nothing.
- With a turn open, a click on a turn closes the open turn. If the turn clicked is another, the
  click then chooses it.

No click opens the routing card or acts in it, so a stray click never moves a session.

## What's remembered

The dashboard keeps some choices in `prefs.json`, in the state directory, as they change. The next
dashboard opens with them.

It remembers:

- the theme, or the pair
- the tab shown
- the Overview's sections shown, and its `b` grouping
- whether `w` shows each model's own window
- Accounts' view and period
- History's view, what its Year shows, Weeks' shape, and Tokens' period and split
- Sessions' view
- whether `a` collapsed Runway's labels
- Graphs' sections shown
- the Log's columns.

If the version or account the Year showed is no longer there to show, the Year shows all use again.

It never remembers:

- a focus or a choice
- a card or help, open
- a session's page
- a calendar year, or Runway's week, stepped back to
- the Log's sub-tab or its filters
- a scroll.

## Themes

<div align="center">
<img src="../art/themes.webp" alt="The theme picker over the Overview, the dashboard redrawn in each theme the cursor reaches: tokyo-night-day, terminal, nord-wave, nord, horizon, gitlab-dark-grey, exchange and amber, then tokyo-night again" width="840">
</div>

The dashboard is drawn in a theme. A theme names its colours by what they mean and how prominent
they are, never by hue.

### Built-in themes

|  |  |
|:---:|:---:|
| <img src="../art/theme-tokyo-night.webp" alt="The Overview in tokyo-night" width="420"><br>`tokyo-night`<br>Tokyo Night. | <img src="../art/theme-tokyo-night-day.webp" alt="The Overview in tokyo-night-day" width="420"><br>`tokyo-night-day`<br>Tokyo Night, for a light terminal. |
| <img src="../art/theme-nord.webp" alt="The Overview in nord" width="420"><br>`nord`<br>The dashboard's palette from the start. | <img src="../art/theme-nord-wave.webp" alt="The Overview in nord-wave" width="420"><br>`nord-wave`<br>Nord on a neutral near-black. |
| <img src="../art/theme-horizon.webp" alt="The Overview in horizon" width="420"><br>`horizon`<br>Horizon's pink and peach on a cool blue-grey. | <img src="../art/theme-gitlab-dark-grey.webp" alt="The Overview in gitlab-dark-grey" width="420"><br>`gitlab-dark-grey`<br>GitLab's Dark Grey: neutral greys with a violet selection. |
| <img src="../art/theme-amber.webp" alt="The Overview in amber" width="420"><br>`amber`<br>An amber CRT. | <img src="../art/theme-exchange.webp" alt="The Overview in exchange" width="420"><br>`exchange`<br>A telephone exchange's brass and walnut. |
| <img src="../art/theme-terminal.webp" alt="The Overview in terminal" width="420"><br>`terminal`<br>For a terminal with a transparent or image background. It paints no background, and draws in the terminal's own sixteen colours: here, a Tokyo Night terminal's. | |

### One theme, or a pair

You can choose one theme for every terminal, or a pair: one for a light background and one for a
dark one.

- As it starts, the dashboard asks the terminal what its background is. It takes a terminal that
  doesn't say for a dark one.
- If you choose nothing, you get the pair of `tokyo-night-day` and `nord`.

### The picker

`t` opens the theme picker, a panel at the right, over the dashboard. It lists every theme.

- `↑` and `↓` move through the themes. The dashboard is redrawn in each theme as you reach it.
- `enter` sets the one theme.
- `d` and `l` set the dark and the light half of the pair. If a single theme is set, they ask first,
  with `y` or `n`, whether to clear it.
- `esc` closes the picker, putting back the theme in force.

While the picker is open, a trace it would cover, in ROUTING or SESSIONS, drops its oldest minutes
instead, so it keeps now and its rate in view. On a narrow screen, where the picker covers the whole
trace, the trace stays under it.

A badge says what each theme fills:

| Badge | Means |
|---|---|
| `●` | the one theme |
| `● light` | the light half of the pair |
| `● dark` | the dark half of the pair |
| `● both` | both halves of the pair |

A theme file that doesn't load is listed with why, such as `bad colour` or `missing tokens`. The
log has the detail.

### Your own themes

A theme is a file named `<slug>.theme`. The slug is lower-case letters, digits and hyphens.

- Put it in the themes directory, at its top level. See
  [Where things live](reference.md#where-things-live).
- Links are followed.
- The picker reads the directory again each time it opens.
- A file can't take a built-in theme's slug.

Each line is `key = #RRGGBB`. A line starting `#` is a comment. Each key appears once.

```
# Lake: a dark theme.
text.primary = #ECEFF4
canvas = #102030
...
```

A theme gives all 19 base tokens, or it doesn't load:

- `text.primary`, `text.secondary`, `text.tertiary`, `text.muted`, `text.subtle`, `text.faint`,
  `text.on-selection` and `text.on-attention`
- `accent.primary`, `accent.key`, `accent.mode` and `accent.attention`
- `state.positive` and `state.destructive`
- `canvas`, `bg.selection`, `bg.attention` and `bg.subtle`
- `border`.

The charts have tokens of their own. Where a file leaves one out, the dashboard works it out from
the base tokens.

| Token | Colours |
|---|---|
| `viz.ramp.1` to `viz.ramp.4` | a bar's fill, first cell to last; the first, a window that lasts to its reset, on Runway and Graphs |
| `viz.track` | a bar's track |
| `viz.pace` | the even-pace mark |
| `viz.reserve` | the cap's mark, and the reserve on Graphs' cards |
| `viz.family.1` to `viz.family.4` | the model families: Opus, Sonnet, Fable and Haiku |
| `viz.version.1` to `viz.version.6` | the model versions: a family's newest in its family's colour, and an older one fainter |
| `viz.day.0` to `viz.day.4` | History's shades of a day, and WHEN YOU WORK's of an hour, from one with nothing on it to the busiest |
| `viz.account.1` to `viz.account.4` | the pool's accounts in Graphs' POOL, by place; a fifth on takes the colour four before it, fainter |

A key switchboard doesn't know is passed over.

### Where the choice is kept

The dashboard keeps your choice in `prefs.json` in the state directory. The dashboard writes this
file, and you never need to. If the file can't be read, it's set aside as
`prefs.json.corrupt-<unix time>`, and the defaults stand. The newest three set aside are kept.

### The background

The dashboard paints the theme's background on every cell. It also sets the terminal's own
background to it, and puts it back as it was when the dashboard stops. That's on `q`, an interrupt
or a terminate signal, or a crash it catches. A terminal that didn't say what its background was is
reset to its profile's own background instead.

[`usage`](commands.md#usage) paints no background, as it prints into the scrollback:

- It asks the terminal what its background is, and takes the light or dark half of the pair to
  match.
- If you chose one theme, it prints in that theme only on a background as dark or as light as the
  theme's own. Otherwise it uses the default pair's half for that background.

### Fewer colours, and none

The colours are brought down to what the terminal shows.

With `NO_COLOR` set to anything:

- The dashboard has no colour, and no background. Its glyphs and bold say what colour would.
- `t` does nothing.

`usage` has no colour either, with `NO_COLOR` set or on a terminal that shows no colour. Its bars
are then filled in `█`. Printed off a terminal, as `usage --pretty` prints it, it has no colour
unless `CLICOLOR_FORCE` asks for it.
