# Loose ends in what the router keeps

A survey of what switchboard stores and holds, made on 5 October 2026 while designing the
dashboard's History tab, found three things that grow without bound. None is urgent: each grows
only when something rare happens.

- **Moves waiting on a limit that never comes.** The notifications gather the sessions a limit
  moves into one notice of it. A move heard before its limit waits in `limitNotices.early`, by the
  limit's identity, until the limit is heard (`internal/router/notices.go`). Two things leave it
  waiting until the router stops: the limit's event dropped from the notifications' full queue,
  of 256, and a newer limit of the same account heard first, after which `reached` returns before
  it deletes the older limit's waiting moves. A fix: when a limit is heard, drop the waiting moves
  of the account's older limits; or drop any that have waited longer than `gatherFor`.
- **launchd's log.** The LaunchAgent sends the router's standard output and error to
  `<state dir>/logs/launchd.log` (`internal/service/agent.go`, `service.go`). The router's own log
  rolls over at 10 MiB; this one is never trimmed. Under launchd, stderr isn't a terminal, so the
  router mirrors no records there, and what lands in it is what passes its log by, such as a
  panic's trace or a failed start's error. So it grows slowly, but for the life of the install. A
  fix: trim it as the router starts, or roll it over as the router's log rolls.
- **Corrupt files set aside.** A `state.json` or `prefs.json` that can't be read is renamed to
  `<name>.corrupt-<unix time>`, for someone to look at, and nothing removes it. A fix: keep the
  newest few of each, removing the rest as another is set aside.

## Found while building milestone 6, 6 October 2026

- **A restart can close the request stream before its last events are written.** The design
  promises a stream's readers see the requests a restart finishes: the stream ends as the control
  API closes, after the drain. But nothing waits for a reader's writer goroutine to flush the
  `done` events queued for it before the control API closes, so under load a reader can miss the
  last ones. `TestTheRequestStreamTellsOfTheRequestsARestartFinishes` failed once this way at a
  load average near 100, and passed 30 times alone. A fix: as the stream closes, write what's
  queued for each reader before ending its response.
- **A restart in place leaves its control socket unanswered as it finishes stopping.**
  `drainHandingOver` (`internal/router/replace.go`) closes the control API once the drain is done,
  and only then does `serve` stop its background: the notifications' finishing, up to
  `finishWait`, the history's and the ledger's last lines, and the state file's save; then come the
  `exec` and the new router's start. All the while, the control socket held to hand over takes
  connections nothing answers yet, so a `claude` started then can miss the half second `run` gives
  the router to answer, and run its whole session unrouted. It predates the ledger; the drain's
  wait for the requests it cuts off, moved inside the drain, narrowed it. A fix: on the handover
  path, stop the background before the control API closes.
- **Compressing a day's file holds the whole day in memory.** `compress`
  (`internal/dayfile/dayfile.go`) reads the day's plain lines whole, and its compressed file, whole
  and decoded, to tell whether it ends with them already, then compresses them as a gzip member held
  whole: about 50 MB at peak, for the request ledger, on a day of some 28,000 requests. A fix:
  stream the plain file into the gzip member, checking the compressed file's tail as it goes.
