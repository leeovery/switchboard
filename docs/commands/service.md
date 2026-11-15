# `switchboard service`: keep the router running

`switchboard service` manages the LaunchAgent that keeps the router running. launchd starts the
router now, at every login, and again whenever it stops. `setup` installs the service for you, so
you need this command mostly to restart the router or to check on it.

```bash
switchboard service status
```

## Usage and flags

```bash
switchboard service install [--log-level <level>]
switchboard service restart
switchboard service status
switchboard service uninstall
```

| Flag | Default | What it does |
|---|---|---|
| `--log-level <level>` | `$SWITCHBOARD_LOG_LEVEL`, or else `info` | `install`: the router logs at `debug`, `info`, `warn` or `error` and above. |

| Subcommand | What it does |
|---|---|
| `install` | Writes the LaunchAgent, has launchd load it, and waits up to 5 seconds for the router to answer. |
| `restart` | Restarts the router, which reads the config and the tokens afresh. |
| `status` | Shows whether the service is installed and loaded, and whether the router is healthy. |
| `uninstall` | Stops the router and removes the LaunchAgent, the copy of switchboard it runs, and the notifications app. |

If the router doesn't answer in time, `install` and `restart` fail, and leave the LaunchAgent in
place. They point you to `switchboard logs router` and `launchd.log` for why.

## `install`: write the LaunchAgent

`install` writes `~/Library/LaunchAgents/io.github.leeovery.switchboard.plist`. launchd loads it in
place of any copy loaded before, once the router of that copy has finished its requests in flight.
The LaunchAgent marks the router as an interactive job, so launchd doesn't throttle it the way it
can a job of no stated kind. Every Claude Code request waits on the router.

- **It checks the config first.** `install` checks what the router would start from, the config
  above all, and the state directory its control socket goes in. If the router couldn't start,
  `install` fails before it touches the router running.
- **It warns if no account has a usable token.** The router starts all the same, and routes as
  soon as a token file holds one.
- **Once it has begun, nothing stops it,** `ctrl+c` included. That way the router is never left with
  nothing to start it. A `ctrl+c` says `install` is finishing first, which can take a minute or two
  while the old router finishes its requests.

### The copy of switchboard the router runs

The LaunchAgent runs a copy of switchboard of its own, which `install` makes from the switchboard
you ran, following its links. The copy lives at `$XDG_DATA_HOME/switchboard/router/switchboard`, or
else `~/.local/share/switchboard/router/switchboard`.

macOS keeps what you allow the router, such as a folder it writes into, against the path of the
file it runs. Homebrew puts each version at a path of its own, so the copy keeps one path that
never changes. The LaunchAgent also passes the path you ran switchboard by, with `serve --from`, so
a Homebrew link stays the link that `brew upgrade` moves on. A temporary build, such as one from
`go run`, is refused.

### The environment the router gets

The LaunchAgent carries `--config`, and these variables if they're set:

- `XDG_CONFIG_HOME`, `XDG_STATE_HOME` and `XDG_DATA_HOME`;
- `SWITCHBOARD_CONFIG` and `SWITCHBOARD_LOG_LEVEL`;
- `CLAUDE_CONFIG_DIR`.

`install` makes `--config`, `SWITCHBOARD_CONFIG` and `CLAUDE_CONFIG_DIR` absolute, so the router
finds what the CLI does. The tokens are files, so the router needs nothing else from your
environment.

- **Variables you added by hand** to the LaunchAgent, such as `HTTPS_PROXY`, stay, even ones added
  with `defaults write`. If `install` can't read the LaunchAgent in place, they're lost, and
  `install` says so.
- **A `HOME` it sets is dropped,** because launchd gives the router yours.
- **`install` waits for the router** where the environment it writes keeps its state, and nowhere
  else.

### Notifications

In a release, the router writes out the Switchboard app as it starts. The first time, macOS asks
whether `Switchboard` may send notifications. A switchboard built from source has no app, and macOS
asks nothing. See [Notifications](../notifications.md).

### LaunchAgents from older versions

launchd reads the LaunchAgent only as it loads it. So a LaunchAgent written by an older switchboard
keeps what it said through upgrades and restarts in place. `service install` writes it afresh, and
restarts the router to load it.

A LaunchAgent written before the router had its own copy is the exception. After the upgrade that
brings the copy, the router moves over to it by itself as it starts. It writes the LaunchAgent
afresh to run the copy, and launchd runs the copy from the next login on. If the router can't move
over, `service status` says so and names `service install`. See [serve](serve.md) for how the router
restarts in place.

## `restart`: restart the router

`restart` restarts the router, which reads the config and the tokens afresh. When the router
restarts, `restart` says so, then waits up to 50 seconds for the new router to answer:

```text
the router is finishing its requests in flight, then it restarts in place
```

What happens depends on the router:

| The router | What `restart` does |
|---|---|
| Answering | Asks the router to restart. The router finishes its requests in flight, for up to 30 seconds, then gives those it cuts off 5 more to unwind. Then it restarts in place. |
| Answering, but can't replace itself | Asks the router to restart, and launchd starts it again. It says `…, then launchd starts it again`. |
| Run by hand | The router refuses, and `restart` says why. |
| Answering, with a config that isn't valid | The router refuses, because it couldn't start again from that config. After an upgrade, the new version judges the config instead, so a config only the new version takes doesn't hold the restart. |
| Answering, with a new binary that refuses to start | The router asks the binary it would become first. If that refuses, as a newer version can refuse a config the running one takes, the router refuses too, in the words of the binary. |
| Answering, mid-upgrade | If the binary is missing for a moment, or the config or the binary changes during the check, the router refuses. `restart` names what changed and says to try again. |
| Answering, with an upgrade it can't copy in | If the router can't copy the upgrade beside its own copy, as on a full disk, it refuses, and `restart` says why. |
| Answering, mid-export | The router never restarts in place over an export on a schedule, so it refuses: `an export into <dir> is under way: try again once it ends`. |
| From before routers restarted in place | The router stops as if signalled, and launchd starts it again. |
| Not answering | launchd starts the service afresh at once. |

`restart` asks the router at two places: where the LaunchAgent keeps its state, and where your
shell does. It starts the service afresh only when neither answers. The first router to answer
after that, at either place, is the one launchd started.

### A restart that's due

When the router has a restart due, such as after you add an account, it waits for a moment with no
request in flight and no export running. `switchboard status` says a restart is due, and `switchboard service restart` has it now.

| While | `switchboard status` shows |
|---|---|
| An export is under way | The export the restart waits for. `service restart` is refused until it ends. |
| The binary the router would become refuses to start | The refusal. Put the config right as it says. |

## `status`: check the service

`status` shows how the service stands:

- whether the LaunchAgent is installed and loaded;
- whether launchd runs a router, with its pid if `launchctl print` names one, or that its state is
  unread if it names neither;
- the health of the router where the LaunchAgent keeps its state, or else where your shell keeps
  it, saying which.

It also warns about the copy of switchboard the router runs:

| If | `status` says |
|---|---|
| The copy is missing, so launchd can't run the router | The copy is missing. Run `service install`. |
| The LaunchAgent was written before the copy, and runs switchboard itself | The copy isn't run at all. Run `service install`. |
| launchd loaded the LaunchAgent before the copy existed | launchd runs the copy only from the next login. `service install` has it run the copy now. |

`status` exits 0 whatever it finds, a router that's down included. It exits 1 only if it can't
look.

If `status` or `restart` can't read the plist back, or can't work out from it where the router
keeps its state, they say so. Then they look for the router where your shell keeps its state.
`restart` fails rather than signal a router launchd may not run. That happens if the router can't
be asked to restart, and either only the router at your shell answers, or the plist is missing or
unreadable.

## `uninstall`: remove the service

`uninstall` stops the router, once it has finished its requests in flight. Then it removes:

1. the LaunchAgent;
2. the copy of switchboard the router runs;
3. the notifications app, `Switchboard.app`, after unregistering it from macOS, with the files
   beside it: its sum, its lock, and any folders an interrupted write left.

If another switchboard is writing the app, `uninstall` waits up to 30 seconds, and leaves the app
if that takes longer. If the app can't be unregistered, `uninstall` removes it all the same.
Whatever fails, it says so, and the service is uninstalled regardless.

To take away everything else `setup` put on this Mac too, use [`switchboard uninstall`](uninstall.md).

## Examples

```bash
switchboard service install
switchboard service install --log-level debug
switchboard service restart
switchboard service status
```

## Related

- [Setup](setup.md): installs the service as one of its steps.
- [serve](serve.md): the router the service runs, and how it restarts in place.
- [Troubleshooting](../troubleshooting.md): when the router won't start or answer.
