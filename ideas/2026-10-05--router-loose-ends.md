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
