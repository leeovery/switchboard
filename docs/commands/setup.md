# `switchboard setup`: set switchboard up, one step at a time

`switchboard setup` walks you through setting switchboard up, full screen, a page for each step.
Each step says what's done already and does only what isn't, so you can run it again at any time.

```bash
switchboard setup
```

When you leave, `setup` prints a line for each step into the scrollback:

```text
  ✓  Accounts             3, all working · primary: personal
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
| Accounts | Lists your accounts and checks each token. Adds, edits and removes accounts. |
| Priming | Asks for your working day, so your accounts' 5-hour windows reset one after another. |
| Service | Installs the LaunchAgent that keeps the router running. |
| `claude` link | Puts switchboard's `claude` first on your `PATH`, and installs the session hook. |
| Skill | Writes the skill that tells Claude about switchboard. |
| Done | Shows where each step stands, and what to run next. |

Each page shows what its step found and did, one fact a row. Its keys are on the last row.

## Keys on every page

| Key | What it does |
|---|---|
| `enter` | Moves on once the step is done. There's no going back. |
| `q` | Leaves `setup`. Run it again to carry on. |
| `ctrl+c` | Leaves `setup`, whatever is open. |
| `esc` | Closes a card, or stops typing the day on Priming. |

While a card is open, `q` does nothing. While you type the day on Priming, `q` types a `q`.

## What it prints when you leave

However you leave, `setup` prints the step lines once the screen has gone. Then:

| How you left | The last line |
|---|---|
| `q`, before Done | `run setup again to carry on` |
| A step failed | The error of that step. `setup` exits with status 1. |
| Done | None. |

If the `claude` link isn't done, the line to add by hand is printed whole, on a line of its own, so
it copies as one line. If you leave before the token checks answer, the Accounts line counts each
kind, for example `3: 1 working, 1 unchecked, 1 refused`.

## Accounts: your accounts and their tokens

The Accounts step shows a table of every account: label, plan, reserve and token. It checks each
token with the API, and shows the date its year is up. A note under the table says what an account
is missing, and which key sees to it.

| Key | What it does |
|---|---|
| `↑`, `↓` | Choose an account. |
| `t` | Make a new token for the account. |
| `e` | Edit the label, plan, reserve and primary, and with a second pool, the pool. |
| `r` | Remove the account, after asking. |
| `p` | Make the account the [primary](../glossary.md) of its pool. |

To add an account, choose `+ add an account`. The card asks for a label and a plan: Pro, Max or
Ultra. If the config has pools, it asks for a pool too. Then it makes the token. The token's
subscription decides the account's id, as [`accounts add`](accounts.md) says.

- **With no account yet,** the add card opens at once.
- **Each pool needs a primary.** With two or more accounts in a pool and none marked, `enter`
  waits until you mark one.
- **On a first run,** if the primary has no reserve, `enter` asks for one, once. Give a percentage,
  or `none`. `esc` leaves it with none.

Sign your browser into the first pool's primary, because artifacts open in a browser signed into
it. See [Room for the Claude app](../room-for-the-claude-app.md) for what a reserve keeps back.

### Pools on the Accounts step

The add card has a Pool field, with every pool by its label, then `new pool`. It opens on the pool
`accounts add` uses without `--pool`. A hint says which sessions use the pool you choose.

`new pool` opens a card for the pool's label and folders, each an absolute path or one starting
`~/`. With no folders, the pool is unfenced. `setup` checks the pool before it makes any token, and
writes the pool only once the token is saved. So an add you cancel leaves no pool behind.

Once there's a second pool, the table groups the accounts by pool, and `◆` marks each primary. The
edit card gains a Pool field, which moves the account as
[`accounts edit --pool`](accounts.md) does. See [Pools](../pools.md).

### Making a token

`setup` makes each token by running `claude setup-token`, which opens a sign-in page in your
browser. The token is checked with the API, then saved for a year.

| Account | The sign-in page opens in |
|---|---|
| The first pool's primary | Your browser, as it is |
| Any other | A private window, so you sign in to that subscription there |

A private window needs a browser that opens one from the command line, such as the Chrome family
or Firefox. With Safari, `setup` shows the address to open in a private window yourself.

- A token for a subscription another account has already isn't saved.
- `p` on the token card pastes a token you made with `claude setup-token`. It shows as dots.

## Priming: your working day

The Priming step asks for your day as `HH:MM-HH:MM`, such as `08:00-23:00`. It writes the day to
`[prime]` in the config only if it changed.

| You | What happens |
|---|---|
| Leave it empty | Priming stays off. |
| Clear a day that was set | Priming is turned off. |
| Have only Ultra plans | The step is skipped. |

The step is skipped because a verified Ultra account has no 5-hour window to start. See
[Staggered resets](../staggered-resets.md).

## Service: the router at login

The Service step runs as its page opens, and asks nothing. It installs the LaunchAgent, as
[`service install`](service.md) does, if it's missing. It installs it again if it's out of date,
for example after an upgrade or with another config file.

Installing again keeps the `--log-level`, `--config` and environment the service had. Before it
touches a running service, `setup` checks the router could start from them. If the router reads
the config and tokens `setup` writes, it takes the changes up itself.

If the step stops, it says why and what to run. For every case, see
[Troubleshooting: setup](../troubleshooting/setup.md).

## The `claude` link: switchboard first on your PATH

The step makes a link named `claude` in switchboard's bin directory, leading to the switchboard
that `setup` was run by. The bin directory is `~/.local/share/switchboard/bin`, or
`$XDG_DATA_HOME/switchboard/bin`.

Then it writes a file for each shell, `init.zsh`, `init.bash` and `init.fish`, beside it in
`shell/`. Each puts the bin directory first on `PATH`, and loads the completions. The router writes
these files afresh as it starts, so an edit to one doesn't last.

Then `setup` asks your login shell which `claude` it runs. The step is done once a `claude` typed in
a new terminal goes through the link.

Until it is, `setup` offers once to add one line to the end of your shell's startup file. It names
the file and shows the line first. `enter` adds it, and `n` leaves it to you.

| Key | What it does |
|---|---|
| `c` | Copies the line, if your terminal lets a program copy (OSC 52). |
| `d` | Copies the bin directory, to put first on `PATH` yourself. |
| `r` | Asks your shell again, as often as you like. |
| `enter` | Leaves the step for now, and `setup` carries on. |

If your shell still runs another `claude`, or you use an alias, see
[Troubleshooting: the claude link](../troubleshooting/claude-link.md). It says which startup file
gets the line, and what each message means.

### The session hook

Then `setup` installs the session hook in Claude Code's `settings.json`, leaving your other
settings and hooks as they are. The hook makes a session that didn't start through switchboard
say so. See [The session hook](../reference/session-hook.md).

## Skill: what Claude knows about switchboard

The Skill step writes a Claude Code skill at `skills/switchboard/SKILL.md`, in `~/.claude` or in
`$CLAUDE_CONFIG_DIR`. The skill tells Claude what switchboard does under `claude`, so no session
needs it explained.

Switchboard owns the file. When a new version brings a new skill, `setup` replaces the file, edits
and all, and so does the router as it starts. See
[Claude and switchboard](../claude-and-switchboard.md).

## Done: where each step stands

Done shows a line for each step, then what to run next. `enter` opens the
[dashboard](../dashboard.md), and `q` leaves without it.

## Good to know

- **`setup` needs a terminal of at least 95 columns by 25 rows.**
- **Without a terminal,** `setup` lists the command for each step instead.
- **A step that fails stops `setup` there.** Its page says why.
- **`setup` refuses a temporary build,** such as one from `go run`, because the service and the
  link both run the switchboard `setup` was run by.

## Related

- [Accounts](accounts.md): add, edit and remove accounts outside `setup`.
- [Service](service.md): install, restart and check the router.
- [Uninstall](uninstall.md): take away everything `setup` put on this Mac.
