# The session page: one session, start to finish

The session page follows one session through its whole life: every request, every move, and what
each cost. Open it to see why a session moved, what a move cost in cache, or which turn took the
most.

<div align="center">
<img src="../../art/session.webp" alt="A session's page for d28c in ~/Code/switchboard: its STORY of requests a minute, its move from work to side at 14:31 marked with the $1.82 its cache cost, its time and worth on each account, and its turns, the one that moved it opened into its requests" width="840">
</div>

*The page for d28c: a move from work to side at 14:31, and the turn that moved it opened.*

## Opening the page

To open the page, do one of these:

- Press `enter` on a session on [Sessions](sessions.md), or in Detail on [Accounts](accounts.md).
- Press `s` on a line of [the Log](log.md).
- Click a session row in SESSIONS on the [Overview](overview.md).

The page opens over a tab. `esc` goes back to where you opened it from, with the tab as you left
it. A session that has run for weeks takes a moment to read. While it does, the heading says
`loading…`, and `esc` still goes back.

## What the page shows

From top to bottom:

1. **Now:** what the session is doing, and where it runs.
2. **STORY:** the requests a minute, from the first to now, with each move marked and costed.
3. **The accounts** the session ran on, and what it took of each. Their worth adds up to the worth
   of the turns, to the cent.
4. **TURNS:** every turn, newest first.

Press `enter` on a turn to open it into its requests, in place. Press `enter` on a request to open
its details.

## An open turn

The requests of a turn are read from the ledger as you open it. If that takes a moment, the foot
of the turn says `loading…` until they're read.

An open turn fills the rows there are as you open it, up to five requests:

- the rows left under the turns;
- or, if the turns fill the screen, the rows under the turn.

On a short screen, that's down to one. The number holds as new turns arrive, until you open the
turn again or resize the screen. An account joining takes a row from the turns. The open turn
shows one request fewer only if it had no row to spare. The request you've chosen stays in view
through all of it.

## Room on a short screen

On a short screen, the page keeps its rows for the turns. The other parts give way in this order:

1. The STORY bars give up a row.
2. The turns go from eight rows down to three. A turn you've opened keeps its eight.
3. The STORY folds to its heading. On a phone, it's left out.
4. The accounts give way to a count, such as `+2 more accounts`.
5. The blank rows go.
6. The turns go down to one.
7. Last, that count, the STORY heading and the two rows on what the session is doing go, one at a
   time.

With a turn open, the page keeps five of its requests until the blank rows have gone. Then it
shows fewer, down to one.

| Height | Always shows |
|---|---|
| From 10 rows, or 11 on a phone | a turn |
| From 13 rows, or 14 on a phone | a turn you've opened, whole |

## Keys on the session page

| Key | Does |
|---|---|
| `↑` `↓` | Chooses a turn, or with a turn open, a request in it. |
| `enter` | Opens the chosen turn into its requests. With a turn open, opens the details of the chosen request. |
| `esc` | Closes the open turn, or goes back. |
| `p` | While the session runs, opens the routing card for this session, to pin it. See [The routing card](routing-card.md). |
| `tab` `shift-tab` | Moves to the tab after or before the one the page was opened from. |

The keys every page shares are on [Every page](every-page.md).
