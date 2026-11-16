# Troubleshooting: the logs

The logs tell you, after the fact, why something happened:

- why a session went to an account;
- why a request failed;
- why the dashboard showed what it did.

```bash
# Follow the router's log
switchboard logs -f
# The last 200 lines of the log that every
# other command shares
switchboard logs cli -n 200
# Print where the log is
switchboard logs --path
```

The router notes every request it routes, with the account, the pool if you have pools, and why it
went there. Each record is one line of logfmt:

```text
time=2026-09-28T14:12:00.123+01:00 level=INFO msg=routed component=router pid=4242 id=5f3a9c2e session=18bb978f model=claude-opus-5-5 account=q4tx pool=h4w2 reason="moved: b6s7 hit its limit" status=200 attempts=2 duration=9.412s
```

## Where the logs are

They're in the `logs/` folder of the state directory.

| File | What writes it |
|---|---|
| `router.log` | The router. |
| `cli.log` | Every other command. |
| `launchd.log` | launchd, with the service's output, such as a crash. |

## How much is logged

`SWITCHBOARD_LOG_LEVEL` sets the level: `debug`, `info`, `warn` or `error`. The level is `info`
unless you set it.

| To set the level for | Use |
|---|---|
| Every command | `SWITCHBOARD_LOG_LEVEL=debug` |
| A router you run by hand | `switchboard serve --log-level debug` |
| The router the service runs | `switchboard service install --log-level debug` |

## How the logs roll over

`router.log` and `cli.log` roll over before they pass 10 MiB, and each keeps five old files.
launchd writes `launchd.log`, so switchboard can't roll it over. Once the file passes 10 MiB, the
service cuts it to the newest 5 MiB as it starts the router.

## What the logs leave out

- No log holds a token or an account's label. Accounts appear by id, and anything shaped like
  a token is replaced with `[redacted]`.
- No command logs to the terminal, except `serve` run at one. So `--json` and the dashboard stay
  clean.
- No command fails because it couldn't log.

## Related

- [`switchboard logs`](../commands/logs.md): every flag of `logs`.
- [The router](router.md): why a session went to an account.
- [Where the files are](../reference/files.md): the state directory, and what else is in it.
