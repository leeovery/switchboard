# `switchboard run`: start Claude Code through the router

`run` starts Claude Code through the router. The `claude` that switchboard links runs it, so you
rarely type it yourself. Give the arguments for Claude Code after `--`.

```bash
switchboard run -- --resume
```

```bash
switchboard run [--account <id>] [--direct] \
  [-- <claude args>]
```

| Flag | Default | What it does |
|---|---|---|
| `--account <id>` | None | Pins the session's conversation to account `<id>`. |
| `--direct` | Off | Starts Claude Code on its own login, without the router or a token, for what needs that login. |

How `claude` starts, and what it does when the router is down, is in [`claude`](claude-command.md).

## Pinning with `--account`

`--account` keeps the conversation on that account while the account can take it. Like any pin, it
gives way when the account hits a limit. The account must be configured, have a usable token, and
be in the pool of the folder the session starts in.

The [primary](../glossary.md) is the account whose token Claude Code holds in a pool.

| If | Then Claude Code connects with |
|---|---|
| The router runs, and the primary has a usable token | The primary's token. The router sends the conversation to `<id>`. |
| The router runs, and the primary has no usable token | The token of `<id>`. |
| The router is down, and the session goes direct | The token of `<id>`. |

Requests outside the conversation, such as uploads and artifacts, aren't routed. They pass through
the router on the token Claude Code holds, whichever account the conversation is on.

## Going direct with `--direct`

`--direct` starts Claude Code on whichever account Claude Code itself is signed into. Nothing is
routed or recorded.

| If | Then |
|---|---|
| You also pass `--account` | `run` refuses: the two don't go together. |
| The folder is inside a fenced pool | `run` refuses, because the login may not be an account of the pool. |
| The folder is a git worktree of a repository inside a fenced pool | `run` refuses, for the same reason. |
| `claude` starts inside a session of a fenced pool | `run` refuses, for the same reason. |

## In a pool

A session belongs to the pool of the folder it starts in, for its whole life. In a git worktree, it
belongs to the pool of its repository. If `run` refuses to start in a folder, see
[Troubleshooting: pools](../troubleshooting/pools.md).

## Inside another session

A `claude` that a session starts belongs to the pool of that session. So a test that runs
`claude -p` in the session's scratchpad spends the same pool as the session. A fence still keeps
its own work:

| If the folder it starts in is | Then it belongs to |
|---|---|
| Inside no fence | The pool of the session |
| Inside the fence of the session's own pool | The pool of the session |
| Inside a fence, and the session is in the unfenced pool | The fenced pool |
| Inside a fence, and the session is in another fenced pool | No pool: `run` refuses |

The project follows the same session. If the folder is in a repository, the project is that
repository. Otherwise the project is the project of the session, rather than the folder.

The session is the one whose `claude` started this `claude`, directly or through other programs.
`run` finds it in the environment that `claude` inherits. If a tool cleaned that environment,
`run` looks at the environments of the programs that started `claude`. A session that has ended
counts as none. So does a session that only started the tmux server a `claude` runs in. Inside a
session of a fenced pool, `--direct` is refused.

## Exit status

`run` becomes Claude Code once it has started it, so its exit status is Claude Code's own.
It exits `1` only if switchboard couldn't start Claude Code.

## Examples

```bash
switchboard run -- --resume
# a one-off prompt, pinned to side
switchboard run --account q4tx \
  -- -p "tidy the changelog"
switchboard run --direct
```

## Related

- [`claude`](claude-command.md): what starts Claude Code through switchboard.
- [Pinning and moving sessions](../pinning-and-moving.md): pins, and how they give way.
- [Pools](../pools.md): which pool a folder belongs to.
