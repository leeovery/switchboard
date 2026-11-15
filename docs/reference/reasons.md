# Routing reasons

Each request records why it went to its account, as a `reason`. If it left an account that
couldn't serve it, it records why as a `why`. Both are fixed words. For how the router chooses,
see [Limits and routing](../limits-and-routing.md).

```bash
switchboard requests --json | jq -r .reason | sort | uniq -c
```

```
   3 moved: b6s7 hit its limit
   9 new
 412 sticky
```

## Where they appear

| Format | Field |
|---|---|
| The [request ledger](ledger.md) | `reason`, and `why` in each entry of `tried` |
| The [router events](events.md), for a session starting or moving | `reason` |
| [`status --session` JSON](status-session-json.md) | `reason` in each assignment |
| [`sessions` JSON](sessions-json.md) | `reason` in its models, its moves and `moved` |

None of these words is ever reworded, because years of lines hold them. A release can add one, so
pass over one you don't know. In the forms below, `<id>` is the id of an account.

## The reasons

| Reason | Why the request went there |
|---|---|
| `new` | The first account of a new session. |
| `pinned` | The own pin of the session. |
| `pinned (pool)` | One of the accounts the pool pin names, chosen afresh. |
| `pinned (global)` | One of the accounts of the pin, chosen afresh, as switchboard wrote it before pools. |
| `moved by pin` | A pool pin set with `--move`. |
| `sticky` | The account of the session, whose cache is warm. |
| `bound` | The account of the session, idle past the cache hour, kept because the thinking of its model is bound to it. |
| `unsessioned` | A request without a session id. |
| `no account has room` | No account can take it. It goes where the API will say why, or switchboard answers 429 itself. |
| `back where it was before its request` | Every account the request went out on refused it, so the session goes back to the account it was on. |
| `token has left its pool` | Switchboard refused the request and told the session to exit and resume. |
| `client` | No longer given. See below. |

- **`token has left its pool`** means the token of the session is of an account no longer in the
  pool of the request. Or the config no longer has the pool.
- **`client`** meant the router kept the token of the client, because the account chosen had no
  token it could send on. Since pools, such a request gets a 503, so nothing goes out on a token
  outside its pool. Lines from before pools still carry it.

## Reasons with a form

| Reason | Why the request went there |
|---|---|
| `moved: <id> <why>` | The account of the session, `<id>`, can't serve the request, for one of the whys below. |
| `pin yields: <id> <why>` | The session pin to `<id>` gave way, because `<id>` can't serve the request. |
| `rescored after <idle> idle` | The session was idle past the cache hour, so its account was chosen afresh. |
| `compacted: <id> would reset with <n>% of its week left` | The first request after a compaction, moved to `<id>`. |
| `<reason>, <id> under pressure` | Any reason above, where passing over `<id>`, under pressure, changed the choice. |

- **`<idle>`** is how long the session was idle, such as `5d 12h`, `6d`, `4h 57m`, `7m` or `now`.
- **`compacted:`** moves a session whose week would otherwise reset with `<n>`% of it unused.

## The whys

| Why | What held the account back |
|---|---|
| `hit its limit` | It answered the request with its limit reached. |
| `was refused` | It refused the request. |
| `has a new token` | It refused the token of the request because it had taken up a new one. The request went on elsewhere. |
| `left the pool` | It left the pool of the request, for another pool or out of the config. |
| `is at its reserve` | It has reached its reserve. |
| `has no room` | It has no room for the request otherwise. |

| Why | Can appear in |
|---|---|
| `hit its limit`, `was refused`, `has a new token` | A reason, or `tried` |
| `left the pool` | A reason, or `tried`, only if there are two pools or more |
| `is at its reserve` | A reason only |
| `has no room` | A reason only, except as below |

- **`left the pool`** sends the request elsewhere, or moves the session off the account. With one
  pool, an account that left the config `has no room` instead.
- **`has no room` appears in `tried`** for an account that left the config while one pool is
  configured. The request left it before it went out on it again.
