# `claude`: start Claude Code through the router

Once `setup` has made the link, the `claude` you type is switchboard. It starts the real Claude
Code, pointed at the router, and hands it every argument you give. So `claude --help` is the help
of Claude Code.

```bash
claude              # routed
claude --resume     # the session finds its account again, in its pool
claude doctor       # a local subcommand: straight to Claude Code
```

`claude` takes no flags of its own. Every argument goes to Claude Code.

## How it finds the real Claude Code

The link is named `claude`, in the bin directory of switchboard. The shell file of switchboard puts
that directory first on `PATH`, ahead of the real `claude`.

The real `claude` is the first `claude` on `PATH` that doesn't start switchboard again, or else the
one where its installers put it. Switchboard passes over its own link, any other build of
switchboard, and a wrapper named `claude` that `exec`s switchboard.

## How it starts

How `claude` starts depends on the router and on your environment:

| If | `claude` |
|---|---|
| The router answers and says where it listens, healthy or not | Starts Claude Code routed through it. |
| The router isn't running, doesn't answer, or doesn't say where it listens | Starts nothing and exits with status 1, unless you've chosen to go direct. |
| The router is down and `when_router_down = "direct"` | Starts Claude Code connected straight to the API. |
| The config can't be read, but the router answers | Starts Claude Code routed, on the accounts the router names. |
| switchboard can't take part at all | Starts Claude Code as if switchboard weren't there. |
| `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set | Starts Claude Code as if switchboard weren't there. |
| The first argument is a local subcommand | Hands it straight to Claude Code. |

Each case below gives the line `claude` writes on stderr.

### Routed

Claude Code starts with `ANTHROPIC_BASE_URL` pointing at the router, and the router chooses an
account for each request. Claude Code gets the token of the primary. If that token isn't usable, it
gets the token of the first account with a usable one.

Claude Code also gets `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`, unless your environment sets it
already. With it, Claude Code tells the router what it tells the API about each request, such as
the prompt the request serves. The router keeps that in the request ledger.

An unhealthy router is routed through all the same. Its failures are upstream, so a direct
connection would meet them too:

```text
switchboard: the router is unhealthy (…) — starting claude through it all the same
```

### Stopped

When the router is down, `claude` starts nothing, exits with status 1, and says why and how to go
on:

```text
switchboard: the router isn't running — not starting claude: see to it with switchboard service status, or go direct this once with SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude
```

`claude` waits at most 5 seconds for a slow router, from the first ask. For the first 2 of those
seconds, it asks again if the router isn't running, because the router may be restarting.

### Direct

With `when_router_down = "direct"` in the config, or `SWITCHBOARD_WHEN_ROUTER_DOWN=direct` for one
launch, Claude Code connects straight to the API when the router is down.

```text
switchboard: the router isn't running — connecting directly on b6s7 · Work
```

- **The token** is the one of `--account`, or else the primary, or else the first account with a
  usable token.
- **A direct session isn't routed or recorded,** and it can spend the reserve of the primary.
- **`SWITCHBOARD_WHEN_ROUTER_DOWN=stop`** stops one launch, whatever the config says. Any other
  value stops it too, and says so.
- **A router that answers and says where it listens** routes, whatever either setting says.

`--account` here comes from [`switchboard run`](run.md), which `claude` runs.

### Without the config

If switchboard can't read its config, for example because the rules of an upgrade refuse it,
`claude` is still routed while the router answers:

```text
switchboard: couldn't read the config (…) — starting claude through the running router
```

Claude Code gets the token of the primary, as the router names it. If that isn't usable, it gets
the token of `--account`, or else of the first account with a usable token. An unhealthy router
adds the second line from Routed.

The router may answer but name no account with a usable token. Then the line says so, so you
don't take it for the router being down: `— the running router names no account with a usable
token —`.

### Without switchboard

When switchboard can't take part at all, `claude` starts as if switchboard weren't there:

```text
switchboard: couldn't read the config (…) — starting claude without it
```

That happens in these cases:

- the config can't be read, and the router doesn't answer;
- no account has a usable token;
- the config is read but the state directory of the router can't be found, and `claude` would go
  direct.

### With an API key

If `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set, Claude Code may use that key in place of
the token of an account. Its requests would go unrouted, billed to the key. So `claude` starts as
if switchboard weren't there:

```text
switchboard: ANTHROPIC_API_KEY is set, so Claude Code uses it — starting claude without switchboard
```

### Within a session

A `claude` started inside a routed session, for example by a script, inherits what switchboard set
there.

| If the inner `claude` | It |
|---|---|
| Has an API key | Leaves out what switchboard set: the address of the router, the custom headers of switchboard, and the hint setting. |
| Starts without switchboard | Keeps the routing of the session, because the router of the session still runs. The line on stderr ends `— starting claude through this session's router`. |
| Starts without switchboard, but the script points it at a gateway of its own | Leaves out what switchboard set. |
| Starts without switchboard, after finding the router down, going direct with no usable token, or with no config | Leaves out what switchboard set. |

What you set yourself stays, your own custom headers among them. So does anything the script
changed since.

### Local subcommands

These go straight to Claude Code, untouched and without a word, whether the router runs or not:

- `setup-token`, `update`, `upgrade`, `install` and `doctor`;
- `mcp`, `plugin`, `plugins`, `auth`, `import`, `project`, `auto-mode` and `gateway`;
- `--version`, `-v`, `--help` and `-h`.

Only the first argument counts, so `claude -p doctor` is a prompt.

## In a pool

Once the config has pools, a session is in the pool of the folder `claude` starts in. It keeps that
pool for its whole life. A resumed session gets the pool its folder is in at the time. See
[Pools](../pools.md).

- **Which pool:** the fenced pool whose folders hold the folder, as given or as its links lead, or
  else the unfenced pool.
- **In a git worktree,** it's the pool of the same place in the main checkout of the repository,
  wherever the worktree lives. If the worktree itself lies in a fence, the two must agree.
- **In a worktree of a bare repository,** it's the pool where the bare repository lies.
- **A submodule or clone inside a worktree** goes by the worktree.

Inside a pool, the rules of the sections above narrow to the pool:

| Case | In a pool |
|---|---|
| Routed | Claude Code starts on the token of the pool primary, or else of the first account in the pool with a usable token. The router is told the pool in a header, and `SWITCHBOARD_POOL` holds its id for the skill. |
| `--account` | Must name an account of the pool. |
| Direct | Goes out only on accounts of the pool: the one of `--account`, or else the primary, or else the first with a usable token. |
| Starting on the login of Claude Code | Refused inside a fence, with `--direct` or without switchboard. That login could go out on any account. |

If the config can't be read, `claude` reads the fences from the config file, for its pools only, or
else from the running router.

### A router that doesn't route by your pools

With two pools or more, a router that doesn't route by the pools of the config counts as down:

- a router from before pools;
- one that would pass the headers of switchboard on to the API;
- one whose pools, folders, accounts or primaries differ from the config.

`claude` gives it 5 seconds to take up the pools of the config, because the router checks the
config every 3 seconds. Then it stops, or goes direct, as `when_router_down` says:

```text
switchboard: the router doesn't route by the config's pools (pool Acme's directories aren't the config's) — not starting claude: restart it with switchboard service restart, or go direct this once with SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude
```

The config may name no pools once the history has moved to them. Then the line says what differs,
such as `the router routes by other pools (pool Acme's directories aren't the same)`. A pool whose
accounts were all added since the router started has none it routes until the router restarts.

## When claude won't start in a pool

With pools, `claude` may start nothing, exit with status 1, and say why on stderr, ending
`— not starting claude`:

| It says | Why | What to do |
|---|---|---|
| `no pool covers ~/Code/other` | Every pool is fenced, and none holds the folder. | Start it inside the folders of a pool, or unfence a pool with [`pool edit --unfence`](pool.md). |
| `~/Code/both is in pools Acme and Client, as its links lead` | The folder, or where its links lead, is inside two pools. | Start it where only one pool holds it. |
| `~/wt/x is in pool Acme, but in its repository it's at ~/Code/web, in pool Default` | The folder is in a git worktree inside one fence, but the same place in its repository is in another pool, or in none. | Make the worktree outside every fence, or inside the fence of its repository. |
| `…: add that bare repository to pool Acme's directories` | The worktree belongs to a bare repository outside every fence. | Give `pool edit --dirs` the folders of the pool and the bare repository together, because `--dirs` replaces them. |
| `can't tell which pool the directory claude starts in is in` | The folder can't be read, for example because it was removed, while a pool is fenced. | Start it in a folder that's there. |
| `can't tell which pool ~/wt/x is in: its .git file at ~/wt/x leads to no repository that can be found` | The `.git` file of a worktree can't be read, or names a git directory that's gone. A stale `.git` file refuses every folder below it. | Run `git worktree repair <worktree>` in its repository, or remove the worktree. |
| `can't tell which pool ~/wt/x is in: its worktree at ~/wt/x is a submodule's or a separate git directory's, which can't be placed` | The folder is in the worktree of a submodule or a separate git directory, which switchboard can't place. | Work in a checkout instead. |
| `pool Acme has no accounts` | The pool has none. | Add one with `accounts add <label> --pool <id>`, or move one in with `accounts edit <id> --pool <id>`. |
| `pool Acme has no account with a usable token` | No account in the pool has a token switchboard can use. | Give one a token with [`accounts token`](accounts.md). |
| `~/Code/acme is in pool Acme, where claude never starts on its own login` | `--direct`, or a start without switchboard, inside a fence or a worktree of a repository there. | Start it through the router, or outside the fence. |
| `can't tell which pool ~/Code/x is in: pool Acme's directory "…" can't be placed` | On the login of Claude Code, the config writes a pool folder that can't be placed. | Put the folder right in the config. |
| `can't tell which pool ~/Code/x is in: neither the config, the router nor the accounts record names any pools` | Once the history has moved to pools, nothing names any pools, so any folder may be fenced. | Put the pools back in the config. See [Troubleshooting](../troubleshooting.md). |
| `the router routes no account of pool Acme yet: any the config gives it waits for the router to restart` | The config names no pools, so the pools of the router are taken, and it routes none of the accounts of this pool yet. | Run `switchboard service restart`. |

A worktree nested in the worktree of a fenced repository says `lies in pool Acme, through the
worktree ~/wt` in the third row. Where every pool is fenced, that row says `in no pool`.

For the bare repository row, for example:

```bash
switchboard pool edit q7mt --dirs ~/Code/acme --dirs ~/repos/app.git
```

`--account` with an account of another pool fails, and names the pool of each:
`Acme 1 (k3f9) is in pool Acme; this directory is in pool Default`.

## Going round switchboard

There are two ways to start Claude Code without switchboard:

- `switchboard run --direct` starts Claude Code on its own login, outside every fence.
- The real `claude`, run by its full path.

If the switchboard binary itself breaks, every `claude` breaks with it, until the bin directory of
switchboard comes off `PATH`.

## Launchers for one account

Switchboard defines no launchers for each account. To make one, alias
`switchboard run --account <id> --`:

```bash
alias cxside='switchboard run --account q4tx --'   # a launcher pinned to side
```

## Related

- [run](run.md): the command `claude` runs, with `--account` and `--direct`.
- [Limits and routing](../limits-and-routing.md): how the router picks an account for each request.
- [Configuration](../configuration.md): `when_router_down` and the `[[pool]]` keys.
