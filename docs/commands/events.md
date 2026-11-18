# `switchboard events`: what the router decided, and when

`events` lists what the router decided and noticed, such as a session moved or an account at its
limit: today's, unless `--since` reaches further back. It reads the files the router keeps, so it
works without the router and looks back across restarts.

```console
$ switchboard events --since 1d
Tue 6 Oct 2026
  Tue 22:00            limit     work              its 5-hour window, till 23:58: its session moves to personal
  Tue 22:05  5b0e7c1a  moved     work → personal   work reached its limit

Wed 7 Oct 2026, so far
  09:00      18bb2c41  started   work
  10:00                pressure  work              its 5-hour to run out at 12:30
  11:00                pin       work, personal    new sessions go there, set from the command line
  12:00                restart                     due: the config changed
```

```bash
switchboard events [--since <when>] [-f] \
  [--pool <id>] [--json | --pretty]
```

| Flag | Default | What it does |
|---|---|---|
| `--since <when>` | The start of today | Starts at a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`). |
| `-f`, `--follow` | Off | Goes on printing each event the router files, until you press ctrl+c. A changed event prints again, whole. |
| `--pool <id>` | The only pool, else the unfenced one | Lists only that pool. See below. |
| `--json` | Off a terminal | Prints each event as the router files it, with `"schema": 1` first: one JSON object a line. |
| `--pretty` | On a terminal | Prints the text, even off a terminal. |

## What it lists

The events cover:

- sessions started and moved;
- accounts under pressure, at a cap or a limit, refused, primed, and open again;
- accounts on a [hold](../glossary.md) because Anthropic refused their usage read;
- accounts found holding another subscription's token;
- pins set and cleared;
- restarts falling due, or put off;
- the router's health.

They're listed oldest first, under the day each happened on. Each event shows once, as it last
stood.

| Column | What it shows |
|---|---|
| Time | When it happened. |
| Session | The session id, cut short, if the event is about one. |
| Kind | The kind of event, such as `moved` or `limit`. |
| Account | The account, by its label. |
| What happened | What happened, in words. |

In the JSON, an event that changed is printed again, and its last line is how it stands. Each line
carries its `run` and `id`.

In the text, a hold says when it lifted, even across a restart. In the JSON, a `held` event stands
as the router filed it. Pair it with its lift by the rule in the
[events reference](../reference/events.md).

## Events in a pool

| Event | Listed under |
|---|---|
| A session started or moved, or a pin | The pool it was filed in. An event from before pools counts as the first pool. |
| Something that befell an account | The pool the account is in now. A removed account goes by the pool the accounts record keeps for it. |
| The router itself, such as a restart | Every pool. |

An event in a pool you've since removed shows only under that pool, read with `--pool` and its id.
So does an event from another Mac that an import brought.

## Good to know

- `events -f` exits `0` when you interrupt it.
- Every kind of event, and its fields, is in [Reference](../reference/events.md).
- Just after an upgrade that moves your record, `events` refuses until the router has moved it.

## Examples

```bash
switchboard events --since 2d
switchboard events -f --json \
  | jq 'select(.kind == "limit")'
```

## Related

- [`logs`](logs.md): every request and decision, in full.
- [Your record](../your-record.md): what the router keeps, and for how long.
- [Notifications](../notifications.md): the events that reach you as they happen.
