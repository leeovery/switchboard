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
  `cells_test.go`), passing over the same cells. Stage 6, the page, holds two parts. **The
  chrome,** rows 1 to 4, the title row and the status line, or 1 to 5 on a phone, whose tabs take a
  row of their own: of `overview-default-160`, `overview-pinned-160` and `overview-bad-hour-160`,
  over the auto, pinned and bad hour worlds; of `accounts-compare-160`, `history-year-160`,
  `sessions-switchboard-160`, `runway-5-hour-160` and `log-requests-160`, over the auto world, its
  tab reached with `tab`; and of `overview-phone-52` and `sessions-phone-52`. **The routing card's
  box,** rows 10 to 27 and columns 40 to 121, which holds nothing of the page faded behind it:
  `routing-auto-160` with `p`, `routing-two-160` with `p`, `2` and `3`, and `routing-move-160` with
  `p`, `2` and `m`, over the auto world, which lists the Overview's six sessions, so the card's cost
  is the moving set's, priced from the world's ledger as the watch prices it; and
  `routing-limit-160` with `p` and `3`, over routing's world of a limit, where the router lists all
  six on side. A frame's stem leaves `holds` as a fixture comes to draw it whole.
- **The cells passed over** are `passedOver`, by stem: a row, or a row's cells from one column to
  another, each with what the frame's `.txt` reads there, which a test checks, as it checks that
  this file names every stem in it.

## The cells passed over

The cells a frame drew otherwise than `docs/design.md` says, by row and column, counted from 1 as
the frame's `.txt` reads in an editor, and what the dashboard draws in their place, as the design
says (see Files, "The capture harness", in `docs/design.md`).

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
- **Accounts' and History's key line,** row 37, of each frame wider than a phone, as its period,
  its pin and its groups' spacing all differ. The dashboard draws Compare's
  `tab views   [ ] view    < > period: this week    ←→ account  enter its page    p pin    ? help  q quit`,
  and Detail's the same with `↑↓ account`, in `accounts-compare-160`,
  `accounts-compare-bad-hour-160`, `accounts-compare-month-160` (`30 days`),
  `accounts-compare-ten-160`, `accounts-detail-all-160`, `accounts-detail-limit-160`,
  `accounts-detail-sessions-160` and `accounts-detail-work-160`; and with one account, in
  `accounts-detail-one-160`, `tab views   < > period: this week    ? help  q quit`. History's
  Year, in `history-year-160`,
  `tab views   [ ] view    < > period: last 12 months    v split: all    p pin    ? help  q quit`,
  and in `history-year-opus-160` `v split: Opus 5.5`; Weeks, in `history-weeks-160` and
  `history-weeks-ten-160`, `tab views   [ ] view    v split: table    p pin    ? help  q quit`,
  and in `history-weeks-columns-160` `v split: columns`; Days, in `history-days-160` and
  `history-days-ten-160`, `tab views   [ ] view    p pin    ? help  q quit`; and Tokens, in
  `history-tokens-160`,
  `tab views   [ ] view    < > period: by week    v split: by account    p pin    ? help  q quit`,
  and in `history-tokens-models-160` `v split: by model`.
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
- **`history-weeks-ten-160`,** whose headline the replay bears out: its short weeks, drawn 4 as
  now (row 8, column 18), 4 with one fewer (row 8, column 96) and 6 with two fewer (row 8, column
  150), and 4 in `all accounts`' row (row 13, column 92), where the replay gives 3 each; `spare`'s
  `5% used`, in the line naming it to drop (row 9, columns 36 to 42) and in its verdict (row 19,
  columns 93 to 99), where its weeks average 6%, `6% used`; and `extra`'s `80% left at a reset`
  (row 22, columns 96 to 114), where its weeks average 78%, `78% left at a reset`.

**The routing card's box,** held in part (see How the frames are held):

- **The windows' figures,** coloured along a ramp in the frames, where the design colours them by
  level, as `usage` does: the dashboard draws them in the ramp's first stop below 70%, its second
  from 70% and its last from 90%. Each account's `5-hour` figure at columns 76 to 78 and its `week`
  at 87 to 89, of work on row 15, personal on 16 and side on 17, in `routing-auto-160`,
  `routing-two-160`, `routing-move-160` and `routing-limit-160`, whose personal at its limit,
  `100%`, the frame colours as the design does.
- **Personal's week in `routing-limit-160`,** row 16, columns 87 to 89, drawn `64%`, where the world
  reads it at 66%, as the Overview's frames draw it (see What the worlds give otherwise than the
  frames): the card draws `66%`.
- **The move's cost in `routing-move-160`,** row 23, columns 76 to 79, `6.20` of `≈ $6.20`, typed by hand:
  the auto world's ledger, through the moving set as the watch prices it, gives side's four
  sessions' moves as `$4.15`, which the card draws. The world isn't fitted to it, as its sessions'
  lines feed the frames owed at later stages.

**A pressure event's mark:** in `accounts-detail-one-160`'s LATELY, row 28, column 20, `●`, drawn
before each kind of event had a mark of its own, where the Log's marks give `◔`, in
`accent.attention`.

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
- **The Overview (stage 7):** personal's week running out ~Sat 06:00 is missing from COMING UP in
  the bad hour and under routing's limit; personal's week reads 66% under routing's limit, where
  the frame's card draws 64%; side's week runs out ~Fri 12:20 at its average pace, as in `usage`,
  which COMING UP lists; the six accounts' pooled 5-hour window works out 47%, drawn 46%; the ten's
  extras' rates and run-outs (client, lab, team, spare, extra, ops, studio) differ from what their
  rows draw. LATELY (`dash2.LATELY_ALL`) and the Log's events (`log2.TODAY`) disagree: LATELY lacks
  d28c's start at 12:02:10, has a prime of personal's at 06:25 the Log lacks, and draws work's cap
  without its three moves. The Overview's sessions (41e0 sonnet answering, 7f3a sonnet idle) and
  the List's and the Log's (every session opus, 41e0 idle, 7f3a answering) are two listings at one
  moment: each frame's world holds its own.
- **History (stage 8):** the year's $27,480 and 236,410 requests, where its own months and today
  come to $24,736 and 212,885; against 3.8× (3.4×); limits; the busiest day (Tue 22 Sep, the
  world's Wed 30 Sep); the longest run (58 days, the world's 20); the Year's cuts; August's and
  September's limits; October so far, drawn 1,548 requests, $180 and 2 limits, where today's lines
  give 1,239, $126 and none, and Opus 5.5's October, drawn 640, $120 and 67%, today's lines being
  nearly all Opus 5.5; Opus 5.5's 48,880 requests, $5,760 and 21% (40,749, $4,761, 19%), and its
  352 sessions, which no summary holds by version; Tokens this week, work's drawn 1.2M, 0.8M,
  11.0M, 178M and 191M (the world's 0.83M, 1.13M, 9.1M, 177M and 188M, today's lines writing the
  hour's cache and putting out more), and all accounts' 4.2M, 2.9M, 38.0M, 615M and 660M (3.3M,
  3.4M, 33.8M, 618M and 658M), and earlier weeks' Tokens against BY MONTH; the weeks' 5-hour limits
  where one weekday holds two; the ten's capacity, counting six weeks where spare's column is empty.
- **The Log (stage 9):** 1,275 lines and 1,240 requests today, where the lines are 1,274 and
  1,239, c61b's fifth request still answering; the Log's rows of d28c and 9c2e in its tail and
  around the cap, against the sessions' own lines (d28c at 14:31:49 in the Log, 14:31:58 on its
  page); a request of db8a's on work at 14:31:06, after work's cap at 14:31:03, as
  `log1.around_the_cap` made it, which no frame draws; db8a's move of 13:22:40 (no pressure event
  behind "out at 15:10", no cost of a move made afresh after 21 hours idle, and its reason the
  idle's); `log-not-live-160`, a world of its own, not built yet.
- **A session's page (stage 10):** d28c's split, $41.09 and $7.12 (drawn $41.06 and $7.14), its
  work ending 14:30; b9c4's stretches ending 16:01 and 18:46, ended 18:46, 5h 26m; turn 8's tools
  in another order; f41a's side stretch ending 18:04; the `% from cache` figures, written in.
- **Accounts (stage 11):** this week's sessions served on personal and side and in all, drawn 6, 11
  and 19, where the world gives 10, 14 and 27 (yesterday's 17 and today's 14, 4 of them both, are
  27); personal's moves, drawn 1 out and 4 in all (2 and 5: db8a's move under pressure at 13:22:40
  as well as 8a2c's); personal's 5-hour limit and 4h 45m at it, from the Log's limit of 19:12:40
  yesterday, where Accounts draws none (all accounts 4 and 8h 47m, drawn 3 and 4h 02m); work's 0%
  and personal's 15% left at their last reset, whose weeks reset on Sunday, before this week's
  Monday; the 30 days; the share left at a reset (32%, drawn 30%); lab's and team's minutes at a
  limit; the ten's all-accounts row (drawn 12,598 requests, 51 sessions, 10 moves, 9h 57m, $1,342
  and 3.4×, the world 11,698, 53, 5, 13h 31m, $1,310 and 3.3×), typed, not summed from its rows; all
  accounts' BY MODEL this week (drawn 57/2/33/3/2/3, the world 56/2/34/3/2/2); Detail's BY DAY
  marks; `accounts-detail-one-160`'s `[ ]` on its heading row, where the design gives one account
  Detail alone.
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
5's, `accounts-*`, `runway-day-*`, `runway-week-3` and `sessions-*`, named after milestone 5's
frames, stay until the stages that replace their views delete them.

Stage 6's, `page-*`, draw the page as it's built, to judge by eye where no frame draws it yet, each
with a tape of its name: `page-routing`, the routing card over the Overview of the auto world at
160 × 40, `p` having opened it, `2` and `3` picked and `m` ticked, the page faded behind it;
`page-routing-phone`, the card on a phone, 52 × 36, `2` picked and `m` ticked, saying what the move
costs; `page-help-scrolled`, the help over Sessions on a terminal 16 rows tall, scrolled down three
rows with `j` (`sessions-3-keys` is the help unscrolled, at 160 × 28); `page-log`, the Log's empty
tab, the six tabs in the title row, its tape at 160 × 37 and `page-log-phone`'s at 52 × 36; and
`page-not-answering`, the auto world's router gone as `R` reads it, the status line saying so and
since when, and `p pin` faint on the key line. Each but the help's is drawn over the auto world,
its records and the sessions its router lists.

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

It writes the PNG its `Screenshot` names. A fixture's tape, such as `accounts-3.tape`, runs it
through `go run`, so the first run waits on a build; it waits for the frame with `Wait+Screen`
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
Screenshot "testdata/vhs/accounts-3.png"   # parses
Screenshot testdata/vhs/accounts-3.png     # a parser error
```

### Gotcha 3: `vhs` can fail silently

`vhs` can run a tape, report nothing wrong, and write no PNG, leaving a stale capture to be judged.
Hash the capture before and after, or remove it first, and check it changed:

```bash
shasum -a 256 testdata/vhs/accounts-3.png
vhs testdata/vhs/accounts-3.tape
shasum -a 256 testdata/vhs/accounts-3.png
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
4. To judge it by eye too, write its tape, modelled on `accounts-3.tape`, its `Width` and `Height`
   from the fixture's size, as the calibrated settings say; clear the tape and its capture at the
   milestone's sign-off. Leave the fixture.
