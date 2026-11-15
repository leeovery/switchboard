# `switchboard requests`

`requests` lists the requests the router has recorded, one line each: today's, unless `--since`
reaches further back. It reads the record from its files, so it works without the router.

```console
$ switchboard requests --since 10:00
Wed 7 Oct 2026, so far
  10:00:00  5b0e9d33  claude-opus-5-5  personal  200 canceled                    2.0s
  11:00:00  18bb2c41  claude-opus-5-5  work      200           185k in, 845 out  14.2s
  12:00:00  5b0e7c1a  claude-opus-5-5  work      200           100 in, 10 out    1.0s
```

```bash
switchboard requests [--session <id>] [--account <id>] [--since <when>] [--pool <id>] [--json | --pretty]
```

| Flag | Default | What it does |
|---|---|---|
| `--session <id>` | Every session | Keeps the requests of that session. Give its id, or as much of it as is unique. |
| `--account <id>` | Every account | Keeps the requests of that account, by its id in any case. A label is refused, and the refusal names the id. |
| `--since <when>` | The start of today | Starts at a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`). |
| `--pool <id>` | The only pool, else the unfenced one | Keeps the requests of that pool. See below. |
| `--json` | Off a terminal | Prints each line as the ledger holds it, with `"schema": 1` first: one JSON object a line. |
| `--pretty` | On a terminal | Prints the text, even off a terminal. |

What a request line records, and why, is in [Your record](../your-record.md).

## Each line

The requests are listed oldest first, under the day each arrived on.

| Column | What it shows |
|---|---|
| Time | When the request arrived. |
| Session | The session id, cut short. |
| Model | The model. |
| Account | The account, by its label. A removed account shows as `team (removed 4 Sep)`. |
| Status | The HTTP status, then a note if the request didn't finish. See below. |
| Tokens | The tokens in and out. |
| Duration | How long the request took. |

| Note | When |
|---|---|
| `canceled` | Claude Code went away before the answer ended. |
| `cut off` | The router cut the request off as it stopped. |
| `broken off` | Anthropic broke the answer off after it began. |

## Requests in a pool

Each request is kept under the pool it was routed in at the time. A request from before pools
counts as the first pool.

| If you pass | Then you get |
|---|---|
| `--pool <id>` | The requests of that pool, including a removed pool or one an import brought. |
| `--session` without `--pool` | The requests of the session in the pool it's in now, the pool of its newest request. |
| `--account` without `--pool` | The requests of the account in the pool it's in now. |

## Good to know

- The JSON names each account by its id, as the files keep it. Every field is in
  [Reference](../reference/ledger.md).
- Just after an upgrade that moves the history, `requests` refuses until the router has moved it.
  It says what holds the move up.
- Once you have a second pool, the text names the pool it read, such as `pool Acme`.

## Examples

```bash
switchboard requests --since 14:00
switchboard requests --session 18bb --json | jq '.usage'
```

## Related

- [`sessions`](sessions.md): the same requests, grouped by session.
- [`history`](history.md): each day summed up and priced.
- [Your record](../your-record.md): what the record holds, and what it never holds.
