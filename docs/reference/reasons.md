# Routing reasons

Each request records why it went to its account, as a `reason`. If it left an account that
couldn't serve it, it records why as a `why`. Both are fixed words. For how the router chooses,
see [Routing](routing.md).

```bash
switchboard requests --json \
  | jq -r .reason | sort | uniq -c
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
| `pinned` | The session's own pin. |
| `pinned (pool)` | One of the accounts the pool's pin names, chosen afresh. |
| `pinned (global)` | One of the pinned accounts, chosen afresh, as written before pools. |
| `moved by pin` | A pool's pin set with `--move`. |
| `sticky` | The session's account, while its cache is warm. |
| `bound` | The session's account, idle past the cache hour but bound to it. |
| `unsessioned` | A request without a session id. |
| `no account has room` | No account can take it. |
| `back where it was before its request` | Every account tried refused it. |
| `token has left its pool` | Switchboard refused it, and told the session to exit and resume. |
| `client` | No longer given. See below. |
| `moved: <id> <why>` | The session's account, `<id>`, can't serve it, for one of the whys. |
| `pin yields: <id> <why>` | The session's own pin to `<id>` gave way, for one of the whys. |
| `rescored after <idle> idle` | Chosen afresh after `<idle>`, such as `5d 12h`, `6d`, `4h 57m`, `7m` or `now`. |
| `compacted: <id> would reset with <n>% of its week left` | The first request after a compaction, moved to `<id>`. |
| `<reason>, <id> under pressure` | Any reason above, if passing over `<id>` changed the choice. |

- **`bound`** keeps a session that has been idle past the cache hour, because the model's
  thinking is bound to that account.
- **`no account has room`** sends the request where the API will say why, or switchboard answers
  429 itself.
- **`back where it was before its request`** puts the session back on the account it was on.
- **`token has left its pool`** means the token of the session belongs to an account no longer in
  the pool of the request. Or the config no longer has the pool.
- **`client`** meant the router kept the token the client sent, because the account chosen had no
  token it could send on. Since pools, such a request gets a 503, so nothing goes out on a token
  outside its pool. Lines from before pools still carry it.
- **`rescored after`** follows an idle spell past the cache hour.
- **`compacted:`** moves a session whose week would otherwise reset with `<n>`% of it unused.
- **`under pressure`** means `<id>` was under pressure, so the router passed it over.

## The whys

| Why | What held the account back |
|---|---|
| `hit its limit` | It answered the request with its limit reached. |
| `was refused` | It refused the request. |
| `has a new token` | It refused the token, because it had taken up a new one. |
| `has no usable token` | Its token file stopped holding a usable token while the request was in flight. |
| `left the pool` | It left the request's pool, for another pool or out of the config. |
| `is at its reserve` | It has reached its reserve. |
| `had its read refused, its subscription ended` | Anthropic refused its last usage read because its subscription has ended, so the router holds it. |
| `had its read refused, its token refused` | Anthropic refused the token on its last usage read, so the router holds it. |
| `had its read refused` | Anthropic refused its last usage read, so the router held it. Only lines written before the why named the cause carry this why. |
| `has no room` | It has no room for the request otherwise. |

## Where each why appears

| Why | Can appear in |
|---|---|
| `hit its limit`, `was refused`, `has a new token`, `has no usable token` | A reason, or `tried` |
| `left the pool` | A reason, or `tried`, only with two pools or more |
| `is at its reserve`, and the three that start `had its read refused` | A reason only |
| `has no room` | A reason only, except as below |

- **`has a new token`** sends the request on elsewhere.
- **`had its read refused`** means the account's subscription has ended, or its token is
  refused. The rest of the why says which. The router holds the account back until a read comes
  back clean. See [Troubleshooting: an account's token or subscription](../troubleshooting/tokens.md).
- **`left the pool`** sends the request elsewhere, or moves the session off the account. With one
  pool, an account that left the config `has no room` instead.
- **`has no room` appears in `tried`** for an account that left the config while one pool is
  configured. The request left it before it went out on it again.
