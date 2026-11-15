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

## Related

- [`switchboard setup`](../commands/setup.md): every page of `setup`, and its keys.
- [`claude` doesn't go through switchboard](claude-link.md): when the `claude` link step finds
  another `claude`.
- [Tokens](tokens.md): an account with the wrong token.
