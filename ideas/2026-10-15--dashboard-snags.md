# Snags found using the dashboard

What the owner meets using v0.2.0's dashboard day to day, gathered as found, to fix in the pass
after the release beside milestone 7's loose ends (`2026-10-14--milestone-7-loose-ends.md`); the
patch after v0.2.0 fixed the rest, and built the charts the owner agreed. The owner, on 15 October 2026: "It is really good ... There's
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

## On a phone, 16 October

Seen on an iPhone 17 Pro Max in Blink, over Mosh into tmux, the dashboard the last tab: portrait
73×61, about 73×35 with the keyboard up; landscape about 138×30, about 138×15 with the keyboard up.
"Making it work well on my phone, my iPad, and my desktop is a priority, especially the phone."
The owner's plan: the screens drawn into Paper on each device's grid, portrait and landscape, and
worked on there till they're right.

- **The phone layout is drawn for 52 columns.** At 73, Compare, Detail, the Year and the List
  leave a third of the width empty, while the header, status line and key line run full width.
  "No one has phones that small anymore."
- **Names cut short where there's room,** to 8–11 cells, so two labels that differ at their end
  read alike: the Log, Sessions' Switchboard, the List, Tokens.
- **In landscape,** Detail uses under half the width; the Year shows the same 22 weeks it does in
  portrait; Days draws its blocks at the left and its figures at the far right, a gap between.
  The Overview stretches well, but needs scrolling; Compare and Tokens read well.
- **Runway** reads best of every view, portrait and landscape, but:
  - under the pool's bars, its key and words are cramped against them, with blank rows spare
    below; and on a phone in portrait with the keyboard up COMING UP is cut to its first line;
  - in landscape with the keyboard up (about 15 rows) it draws no lane at all: "it'd be better to
    show something than nothing";
  - the owner would put `Σ pool` at the top, not the bottom ("I think that was a mistake"), and
    drop the rule above it now it's named and marked; perhaps a texture on its bars, to discuss.
  - the words under the pool's lane, "3 of 4 with room till ~Sat 11:18, 2 till ~Sat 20:47, 1 till
    Sun 02:00, …", read its bar aloud a step at a time, crammed on one row: "I don't know even what
    all that means, let alone if we need it." The owner likes "week 0.9 of 4 left" beside it;
  - the pool's red `at its limit` line sits in the lane's bottom row, its `─` at mid-cell, so it
    runs level with the top of the half-cell yellow beside it, not through the lane's middle as the
    accounts' lanes have it. Seen on the Mac at 13:30 on 16 October, "0 till Sun 02:00" between two
    yellow stretches of 1 (screenshots in the session's records, `shots/pool-limit-line-*.png`); it
    looks wrong there. The owner's idea, to mock up: the red a bar, not a line, a little thicker:
    "the green, then maybe a half yellow, and then a quarter red", the lane stepping down to a
    quarter-cell red block (`▂`). A nitpick.
- **The key floats mid-page,** on Sessions' Switchboard most: the owner's idea, to spike, is the
  key at the foot with the key line, a footer of two, any other words above it.
- **The List's summary** sits right-aligned on the row over the key line, so it reads as the key
  line broken onto two rows, its right half higher.
- **Weeks says there aren't enough weeks yet** on a ledger begun 7 October: check that's the rule,
  not a fault.
- **A phone's charts start under the bars' column,** about 10 columns in, the room to their left
  unused: "Let the graphs be full width there."
- **A phone's view tabs** (Compare, Detail) draw their highlight hard against the word, no cell
  before it, so it reads left-aligned: a cell either side, as the page's tabs have.
- **A card left open reads as a hang.** Under a card the page holds as it stood when the card
  opened (`watch.go` screen: `m.page.frame` at `m.page.at`), its clock included. A dashboard left
  with a card open on the phone at 08:28 was taken for one that had frozen, its keys no longer
  reaching it over Mosh. To decide: whether the heading's clock stays live under a card.

## Loading

- **`loading…` in the heading's state slot is too small to notice.** The owner would rather a
  central indicator of a decent size, animated perhaps: "We need something visual, decent-sized."
  Not urgent.

## Accounts' Detail

- **The period keys need shift.** `<` `>` are shifted keys on most keyboards. The owner found it
  out and was content ("that makes sense, I guess"); unshifted `,` `.` as the same keys is a
  possible ease, not asked for.

## Works well

Opening a session's page, stepping through its turns, `enter` into a turn's requests, and `enter`
again for a request's card: the owner tried it all and it works.
