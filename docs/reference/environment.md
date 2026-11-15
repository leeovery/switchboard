# What `claude` sets

A session that `claude` starts through switchboard gets a few variables and headers of its own.
This page lists them. For how a session reaches the router, see
[Claude and switchboard](../claude-and-switchboard.md).

```bash
# inside a routed session
env | grep -E 'BASE_URL|SWITCHBOARD_POOL'
```

```
ANTHROPIC_BASE_URL=http://127.0.0.1:4747
SWITCHBOARD_POOL=q7mt
```

## The variables

The token Claude Code starts with goes in `CLAUDE_CODE_OAUTH_TOKEN`. Beside it, a routed session
gets these:

| Variable | What it holds |
|---|---|
| `ANTHROPIC_BASE_URL` | The address the router says it listens on. |
| `ANTHROPIC_CUSTOM_HEADERS` | A line for each switchboard header the router reads, added to any headers you set. |
| `CLAUDE_CODE_GATEWAY_HINT_HEADERS` | `1`, unless your environment sets it already. |
| `SWITCHBOARD_POOL` | The id of the pool the session is in, for the skill to name to the verbs. |
| `SWITCHBOARD_PROJECT` | The path of the project the session runs in. |
| `SWITCHBOARD_SET` | What `claude` set of these, and the values of yours it replaced. |
| `SWITCHBOARD_STARTED` | A mark on every `claude` that switchboard starts, routed or not. |

- **`SWITCHBOARD_POOL`** is set on a session that goes direct too, because the session is still in
  its pool. The variable is left out if the config has no pools.
- **`SWITCHBOARD_PROJECT`** is set on a session that goes direct too.
- **`SWITCHBOARD_SET`** lets a `claude` started inside the session start from your own
  environment. It also names the `claude` it was set for.
- **`SWITCHBOARD_STARTED`** is what the [session hook](session-hook.md) reads.
- **A `claude` started inside the session** joins the session through all four `SWITCHBOARD_`
  variables above, as the session's `claude` started with them. See
  [`switchboard run`](../commands/run.md).
- **`CLAUDE_CODE_GATEWAY_HINT_HEADERS`** makes Claude Code send the headers that fill most of the
  hint fields of the [request ledger](ledger.md).

## The headers

Each header is named `X-Switchboard-…`. None reaches the API: the router takes every header with
that prefix off a request before it goes upstream.

| Header | What it holds |
|---|---|
| `X-Switchboard-Account` | The id of the account that `--account` pins the session to. |
| `X-Switchboard-Dir` | The directory `claude` started in, with your home as `~`. |
| `X-Switchboard-Project` | The project the session runs in, with your home as `~`. |
| `X-Switchboard-Pool` | The id of the pool the session is in. The router routes its requests within it. |

- **`X-Switchboard-Dir`** percent-encodes only what a header can't carry. That's a control
  character, a byte past ASCII, a `%`, or a space at either end. `X-Switchboard-Project` is
  encoded the same way.
- **A request without `X-Switchboard-Pool`** comes from a session started before pools. The router
  routes it in the first pool.
- **The directory, the project and the pool** go only to a router whose `GET /health` answer says
  `strips_own_headers`. See [The control API](control-api.md).
