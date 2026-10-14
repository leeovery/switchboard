# The README's demos

Records the animated clips and stills in the project README, `art/*.webp`, by playing scripted
scenarios through the real dashboard: `cmd/capturetool --scenario`, the visual capture harness's
tool (see `testdata/vhs/README.md`), builds the dashboard's watch model through `watch.New`, as
`switchboard dashboard` does, with every seam faked. It never dials the router, probes, touches the
network, or reads or writes the real config, themes, state, prefs or tokens, so nothing needs a
container, and it's safe to record beside a live router.

```bash
demo/record.sh                # every tape: record it, then make its art
demo/record.sh routing help   # the tapes named
```

## What's recorded

| Tape | Scenario | Size | Art |
|---|---|---|---|
| `routing` | `routing` | 160 × 37 | `art/routing.webp`, `.mp4`: the README's lead, the Overview, then the routing card pinning new and running sessions to side |
| `dashboard` | `dashboard` | 160 × 37 | `art/dashboard.webp`, `.mp4`: one account's Overview, then Runway |
| `themes` | `routing` | 160 × 37 | `art/themes.webp`, `.mp4`: the theme picker, previewing the built-ins over the Overview |
| `repatch` | `repatch` | 160 × 37 | `art/repatch.webp`, `.mp4`: Sessions as personal reaches its limit |
| `runway-week` | `routing` | 160 × 37 | `art/runway-week.webp`, a still: Runway over the week |
| `phone` | `routing` | 52 × 36 | `art/phone.webp`, a still: the Overview at 52 × 36 |
| `help` | `routing` | 160 × 37 | `art/help.webp`, a still: the Overview's help |

## The scenarios

A scenario, in `internal/capture/scenarios.go`, is a world the signed-off frames draw, as the
router stands at the scenario's start, and its cues: what the router does from then, a request at
a time, each at its offset in seconds from the start. The world has the accounts, a year of
readings, today's requests, the router's events, the sessions routed with their directories and
the requests in flight, so a demo draws what the frames do. There are three: `routing`, the auto
world from 14:42:07, three accounts routed among; `dashboard`, work alone; and `repatch`, the
limit's world from 16:05:26, personal about to reach its limit. The themes tape and the stills
play `routing`. The world is built before the clock starts, so a cue at 2.5 seconds plays 2.5
seconds after the tape's first frame. A cue names its session and model, and how its answer comes:
`brief`, `steady`, `long` or `lengthy`, from a short reply soon to a long one streaming a while.

```go
starts(2.5, idB3E9, opus, "~/Code/cli", long), // routing: a new session in ~/Code/cli, sent where new sessions go
asks(17, id41E0, sonnet, steady),              // routing: 41e0's next request: with the pin on side, it goes there
reaches(14.5, id41E0, opus, steady),           // repatch: the request that hits personal's 5-hour limit
```

The fake router carries each cue out by the real one's rules, but for scoring the accounts: the
best, where new sessions go, is the one the world names, and of the accounts a pin names, it
takes the best where the pin names it, else the first with room. It sends a request where its
session is, or where the session's own pin, a pin that moves running sessions, once, or an
account with no room sends it, the session moving with it; reads each answer's use off it for its
account, as a reading of each of its windows; and tells its request stream of it all, `sent`,
`first`, `progress`, `done`, `limited`, `moved`, as the router's `GET /stream` does, a reader
joining told first of the requests in flight. Its status document, its sessions and its readings
stand at every read as the cues have played up to then, the events joining LATELY as they come.
Pins the keys give it, a digit and `m` on the routing card, say, play in from when they're given,
and tell the router's `pin` and `auto` events, as the router does for a pin set from the dashboard.

A scenario's own requests write no ledger line and no line of the events file: they reach the
dashboard through the request stream, the readings and the document's events. The world's lines
stand as they were, and the Log and a session's page, which read lines, are in no demo.

The dashboard runs as it does live: its clock reads the scenario's start as it starts and runs on
in real time, and its timers fire, so bars ease, pulses travel the cords and highlights fade. A
scenario plays the same way every time: its router does what its cues say, when they say. Two
things vary a little from one recording to the next: when the dashboard shows a change to the
router's document, as it looks at it every 5 seconds, as it does live, which can put it a second
either way; and the moment each key comes, by the milliseconds vhs takes to type it. Each key
pressed is shown at the bottom right of the screen for a moment, as a keycap, so a reader sees
what was typed: the capture tool draws them, not the dashboard.

The keys are the tape's, timed to the cues: a tape's comments say which, and each scenario's cues
say which keys they're timed against. Change a cue's moment and the tape's sleeps follow it.
`go test ./internal/capture` plays every scenario at its tape's size, with its tape's keys at
their times.

## How a tape becomes art

`record.sh` builds the capture tool into `demo/out/`, runs each tape through vhs, and hands its
frames to `finalize.sh`, which makes the art:

- **The frames, not a video.** vhs writes every frame it shows to `demo/out/<name>/`, as the
  terminal drew it. A video's compression shifts every pixel a little from frame to frame, which
  an animated WebP then stores whole; from the frames themselves it stores only what changed,
  losslessly. The lead clip, about 26 seconds at 30 frames a second, is half a megabyte.
- **The art.** For a clip, an animated WebP, which GitHub shows inline, at the frames' 30 a
  second, and an H.264 MP4 of the same frames; for a still, a lossless WebP of the last frame.
  Each frame is set in a margin of its own canvas, as the theme picker changes it.
- **The size.** JetBrains Mono at 17px makes a cell 10 × 22 px, so the 160 × 37 clips are
  1648 × 862 with their margin: twice the 840 px the README shows them at, so a high-density
  screen draws them sharp.
- **The colours** are the themes' own: nothing is graded. Beside the signed-off frames in
  `testdata/vhs/reference/`, which carry a Display P3 profile, as a wide-gamut screen shows a
  terminal's colours, they don't read flat.

`demo/out/` is remade each time and never committed: `art/` is.

## Setting up

```bash
brew install vhs ffmpeg webp
ls ~/Library/Fonts /Library/Fonts | grep JetBrainsMono-Regular
```

vhs records a tape's frames into a directory of its own under `$TMPDIR`, then moves it to the
tape's `Output`. Where `$TMPDIR` is on another volume than the repository, the move fails without
a word, and no frames come: `record.sh` gives vhs a `TMPDIR` in `demo/out` for that.

vhs drives a headless browser that connects to `ttyd` over loopback, which an agent's sandbox
refuses: run `record.sh` with the sandbox off, as `testdata/vhs/README.md` says.
