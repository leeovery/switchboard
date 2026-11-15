# `switchboard run`

`run` starts Claude Code through the router. The `claude` that switchboard links runs it, so you
rarely type it yourself. Give the arguments for Claude Code after `--`.

```bash
switchboard run -- --resume
```

```bash
switchboard run [--account <id>] [--direct] [-- <claude args>]
```

| Flag | Default | What it does |
|---|---|---|
| `--account <id>` | None | Pins the conversation of the session to account `<id>`. |
| `--direct` | Off | Starts Claude Code on its own login, without the router or a token, for what needs that login. |

How `claude` starts, and what it does when the router is down, is in [`claude`](claude.md).

## Pinning with `--account`

`--account` keeps the conversation on that account while the account can take it. Like any pin, it
gives way when the account hits a limit. The account must be configured, have a usable token, and
be in the pool of the folder the session starts in.

| If | Then Claude Code connects with |
|---|---|
| The router runs, and the primary has a usable token | The token of the primary. The router sends the conversation to `<id>`. |
| The router runs, and the primary has no usable token | The token of `<id>`. |
| The router is down, and the session goes direct | The token of `<id>`. |

Uploads and artifacts don't go through the router, so they go out on the token Claude Code holds.

## Going direct with `--direct`

`--direct` starts Claude Code on whichever account Claude Code itself is signed into. Nothing is
routed or recorded.

| If | Then |
|---|---|
| You also pass `--account` | `run` refuses: the two don't go together. |
| The folder is inside a fenced pool | `run` refuses, because the login may not be an account of the pool. |
| The folder is a git worktree of a repository inside a fenced pool | `run` refuses, for the same reason. |

## In a pool

A session belongs to the pool of the folder it starts in, for its whole life. In a git worktree, it
belongs to the pool of its repository. If `run` refuses to start in a folder, see
[`claude`](claude.md).

## Exit status

`run` becomes Claude Code once it has started it, so its exit status is the status of Claude Code.
It exits `1` only if switchboard couldn't start Claude Code.

## Examples

```bash
switchboard run -- --resume
switchboard run --account q4tx -- -p "tidy the changelog"
switchboard run --direct
```

## Related

- [`claude`](claude.md): what starts Claude Code through switchboard.
- [Pinning and moving sessions](../pinning-and-moving.md): pins, and how they give way.
- [Pools](../pools.md): which pool a folder belongs to.
