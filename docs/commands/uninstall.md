# `switchboard uninstall`: take switchboard off this Mac

`switchboard uninstall` takes away what `setup` put on this Mac, so nothing switchboard leaves
keeps working on your behalf. Run it before `brew uninstall switchboard`, because Homebrew has no
way to run it for you.

```bash
switchboard uninstall
```

```text
✓  Service       removed ~/Library/LaunchAgents/io.github.leeovery.switchboard.plist
✓  Startup line  removed from ~/.zshrc
✓  Shell files   removed from ~/.local/share/switchboard/shell
✓  Session hook  removed from ~/.claude/settings.json
✓  Skill         removed ~/.claude/skills/switchboard/SKILL.md
✓  claude link   removed ~/.local/share/switchboard/bin/claude
✓  Data          removed ~/.local/share/switchboard
✓  Kept          the config, the accounts' token files and the history: switchboard uninstall --all removes them too

The accounts' setup tokens stay valid on Anthropic's side until their year is out, their files removed here or not.
Last, brew uninstall switchboard removes switchboard itself.
```

The paths it prints are whole. They're written from `~` here.

| Flag | Default | What it does |
|---|---|---|
| `--all` | off | Also removes the config, the accounts' token files, and your record. |

## What it removes

`uninstall` takes these away, in this order:

1. **The service,** as [`service uninstall`](service.md) does: the router, once it has finished its
   requests in flight, the LaunchAgent, the copy of switchboard the router runs, and the
   notifications app.
2. **The startup line** that `setup` added to your shell startup files, and the shell files that
   the line loads, with the bash completions.
3. **The session hook,** the entry `setup` added to the SessionStart hooks in `settings.json`. Every other setting and hook stays as it is.
4. **The skill,** in the Claude Code config directory.
5. **The `claude` link** in the switchboard bin directory, if it's a link to a `switchboard`.
   Anything else named `claude` there stays, and `uninstall` says so.
6. **The bin directory and the data directory,** `~/.local/share/switchboard`, each only if nothing
   else is in it. If something is left in the data directory, `uninstall` says it's kept.

It never touches the switchboard binary itself, which belongs to Homebrew.

## What stays, and what --all removes

Without `--all`, the config, the token files and everything recorded in the state directory stay.
That's the ledger, the readings, the events, the logs and the router's state. A reinstall
carries on where it left off.

`--all` removes them too: the config directory, with any themes of your own, and the state
directory, wherever the environment puts them. Nothing asks first, because the flag is the asking.

| If | What `--all` does |
|---|---|
| `SWITCHBOARD_CONFIG` or `--config` puts the config file outside the config directory | Removes that file only. |
| `SWITCHBOARD_THEMES_DIR` puts a themes folder outside it | Removes only the `.theme` files, then the folder if nothing else is in it. |
| The config or state directory is a link, such as into your dotfiles | Removes the link only, and leaves what it leads to whole. |
| The export folder is inside a directory it removes, even through a link | Keeps the export folder and the directories on the way to it, and says so. |
| The export folder is the directory itself | Keeps the directory whole. |
| The config can't be read, so it can't tell where the export folder is | Keeps the config and the state directory, and says why. |

`uninstall` never touches the export folder, wherever it is. See
[Backups and a new Mac](../several-macs.md) for exports.

Your accounts' setup tokens stay valid on Anthropic's side until their year is out,
whether or not their files are removed. `uninstall` says so.

## The startup line

`uninstall` finds the line by its `# switchboard` mark, in each of these files where it's present:

| Shell | Files it looks in |
|---|---|
| zsh | `~/.zshrc`, and `.zshrc` where zsh says it reads it |
| bash | `~/.bash_profile`, `~/.bash_login` and `~/.profile` |
| fish | `~/.config/fish/config.fish`, and `config.fish` where fish says it reads it |

It asks a zsh or fish login shell where it reads its file, the way `setup` does. The `ZDOTDIR` of
the terminal you run `uninstall` in, an editor's for example, may not be a new terminal's.

- **If the shell can't be asked,** or `$SHELL` isn't set, `uninstall` says it couldn't look there.
  It doesn't say the line isn't there. A line left behind does nothing once the shell files are gone.
- **An interrupt while it asks** stops `uninstall` there, with only the service gone, and it says
  so. Run it again, with `--all` if you gave it before, to finish.
- **Only the line as `setup` wrote it,** at the start of its line, is taken out. An indented copy
  may be all there is of an `if` block of yours, which the shell couldn't read if it were left
  empty. So `uninstall` leaves it, and names the file for you to edit.
- **Every other line stays as it was.** The file is rewritten in place, where a link to it leads,
  so it stays the same file, with its hard links, owner and permissions.

## When a piece fails

`uninstall` prints a line for each piece, saying it was removed, wasn't there, or failed and why.
The service line says both if the LaunchAgent went but the notifications app couldn't.

- A piece that fails doesn't stop the rest. `uninstall` goes on to the last piece, then fails.
- A `settings.json` that Claude Code couldn't read stays as it is, and `uninstall` says why.
- Run again, `uninstall` says each piece is not there, and succeeds.

## Examples

```bash
# keeps the config, the tokens and your record
switchboard uninstall
# removes those too
switchboard uninstall --all
# last, removes switchboard itself
brew uninstall switchboard
```

## Related

- [Setup](setup.md): what `uninstall` takes away.
- [Service](service.md): remove only the service.
- [Backups and a new Mac](../several-macs.md): keep a copy of your record before `--all`.
