# Every page

Every page is one frame. From top to bottom, it has:

1. The title row, with the tabs and the clock, and once there's a second pool, the pool shown's
   label after the badge.
2. The status line.
3. On every tab but the Overview and Trends, which head their own sections, the tab's heading row:
   its name and a rule.
4. The view.
5. The key line. See [The key line](#the-key-line).

When a tab reads a lot, as Sessions or the Log first showing a long ledger, its heading row says
`loading…` until the read lands. On the Overview, which has no heading row, SESSIONS' heading says
it, and on Trends, its first section's heading. A quick read says nothing. Once the Log, Sessions' List or a session's page has read the
ledger, it keeps reading what the ledger gains while another tab shows, so it's up to date when you
come back to it. The Log keeps only the days it shows: once today's lines fill it, it lets go of
yesterday's, and scrolling back reads them in again.

## The tabs

`tab` and `shift-tab` move between the seven tabs, in the title row's order: the Overview, Accounts,
Projects, Sessions, Runway, Trends and the Log. The order goes round, from the Log back to the Overview. The
next dashboard opens on the tab last shown.

## The status line

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

## Help

To see every key that works on a page, press `?`. The help card shows:

- every key that works on the page
- what each part of the page shows, and how to read it
- what the page's marks mean.

The card sits over the page. `j` and `k` scroll it a row, and `PgDn` and `PgUp` a page. `?` or
`esc` closes it.

<div align="center">
<img src="../../art/help.webp" alt="The Overview's help: every key that works there, and what its marks mean" width="840">
</div>

## Keys on every page

These keys work on every page. Each page's own keys are on its page:
[Overview](overview.md), [Accounts](accounts.md), [Projects](projects.md),
[Sessions](sessions.md), [Runway](runway.md), [Trends](trends.md),
[the Log](log.md) and
[a session's page](session-page.md).

| Key | Does |
|---|---|
| `tab` | Moves to the next tab, in the title row's order and round: the Overview, Accounts, Projects, Sessions, Runway, Trends and the Log. From a session's page, moves to the tab after the one the page was opened from. |
| `shift-tab` | Moves to the tab before. From a session's page, moves to the tab before the one the page was opened from. |
| `[` `]` | On Accounts, Sessions and the Log, moves to the sub-tab before or after, round. |
| `p` | Opens the routing card. See [The routing card](routing-card.md). |
| `P` | Turns to the next pool, where the config has pools, the pools read afresh: with one, says so. Listed where there's more than one. See [In a pool](running-it.md). |
| `R` | Refreshes now. See [Refreshing](#refreshing). |
| `t` | Opens the theme picker. See [Themes](themes.md). |
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
- **Projects:** a project's SESSIONS. The list follows the project chosen, by `↑` and `↓`.
- **Sessions:** the List, and the Switchboard's accounts where they don't fit.
- **Runway:** its lanes.
- **Trends:** the whole page, where its sections don't fit. `?` lists its scrolling keys always.
- **The Log:** its table.
- **A session's page:** its turns, or with a turn open, the turn's requests.

The Switchboard on Sessions and the lanes on Runway scroll an account at a time. A turn of the
wheel scrolls the same.

Where one account doesn't fit, as on a short screen, the view scrolls whole, a row at a time. It
then says how many rows are out of view above and below.

Scrolled a row at a time, a page down starts at the first row the page didn't show, and a page up
ends just above the first row it showed, so paging either way passes over nothing. The Switchboard
on Sessions follows the same rule when it scrolls an account at a time: a page down starts at the
first account the page didn't show, and a page up ends at the one just above the first it showed.

When a resize or what's read turns a view from one kind of scrolling to the other, the view goes
back to its top. So does USAGE where `b` groups it the other way, a resize changes how it's laid
out, or a read changes its rows over the key line, as a model's window appearing does.

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

Trends' key line lists its toggles the same way, `w week  o pool  g glance  h when  y year  m months
d models`; then, while YEAR or BY MONTH shows, `v split: all`; then `p pin  ? help  q quit`. Where
the line doesn't fit whole, `v split` goes first.

On a phone, or wherever the line doesn't fit whole:

- `? keys` and `q quit` end it. On the Overview and Trends, it's `? keys and toggles`.
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
| a row that names an account, a project, a session, a line or a turn, or a Compare column's head | Chooses it, as the arrows would. On the one already chosen, opens it, as `enter` does. |
| a section's heading, on the Overview or Trends | Hides the section. |
| `▼ 214 new`, on the Log | Follows again. |
| a tick in the Log's `f` panel | Ticks it. |

The wheel scrolls what's under the pointer.

Some rows act differently:

- A click on a row of the Overview's SESSIONS opens the session's page at once.
- A click on an account in Detail's list, or a project in Projects' list, opens nothing.
- With a turn open, a click on a turn closes the open turn. If the turn clicked is another, the
  click then chooses it.

No click opens the routing card or acts in it, so a stray click never moves a session.
