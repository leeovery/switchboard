# `switchboard sessions`

`sessions` lists the sessions of today, with what each did and what it was worth. Give it a
session, and it shows the page of that session: each turn, and each account it ran on.

```console
$ switchboard sessions
   session              on        model     state        started          requests  worth   routing
▸  5e6f7a8b ~/Code/cli  personal  opus-5-5  ended 11:00  10:15            2         $0.08   from work, at its cap  $0.01
   1a2b3c4d ~/Code/api  work      opus-5-5  ended 09:00  yesterday 22:00  1         $0.04
   9c0d1e2f ~/Code/web  work      opus-9    ended 08:30  08:00            2         $0.04+

today  3 sessions  ·  5 requests  ·  $0.15+ at API prices
```

```bash
switchboard sessions [<id>] [--pool <id>] [--json | --pretty]
```

| Argument or flag | Default | What it does |
|---|---|---|
| `<id>` | None | Shows the page of that session. See below. |
| `--pool <id>` | The only pool, else the unfenced one | Lists the sessions of that pool. See below. |
| `--json` | Off a terminal | Prints the sessions, or the page, as JSON, even on a terminal. |
| `--pretty` | On a terminal | Prints the text, even off a terminal. |

For how sessions are routed and moved, see [Limits and routing](../limits-and-routing.md).

## The list

Sessions still running come first, the oldest first. Then come the sessions that ended today, the
latest first. A last line adds them up.

| Column | What it shows |
|---|---|
| Session | The session id, cut short, and its folder. |
| On | The account the session is on. |
| Model | The model of the session. |
| State | What it's doing: asking, answering, idle, or when it ended. |
| Started | When it started. |
| Requests | Its requests today. |
| Worth | What they'd have cost through the API. A `+` means some were unpriced. |
| Routing | How it came to its account, such as `from work, at its cap`, and what writing its context again there cost. |

`sessions` reads the record from its files, and asks the router which sessions are running. If the
router is down, it lists the sessions from the record only, none as running, and says so.

## A session page

```bash
switchboard sessions 5b0e
```

Name the session by its id, or by as much of it as is unique among the sessions of today. A
session from an earlier day needs its whole id.

The page shows four parts:

| Part | What it shows |
|---|---|
| The session | What it's doing, where it runs and why, when it started, and when it came back after an hour or more idle. |
| Each account | Its time there, its requests, the points of each window they took, and their worth. |
| Turns | Newest first. A turn is a prompt and every request that serves it: subagents, compactions and side requests too. |
| Totals | The session as a whole. |

Each turn shows when it started and how long it took, its requests, and the tools called most.
It also shows the tokens read from the cache, written to it and put out, and the worth. Last
come the accounts the conversation went to, such as `work → personal` if it moved. A subagent on
another account isn't a move.

The points are an estimate. A window counts every session of its account. So each rise in the
window between two answers is shared out among the requests that ended between them, by their
tokens.

Without the router, the page comes from the record only, and the session shows as not running. It
shows as idle if its last request came within the hour, and as ended if not.

## Sessions in a pool

Each session is listed under the pool its requests were routed in. A session from before pools
counts as the first pool.

| If you pass | Then you get |
|---|---|
| `--pool <id>` | The sessions of that pool, including a removed pool or one an import brought. |
| `<id>` without `--pool` | The session, found in whatever pool it's in. The page is its work in the pool of its newest request. |
| `<id>` with `--pool` | The session in that pool only. The page is its work there. |

A session resumed in another pool can have work in both. Once you have a second pool, the text
names the pool it read, such as `pool Acme`.

## Good to know

- The JSON names each account by its id, with its label beside it. Every field is in
  [Reference](../reference/sessions-json.md).
- An account removed shows as `team (removed 4 Sep)` in the text.
- Just after an upgrade that moves the history, `sessions` refuses until the router has moved it.

## Examples

```bash
switchboard sessions
switchboard sessions --json | jq '.sessions[] | select(.running)'   # those running now
switchboard sessions 5b0e --json | jq '.turns[0]'                    # its newest turn
```

## Related

- [Your record](../your-record.md): what the record holds, and what worth means.
- [Pinning and moving sessions](../pinning-and-moving.md): move a session to another account.
- [`requests`](requests.md): each request of a session, one line each.
