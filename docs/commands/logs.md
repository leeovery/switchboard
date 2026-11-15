# `switchboard logs`

`logs` shows the last lines of a log, so you can work out why a session went to an account or why a
request failed. The router keeps one log, and every other command shares the second.

```console
$ switchboard logs -n 1
time=2026-09-28T14:12:00.123+01:00 level=INFO msg=routed component=router pid=4242 id=5f3a9c2e session=18bb978f model=claude-opus-5-5 account=q4tx reason="moved: b6s7 hit its limit" status=200 attempts=2 duration=9.412s
```

```bash
switchboard logs [router|cli] [-n <lines>] [-f] [--path]
```

| Argument or flag | Default | What it does |
|---|---|---|
| `router` or `cli` | `router`, once the router has logged | Picks the log. Before the router logs, the default is `cli`. |
| `-n`, `--lines <n>` | `50` | Shows the last `n` lines. If the log is shorter, it reaches into the rolled-over file. |
| `-f`, `--follow` | Off | Goes on printing lines as they're logged, across rotations, until you press ctrl+c. |
| `--path` | Off | Prints the path of the log, and nothing else. |

`SWITCHBOARD_LOG_LEVEL` sets how much is logged: `debug`, `info`, `warn` or `error`. The default is
`info`. `logs -f` exits `0` when you interrupt it.

Where the logs live, how they roll over, and what they never hold are in
[Troubleshooting](../troubleshooting.md).

## Examples

```bash
switchboard logs -f                # follow the router: a line a request, with its account and why
switchboard logs cli -n 200
switchboard logs router --path
```

## Related

- [`events`](events.md): what the router decided, in plain words.
- [`serve`](serve.md): the router, and its `--log-level`.
