# The router events

The router writes a line of JSON for each version of an event, in `ledger/events-<date>.jsonl` in
the state directory, a file a day. An event can change, so if one appears more than once, its last
version is how it stands. The Log on the dashboard and `switchboard events` show the same events.

```bash
switchboard events --json | tail -1 \
  | jq '{id, kind, from, to, reason}'
```

```json
{
  "id": 42,
  "kind": "moved",
  "from": "b6s7",
  "to": "q4tx",
  "reason": "moved: b6s7 hit its limit"
}
```

`events --json` prints each line as it's stored, with `"schema": 1` put first. With pools, it puts
`pool_label` after it too.

## The kinds

| `kind` | What happened |
|---|---|
| `started` | A session went to an account for the first time. |
| `moved` | A session moved to another account. |
| `pressure` | An account came under pressure. |
| `cap` | A window of an account reached its reserve. |
| `limit` | An account reached its limit. |
| `refused` | The upstream refused requests on an account. |
| `held` | A [hold](../glossary.md) started on an account, because Anthropic refused its usage read. |
| `foreign` | The token of an account turned out to be another subscription's. |
| `primed` | A prime started a window of an account. |
| `room` | An account had room again, after a limit, a cap or a hold. |
| `pin` | Routing was set by hand: a pin of a pool, or of one session. |
| `auto` | Routing was given back to the router: a pin cleared. |
| `restart` | A restart fell due, or was put off. |
| `health` | The router turned unhealthy, or healthy again. |

## Fields every event has

| Field | Type | Meaning |
|---|---|---|
| `run` | time | When the router that told of the event started. |
| `id` | number | The number of the event in that run, rising from 1. |
| `at` | time | When the event happened. |
| `kind` | string | What happened. See above. |

`run` and `id` together name an event. The other fields appear only on the kinds that need them.

## Who and where

| Field | Type | Meaning |
|---|---|---|
| `account` | string | The account the event befell. |
| `accounts` | list of strings | On a pool's `pin` or `auto`, the accounts its pin names, in the config order. |
| `pool` | string | The id of the pool the event is of. |
| `session` | string | The session that started or moved, or whose own pin changed. |
| `model` | string | The model whose requests started or moved. |
| `dir` | string | The directory the session ran in, with your home as `~`. |
| `project` | string | The repository that directory is in, as the [request ledger](ledger.md) gives it. |
| `from` | string | The account a session moved from. |
| `to` | string | The account a session moved to. |

- **`account`** on the `pin` of a pool is the first account the pin names. On the `pin` or `auto`
  of one session, it's the account that session pin names. The `auto` of a pool has no `account`,
  only `accounts`.
- **`to`** on a `limit`, `cap`, `refused` or `held` event is where every session it moved went, if
  they all went to one account.
- **`dir` and `project`** are each left out if none was known yet, and on events written before
  switchboard recorded them.

### The pool of an event

| Kind | `pool` is |
|---|---|
| `started` and `moved` | The pool the request of the session was routed within |
| `pin` and `auto`, for a pool | The pool whose pin was set or cleared |
| `pin` and `auto`, for one session | The pool the session pin was set or cleared in |
| Any other kind | Left out |

`pool` is left out if the config has no pools. The move to pools gave the first pool to every
event of these kinds from before pools. An import of a folder exported before pools does the same.
Once the move is done, an event of these kinds without a pool is in the first pool.

## Why, and until when

| Field | Type | Meaning |
|---|---|---|
| `reason` | string | Why. See below. |
| `held` | string | On `restart` only, why the binary the router would become refuses to start. That puts off the restart. |
| `windows` | list of strings | The keys of the windows the event is of. |
| `joined` | object | On `limit` or `cap`, when each window joined it after it began, by key. |
| `until` | time | When a limit, cap, refusal or `held` hold ends, or a window runs out or resets. |
| `since` | time | When the rate of a `pressure` event is measured from. On a `held` event told late, when the hold began. |

| Kind | `reason` holds |
|---|---|
| `started` and `moved` | Why the session went to its account. See [Routing reasons](reasons.md). |
| `held` | Why Anthropic refused the read: `subscription lapsed` or `token refused`. |
| `room` | After a hold, the cause of the hold that lifted. Left out after a limit or a cap. |
| `restart` | Why the restart is due, in words for a person. |
| `health` | Why the router turned unhealthy, in words for a person. |

- **`joined`** shows, for example, the 5-hour window joining a cap on Fable's week. A window
  it leaves out was in the limit or cap from `at`. It's left out if no window joined.
- **`until`** is brought forward to the lift if a limit or a refusal lifts early. A `held` event
  gets an `until` only if its hold lifts in the same run of the router, within 8 days, at the
  time of the `room` event that tells of the lift.
- **A hold's lift** is the first `room` event of its account after the `held` event that has a
  `reason`. Pair them that way, because a hold across a restart, or longer than 8 days, has no
  `until`. Whether an account is held now is in the status document, never in a missing
  `until`.
- **A hold is told once.** Another refused read during the hold adds no event. After a restart, a
  hold that already has a `held` event gets no new one.
- **A hold from before this event existed** gets a `held` event once, when the router first starts
  on a version that tells holds. No notification goes out. The event's `since` is when the hold
  began. The event's `at` is the same, but no more than 7 days back, so the event files can take
  it. Such an event has no `status`, because nothing kept the read's answer.

## Counts and settings

| Field | Type | Meaning |
|---|---|---|
| `count` | number | How many sessions a limit, cap, refusal or hold moved while it lasted. |
| `limit` | number | On `limit`, its identity, kept while the limit lasts. On `moved`, the `id` of the limit event that counts this move. |
| `forced_by` | number | On `moved`, the `id` of the limit, cap, refusal or hold event that counts this move. |
| `reserve` | number | On `cap`, the reserve of the account. |
| `status` | number | On `refused` and `held`, the HTTP status the API refused with. |
| `family` | string | On `refused`, if the API refused the request rather than the token, the model family it bars. |
| `move` | boolean | On `pin`, set if the pin moves the running sessions too. |
| `force` | boolean | On `pin` and `auto`, set if the pin cleared the pin of every session. |
| `by` | string | What set or cleared a pin: `cli`, `dashboard` or `router`. |

- **`force`** clears only the session pins in its own pool, if there are pools.
- **`by` is `router`** on an `auto` event for a pool pin the router dropped as the pools changed.
  That happens if its accounts left the pool, the pool went, or a later pin took its place.

How long the files stay is in [Where the files are](files.md).
