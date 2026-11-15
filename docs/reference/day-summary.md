# The day summary

An hour after each day ends, the router sums up the day in `ledger/day-<date>.json`, beside its
lines. The summary outlasts the lines: once they go, it's kept for good. `history` and the dashboard
read it. For what the record is good for, see [Your record](../your-record.md).

```bash
cd ~/.local/state/switchboard/ledger
jq '{version, day, lines}' day-2026-11-12.json
```

```json
{
  "version": 7,
  "day": "2026-11-12",
  "lines": 1840
}
```

## The top level

| Field | Type | Meaning |
|---|---|---|
| `version` | number | The version the summary was written in. `7` since pools. |
| `day` | string | The local date of the day, such as `2026-11-12`. |
| `lines` | number | How many lines the summary was made from. |
| `bytes` | object | How large the files of the day were then, as `{plain, compressed}`. |
| `accounts` | list of objects | The day of each account. See below. |

A summary of version 6 or older holds all its work in no pool. A day with no requests, and no
account read, has no `accounts`.

## An account's day

| Field | Type | Meaning |
|---|---|---|
| `account` | string | The account, by id only. Left out of the requests no account answered. |
| `pool` | string | The pool all the account's work was in that day. |
| `models` | list of objects | The requests of the account, by the model each asked for. |
| `sessions` | number | How many sessions the requests were of. |
| `session_ids` | list of strings | The ids of those sessions. |
| `moved_on` | number | How many sessions its requests moved onto the account. |
| `moved_off` | number | How many sessions its requests moved off the account. |
| `pools` | list of objects | The work split by pool, if it was in more than one. |

`pool` is left out in each of these cases:

- the requests named no pool;
- the requests were in more than one pool;
- the account did no work that day.

A pool in which the account did no work, such as one named only by a move that moved no session, isn't
counted.

## Window fields

| Field | Type | Meaning |
|---|---|---|
| `highest` | object | The highest use of each window that day, by the key of the window. |
| `rise` | object | How far the use of each window rose that day, from the readings history. |
| `resets` | list of objects | The resets of the windows that day, as `{window, at, before}`. |
| `limits` | list of objects | The limits the account reached, as `{window, at, resets_at}`. |
| `minutes_at_cap` | number | The minutes the account spent at its cap. |
| `minutes_at_limit` | number | The minutes the account spent at a limit. |
| `read_before` | list of strings | The windows the readings history read in the week before. |

- **`before`** in a reset is the highest use of the window before the reset.
- **A limit** is a window turning `rejected`.
- **`rise`, `resets` and the minutes** are left out if the readings history held no window of the
  account, that day or the week before. Then they're unknown, not zero.

## A model entry

Each entry in `models` counts requests of one model by how they went.

| Field | Type | Meaning |
|---|---|---|
| `model` | string | The model the requests asked for. |
| `inference_geo` | string | Where the requests asked to run, if they asked. |
| `upstream` | number | The requests that went upstream. |
| `hours` | list of numbers | The `upstream` requests by local hour. See below. |
| `no_usage` | number | The upstream requests that got no usage back. |
| `unsent` | number | The requests the router answered itself. |
| `checks` | number | Quota checks by Claude Code. |
| `counts` | number | Counts of tokens by Claude Code. |
| `sessions` | number | How many sessions the requests were of. |
| `session_ids` | list of strings | The ids of those sessions. |
| `usage` | object | The usage of the requests, summed field for field. |

- **`hours`** holds 24 counts, from the midnight hour on, by the local hour of `at` on each line.
  They add up to `upstream`. An hour the clocks skip has none, and an hour they repeat counts both.
- **The `usage` of quota checks** is left out, because checks aren't work done for you, so nothing
  prices them.
- **A model asked for in another region** gets an entry of its own, because it can cost more.

## Pools in a day

An account can move to another pool partway through a day. Its day then splits by the pool of each
request.

- **The work splits.** Each entry in `pools` holds `models`, `sessions`, `session_ids`,
  `moved_on` and `moved_off` for one pool, with that pool id as its `pool`.
- **Requests that named no pool** come first in `pools`, without a `pool`.
- **The windows don't split.** `highest`, `rise`, `resets`, `limits`, the minutes and
  `read_before` are given once, whatever pool the account is in.
- **The totals don't change.** The `models`, sessions and moves of an account are always its work
  in every pool together. Adding up the days of the accounts gives the same totals however the day
  splits.

The day of the requests no account answered splits the same way.

Once the move to pools is done, work in no pool counts as the first pool's. That covers requests
filed without a pool since, for example by an older release after a downgrade. The first pool is
the one the [accounts record](accounts-record.md) marks `first`, if the config has it. If not, it's
the first pool in the config.

## Which accounts get a day

An account gets a day of its own if any of these is true:

- The readings history read the account that day, whether or not it served a request.
- A window of the account reset that day, going by the reset its last reading gave. This keeps the
  peak of a week that ended on a quiet day.
- The readings history read the account in the week before, and a window ran on into the day
  without a reset.

## Which days get a summary

- **A day with requests** gets a summary an hour after it ends.
- **A day with no requests** gets one if the router read an account, or saw a window reset, that
  day. It waits until the readings history holds the week before. Otherwise a window already at
  its limit when the readings history began would read as reaching its limit that day.
- **A day that ended while the router was stopped** gets one when the router starts again. That
  happens before the readings of the day can go.

## Summarised again

A line can be filed under a day after the summary is written, for example by a request still in
flight past that hour. The router then summarises the day again.

- The new summary knows no less than the one before it.
- That holds even if the readings of the day have gone, or a damaged file has lost lines. The new
  lines can't be told from lost ones, so the counts are then the least there were.
- The `hours` of a model come whole from whichever summary counted more of its requests upstream.
  So they still add up to `upstream`.

A release that summarises days in a new way summarises every kept day again, once.

- It works eight days at a time, oldest first, so no request or reading waits on it.
- A day whose readings are about to go is done first, so it's still summarised with them. The
  exception is a day that can't be summarised then, for example because a file can't be opened.
- Until its turn comes, `history` and the dashboard work the day out from its lines as they read
  it, with the same figures.
- A day whose lines are gone keeps the summary it had.

## Kept for good

Once the lines of a day go, its summary is kept for good. Only a release that summarises days in a
new way summarises it again, and then from the readings of the day while they're kept.

A day summarised before the router counted `hours` gains them when it's summarised again from its
lines. If its lines went first, it never gains them: the readings can't give them.
