# `switchboard status`: where every account stands

`status` shows where every account stands right now: each window, what holds an account back, and
which account is best next. On a terminal it prints text. In a pipe or an agent shell it prints the
status document as JSON.

```console
$ switchboard status
b6s7 · work (primary)
  Week    40%  resets in 5d 11h · Sun 02:10 · runs out ~Wed
  5-hour  23%  resets in 2h 58m · Mon 17:10 · on pace for 57%
  2 sessions

h8rc · personal
  Week    10%  resets in 5d 11h · Sun 02:10 · on pace for 47%
  5-hour  no 5-hour window
  1 session

best next: b6s7 · work (primary)
from the router: healthy  ·  3 sessions  ·  routing automatically
```

```bash
switchboard status [--json | --pretty] \
  [--pool <id>] [--probe] [-r]
switchboard status --session <id> \
  [--json | --pretty] [--pool <id>]
```

| Flag | Default | What it does |
|---|---|---|
| `--json` | Off a terminal | Prints the status document as JSON, even on a terminal. |
| `--pretty` | On a terminal | Prints the text, even off a terminal. |
| `--pool <id>` | The only pool, else the unfenced one | Shows the accounts of that pool. |
| `--probe` | Off | Probes every account, even while the router runs. |
| `-r`, `--refresh` | Off | Has the router read every account it can first, and waits up to ten seconds for it. |
| `--session <id>` | None | Prints the label of the account a session is on. See below. |

Once you have a second pool, the text names the pool it shows first. For what the windows, pace
and reserves mean, see [Limits and routing](../limits-and-routing.md).

## Where the numbers come from

`status` asks the router while it runs. Without the router, it probes each account itself. The last
line says which it did, and how healthy the router is.

| If | Then |
|---|---|
| The router runs | `status` reads the router. |
| The router is down, or you pass `--probe` | `status` probes each account. `--refresh` changes nothing. |
| The `claude` your shell runs isn't switchboard | The first line says so. Sessions started there aren't routed. |
| The config can't be read, and the router answers | `status` prints the router's document, then fails with the config error. |
| The config can't be read, and the router is down | `status` fails with the config error only. |

A config can't be read, for example, if an upgrade brings rules that refuse it. The router then
holds its restart into the new version until you fix the config.

## What the text shows

The text runs in this order:

1. Each window of each account: its use, its reset, and where it's heading.
2. What holds an account back, such as a limit or its reserve.
3. Whether an account is under pressure, and the rate that judges it.
4. The sessions routed in the last hour, a line each, with the account each model goes to.
5. The priming schedule.
6. The best account to use next.
7. Where the usage came from, and how healthy the router is.

A week shows the router's forecast, such as `on pace for 92%` or `runs out ~Tue`. A probed
week, and a week that one model has of its own, show their pace since the week began. A 5-hour window shows
its rate over the last 30 minutes, such as `runs out ~17:25 at its rate over the last 30 min`. A
run-out within a day shows a time, and one further ahead shows a day.

A verified Ultra account has no 5-hour window, and its line reads `no 5-hour window`. An Ultra
account not yet verified keeps its 5-hour window.

Under the last line, `status` names anything the router waits on:

- a restart that's due, and how to have it now;
- an export under way, which a restart waits for;
- the move of your record onto account ids, while the router still owes it.

## What the JSON holds

The JSON is what Claude and scripts read. It holds:

- every window of every account, with its even pace and its allowance an hour and a day;
- extra usage, if the API reports it;
- the pool: the room of the pool's accounts added up, and their mean use;
- each account's plan, and the rates the week forecasts go by;
- what's coming up, soonest first: limits lifting, windows running out or resetting, and primes;
- how the hourly export stands, if `[export]` sets a folder.

Every field is in [Reference](../reference/status-json.md).

## One session: `--session`

```console
$ switchboard status \
    --session "$CLAUDE_CODE_SESSION_ID"
work
```

`--session` prints the label of the account the session is on, on a terminal or off one. It's the
account that the last model the session used went to. With `--json`, it prints every model the
session used, and why each went where it did. Its fields are in
[Reference](../reference/status-session-json.md).

| If | Then |
|---|---|
| The router isn't running, or hasn't seen the session | It exits `1`, so a statusline can tell. |
| The account was removed since | It names the old label, and when the account was removed. |
| Nothing names the account | It prints the account id. |
| You pass `--probe` or `--refresh` | It refuses: `--session` only asks the router. |
| The router started before pools | It reads the session by its id, as before, with `--pool` or without. |
| The router started before pools, and you pass `--json` with `--pool` | It refuses, and says to restart the router. |

Name the session by its id, or by as much of it as is unique.

## Statuslines in a config with pools

In a config with pools, a statusline passes the session's pool:

```bash
switchboard status \
  --session "$CLAUDE_CODE_SESSION_ID" \
  --pool "$SWITCHBOARD_POOL"
```

A session resumed in another pool runs in both pools while the first process lives. Asked by its
id alone, the session reads as the pool its latest request came from. So each process's statusline
would show the other's account in turn. `--pool` costs the label no health check, so a statusline can ask at
every refresh.

## Exit status

`status` exits `0` once it has printed. It exits `1` if the config can't be read, even after it
prints the router's document.

## Examples

```bash
switchboard status
switchboard status --json \
  | jq '.accounts[] | {id, windows}'
# read every account first, for example
# after a reset made by hand
switchboard status --json -r
```

## Related

- [Claude and switchboard](../claude-and-switchboard.md): how Claude reads `status` for you.
- [Limits and routing](../limits-and-routing.md): windows, reserves and pressure.
- [`usage`](usage.md): the windows alone, as a table.
