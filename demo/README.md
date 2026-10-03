# The README's demos

Records the animated clips and stills in the project README, `art/*.webp`, by playing scripted
scenarios through the real dashboard: `cmd/capturetool --scenario`, the visual capture harness's
tool (see `testdata/vhs/README.md`), builds the dashboard's watch model through `watch.New`, as
`usage -w` does, with every seam faked. It never dials the router, probes, touches the network, or
reads or writes the real config, themes, state, prefs or tokens, so nothing needs a container, and
it's safe to record beside a live router.

```bash
demo/record.sh                # every tape: record it, then make its art
demo/record.sh routing help   # the tapes named
```

## What's recorded

| Tape | Scenario | Size | Art |
|---|---|---|---|
| `routing` | `routing` | 160 × 34 | `art/routing.webp`, `.mp4`: the README's lead, three accounts routed among |
| `usage` | `usage` | 160 × 27 | `art/usage.webp`, `.mp4`: one account, its charts and Runway |
| `themes` | `themes` | 160 × 34 | `art/themes.webp`, `.mp4`: the theme picker, previewing the built-ins |
| `flipped` | `routing` | 160 × 34 | `art/flipped.webp`, `.mp4`: side's card flipped to its sessions |
| `repatch` | `routing` | 160 × 34 | `art/repatch.webp`, `.mp4`: Sessions as work reaches its limit |
| `runway-week` | `routing` | 160 × 34 | `art/runway-week.webp`, a still: Runway over the week |
| `phone` | `routing` | 52 × 36 | `art/phone.webp`, a still: the phone layout |
| `help` | `routing` | 160 × 34 | `art/help.webp`, a still: `?`'s help |

## The scenarios

A scenario, in `internal/capture/scenarios.go`, is the router as it stands at the scenario's
start, and its cues: what the router does from then, a request at a time, each at its offset in
seconds from the start.

```go
asks(9.3, idD28C, opus, long),      // d28c's opus asks, wherever its session is; a long answer
starts(2.5, idB3E9, opus, long),    // a new session, sent where new sessions go
reaches(12, idDB8A, sonnet, long),  // the request that hits its account's session limit
```

The fake router carries each cue out by the real one's rules, but for scoring the accounts: the
best, where new sessions go, is the one the scenario names, and of the accounts a pin names, it
takes the best where the pin names it, else the first with room. It sends a request where its
session is, or where the session's own pin, a pin that moves running sessions, once, or an
account with no room sends it, the session moving with it; reads each answer's use off it for its
account; and tells its request stream of it all, `sent`, `first`, `progress`, `done`, `limited`,
`moved`, as the router's `GET /stream` does, a reader joining told first of the requests in
flight. Its status document, its sessions and its history stand at every read as the cues have
played up to then, the events joining RECENT and LOG as they come. Pins the keys give it, a digit
on a card's back, say, play in from when they're given.

The dashboard runs as it does live: its clock reads the scenario's start as it starts and runs on
in real time, and its timers fire, so bars ease, pulses travel the cords, sand falls and
highlights fade. A scenario plays the same way every time: its router does what its cues say,
when they say. Two things vary a little from one recording to the next: when the dashboard shows
a change to the router's document, as it looks at it every 5 seconds, as it does live, which can
put it a second either way; and the moment each key comes, by the milliseconds vhs takes to type
it. Each key pressed is shown at the bottom right of the screen for a moment, as a keycap, so a
reader sees what was typed: the capture tool draws them, not the dashboard.

The keys are the tape's, timed to the cues: a tape's comments say which. Change a cue's moment
and the tape's sleeps follow it. `go test ./internal/capture` plays every scenario to its end.

## How a tape becomes art

`record.sh` builds the capture tool into `demo/out/`, runs each tape through vhs, and hands its
frames to `finalize.sh`, which makes the art:

- **The frames, not a video.** vhs writes every frame it shows to `demo/out/<name>/`, as the
  terminal drew it. A video's compression shifts every pixel a little from frame to frame, which
  an animated WebP then stores whole; from the frames themselves it stores only what changed,
  losslessly. The lead clip, 34 seconds at 30 frames a second, is half a megabyte.
- **The art.** For a clip, an animated WebP, which GitHub shows inline, at the frames' 30 a
  second, and an H.264 MP4 of the same frames; for a still, a lossless WebP of the last frame.
  Each frame is set in a margin of its own canvas, as the theme picker changes it.
- **The size.** JetBrains Mono at 17px makes a cell 10 × 22 px, so the 160 × 34 clips are
  1648 × 796 with their margin: twice the 840 px the README shows them at, so a high-density
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
