# Snags found using the dashboard

What the owner meets using v0.2.0's dashboard day to day, gathered as found, to fix in the pass
after the release beside milestone 7's loose ends (`2026-10-14--milestone-7-loose-ends.md`). The
owner, on 15 October 2026: "It is really good ... There's literally only one thing at the moment
that's jumped out at me", then a few nitpicks, "no doubt I'll find others".

## A smaller screen

Seen on an iPad over Mosh, the Overview in a phone's layout (under 100 columns), USAGE alone with
COMING UP and LATELY, by window. "Maybe we've got the UI a little wrong on this screen size": to
come back to after the patch.

- **USAGE scrolls as several boxes at once.** The windows stack, 5-HOUR, WEEK and FABLE WEEK, each
  with its accounts and its pool; with four accounts each block shows two, `2 more accounts`, and
  scrolling moves every block's accounts together while each pool stays put outside them. It reads
  as three linked scrolling boxes. Scrolling USAGE whole, as the views scrolled whole do, may read
  better.
- **The blank rows between the blocks differ:** two after the 5-hour's and the week's pools, one
  after Fable week's before the key; and the sections below are cramped while every one shows.
- **The Log is slow to open** there, beside Detail's `enter` (see Anywhere slow).

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

Agreed with the owner, 15 October 2026, from text frames, to build after the snags:

- **A burn-down beside the burn-up.** `v` flips GRAPHS between them while GRAPHS shows, kept in
  the preferences; the structure is the same. The burn-up stays as built. The burn-down is drawn as
  v0.1.6 drew it: the room left as a level falling from full at the window's start, its colour
  running from the ramp's first stop to its last as it falls; now's line; where it's heading, a
  braille dotted line from the level now at the projection Runway's words go by, to `✕` on the
  floor where it runs out, else to the room left at its reset; the cap, where the account has one,
  as a faint braille line along its floor; held at its limit, a line along the floor in
  `state.destructive` till it lifts. No even-pace line: drawn a row at a time it reads as a
  staircase (the owner: "I actually preferred how the previous released version did it").
- **By account, every account's charts beside its bars.** With `b`, GRAPHS draws each account's
  windows' charts to the right of its block, so it needs no rows of its own (today it says
  `no room at 50 rows` in a near full-screen terminal on a 16-inch MacBook Pro) and every account
  shows at once, as v0.1.6's cards did. While they show, ROUTING goes back to its own section; no
  focus by account. Narrower, it charts the windows that fit, about 26 columns each, then falls
  back to the focused account's charts as by window.
- **The pool's charts,** by account beside `Σ pool`: the pool's room at each moment, the sum of
  what each account has left to its cap, back from now over a window's length, each account's reset
  a step up. By window, `↑` `↓` stay on the accounts.
- **No chart view on Runway:** by account covers every account at once; a shared time axis is all
  it would add.

## Anywhere slow

- **Nothing says a page is loading.** `enter` on a session in Detail takes about two seconds to
  open its page, with nothing on screen meanwhile. Show that something is under way wherever a
  step takes long enough to be noticed.

- **The Log takes three or four seconds to open the first time** after the dashboard starts, on
  the iPad and the Mac alike; going back to it later, it's there at once. The owner isn't sure the
  Log keeps updating while he's away from it: check it does.

## Works well

Opening a session's page, stepping through its turns, `enter` into a turn's requests, and `enter`
again for a request's card: the owner tried it all and it works.
