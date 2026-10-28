# Commands

This page describes every switchboard command: what it does, its flags, and what it prints.

| Group | Command | What it does |
|---|---|---|
| [Everyday](#everyday) | [`usage`](#usage) | Prints every account's windows as they stand. |
| | [`dashboard`](#dashboard) | Shows every account's usage full screen, on seven tabs. |
| | [`pin`](#pin) | Tells the router where to send sessions. |
| | [`claude`](#claude) | Starts the real Claude Code through the router. |
| [Setting up](#setting-up) | [`setup`](#setup) | Walks you through setting switchboard up. |
| | [`accounts`](#accounts) | Lists, adds and removes accounts, and replaces their tokens. |
| | [`service`](#service) | Manages the LaunchAgent that keeps the router running. |
| [Checking in](#checking-in) | [`status`](#status) | Prints every account's usage as text, or the status document as JSON. |
| | [`logs`](#logs) | Shows the last lines of the router's log or the CLI's. |
| [Looking back](#looking-back) | [`requests`](#requests) | Lists the request ledger's requests. |
| | [`history`](#history) | Sums up the ledger's days, priced, and judges whether you have enough accounts. |
| | [`sessions`](#sessions) | Lists today's sessions, or shows one session's page. |
| | [`events`](#events) | Lists what the router decided and noticed. |
| [Plumbing](#plumbing) | [`run`](#run) | Starts Claude Code through the router: what `claude` runs. |
| | [`serve`](#serve) | Runs the router in the foreground. |
| [Help and version](#help-and-version) | [`help`](#help) | Lists the commands, or shows one command's help. |
| | [`version`](#version) | Prints switchboard's version. |

## Flags every command takes

Every command takes these flags.

| Flag | What it does |
|---|---|
| `--config <file>` | Use `<file>` as the config file, in place of the one switchboard finds. See [Configuration](configuration.md). |
| `-h`, `--help` | Show the command's help. |

To print the version, run `switchboard --version`, `switchboard -v` or [`switchboard version`](#version).

A command that needs the router fails without it, and says how to start it.

If you paste a token by mistake, what you see depends on where it went:

- Where an account's id or a session's id goes, no command repeats it, nor does a command in
  refusing an argument or a flag's value. It shows as `[redacted]`.
- Where a command goes, as `switchboard <token>`, or as `help`'s topic, as
  `switchboard help <token>`, it's repeated as given, in saying there's no such command or topic.

## Everyday

### `usage`

`usage` prints every account's windows as they stand, once, into the scrollback, with the time
they were read. You can hold it against one printed an hour before.

```bash
switchboard usage [--5h] [--week] [--<model>] [--by window|account] [--probe] [-r] [--json | --pretty]
```

| Flag | What it does |
|---|---|
| `--5h`, `--week` | Print that window alone, with the header and the key line. Given together, they print the windows given, in the order of care: the week, the 5-hour window, then each model's own. |
| `--<model>` | Print a model's own window, as `--week` does the week. There's a flag for each model with a window of its own, named for it: `--fable` is for Fable's week. A flag for a window no account reports fails, naming the windows there are. |
| `--by window\|account` | Lay the windows out by window, comparing the accounts one window at a time, or by account, keeping each account's windows together. Overrides [`[usage] by`](configuration.md#usage-keys). Where neither says, it's by window. |
| `--probe` | Probe every account, even while the router runs. |
| `-r`, `--refresh` | Have the router first read every account it may, as the dashboard's `R` does, and wait for it, ten seconds at most. Without the router, or with `--probe`, every account is probed anyway. |
| `--json` | Print the status document as JSON, even on a terminal. It does so off a terminal anyway. The window flags and `--by` shape the printout alone: the JSON is the whole document whatever they say. |
| `--pretty` | Print the bars, even off a terminal, as it does on one. Off a terminal, the bars have no colour unless `CLICOLOR_FORCE` asks for it, and they're as wide as `COLUMNS` says, else 80 columns. |

`usage` reads the router's status document while the router answers. Otherwise, it probes each
account.

When its output isn't a terminal, such as in a pipe or an agent's shell, `usage` prints the
status document as JSON instead, as [`status --json`](#status) does, read as its flags say.

#### What it prints

From the top, `usage` prints:

1. A header, with when it read and the router's status, such as
   `● healthy  ·  6 sessions  ·  new sessions → ▲ side`. While a global pin holds, the header adds
   `pinned to personal`.
2. A bar for every window of each account.
3. The pool, last.
4. A key line saying what the marks mean.

Each row is an account, in the router's order: the config's, once the router has taken it up. A
row shows these parts:

| Part | What it shows |
|---|---|
| Number | The account's number, as the dashboard numbers it: its place in the router's order, the config's once the router has taken it up. |
| Label | The account's label, with `◆` on the primary and `▲` on the account new sessions go to. |
| Bar | The window's use, filled along the theme's ramp. `┃` marks where even use would have put it by now. `╎` marks where the account's cap starts: 1 less its `reserve`. |
| Percentage | How much of the window is used. |
| Words | The first of the states in the next table that holds. |

| Words | When they show |
|---|---|
| `limit · back 15:54` | Claude's limit holds the window. |
| `at cap · back 17:10` | The window's use has reached the cap, and the cap holds the account back. |
| `out ~Sat 06:00` | The window runs out before its reset, at its recent rate. |
| `not started` | A 5-hour window hasn't started yet. |
| `19%/day till Sun 21:00` | Otherwise, the window's allowance: what can be spent of it an hour or a day and still last to its reset. |
| `34% till 16:30` | In place of the allowance, under an hour from the 5-hour window's reset, or under a day from a week's reset: the room itself. |

Some accounts can't show a reading:

- An account with no usable token reads `no token`, and its bars are empty.
- An account nothing has been read of reads `not read yet`, and its bars are empty.
- An account whose token is refused, or whose last read failed, shows its last reading faded, as
  `not read since 14:20`.

**The pool** is the room new sessions can go to, summed across the accounts in it. It holds
every account on auto, or, when there's a global pin, only the accounts pinned. `usage` shows the
pool wherever there's more than one account:

- The pool's row, `Σ pool`, comes last, under a rule.
- Its percentage is its accounts' mean use, each as its row shows it. An account without a usable
  token, nothing read, or whose token is refused, is left out of it.
- Its words are the room left to the accounts' caps, in accounts' worth, such as `1.5 of 3 left`.
  When pinned, they add ` · pinned`.
- An account outside the pool has its number drawn faintly.

**Every window shows,** used or not. A model's own window, such as Fable's week, shows on every
account, at 0% as at 40%.

**The width** shapes the layout:

- **By window:** the windows sit side by side, a column each, wherever every bar has 12 cells and
  the words their short form: from 86 columns with two windows and short names, 122 with three.
  Narrower, they're stacked, a section each.
- **By account:** each account is a block, with its windows as rows. The account's name sits beside
  its first bar wherever that fits, from 60 columns with short names. Narrower, it's on a line of
  its own above it.
- **The header** is one line wherever the date and the whole status fit side by side, every part
  of it and every reason it gives said in full, from 84 columns for a healthy router on
  auto with the sample's names, as the account it names moves it; else two. So one line never
  says less than two would.
- **The key line** is one line wherever it fits beside the names, else two. The second line names
  the cap, where a bar draws one.
- **The words** are in full where they fit, else short, as `back 15:54`, `out ~Sat` and
  `19%/day · Sun`. As the terminal narrows, the bars shorten to 12 cells first, then the words.
- **The name column** is as wide as the longest name, up to 24 cells of label. A label too long for
  it is cut with `…`. It never moves as `▲` moves between accounts.

**Colour:** `usage` draws in the dashboard's theme, brought down to what the terminal shows, with
no background of its own. See [Themes](dashboard.md#themes). With `NO_COLOR` set, or on a terminal
that shows no colour, a bar's fill is drawn in `█` and its track left as spaces. The names and
percentages keep their bold.

Examples:

```bash
switchboard usage                  # every window, every account
switchboard usage --5h             # the 5-hour window alone
switchboard usage --by account     # each account's windows together
switchboard usage --probe          # read every account from the API, whatever the router says
switchboard usage -r               # have the router read every account it may first, as after a reset made by hand
```

### `dashboard`

`dashboard` shows every account's usage full screen, on seven tabs, until you press `q`. For its
flags, keys and tabs, see [Dashboard](dashboard.md).

### `pin`

`pin` tells the router where to send sessions. It needs the router.

```bash
switchboard pin <account>... [--move] [--force]
switchboard pin auto [--force]
switchboard pin <account>|auto --session <id>
```

| Flag | What it does |
|---|---|
| `--move` | Move running sessions on other accounts there too, each on its next request, at the cost of a cache rebuild each. Not with `auto`. |
| `--force` | Clear every session's own pin too, including the one `run --account` gave it. |
| `--session <id>` | Pin one running session alone to one account, from its next request, in place of any pin it had. Takes neither `--move` nor `--force`. |

`pin <account>...` sends every new session, and any other session whose account is chosen afresh,
to the accounts given while one of them has room. With one account, sessions go to it. With
several, they go to the best of them, as the router would choose were they the only accounts.

- A pin replaces any pin before it.
- When none of the accounts pinned has room, the router chooses among every account as though
  nothing were pinned.
- `pin auto` goes back to routing.

| Command | New sessions | Running sessions |
|---|---|---|
| `pin work` | go to `work` | stay where they are while their caches are warm and their accounts have room |
| `pin work --move` | go to `work` | move to `work` on their next request, except those with a pin of their own |
| `pin work --move --force` | go to `work` | all move to `work`, their own pins cleared |
| `pin work side` | go to the best of `work` and `side` | stay where they are while their caches are warm and their accounts have room |
| `pin work side --move` | go to the best of `work` and `side` | those on neither move to the best of them on their next request, except those with a pin of their own |
| `pin auto` | routed | stay where they are while their caches are warm and their accounts have room |
| `pin auto --force` | routed | as `pin auto`, their own pins cleared |
| `pin work --session 18bb978f` | unaffected | that session moves to `work` on its next request, and stays pinned there |
| `pin auto --session 18bb978f` | unaffected | that session's own pin is cleared, and it's routed like any other |

Pins follow these rules:

- A session's own pin beats the global one.
- Every pin yields at a limit. A pinned session that hits one moves by the usual rules, rather than
  failing.
- A pin spends the reserves of the accounts it names, and no other account's. See
  [Reserves](how-it-works.md#reserves).
- A router that closes as it's asked, as one stopping does, may have made the pin first, so `pin`
  says just that rather than that it isn't running. `switchboard status` shows which.

**Pinning several accounts** sets an order to use them up in. With `pin work side`, `work` and
`side` take the new sessions, the better of the two first, until both are out. Only then does
anything go to the rest.

For example, say two accounts have a weekly reset banked on claude.ai and a third hasn't. Pin the
two, let them run out, and reset them by hand. The router sees a reset the next time it reads the
account, which happens:

- on its own rounds, within 5 minutes;
- before a choice it makes afresh, once its reading is 15 minutes old; or
- at once, with `switchboard usage -r` or `R` on the dashboard. These probe an account that can
  take no request however lately the router read it, unless the router probed the account in the
  last minute.

**To name a session,** give its id, or as much of it as is unique among the sessions routed in the
last hour. To find a session's id:

- `switchboard status` lists the sessions.
- Claude Code's `/status` shows a session's own.
- Inside a session, `$CLAUDE_CODE_SESSION_ID` holds it.

Examples:

```bash
switchboard pin side                  # new sessions go to side
switchboard pin side --move           # and running ones move there too
switchboard pin work side             # new sessions go to the better of work and side
switchboard pin auto --force          # back to routing, every session's own pin cleared
switchboard pin work --session 18bb   # one session, by the start of its id
```

### `claude`

Once `setup` has linked it, `claude` is switchboard, and starts the real Claude Code connected to
the router.

```bash
claude [<claude args>]
```

The link is a link named `claude` in switchboard's bin directory, which switchboard's file for your
shell puts first on `PATH`, ahead of the real `claude`. Run by that name, switchboard hands Claude Code every argument, so `claude --help` is
Claude Code's.

The real `claude` is the first `claude` on `PATH`, else where its installers put it, that doesn't
start switchboard again. Switchboard passes over its own link, any other build of switchboard, and
a wrapper named `claude` that `exec`s switchboard.

How `claude` starts depends on the router and your environment:

- **Routed.** When the router answers saying where it listens, healthy or not, Claude Code starts
  with `ANTHROPIC_BASE_URL` pointing at the router. It gets the primary's token, or, while that
  isn't usable, the token of the first account whose token is. The router chooses an account for
  each request. Claude Code also gets `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`, unless your environment
  sets it already. With it, Claude Code tells the router what it tells the API of each request, such
  as the prompt the request serves, for the request ledger. An unhealthy router is routed through
  all the same, as its failures are upstream, where a direct connection would meet them too, and a
  line on stderr says so: `switchboard: the router is unhealthy (…) — starting claude through it all
  the same`.
- **Stopped.** When the router isn't running, isn't answering, or answers without saying where it
  listens, `claude` starts nothing and exits with status 1. A line on stderr says why, and how to go
  on: `switchboard: the router isn't running — not starting claude: see to it with switchboard
  service status, or go direct this once with SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude`. A slow
  router is waited for, at most 5 seconds in all from the first ask; through the first 2 of them, a
  router that isn't running is asked again, as it may be restarting.
- **Direct.** With `when_router_down = "direct"` in the config, or
  `SWITCHBOARD_WHEN_ROUTER_DOWN=direct` for one launch, Claude Code connects straight to the API
  instead. It uses `--account`'s token, else the primary's, or, while that isn't usable, the token
  of the first account whose token is. A line on stderr says so: `switchboard: the router isn't
  running — connecting directly on work · Work`. A direct session isn't routed or recorded, and it
  can spend the primary's reserve. `SWITCHBOARD_WHEN_ROUTER_DOWN=stop` stops one launch whatever the
  config says; any other value stops it too, saying so. A router that answers saying where it
  listens, healthy or not, routes whatever either says.
- **Without switchboard.** When switchboard can't take part at all, such as when it can't read its
  config or no account has a usable token, `claude` starts as if switchboard weren't there; with
  the config read but the router's state directory not found, only where it would go direct. A line
  on stderr says why:
  `switchboard: couldn't read the config (…) — starting claude without it`.
- **With an API key.** When `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set, Claude Code may
  use that key rather than an account's token, and its requests would go unrouted, billed to the
  key. So `claude` starts as if switchboard weren't there, and says so:
  `switchboard: ANTHROPIC_API_KEY is set, so Claude Code uses it — starting claude without switchboard`.
- **Within a session.** With an API key, a `claude` started within a routed session, as by a
  script, leaves out what switchboard set there: the router's address, switchboard's own custom
  headers and the hint setting. What you set yourself stays, your own custom headers among them,
  and so does what the script changed since, such as a gateway of its own. Without switchboard, it
  keeps the session's routing, as the session's router still runs, and the line on stderr ends
  `— starting claude through this session's router`; unless the script has pointed it at a
  gateway of its own, or `claude` found the session's router down, going direct without a usable
  token: then it leaves out what switchboard set, as with a key.
- **Local subcommands** go straight to Claude Code, untouched and without a word, whether the
  router runs or not: `setup-token`, `update`, `upgrade`, `install`, `doctor`, `mcp`, `plugin`,
  `plugins`, `auth`, `import`, `project`, `auto-mode` and `gateway`, and `--version`, `-v`,
  `--help` and `-h`. Only the first argument counts, so `claude -p doctor` is a prompt.

There are two ways round switchboard:

- `switchboard run --direct` starts Claude Code on its own login.
- The real `claude`, run by its path.

If the switchboard binary itself breaks, every `claude` breaks with it, until switchboard's bin
directory comes off `PATH`.

Switchboard defines no per-account launchers. To make one, alias
`switchboard run --account <id> --`.

Examples:

```bash
claude                                             # routed
claude --resume                                    # the session finds its account again
claude doctor                                      # a local subcommand: straight to Claude Code
alias cxside='switchboard run --account side --'   # a launcher pinned to side
```

## Setting up

### `setup`

`setup` walks you through setting switchboard up, or what's left of it.

```bash
switchboard setup
```

Each step says what's done already, and does only what isn't, so `setup` is safe to run again. It
asks as it goes, a line at a time, so it needs a terminal. Without one, it lists what takes each
step alone: a command where there is one, and for the skill and the session hook, `setup` alone.

`setup` stops at a step that fails, at an interrupt, or when its input ends. Run it again to carry
on from there.

The steps are:

1. **Accounts.** Lists the accounts, each with whether its token is usable, and asks for a missing
   token. Pressing Enter leaves it for later. Offers to add accounts, one at a time, asking each
   one's id, label and token. With no accounts configured, it asks for the first straight away.
   With more than one and none marked, it asks which is the primary.
2. **Priming.** With no day set, asks for one, as `HH:MM-HH:MM`, and writes it to `[prime]`.
   Pressing Enter leaves priming off.
3. **The service.** Installs the LaunchAgent when it isn't installed or loaded, or when it runs
   another switchboard than the one `setup` runs as. Restarts the router only when it doesn't
   answer. The router takes up `setup`'s changes to the config and the tokens itself.
4. **The `claude` link.** Makes `claude` in switchboard's bin directory, leading to the switchboard
   `setup` was run by. The bin directory is `$XDG_DATA_HOME/switchboard/bin`, else
   `~/.local/share/switchboard/bin`.

   Then it writes switchboard's own file for each shell, `init.zsh`, `init.bash` and `init.fish`,
   in its shell directory, `$XDG_DATA_HOME/switchboard/shell`, else
   `~/.local/share/switchboard/shell`. Each puts the bin directory first on `PATH`, once, and loads
   switchboard's completions, for its commands and flags. bash's are in a file of their own beside
   it, `completion.bash`, as `init.bash` is plain POSIX shell, for a `~/.profile` that `sh` reads
   too. Sourcing one prints nothing, and sourcing it again changes nothing more. `setup` writes
   them, and the router brings them up to date as it starts, so an edit to one doesn't last.

   Then `setup` asks your login shell, `$SHELL`, started as a new terminal starts it, which
   `claude` its `PATH` finds. The step is done once a `claude` typed there goes through the link.
   An alias named `claude` in front of it decides that by what it runs: `claude` by name leaves it
   to `PATH`; a path, such as Claude Code's old `~/.claude/local/claude`, goes through switchboard
   only where it's the link, and otherwise `setup` says to remove the alias or point it at
   `claude`; and one that sets `PATH` for `claude` doesn't count, as `setup` can't tell which
   `claude` that `PATH` finds, so it says to take `PATH=` out of it. A function `setup` can't
   read, so it says the function goes through switchboard where it runs `claude` by name. Ctrl-C
   while it asks stops `setup`. `setup` asks zsh, bash and fish alone: for another shell, it shows
   the bin directory, as `$HOME/…`, to put first on `PATH` by hand. Until it's done, for zsh, bash
   and fish, `setup` offers to add one line to the end of your shell's startup file, naming the
   file and showing the line first. For zsh, the line is:

   ```bash
   if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard
   ```

   The file is `.zshrc` where zsh says it reads it, `$ZDOTDIR` or your home; for bash, the first
   there of `~/.bash_profile`, `~/.bash_login` and `~/.profile`, the one a login bash reads, else
   a new `~/.bash_profile`; and for fish, `config.fish` where fish says it reads it, as
   `$__fish_config_dir`. Where your shell couldn't be asked, `setup` writes nothing: it shows the
   line to add by hand, naming the file it usually goes in. Nor does it where zsh names a
   `$ZDOTDIR` without a `.zshrc` while your home has one, as that `.zshrc` may not be the one a
   new terminal reads, where `.zshrc` itself or a file after it sets `ZDOTDIR`: it shows the line
   by hand, naming both. A startup file that's a link, as to your dotfiles, is written where it
   leads. The line is added once, ever: everything else switchboard does in your shell is in its
   own file. Should the file go, the line does nothing.

   When the line's there but the shell still runs another `claude`, something after the line
   changes `PATH`: `setup` says so, and to move the line to the end of the file, or what changes
   `PATH` after it to before it. Where the line is last already, something read after the file
   changes `PATH`, such as `.zlogin`, and `setup` says to put the line after it. A line there
   reading another switchboard's file, `setup` shows the line to put in its place. Declined,
   `setup` shows the line to add by hand, and the bin directory alone, as `$HOME/…`, to put first
   on `PATH` wherever you keep it another way. Then it asks your login shell again, as often as you
   like, until it runs the link. Leaving it isn't a failure: `setup` carries on. A shell that exits
   without answering may have a startup file that starts something needing a terminal, such as
   tmux, and `setup` says so.

   Then, whatever it found, it installs the session hook in Claude Code's `settings.json`, or puts
   it right, leaving your other settings and hooks as they are. The hook has a session that didn't
   start through switchboard say so as it starts. See
   [A session that isn't going through switchboard](how-it-works.md#a-session-that-isnt-going-through-switchboard).
   A `settings.json` it can't read as Claude Code does, it leaves as it is, saying what's wrong,
   and carries on.
5. **The skill.** Writes a Claude Code skill, or brings it up to date. The skill tells Claude what
   switchboard does under `claude`, so no session needs it explained. It's
   `skills/switchboard/SKILL.md` in `~/.claude`, or in `$CLAUDE_CONFIG_DIR`. Switchboard owns the
   file. When a new switchboard brings a new version of it, `setup`, and the router as it starts,
   replace it, edits and all. Otherwise, they leave it be.
6. **Usage.** Prints [`usage`](#usage): every account's windows as they stand.

`setup` refuses a switchboard that won't last, such as `go run`'s, because the service and the link
both run the switchboard `setup` was run by.

`setup` asks for tokens where they don't show as they're pasted. It never takes one pasted where
it would show.

### `accounts`

`accounts` lists the accounts, in config order. Its subcommands add an account, edit one, replace
an account's token, and remove an account.

```bash
switchboard accounts
switchboard accounts add <id> [--label <label>] [--primary]
switchboard accounts edit <id> [--label <label>] [--plan <plan>] [--primary] [--reserve <percent>]
switchboard accounts token <id>
switchboard accounts remove <id>
```

| Flag | What it does |
|---|---|
| `--label <label>` | `add` and `edit`: show the account as `<label>`. Default: the id, which `--label ""` goes back to. |
| `--plan <plan>` | `edit`: the plan the account's subscription is on: `pro`, `max5x` or `max20x`, or `none` to leave it unknown. |
| `--primary` | `add` and `edit`: make the account the primary, the account the browser and the Claude apps use, in place of the one that was. |
| `--reserve <percent>` | `edit`: the share of every window the router leaves unused on the account, as a percentage to two decimal places at most, such as `5%` or `2.5%`, from `0%` up to but not including `100%`. `0` or `0%` takes the reserve away. |

For each account, `switchboard accounts` shows:

- its id and label;
- whether it's the primary; and
- whether its token file holds a token switchboard can use, or, if not, why not and what would put
  it right.

The subcommands do the following:

- **`add`** registers an account, as an `[[account]]` table in the config, making the config if
  there's none.
  - The id names the account for good, and names its token file. It takes letters, digits, `-` and
    `_`.
  - When the account's token file holds no usable token, `add` takes one, and writes it over
    whatever the file held.
  - A `--label` is taken as it shows: without the characters Unicode draws as nothing (its
    default ignorables, such as zero-width spaces and joiners, variation selectors and tags),
    each run of whitespace within it one space, and the whitespace and direction marks at its
    edges trimmed. Such a character is kept only where it makes an emoji or a flag, or shapes a
    joining script, such as Persian or Devanagari, and a variation selector only after a symbol,
    as in ❤️, non-ASCII punctuation, or a letter outside Latin, Greek and Cyrillic, or in a
    keycap, as in 1️⃣; it's dropped everywhere else. Direction marks within it are kept, and so
    is every other character, such as an Arabic number sign. One that's the id is no label, as it
    shows the same.
  - An id or a `--label` that holds anything shaped like a token is refused, before `add` asks for
    the token, and so is a `--label` over 100 characters as it shows, one that holds a control
    character, such as a newline or a tab, or a bidirectional embedding, override or isolate,
    which could reorder the row it's shown in, one that would show as nothing, such as spaces or
    zero-width characters alone, and one that isn't UTF-8 text. Private-use glyphs, such as Nerd
    Font icons, show.
- **`edit`** changes an account's label, plan, primary or reserve, in its table in the config.
  - It takes one or more of `--label`, `--plan`, `--primary` and `--reserve`, and leaves what it
    isn't given as it is.
  - It says what it changed, each value as it is now and as it was, labels in quotes, and a label
    of none as none, the account shown by its id. When every value given is what the account has
    already, it says so, and writes nothing.
  - Made the primary, an account takes it from the one that had it. The sessions running through
    the router keep the token they started on as Claude Code's own until they restart: the former
    primary's, pinned or not, or, where it has no usable token, the first account's with one, or a
    pinned session's pinned account's, as the tokens stand now. So what they send outside the
    conversation, such as artifacts and uploads, still goes out on that account. `edit` says so:
    where the first account with a usable token is the new primary, it says the pinned sessions
    keep their pinned account's token. It speaks of pinned sessions only where an account other
    than the first with a usable token has one, as otherwise none can be pinned elsewhere; and
    where no account has one, it says nothing. Sessions started
    directly, while the router was down, are untouched by the move. It warns on stderr when the
    new primary has no usable token, as new sessions then start on another account's.
  - A reserve written as a number alone, such as `5` or `0.05`, is refused, as either could mean
    the other: write it as a percentage. So is one past two decimal places.
  - A `--label` is taken and refused as `add` takes and refuses one; `--label ""`, or the id,
    shows the account by its id, and takes the label out. A label written untidily in the config
    and given tidily is saved tidily, and `edit` says it was written untidily. A label written by
    hand is said as it holds: a character drawn as nothing that `add` would drop, a
    bidirectional control, or a direction mark, which `add` keeps, shows as its escape, such as
    `\u200B`. What `add` keeps in an emoji, a keycap, a variant, a flag or a joining script shows
    as it is, unless an escape stands before it.
  - `--primary=false` is refused: to move the primary, give `--primary` to another account.
  - An id no account has is said before anything wrong with a flag's value. Errors in the flags
    themselves, such as a flag without its value or an unknown flag, come first.
  - A value it replaces keeps the comment beside it; a key it takes out goes with its comment, as
    the primary's mark does when another account is made the primary.
  - A label the config writes over several lines, as a multi-line string, can't be edited as text:
    a label edit of that account is refused, to be made by hand, and nothing is written. A plan,
    reserve or primary edit is made: a key the account has is replaced where it stands, and one
    it hasn't is put below the line that closes the label, whatever the label holds.
- **`token`** replaces an account's token.
- **`remove`** removes an account's table from the config, with the comments directly above it,
  and deletes its token file, then the record of when its token was taken (see
  [When each token was taken](configuration.md#when-each-token-was-taken)).
  - Of a token file or a record that's a link, it deletes the link alone, and leaves the file it
    leads to, even where that's at the record's own path. Where the token file led to the
    record's own path, and the file there has another name too, the record's path goes with the
    token, the file kept under its other name, and it says so.
  - The only account can't be removed.
  - Removing the primary says which account is the primary now: the one marked
    `primary = true`, else the first.
  - Every session holds the primary's token. Sessions running on a removed account's token stay
    routed, as the primary's, for a week from when the router takes the change up, while the
    primary has a usable token. `remove` says when the primary has none.

**Tokens.** At a terminal, `add` and `token` ask for the token, which doesn't show as it's pasted.
Otherwise, they read it from stdin. To make a token, run `claude setup-token` while signed in to
that subscription.

`add` and `token` check the token with the API before they save it:

- A token the API refuses isn't saved.
- A token the API doesn't answer for is saved, with a warning.
- An interrupt while they wait for the token saves nothing.

A saved token's year counts from when it's saved, as nothing says when a pasted token was made.
Saving a token again, even after another, keeps the earlier of its two dates, unless 8 others
were saved since, when it counts afresh. A record restored readable by others, but not writable,
keeps its dates too, written afresh, private.

**The config** is edited as text, keeping its comments and layout. A config that's a link is
written through, to where it leads. The router takes the change up itself: the service's router
restarts to read it, and one started by hand with `serve` logs that a restart is due. See
[`serve`](#serve).

Examples:

```bash
switchboard accounts add work --label Work --primary
switchboard accounts add side --label Side
switchboard accounts edit side --plan max5x --reserve 5%
pbpaste | switchboard accounts token side
switchboard accounts remove side
```

### `service`

`service` manages the LaunchAgent that keeps the router running. launchd starts
`switchboard serve` now, at every login, and again whenever it stops, without the light throttling
it gives a job that doesn't say what kind it is, as every Claude Code request waits on it. `setup`
installs it. launchd reads the LaunchAgent only as it loads it, so one an older switchboard wrote
keeps what it said through upgrades and restarts in place: `service install` writes it afresh, and
restarts the router to load it.

```bash
switchboard service install [--log-level <level>]
switchboard service uninstall
switchboard service restart
switchboard service status
```

| Flag | What it does |
|---|---|
| `--log-level <level>` | `install`: have the router log at `debug`, `info`, `warn` or `error` and above. Default: `$SWITCHBOARD_LOG_LEVEL`, else `info`. |

#### `service install`

`install` writes `~/Library/LaunchAgents/io.github.leeovery.switchboard.plist`. It has launchd load
the file in place of any copy launchd had loaded, once that copy's router has finished its requests
in flight, and waits up to 5 seconds for the router to answer.

- **The config.** `install` first checks what the router would start from, as the router itself
  checks a new version before restarting into it: the config above all, and the state directory
  its control socket goes in. It fails where the router couldn't start, before it touches the
  router running.
- **The binary.** The LaunchAgent runs switchboard by the path you ran it by, so a Homebrew link
  stays the link `brew upgrade` moves on. A temporary build, such as `go run`'s, is refused.
- **The environment.** The LaunchAgent carries `--config`, and these variables where they're set:
  `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `SWITCHBOARD_CONFIG`, `SWITCHBOARD_LOG_LEVEL` and
  `CLAUDE_CONFIG_DIR`. It makes `--config`, `SWITCHBOARD_CONFIG` and `CLAUDE_CONFIG_DIR`
  absolute, so the router finds what the CLI does. The tokens are files, so the router needs
  nothing else of your environment.
- **The tokens.** `install` warns when no account has a usable token. The router starts all the
  same, and routes as soon as a token file holds one.

#### `service restart`

`restart` restarts the router, which reads the config and the tokens afresh. What happens depends
on the router:

| The router | What `restart` does |
|---|---|
| Answering | Asks the router to restart. The router finishes its requests in flight first, for up to 30 seconds, then gives those it cuts off 5 more to unwind. Then it restarts in place, as it restarts itself. |
| Answering, but can't replace itself | Asks the router to restart, as above, and launchd starts it again. |
| Run by hand | The router refuses, and `restart` says why. |
| Answering, with a config that isn't valid | The router refuses, as it couldn't start again from that config, and `restart` says why. After an upgrade, the new version judges the config instead where it can answer, as the next row says, so a config only it takes doesn't hold the restart. |
| Answering, with a binary that refuses to start | The router asks the binary it would become first. Where that refuses, as a newer version can refuse a config the running one takes, the router refuses too, and `restart` says why, in the binary's words. |
| Answering, mid-upgrade | Where the binary isn't there, as for a moment while an upgrade moves it, or the config or the binary changes as the router checks whether it could restart, the router refuses, and `restart` names what changed and says to try again. |
| From before routers restarted in place | The router stops as at a signal, and launchd starts it again. |
| Not answering | launchd starts the service afresh at once. |

When the router restarts, `restart` says so,
`the router is finishing its requests in flight, then it restarts in place`, or, for a router that
can't replace itself, `…, then launchd starts it again`. Then it waits up to 50 seconds for the new
router to answer.

When the router has a restart due, it waits for a moment with no request in flight. To have the
restart now, run `switchboard service restart`, as `switchboard status` says. When the restart is
held, as the binary the router would become refuses to start, `status` gives the refusal instead:
put the config right as it says.

#### `service uninstall` and `service status`

- **`uninstall`** stops the router, once it has finished its requests in flight, and removes the
  LaunchAgent.
- **`status`** shows whether the LaunchAgent is installed and loaded, and the router's health.

When the router doesn't answer in time, `install` and `restart` fail, with the LaunchAgent left in
place. They point to `switchboard logs router` and `launchd.log` for why.

Examples:

```bash
switchboard service install
switchboard service install --log-level debug
switchboard service status
```

## Checking in

### `status`

`status` prints every account's usage as text. Off a terminal, it prints the status document as
JSON instead.

```bash
switchboard status [--json | --pretty] [--probe] [-r]
switchboard status --session <id> [--json | --pretty]
```

| Flag | What it does |
|---|---|
| `--json` | Print the status document as JSON, what an agent or a script reads, even on a terminal. It does so off a terminal anyway. With `--session`, it prints the session's every model and why it went where it did. See [`status --session`'s JSON](reference.md#status---sessions-json). |
| `--pretty` | Print the text, even off a terminal, as it does on one. |
| `--probe` | Probe every account, even while the router runs. |
| `-r`, `--refresh` | Have the router first read every account it may, as [`usage -r`](#usage) does, and wait for it, ten seconds at most. Without the router, or with `--probe`, every account is probed anyway. |
| `--session <id>` | Print the id of the account the router sends that session's requests to: the one its last-used model went to, as a statusline asks. It does so on a terminal or off one. It needs the router, and takes no `--probe` or `--refresh`. |

`status` reads the router while it runs. Otherwise, it probes each account. When the `claude` a
shell runs from `PATH` isn't switchboard, the first line says so.

When switchboard can't read the config, as when an upgrade's rules refuse it and the router holds
its restart into the new version, `status` shows the router's document while the router answers,
as text or JSON, then fails with the config's error on stderr. Without the router, or with
`--probe`, which needs the config to probe, it fails with the error alone.

The text shows, in order:

- each window's utilization, when it resets and where it's heading;
- what holds an account back, such as a limit it reached or its reserve;
- whether an account is under pressure, with the rate it goes by, such as
  `under pressure: runs out ~18:21 at 5-hour's rate over the last 30 min, before its reset at 20:10`;
- the sessions routed in the last hour, a line each, with the account each of the session's models
  goes to;
- the priming schedule;
- the best account to use next; and
- last, where the usage came from, with the router's health. Under it comes a restart the router
  has due, if any, with how to have it now.

When its output isn't a terminal, such as in a pipe or an agent's shell, `status` prints the status
document as JSON instead. The document holds:

- every account's windows, each with its even pace, where even use would have put it by now, and
  its allowance, what can be spent of it an hour or a day and still last to its reset;
- each account's extra usage, where the API reports it;
- the pool: the room new sessions can go to, summed across its accounts, and their mean use; and
- what's coming up, soonest first: each limit or cap lifting, window running out or resetting, and
  prime.

[The status document](reference.md#the-status-document) lists its fields.

Examples:

```bash
switchboard status
switchboard status --json | jq '.accounts[] | {id, windows}'
switchboard status --json -r   # have the router read every account it may first, as after a reset made by hand
switchboard status --session "$CLAUDE_CODE_SESSION_ID"   # inside a session: the account it's on, such as work
```

### `logs`

`logs` shows the last lines of a log: the router's, or the one every other command shares. Without
a log named, it shows the router's once the router has logged, else the CLI's.

```bash
switchboard logs [router|cli] [-n <lines>] [-f] [--path]
```

| Flag | What it does |
|---|---|
| `-n`, `--lines <n>` | Show the last `n` lines. Default: 50. When the log is shorter, it reaches into the rolled-over file. |
| `-f`, `--follow` | Go on printing lines as they're logged, across rotations, until interrupted. |
| `--path` | Print the log's path, and nothing else. |

Examples:

```bash
switchboard logs -f              # follow the router: a line per request, with its account and why
switchboard logs cli -n 200
switchboard logs router --path
```

## Looking back

These commands read what switchboard has recorded:

- `requests`, `history` and `sessions` read the
  [request ledger](reference.md#the-request-ledger)'s files where they lie.
- `events` reads the files the router keeps its events in, beside the ledger's.

So none of them needs the router, though `sessions` asks the router which sessions are running,
where it answers.

Their plain text is a first cut. Agents read their JSON, which each command prints wherever its
output isn't a terminal.

### `requests`

`requests` lists the ledger's requests: today's, unless `--since` reaches further back.

```bash
switchboard requests [--session <id>] [--account <id>] [--since <when>] [--json | --pretty]
```

| Flag | What it does |
|---|---|
| `--session <id>` | Keep that session's requests. Name the session by its id, or as much of it as is unique among the sessions read. |
| `--account <id>` | Keep that account's requests. |
| `--since <when>` | Start at a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`). |
| `--json` | Print each line as the ledger holds it, a JSON object a line, even on a terminal. It does so off a terminal anyway. See [The request ledger](reference.md#the-request-ledger). |
| `--pretty` | Print the text, even off a terminal, as it does on one. |

The requests are listed oldest first, under the day each arrived on. Each request gets a line,
with these fields:

| Field | What it shows |
|---|---|
| Time | When the request arrived. |
| Session | The session's id, cut short. |
| Model | The model. |
| Account | The account. |
| Status | The status. |
| Tokens | The tokens in and out. |
| Duration | How long the request took. |

Examples:

```bash
switchboard requests --since 14:00
switchboard requests --session 18bb --json | jq '.usage'
```

### `history`

`history` sums up the ledger's days, prices them, and judges whether you have as many accounts as
your use needs. It covers the last 30 days, unless `--since` says otherwise.

```bash
switchboard history [--since <when>] [--windows] [--json | --pretty]
```

| Flag | What it does |
|---|---|
| `--since <when>` | Start on the day of `<when>`: a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`). |
| `--windows` | Print each account's every window's readings over those days, in place of the days. |
| `--json` | Print the blocks as JSON, even on a terminal. It does so off a terminal anyway. The JSON holds the day the prices were read, then the days' summaries, priced, then the year, months, weeks, tokens, totals, plans and capacity over them. With `--windows`, it holds the windows' readings. See [`history`'s JSON](reference.md#historys-json). |
| `--pretty` | Print the text, even off a terminal, as it does on one. |

#### The days

Each day has a row for each account and model, with these columns:

| Column | What it shows |
|---|---|
| Requests | The requests the account served with the model. |
| Tokens | Their tokens. |
| Sessions | Their sessions. |
| Worth | What they'd have cost through the API. Switchboard prices them at the prices it carries, read from Anthropic's pricing page, or at the config's, in [`[prices]`](configuration.md#prices-keys). |

Under the rows come each account's notes for the day:

- its sessions;
- its moves;
- the limits it reached;
- the weeks it reset, as `week reset 10:00`; and
- its windows' highest use.

An account that served no request that day gets notes only where it reached a limit or one of its
weeks reset. Where none of the days has requests, `history` says so, then shows those notes alone,
without the totals under them.

These rules hold for what's counted:

- **Requests are messages:** those that went upstream, and those the router answered itself.
- **Quota checks and token counts** are never among the requests, nor make a row of their own.
  Claude Code sends them, and they spend next to nothing. A quota check is worth nothing either,
  as it isn't work done for you, so its tokens are left out of the day's. A day summarised before
  this rule keeps the few its checks spent. A model only they named wasn't used, so it's in no list
  of the models, versions or families, nor counted among them.
- **Today** is summed up from its lines, as far as it has gone.
- **A model the price table doesn't know** shows as unpriced, never free.

#### Plan prices

Each account's plan, the [`plan`](configuration.md#account-keys) the config gives it, has a price
a month. `history` prorates it as the dashboard prices its periods:

| Period | Priced at |
|---|---|
| A day | 12/365 of a month |
| A week | 12/52 of a month |
| 30 days | a month |
| 12 weeks | twelve weeks' price |

- A period still running is priced whole.
- A month or week that the days asked for begin part-way through is priced by its days from then.
- The plans' own period runs from the later of the day asked for and the first day the ledger
  holds, as the dashboard prices it: there's no worth before the ledger began to set against it.
- An account without a `plan` has no cost and no ratio. Every account's plan, cost and ratio are
  those of the accounts that have one, while the worth above them is every account's. So where
  an account's plan isn't known, every account's ratio says how many plans it's over: `4.4× its
  $92 · 2 of 3 plans known`.

#### Over all the days

Under the days, `history` shows, over all of them:

- each account's totals, its plan and its worth against it, a column each, and one column for every
  account;
- each week's peaks; and
- whether there are as many accounts as the use needs.

To judge whether there are enough accounts, `history` works with weeks as follows:

- **An account's own week** runs from one reset of the account's week to the next, as the readings
  history saw them, a reset by hand among them. Its peak is the use read just before the reset
  that ended it.
- **Placing a week:** an account's week is placed by the date it began on. It goes under its
  calendar week, from the config's [`week_starts`](configuration.md#top-level-keys), where it began
  up to half the week in, as a Thursday is in a week from Monday. Otherwise, it goes under the next
  calendar week.
- **The week still running** is always this week's, and its peak is its highest so far. Where the
  reset that began it came before the days asked for, it's read from the week before them.
- **A short week** is one in which some account hit its week's limit.
- **The replay:** each whole week, in which every account's week is known and ended, is replayed
  twice:
  - With an account fewer. The dropped account's peak, times its plan's size, is shared among the
    rest by their plans' sizes.
  - With one more account, of the plan most of the accounts have. Every peak falls in proportion.

  In the replay, an account without a `plan` counts as the plan most of the accounts have. Where
  no account has one, each counts as one.

The verdict, as the dashboard's [History](dashboard.md#history) words it, is the first of these
that holds:

| Verdict | When |
|---|---|
| `one too few` | Now is short on 4 or more of every 7 weeks, and one more account would halve that. |
| `too few` | Now is short on as many, and one more account wouldn't halve that. The replay says how short one more would still leave it. |
| `one more than you need` | Now is short on fewer than 4 of every 7 weeks, and one account fewer would be short on no more weeks. It names the account to drop, and, of three accounts or more, how many a replay without it and the next by the same rule finds. |
| `about right` | None of the above holds. |

Each account also gets a verdict, over its own whole weeks:

| Account verdict | When |
|---|---|
| `hits its limit most weeks` | It hit its limit in 3 or more of every 7 weeks. |
| `plenty to spare most weeks` | It had 40% or more left at a reset, on average. |
| `ends most weeks with room` | None of the others holds. |
| `barely used` | It had 75% or more left at a reset, on average. |
| `could go` | The verdict is `one more than you need`, the account hit no limit, used 10% or less, and the rest cover it. |

Then every account together gets one:

| Verdict on every account | When |
|---|---|
| `room to spare together` | A quarter or more was left at a reset, on average across them. |
| `little to spare together` | Otherwise. |

Two more rules hold:

- **Not enough weeks:** where no whole week can be judged yet, `history` says
  `Weeks: not enough weeks yet`.
- **Never guessed:** what a day never counted is left out, never taken for none. One example is a
  day recorded before the ledger counted resets and time at a limit.

#### Windows

`--windows` prints, in place of the days, each account's windows over those days. Each time a
window's use, its reset or its status changed, it prints a line, as the readings history holds it.

Examples:

```bash
switchboard history --since 7d
switchboard history --json | jq '.days[-1]'      # today's, as far as it has gone
switchboard history --json | jq '.capacity'      # whether there are as many accounts as the use needs
switchboard history --windows --since 14:00      # today's readings of every window
```

### `sessions`

`sessions` lists today's sessions. Given a session, it shows that session's page.

```bash
switchboard sessions [<id>] [--json | --pretty]
```

| Flag | What it does |
|---|---|
| `--json` | Print the sessions, or the session's page, as JSON, even on a terminal. It does so off a terminal anyway. See [`sessions`' JSON](reference.md#sessions-json). |
| `--pretty` | Print the text, even off a terminal, as it does on one. |

#### Today's sessions

`sessions` lists those running first, the oldest started first. Then come those that ended today,
the latest ended first. Each session gets a line, with these fields:

| Field | What it shows |
|---|---|
| Id | The session's id, cut short. |
| Directory | The session's directory. |
| Account | The account the session is on. |
| Model | The session's model. |
| Doing | What it's doing: asking, answering, idle, or when it ended. |
| Started | When it started. |
| Requests | Its requests today. |
| Worth | What they'd have cost through the API. |
| How it came | How it came to its account, such as `from work, at its cap`, and what writing its context again there cost. |

A last line sums them up.

`sessions` reads the ledger's files, and the router's sessions where the router answers. Without
the router, it lists today's sessions from the ledger alone, none as running, and says so.

#### A session's page

To show a session's page, name it by its id, or as much of it as is unique among today's sessions.
A session of an earlier day is found by its whole id.

The page shows:

- **The session:** what it's doing, where it runs and why, when it started, and when it came back
  after an hour or more without a request.
- **Each account it ran on:** its time there, its requests, the points of each of the account's
  windows they took, and their worth.
- **Its turns,** newest first. A turn is a prompt and every request that serves it, including its
  subagents', compactions and side requests, as Claude Code's own headers tell them.
- **Its totals.**

Each turn shows:

- when it started and how long it took;
- its requests;
- the tools its answers called most;
- the tokens it read from the cache, wrote to it, and put out;
- its worth; and
- the accounts its own conversation went to, as `work → side` where it moved. A subagent on
  another account is no move.

The points are an estimate. A window counts every session of its account, so each rise in the
window's use between two answers is shared out among the requests that ended between them, by
their tokens.

Without the router, the page is shown from the session's lines alone, as not running. The session
shows as idle where its last request came within the hour, else as ended.

Examples:

```bash
switchboard sessions
switchboard sessions --json | jq '.sessions[] | select(.running)'   # those running now
switchboard sessions 5b0e --json | jq '.turns[0]'                    # its newest turn
```

### `events`

`events` lists what the router decided and noticed: today's, unless `--since` reaches further back.

```bash
switchboard events [--since <when>] [-f] [--json | --pretty]
```

| Flag | What it does |
|---|---|
| `--since <when>` | Start at a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`). |
| `-f`, `--follow` | Then go on printing a line each time the router files one, until interrupted. A changed event is printed again, whole. |
| `--json` | Print each event as the router files it, a JSON object a line, with its `run` and `id`, even on a terminal. It does so off a terminal anyway. Of an event printed again, the last line is how it stands. See [The router's events](reference.md#the-routers-events). |
| `--pretty` | Print the text, even off a terminal, as it does on one. |

The events cover:

- sessions started and moved;
- accounts under pressure, at their caps and limits, refused, primed, and open again;
- pins set and cleared;
- restarts falling due, or held; and
- the router's health.

They're listed oldest first, under the day each happened on. Each event shows once, as it last
stood, on a line with these fields:

| Field | What it shows |
|---|---|
| Time | When it happened. |
| Session | The session's id, cut short, if the event is of one. |
| Kind | The kind of event. |
| Account | The account. |
| What happened | What happened. |

The events' files outlast a restart, so `events` looks back across restarts.

Examples:

```bash
switchboard events --since 2d
switchboard events -f --json | jq 'select(.kind == "limit")'
```

## Plumbing

### `run`

`run` starts Claude Code through the router. It's what `claude` runs. Claude Code's own arguments
go after `--`.

```bash
switchboard run [--account <id>] [--direct] [-- <claude args>]
```

| Flag | What it does |
|---|---|
| `--account <id>` | Pin the session's conversation to account `<id>`, which must be configured and have a usable token. |
| `--direct` | Start Claude Code on its own login, without the router or a token, for what needs the login. Not with `--account`. |

`--account` pins the session's conversation to an account while that account can take it. Claude
Code's own token stays the primary's, or is `--account`'s while the primary's isn't usable. Without
the router, going direct, Claude Code connects on `--account`'s token.

Examples:

```bash
switchboard run -- --resume
switchboard run --account side -- -p "tidy the changelog"
switchboard run --direct
```

### `serve`

`serve` runs the router in the foreground, until interrupted or terminated. The service normally
runs it.

```bash
switchboard serve [--log-level <level>]
```

| Flag | What it does |
|---|---|
| `--log-level <level>` | Log at `debug`, `info`, `warn` or `error` and above. Overrides `SWITCHBOARD_LOG_LEVEL`. |

The router serves two things:

- the proxy, on `listen`; and
- its control API, on a unix socket in the state directory.

It logs to the router's log, and to the terminal when it runs in one. A second router fails while
one is running.

#### Starting

The router starts even when no account has a usable token, warning in its log that nothing will be
routed until one has. As it starts, it:

- makes the `tokens`, `history` and `ledger` directories private;
- takes up the readings its history holds, so the recent rates it judges pressure by carry on from
  before; and
- brings the skill up to date.

#### Token changes

Every 3 seconds, the router reads the token files again, and takes up what's changed in place:

- An account goes out on the token its file holds now.
- An account without a usable token gains the one its file comes to hold.
- An account whose file holds no token it can use, at two looks in a row, has nothing to send on
  until a token is back.

A single look finding no token, as when a file is caught while it's rewritten, keeps the account's
token.

#### Restarting in place

The router also watches:

- its config file, following links;
- the binary it was started as: the Homebrew link the service runs; and
- `/etc/localtime`, which says the Mac's time zone.

The service's router restarts when the config changes into one that's valid, when an upgrade leads
the link to another binary, or when the time zone changes. To restart, it:

1. Waits for a moment with no request in flight.
2. Asks the binary the link leads to whether it can start from the config.
3. Saves its state.
4. Replaces itself in place with the binary the link leads to, handing it the sockets it listens on.

Some cases go differently:

- **A config that isn't valid** is logged, and the router carries on as it was.
- **A binary that refuses to start,** as a newer version can refuse a config the running one
  takes, holds the restart: the router carries on as it was, and says why in its log, its events
  and `status --json`'s `restart`. It asks again once the config or the binary changes. A binary
  from before the check can't answer, and the router restarts into it as before.
- **A binary that isn't there,** as for a moment while `brew upgrade` moves the link on, is tried
  again for a couple of seconds. If replacing itself still fails, the router exits, and launchd
  starts it again.
- **Told to stop** while it finishes its requests to restart, the router stops then and there.
- **Run by hand,** the router logs, once, that a restart is due, and `switchboard status` says to
  run `serve` again.

## Help and version

### `help`

`switchboard help` lists the commands, and `switchboard help <command>` shows one command's help.

Every command also takes `-h` and `--help`. `claude --help` is Claude Code's own.

### `version`

`version` prints switchboard's version. `switchboard --version` and `switchboard -v` do the same.
