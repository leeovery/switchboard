# Troubleshooting: `claude` doesn't go through switchboard

When you type `claude`, your shell should run the `claude` link that switchboard puts first on
`PATH`. This page covers the times it runs another `claude`, and how to go round switchboard on
purpose.

## `claude` in a new terminal isn't switchboard's

Your shell finds another `claude` before the link that switchboard installs.

```bash
# Asks your login shell which claude it runs,
# and says what it found
switchboard setup
# Prints the claude your shell runs now
command -v claude
```

| If | Then |
|---|---|
| The `# switchboard` line is there, but another `claude` runs | Move the line to the end of the file. |
| You set `PATH` in a file of your own, or a tool writes it | Put the switchboard bin folder first on `PATH` there. |
| You fixed it while `setup` was open | Press `r` on `setup`'s `claude` link page to check again. |

If the line is in your startup file but another `claude` still runs, something after the line
changes `PATH`. You can also move what changes `PATH` above the line instead. The bin folder is
`~/.local/share/switchboard/bin`, or `$XDG_DATA_HOME/switchboard/bin` if you set `XDG_DATA_HOME`.

## An alias named `claude` runs something else

`setup` reads an alias named `claude` and says whether it goes through switchboard.

| If the alias runs | It goes through switchboard |
|---|---|
| `claude`, `\claude` or `command claude`, after any plain variables such as `FOO=1` | Yes |
| The switchboard link by its path, absolute or from an unquoted `~/`, `$HOME/` or `${HOME}/` | Yes |
| `claude` after a variable that sets `PATH`: `PATH=`, `PATH+=`, or in zsh `path=` and `path+=` | `setup` can't tell. Take the `PATH` setting out of the alias. |
| Another `claude` by its path, or another program | No. Remove the alias, or point it at `claude`. |

`setup` doesn't read a function, or an alias that runs `claude` after another command, such as
`caffeinate -i claude` or `cd ~/work && claude`. It doesn't read an alias with characters beyond
letters, digits and `_./:@%+,=-` either. Each of these goes through switchboard if it runs
`claude` by name.

## A session says it isn't going through switchboard

The session was started by a `claude` that isn't switchboard's. Its requests go straight to
Anthropic on Claude Code's own login, unrouted and unrecorded.

| If you started it | Then |
|---|---|
| From a terminal | Run `switchboard setup`, then start the session again in a new terminal. |
| From an app with a Claude Code of its own | It can't be routed, and `setup` won't change that. |

From a terminal, something put the real `claude` ahead of the link on `PATH`, such as a changed
startup file. The apps with a Claude Code of their own include the Claude desktop app's Code
sessions, and an editor's bundled Claude Code. See [How it works](../how-it-works.md) for what
doesn't go through the router.

## Completions don't work

The switchboard file for your shell loads completions for commands and flags. The switchboard line
in your startup file loads that file. If you declined the line, run `switchboard setup` again. It
offers the line once more only if your shell can take completions, as the table below says, and
has none of switchboard's.

`setup` doesn't offer the line in these cases:

- your shell can't take completions;
- an alias runs the switchboard link while your `PATH` finds another `claude`;
- `setup` can't read your startup file;
- `setup` can't tell which startup file a new terminal reads;
- another switchboard's line is in the file.

If another switchboard's line is there, replace it with this switchboard's line. In the other
cases, add the line yourself. "Which startup file gets the line" below says where it goes.

| Shell | Completions load if |
|---|---|
| zsh | `compinit` has run before the switchboard line. A usual `.zshrc` does. |
| bash | The bash-completion package is loaded, and bash isn't in POSIX mode. |
| fish | Always. |

## Going round switchboard

To start Claude Code on its own login, without the router:

```bash
# Refused inside a fence
switchboard run --direct
```

You can also run the real `claude` by its path. If the switchboard binary itself breaks, every
`claude` breaks with it. Remove the line marked `# switchboard` from your startup file, then open a
new terminal. That takes the switchboard folder off `PATH`.

## Which startup file gets the line

`setup` offers to add one line, marked `# switchboard`, to the end of your shell's startup file:

| Shell | The startup file |
|---|---|
| zsh | `.zshrc` where zsh reads it: `$ZDOTDIR`, or your home |
| bash | The first of `~/.bash_profile`, `~/.bash_login` and `~/.profile` that exists |
| fish | `config.fish` where fish reads it |
| Any other | None |

For bash with none of those files, `setup` makes `~/.bash_profile`. For another shell, it shows
the bin directory to put first on `PATH` yourself. A startup file that's a link, such as one into
your dotfiles, is written where the link leads. `setup` adds the line only once, ever.

`init.bash` is plain POSIX shell, so a `~/.profile` that `sh` reads too can load it. Its
completions are in `completion.bash` beside it.

## `setup` shows the line but doesn't add it

| If | Then |
|---|---|
| `setup` couldn't ask your shell | It names the file the line usually goes in. Add it there. |
| zsh names a `$ZDOTDIR` with no `.zshrc`, and your home has one | `setup` names both files. Add the line to the one a new terminal reads. |

A `ZDOTDIR` set in `.zshenv` or `.zprofile` is the one a new macOS terminal reads, because it starts
a login shell. One set in `.zshrc` or later isn't.

## `setup` says something else changes `PATH`

| If `setup` says | Then |
|---|---|
| The line is last, but something read after the file changes `PATH` | Something such as `.zlogin` runs later. Put the line there, after what changes `PATH`. |
| The line reads another switchboard's file | Put the line `setup` shows in its place. |
| Your shell exited without answering | The startup file may start something that needs a terminal, such as tmux. |

## An alias that runs a path

An alias that runs a `claude` by its path goes through switchboard only if the path is the link,
with no `PATH` set before it. The path must be clean, with no trailing `/` and no `.` or `..` in
it. For example, Claude Code's old `~/.claude/local/claude` doesn't go through switchboard.

## Related

- [`switchboard setup`](../commands/setup.md): every page of `setup`, the `claude` link among them.
- [`switchboard claude`](../commands/claude-command.md): what the link does when it starts a
  session.
- [Setup](setup.md): when the `setup` screen itself goes wrong.
