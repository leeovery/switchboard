# The dashboard's visual capture harness (`vhs`)

This directory is where the dashboard is checked against its design. Milestone 7 rebuilds the
dashboard, and `usage`'s printout, to the frames signed off in Paper from 4 to 8 October 2026,
which `reference/` keeps. It's checked two ways. A test holds every fixture named after a frame to
that frame, cell for cell: its glyphs, its colours and its bold. And by eye, for what a test can't
judge, how a page reads and what moves: a stage that changes what the dashboard draws writes a `vhs`
tape here, screenshots a fixture through it, and judges the capture against the frame it was built
to.

The dashboard is drawn by `cmd/capturetool`, a program of its own, not a switchboard subcommand.
It builds the dashboard's real watch model through `watch.New`, as the dashboard does, with every
seam faked by `internal/capture`: the router's status document, sessions, history and request
stream, the ledger's, the readings history's and the events' readers, the price table, the clock
and its timers, the notifier, the terminal's size and background, and the themes; a fixture of
`usage`'s printout is drawn through a path of its own, outside the watch, from its fake router's
status document alone, read as `usage` reads it. It never dials the router's socket, probes,
touches the network, reads or writes the real config, themes, state, ledger, prefs or tokens, or
runs another process, so it's safe to run beside a live router. A theme picked in it is kept for
the run alone. Switchboard's own binary never imports `internal/capture`: `cmd/capturetool`'s
import guard test fails if anything `cmd/switchboard` builds ever does.

## What lives here, and for how long

| Path | What | Lifetime |
|---|---|---|
| `<name>.tape` | A tape that runs a fixture, or cats a reference, and screenshots it | Scaffolding: cleared, with its capture, as each milestone is signed off |
| `<name>.png` | Its capture | Scaffolding, as its tape |
| `reference/<stem>.png` | A signed-off frame, exported at 1x from its area's Paper file | **Kept** |
| `reference/<stem>.txt` | The frame's character grid, from the generator that drew it: a line a row, its trailing blanks trimmed | **Kept** |
| `reference/<stem>.ansi` | The frame in 24-bit colour, from the generator that drew it: every cell painted, every row its full width, and no newline after the last, so `cat` in a terminal of its size doesn't scroll it | **Kept** |

A capture is a render of the code, so it goes stale as the code moves on, and nothing checks it
against a baseline: tapes and captures are written as a stage proceeds, committed while it's
reviewed, so the reviewer opens the frame the implementer did, and cleared at the milestone's
sign-off. `vhs` writes no GIF unless a tape names an `Output`, and these name none.

`reference/` is kept: it's the design the code is built against, not a render of the code, and it
doesn't go stale the way a capture does. Capture's tests read it. The `.png`s are the frames
themselves; the `.txt` and `.ansi` are the same frames as the mock generator drew them, before
Paper did, which the test holds fixtures to, and which `vhs` is calibrated against. They're the
generators' output, byte for byte, and never patched: where a frame differs from
`docs/design.md`, the design holds, and the test passes over those cells (see The cells passed
over).

## The frames

77 of them, each by its stem: its area, what it shows, and its width, then its height where it
isn't its area's usual, as in `overview-everything-160x39`. They're drawn in `nord` at Thu 1 Oct
14:42:07, or at 16:05:41 and 16:05:42 for `sessions-refused-160` and `sessions-moved-160`, with the
placeholder accounts and sample numbers the design's sections give.

| Area | Stems | Paper file, page |
|---|---|---|
| `usage` | `usage-*`, 9 | *Switchboard · usage*, **FINAL · usage · signed off 4 Oct 2026 · revised 6 Oct** |
| The Overview | `overview-*`, 13 | *Switchboard · Overview*, **FINAL · dashboard overview · signed off 5 Oct 2026 · revised 6 Oct** |
| Routing by hand | `routing-*`, 5 | *Switchboard · Routing*, **FINAL · routing · signed off · 7 Oct 2026** |
| Accounts | `accounts-*`, 11 | *Switchboard · Accounts*, **FINAL · accounts · signed off 5 Oct 2026 · revised 6 Oct** |
| History | `history-*`, 10 | *Switchboard · History*, **FINAL · history · signed off 5 Oct 2026 · revised 6 Oct** |
| A session's page | `session-*`, 6 | *Switchboard · Session and Log*, **FINAL · session page · signed off · 7 Oct 2026** |
| The Log | `log-*`, 9 | *Switchboard · Session and Log*, **FINAL · Log · signed off · 7 Oct 2026** |
| Sessions | `sessions-*`, 9 | *Switchboard · Sessions and Runway*, **FINAL · Sessions and Runway · signed off · 8 Oct 2026** |
| Runway | `runway-*`, 5 | *Switchboard · Sessions and Runway*, the same page |

## How the frames are held

`internal/capture`'s `reference_test.go` holds each fixture named after a frame's stem to that
frame, with the cells `cells_test.go` reads and compares:

- **The fixture is drawn** at its own size, which is its frame's, and read into cells as a terminal
  reads it: a glyph a cell, each with its foreground, its background and its bold, its SGR's state
  carried from one cell to the next until reset. So the generators' form, every cell painted after
  a reset, and Lip Gloss's, a run in one style, reset after it, read alike. What it can't read as a
  terminal would is a failure, not a guess: an escape that isn't SGR, an attribute but bold, a
  colour not in 24 bits, a control character, or a glyph not one cell wide.
- **Cell for cell,** it's compared with the frame's `.ansi`, and nothing passes but this:
  - **the canvas:** a cell whose background isn't painted counts as Nord's canvas, `#2E3440`, as a
    terminal of that background shows it, since `usage`'s printout paints no background where the
    frames paint every cell;
  - **rounding:** a colour's channel may be one apart, as the generators round a half to even,
    Python's `round`, where Go's mixing rounds it away from zero;
  - **a blank's foreground and bold,** which nothing shows: a blank's background is compared;
  - **the cells passed over,** below.

  A drawn row shorter than its frame's is read as a terminal shows it, blank and unpainted to the
  frame's width, as `usage`'s printout writes nothing past a line's end; so it passes where the
  frame is blank on the canvas there, and fails where the frame has a glyph or another background.
  A drawn row wider than its frame's, or a row more or fewer, fails. A failure names the stem and
  lists the cells that differ, the first 20, a line each with its row and column, counted from 1,
  and the frame's and the fixture's glyph, colours and bold; then how many more.
- **The frames prove the reading:** every frame's `.ansi` reads to its `.txt`, every cell painted,
  at the width its stem names, and its height where it names one; and every fixture's frame reads
  to its own plain text.
- **What's owed:** `owed`, in `reference_test.go`, names the frames no fixture draws yet. Every
  frame is drawn by a fixture of its stem's name or owed, never both. A stage that builds a view
  adds its fixtures, named after its frames, and deletes their stems from `owed`, and from then on
  they're held; it's empty by stage 14. Milestone 5's fixtures, none named after a frame, are held
  to nothing, and go as their views do.
- **What's held of a frame owed:** `holds`, beside `owed`, holds part of a frame no fixture draws
  yet, drawn over the frame's world with the keys that reach it, at the frame's size, and compared
  as a whole frame is, cell for cell, but over that part alone (`compareWithin`, in
  `cells_test.go`), passing over the same cells. Stage 6, the page, held the chrome and the routing
  card's box; stage 7 draws the Overview's frames and the routing card's whole, so `holds` keeps
  **the chrome** alone, rows 1 to 4, the title row and the status line, or 1 to 5 on a phone, whose
  tabs take a row of their own: of `sessions-switchboard-160` and `runway-5-hour-160`, over the
  auto world, its tab reached with `tab`, and of `sessions-phone-52`. A frame's stem leaves `holds`
  as a fixture comes to draw it whole, as `history-year-160` did at stage 8, `log-requests-160` at
  stage 9, and `accounts-compare-160` at stage 11.
- **The cells passed over** are `passedOver`, by stem: a row, or a row's cells from one column to
  another, each with what the frame's `.txt` reads there: the words it starts with; or blank; or,
  where the frame draws a trace or a chart from data no world gives, those glyphs alone, a chart's
  lines among them. A test checks each, as it checks that this file names every stem in it.

## The cells passed over

The cells a frame drew otherwise than `docs/design.md` says, or than its world gives (see What the
worlds give otherwise than the frames), by row and column, counted from 1 as the frame's `.txt`
reads in an editor, and what the dashboard draws in their place, as the design says and the world
gives (see Files, "The capture harness", in `docs/design.md`).

**`usage`'s printout, where its frames and the design part:**

- **Side's week, where it runs out.** At its average pace, side's week runs out ~Fri 12:20, before
  it resets on Monday at 10:00, so `usage` words it `out ~Fri 12:20`, short `out ~Fri`, in
  `accent.attention`, where the frames give its allowance, `6%/day till Mon 10:00`, short
  `6%/day · Mon`: columns 88 to 108 of row 9 of `usage-160`, 98 to 118 of row 20 of `usage-120`,
  58 to 78 of row 22 of `usage-80`, and 138 to 158 of row 20 of `usage-by-account-160`; and, short,
  38 to 49 of row 22 of `usage-52` and of row 25 of `usage-by-account-52`.
- **Even pace on a week, a cell off.** The frames place `┃` as if each week's even pace were a
  whole percent, work's 53%, personal's 65% and side's 46%, where at the moment drawn they're
  53.4%, 64.7% and 45.7%, so the design's rule, `min(round(pace × bar), bar − 1)`, puts it a cell
  on from the frame's for work, and a cell back for personal and side: `usage` draws `┃` where the
  frame has the bar, and the bar where the frame has `┃`. In `usage-160`, row 5, columns 73 and 74
  and 121 and 122, work's week and Fable week; in `usage-120`, rows 16 and 27, columns 56 and 57,
  work's; and in `usage-by-account-160`, columns 82 and 83 of rows 6 and 8, work's, 94 and 95 of
  rows 13 and 15, personal's, and 74 and 75 of rows 20 and 22, side's.
- **`◆` on work alone,** in `usage-one-account-160`, row 5, columns 8 to 10, which the frame draws
  `1 work ▲`. Work is the primary, as the config makes the one account it lists, and the design
  marks the primary `◆` with one account as with several (the owner's call, 11 October 2026), so
  `usage` draws `1 work ◆ ▲`.
- **`usage-by-account-160`'s key line,** row 31, which the frame starts at column 15, where the
  design starts it at the name column, 16 cells in: `usage` draws it two cells on, from column 17.

**Key lines drawn before the keys were swept,** on 7 and 8 October 2026, which the dashboard draws
as Keys has them:

- **The Overview's key line,** its last row, in every `overview-*` frame but `overview-phone-52`:
  row 37 of `overview-default-160`, `overview-pinned-160`, `overview-bad-hour-160` and
  `overview-usage-alone-160`; row 39 of `overview-everything-160x39`; 41 of
  `overview-six-accounts-160`; 42 of `overview-by-account-160`; 53 of `overview-ten-accounts-160`;
  70 of `overview-default-216`, `overview-sessions-216` and `overview-sessions-graphs-216`; and 106
  of `overview-everything-216x106`. The frames end it `↑↓ focus  0 auto  1-3 pin  m move  ? help
  q quit`; the dashboard, `↑↓ focus  p pin  ? help  q quit`.
- **ROUTING's heading under a pin,** row 19 of `overview-pinned-160` and `overview-bad-hour-160`,
  which ends `0 back to auto` in the frames, and `p back to auto` in the dashboard.
- **The period on Accounts' and History's heading row,** row 5, from the rule's last two cells to
  the last column: the frames' `p this week`, where the dashboard draws the period between its keys,
  `< this week >`, two cells wider, so its rule ends two cells sooner; on History's Weeks and Days,
  `eight weeks`, with no key. Columns 146 to 160 of `accounts-compare-160`,
  `accounts-compare-bad-hour-160`, `accounts-compare-ten-160`, `accounts-detail-all-160`,
  `accounts-detail-limit-160`, `accounts-detail-one-160`, `accounts-detail-sessions-160` and
  `accounts-detail-work-160`; 148 to 160 of `accounts-compare-month-160` (`< 30 days >`),
  `history-tokens-160` and `history-tokens-models-160` (`< by week >`); 144 to 160 of
  `history-weeks-160`, `history-weeks-columns-160`, `history-weeks-ten-160`, `history-days-160` and
  `history-days-ten-160`; and 141 to 160 of `history-year-160` and `history-year-opus-160`
  (`< last 12 months >`). On a phone, the period's own row, row 7, under the heading:
  `accounts-phone-compare-52`, `accounts-phone-detail-52` and `history-phone-year-52`.
- **Accounts' key line,** row 37, of each frame wider than a phone, as its period, its pin and its
  groups' spacing all differ. The dashboard draws Compare's
  `tab views   [ ] view    < > period: this week    ←→ account  enter its page    p pin    ? help  q quit`
  in `accounts-compare-160`, `accounts-compare-bad-hour-160`, `accounts-compare-month-160`
  (`30 days`) and `accounts-compare-ten-160`; Detail's, the same with `↑↓ account`, in
  `accounts-detail-all-160`, `accounts-detail-limit-160` and `accounts-detail-work-160`; and in
  `accounts-detail-sessions-160`, whose fixture reaches SESSIONS, as d28c's row, lit, and
  `enter opens d28c's page` read, where the frame draws the list's line, SESSIONS',
  `↑↓ session  ← accounts  enter its page` in place of `↑↓ account  enter its page`; and with one
  account, in `accounts-detail-one-160`, `tab views   < > period: this week    ? help  q quit`.
- **History's key line,** row 37, of each frame wider than a phone, as its period, its pin and its
  groups' spacing differ: from column 24, where the frames' `p period` starts and the dashboard
  leaves a cell more after `[ ] view`, to the last cell either draws otherwise,
  `tab views   [ ] view` held, and `? help  q quit` where both end the line there. The dashboard
  draws the Year's
  `tab views   [ ] view    < > period: last 12 months    v split: all    p pin    ? help  q quit`,
  to column 93 of `history-year-160`, and with `v split: Opus 5.5` to column 82 of
  `history-year-opus-160`; Weeks',
  `tab views   [ ] view    v split: table    p pin    ? help  q quit`, to column 92 of
  `history-weeks-160` and `history-weeks-ten-160`, and with `v split: columns` to 94 of
  `history-weeks-columns-160`; Days', `tab views   [ ] view    p pin    ? help  q quit`, to column
  75 of `history-days-160` and `history-days-ten-160`; and Tokens',
  `tab views   [ ] view    < > period: by week    v split: by account    p pin    ? help  q quit`,
  to column 77 of `history-tokens-160`, and with `v split: by model` to 75 of
  `history-tokens-models-160`.
- **`session-phone-52`'s key line,** row 32, which lacks `q quit`: a phone's line always ends
  `? keys   q quit`.
- **`session-turn-opened-160`'s key line,** row 37, which lacks `enter open`:
  `esc close    ↑↓ request  enter open    p pin    ? help  q quit`.
- **`sessions-one-account-160`'s key line,** row 37, which shows `p pin` with one account, where
  no page shows it: `tab views   [ ] view    ↑↓ session  enter its page    ? help  q quit`.
- **`runway-phone-52`'s key line,** row 33, which lacks `tab views`, leading a phone's line while
  it fits: `tab views   < > window   ←→ day   ? keys   q quit`.

**History's capacity, where the replay gives other numbers** (see History in `docs/design.md`):

- **`history-weeks-160`'s `without it: short` counts,** drawn 6, 5 and 3 for `work`, `personal`
  and `side`, at row 15 column 137, row 16 column 138 and row 17 column 139, where the capacity
  replay gives 7, 7 and 6.
- **`history-weeks-columns-160`'s,** the same counts, at column 114 of rows 15, 21 and 27.
- **`history-weeks-ten-160`,** whose headline the replay bears out, over the seven weeks its world
  gives spare: its short weeks, drawn 4 as now, 4 with one fewer and 6 with two fewer, and 4 in
  `all accounts`' row (row 13, column 92), where the replay gives 3 each, so all three are the
  worst, tied, and each bold `accent.attention`, where the frame bolds the 6 alone: as now's
  `short on 4 of the last 7 weeks` (row 8, columns 9 to 38), one fewer's
  `short on 4 of 7, the same weeks` (columns 87 to 117) and two fewer's count (column 150);
  `spare`'s `5% used`, in the line naming it to drop (row 9, columns 36 to 42) and in its verdict
  (row 19, columns 93 to 99), where its seven weeks average 6%, `6% used`; and `extra`'s
  `80% left at a reset` (row 22, columns 96 to 114), where its weeks average 78%,
  `78% left at a reset`.

**History, where its frames and the design part** (see History in `docs/design.md`):

- **The current month,** today's, October, where the frames light September, the month before:
  over the grid, September's name, which the dashboard draws in `text.faint`, and October's, which
  the frames leave out, over the first week holding four of its days, its days to come counted,
  in `text.secondary`: row 7, columns 111 to 113 and 119 to 121, of `history-year-160` and
  `history-year-opus-160`, and row 9, columns 39 to 41 and 47 to 49, of `history-phone-year-52`;
  and in BY MONTH, September's name, bold in the frames, and October's, bold in the dashboard:
  columns 3 to 5 of rows 31 and 32 of `history-year-160`, and of rows 24 and 25 of
  `history-year-opus-160`, and columns 2 to 4 of rows 30 and 31 of `history-phone-year-52`.
- **`▲` after side,** where new sessions go, which the frames mark beside its name, where Accounts
  and History show no routing, as the rules they share have it: column 6 of row 17 of
  `history-weeks-160` and `history-weeks-ten-160`, of row 25 of `history-weeks-columns-160`, and of
  row 26 of `history-days-160` and `history-days-ten-160`.
- **The legend,** row 35: its sample of this week, which `history-weeks-160` draws `59%…`, columns
  57 and 58, where the design and the frame of ten draw `34%…`; and in `history-weeks-columns-160`,
  columns 48 to 125, where the frame drops `each week at its peak` and draws `59%…`, the dashboard
  draws the full legend, led by `short: a week some account hit its limit   ·   `, as the design
  has it, with `34%…`.
- **`, each time work alone`** after as now's count, in `text.subtle` in
  `history-weeks-columns-160`, row 8, columns 121 to 142, where the dashboard draws it in
  `text.muted`, as part of the count, as the table's frame does.
- **A column's top cell filled to its eighth eighth,** `█████` in the frame, of every account's
  week of 24 Aug, at 66%, row 8, columns 31 to 35, and personal's this week, at 66%, row 20,
  columns 71 to 75, of `history-weeks-columns-160`: the dashboard fills a row as full as that the
  way it fills the rows under it, blanks on the row's colour, which looks the same.
- **Tokens' scroll line,** whose `·` the frames draw in `text.muted`, where the design has every
  ` · ` on the page in `text.faint`; its count, too, the world's (see below): row 33, columns 3 to
  41, of `history-tokens-160`, and row 32 of `history-tokens-models-160`.

**History, where its worlds give otherwise** (see What the worlds give otherwise than the frames),
which the dashboard draws as its world gives it:

- **The Year's numbers:** in `history-year-160`, `requests`, `236,410`, row 8, columns 140 to 146;
  `limits hit`, `5-hour ×402`, row 10, columns 140 to 160, which the world gives
  `5-hour ×412 · week ×3`; `worth`, `$27,480`, row 11, columns 140 to 146; `against it`, `3.8×`,
  row 13, columns 140 to 143; `busiest day`, `Tue 22 Sep`, row 14, columns 140 to 149; and
  `longest run`, `58 days`, row 15, columns 140 to 146. On a phone, `history-phone-year-52`'s
  `requests` and `worth`, row 21, columns 11 to 17 and 37 to 43, and `limits` and `against`, row
  23, columns 11 to 25 and 37 to 40, the world's two windows' limits cut short a cell before
  `against`.
- **The Year's cuts, and the squares they shade:** the key, `under 450`, `450–750`, `750–1,000` and
  `over 1,000`, which the world's active days cut at 361, 631 and 927, rounded `350`, `650` and
  `950`: row 16, columns 32 to 91, of `history-year-160`, and row 18, columns 8 to 52, of
  `history-phone-year-52`. The squares the frames shade otherwise, every one a weekday, drawn as
  the world's records shade them (`yearSquares` and `phoneSquares` in `reference_test.go`): 43 of
  `history-year-160`'s, of rows 8 to 12, and of the phone's, the same days among its 22 weeks, 14
  of rows 10 to 14.
- **BY MONTH:** August's and September's limits, `55` and `60`, rows 30 and 31, columns 49 and 50,
  of `history-year-160`; September's bar's split by family, row 31, columns 77, 78 and 89, as the
  world's last 30 days are Accounts' versions, and on a phone row 30, column 43; October so far,
  `1,548` requests, `$180` and `2` limits, row 32, columns 15 to 19, 27 to 30 and 49, where today's
  lines give 1,239, $126 and none, and its bar and `so far`, columns 62 to 70, a cell shorter, and
  on a phone its `$180`, row 31, columns 13 to 16; and August's phone bar, row 29, column 49, 18 ×
  $2,860 ÷ $3,120 = 16.5 cells, which the frame rounds to even, 16, where the world's worths, each
  a hair over the frame's through their tokens, August's the more, give a hair over the half, 17.
- **The Year filtered to Opus 5.5,** `history-year-opus-160`: its numbers, `48,880` requests, `352`
  sessions, `$5,760`, `21%` of all use and its busiest day, `Tue 22 Sep`, rows 8 to 11 and 13,
  columns 140 to 149; its cuts, `400`, `600` and `700`, which its active days cut at 312, 485 and
  590, rounded `300`, `500` and `600`, row 16, columns 32 to 71; 17 of its squares (`opusSquares`),
  of rows 8 to 12; and BY MONTH's rows from June, their requests, worth and share where they differ,
  columns 15 to 20, 27 to 32 and 39 to 42, of rows 21 to 25, and its bars where they're longer, row
  21, columns 67 to 82, with `from the 22nd`, row 22, columns 84 and 85, and row 23, columns 87 to
  89.
- **Weeks:** every account's week of 31 Aug in `history-weeks-ten-160`, row 13, column 42, the
  mean of the ten's peaks, 54.5%, which the frame rounds to even, `54%`, where the dashboard's
  rounding of it, a hair over the half as a float, gives `55%`.
- **Days' strips, their shares and limits:** in `history-days-160` and `history-days-ten-160`, the
  days the frames shade otherwise than the world's records, each 2 cells, among them the days its
  accounts used nothing, Saturday 15 and Sunday 16 August, Sunday 6 September and Sunday 27
  September, which the frames shade and the world, as the Year's frames do, leaves at level 0, and
  this week's limits, work's on Wednesday and personal's, as the Log tells of them, where the
  frames mark work's on Thursday (`daysStrips` and `daysTenStrips`, rows 11 to 17 and from row
  22); BY MODEL's shares, Opus's and Sonnet's `59%` and `37%`, rows 11 and 13, columns 138 and 139,
  of `history-days-160`, and Fable's `1%`, row 15, column 139, of `history-days-ten-160`; and
  personal's limits, `11`, row 24, column 140, of both, where the world counts yesterday's.
- **Tokens:** each period's counts, worth and against its price, and each account's or version's,
  that the frames give otherwise, a figure or a run of them (`byAccountFigures` and
  `byModelFigures`), from column 39 to 110 of rows 9 to 12, 14 to 17, 19 to 22, 24 to 27 and 29
  to 32 of `history-tokens-160`, and of rows 9 to 15, 17 to 23 and 25 to 31 of
  `history-tokens-models-160`; personal's and side's models counts, `4`, column 15 of rows 16, 21,
  26, 31 and 32 of `history-tokens-160`; the accounts each version ran on, row 10, columns 15 to
  23, and rows 22 and 30, columns 20 to 32, of `history-tokens-models-160`; and the weeks below
  those shown, `3` and `5` in the frames, which drew eight weeks, where the world holds the 12
  months, 48 and 50 below, in the scroll line passed over above.

**The Overview, and the page behind the routing card, where their frames and the design part:**

- **Even pace, a cell off,** as in `usage`'s frames, on the same windows: work's week and Fable
  week on the compact matrix's row 9, columns 73 and 74 and 121 and 122, of
  `overview-default-160`, `overview-pinned-160`, `overview-bad-hour-160`,
  `overview-usage-alone-160`, `overview-everything-160x39` and `overview-six-accounts-160`, and
  above the card, of `routing-auto-160`, `routing-two-160`, `routing-move-160` and
  `routing-limit-160`; columns 74 and 75 and 122 and 123 of `overview-ten-accounts-160`, whose
  numbers take two cells; by account, columns 52 and 53 of rows 9 and 11 of
  `overview-by-account-160`; and in the big bars of `overview-default-216`,
  `overview-sessions-216` and `overview-everything-216x106`, work's 5-hour window, 50.7% gone,
  placed as if 51%, row 9, columns 103 and 104, and its week and Fable week, rows 20 and 31,
  columns 107 and 108.
- **Even pace, placed otherwise than the design's rule places it for the reset the frame gives,**
  on the extras of `overview-six-accounts-160`, and of `overview-ten-accounts-160` a column on:
  client's 5-hour window, resetting at 17:40, row 15, columns 23 and 24, and its week and Fable
  week, at Sun 23:00, columns 73 and 74 and 121 and 122; spare's 5-hour window, at 19:35, row 17,
  columns 17 and 18, the frame's `▋┃`, where the rule puts `┃` on the bar's first cell; and lab's
  week and Fable week, at Mon 08:00, row 19, columns 72 and 73 and 120 and 121. Of ten alone:
  team's 5-hour window, at 15:30, row 21, columns 29 to 31, and its week and Fable week, at Sat
  22:00, columns 76 and 77 and 124 and 125; and studio's 5-hour window, at 16:20, row 27, columns
  27 to 29, and its week and Fable week, at Sun 06:00, columns 72 to 76 and 120 to 124.
- **GRAPHS' now, a column late,** in work's 5-hour chart at 216 columns, 64 cells wide: the frames
  place now's column as if its pace were 51%, a column past the design's
  `min(round(pace × width), width − 1)`, so they fill column 50, past the dashboard's now, and start
  `now` on the axis a cell on, columns 48 to 51: rows 23 to 40, and 41, of
  `overview-sessions-graphs-216`, and rows 45 to 62, and 63, of `overview-everything-216x106`.
- **A state cut by whole clauses,** beside a block, by account: row 7 of `overview-by-account-160`,
  columns 133 to 160, where the frame runs work's state to the edge, and the design ends it two
  cells before it, so the dashboard draws `● at its cap · back 17:10` without
  ` · new sessions go elsewhere`.
- **A start under a pin, `as pinned`:** row 33 of `overview-bad-hour-160`, columns 112 to 137,
  the frame's `: personal is at its limit` after `c61b started on side`, under a pin of personal
  and side; LATELY's marks word a start under a pin `as pinned`, which the dashboard draws. The
  design doesn't say how a start under a pin of several reads where it passed the first over.
- **A folded heading's first line, as its section draws it:** row 37 of
  `overview-everything-160x39`, columns 117 to 120, LATELY's `side`, which the frame draws in
  `text.muted`; folded, LATELY keeps its first row as drawn, its object in bold `text.secondary`.
- **An account out of the pool, faint wherever the page names it,** where the frames draw
  COMING UP's and LATELY's names in their colours: in `overview-pinned-160`, `work` at columns 8 to
  11 and 92 to 95 of row 34, and `side` at 134 to 137 of row 34 and 8 to 11 of row 35; in
  `overview-bad-hour-160`, `work` at 92 to 95 of row 34.

**The Overview, and the page behind the routing card, where their worlds give otherwise** (see What
the worlds give otherwise than the frames), which the dashboard draws as its world gives it:

- **Side's week, where it runs out,** `out ~Fri 12:20`, as in `usage`'s frames: row 13, columns 88
  to 108, of `overview-default-160`, `overview-pinned-160`, `overview-bad-hour-160`,
  `overview-usage-alone-160`, `overview-everything-160x39` and `overview-six-accounts-160`, 89 to
  109 of `overview-ten-accounts-160`, and 126 to 146 of `overview-sessions-graphs-216`; row 23,
  columns 82 to 102, of `overview-by-account-160`; row 24, columns 194 to 214, of
  `overview-default-216`, `overview-sessions-216` and `overview-everything-216x106`; and short,
  `out ~Fri`, columns 38 to 49 of row 25 of `overview-phone-52`.
- **Personal's 5-hour window at its limit,** which the frames place as if it reset at 16:30, as the
  auto world's does, though they say the limit lifts at 15:54, when the world's window, primed at
  10:54, resets: its even pace, row 11, columns 27 to 29, and the pool's, row 15, columns 23 and 24
  of `overview-bad-hour-160` and 24 and 25 of `routing-limit-160`.
- **The extras' typed figures,** in `overview-six-accounts-160`, and in `overview-ten-accounts-160`
  a column on: client's 5-hour words, `16%/h till 17:40`, row 15, columns 40 to 55, where the world
  has it running out ~16:40, and so under pressure in ROUTING, row 30, columns 17 to 45 (row 38 of
  ten), the frame's `● open`; its week's and Fable week's allowances, `14%/day` and `25%/day`,
  columns 88 to 94 and 136 to 142; spare's 5-hour rate, `19%/h`, row 17, columns 40 to 44, and its
  week's and Fable week's words, which the frames reset `Thu 18:00` with their even pace two days
  in, where the world resets them on Tuesdays at 14:00, as that pace places them and History's Weeks
  needs, columns 88 to 109 and 136 to 157; lab's 5-hour even pace, row 19, columns 26 and 27, its
  window starting at 11:55 in the world, later than the frame's; its week's words,
  `11%/day till Mon 08:00`, columns 88 to 109, where the world has it run out ~Sat 23:41; its Fable
  week's allowance, `20%/day`, columns 136 to 142; and the pool's week's and Fable week's even
  paces, the mean of its accounts', row 21, columns 73 to 75 and 121 to 123 (row 29 of ten). Of ten
  alone: team's week's words, `4%/day till Sat 22:00`, row 21, columns 89 to 109, where the world
  has it run out ~Fri 13:47, and its Fable week's allowance, columns 137 to 143; and the 5-hour
  rates and the weeks' allowances of extra, ops and studio, rows 23, 25 and 27, columns 41 to 45, 89
  to 95 and 137 to 143.
- **COMING UP,** as each world's status document gives it: at 216 columns, side's week running out
  comes fourth, so the frames' lines from their fourth are a line early, columns 1 to 97, rows 58
  to 62 of `overview-default-216`, 67 and 68 of `overview-sessions-216`, and 91 to 95 of
  `overview-everything-216x106`, where the frames' eighth rows are blank; in the bad hour, rows 33
  to 35, columns 1 to 72, which list work back from its cap at 17:10 and not personal's week running
  out; under routing's limit, row 33, columns 1 to 72, which starts
  `15:54  personal back from its limit`; and of six and ten accounts, rows 37 to 39 and 49 to 51,
  columns 1 to 72, which start with lab running out ~16:05, of ten after team back from its limit
  at 15:30, where the frames list the auto world's three.
- **LATELY at 216 columns,** from its eighth line, as the world's day holds d28c's start at 12:02:
  rows 62 to 66 of `overview-default-216` and 95 to 99 of `overview-everything-216x106`, columns 111
  to 216.
- **The traces and their rates,** which the frames draw from the generators' curves: each trace's
  cells, from column 93 to eight columns from the edge, where its rate starts, and its rate.
  ROUTING's at size 2, on rows 21 and 22, 24 and 25, and 27 and 28 of `overview-default-160`,
  `overview-pinned-160` and `overview-bad-hour-160`, and of `routing-auto-160`, `routing-two-160`,
  `routing-move-160` and `routing-limit-160` beside the card, from column 122, and under it, on row
  28; on rows 43 to 50 of `overview-default-216` and `overview-sessions-216`, 45 to 52 of
  `overview-sessions-graphs-216`, and 67 to 74 of `overview-everything-216x106`; personal's upper
  row, blank in the frames, held blank. At size 1, rows 31 to 33 of `overview-everything-160x39`, 27
  to 32 of `overview-six-accounts-160` and 35 to 44 of `overview-ten-accounts-160`. Beside USAGE's
  blocks, rows 11, 18, 25 and 32 of `overview-by-account-160`, from column 108. On a phone, rows 46,
  49 and 52 of `overview-phone-52`, the trace at columns 17 to 32 and the rate at 46 to 52. And
  SESSIONS', rows 55 to 60 of `overview-sessions-216`, 57 to 62 of `overview-sessions-graphs-216`
  and 79 to 84 of `overview-everything-216x106`. The dashboard's own tests hold how a trace is
  drawn.
- **GRAPHS' fills,** of work's windows, which its world read otherwise: in
  `overview-everything-160x39`, the 5-hour window's, rows 22 to 26, columns 17 to 37, the week's,
  rows 25 and 26, columns 68 to 87, and the Fable week's, row 26, columns 118 to 136; in
  `overview-sessions-graphs-216`, rows 23 to 40, columns 17 to 47, rows 34 to 40, columns 87 to
  117, and rows 38 to 40, columns 158 to 184; and in `overview-everything-216x106`, the same
  columns of rows 45 to 62, 56 to 62 and 60 to 62.

**The routing card:**

- **The windows' figures,** coloured along a ramp in the frames, where the design colours them by
  level, as `usage` does: the dashboard draws them in the ramp's first stop below 70%, its second
  from 70% and its last from 90%. Each account's `5-hour` figure at columns 76 to 78 and its `week`
  at 87 to 89, of work on row 15, personal on 16 and side on 17, in `routing-auto-160`,
  `routing-two-160`, `routing-move-160` and `routing-limit-160`, whose personal at its limit,
  `100%`, the frame colours as the design does.
- **The cursor's row, lit,** which the frames draw on the canvas, where the design has the row the
  cursor's on lit on `bg.selection` always, as the Log's `f` panel lights its row (Routing by hand,
  "Its keys"; the owner's call, 12 October 2026): columns 42 to 119, from a cell before the card's
  content to a cell after it, its glyphs, colours and bold kept, of row 14, `■ 0 auto`, in
  `routing-auto-160`; row 17, `■ 3 side`, in `routing-two-160` and `routing-limit-160`; and row 16,
  `■ 2 personal`, in `routing-move-160`.
- **Personal's week in `routing-limit-160`,** row 16, columns 87 to 89, drawn `64%`, where the world
  reads it at 66%, as the Overview's frames draw it (see What the worlds give otherwise than the
  frames): the card draws `66%`.
- **The move's cost in `routing-move-160`,** row 23, columns 76 to 79, `6.20` of `≈ $6.20`, typed by hand:
  the auto world's ledger, through the moving set as the watch prices it, gives side's four
  sessions' moves as `$4.15`, which the card draws. The world isn't fitted to it, as its sessions'
  lines feed the frames owed at later stages.

**The Log, where its frames and the worlds part** (see What the worlds give otherwise than the
frames):

- **Requests' tail,** rows 8 to 34 of `log-requests-160`, its newest 27 lines, from `14:40:26`, and
  rows 9 to 30 of `log-requests-phone-52`, its newest 22, from `14:40:40`: the frames' generator
  drew d28c's lines, where the world's are as its session's page draws them, and the requests in
  flight, where the world's are the Overview's frames' (41e0's, c61b's and d28c's): the dashboard
  draws the world's lines in their place, which `log-not-live-160`, over a world of its own, holds
  cell for cell. Today's count on the state line, `1,240` of `1,240 lines today`, row 36, columns
  144 to 148, and on a phone row 32, columns 36 to 40, is the world's: `1,242`, its 1,239 requests
  and 3 in flight.
- **The rows around the cap,** behind `log-request-details-160`'s card, for the same reason: row 10
  whole, `14:31:18  9c2e`; the first 16 columns of rows 11 to 31, each line's time and session,
  from `14:31:20  9c2e`; the last 16 of rows 11 and 15, blank, and of rows 12 and 16, the ends of
  7f3a's and db8a's moves, `s cap  $0.98` and `s cap  $0.66`; and rows 32 to 34 whole, from
  `14:32:29`. The state line's `214` of `▼ 214 new`, row 36, columns 26 to 28, is the world's
  count of the requests since the fixture paused at 14:32:41, `126`, and `1,240`, columns 144 to
  148, its `1,242`.
- **Anthropic's id,** row 15 of `log-request-details-160`, columns 35 to 62, `req_011CQx…`: the
  frame cut an id longer than the column's room, and the world's, `req_011CQx4n7RkV2bW9pT3sYd6M`,
  as long as Anthropic's are, fits it whole, as the dashboard draws it.
- **d28c's lines alone,** rows 8 to 34 of `log-requests-filtered-160`, its newest 27, from
  `14:36:27`: the frame's generator drew them, where the world's are as d28c's page draws them, and
  its request in flight the Overview's frames' (`14:42:06`, with no directory); the dashboard draws
  the world's in their place. The counts are the world's, as `log-requests-160`'s: `830` of `830
  hidden`, row 5, columns 133 to 135, is `831`; and `445` of `445 of d28c's today`, row 36,
  columns 142 to 144, is `446`.
- **db8a's move of 13:22:40,** row 27 of `log-events-160`, columns 88 to 160, `, its 5-hour to run
  out at 15:10   48k cached again, $0.48`: no pressure event of personal's behind the run-out fits
  in the world, as none shows among the frame's rows, and a move made afresh after 21 hours idle
  writes no cache again, its cache gone cold; the dashboard draws `personal came under pressure`
  alone. Its last cell shows past the details card's edge in `log-event-details-160`, row 27,
  column 145, the `8` of `$0.48`, faded: the dashboard draws the row ending under the card.
- **Behind the `f` panel,** faded: in `log-requests-filter-160`, the Requests' tail, as
  `log-requests-160` passes it over, left of the panel, rows 8 to 29, columns 1 to 18, and whole
  under it, rows 30 to 34, with today's count on the state line, row 36, columns 144 to 148,
  `1,240`; and in `log-events-filter-160`, db8a's move's cost right of the panel, row 27, columns
  143 to 145, `.48`, as `log-events-160` passes it over.
- **The panel's counts of today's lines** in `log-requests-filter-160`, the world's, as the state
  line's: `1,275` of `which of today's 1,275 lines`, row 10, columns 70 to 74, drawn `1,277`;
  requests' `1,240`, row 15, columns 83 to 87, drawn `1,242`; and `1,240 of 1,275`, row 26,
  columns 31 to 45, drawn `1,242 of 1,277`.
- **The panel's models,** row 14 of `log-requests-filter-160`, columns 98 to 110, blank in the
  frame: the world's lines today ask Sonnet 5.5 too, 41e0's in flight among them, so the dashboard
  ticks `sonnet-5-5` after `haiku-4-5`.
- **Today's sessions in the panel,** whose order and doing are the world's: 41e0, answering, seen
  last, lists first, and db8a, its last request ended over a minute before, reads `idle 1m`, where
  the frames list d28c first and draw db8a busy. In `log-requests-filter-160`, rows 18, 20, 21 and
  22, columns 73 to 131, `□ d28c`, `□ 7f3a`, `□ db8a` and `□ 41e0`, the cursor on d28c, third in
  the world's list, lighting row 20; in `log-events-filter-160`, rows 15, 17, 18 and 19, columns
  80 to 138, the same four.
- **The cursor's tick, lit,** in `log-events-filter-160`, row 13, columns 21 to 31, `■ started`,
  where the cursor starts: the frame lights no tick, where the design lights the row the cursor's
  on, as the routing card's (see The routing card).

**Accounts, where its frames and the design part** (see Accounts in `docs/design.md`):

- **`[ ]` after Detail alone,** on `accounts-detail-one-160`'s heading row, row 5, columns 21 to 25,
  where with one account the heading shows ` Detail ` alone, with no sub-tabs and so no `[ ]`
  (1834–1835, 1947): the dashboard draws the rule two cells after ` Detail `.

- **`▲` after side,** where new sessions go, which the frames mark, where Accounts and History show
  no routing (1744–1745), as History's frames pass it over: on Compare's heads, row 7, column 100
  of `accounts-compare-160`, `accounts-compare-bad-hour-160` and `accounts-compare-month-160`, and
  72 of `accounts-compare-ten-160`; in Detail's list, row 13, column 9, of
  `accounts-detail-all-160`, `accounts-detail-limit-160`, `accounts-detail-sessions-160` and
  `accounts-detail-work-160`; on Detail's head, `3 side ▲`, in `accounts-detail-sessions-160`, row 7,
  columns 32 to 66, its state two cells sooner without it; and on a phone, column 7 of rows 15, 25,
  32, 38 and 44 of `accounts-phone-compare-52`. Of `1 work ▲`, `accounts-detail-one-160`'s head,
  row 7, column 8, where the dashboard marks work, the primary, `◆`, as `usage` marks it with one
  account (see `usage-one-account-160`, above).
- **Haiku 4.5's shade,** which the frames draw as Haiku's own, where the version table holds Haiku
  5.5, so the dashboard mixes 4.5's 30% toward the canvas, as an older version's (1755–1758): its
  `●` in Compare's frames, row 18, column 3, and its cells of Compare's bars, row 12, columns 83 and
  149 of `accounts-compare-160`, 83 of `accounts-compare-month-160`, and 60, 98 and 117 of
  `accounts-compare-ten-160`; of Detail's bar, row 16, and its `●`, row 17: columns 139 and 140, and
  115, of `accounts-detail-work-160`, and 140, and 87, of `accounts-detail-sessions-160`; and on a
  phone, row 23, column 33, of `accounts-phone-compare-52`, and row 24, column 50, and row 26,
  column 32, of `accounts-phone-detail-52`. All accounts' bar in `accounts-detail-all-160` is passed
  over with its shares, below.
- **A stale reading's percentage, bold,** work's `34%` on WEEK USED in
  `accounts-compare-bad-hour-160`, row 10, columns 47 to 49, where `usage` draws a stale reading's
  percentage in `text.subtle`, nothing bold (1279–1281).
- **Even pace a cell off,** placed as if it were a whole percent, in `accounts-detail-one-160`:
  work's 5-hour window, row 10, columns 35 and 36, and its week and Fable week, rows 12 and 14,
  columns 36 and 37.
- **All accounts' WEEKS,** in `accounts-detail-all-160`, row 24, whose bars the frame fills to the
  mean of the accounts' peaks rounded to a whole percent, where the dashboard fills them to the mean
  itself (1890–1891): the week of 14 September's, column 97, `▋`, to 73%, where the mean of 81%, 90%
  and 47% is 72.7%, `▌`; and this week's, column 132, `▎`, to 59%, where the mean is 59.3%, `▍`.
- **LATELY of an account,** which tells the account's events, the moves onto it and the starts on
  it (1899–1903), where the frames leave some out: in `accounts-detail-work-160`, row 30, columns 37
  to 160, the frame's `12:10  ◇ primed: its 5-hour window started`, where the dashboard tells db8a's
  move onto work, `13:22  ▸ db8a came from personal: personal came under pressure`, and on a phone,
  row 38 of `accounts-phone-detail-52`; in `accounts-detail-limit-160`, row 30, columns 37 to 160,
  the frame's `11:30  ◇ primed`, where the dashboard tells 41e0's start on personal,
  `12:47  ▲ 41e0 started here`, the bad hour's morning holding it and 9c2e's at 11:55 after its
  prime at 10:54, as its 5-hour window resets at 15:54; `, the best` after 7f3a's start, as 7f3a
  started on the router's best, as its reason says, wherever ▲ marks since (1901–1902), in
  `accounts-detail-one-160`, after `7f3a started here`, row 29, columns 39 to 49, in
  `accounts-detail-work-160`, row 29, columns 63 to 72, in `accounts-detail-all-160`, after
  `7f3a started on work`, row 30, columns 66 to 75, and on a phone, row 37, columns 27 to 36 of
  `accounts-phone-detail-52`; and in `accounts-detail-one-160`, the pressure event's mark, row 28,
  column 20, `●`, drawn before each kind of event had a mark of its own, where the Log's marks give
  `◔`, in `accent.attention`.
- **Detail's key line** in `accounts-detail-sessions-160`, under Key lines drawn before the keys were
  swept, above.

**Accounts, where its worlds give otherwise** (see What the worlds give otherwise than the frames),
which the dashboard draws as its world gives it:

- **This week of the three,** as the auto world counts it, in `accounts-compare-160`, in
  `accounts-compare-ten-160`, whose columns are 19 wide, personal's from column 46 and side's from
  65, and on a phone: personal's and side's sessions served, `6` and `11`, row 22, where the world
  gives 10 and 14; work's moves, `3 out`, row 23, which the world gives `1 in · 3 out`, db8a's move
  from personal under pressure at 13:22:40, as the Log's frames draw it, its move in; personal's,
  `1 out`, the world's `2 out`, that move too; and personal's limits hit, `none`, and its time at a
  limit, `—`, rows 26 and 27, where the world counts its limit of 19:12:40 yesterday, `5-hour ×1` and
  `4h 45m`. All accounts' too, in `accounts-compare-160`, from column 126: BY MODEL's Opus 5.5,
  Sonnet 5.5 and Haiku 4.5, `57%`, `33%` and `3%`, rows 13, 15 and 18, the world's 56%, 34% and 2%;
  sessions served, `19`, the world's 27; moves, `4 moves`, its 5; limits hit, `3`, its 4; and at cap
  or limit, `4h 02m`, its 8h 47m. On a phone, in `accounts-phone-compare-52`, the same: sessions
  served, columns 24 and 25 of rows 31 to 33; moves, from column 36 of rows 30, 31 and 33; personal's
  limits and time at a limit, row 37, columns 14 to 18 and 27 to 32, and all accounts', row 39,
  column 14 and columns 27 to 32; and all accounts' BY MODEL, its bar, row 27, columns 32 and 33,
  and its `57%`, columns 47 to 49. In Detail, `accounts-detail-all-160`'s: THIS WEEK's limits hit,
  `3`, row 11, column 119; BY MODEL's `57%`, `33%` and `3%`, row 17, columns 48 to 50, 79 to 81 and
  127 and 128, and its bar, row 16, columns 136 to 140; and the share left at a reset in its
  verdict, `30%`, row 26, columns 50 to 52, the world's 32%.
- **The ten's,** in `accounts-compare-ten-160`: client's state, `● open`, row 8, columns 84 to 93,
  where the world has client run out ~16:40, under pressure, as the Overview's frames of ten have it;
  lab's moves, `1 out`, row 23, and its time at a limit, `35m`, row 27, from column 122, where the
  world gives none and `2h`; client's and lab's left at their last reset, `22%` and `30%`, row 28,
  from columns 84 and 122, the world's 37% and 21%; and all accounts' column, from column 141, typed,
  not summed from its rows: BY MODEL's `53%`, `4%`, `4%` and `1%`, rows 13, 14, 16 and 17, the
  world's 55%, 3%, 2% and 2%; and `12,598` requests, `51` sessions served, `10 moves`, `9h 57m` at cap
  or limit, `$1,342` and `3.4×`, rows 21 to 23, 27, 33 and 34, the world's 11,698, 53, 5, 13h 31m,
  $1,310 and 3.3×.
- **The 30 days,** typed, in `accounts-compare-month-160`: each column's requests, sessions served,
  moves, limits hit, time at cap or limit, worth and against its price, rows 21 to 23, 26, 27, 33
  and 34, from column 27, but all accounts' `31 moves`, which the world gives; and all accounts' BY
  MODEL, its bar, row 12, columns 126 to 160, and its Opus 5.5, Sonnet 5.5 and Fable 5.1, `54%`,
  `35%` and `2%`, rows 13, 15 and 17, the world's 51%, 37% and 3%.
- **The bad hour's week,** which its frames draw as the auto world's, where the bad hour's world
  holds no lines today, so its week counts the days before today's: in
  `accounts-compare-bad-hour-160`, BY MODEL's bars, row 12 from column 27, and its shares, rows 13
  to 18, each column's that differs; and the rows of ACTIVITY, LIMITS, worth and against, rows 21 to
  23, 26, 27, 33 and 34, each column's that differs, personal's `5-hour ×1` and `—` among them, where
  the world counts today's limit at 14:12 as well as yesterday's. In `accounts-detail-limit-160`,
  personal's THIS WEEK, its `2,330` requests, `5-hour ×1` and `$212  4.6×`, rows 10, 11 and 13 from
  column 119; its BY MODEL, its bar, row 16, columns 37 to 140, and its versions, row 17 from column
  37; and its 5-hour window's even pace, row 10, columns 60 to 64, placed as if its window reset at
  16:30, where the world's, primed at 10:54, resets at 15:54, as in `overview-bad-hour-160`.
- **Work alone's week,** which `accounts-detail-one-160` draws as the auto world's of work, where
  work alone's world holds no lines today: THIS WEEK's `1,204` requests and `$184  4.0×`, rows 10
  and 13, from column 111; and BY MODEL, which leaves Fable 5.1 out, its bar and its versions, rows
  16 and 17, columns 13 to 143.
- **Side's week, where it runs out,** `out ~Fri 12:20`, as in `usage`'s frames: row 12, columns 81 to
  102, of `accounts-detail-sessions-160`.
- **BY DAY,** whose days the frames shade from their generators' own, and mark `▪` where they have a
  5-hour limit hit, where the world's records shade and mark them otherwise, this week's limits as
  the Log tells of them (`byDays`, in `reference_test.go`): row 21 of `accounts-detail-all-160`,
  `accounts-detail-work-160`, `accounts-detail-limit-160`, `accounts-detail-sessions-160` and
  `accounts-detail-one-160`, and row 28 of `accounts-phone-detail-52`, the days that differ, each 2
  cells, or 3 with one account.

**A session's page, where its frames and the design part:**

- **The tab lit,** on the title row: the frames light ` Overview `, where the fixtures reach the
  page from the Log, `/`, the session's id, `enter`, `↑` and `s`, as the Overview has no room for
  SESSIONS at 37 rows, nor ever shows an ended session, so the dashboard lights ` Log `: row 1,
  columns 16 to 25 and 68 to 72, of `session-moved-160`, `session-quiet-160`, `session-ended-160`,
  `session-resumed-160`, `session-turn-opened-160` and `routing-session-160`; and on a phone, row 2,
  columns 1 to 10 and 48 to 52, of `session-phone-52`.
- **`↓ streaming`,** row 7 of `session-resumed-160`, columns 1 to 11, where the design's state for
  an answer streaming is `↓ answering`.
- **`: sticky`,** after `since it started` on row 7 of `session-quiet-160` and
  `routing-session-160`, columns 41 to 48: the router lists 9c2e's reason as it began, `new`, as a
  request that keeps a session where it runs leaves the reason as it was, and the design words
  `new` `since it started` on a session's page, which the dashboard draws alone.
- **The first request of a turn opened,** chosen as the turn opens, on `bg.selection`, where the
  frame lights none: row 29 of `session-turn-opened-160`.
- **More turns than fit on a phone,** row 31 of `session-phone-52`, blank in the frame, where the
  dashboard draws `▼ 6 more turns   ·   j/k to scroll`, as the wide page's turns say it, its seven
  turns' rows holding the newest seven of its 13.
- **Behind the routing card,** in `routing-session-160`, drawn before the page's headings settled:
  STORY's heading, row 10, which goes on `, its whole life, the turn chosen lit`, where the dashboard
  draws `requests a minute` alone; personal's stay, row 20, columns 1 to 39, `personal ◆  11:55 –
  now`, which the dashboard numbers `2 personal ◆`; TURNS' heading, row 22, columns 1 to 39, which
  goes on `newest first, a prompt to the a…`, where the dashboard draws its rule. And faded, `◆`
  after personal on row 7, column 23, and on row 8 `personal`, columns 12 to 19 and 110 to 117, and
  `66%`, columns 142 to 144, which the frame fades from other colours than its own
  `session-quiet-160` draws them in, where the dashboard fades each as it draws it there.
- **The card's figures and its cursor's row,** as the routing card's over the Overview: the windows'
  figures coloured along a ramp, of work on row 18 and personal on row 19, columns 76 to 78 and 87
  to 89; and the cursor's row, `● 3 side`, row 20, columns 42 to 119, lit.
- **Where auto would send it,** `: side for now`, row 17, columns 77 to 94 of
  `routing-session-160`: the frame predates the owner's call, and the router names where it would
  send it, its own rule keeping 9c2e where it runs, so the dashboard draws `: personal for now`.
- **The key lines** of `session-phone-52` and `session-turn-opened-160`, under Key lines drawn before
  the keys were swept, above.

**A session's page, where its world gives otherwise** (see What the worlds give otherwise than the
frames), which the dashboard draws as its world gives it:

- **How long it's run,** which the frames count from the minute it started, where the world's first
  requests came at 12:02:10 and 11:55:31, as their started events tell: d28c's `2h 40m`, row 7,
  columns 155 to 160, of `session-moved-160` and `session-turn-opened-160`, and row 9, columns 47 to
  52, of `session-phone-52`, which the dashboard draws `2h 39m`; and 9c2e's `2h 47m`, row 7, columns
  155 to 160, of `session-quiet-160` and `routing-session-160`, drawn `2h 46m`.
- **The stays' points,** `≈ 41 points of its 5-hour · 6 of its week` and the like: two of the
  world's lines read their accounts' windows, 7f3a's answer of 14:30:51 work's and d28c's move side's,
  a reading each, which climb no window, so no rise is shared out, and the dashboard draws no points.
  Rows 20 and 21, from column 57, of
  `session-moved-160`, `session-turn-opened-160`, `session-ended-160` and `session-resumed-160`, and
  row 20 of `session-quiet-160`; on a phone, rows 19 and 20, from column 12, of `session-phone-52`.
- **d28c's stays,** its work's ending `14:31`, row 20, columns 23 to 27, where its last request
  there came at 14:30, and their worth, `$41.06` and `$7.14`, rows 20 and 21, columns 155 to 160 and
  156 to 160, which the world gives `$41.09` and `$7.12`; on a phone, rows 19 and 20, columns 47 to
  52 and 48 to 52.
- **b9c4's end,** `18:47`, at the start of row 7, columns 17 to 21, and on row 21, columns 33 to 37,
  and how long it ran, `5h 27m`, row 7, columns 155 to 160, where its last request came at 18:46,
  in the 11 minutes its last turn took from 18:36; its stay on work ending `16:02`, row 20, columns
  33 to 37, where its last request there came at 16:01; and its story's axis, a minute shorter, so
  `15:00`, row 18, columns 49 to 54, a column on, in `session-ended-160`.
- **b9c4's models,** row 8, columns 10 to 40, `· haiku-4-5` and `kept until 29 Dec` after it: the
  world's side requests ask Opus, as every page's world's do, so the dashboard draws `opus-5-5`
  alone, `kept until 29 Dec` after it.
- **f41a's stay on side ending `18:05`,** row 20, columns 33 to 37 of `session-resumed-160`, where
  its last request there came at 18:04:38, so its story's fold comes a column sooner: the fold, rows
  12 to 16, columns 32 to 37; side's stretch on the ribbon, its name, row 17, columns 14 to 20, and
  its end and the come-back's start, columns 32 to 37; and the fold's hours and the come-back's time
  on the axis, row 18, columns 32 to 42.
- **f41a's rescore,** after `15h 7m` idle, from 18:04:38 to 09:12:15, as the router writes it,
  where the frame says `15h`: row 7, columns 57 to 95, and the move's label, row 11, columns 36 to
  83, a column sooner too.
- **A half the generator rounds to even:** b9c4's turn 15 put out 40,500 tokens, `40k` in the frame
  and `41k` in the dashboard, row 27, columns 86 to 88, of `session-ended-160`; and f41a's turn 20
  read 16,150,000 and put out 76,500, `16.1M` and `76k` (`16.2M`, `77k`), row 27, columns 67 to 71
  and 86 to 88, and its turn 14 read 14,250,000, `14.2M` (`14.3M`), row 33, columns 67 to 71, of
  `session-resumed-160`.
- **The share from cache,** `97%` on TURNS' heading, which the frames wrote in: row 23, columns 124
  to 126 of `session-ended-160` and 123 to 125 of `session-resumed-160`, which their worlds give
  `98%`.
- **b9c4's turn 8's tools,** row 34, columns 33 to 45, `Edit ×2, Bash ×2`, which the generator
  listed in its sample's order, where the views list those called as often by their names.
- **The move's cost** in `routing-session-160`, `1.56` of `≈ $1.56`, row 23, columns 73 to 76: 9c2e's
  context, priced as the watch prices a move, which the dashboard draws `$1.97`.
- **The stories' columns,** which the frames draw from their generator's curves, where the worlds
  spread each turn's requests over its minutes by a sine of their own: only the columns whose
  heights differ, rows 12 to 16, or 12 to 15 on a phone, each cell an eighth, `┊` where a move's
  marker runs down to its column, or blank (`storyColumns`, and `d28cStory`, `d28cPhoneStory`,
  `quietStory`, `behindTheSessionsCard`, `endedStory` and `resumedStory`, in `reference_test.go`).
  The rest of each story is held: its floor, the markers, the labels, the folds, the ribbon and the
  axis, as the world's minutes place them, and the scale, which capture pins at 8 requests a minute
  for a session's story, as its frames draw it.

## What the worlds give otherwise than the frames

`internal/capture` builds the worlds the frames were drawn from (its `testdata/README.md` says where
their data came from), and the real code works out what each frame shows from them. The generators
typed some figures by hand, and some disagree with each other or with the design's rules, so no
world gives them. Each stage that adds a view's fixtures fits what it can and passes over the rest,
cell by cell, listing each under The cells passed over with what the dashboard draws in its place
(the owner's call, 11 October 2026). Known when the worlds were built:

- **Where new sessions go.** Every frame drawn in auto marks side `▲`, though by the design's
  scoring the router would choose personal, as its week resets sooner with more left, or, of six or
  ten accounts, spare. The fake router says side, as the frames draw it (`framesChoice`): the
  document's choice is the frame's, not the scoring's (the owner's call). Where the scoring agrees
  with a frame, it's worked out.
- **`usage` (stage 5):** side's week runs out ~Fri 12:20 at its average pace, where the frames give
  its allowance, passed over; and `usage-one-account-160` counts six sessions on work, where the
  dashboard's frames of work alone count five, or three, so its world, work alone's, is fitted
  with six (`alone`, in `documents.go`).
- **The Overview (stage 7):** the traces and their rates, which the frames draw from the
  generators' curves (`dash1.rate`), where the worlds count requests from today's lines and the
  stream, and the pinned, bad hour and limit worlds hold no lines; GRAPHS' fills, which work's
  readings fill otherwise than the frames' curves; side's week runs out ~Fri 12:20 at its average
  pace, as in `usage`, which COMING UP lists; in the bad hour and under routing's limit, personal's
  week running out ~Sat 06:00 is missing from COMING UP, as an account held at its limit takes no
  request, and personal's 5-hour window resets at 15:54, as its limit lifts, where the frames place
  its even pace as if it reset at 16:30; in the bad hour, COMING UP lists work back from its cap at
  17:10, its cap lifting, though its token's refused; personal's week reads 66% under routing's
  limit, where the frame's card draws 64%; the six and ten accounts' COMING UP lists what their
  worlds give, lab running out ~16:05 and, of ten, team back from its limit at 15:30, where the
  frames list the auto world's three; and the extras' rates, allowances and run-outs (client, lab,
  team, spare, extra, ops, studio) differ from what their rows draw: spare's weeks reset on Tuesdays
  at 14:00, lab's 5-hour window starts at 11:55, and client runs out ~16:40, so it's under pressure.
  Fitted rather than passed over: the bad hour's 7f3a starts on side at 13:55:30, under the pin,
  before personal's limit, as the frame's LATELY tells nothing between that limit and work's
  refusal; the pinned world's cap moves work's sessions to side, as the frame draws it, where a
  router would move them to personal, the pin's; and no event tells of work's cap at 14:15 in the
  bad hour, as the frame tells none. LATELY (`dash2.LATELY_ALL`) and the Log's events
  (`log2.TODAY`) disagree: LATELY lacks d28c's start at 12:02:10, has a prime of personal's at
  06:25 the Log lacks, and draws work's cap without its three moves. The Overview's sessions (41e0
  sonnet answering, 7f3a sonnet idle) and the List's and the Log's (every session opus, 41e0 idle,
  7f3a answering) are two listings at one moment: each frame's world holds its own.
- **History (stage 8):** the year's $27,480 and 236,410 requests, where its own months and today
  come to $24,736 and 212,885; against 3.8× (3.4×); limits (5-hour ×402, the world's ×412 and
  work's week ×3); the busiest day (Tue 22 Sep, the world's Wed 30 Sep); the longest run (58 days,
  the world's 20); the Year's cuts (450, 750 and 1,000, where the world's active days cut at 361,
  631 and 927), and the squares the frames shade from the generator's own days; August's and
  September's limits, and September's bar's split by family, its last 30 days Accounts' versions;
  October so far, drawn 1,548 requests, $180 and 2 limits, where today's lines give 1,239, $126
  and none, and Opus 5.5's October, drawn 640, $120 and 67%, today's lines being nearly all Opus
  5.5 (1,170, $126, 100%), which makes today its busiest day; Opus 5.5's 48,880 requests, $5,760
  and 21% (41,368, $4,822, 19%), its 352 sessions (543, each counted once, as summaries of version
  4 name each version's sessions), its months from June (July to September 48%, 52% and 50% of the
  month, drawn 57%, 60% and 64%), and its cuts (400, 600 and 700, the world's 312, 485 and 590);
  Days' strips, which the frames shade from the generator's own days, among them days no account
  used, 15 and 16 August and 6 and 27 September, and work's 26 September, when its week rose none;
  Days' BY MODEL shares (Opus 59% and Sonnet 37%, the world's 55% and 41%, and of ten, Fable 1%,
  the world's 2%); personal's 5-hour limits in the eight weeks, 11, where the world counts the
  Log's of yesterday, 12, and this week's limits, work's on Thursday in the frames, Tuesday and
  Wednesday in the Log; Tokens this week, work's drawn 1.2M, 0.8M, 11.0M, 178M and 191M (the
  world's 0.83M, 1.13M, 9.1M, 177M and 188M, today's lines writing the hour's cache and putting out
  more), and all accounts' 4.2M, 2.9M, 38.0M, 615M and 660M (3.3M, 3.4M, 33.8M, 618M and 658M),
  earlier weeks' Tokens against BY MONTH, personal's versions, Fable 5.1 among them in September,
  and the accounts a version ran on, most worth first, side first on Opus 5.5 this week, as
  Accounts' versions give it; the weeks' 5-hour limits where one weekday holds two; the mean of the
  ten's peaks of 31 August, 54.5%, and August's bar on a phone, 16.5 cells, each a half the frames
  round to even, the world's float or tokens a hair over it. Fitted rather than passed over:
  spare's week resets on Tuesdays at 14:00, so its seventh week is whole, under 21 September, as
  the frame of ten draws its peaks, where the Overview's frames reset it on Thursdays at 18:00.
- **The Log (stage 9):** 1,275 lines and 1,240 requests today, where the lines are 1,274 and
  1,239, c61b's fifth request still answering; the Log's rows of d28c and 9c2e in its tail and
  around the cap, against the sessions' own lines (d28c at 14:31:49 in the Log, 14:31:58 on its
  page); a request of db8a's on work at 14:31:06, after work's cap at 14:31:03, as
  `log1.around_the_cap` made it, which no frame draws; db8a's move of 13:22:40 (no pressure event
  behind "out at 15:10", no cost of a move made afresh after 21 hours idle, and its reason the
  idle's); work's windows as 7f3a's answer of 14:30:51 read them, 95% and 34%, typed from
  `log-event-details-160`, the only line on work today that reads them, fitted rather than passed
  over. `log-not-live-160` has a world of its own: the auto world, its ledger's lines today cut
  at 14:36:18, as 1,171 of its requests had arrived, then the frame's 27 and a quota check ending
  at 14:40:52, so today counts 1,198 requests, and its router read last at 14:40:51 and gone at
  14:40:55 (`internal/capture/testdata/README.md` says how).
- **A session's page (stage 10):** d28c's split, $41.09 and $7.12 (drawn $41.06 and $7.14), its
  work ending 14:30; b9c4's stretches ending 16:01 and 18:46, ended 18:46, 5h 26m; turn 8's tools
  in another order; f41a's side stretch ending 18:04; the `% from cache` figures, written in; how
  long d28c and 9c2e have run, counted from their first requests, at 12:02:10 and 11:55:31, where
  the frames count from the minute; the stays' points, as the lines read no windows; b9c4's side
  requests, which ask Opus, not Haiku; a half its turns' tokens come to, which the generator rounds
  to even; 9c2e's move's cost; and each story's columns, which the frames draw from their
  generator's curves. Fitted rather than passed over: the ledger keeps a day's lines 90 days, as
  `kept until 29 Dec` reads; the router says auto would keep 9c2e on personal, where it runs; f41a
  is rescored after the idle the router measures, 15h 7m; and f41a's world, `resumed`, the auto
  world with f41a's lines among the sessions' world's, its router listing it in 9c2e's place, six
  sessions, as the status line counts them.
- **Accounts (stage 11):** this week's sessions served on personal and side and in all, drawn 6, 11
  and 19, where the world gives 10, 14 and 27 (yesterday's 17 and today's 14, 4 of them both, are
  27); work's and personal's moves, drawn 3 out and 1 out, and 4 in all (1 in · 3 out, 2 out and 5:
  db8a's move from personal to work under pressure at 13:22:40, as well as 8a2c's); personal's
  5-hour limit and 4h 45m at it, from the Log's limit of 19:12:40 yesterday, where Accounts draws
  none (all accounts 4 and 8h 47m, drawn 3 and 4h 02m); the 30 days; the share left at a reset (32%,
  drawn 30%); client's and lab's left at their last reset (37% and 21%, drawn 22% and 30%), lab's
  move and minutes at a limit, and client under pressure; the ten's all-accounts row (drawn 12,598
  requests, 51 sessions, 10 moves, 9h 57m, $1,342 and 3.4×, the world 11,698, 53, 5, 13h 31m, $1,310
  and 3.3×), typed, not summed from its rows; all accounts' BY MODEL this week (drawn 57/2/33/3/2/3,
  the world 56/2/34/3/2/2), and its WEEKS' bars, filled to the rounded mean of the peaks; Detail's
  BY DAY; the bad hour's week, and work alone's, which their frames draw as the auto world's, where
  neither world holds lines today, work alone's with no Fable 5.1; and the bad hour's LATELY of
  personal, which leaves out 41e0's start of 12:47. Fitted rather than passed over: every account
  reads extra usage off, rejected, as every frame draws it; work alone's world as Accounts' frame
  has it (`oneLately`, in `documents.go`), three sessions on work, which its router lists none of, as
  the frame draws no SESSIONS, and its router telling of work's prime at 12:10, 7f3a's start at
  14:12:30 and its pressure at 14:38, where `one`, usage's, has five and tells of nothing; and
  `accounts-detail-sessions-160` opens on Accounts, as its preferences keep it, so the request stream
  first opens on Detail and tells each request in flight as it went, c61b's answer seen after d28c's
  ask, as the frame orders side's sessions: reached with `tab` from the Overview, whose stream closes
  on Compare, it opens again, and tells the requests in flight as it's joined, with no time their
  answers were last heard, so SESSIONS lists d28c, asked last, first.
- **Sessions (stage 12):** c61b's worth ($0.22, drawn $0.29) and today's ($125.94, drawn
  $126.02); 8a2c's move, made yesterday, which the List shows only of today's; b9c4's move of
  yesterday, which yesterday's summary doesn't hold.
- **Runway (stage 13):** work at its 5-hour limit Wed 15:02–17:10, while b9c4 and 4b0e ran on it
  then.

## Setting up

`vhs` needs `ttyd` and `ffmpeg`, which Homebrew installs with it. Comparing a capture with its
frame needs ImageMagick, and the tapes need JetBrains Mono, as the frames used it.

```bash
brew install vhs imagemagick
vhs --version       # vhs 0.12 or later
ttyd --version
ffmpeg -version
magick -version
ls ~/Library/Fonts /Library/Fonts | grep JetBrainsMono-Regular
```

## Looking at a fixture

```bash
go run ./cmd/capturetool --fixture <name>
```

The fixture runs full screen, at the size the terminal gives, until `q`. An empty or unknown
`--fixture` is an error that lists them all. Keys work as they do in the dashboard, though the
router takes an order and changes nothing, and the clock stands still at the fixture's moment, so
nothing moves of itself. A fixture named after a frame draws that frame, at its size; milestone
5's, `runway-day-*` and `runway-week-3`, named after milestone 5's frames, stay until stage 13
replaces Runway and deletes them. Stage 7 deleted its cards' `accounts-*`, and stage 12 its
Sessions' `sessions-*` and storyboards, with their tapes.

Stage 7's, `overview-*` and `routing-*`, draw the Overview's frames and the routing card's over it,
each over its frame's world with the keys that reach it: `overview-by-account-160` with `b`,
`overview-usage-alone-160` with `r`, `c` and `l`, `overview-everything-160x39` and
`overview-everything-216x106` with `g` and `s`, `overview-sessions-216` with `s`, and
`overview-sessions-graphs-216` with `s` and `g`; `routing-auto-160` with `p`, `routing-two-160`
with `p`, `2` and `3`, and `routing-move-160` with `p`, `2` and `m`, over the auto world, which
lists the Overview's six sessions, so the card's cost is the moving set's, priced from the world's
ledger as the watch prices it; and `routing-limit-160` with `p` and `3`, over routing's world of a
limit, where the router lists all six on side. Tapes of their names screenshot
`overview-default-160`, `overview-by-account-160`, `overview-everything-216x106`,
`overview-phone-52` and `routing-move-160`, to judge by eye.

Stage 8's, `history-*`, draw History's frames, each over its frame's world, History reached from
the Overview with `tab` twice: `history-year-160` and, on a phone, `history-phone-year-52`, as
it opens, on the Year; `history-year-opus-160` with `v`, the Year filtered to Opus 5.5;
`history-weeks-160` with `]`, and `history-weeks-columns-160` with `]` and `v`; `history-days-160`
with `]` twice; and `history-tokens-160` with `]` three times, and `history-tokens-models-160` with
`v` after them; all over the auto world, but `history-weeks-ten-160` and `history-days-ten-160`,
over ten accounts'. A tape of each name screenshots it, to judge by eye.

Stage 6's, `page-*`, draw the page as it's built, to judge by eye where no frame draws it yet, each
with a tape of its name: `page-routing`, the routing card over the Overview of the auto world at
160 × 40, `p` having opened it, `2` and `3` picked and `m` ticked, the page faded behind it;
`page-routing-phone`, the card on a phone, 52 × 36, `2` picked and `m` ticked, saying what the move
costs; `page-help-scrolled`, the help over the Overview on a terminal 16 rows tall, scrolled down
three rows with `j`; and `page-not-answering`, the auto world's router gone as `R` reads it, the
status line saying so and since when, and `p pin` faint on the key line. Each is drawn over the
auto world, its records and the sessions its router lists.

Stage 9's, `log-*`, draw the Log's frames, the Log reached from the Overview with `shift-tab`:
`log-requests-160` and, on a phone, `log-requests-phone-52`, as it opens, on Requests;
`log-events-160` with `]`; all over the auto world, but `log-not-live-160`, over a world of its own,
its router gone at 14:40:55, as the fixture's settling finds it with `R` then. The details card:
`log-event-details-160`, the cap's, `↑` five times on Events and `enter`; and
`log-request-details-160`, d28c's move to side, as the Log stood at 14:32:41, `↑` to 14:31:09 and
`↓` to the move, the table paused there; then at 14:42:07, read again with `R`, `enter`. The `f`
panel: `log-events-filter-160`, over Events, with `]` and `f`; and `log-requests-filter-160`, over
Requests, with `f`, then `→` and `↓` twelve times, the cursor on d28c's row. The find:
`log-requests-filtered-160`, d28c's lines alone, found with `/ d28c` and `enter`, `worth` shown as
its preferences keep it. The page as stage 6 built it, `page-log`, went with its tapes as they came
to draw it. Tapes of their names screenshot `log-requests-160`, `log-events-160`,
`log-not-live-160`, `log-event-details-160`, `log-request-details-160`, `log-events-filter-160`,
`log-requests-filter-160` and `log-requests-filtered-160`, to judge by eye. Where no frame draws
them, `page-log-find` draws the find open on Requests, `/ sw` typed, and `page-log-help` HELP · LOG
over them, each with a tape of its name.

Stage 10's, `session-*`, draw a session's page, reached from the Overview through the Log: `shift-tab`,
then the find, `/` and the session's id, `enter` to filter the Log to its lines, `↑` to its newest,
and `s` to open its page, as the Overview's SESSIONS has no room for it at 37 rows, nor ever shows
an ended session. Over the auto world: `session-moved-160` and, on a phone, `session-phone-52`,
d28c's page; `session-turn-opened-160`, d28c's with `↓` to turn 12 and `enter`; `session-quiet-160`,
9c2e's; and `session-ended-160`, b9c4's, ended yesterday, the find walking back to its day. Over
f41a's world, `resumed`: `session-resumed-160`, f41a's, come back this morning. And
`routing-session-160`, the routing card over 9c2e's page, with `p` and `3`. Each pins its story's
scale at 8 requests a minute, as its frame draws it, where the Overview's traces are pinned at 16.
Tapes of `session-moved-160` and `session-phone-52` screenshot them, to judge by eye.

Stage 11's, `accounts-*`, draw Accounts' frames, Accounts reached from the Overview with `tab`, on
Compare as it opens: `accounts-compare-160` over the auto world, `accounts-compare-bad-hour-160`
over the bad hour's and `accounts-compare-ten-160` over ten accounts'; and
`accounts-compare-month-160` with `>`, a period longer, the last 30 days. Detail, with `]`:
`accounts-detail-all-160`, all accounts chosen as it opens; `accounts-detail-work-160` with `↓`,
work chosen; and `accounts-detail-limit-160`, over the bad hour's world, with `↓` twice, personal.
`accounts-detail-sessions-160` opens on Accounts, as its preferences keep it (see What the worlds
give otherwise than the frames), then `]`, `↓` three times to side, `→` into its SESSIONS and `↓`
to d28c. With one account, `accounts-detail-one-160`, over work alone's world as Accounts' frame of
it has it, Detail alone, as `tab` reaches it. On a phone, `accounts-phone-compare-52` with `tab`,
and `accounts-phone-detail-52` with `tab`, `]` and `↓`, work. Tapes of `accounts-compare-160` and
`accounts-detail-sessions-160` screenshot them, to judge by eye.

### `--theme`, and `NO_COLOR`

```bash
go run ./cmd/capturetool --fixture <name> --theme amber
go run ./cmd/capturetool --fixture <name> --theme path/to/lake.theme
NO_COLOR=1 go run ./cmd/capturetool --fixture <name>
```

A fixture is drawn in `nord`, the frames' theme, as the one theme chosen, on a terminal that says
its background is Nord's canvas. `--theme` names another: a built-in by its slug, or a `.theme`
file by its path, whatever it's named, as a theme written and not yet in the themes directory. It's
an input, never looked for in the themes directory, and one that doesn't load is an error, never a
theme drawn in its place. With `NO_COLOR` set, the fixture is drawn without colour, as the
dashboard is.

### `--print`

```bash
go run ./cmd/capturetool --fixture <name> --print
go run ./cmd/capturetool --fixture <name> --print --size 120x40
go run ./cmd/capturetool --fixture <name> --print --ansi
```

`--print` draws the fixture once, with no terminal, after its keys are pressed, and prints it a line
for each of the terminal's rows: at the fixture's own size, or at `--size WxH`. It prints text alone,
its escape codes stripped and each line's trailing spaces trimmed, which diffs against a frame's
`.txt`; with `--ansi`, its colours too, which `cat` shows beside the frame's `.ansi`. The same
fixture prints the same, byte for byte, in any time zone, or, over a world, any whose clocks don't
change from the moment drawn to the weeks' resets (see Determinism). A printout of `usage`, a
`usage-*` fixture, is drawn only so: as `usage` prints it, but for the blank line it prints first,
as wide as the size and as tall as it is.

```bash
diff <(go run ./cmd/capturetool --fixture usage-160 --print) testdata/vhs/reference/usage-160.txt
```

The test says where, cell by cell, colours and bold included; the diff shows the text's lines
whole.

## Running a tape

From the repository's root, as the tapes' paths are from there:

```bash
vhs testdata/vhs/<name>.tape
```

It writes the PNG its `Screenshot` names. A fixture's tape, such as `history-year-160.tape`, runs
it through `go run`, so the first run waits on a build; it waits for the frame with `Wait+Screen`
rather than sleeping a guess. `calibrate-overview-default-160.tape` cats the frame's `.ansi`
instead, for the calibration below.

### Gotcha 1: the sandbox

`vhs` drives a headless browser that connects to `ttyd` over loopback, which an agent's sandbox
refuses (`could not open ttyd: ... ERR_CONNECTION_REFUSED`). Run `vhs` with the sandbox off: in the
agent harness, the Bash tool's `dangerouslyDisableSandbox: true`. Everything else, `go build` and the
tests included, runs sandboxed.

### Gotcha 2: quoted paths

A path with a `/` in a tape must be quoted, or the tape doesn't parse:

```
Screenshot "testdata/vhs/history-year-160.png"   # parses
Screenshot testdata/vhs/history-year-160.png     # a parser error
```

### Gotcha 3: `vhs` can fail silently

`vhs` can run a tape, report nothing wrong, and write no PNG, leaving a stale capture to be judged.
Hash the capture before and after, or remove it first, and check it changed:

```bash
shasum -a 256 testdata/vhs/history-year-160.png
vhs testdata/vhs/history-year-160.tape
shasum -a 256 testdata/vhs/history-year-160.png
```

### Gotcha 4: settings come first

A `Set` after any other command is ignored, and `LetterSpacing` takes no negative value. A tape
that shows no frame fails with `no frames`, so `Show` before the `Screenshot`, and sleep a little
either side of it.

## The calibrated settings

Every tape uses these, for a terminal of C columns and R rows:

```
Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 15
Set LetterSpacing 0
Set LineHeight 1
Set Width <C × 9 + 65>
Set Height <R × 20 + 72>
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 30
Set CursorBlink false
Set Theme { "name": "nord", "background": "#2E3440", "foreground": "#D8DEE9", ... }
```

Copy the `Theme` from a tape: Nord's canvas as the terminal's background, its `text.tertiary` as
the foreground, and Nord's own sixteen colours, so a dashboard that paints no background of its own
shows on the frames' canvas.

The frames are a terminal of 9 × 20 px cells, JetBrains Mono at 15px, under a 30px title bar, the
grid 24px in and 16px down from it. The settings come from these, and from what was proven by
experiment:

- **`LetterSpacing 0`**: `vhs` defaults to 1, a pixel more for every cell.
- **The width and height**: `ttyd` and `xterm` keep 25px of the width, `ttyd`'s padding either
  side and a scrollbar's, and 10px of the height, so it takes C × 9 + 25 px of width for C columns,
  and R × 20 + 10 px of height for R rows, measured to the pixel: a pixel short of either gives a
  column or a row fewer, and the frame wraps. The 65 and 72 add `Padding` on each side, the window
  bar, and 8px of width to spare.
- **The scale**: `vhs` captures `xterm`'s canvas, which holds the cells alone, and has `ffmpeg` fit
  it to the whole terminal, padding included, so the grid comes out scaled by
  k = (R × 20 + 10) / (R × 20): 1.35% at 37 rows, its cells 9.12 × 20.27 px. No setting undoes it:
  the grid matches the frame cell for cell, a little larger. The spare width keeps the fit to the
  height, so the grid is R × 20 + 10 px tall at 46px down, under the bar, as the frames' grid is,
  and round(C × 9 × k) px wide, centred: at 160 × 37, 1459 × 750 at +23+46.

Through these settings, a frame's `.ansi` renders what its `.png` shows, cell for cell, as
`calibrate-overview-default-160` proves: once both are in sRGB and the capture's grid is scaled to
the frame's, RMSE over the whole grid is 0.07, with 1.3% of pixels more than 10% apart. What differs
is how a glyph is drawn, not where: anti-aliasing, and the block and box-drawing glyphs, which
Paper drew from the font, where `xterm` draws its own.

## Comparing a capture with its frame

By eye, the two side by side or one over the other:

```bash
magick testdata/vhs/reference/overview-default-160.png testdata/vhs/calibrate-overview-default-160.png -append side-by-side.png
```

With `magick compare`, on the terminal grid alone. The frames carry a Display P3 colour profile, so
take the frame to sRGB first, as a capture is, or every saturated colour reads as a difference;
crop the frame's grid; crop the capture's, as the calibrated settings place it, and scale it to the
frame's; then compare. At 160 × 37:

```bash
magick testdata/vhs/reference/overview-default-160.png -profile "/System/Library/ColorSync/Profiles/sRGB Profile.icc" \
  -crop 1440x740+24+46 +repage frame.png
magick testdata/vhs/calibrate-overview-default-160.png -crop 1459x750+23+46 +repage -resize '1440x740!' capture.png
magick compare -metric RMSE frame.png capture.png diff.png
magick compare -metric AE -fuzz 10% frame.png capture.png null:
```

For C × R, the frame's grid is (C × 9)x(R × 20)+24+46, and the capture's
(round(C × 9 × k))x(R × 20 + 10)+(16 + floor((C × 9 + 33 − w) / 2))+46, where w is that width.
`diff.png` marks what differs. The comparison is judged by eye in the end: anti-aliasing, bold and
the box-drawing glyphs always differ a little, and a frame's colours are its theme's tokens, not
what to read off a picture: the test holds those.

## Determinism

The same tape gives the same PNG, byte for byte, run after run. A fixture is fixed data, its clock
stands still, and its timers never fire: the harness takes the model's first read itself, 4s
before the moment drawn, then sets the clock to the moment, presses the fixture's keys, and only
then hands the model to Bubble Tea, which starts it reading nothing more. So there's no first
paint to race, and nothing on screen moves until a key does.

A fixture over milestone 5's samples is drawn at the frames' moment in a zone whose clocks never
change, fixed as the zone it's asked for stands then, so it reads the same in every zone. One over a
world, its records or, as `usage`'s printouts are, its status document alone, is built and drawn in
the machine's own zone, `time.Local` itself, as the ledger, its readers and the views split days
there, which a test can't change in process; the worlds are built there once, and every zone's
fixtures share them. So its frame reads the same in any zone whose clocks don't change from the
moment drawn to the weeks' resets. Capture's tests under `TZ` (11 October 2026), `usage`'s
printouts held to their frames among them, pass in UTC, London, Tokyo, Los Angeles, Santiago,
Havana and Auckland, and fail in Sydney, whose clocks go forward at 02:00 on Sunday 4 October:
personal's week resets at 03:00, and work's can't both reset when drawn and run its 168 hours, so
the weeks' even pace and allowances, which the printouts draw, come out a point off.

A view that follows the router's request stream has the watch open it: the fixture's tells of what
has befallen its requests, each at its time before the moment drawn, then ends, as a router ends
it. The watch keeps what it told, and asks for it again only once a timer fires, which none does,
so what travels the cords stands where the moment puts it: a pulse as far along as the time since
its request went says, a shimmer as many of the clock's steps on as have passed since its answer's
first byte. That's how a frame of motion is brought to its point in the animation.

## The capture tool and its fixtures

`cmd/capturetool` takes `--fixture`, `--scenario`, `--theme`, `--print`, `--size` and `--ansi`,
and holds the import guard. `internal/capture`, which it alone imports, holds the fixtures and their
registry, `Names` and `ByName`, in `capture.go`; the sample worlds they're drawn over; the fakes
the watch reads them through; the harness that settles a fixture at its moment and draws it; the
scenarios; and the tests that hold the fixtures to `reference/`.

`--scenario` plays a scenario through the same watch model in real time, its clock running and its
timers firing, for the README's demos: [demo/README.md](../../demo/README.md) says how.

A fixture is a moment of the dashboard: what the router and the files it reads give at it, the
terminal it's drawn on, and the keys pressed once its document is read. What they give mirrors the
frame's sample world, from the generator that drew it: the same accounts, in the same order, with
the same numbers and the same session ids. The dashboard draws its own numbers from these, by its
own rules, which the test holds to the frame's, cell for cell.

### Adding a fixture

1. Add it to `fixtures` in `internal/capture/capture.go`, named after the frame it draws: the
   frame's size, the world it's drawn over, and the keys that reach it. `Names` and `ByName` both
   read that one list, and the two drawn over the worlds, built as they're asked for: a page the
   watch draws over a world goes in `pages`, with its world and keys, and a printout of `usage` in
   `printouts`, with how it groups the windows and which it prints, as its flags have them, and its
   flags in `TestAPrintoutIsWhatUsagePrintsOfItsRouter`, which holds it to what `usage` prints given
   them. Where `holds` holds part of its frame, take its stem from there.
2. Delete its stem from `owed` in `internal/capture/reference_test.go`: from then on, the test
   holds it to its frame.
3. Where the frame drew a cell otherwise than the design says, the design holds; where its world
   can't give a figure the frame draws (see What the worlds give otherwise than the frames), fit
   the world where it can. Either way, pass over what's left in `passedOver`, beside `owed`, and
   list it here, under The cells passed over, with what the dashboard draws in its place. Never
   patch the frame.
4. To judge it by eye too, write its tape, modelled on `history-year-160.tape`, its `Width` and
   `Height` from the fixture's size, as the calibrated settings say; clear the tape and its capture
   at the milestone's sign-off. Leave the fixture.
