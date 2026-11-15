# Troubleshooting: sessions in a pool

A pool keeps its sessions on its own accounts, and refuses rather than let work cross a fence.
This page covers what `claude` and the router refuse for a pool, and what to do. For what pools
are, see [Pools](../pools.md).

## `claude` starts nothing in a folder

`claude` can't tell which pool the folder is in, or the pool can't take a session. It says why,
ending `— not starting claude`.

| If it says | Then |
|---|---|
| `no pool covers ~/Code/other` | Every pool is fenced, and none holds the folder. Start it inside a fence, or leave one pool unfenced with `switchboard pool edit <id> --unfence`. |
| `~/Code/both is in pools Acme and Client, as its links lead` | Start it in a folder that only one pool holds. |
| `~/wt/x is in pool Acme, but in its repository it's at ~/Code/web, in pool Default` | Make the git worktree outside every fence, or inside the fence of its repository. |
| `pool Acme has no accounts` | Add one with `switchboard accounts add <label> --pool <id>`. |
| `pool Acme has no account with a usable token` | Give one a token with `switchboard accounts token <id>`. |
| `~/Code/acme is in pool Acme, where claude never starts on its own login` | You asked for `--direct` inside a fence. Start it through the router, or outside the fence. |

Every message, with what to do, is in [`switchboard claude`](../commands/claude.md).

## `claude` says the router doesn't route by the pools

The message is `the router doesn't route by the config's pools (…)`, or
`the router routes by other pools (…)`. The router can't take up the pools in the config.

```bash
switchboard service restart
```

With two pools or more, `claude` counts the router as down if its pools differ from the config. A
router from before pools would spread a session across every account. `claude` waits 5 seconds
for the router to take a change up first. So this is a router from before pools, or one that routes
no account of a pool yet. Until it restarts, `claude` stops, or goes direct on the accounts of the
pool, as `when_router_down` says.

## A request answered `no account in pool Client can take the request`

None of the accounts in the pool can take it, and nothing falls back to another pool. What follows
the `;` says why for each account, as `account m4zq: …`.

| If an account says | Then |
|---|---|
| `taken up when the router restarts` | It was added since the router started. Run `switchboard service restart`. |
| A problem, such as a token | Give the account what it lacks. |
| A limit | Wait for a window to reset, as with one pool whose accounts are all out. |

## A session says its account has left the pool

The message is `this session's account has left pool Client — exit and resume it to carry on in
the pool its directory is in now`. The account the session started on isn't in that pool any
more. For example, the primary moved to another pool, or the pool was removed
(`this session's pool isn't configured`). Claude Code can't change the token while it runs.

```bash
claude --resume    # in the same folder: it starts in the pool of that folder now, on its primary
```

## A client session in a folder you just fenced uses your accounts

A session keeps the pool it started in. One running when `pool add` or `pool edit` made the fence
keeps the old pool. Exit each session running there and resume it.

## The config lost its pools

Once the history has moved to pools, the router gives a config with no pools its pools back, from
the accounts record. It routes by them meanwhile, so no session crosses a fence.

| If | Then |
|---|---|
| The router can't write them back, for example because an account added since has no pool to go to | The log says why once, and the router tries again each minute. Give that account a pool, or put the pools back in the config yourself. |
| The accounts record can't be read either, and the router never routed within a pool | The router refuses every request, saying why, and reports itself unhealthy. `claude` still goes through it rather than go direct past the fences. Put the pools back in the config. |

## The move to pools waits, `awaiting restart`

The first time the router starts on a version with pools, it writes the first pool into the config,
then takes it up. If it can't read the config at that point, the move waits. Put the config right,
as `switchboard status` says. The move never waits on the API.

## Related

- [Pool commands](pool-commands.md): what the commands and the config refuse for a pool.
- [`switchboard pool`](../commands/pool.md): making and changing pools.
- [Pools](../pools.md): what a pool does, and the fences.
