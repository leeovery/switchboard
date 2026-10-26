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

## Found in the request ledger's third review, 7 October 2026

- **A crash mid-compress, then a line appended,** has the next `compress` add the plain lines again.
- **`requests` decodes the day before its span** for lines filed under it, then drops them: CPU only.

## Found while building milestone 7's stage 1, 8 October 2026

- **`TestAnAssignmentUsedAgainIsSavedOnceAMinute` failed once** at a load average near 10: the
  state file was written once more than it wants while the session was used again. It passed 48
  times since on the stage, and 40 times on main, so no change of the stage's is to blame. The
  test's timers fall on the same instants of synctest's clock, the save a second after a change
  and the session used every second, and the order goroutines woken at one instant run in isn't
  fixed. A fix: move the uses off the save's instants, or wait for the save before each.
- **`GET /sessions/{id}` groups every session to report one.** `sessions.session` calls
  `s.entries()`, which groups and sorts the assignments of every session in the last hour, to
  report the one asked for, and statuslines ask for it often. A fix: gather that one session's
  entries alone, sorted by `byUse`, as `lastUsed` finds its newest.

## Built in the cleanliness release, 26 October 2026

The owner's choice on 26 October 2026; its PR to come. The control socket's gap is taken up beside
them, on the router's restart path; `requests` decoding the day before its span, the flaky save
test and `GET /sessions/{id}` stay open.

- **launchd's log:** as the service starts the router, one past 10 MiB is cut in place to its newest
  5 MiB, from the start of a line, and the router's output and errors moved to its new end, so
  launchd's writes still land in it and a start that fails writes why after what's kept; a router
  run by hand, its output elsewhere, leaves it alone (`logs.TrimOutput`, called by `serve`).
- **Corrupt files set aside:** the newest 3 of each name are kept, the older removed as another is
  set aside, by one rule the state file and the preferences use (`atomicfile.SetAside`).
- **Moves waiting on a limit that never comes:** a limit's moves heard before it wait 5 seconds,
  `gatherFor`, at most, which bounds both ways a limit is never heard of: its event dropped, and a
  newer limit of its account heard first (`limitNotices.forget`).
- **Compressing a day's file:** it streams the plain file into its gzip member, and checks the
  compressed file's lines against it a member at a time, holding neither whole: some 1.4 MB,
  whatever the day's size. It writes the bytes v0.2.8's did, as far as the tests show, and its files
  read as they did. The crash mid-compress fell out of the same change: where a line was appended
  after a compress stopped before it removed the plain file, the lines the compressed file's last
  members give, from the earliest member on whose lines the plain file starts with, aren't added
  again, however many times that happened in a row (`dayfile/compress.go`); the earliest, as a
  later member can start it too where its lines are a line cut short to a bare start of one, such
  as `{"n":`, which a crash or a full disk leaves. A read of the day in that window still gives
  those lines twice, until the next compress, within the hour. Accepted: a fresh plain file for a
  day compressed before, as after the clock was set back, whose first lines are byte for byte the
  last members', is taken as a stopped compress's, and those lines are counted once. The files
  can't tell the two apart, and no line repeats another but for a line cut short: every writer of
  a day's file, the ledger, the readings history and the events, puts its time in each line, as
  the clock gives it, or to the millisecond in the ledger, beside a request's id, an account and
  window, or the run and a rising id.

Left, as rare as it is harmless: a member of a line cut short alone, to a bare start of one such as
`{"n":`, last in a day's compressed file, as a crash mid-write after a compress stopped can leave
(the compress tests' sequences found one), and then a fresh plain file for that day, from a late
write or a clock set back, has the line cut short taken as the start of the new file's first line.
A read gives that line whole, and the line cut short drops out of the count of those unread: no line
that reads is lost. It needs both a crash mid-write and a past day written to again.
