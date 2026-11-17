# Troubleshooting: sessions in a pool

A pool keeps its sessions on its own accounts, and refuses rather than let work cross a fence.
This page covers what `claude` and the router refuse for a pool, and what to do. For what pools
are, see [Pools](../pools.md).

## `claude` starts nothing in a folder

`claude` can't tell which pool the folder is in, or the pool can't take a session. It says why,
ending `— not starting claude`.

| If it says | Then |
|---|---|
| `no pool covers ~/Code/other` | Start it inside a fence, or leave one pool unfenced. |
| `~/Code/both is in pools Acme and Client, as its links lead` | Start it in a folder that only one pool holds. |
| `~/wt/x is in pool Acme, but in its repository it's at ~/Code/web, in pool Default` | Make the worktree in the same pool as its repository. |
| `~/Code/client/site is in pool Client, but the session it's started within is in pool Acme` | Start it outside the fence of Client, or from a session in Client. |
| `pool Acme has no accounts` | Add one with `switchboard accounts add "Acme 1" --pool q7mt`. |
| `pool Acme has no account with a usable token` | Give one a token with `switchboard accounts token m2kp`. |
| `~/Code/acme is in pool Acme, where claude never starts on its own login` | Start it through the router, or outside the fence. |
| `/tmp/x is in pool Acme, through the session it's started within, where claude never starts on its own login` | Start it through the router. A `claude` that a session of Acme starts outside every fence is in Acme. |
| `…: add that bare repository to pool Acme's directories` | The worktree's repository is a bare one outside every fence. Add it to the pool's folders, as below. |
| `can't tell which pool the directory claude starts in is in` | The folder can't be read, for example because it was removed. Start it in a folder that's there. |
| `can't tell which pool ~/wt/x is in: its .git file at ~/wt/x leads to no repository that can be found` | The worktree's `.git` file is stale. Run `git worktree repair <worktree>` in its repository, or remove the worktree. |
| `can't tell which pool ~/wt/x is in: its worktree at ~/wt/x is a submodule's or a separate git directory's, which can't be placed` | Switchboard can't place either. Work in a checkout instead. |
| `can't tell which pool ~/Code/x is in: pool Acme's directory "…" can't be placed` | The config writes a pool folder that can't be placed. Put the folder right in the config. |
| `can't tell which pool ~/Code/x is in: neither the config, the router nor the accounts record names any pools` | Nothing names any pools, so any folder may be fenced. Put the pools back in the config. |
| `the router routes no account of pool Acme yet: any the config gives it waits for the router to restart` | Wait for the router to restart itself, or run `switchboard service restart`. |

`no pool covers` means every pool is fenced, and none holds the folder. To leave your first pool
unfenced, run `switchboard pool edit d3wn --unfence`. A git worktree belongs in the same fence as its
repository, or outside every fence with it. The own-login refusal comes from `--direct`, or from a
start without switchboard, inside a fence. Without switchboard, the message starts with why.

More on the rows above:

- **The worktree row** fits a worktree nested inside another worktree of a fenced repository
  too, saying `lies in pool Acme, through the worktree ~/wt`. If every pool is fenced, it says `in no pool`.
- **The `.git` file is stale** if it can't be read, names a git directory that's gone, or its
  `commondir` can't be read. A stale `.git` file refuses every folder below it.
- **The own-login row,** for a worktree of a repository inside a fence, says
  `in its repository, ~/Code/acme-feature is at ~/Code/acme`.
- **The session rows** come from a `claude` started inside another session, for example by a test
  the session runs. Inside a session, the session decides the pool, unless a fence holds the
  folder. See [`switchboard run`](../commands/run.md).
- **The last row** comes when the config names no pools, so the router's pools are taken.

For a bare repository, give `--dirs` the folders the pool has and the bare repository together,
because `--dirs` replaces them:

```bash
switchboard pool edit q7mt --dirs ~/Code/acme \
  --dirs ~/repos/app.git
```

`--account` with an account of another pool fails, and names the pool each is in:
`Acme 1 (m2kp) is in pool Acme; this directory is in pool Default`. If no fence holds the folder,
and the session that started `claude` puts it in a fenced pool, the message ends `, through the
session it's started within`.

## `claude` says the router doesn't route by the pools

The message is `the router doesn't route by the config's pools (…)`, or
`the router routes by other pools (…)`. The router can't take up the pools in the config.

```bash
switchboard service restart
```

With two pools or more, `claude` counts the router as down if its pools differ from the config.
That's because a router from before pools would spread a session across every account. `claude`
first gives the router 5 seconds to take up a change. So a router still refused is one from before
pools, or one that routes no account of a pool yet. Until it restarts, `claude` stops, or goes
direct on the pool's accounts, as `when_router_down` says.

## A request answered `no account in pool Client can take the request`

None of the accounts in the pool can take it, and nothing falls back to another pool. What follows
the `;` says why for each account, for example `account m4zq: …`.

| If an account says | Then |
|---|---|
| `taken up when the router restarts` | Wait, or run `switchboard service restart`. |
| A problem, such as a token | Give the account what it lacks. |
| A limit | Wait for a window to reset. Nothing falls back to another pool. |

An account taken up when the router restarts was added since the router started. The router
restarts itself to take it up once no request is in flight. `switchboard service restart` does it
now.

## A session says its account has left the pool

The message is `this session's account has left pool Client — exit and resume it to carry on in
the pool its directory is in now`. The account the session started on isn't in that pool any
more. For example, the primary moved to another pool, or the pool was removed
(`this session's pool isn't configured`). Claude Code can't change the token while it runs.
Until the session exits, what never reaches the router, such as fast mode, still goes out on that
token.

```bash
# In the same folder: it starts in the pool
# that folder is in now
claude --resume
```

## A client session in a folder you just fenced uses your accounts

A session keeps the pool it started in. One running when `pool add` or `pool edit` made the fence
keeps the old pool. Exit each session running there and resume it.

## The config lost its pools

Once the record has moved to pools, the router gives a config with no pools its pools back, from
the accounts record. It routes by them meanwhile, so no session crosses a fence.

| If | Then |
|---|---|
| The router can't write them back | Give the new account a pool, or put the pools back yourself. |
| The accounts record can't be read either | Put the pools back in the config. |

The router can't write them back if, for example, every pool is fenced and an account added since
has no pool to go to. The log says why once, and the router tries again each minute.

If the accounts record can't be read either, and the router never routed within a pool, it refuses
every request, saying why. It reports itself unhealthy. `claude` still goes through it, rather
than go direct past the fences.

## The move to pools waits, `awaiting restart`

The first time the router starts on a version with pools, it writes the first pool into the config,
then takes it up. If it can't read the config at that point, the move waits. Put the config right,
as `switchboard status` says. The move never waits on the API.

## Related

- [Pool commands](pool-commands.md): what the commands and the config refuse for a pool.
- [`switchboard pool`](../commands/pool.md): making and changing pools.
- [Pools](../pools.md): what a pool does, and the fences.
