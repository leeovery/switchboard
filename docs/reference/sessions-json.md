# `sessions` JSON

`sessions --json` lists the sessions of today in a pool. Given a session id, it prints the page of
that session instead. For moving and pinning sessions, see
[Pinning and moving sessions](../pinning-and-moving.md).

```bash
switchboard sessions --json | jq \
  '.sessions[0]
  | {session, label, running, requests}'
```

```json
{
  "session": "5f0c2a…",
  "label": "work",
  "running": true,
  "requests": 212
}
```

## The list

| Field | Type | Meaning |
|---|---|---|
| `schema` | number | `1`. See [Reading the JSON](reading-the-json.md). |
| `generated_at` | time | When the list was built. |
| `prices_as_of` | string | The date the prices were read. |
| `sessions` | list of objects | Each session. See below. |
| `today` | object | The sessions summed, as `{sessions, requests, worth, unpriced}`. |

The list holds the sessions the router lists, oldest started first, then those that ended today,
latest ended first.

Once the config has pools, it lists the sessions of one pool, each by the pool its requests were
routed within. A session from before pools is in the first pool.

## A session in the list

| Field | Type | Meaning |
|---|---|---|
| `session` | string | The whole id of the session. |
| `dir` | string | The directory it runs in, with your home as `~`. |
| `account` and `label` | string | The account its main thread is on now. |
| `model` | string | The model of its latest main request. |
| `running` | boolean | Set while the router lists it. |
| `started` | time | When its first request came, today or earlier. |
| `ended` | time | For an ended session, when its last request came. |
| `requests` | number | Its requests today. |
| `worth` and `unpriced` | number, list | What those requests were worth, in US dollars, and what that leaves out. |
| `moved` | object | The move that brought it to its account today. See below. |

A running session adds these:

| Field | Type | Meaning |
|---|---|---|
| `last_seen` | time | When the router last saw it. |
| `models` | list of objects | Each model, as `{model, account, label, reason}`: where it goes now, and why. |
| `state` | string | `answering` or `asking`, while a request is in flight. |
| `move_cost` | number | What moving it now would cost, in US dollars. |

- **`move_cost`** is left out if the cache would have run out anyway, or the cost isn't known.
- **`moved`** holds `{at, from, from_label, reason, cost}`. `from` is the account it left. `cost`
  is what writing its context again there cost.
- **`reason`** is one of the [routing reasons](reasons.md).

## The page of a session

`sessions <id> --json` prints one session. It has the fields of the list: `session`, `dir`,
`account`, `label`, `model`, `running`, `last_seen`, `models`, `state`, `move_cost` and `ended`.
It adds these:

| Field | Type | Meaning |
|---|---|---|
| `pin` and `pin_label` | string | The session's own pin, and that account's label. |
| `started` | time | When its first request came. |
| `resumed` | time | The latest time it came back after an hour or more without a request. |
| `kept_until` | string | For an ended session, the last day the ledger keeps the lines of its first day. |
| `accounts` | list of objects | Each account it ran on. See below. |
| `moves` | list of objects | Each move. See below. |
| `turns` | list of objects | Each turn, newest first. See below. |
| `totals` | object | The whole session, as `{turns, requests, from_cache, worth, unpriced}`. |
| `pool` and `pool_label` | string | The pool the page is of. |

- **`kept_until`** is left out of a running session, and if the ledger keeps lines for good.
- **`from_cache`** is the share of the tokens sent that the cache served.

| Entry | Holds |
|---|---|
| An account | `{account, label, from, to, requests, worth, points}`. `points` is by window, an estimate of the points of each window it took. |
| A move | `{at, model, from, to, from_label, to_label, reason, written, cost}`. `written` is the tokens the move wrote to the cache. |
| A turn | `{turn, started, ended, requests, tools, read, written, out, worth, accounts, labels}`. |

- **`ended`** is left out of a turn still going.
- **`tools`** is a list of `{tool, times}`, most called first.
- **`labels`** are the labels of `accounts`, in the same order.

## The pool of a page

Once the config has pools, a page is the work of the session in one pool.

| If | The page is of |
|---|---|
| `--pool` names a pool | That pool |
| Not | The pool the session is in now: the one its newest request was routed in |

Without `--pool`, `pool` and `pool_label` are left out if the pool can't be read from the ledger:

- a running session that hasn't filed a request yet, because the session list of the router names
  no pool;
- a session of a pool that neither the config nor the accounts record keeps.

Every `worth` is exact, and left out if nothing was priced. `unpriced` names what it leaves out, as
[`history` JSON](history-json.md) explains under Worth.
