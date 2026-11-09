# The weeks' forecast: what it left

The build of the weeks' forecast (see Forecasting run-outs in the design) ran #257, #259, #261 and
the replay tool's PR. The reviews of its slices accepted these: an improvement, or an edge past
ordinary use that fails safe. Gathered here for after the release.

## The readings history's unread lines

`readings.Reader.Between` counts the lines it can't read and drops the count, so a damaged line in
the readings history is passed over without a word, by the router's seed at startup and by
`cmd/forecasttool` alike. The request ledger's reader says how many it couldn't read; the readings
history's could too.

## The forecast's own edges

- **Use within a step.** The play steps every 10 minutes: use that falls after a week's reset
  within a step is lost, and an account chosen second in a step can run out up to a step early.
- **A refusal held all week.** A token refused as the document is built is played as refused for
  the whole play, though the router bars it for minutes. While a bad token keeps being refused on
  and off, the other accounts' run-outs can shift between two times every ten minutes or so.
- **A limit isn't a refusal.** An account a limit holds back still takes requests in the play, as
  only a refused or missing token keeps it out.
- **A week reset since it was read** shows no run-out of its own until it's read again, though the
  play has one for it; its fresh room shapes the others' run-outs meanwhile.
- **A day alone six days on.** A run-out six to seven days ahead, on today's weekday, reads as
  today's day, `out ~Thu` on a Thursday; only COMING UP's countdown beside it settles which.
- **Accounts sharing an id** in a hand-edited config can have use placed on the first of them.
- **The forecast's cost.** A play allocates about 5,000 times, nearly all in `score.Policy.Choose`,
  called each 10-minute step across the week; it costs about 1.5 ms for four accounts.

## The router's rates across a restart

- **A long outage.** After the router has been down more than two days, the first reading of a
  week lands the whole gap's rise at once in the day's rate, inflating it for a day.
- **The pin's history.** Only the global pin as it stands is kept, with when it was set: a pin
  cleared within the two days the seed replays, or set again on the same accounts, can't be
  recovered, so the history before it is judged without it.
- **Limits and refusals** aren't kept across a restart, so the seed judges the pool's room without
  them, as routing's own state does.
- **A pin naming an account never read** gives the pool no room in the day's rate, until it's read.

## The hold curve

- **An answer ending after its session moved** counts as none of its time on the account.
- **Weights edited by hand** in `state.json` to below zero are taken as they are.

## The replay

What `cmd/forecasttool`'s playback of a state directory can't tell from what's recorded:

- **When each attempt went out.** The ledger holds the accounts a request was tried on, in order,
  but not when, so the playback moves a session to each as its request arrived, where the router
  moves it as each attempt goes out, seconds later, or a throttle's wait.
- **Requests of a session that overlap.** The playback takes each line whole as its request
  arrived, where the router can have one request of a session move it between another's attempts,
  or find it moved meanwhile and leave the newer assignment standing.
- **The pool's room** is judged only as a changed reading is taken in and as a document is built
  on the hour, as the router's seed judges it only at the readings it replays, where the router
  judges it at every reading, changed or not, and every status document it builds.
- **When an account was last read.** The readings history keeps a reading only when it changed, so
  an account's `FetchedAt` in the playback is its last change, not its last read.
- **A reset the provider moves mid-week.** A week is known by its reset, so the forecasts made
  before the move are scored under the old reset, as a week of its own.

## Smaller

- `accountStatus` builds the pool's demand on each statusline's call and throws it away.
- The status document's accounts and its `demand` take the router's lock separately, and a pin's
  `Since` is read outside it.
- capture's `documents.go` takes its worlds' rates up by a copy of the router's rules (`takenUp`):
  its worlds cut each account's history at its own stale time and count the pool as having room all
  day, so it can't go through the router's playback simply.
