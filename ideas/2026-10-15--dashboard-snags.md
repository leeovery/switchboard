# Snags found using the dashboard

What the owner meets using v0.2.0's dashboard day to day, gathered as found, to fix in the pass
after the release beside milestone 7's loose ends (`2026-10-14--milestone-7-loose-ends.md`). The
owner, on 15 October 2026: "It is really good ... There's literally only one thing at the moment
that's jumped out at me", then a few nitpicks, "no doubt I'll find others".

## Accounts' Detail

- **The account list cuts names it has room for.** In a near full-screen terminal on a 16-inch
  MacBook Pro, the list at the left cuts each label (`me@leeo…`, `lee@overy…`) while the page has
  plenty of width spare. Give the list the room its names need, up to a cap, as USAGE fits its name
  column to the names.
- **Where the focus is isn't shown.** On opening, `↑` `↓` move through the account list, yet a
  session row in SESSIONS is lit too, so both look chosen. `→` moves into the sessions, but the
  list still looks focused; `←` back gives no sign but that `↑` `↓` move the list again. Draw the
  choosing bar only where the focus is, and show which panel holds it, its heading lit or its edge
  marked, say.
- **The period keys need shift.** `<` `>` are shifted keys on most keyboards. The owner found it
  out and was content ("that makes sense, I guess"); unshifted `,` `.` as the same keys is a
  possible ease, not asked for.

## The Overview

- **The focus bar stops short of a long name.** While GRAPHS shows, the focused account's number
  and name are lit on `bg.selection` for their first 12 cells, as the design says, a width written
  for names like `work`; a longer name is lit partway (`1 me@leeove` of `1 me@leeovery.com ◆`).
  Light the whole name column, as USAGE fits it to the names.

## Sessions

- **A chosen call is lit across the whole page.** On the Switchboard, `↑` `↓` light the chosen
  call on `bg.selection` across the width, as the design says, a rule that suits the List; there it
  runs on over the cords and the accounts' side, and where the row meets an account's bars it
  paints over their fill (the first account's 5-hour bar lost its colours under `57ad`'s row).
  Light the call's own columns alone, its id, name, state and `from`, up to where its cord begins.

## Runway

- **The pool's lane draws nothing where no account has room.** Each account's lane draws `─` in
  `state.destructive` while it's at its limit, but the pool's columns are as high as the share with
  room, so where none has room (`0 till Sun 02:00`) the lane is blank. Draw the pool's lane at none
  as a lane at its limit draws it.
- **The lines miss the bars' middle, and the reset mark overshoots them.** A lane's bars are `▆`,
  the lower three quarters of a cell, while its `─` sits at the cell's middle and its `│` spans the
  cell whole: a cap's or a limit's line meets the bar above its middle, and the reset's `│` stands
  taller than the bar beside it. The owner wants the line to meet the bar's middle and the `│` to
  meet its top and bottom. One way: draw the bars the full cell, as USAGE draws its bars.

## Charts

- **Every account's burn-down, back.** The owner grew to like v0.1.6's burn-down charts: every
  account's 5-hour window and week on screen at once, drawn as what's left falling. v0.2.0's
  GRAPHS (`g`) draws one account at a time, the one the focus is on, as use rising, a burn-up; and
  by account (`b`), USAGE's blocks push GRAPHS below the fold, so the owner found them only with
  USAGE hidden. The ask: a place to see every account's charts at once, and the burn-down's way
  round. The owner's calls, 15 October 2026: GRAPHS keeps its burn-up and gains a burn-down, a
  key flicking between them in the same structure ("some people like burning up, some people like
  burning down"); a burn-down draws where it's heading, faint, at the projection Runway's words go
  by, so the two never disagree; and every account's burn-down at once is wanted back. v0.1.6 had
  it: a card an account, each drawing its featured window's burn-down with its dotted projection,
  `w` featuring the week on every card at once. Where it goes now is open: the owner floated a
  view of Runway's, every account on one time axis, the Overview staying as it is.
- **By account, GRAPHS has no room.** With `b`, USAGE's blocks take the rows GRAPHS would have,
  and GRAPHS says `no room at 50 rows` in a near full-screen terminal on a 16-inch MacBook Pro.
- **The pool, chosen for GRAPHS.** `↑` `↓` stop at the last account; the owner wants `↓` to go on
  to `Σ pool` and GRAPHS to chart the pool across the board. The accounts' windows start and reset
  at different times, so the pool's charts need a time axis of their own. Agreed with the owner,
  15 October 2026: the pool's room at each moment, the sum of what each account has left to its
  cap, back from now over a window's length, each account's reset a step up marked `│`, no even
  pace. Whether a burn-down draws where it's heading is open (see the projection, in the design's
  Running out).

## Anywhere slow

- **Nothing says a page is loading.** `enter` on a session in Detail takes about two seconds to
  open its page, with nothing on screen meanwhile. Show that something is under way wherever a
  step takes long enough to be noticed.

## Works well

Opening a session's page, stepping through its turns, `enter` into a turn's requests, and `enter`
again for a request's card: the owner tried it all and it works.
