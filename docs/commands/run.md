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

A `claude` that a session starts belongs to the same pool as the session. So a test that runs
`claude -p` in the session's scratchpad spends accounts in that pool. A fence still keeps its own
work:

| If the folder it starts in is | Then it belongs to |
|---|---|
| Inside no fence | The same pool as the session |
| Inside the fence of the pool the session is in | The same pool as the session |
| Inside a fence, and the session is in the unfenced pool | The fenced pool, as if no session started it |
| Inside a fence, and the session is in another fenced pool | No pool: `run` refuses |

The project follows the pool:

| If the folder it starts in is | Then its project is |
|---|---|
| In a repository | That repository |
| In no repository, and in the same pool as the session | The same project as the session |
| In no repository, and a fence takes it into another pool | The folder itself |

The session is the one whose `claude` started this `claude`, directly or through other programs.
`run` looks for that `claude` among the programs that started this one, nearest first. It reads
the environment that `claude` started with, which the programs the session runs can't change.
Claude Code under Node writes its title over the memory `run` reads, though. If `run` finds no
session in what it reads there, it reads the environment this `claude` inherits instead.

No session is found if:

- The session has ended.
- The session only started the tmux server a `claude` runs in.
- The `claude` started in the background from a shell that exits at once, for example with
  `claude -p … &`. The `claude` may pass to launchd before `run` looks, and is then placed by its
  folder.

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
