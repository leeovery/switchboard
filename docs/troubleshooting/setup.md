# Troubleshooting: the `setup` screen

`switchboard setup` walks you through your accounts, priming, the service, the `claude` link and
the skill, one step each. This page covers what can go wrong on its screen.

## `setup` says to make the terminal larger

The `setup` screen needs at least 95 columns by 25 rows. On a phone, turn the terminal
sideways, or use a larger terminal.

## `c` on the screen copies nothing

`setup` copies by OSC 52, which some terminals don't allow, or allow only once you turn it on.

```bash
# In tmux, for example
tmux set -g set-clipboard on
```

When `setup` leaves, it prints each step's line into the scrollback, so you can copy from
there. That includes the line to add to your startup file, or the folder to put first on `PATH`.

## A token `setup` made but couldn't save

`setup` made a token with `claude setup-token`, then couldn't read or save it. Once the screen has
gone, `setup` prints the token as `claude setup-token` showed it. The token lasts a year on the
account, so it's worth keeping.

```bash
# Then press p on the token card, and paste
# the token
switchboard setup
```

## `setup` carries on without the session hook

`setup` adds the session hook to Claude Code's `settings.json`, and leaves the file as it is
if it can't read it. It says why, then carries on.

| If `setup` says | Then |
|---|---|
| The file doesn't read, at a line and column | Put the file right there, then run `switchboard setup` again. |
| The file starts with a byte order mark | Save the file without one, then run `switchboard setup` again. |
| The file kept changing | Claude Code was writing it at the same time. Run `switchboard setup` again. |
| `switchboard is running as sb here` | Run `setup` as `switchboard`, by the path you installed it at. |

The file doesn't read if it isn't a JSON object, if `hooks` isn't an object, or if
`hooks.SessionStart` isn't an array. `switchboard is running as sb here` means you ran switchboard
by another name, such as a link of your own.

## The Service step installs the service again

The Service step installs the LaunchAgent again if it's out of date:

| If | `setup` |
|---|---|
| launchd hasn't loaded it, and no router answers | Installs it again. |
| It runs another switchboard than `setup` | Installs it again. |
| You give `setup` another config file | Installs it again. |
| Its plist differs from a fresh one | Installs it again, saying what differed. |
| The router's copy of switchboard is missing | Installs it again. |
| launchd still runs switchboard itself | Installs it again. |

A plist differs after an upgrade that adds a key. The LaunchAgent runs switchboard itself if
launchd loaded it before the router had a copy of its own.

Installing again keeps every environment variable the LaunchAgent had, `HOME` included, because
the environment decides where the router keeps its state. A `--config` you give `setup` replaces
the kept one only if it names another file than the router serves. If the router couldn't start
from what `setup` would keep, `setup` leaves a router that answers as it is, and says why. With no
router answering, it fails and says how to install the service.

## The Service step stops

`setup` looks for the router in two places: where the service's plist keeps its state, and where
your shell does. It also asks launchd whether it runs the router. In these cases `setup` leaves the
service as it is and fails, saying why:

| If | Then |
|---|---|
| There's no plist `setup` can use, and a router answers or runs | Run `switchboard service install`. |
| launchd has loaded a plist `setup` can't read | Run `switchboard service install`. |
| A router run some other way answers | Stop that router, then run `switchboard service install`. |
| Only the router where your shell keeps its state answers | Run `switchboard service install` from the shell the router should use. |
| launchd runs a router that answers in neither place within 5 seconds | It may be starting, stopping, or keeping its state elsewhere. |
| The plist sets `HOME` empty, with the state directory or config under it | Run `switchboard service install`. |

"No plist `setup` can use" means none, or one it can't read. A plist it can't read holds options it
can't keep, so you choose, by installing afresh. When a router run some other way answers, `setup`
also says what launchd has. That's the service not loaded, loaded with no router, or running one
that doesn't answer, with its pid.

A router that answers in neither place may come back soon, as `switchboard service status` shows.
If it doesn't, run `switchboard service install` with the `--config` and `--log-level` it ran with.
`setup` names the pid if it knows it. An empty `HOME` may leave only Claude Code's config directory
under it. Then `setup` says the router won't keep the skill up to date, and carries on.

A router too old to give its pid counts as launchd's only if launchd runs one. If launchd runs a
router but `setup` can't read its pid, the router that answers counts as launchd's.

## A plist `setup` can't read

- **A binary plist,** such as `defaults write` leaves, is read through `plutil`.
- **A plist that can't be read back, or names no program,** is installed afresh from `setup`'s
  options and your shell's environment. That happens only if the router could start from them,
  launchd hasn't loaded the service, and no router answers.

## When `setup` starts the router

`setup` starts the router only when none answers and none runs, once it has checked the router
could start. It never starts the service afresh while any router answers. It installs the service
again while one answers only if that router answers where the plist keeps its state.

A router `setup` started is judged where it answers. If that's where your shell keeps its state,
not where the plist does, `setup` says so.

## The router and `setup` look in different places

If the router uses another state directory, config file or Claude Code config directory than
`setup`, `setup` says which one differs. To line them up:

- Run `switchboard service install` from the shell the router should use.
- For the config file, run `switchboard setup --config` with the file the router reads.

## Related

- [`switchboard setup`](../commands/setup.md): every page of `setup`, and its keys.
- [`claude` doesn't go through switchboard](claude-link.md): when the `claude` link step finds
  another `claude`.
- [Tokens](tokens.md): an account with an ended subscription, a refused token or the wrong token.
