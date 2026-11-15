# The request ledger

The router writes a line of JSON for each request it routes, in `ledger/requests-<date>.jsonl` in
the state directory, a file a day. A line keeps everything about a request except its content. For
what the record is good for, see [Your record](../your-record.md).

```bash
switchboard requests --json | tail -1 \
  | jq '{at, kind, model, pool, account,
         reason, status}'
```

```json
{
  "at": "2026-11-12T09:30:01.204Z",
  "kind": "message",
  "model": "claude-opus-5-5",
  "pool": "q7mt",
  "account": "m2kp",
  "reason": "sticky",
  "status": 200
}
```

`requests --json` prints each line as it's stored, with `"schema": 1` put first. With pools, it
puts `pool_label` after it too.

## What a line never holds

- The messages of the request, its system prompt, or its tools.
- The ids in its metadata.
- Any setting or header that the line doesn't name below.
- Anything shaped like a token.

## Routing fields

| Field | Type | Meaning |
|---|---|---|
| `at` | time | When the request arrived, in UTC, to the millisecond. |
| `request` | string | The router's id for the request, as its routed line in the log gives it. |
| `kind` | string | `message`, `check` for a quota check by Claude Code, or `count` for a count of tokens. |
| `session` | string | The session id of the request. |
| `dir` | string | The directory `claude` started Claude Code in, with your home as `~`. |
| `project` | string | The repository that directory is in, with your home as `~`. See below. |
| `pool` | string | The id of the pool the request was routed within. |
| `model` | string | The model the request asked for. |
| `account` | string | The account whose answer the client got, by id only. |
| `reason` | string | Why the request went to that account. See [Routing reasons](reasons.md). |
| `from` | string | The account the session was on before, if the request moved it. |
| `tried` | list of objects | The accounts that couldn't serve it, in the order tried. |

- **`account`** is left out if the router answered the request itself.
- **`tried`** holds `{account, why}` for each account the request left. `why` is one of
  [the whys](reasons.md).
- **`dir`** reaches the router in a header the API never sees. A `claude` started inside that
  session other than through switchboard carries the directory too.

### The project of a line

`claude` finds the repository as it starts the session. It walks up from the directory to the
first `.git`, without running git. For example, `~/Code/api` is the project of a session in
`~/Code/api/web`, and of one in `~/Code/api/.claude/worktrees/fix-login`.

| If the directory is in | `project` is |
|---|---|
| A repository | That repository |
| A linked worktree | The main repository of the worktree |
| A worktree of a bare repository | The folder of the bare repository, such as `~/repos/api.git` or `~/.dotfiles` |
| A worktree inside the folder that holds a bare repository named `.bare` | The folder that holds both: `~/Code/api` for `~/Code/api/.bare` and `~/Code/api/main` |
| No repository | The directory itself |

Links change the walk as follows:

- **The walk resolves links first.** So a link into a folder inside a repository counts as that
  repository.
- **If that finds no `.git`,** the walk goes up the directory as given. So a link inside a
  repository that leads outside every repository counts as the repository the link is in.
- **A worktree goes by the path the walk took.** So a repository in a folder reached through a
  link stays one project, with the worktrees beside it in that folder.
- **A repository folder that is itself a link** counts as one project, and a worktree beside it as
  another. So do worktrees kept outside the linked folder.

A line from before switchboard recorded `project` has none. Then a `dir` in
`<repo>/.claude/worktrees/<name>` counts as `<repo>`, and any other `dir` as itself.

### The pool of a line

| If | `pool` is |
|---|---|
| The config has no pools | Left out |
| The request named its pool | That pool |
| The request named none, because its session started before pools | The first pool |
| The line was filed before pools | The first pool, given by the move to pools |
| The line was filed without a pool since the move, for example by an older release after a downgrade | Left out, and read as the first pool |

## Outcome fields

| Field | Type | Meaning |
|---|---|---|
| `status` | number | The HTTP status of the answer. `0` if the request ended before an answer came. |
| `canceled` | boolean | Set if the client went away before the end. |
| `cut_off` | boolean | Set if the router cut the request off as it stopped. |
| `broken` | boolean | Set if the upstream broke the answer off after it began. |
| `attempts` | number | How many times the request went upstream. `0` if the router answered it itself. |
| `first_ms` | number | Milliseconds from arrival until the first byte of the answer reached the client. |
| `total_ms` | number | Milliseconds from arrival until the answer ended. |
| `agent` | string | The client's user agent. |
| `betas` | list of strings | The features the request asked the API for beyond its version. |

At most one of `canceled`, `cut_off` and `broken` is set: whichever came first. `first_ms` is left out
if no byte reached the client.

## Hint fields

These come from Claude Code's headers. Each is left out if Claude Code sent no such header.

| Field | Type | Meaning |
|---|---|---|
| `prompt` | string | The random id of the prompt the request serves. |
| `class` | string | `main`, `subagent`, `workflow`, `compaction` or `auxiliary`. |
| `agent_id` | string | The id of the subagent that sent the request. |
| `parent_agent_id` | string | The id of the subagent that started that subagent. |
| `agent_type` | string | The kind of subagent: a built-in agent name, `custom`, `teammate` or `fork`. |
| `compaction` | string | On a compaction request, what started it: `auto`, `manual` or `reactive`. |
| `compacted` | string | On the first request after a compaction, what started that compaction. |
| `tool_ms` | list of objects | How long each tool call ran, for each result the request carries, as `{tool, ms}`. |

- **`prompt`** is the same on every request of one prompt, including the requests of its
  subagents.
- **`agent_id`** is random for a subagent. For a member of an agent team, it comes from its name.
- **`agent_type`** is never a name you chose.
- **Claude Code always sends** `agent_id` and `parent_agent_id`. It sends the rest because
  `claude` sets `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`. See [What `claude` sets](environment.md).

## The shape

`shape` holds the size of the request and the settings switchboard knows.

| Field | Type | Meaning |
|---|---|---|
| `bytes` | number | The size of the request body. |
| `messages` | number | How many messages it carried. |
| `system` | number | How many system blocks it carried. |
| `tools` | number | How many tools it carried. |

Beside those, `shape` holds each of these settings the request gave:

- `max_tokens`, `temperature`, `top_k` and `top_p`;
- `thinking`, as `type`, `budget_tokens` and `display`;
- `stream`;
- `tool_choice`, as its `type`;
- `service_tier`, `speed` and `inference_geo`;
- `output_config`, as its `effort`;
- `context_management`, as the types of its `edits`.

## The answer

| Field | Type | Meaning |
|---|---|---|
| `answer` | object | The answer the client got. See below. |
| `usage` | object | The closing usage of the answer, as the API gave it, field for field. |
| `limits` | object | The `anthropic-ratelimit-unified-*` headers of the answer, with the prefix taken off. |

`answer` holds these:

| Field | Type | Meaning |
|---|---|---|
| `id` | string | The Anthropic id for the answer. |
| `model` | string | The model that served it. |
| `stop` | string | Why it stopped. |
| `blocks` | object | How many blocks of each kind it held. |
| `tools` | list of strings | The tools it called, by name. |
| `error` | object | An error, as its `type` and `message`. |
