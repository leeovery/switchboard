# The dashboard's visual capture harness (`vhs`)

This directory is where the dashboard gets looked at. Milestone 5 rebuilds `switchboard usage -w`
to the frames signed off in Paper, and every stage of it ends looking like them, as close as a
terminal allows. A stage that changes what the dashboard draws writes a `vhs` tape here,
screenshots the dashboard through it, and judges the capture against the frame it was built to.

The dashboard is drawn by `cmd/capturetool`, a program of its own, not a switchboard subcommand.
It builds the dashboard's real watch model through `watch.New`, as `usage -w` does, with every seam
faked by `internal/capture`: the router's status document, sessions and history, the clock and its
timers, the notifier, the terminal's size and background, and the themes. It never dials the
router's socket, probes, touches the network, reads or writes the real config, themes, state, prefs
or tokens, or runs another process, so it's safe to run beside a live router. A theme picked in it
is kept for the run alone. Switchboard's own binary never imports `internal/capture`:
`cmd/capturetool`'s import guard test fails if anything `cmd/switchboard` builds ever does.

## What lives here, and for how long

| Path | What | Lifetime |
|---|---|---|
| `<name>.tape` | A tape that runs a fixture, or cats a reference, and screenshots it | Scaffolding: cleared at milestone 5's sign-off |
| `<name>.png` | Its capture | Scaffolding: cleared at milestone 5's sign-off |
| `reference/*.png` | The signed-off frames, exported at 1x from the Paper file *Switchboard dashboard spikes*, page **FINAL · signed off · 2 Oct 2026** | **Kept** |
| `reference/*.txt` | Each terminal frame's character grid, from the generator that drew it | **Kept** |
| `reference/*.ansi` | Each terminal frame in 24-bit colour, from the generator that drew it: every cell painted, and no newline after its last row, so `cat` in a terminal of its size doesn't scroll it | **Kept** |

A capture is a render of the code, so it goes stale as the code moves on, and nothing checks it
against a baseline: tapes and captures are written as a stage proceeds, committed while it's
reviewed, so the reviewer opens the frame the implementer did, and cleared at the milestone's
sign-off. `vhs` writes no GIF unless a tape names an `Output`, and these name none.

`reference/` is kept: it's the design the code was built against, not a render of the code, and
it doesn't go stale the way a capture does. No Go source points at it, so a sign-off sweep mustn't
take it for orphaned. The `.png`s are the frames themselves; the `.txt` and `.ansi` are the same
frames as the mock generator drew them, before Paper did, which lets a capture be diffed as text
and lets `vhs` be calibrated against the frames. Each was checked against its `.png`, a cell at a
time, and holds what it shows; the Amber sheet's carry the edit the design session made to its
title row in Paper. `accounts-layout-rules.png` and `final-page-readme.png` are the page's notes,
not terminal frames, so have neither.

Where the frames differ from `docs/design.md`'s Dashboard section, the doc holds. The differences
known:

- **Account 1's cords** are red in the frames: in the doc they're `viz.series.1`, which is
  `accent.key` where a theme doesn't set it, and never `state.destructive`. So too the theme
  sheets' `series` rows, which open with each theme's red: the built-ins' series take none,
  `nord`'s and the Tokyo Nights' being the doc's defaults, and `amber`'s and `exchange`'s their
  sheets' other five, and a sixth of their own. So every account's cords in Sessions are the doc's
  series in `nord`: work's `accent.key`, side's `accent.primary` and client's `accent.mode`, where
  the frames' are red, `accent.key` and Aurora green.
- **The sample session ids** differ: the frames have `d28c`, `c61b`, `db8a` and so on, the doc's
  examples `5b19`, `3e7a`, `9e21`. The fixtures use the frames'.
- **The footer's keys** are the doc's, in its order, `? keys` and `q quit` always listed: where
  the frames say `? explain` or `? help`, it's `? keys`. The phone's frame lists `r refresh`,
  which the doc leaves to `?`, and drops `q quit` and when the document was read, both of which a
  phone's footer keeps. Runway's day frames list the generator's own `r refresh`, `d day`, `w
  week` and `t theme`, and its week's frame leaves out `m move`: the doc's `w` switches between
  the day and the week, the footer saying which shows, `w window: day`, as the week's frame says
  `w window: week`. Sessions' frames list `r refresh` and `t theme` too, which the doc leaves to
  `?`. Runway's and Sessions' frames say `read 4s ago · next look 1s`, where the dashboard,
  reading the router, says `read 4s ago`, as the cards' frames do.
- **Runway's lanes** go by the doc: a lane is in `accent.attention` wherever its account has room
  but is heading to run out before its reset, whichever window that is, so personal's, back from
  its limit at 15:54, is till its week runs out on Friday at 04:06, as the week's frame draws it,
  where the day's draws it in `state.positive`. And each stretch without room starts, its words
  with it, where the document says it does: personal's when the router told of it reaching its
  limit, at 14:12, so its lane has room the past hour till then, where the frames draw none all
  that hour, its words at the hour's start.
- **A card's featured window** takes the colour of its account's state, digits and chart alike,
  whichever window it is: an open account's in `state.positive`, an idle one's dim, and a week
  featured on an account under pressure in `accent.attention`. The frames draw an open account's
  digits in `text.primary`, and colour a chart, or a week's digits, by its window's own use.
- **The blends** are halfway, as the doc says: a chart's past level is its state's colour faded
  halfway into the canvas, and a bar's projection the ramp's, where the frames take 55% and 38%
  of the colour. A limit's line along a chart's floor is `state.destructive` itself, where the
  frames fade it.
- **The reserve** is the floor a chart runs out at where it holds an account back, a faint dotted
  line with `✕` on it, and `╎` marks where it starts on every bar of an account that keeps one, its
  session's too. The frames draw neither on the charts, and no mark on a session's bar.
- **A bar at its limit** fills along `viz.ramp` as every bar does, where the frames fill it solid
  `state.destructive`.
- **The key line** says the doc's words: the chart's dotted line is `heading`, where the frames say
  `where it's heading`, and `● session, lit while busy` follows the bars' glyphs, where the frames
  stop at `╎ reserve`.
- **The line over the footer, as the cards scroll,** says the doc's `▼ 2 more accounts below · j/k
  or wheel to scroll`, centred, where the frames add `header and footer stay put`.
- **LOG** shows under the calls wherever it has room, as the doc puts it there: at 28 rows, as
  sessions-3 has, its five lines fit under the calls, where the frame draws none. And it lists
  each move the router told of, and why, those a limit forced too, which RECENT and LATELY fold
  into the limit's line: so sessions-5's has db8a's and 41e0's moves off personal, where the
  frame has personal's limit and side's prime.
- **A line's use toward its reserve** says so, as a card's bar does: work's week, heading for 87%
  within ten points of its 90% reserve, reads `→ 87%` in `accent.attention`, where the Sessions
  frames' panels have it muted.
- **The storyboards** are moments of Sessions, drawn by the fixtures whole, at 160 × 28: their
  frames draw the calls and lines alone, under a caption, so a capture's rows 7 to 25 are a
  frame's rows 2 to 20. The fourth frame's line telling of d28c's move is LOG's first, under the
  calls, cut short beside the cords, its `▸` dim, as RECENT's is, where the frame draws it whole
  in `accent.mode`, under the lines; and its pulse is five cells, as every pulse is, where the
  frame's has a sixth, glowing a tenth. A call the stream has just told of was seen `now`: d28c,
  refused in the third frame, where the frame has `1m`.
- **COMING UP beside one account's card** gives each thing's countdown at the right, as COMING UP
  always does, where the frame gives none.
- **The phone's line over the footer** says `? for the key`, as any frame does where the key line
  has no room, where the frame leaves it blank.
- **An answer's tokens, as it streams,** are an estimate, `↓ ~1.2k`, as the doc says, on a card's
  back and in Sessions alike, where the frames say `↓ 1.2k`: the exact count comes as it ends.
- **An idle session's id** isn't bold, as the doc has an id bold while busy alone: on a card's back
  and among Sessions' calls, 7f3a's and 41e0's are muted, where the frames draw every id bold.
- **A session's note** says the doc's words: c61b's is `here since 14:41`, where the frames have
  `new at 14:41: side was the best`; and every row has one where there's room for every one's,
  7f3a's `here since 13:10` too, where the frames give it none.
- **LATELY** tells of an account's events as RECENT does, in the doc's words, but from its side,
  its name left out, and what follows the gist only where it fits whole: work `came under
  pressure`, where the frames add `new sessions go elsewhere`; `d28c started here, the best`;
  `primed: its 5-hour window started`, where the frames have `its window`; side's `c61b started
  here, the best`, where the frames have `side was the best`; and personal's limit, with the
  sessions it moved, `3 sessions moved to side`, as RECENT counts them, where the frames name
  `c61b, db8a and 41e0`.

The fixtures, the harness and the tool are permanent: only the tapes and captures are scaffolding.

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
go run ./cmd/capturetool --fixture accounts-3
```

The fixture runs full screen, at the size the terminal gives, until `q`. An empty or unknown
`--fixture` is an error that lists them all. Keys work as they do in `usage -w`, though the router
takes an order and changes nothing, and the clock stands still at the frames' moment, Thursday 1
October 2026 at 14:42:07, so nothing moves of itself.

| Fixture | Accounts | Size | Its frame |
|---|---|---|---|
| `accounts-1` | `work` | 160 × 27 | `accounts-1` |
| `accounts-3` | `work`, `personal`, `side` | 160 × 34 | `accounts-3-auto` |
| `accounts-3-5h` | `work`, `personal`, `side`, `w` pressed: every card featuring its 5-hour window | 160 × 34 | `accounts-3-5h` |
| `accounts-3-week` | `work`, `personal`, `side`, `w` pressed twice: every card featuring its week | 160 × 34 | `accounts-3-week` |
| `accounts-3-keys` | `work`, `personal`, `side`, `?` pressed: the help open over them | 160 × 34 | none: the frames don't draw the help |
| `accounts-3-themes` | `work`, `personal`, `side`, the theme picker open, the cursor up a theme | 160 × 34 | none: Portal draws the picker |
| `accounts-4` | and `client` | 160 × 40 | `accounts-4` |
| `accounts-5` | and `spare` | 160 × 40 | none: Sessions' and Runway's are 5 accounts |
| `accounts-6` | and `lab` | 160 × 40 | `accounts-6` |
| `accounts-8` | and `team` and `extra` | 160 × 40 | `accounts-8` |
| `accounts-8-scrolling` | the same eight, too many for 28 rows, so they scroll | 160 × 28 | `accounts-8-scrolling` |
| `accounts-flipped-all` | `work`, `personal`, `side`, four requests in flight, `s` pressed: every card flipped, the focus on work's | 160 × 34 | `accounts-flipped-all` |
| `accounts-flipped-selected` | `work`, `personal`, `side`, four requests in flight, `space` and `↓` twice pressed: work's card flipped, d28c picked out | 160 × 34 | `accounts-flipped-selected` |
| `accounts-phone` | `work`, `personal`, `side` | 52 × 36 | `accounts-phone` |
| `runway-day-3` | `work`, `personal`, `side`, `tab` pressed twice: Runway, over the day | 160 × 26 | `runway-day-3` |
| `runway-day-5` | and `client` and `spare`, `tab` pressed twice | 160 × 32 | `runway-day-5` |
| `runway-week-3` | `work`, `personal`, `side`, `tab` pressed twice, then `w`: Runway, over the week | 160 × 26 | `runway-week-3` |
| `sessions-1` | `work`, `tab` pressed: Sessions, a plain list of one account | 160 × 27 | none: the frames don't draw one account's |
| `sessions-3` | `work`, `personal`, `side`, `tab` pressed: Sessions | 160 × 28 | `sessions-3` |
| `sessions-3-keys` | `work`, `personal`, `side`, `tab` then `?` pressed: the help over Sessions | 160 × 28 | none: the frames don't draw the help |
| `sessions-5` | and `client` and `spare`, `tab` pressed | 160 × 40 | `sessions-5` |
| `sessions-storyboard-1-request-out` | the storyboard's, `tab` pressed: c61b's request 0.3s out | 160 × 28 | `sessions-storyboard-1-request-out` |
| `sessions-storyboard-2-streaming-back` | its answer streaming back, 1.2k tokens of it | 160 × 28 | `sessions-storyboard-2-streaming-back` |
| `sessions-storyboard-3-refused` | work at its limit: d28c's request refused, its red pulse a quarter of the way back | 160 × 28 | `sessions-storyboard-3-refused` |
| `sessions-storyboard-4-repatched` | d28c moved to side, and sent again there, its pulse almost at side's jack | 160 × 28 | `sessions-storyboard-4-repatched` |

The four, six and eight accounts have nothing used of Fable's week, as their frames do. The
storyboard's accounts are `work`, `personal` and `side`, but side with c61b and db8a alone on it,
and in its last two frames, work at its session's limit, reached at 14:43, as the storyboard has
them; its frames are drawn at 14:43:02.

### `--theme`, and `NO_COLOR`

```bash
go run ./cmd/capturetool --fixture accounts-3 --theme amber
go run ./cmd/capturetool --fixture accounts-3 --theme path/to/lake.theme
NO_COLOR=1 go run ./cmd/capturetool --fixture accounts-3
```

A fixture is drawn in `nord`, the frames' theme, as the one theme chosen, on a terminal that says
its background is Nord's canvas. `--theme` names another: a built-in by its slug, or a `.theme`
file by its path, whatever it's named, as a theme written and not yet in the themes directory. It's
an input, never looked for in the themes directory, and one that doesn't load is an error, never a
theme drawn in its place. With `NO_COLOR` set, the fixture is drawn without colour, as the
dashboard is.

### `--print`

```bash
go run ./cmd/capturetool --fixture accounts-4 --print
go run ./cmd/capturetool --fixture accounts-4 --print --size 120x40
go run ./cmd/capturetool --fixture accounts-4 --print --ansi
```

`--print` draws the fixture once, with no terminal, after its keys are pressed, and prints it a line
for each of the terminal's rows: at the fixture's own size, or at `--size WxH`. It prints text alone,
its escape codes stripped and each line's trailing spaces trimmed, which diffs against a frame's
`.txt`; with `--ansi`, its colours too. The same fixture prints the same, byte for byte, in any time
zone.

```bash
diff <(go run ./cmd/capturetool --fixture accounts-4 --print) testdata/vhs/reference/accounts-4.txt
```

## Running a tape

From the repository's root, as the tapes' paths are from there:

```bash
vhs testdata/vhs/accounts-3.tape
```

It writes the PNG its `Screenshot` names. `accounts-3.tape` runs the fixture through `go run`, so the
first run waits on a build; it waits for the frame with `Wait+Screen` rather than sleeping a guess.
`calibrate-accounts-3-auto.tape` cats the frame's `.ansi` instead, for the calibration below.

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
  k = (R × 20 + 10) / (R × 20): 1.5% at 34 rows, 1.25% at 40, its cells 9.13 × 20.3 px at 34
  rows. No setting undoes it: the grid matches the frame cell for cell, a little larger. The spare
  width keeps the fit to the height, so the grid is R × 20 + 10 px tall at 46px down, under the bar,
  as the frames' grid is, and round(C × 9 × k) px wide, centred: at 160 × 34, 1461 × 690 at
  +22+46.

Through these settings, every terminal frame's `.ansi` renders what its `.png` shows, cell for
cell. What differs is how a glyph is drawn: Paper drew bold lighter in the frames it exported
last, took the storyboards' circled digits from a fallback font a little wider than a cell, which
shifts the rest of their captions, and drew braille and box drawing from the font, where `xterm`
draws its own. Over the whole grid, once both are in sRGB and the capture's is scaled to the
frame's, RMSE is 0.03 to 0.05 for half the frames, with 0.2% to 0.7% of pixels more than 10%
apart; the rest, with more bold, braille and box drawing, such as Sessions', come to 0.06 to 0.08
and 0.9% to 1.6%.

## Comparing a capture with its frame

By eye, the two side by side or one over the other:

```bash
magick testdata/vhs/reference/accounts-3-auto.png testdata/vhs/calibrate-accounts-3-auto.png -append side-by-side.png
```

With `magick compare`, on the terminal grid alone. The frames carry a Display P3 colour profile, so
take the frame to sRGB first, as a capture is, or every saturated colour reads as a difference;
crop the frame's grid; crop the capture's, as the calibrated settings place it, and scale it to the
frame's; then compare. At 160 × 34:

```bash
magick testdata/vhs/reference/accounts-3-auto.png -profile "/System/Library/ColorSync/Profiles/sRGB Profile.icc" \
  -crop 1440x680+24+46 +repage frame.png
magick testdata/vhs/accounts-3.png -crop 1461x690+22+46 +repage -resize '1440x680!' capture.png
magick compare -metric RMSE frame.png capture.png diff.png
magick compare -metric AE -fuzz 10% frame.png capture.png null:
```

For C × R, the frame's grid is (C × 9)x(R × 20)+24+46, and the capture's
(round(C × 9 × k))x(R × 20 + 10)+(16 + floor((C × 9 + 33 − w) / 2))+46, where w is that width.
`diff.png` marks what differs. The comparison is judged by eye in the end: anti-aliasing, bold and
the box-drawing glyphs always differ a little, and a frame's colours are its theme's tokens, not
what to read off a picture.

## Determinism

The same tape gives the same PNG, byte for byte, run after run. A fixture is fixed data, its clock
stands still, and its timers never fire: the harness takes the model's first read itself, 4s
before the moment drawn, then sets the clock to the moment, presses the fixture's keys, and only
then hands the model to Bubble Tea, which starts it reading nothing more. So there's no first
paint to race, and nothing on screen moves until a key does.

A key that shows Sessions, or flips a card, has the watch open the router's request stream: the
fixture's tells of what has befallen its requests, each at its time before the moment drawn, then
ends, as a router ends it. The watch keeps what it told, and asks for it again only once a timer
fires, which none does, so what travels the cords stands where the moment puts it: a pulse as far
along as the time since its request went says, a shimmer as many steps on as the time since its
answer's first byte. That's how a storyboard's frame is brought to its point in the animation.

## The capture tool and its fixtures

```
cmd/capturetool/       the tool: --fixture, --theme, --print, --size, --ansi, and the import guard
internal/capture/      imported by the tool alone
  capture.go           the Fixture, and the registry: Names, ByName, and every fixture's name, size, router and theme
  samples.go           the frames' sample accounts and sessions, and the sets of them the frames draw
  source.go            the fake router: its status document and health, GET /sessions, GET /history and GET /stream
  history.go           the accounts' windows' use over time, as the frames' charts draw it
  stream.go            what the request stream tells: the cards' backs' requests in flight, and the storyboard's moments
  harness.go           builds the watch model, its themes faked, settles it at the fixture's moment, and draws it
```

A fixture is a moment of the dashboard: what the router gives at it, the terminal it's drawn on,
and the keys pressed once its document is read. What the router gives mirrors the frames' sample
data, from the generator that drew them: the same accounts, in the same order, with the same
numbers, the same session ids, its document read 4s before the clock the frames read, and the
events the frames' RECENT has. Its sessions answer as `GET /sessions` would, and its history as `GET
/history` would, at any step, from a reading a minute of each session and a reading each half hour
of each week, shaped as the frames' charts are. The dashboard draws its own numbers from these, by
its own rules, so a few read otherwise than the generator drew them:

- **Work runs out** where its 90% reserve starts, as its reserve holds it back, and at its last
  half hour's rate its session reaches it at 15:45: the frames say it runs out at 16:05, which is
  when it reaches its limit. So COMING UP says `15:45 work reaches its reserve`, not `16:05 work
  runs out at its pace`, and RECENT, telling of it coming under pressure, says `its session
  reaches its reserve ~15:45 at its last-30-min rate`, not `its session runs out ~16:05 at its
  last-30-min rate`. Its card says `→ reaches its reserve ~15:45`, or in brief `→ out ~15:45`, and
  its chart's `✕` is there, on its reserve. Its lane in Runway has no room from 15:45, saying
  `reaches its reserve ~15:45 · back 17:10, as it resets`, so the strip doesn't rise to every
  account with room between personal's return at 15:54 and 16:05, as the frames' does.
- **ROOM LEFT's sums** round the rooms' sum as it is: the six accounts' sessions have 3.55
  accounts' worth left, which reads 3.6, where the generator's sum came to a hair under, and read
  3.5.
- **Projections** the generator gave without its numbers giving them: client's week, 78% used a
  third of the way through, reaches its 80% reserve at 16:04, which holds it back, rather than
  heading for 97%, so its card says `→ out ~16:04` and its chart's `✕` is just past now, and
  COMING UP, with four accounts or more, says `16:04  client's week reaches its reserve` as its
  third thing, where the frames have `16:40  client's session resets` or `16:20  spare is
  primed`; and its lane in Runway, over the day, has no room from 16:04, saying `week reaches
  its reserve ~16:04 · back Tue 09:00, as it resets`, where the frame has `room all day`, the
  strip counting an account fewer from then. At their pace, spare's week heads for 34% and
  extra's for 8%, not 12% and 5%, and with Fable's week used, client's heads for 63%, not 36%;
  and work's Fable week heads for 30.7%, which reads 31% but fills its bar an eighth of a cell
  short of the frames' 31%.
- **Countdowns** run from 14:42:07, the frames' from 14:42: as the dashboard counts now, personal
  is back in 1h 11m, not 1h 12m, and its big digits read `1:11`. They're `status`'s countdowns,
  so side's session resets `in 4h 7m`, not `in 4h 08m`.
- **A card's dots** come in the order the router lists its sessions, the one seen last first, so
  the busy ones lead: work's read `● ● ○`, where the frames have `● ○ ●`. Its back lists them in
  that order too, work's db8a, d28c and 7f3a, where the frames have d28c, 7f3a and db8a, so the
  flipped-selected fixture picks out d28c, the frames' session, with `↓` twice, on work's second
  row rather than its first. Sessions' calls, at the first look, run the one put on its account
  last first, which gives the frames' order: 7f3a was put on work at 13:10, between db8a at 13:05
  and d28c at 13:20. From then on each keeps its row, a new one joining its group's foot.
- **The cards' backs and Sessions draw different moments**, as their frames do: the backs' frames
  have four requests in flight, d28c's opus streaming on work, 1.2k tokens of it, db8a's sonnet
  waiting 38s there, c61b's opus streaming on side, 3.4k tokens, and db8a's opus waiting 12s; the
  Sessions frames have none, their cords at rest. So the flipped fixtures' router tells of those
  four as its request stream opens, and the Sessions fixtures' of none.
- **A line's pace** is the dashboard's, from the samples' times: side's week is 45.6% through,
  so its marker is a cell left of the frames', whose generator rounded it to 46%, and so is its
  Fable week's. In the storyboards, drawn at 14:43:02, personal's week runs out `~Fri 04:07`, its
  pace moving it on a minute a minute, where the frames have 04:06.
- **A card's back** draws its notes and LATELY from the router's events among the 50: the
  fixtures' tell, older than RECENT's four or counted in personal's limit, of db8a's opus and 41e0
  moving to side as personal reached its limit, d28c starting on work, and work's and personal's
  primes, which RECENT leaves out. Personal's limit counts three sessions moved, two of them told
  of, as the frames' RECENT and samples have it.
- **A chart's columns** each take the last reading before their middles, from the history at its
  steps, so a level steps an eighth of a cell apart from the frames' here and there; and its line
  for now stands just past the column now falls in, one column from the frames' on team's week.
- **Work's prime**, in the one account's RECENT, is told of at 12:10, when it started work's
  session: the frame has it at 13:02, saying it primed at 12:10.

### Adding a fixture

1. Add it to `fixtures` in `internal/capture/capture.go`: a name, the size of the frame it mirrors,
   the router of one of the sets of accounts in `samples.go`, or a new one, and the keys that reach
   the frame, such as `tab` for Sessions or `w` for the week. `Names` and `ByName` both read that
   one list.
2. Write its tape, modelled on `accounts-3.tape`, its `Width` and `Height` from the fixture's size,
   as the calibrated settings say.
3. Clear the tape and its capture at the milestone's sign-off. Leave the fixture.

What a later view reads joins the router in `source.go`, its data with the samples it's of, so
every fixture over those samples has it. The request stream is the exception, as it's a moment's:
what it tells is each fixture's own, `telling` the events, its data in `stream.go`.
