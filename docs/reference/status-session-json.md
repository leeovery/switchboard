# `status --session` JSON

`status --session <id> --json` prints where one session sends its requests, model by model, and
why. A statusline or an agent reads it. For pinning a session, see
[Pinning and moving sessions](../pinning-and-moving.md).

```bash
switchboard status --session "$SESSION_ID" \
  --json | jq '.assignments[0]
  | {model, account, reason}'
```

```json
{
  "model": "claude-opus-5-5",
  "account": "b6s7",
  "reason": "sticky"
}
```

## The fields

| Field | Type | Meaning |
|---|---|---|
| `schema` | number | `1`. See [Reading the JSON](reading-the-json.md). |
| `session` | string | The whole id of the session. |
| `pin` | string | The session's own pin, if it has one. |
| `assignments` | list of objects | One for each model in the pool, the one used last first. |
| `account` | object | The account the last-used model went to, in the form of an entry in [the status accounts](status-accounts.md). |

The document is of one session in one pool. A session resumed in another pool runs in both.

| If | The pool is |
|---|---|
| `--pool` names one | That pool |
| Not | The pool the session was routed in last |

## An assignment

| Field | Type | Meaning |
|---|---|---|
| `model` | string | The model. |
| `family` | string | The model family, such as `opus`. |
| `account` | string | The account the requests of the model go to. |
| `auto` | string | The account the next request would go to without the session's own pin. |
| `dir` | string | The directory the last request of the model named. |
| `project` | string | The repository of that directory, as the [request ledger](ledger.md) gives it. Left out until a request names one. |
| `name` | string | The name of the terminal session the last request of the model to name a place was started in, as the [request ledger](ledger.md) gives it. Left out if none was. |
| `in_flight` | string | `asking` or `answering`, while a request is in flight. |
| `pinned` | boolean | Set if the session's own pin put it there. |
| `yielded` | boolean | Set while the session's own pin has given way. |
| `pinned_at` | time | When the session was pinned while it ran. |
| `reason` | string | Why it went to the account. See [Routing reasons](reasons.md). |
| `assigned_at` | time | When it went to the account. |
| `last_seen` | time | When the router last saw a request of the model. |

- **`auto`** is left out if the router can't say, for example if no account has room.
- **`in_flight`** is `asking` while a request waits for its first byte, and `answering` while an
  answer streams.
- **`pinned_at`** is left out for the pin the session was launched with.
