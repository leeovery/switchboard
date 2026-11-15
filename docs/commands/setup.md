# `switchboard setup`: set switchboard up, one step at a time

`switchboard setup` walks you through setting switchboard up, full screen, a page for each step.
Each step says what's done already and does only what isn't, so you can run it again at any time.

```bash
switchboard setup
```

When you leave, `setup` prints a line for each step into the scrollback:

```text
  ✓  Accounts             2, all working · primary: work
  ✓  Priming              on, 08:00–23:00
  ✓  Service              installed · router healthy
  !  claude link          created, but it isn't first on your PATH. Add this line to ~/.zshrc:
     if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard
  ✓  Skill                up to date
```

`setup` takes no flags of its own, only the ones every command takes. See
[Commands](../commands.md).

## The steps

| Step | What it does |
|---|---|
| Accounts | Lists your accounts and checks each token. Adds, edits and removes accounts, and makes their tokens. |
| Priming | Asks for your working day, so that the 5-hour windows of your accounts reset one after another. |
| Service | Installs the LaunchAgent that keeps the router running, or puts it right. |
| `claude` link | Puts the `claude` of switchboard first on your `PATH`, and installs the session hook. |
| Skill | Writes the skill that tells Claude about switchboard, or brings it up to date. |
| Done | Shows where each step stands, and what to run next. |

Each page shows what its step found and did, one fact a row. The keys for the page are on its last
row.

## Keys on every page

| Key | What it does |
|---|---|
| `enter` | Moves on once the step is done. There's no going back. On Done, it opens the [dashboard](../dashboard.md). |
| `q` | Leaves `setup`. Run it again to carry on from where things stand. |
| `ctrl+c` | Leaves `setup`, whatever is open. |
| `esc` | Closes a card, or stops typing the day on Priming. |

While a card is open, `q` does nothing. While you type the day on Priming, `q` types a `q`: press
`esc` first.

## What it prints when you leave

However you leave, `setup` prints the step lines once the screen has gone. Then it adds a line that
depends on how you left:

| How you left | The last line |
|---|---|
| `q`, before Done | `run setup again to carry on` |
| A step failed | The error of that step. `setup` exits with status 1. |
| Done | None. |

If the `claude` link isn't done, the line to add by hand is printed whole, on a line of its own,
however narrow the terminal. That way it copies as one line.

If you leave before the token checks answer, the Accounts line counts each kind of token. For
example, `3: 1 working, 1 unchecked, 1 refused`, or `3, all unchecked`.

## Accounts: your accounts and their tokens

The Accounts step shows a table of every account: label, plan, reserve and token. As `setup` opens,
it checks each token with the API, and shows the date its year is up, with the days left. A note
under the table says what an account is missing, and which key sees to it.

| Key | What it does |
|---|---|
| `↑`, `↓` | Choose an account. |
| `t` | Make a new token for the account. |
| `e` | Edit the label, plan, reserve and primary of the account. Once there's a second pool, also its pool. |
| `r` | Remove the account. `setup` asks first. |
| `p` | Make the account the primary of its pool. Sessions in the pool start on it. |

To add an account, choose `+ add an account`. The card asks for a label, then a plan: Pro, Max or
Ultra, with none chosen until you choose. If the config has pools, it asks for a pool too. Then it
makes the token.

- **With no account yet,** the card to add one opens at once.
- **The subscription of the token decides the id,** as [`accounts add`](accounts.md) does. A
  subscription this Mac had before comes back under its old id. If `setup` can't tell, a card asks
  whether the account is new or one this Mac had.
- **Each pool needs a primary.** With two or more accounts in a pool and none marked primary,
  `enter` waits until you mark one.
- **On a first run,** if the primary has no reserve, `enter` asks for one, once. Give a percentage,
  or `none`. `esc` leaves it with none and writes nothing.
- **The edit card offers the plan `not set`** only for an account that has no plan in the config.

The primary of the first pool is the account your browser and the Claude apps are signed in to.
See [Room for the Claude app](../room-for-the-claude-app.md) for what a reserve keeps back.

### Pools on the Accounts step

The add card has a Pool field. It offers every pool by its label, then `new pool`, even with only
one pool, because that's where you make a second. It opens on the pool that `accounts add` uses
without `--pool`: the only pool, or else the unfenced pool, or else the first. A hint says which
sessions use the pool you choose.

With `new pool` chosen, `enter` opens the new pool card. Give the pool a label and its folders,
each an absolute path or one starting `~/`. `+ another directory` adds a folder. With no folders,
the pool is unfenced, which is refused if there's an unfenced pool already.

`setup` checks the pool and its folders against the rules of the config before it makes any token.
It writes a new pool only once the token is saved, so an add you cancel leaves no pool behind. The
first account in a pool is its primary.

Once there's a second pool, the table groups the accounts by pool. Each group sits under its label
and folders, or `everywhere else` for the unfenced pool, and `◆` marks each primary. The edit card
gains a Pool field, which moves the account to another pool or a new one, as
[`accounts edit --pool`](accounts.md) does. The sessions on the account move off it at once. See
[Pools](../pools.md).

### Making a token

`setup` makes each token by running `claude setup-token` itself, which opens a sign-in page in your
browser. The token is checked with the API, then saved. It's good for a year.

| Account | Where the sign-in page opens |
|---|---|
| The first account, or on a later run the primary of the first pool | Your browser, as it is |
| Any other account, with a browser that opens a private window from the command line, such as the Chrome family or Firefox | A private window, so you sign in to that subscription there |
| Any other account, with a browser that can't, such as Safari | Nowhere: `setup` shows the address to open in a private window yourself |

- A token for a subscription that another account has already isn't saved.
- `t` takes only a token for the subscription of the account itself.
- `p` on the token card pastes a token you made with `claude setup-token` instead. A pasted token
  shows as dots, never as the token.

## Priming: your working day

The Priming step asks for your day as `HH:MM-HH:MM`, such as `08:00-23:00`, and shows the day
already set, if any. It writes the day to `[prime]` in the config only if it changed.

| You | What happens |
|---|---|
| Leave it empty, with no day set | Priming stays off. |
| Clear a day that was set | Priming is turned off. |
| Keep the day already set | Nothing is written. |
| Have only Ultra accounts | The step is skipped, and says so: a verified Ultra account has no 5-hour window to start. |

See [Staggered resets](../staggered-resets.md) for what priming does.

## Service: the router at login

The Service step runs as its page opens, and asks nothing. It installs the LaunchAgent that keeps
the router running, as [`service install`](service.md) does, or installs it again when it's out of
date.

| If | `setup` |
|---|---|
| The LaunchAgent isn't installed, and no router answers or runs | Installs it. |
| launchd hasn't loaded it, and no router answers | Installs it again. |
| It runs another switchboard than the one running `setup` | Installs it again. |
| You give `setup` another config file than the one the router serves | Installs it again. |
| Its plist differs from what `service install` would write, as after an upgrade that adds a key | Installs it again, and says what differed. |
| The copy of switchboard that the router runs is missing | Installs it again. |
| launchd still starts switchboard itself, because it loaded the LaunchAgent before the router had a copy | Installs it again. |

Installing again keeps the `--log-level`, the `--config` and every environment variable the
LaunchAgent had, `HOME` included. The environment decides where the router keeps its state, tokens
included. A `--config` you give `setup` takes the place of the kept one only if it names another
file than the one the router serves.

Before it touches a running service, `setup` checks that the router could start from what it would
keep. If it couldn't, and a router answers, `setup` leaves the service as it is and says why. If
none answers, `setup` fails and says how to install the service once you've put it right.

If the router reads the config and the tokens that `setup` writes, it takes up the changes itself.
The cases where the step stops are under When the Service step stops, below.

## The `claude` link: switchboard first on your PATH

The step makes a link named `claude` in the bin directory of switchboard. The link leads to the
switchboard that `setup` was run by. The bin directory is `$XDG_DATA_HOME/switchboard/bin`, or
else `~/.local/share/switchboard/bin`.

Then `setup` writes a file for each shell, `init.zsh`, `init.bash` and `init.fish`, in the shell
directory: `$XDG_DATA_HOME/switchboard/shell`, or else `~/.local/share/switchboard/shell`.

- Each file puts the bin directory first on `PATH`, once, and loads the completions for the
  commands and flags of switchboard.
- The completions for bash are in `completion.bash` beside it. `init.bash` is plain POSIX shell,
  so a `~/.profile` that `sh` reads too can load it.
- Sourcing a file prints nothing, and sourcing it again changes nothing.
- The router writes the files afresh each time it starts, so an edit to one doesn't last.

Then `setup` asks your login shell, `$SHELL`, which `claude` it finds on `PATH`. It starts the
shell the way a new terminal does. The step is done once a `claude` typed there goes through the
link. `q` while it asks stops `setup`.

### The line in your startup file

Until the step is done, `setup` offers once, in a card, to add one line to the end of your shell
startup file. It names the file and shows the line first. `enter` adds it, and `n` leaves it for
you to add by hand. For zsh, the line is:

```bash
if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard
```

| Shell | The startup file |
|---|---|
| zsh | `.zshrc` where zsh says it reads it: `$ZDOTDIR`, or else your home |
| bash | The first of `~/.bash_profile`, `~/.bash_login` and `~/.profile` that exists, which is the one a login bash reads, or else a new `~/.bash_profile` |
| fish | `config.fish` where fish says it reads it, `$__fish_config_dir` |
| Any other | None. `setup` shows the bin directory, as `$HOME/…`, for you to put first on `PATH` by hand. |

`setup` adds the line only once, ever. Everything else switchboard does in your shell is in its own
file, and if that file goes, the line does nothing. A startup file that's a link, such as one into
your dotfiles, is written where the link leads.

`setup` writes nothing, and shows the line to add by hand, in these cases:

- It couldn't ask your shell. It names the file the line usually goes in.
- zsh names a `$ZDOTDIR` with no `.zshrc` in it, while your home has one. `setup` names both files,
  because the `.zshrc` in your home may not be the one a new terminal reads. A `ZDOTDIR` set in
  `.zshenv` is read by a new terminal, and so is one set in `.zprofile`, because macOS terminals
  start login shells. One set in `.zshrc` or later isn't.

### When the shell still runs another claude

| If | `setup` says |
|---|---|
| The line is there, but the shell still runs another `claude` | Something after the line changes `PATH`. Move the line to the end of the file, or move what changes `PATH` before it. |
| The line is last in the file already | Something read after the file, such as `.zlogin`, changes `PATH`. Put the line after it. |
| The line reads the file of another switchboard | It shows the line to put in its place. |
| The shell exits without answering | The startup file may start something that needs a terminal, such as tmux. |
| You declined the line | It shows the line, and the bin directory as `$HOME/…`. `c` copies the line, `d` the directory, and `r` asks your shell again, as often as you like. |

`enter` leaves the step for now. That isn't a failure: `setup` carries on. `c` and `d` copy only if
your terminal lets a program copy (OSC 52). If it doesn't, copy the text from the screen.

### An alias named claude

If your shell has an alias named `claude`, `setup` reads it by a plain rule:

- It splits the alias at spaces and tabs.
- It reads only plain words. A plain word is made of letters, digits and `_./:@%+,=-`, or is a path
  starting with an unquoted `~/`, `$HOME/` or `${HOME}/`.
- It passes over `command`, and leading variables such as `CLAUDE_CONFIG_DIR=~/.claude-work` or
  `PATH=/x:$PATH`.

| The alias | What `setup` makes of it |
|---|---|
| Runs `claude` by name, such as `CLAUDE_CONFIG_DIR=~/.claude-work claude` | `PATH` decides, so it goes through switchboard. |
| Sets `PATH` before `claude`, such as `PATH=… claude` | It can't tell which `claude` that `PATH` finds. It says to take the `PATH` setting out of the alias. |
| Runs a clean path to a `claude`, such as `~/.claude/local/claude` | It goes through switchboard only if the path is the link, with no `PATH` set before it. Otherwise, `setup` says to remove the alias or point it at `claude`. |
| Is all plain words and names no `claude`, such as `npx …` | It runs something else. |
| Anything else, such as `caffeinate -i claude` or `cd ~/work && claude` | A wrapper that `setup` doesn't read. |

A clean path has no trailing `/`, and no `.` or `..` in it. For a function, or a wrapper it doesn't
read, `setup` says the wrapper goes through switchboard if it runs `claude` by name.

### The session hook

Whatever the link step found, `setup` then installs the session hook in the `settings.json` of
Claude Code, or puts it right. Your other settings and hooks stay as they are. The hook makes a
session that didn't start through switchboard say so as it starts. See
[Claude and switchboard](../claude-and-switchboard.md).

If `setup` can't read `settings.json` the way Claude Code does, it leaves the file as it is, says
what's wrong, and carries on.

## Skill: what Claude knows about switchboard

The Skill step writes a Claude Code skill, or brings it up to date. The skill tells Claude what
switchboard does under `claude`, so no session needs it explained. It lives at
`skills/switchboard/SKILL.md` in `~/.claude`, or in `$CLAUDE_CONFIG_DIR`.

Switchboard owns the file. When a new version of switchboard brings a new skill, `setup` replaces
the file, edits and all, and so does the router as it starts. Otherwise they leave the file alone.

## Done: where each step stands

Done shows a line for each step, as it stands, then what to run next. If the `claude` link isn't
done, its line holds the line to add by hand. `enter` opens the [dashboard](../dashboard.md), and
`q` leaves without it.

## Good to know

- **`setup` needs a terminal of at least 95 columns by 25 rows.** On a smaller one, it says to make
  the terminal larger.
- **Without a terminal,** `setup` lists what does each step instead. That's a command for most
  steps, and `setup` itself for the skill and the session hook.
- **A step that fails stops `setup` there.** Its page says why. Put it right, and run `setup`
  again.
- **`setup` refuses a switchboard that won't last,** such as one built by `go run`. The service
  and the link both run the switchboard that `setup` was run by.
- **A pasted token never shows.** `setup` takes a token only where it shows as dots, such as on the
  token card.

## When the Service step stops

`setup` looks for the router at two places: where the plist of the service keeps its state, and
where your shell does. It also asks launchd whether launchd runs the router. In the cases below,
`setup` leaves the service as it is and fails, saying why. It never sends you back to `setup`.

| If | What to do |
|---|---|
| There's no plist it can use, none or one it can't read, and a router answers or runs | Run `switchboard service install`, which writes the plist afresh. |
| launchd has loaded a plist that `setup` can't read | Run `switchboard service install`. A plist that `setup` can't read holds options it can't keep, so you choose. |
| A router run some other way answers | Stop that router first, then run `switchboard service install`. `setup` says what launchd has: the service not loaded, loaded but running no router, or running one, with its pid, that doesn't answer. |
| Only the router at the state of your shell answers | `setup` can't tell which router the service runs. Run `switchboard service install` from the shell the router should use. |
| launchd runs a router that answers at neither place within 5 seconds | It may be starting, stopping, or keeping its state somewhere else. If it doesn't answer soon, as `switchboard service status` shows, run `switchboard service install`, with the `--config` and `--log-level` it ran with, if any. `setup` names the pid if it knows it. |
| The plist sets `HOME` empty, and the state directory or config file sits under it | A router can't start from it again. Run `switchboard service install`. If a router answers that launchd doesn't run, stop it first. |

A router too old to give its pid counts as the one launchd runs only if launchd runs one. If
launchd runs a router but `setup` can't read its pid, or `launchctl print` says neither, the router
that answers counts as the one launchd runs.

### Plists setup can't read

- **A plist that isn't XML,** such as the binary one `defaults write` leaves, is read through
  `plutil`.
- **A plist that can't be read back, or names no program,** is installed afresh from the options
  of `setup` and the environment of your shell. That happens only if the router could start from
  them, launchd hasn't loaded the service, and no router answers.
- **If only the config directory of Claude Code sits under an empty `HOME`,** `setup` says the
  router won't keep the skill up to date, and carries on.

### When setup starts the router

`setup` restarts the router only when none answers and none runs, and only once it has checked the
router could start. If it couldn't, the step fails and says how to install the service. `setup`
never starts the service afresh while any router answers. It installs the service again while a
router answers only if that router answers where the plist keeps its state, with launchd having it
loaded.

A router that `setup` started is judged where it answers. If that's where your shell keeps its
state, not where the plist does, `setup` says so. It says where the router finds things apart from
`setup` only once a router answers.

### When the router and setup look in different places

If the router uses another state directory, config file or Claude Code config directory than
`setup`, `setup` says which one differs, but not where. To line them up:

- Run `switchboard service install` from the shell the router should use.
- For the config file, run `switchboard setup --config` with the file the router reads. Then
  `setup` finds them the same, and changes nothing.

## Related

- [Accounts](accounts.md): add, edit and remove accounts, and replace tokens, outside `setup`.
- [Service](service.md): install, restart and check the router on its own.
- [Uninstall](uninstall.md): take away everything `setup` put on this Mac.
