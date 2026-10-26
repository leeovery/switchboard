# Troubleshooting

This page shows where to look when switchboard doesn't do what you expect, and how to read its logs.

## Where to start

- **Where things stand:** to see how each account stands and why, the router's health, and any
  restart the router has due, run [`switchboard status`](commands.md#status). To see whether the
  LaunchAgent is installed and loaded, run [`switchboard service status`](commands.md#service).
- **Why a request went where it did:** to follow the router's log, run `switchboard logs -f`. The log
  has a line for each request, with its account and why it went there. To look back over the days,
  use [`requests`](commands.md#requests) and [`events`](commands.md#events).
- **A limit reset by hand:** to have the router read the accounts now, run `switchboard usage -r` or
  `status -r`, or press `R` on the dashboard.
- **Not routed:** some of what Claude Code sends never reaches the router. See
  [What doesn't go through the router](how-it-works.md#what-doesnt-go-through-the-router).
- **The ledger's digests:** the `ledger/digest-<date>.json.gz`, `ledger/parts-<date>.gz`,
  `ledger/logday-<date>.json.gz` and `ledger/rows-<date>.gz` files are safe to delete. They're
  worked out from the day's lines, and the router writes them again on its next round, or when it
  next starts. Until then, the dashboard reads the lines instead, which is slower but shows the
  same.
- **A session's page slow to open:** a damaged parts file whose size hasn't changed isn't noticed
  until a page reads it. The page then rests on an older digest, or reads the session's days whole,
  at every open, until the router writes the day again, as after a release that changes the
  digests' form. To have it written now, delete that day's `digest-<date>.json.gz` and
  `parts-<date>.gz`: the router writes both again on its next round.
- **The Log's find slow to go back:** the find reads each past day's head, and a day's lines only
  where its head isn't written yet. After the upgrade that brings heads in, the router writes one
  for every day kept, yesterday's first, 1 to 1.5 s of work a day with its rows, once it has written
  every digest due, which on a long ledger takes some minutes; until it reaches a day, the find
  reads that day's lines, half a second or more. Yesterday's head is written about an hour after
  midnight, just after its digest. A head damaged where its size hasn't changed is never written
  again on its own, and the find reads that day's lines at every walk: delete that day's
  `logday-<date>.json.gz`, and the router writes it again on its next round.
- **The Log slow to go back a day, or a turn slow to open:** the Log reads each past day from its
  rows, and a turn its own past days, all but yesterday until about an hour after midnight, which
  read the day's lines, as today's do. A day whose rows file or head is deleted, or damaged so its
  size changed, is safe: that day reads its lines instead, half a second or more, never showing
  otherwise, until the router writes both again on its next round. After the upgrade that brings
  rows in, the router writes them for every day kept, yesterday first, as it writes heads. A rows
  file damaged where its size hasn't changed isn't noticed by the router, and that day reads its
  lines at every day back: delete that day's `rows-<date>.gz`, and the router writes it again on its
  next round.
- **`claude` won't start, the router down:** with the router not running, or not healthy, `claude`
  starts nothing and says so. Run [`switchboard service status`](commands.md#service) to see
  whether the LaunchAgent is installed and loaded and how the router answers, then
  `switchboard service install` or `switchboard service restart`. To start Claude Code without the
  router this once, unrouted and unrecorded, run `SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude`; to
  have it do so always, set [`when_router_down = "direct"`](configuration.md#top-level-keys).
- **Getting round switchboard:** to start Claude Code on its own login, run
  `switchboard run --direct`, or run the real `claude` by its path. If the switchboard binary itself
  breaks, every `claude` breaks with it, until you take switchboard's directory off `PATH`.

## The logs

The logs are for working out, after the fact, why a session went to an account, why a request
failed, or why the dashboard showed what it did. They're in `<state dir>/logs/`:

| File | What writes it |
|---|---|
| `router.log` | The router. |
| `cli.log` | Every other command. |
| `launchd.log` | launchd, with the service's own output, such as a crash's. |

Each record is a line of logfmt. The router notes every request it routes, with its account and why
it went there:

```
time=2026-09-28T14:12:00.123+01:00 level=INFO msg=routed component=router pid=4242 id=5f3a9c2e session=18bb978f model=claude-opus-5-5 account=side reason="moved: work hit its limit" status=200 attempts=2 duration=9.412s
```

- **Level:** `SWITCHBOARD_LOG_LEVEL` is `debug`, `info`, `warn` or `error`, and is `info` unless
  set. For the router, `serve --log-level` overrides it, and so does `service install --log-level`,
  which passes it on.
- **Rotation:** `router.log` and `cli.log` roll over before they'd pass 10 MiB, and each keeps five
  old files. Nothing rolls `launchd.log`, which launchd writes.
- **Redaction:** nothing logs a token or an account's label. Accounts appear by id, and anything
  shaped like a token is replaced with `[redacted]`.
- **Never in the way:** no command logs to stdout or stderr, except `serve` at a terminal. So
  `--json` and the dashboard stay clean. No command fails because it couldn't log.

To print and follow the logs, use [`switchboard logs`](commands.md#logs).
