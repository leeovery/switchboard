# Snags found using the dashboard

What the owner meets using v0.2.0's dashboard day to day, gathered as found, to fix in the pass
after the release beside milestone 7's loose ends (`2026-10-14--milestone-7-loose-ends.md`); the
patch after v0.2.0 fixed the rest. The owner, on 15 October 2026: "It is really good ... There's
literally only one thing at the moment that's jumped out at me", then a few nitpicks, "no doubt
I'll find others".

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

## Accounts' Detail

- **The period keys need shift.** `<` `>` are shifted keys on most keyboards. The owner found it
  out and was content ("that makes sense, I guess"); unshifted `,` `.` as the same keys is a
  possible ease, not asked for.

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

## Works well

Opening a session's page, stepping through its turns, `enter` into a turn's requests, and `enter`
again for a request's card: the owner tried it all and it works.
