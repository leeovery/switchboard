# The status document

`status --json` and `usage --json` print the status document: where every account in a pool stands
now, and what the router is doing. It's what an agent reads to see where usage stands. For what the
dashboard draws from it, see [Dashboard](../dashboard.md).

```bash
switchboard status --json \
  | jq '{source, pool_label, best, sessions}'
```

```json
{
  "source": "router",
  "pool_label": "Acme",
  "best": "m2kp",
  "sessions": 4
}
```

## General rules

- Times are in UTC, except the times of day in `prime`, which are local.
- Utilizations are fractions, and can pass 1.
- A document built by probing leaves out every field only the router knows.
- Once the config has pools, each document is of one pool: the one `--pool` names, else the pool
  the verbs act on. See [`status`](../commands/status.md).
- In a document of one pool, the accounts, `best`, `primary`, `pin`, `pool`, `sessions` and
  `demand` are of that pool only.
- The document of every pool shares the parts that are no pool's own. Those are the router's
  health, its restart, the moves, the export, and the events of no pool.

The entries of `accounts` have [a page of their own](status-accounts.md).

## About the document

| Field | Type | Meaning |
|---|---|---|
| `schema` | number | `1`. See [Reading the JSON](reading-the-json.md). |
| `generated_at` | time | When the document was built. |
| `source` | string | `router`, or `probe` if the document was built by probing every account. |
| `fallback` | object | Why the document was built by probing, if the router was asked first. |
| `pool_id` | string | The id of the pool the document is of. Left out if the config has no pools. |
| `pool_label` | string | The label of that pool. Left out with `pool_id`. |

`fallback` holds `router`, how the router was found, and `reason`, what was wrong with its answer:

| `fallback.router` | Meaning |
|---|---|
| `not running` | Nothing answers on the router's socket. |
| `unhealthy` | Something answers, but not as a router does. `reason` says why. |
| `before pools` | The router reports no pools, but the config has them. |

A document that falls back for `before pools` is of every account, never of one pool.

## Where new sessions go

| Field | Type | Meaning |
|---|---|---|
| `best` | string | The account new sessions go to next. Left out if there's none. |
| `primary` | string | The id of the pool's [primary](../glossary.md). |
| `pin` | object | The pool's pin, as `{accounts, since, move}`. Left out if there's none. |
| `pool` | object | The room new sessions can go to, summed across the pool's accounts. See below. |
| `prime` | object | The priming schedule. Left out if priming is off. |
| `coming_up` | list of objects | What happens next to the accounts, soonest first. |
| `demand` | object | The pool's use, as the week forecast takes it. From the router only. |
| `sessions` | number | How many sessions the router has routed in the pool in the last hour. |

- **`pin.move`** is `true` if the pin moved the running sessions too.
- **`pool`** is left out if the pool has one account, whose own windows say the same.

### `pool`

`pool` holds `accounts`, the ids of the accounts it sums: every account in the pool, or only the
pinned ones while a pin holds. `pinned` is set if those are the accounts the pin names. Its `windows` hold one entry for each window:

| Field | Type | Meaning |
|---|---|---|
| `key` and `label` | string | The window, such as `7d`. |
| `of` | number | How many of the accounts have the window. |
| `room` | number | The room left in it, summed in accounts: `1.5` is an account and a half. |
| `used` | number | The mean use of the window across the accounts. |
| `pace` | number | The mean even pace of the accounts whose window is running. |

- **`room`** for each account runs to its cap, `1 − reserve`. For an account the pin names, it
  runs to the limit instead, because a pin spends the reserve.
- **`of`** counts an account not read yet, or without a usable token, as having the window.
- **A window is kept** only if at least two of the accounts have it, or all of them do.

### `prime`

| Field | Type | Meaning |
|---|---|---|
| `day` | string | The local hours the resets spread over, such as `08:00-23:00`. An end before the start is past midnight. |
| `window` | string | The key of the window a prime starts, such as `5h`. |
| `slots` | list of objects | The daily prime of each account, as `{account, at, next}`. |

`at` is the local time of day, such as `03:50`. `next` is when the router next primes the account.
An account without that window, such as a verified Ultra account, has no slot. See
[Staggered resets](../staggered-resets.md).

### `coming_up`

Each entry is `{at, account, kind, window, cap, refused}`.

| `kind` | What comes |
|---|---|
| `back` | The account has room again: its limit lifts, or its cap window resets, or its refused token is allowed again. |
| `runs_out` | A window runs out before it resets, at the cap with `cap` set, else at the limit. |
| `reset` | A window resets. |
| `prime` | The router primes the account. |

`refused` is set if the account is back from a refusal of its token. `window` is left out of a
prime, and of an account held back from every request.

### `demand`

| Field | Type | Meaning |
|---|---|---|
| `day` | number | The pool's rate over the last day, in Pro weeks an hour. |
| `hours` | number | The hours with room that `day` is measured over, 24 at most. |
| `hold` | list of objects | The share of an account's fast rate still on it after so many hours, as `{hours, share}`. |
| `learning` | boolean | `true` while the router has less than a day of answers. |

- **`hours` is 0** if `day` is the fast rate of the pool instead.
- **`hold`** gives points at 1, 3, 6, 12, 24 and 48 hours.
- **While `learning`**, `hold` is the default: all the use of an account stays on it.
- **`demand` is left out** if the router has read no account.

How the week forecast uses these is in [Routing rules](routing.md).

## The router

| Field | Type | Meaning |
|---|---|---|
| `router` | object | The router's health over the last 5 minutes. |
| `restart` | object | A restart the router has due. Left out if none is. |
| `moving` | object | How a move of the record stands. Left out if no move is owed. |
| `export` | object | How the hourly export stands. Left out if `[export] dir` isn't set. |
| `events` | list of objects | What has happened lately, newest first. |
| `events_kept` | object | The ids of the oldest and newest events the router keeps, as `{oldest, newest}`. |

### `router` and `restart`

`router` holds `healthy`, `requests` and `failures`, and a `reason` while it's unhealthy.

| `restart` field | Meaning |
|---|---|
| `reason` | Why it's due: `config changed`, `upgraded` or `time zone changed`. |
| `since` | When the router found it due. |
| `in_flight` | How many requests the router had in flight. |
| `exporting` | The folder of an export under way that the restart waits for. |
| `by_hand` | Set if the router was run by hand, with `serve`. |
| `held` | Why the binary the router would become refuses to start. That holds the restart. |

### `moving`

The router owes a move after the upgrade that made account ids, and then the move to pools.

| `moving` field | Meaning |
|---|---|
| `move` | `ids` for the move onto account ids, `pools` for the move to pools. |
| `stage` | `running`, `awaiting api`, `awaiting restart` or `failed`. |
| `account` | While it awaits the API, the account whose subscription the API hasn't named, by the id it had. |
| `why` | Why it awaits the API, or why it failed. |

The move to pools never awaits the API. It awaits a restart while the router hasn't taken up the
first pool its config now holds. A router from before pools leaves `move` out. See
[Upgrades and restarts](../upgrades-and-restarts.md).

### `export`

| `export` field | Meaning |
|---|---|
| `dir` | The folder it exports into. |
| `at` | When an export last succeeded. Left out before one has. |
| `error` | Why the last export failed, in plain words. Left out once one succeeds. |
| `running` | When the export under way began. Left out while none is. |

### `events` and `events_kept`

Each event has the fields of a [router event](events.md), without `run`. They name accounts and
pools by id only. A document of one pool holds these events:

- the starts and moves of its sessions, and its pins, by the pool each recorded;
- what befell an account, by the pool it's in now, or the pool the accounts record keeps for a
  removed account;
- the router's own events.

An event that names no pool counts as the first pool. An event of a removed pool is in no
configured pool: `events --pool` reads it by the id of that pool.

`events_kept` lets a reader tell an event the router no longer keeps from one of another pool.

| If the router | `events_kept` is |
|---|---|
| Routes within pools, one pool included | Given |
| Routes by the pools the accounts record keeps | Given |
| Routes in no pools | Left out |
| Didn't build the document | Left out |

- **The accounts record keeps the pools** if the config names none.
- **With no pools,** `events` are every event the router keeps.
- **A document built by probing** has no `events_kept`.
