# The control API

The commands and the dashboard talk to the router over `control.sock` in the state directory. Only
its owner can read the socket. Every answer is JSON. The verbs are the stable way in: this page is
for anyone who'd ask the router directly.

```bash
curl -s --unix-socket ~/.local/state/switchboard/control.sock http://router/health | jq '{ok, version, reports_pools}'
```

```json
{"ok": true, "version": "0.8.0", "reports_pools": true}
```

## The endpoints

| Endpoint | What it does |
|---|---|
| `GET /health` | Says the router is alive, whether it's healthy, and the pools it routes for. |
| `GET /status` | The [status document](status-json.md) of a pool. |
| `GET /sessions` | The sessions routed in a pool in the last hour. |
| `GET /sessions/{id}` | Where one session sends its requests, as [`status --session` JSON](status-session-json.md). |
| `POST /sessions/{id}/pin` | Pins a session to an account, given as `{"account": "b6s7"}`. |
| `DELETE /sessions/{id}/pin` | Clears the own pin of a session. |
| `POST /pin` | Pins a pool, given as `{"accounts": ["b6s7"], "move": false, "force": false}`. |
| `DELETE /pin` | Clears the pin of a pool. |
| `POST /refresh` | Probes the accounts whose usage is older than `{"max_age": "30m"}`. |
| `GET /history` | The use of a window by each account over its current length, for the charts. |
| `GET /stream` | Each routed request as it happens, a JSON object a line, held open. |
| `POST /restart` | Restarts the router now, without waiting for a quiet moment. |
| `GET /import` and `POST /import` | Whether an import is running, and taking in a staged import. |

- **The pin endpoints** answer with what they leave: a session, or the status document of the pool.
- **`POST /refresh`** also probes the accounts that can take no request anyway. It answers with the
  status document.
- **`GET /stream`** names the `pool` of each request, by id, if the config has pools.
- **`POST /restart`** refuses with a 409, saying why, if the router can't restart. It can't if it
  was run by hand, or if its binary refuses to start. Nor can it if the upgrade can't be copied in,
  or an export is under way.

## `GET /health`

| Field | Type | Meaning |
|---|---|---|
| `ok` | boolean | `false` while the router fails too many of the requests it routes. |
| `reason` | string | Why it's unhealthy. |
| `listen` | string | The address the proxy listens on. |
| `version` | string | The version of the router. |
| `pid` | number | Its process id. |
| `started_at` | time | When it started. |
| `strips_own_headers` | boolean | `true` from a router that takes every `X-Switchboard-` header off before going upstream. |
| `reports_pools` | boolean | `true` from a router that reports the pools it routes for. |
| `pools` | list of objects | The pools the router routes for now, in the order of the config. |
| `moving` | object | How the moves of the history stand, as `moving` in the status document. |
| `restart` | object | The restart the router has due, as `restart` in the status document. Left out if none is. |

- **`listen`** may differ from the address the config names now. It's empty until the proxy
  listens.
- **`strips_own_headers`** decides whether `claude` sends the directory and the pool. See
  [What `claude` sets](environment.md).
- **A router from before pools** leaves `reports_pools` out. `claude` counts that router as down if
  the config has two pools or more.

Each entry in `pools` holds these:

| Field | Type | Meaning |
|---|---|---|
| `id` and `label` | string | The pool. |
| `dirs` | list of strings | Its folders, made absolute. Left out for the unfenced pool. |
| `written_dirs` | list of strings | Its folders as the config writes them. Left out for the unfenced pool. |
| `accounts` | list of strings | The ids of the accounts it routes in the pool. |
| `primary` | string | The primary among them. Left out if it has none. |

An account the config added after the router started waits for the next restart. Until then, it's
left out of `accounts`.

| If | `pools` is |
|---|---|
| The config has pools | Given |
| The config has no pools | Left out |
| The router routes nothing, because the config lost its pools and the accounts record can't say them | Left out, with `ok` `false` and `reports_pools` set |

That last case is described in [The accounts record](accounts-record.md).

## Choosing a pool

`GET /status`, `GET /sessions`, `GET /history`, `GET /stream`, `POST /refresh` and `DELETE /pin`
each act for one pool:

| If | They act for |
|---|---|
| `?pool=<id>` names a pool | That pool |
| No pool is named | The pool the verbs act on |
| The config has no pools | Every account |
| The pool named isn't configured | Nothing: a 400 |
| No pool is named, and every pool is fenced | Nothing: a 400 |

The 400 answers `{error, pools, named}`. `error` says why in words. `pools` lists each pool you can
name, as `{id, label}`. `named` is the id named, if one was.

`POST /pin` acts in the pool of the accounts it names. `?pool=` needn't name it, but if it does, it
must be that pool.

## A session in a pool

A session resumed in another pool runs in both, and the work in each is its own.
`GET /sessions/{id}`, `POST /sessions/{id}/pin` and `DELETE /sessions/{id}/pin` take `?pool=` too.

| If | They answer or act for |
|---|---|
| `?pool=` names a pool | The session in that pool, or a 404 if it was never seen there |
| No pool is named, for `GET` or `DELETE` | The pool the session was routed in last |
| No pool is named, for `POST` | The pool of the account pinned to, if the session runs there |
| No pool is named, for `POST`, and the session doesn't run in the pool of that account | The pool the session was routed in last, where an account of another pool is refused |
