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
| | [`uninstall`](#uninstall) | Takes away what `setup` put on this Mac, before `brew uninstall`. |
| [Checking in](#checking-in) | [`status`](#status) | Prints every account's usage as text, or the status document as JSON. |
| | [`logs`](#logs) | Shows the last lines of the router's log or the CLI's. |
| [Looking back](#looking-back) | [`requests`](#requests) | Lists the request ledger's requests. |
| | [`history`](#history) | Sums up the ledger's days, priced, and judges whether you have enough accounts. |
| | [`sessions`](#sessions) | Lists today's sessions, or shows one session's page. |
| | [`events`](#events) | Lists what the router decided and noticed. |
| [Keeping history](#keeping-history) | [`export`](#export) | Copies this Mac's history into a folder of its own. |
| | [`import`](#import) | Brings the history of other Macs' folders, or a Mac's own, into this Mac's. |
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
| `--<model>` | Print a model's own window, as `--week` does the week. There's a flag for each model with a window of its own, named for it: `--fable` is for Fable's week. A flag for a window no account reports fails, naming the windows there are, as `--5h` does where every account is on a verified Ultra plan. An unverified Ultra account keeps its 5-hour window, and `--5h` works. |
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
| `out ~16:05`, `out ~Sat` | The window runs out before its reset. Within a day the time shows, as `out ~16:05` or `out ~Sat 06:00`; further ahead, the day alone, as a forecast days out can miss by a day, with its date where the day is today's weekday, as `out ~Thu 12 Nov`. A week goes by its forecast, the 5-hour window by its recent rate: see [Where a week runs out](how-it-works.md#where-a-week-runs-out). |
| `no 5-hour limit` | The account has no 5-hour window, as a verified Ultra account hasn't. It shows in place of the bar: `no limit` where the room is short, and by account, where the window's label beside it names the window. |
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
- Its words are the room left to the accounts' caps, in accounts' worth, such as `1.5 of 3 left`,
  out of the accounts that have the window. When pinned, they add ` · pinned`.
- A window that fewer than two of the pool's accounts have, and not all of them, has no pool figure:
  side by side its part of the pool's row is blank, and stacked its section has no pool.
- An account outside the pool has its number drawn faintly.

**Every window shows,** used or not. A model's own window, such as Fable's week, shows on every
account, at 0% as at 40%.

**A window an account doesn't have** is learnt from its answers, never from its plan: a verified
Ultra account's carry no 5-hour window. Stacked, the window's section leaves the account out. A
window no account has isn't shown at all. An account not read yet may have any window, so it keeps
its rows.

**A token's year nearly up:** under the key line, after a blank line, each account whose token's
year is up within 30 days, or is up, has a line, as the Accounts tab's Detail tells it:
`1 work  token expires 17 Nov 2026 · 21 days left — switchboard setup makes a new one`,
`· last day` on its last day, and `token expired 17 Nov 2026` once it's up. Other accounts add
nothing.

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
  `19%/day · Sun`: short, a time on another day than today gives its day alone, a run-out's within
  a day too, but a run-out or `back` further ahead on today's weekday gives its date,
  `out ~12 Nov`, `back 12 Nov`. An allowance keeps its day, `19%/day · Thu`, as a date doesn't fit
  beside the rate. As the terminal narrows, the bars shorten to 12 cells first, then the words.
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
- **Without the config.** When switchboard can't read its config, as when an upgrade's rules refuse
  it, `claude` is routed all the same while the router answers, as Routed says, on the accounts the
  router names: Claude Code gets the primary's token, as the router names it, or, while that isn't
  usable, `--account`'s, else the token of the first account whose token is. A line on stderr says
  so: `switchboard: couldn't read the config (…) — starting claude through the running router`. An
  unhealthy router adds Routed's second line: `switchboard: the router is unhealthy (…) — starting
  claude through it all the same`. Where the router answers but none of the accounts it names has a
  usable token, the line says so before `— starting claude without it`, so you don't take it for
  the router being down: `— the running router names no account with a usable token —`.
- **Without switchboard.** When switchboard can't take part at all, such as when it can't read its
  config and the router doesn't answer, or no account has a usable token, `claude` starts as if
  switchboard weren't there; with the config read but the router's state directory not found, only
  where it would go direct. A line on stderr says why:
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
  token or without the config: then it leaves out what switchboard set, as with a key.
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

`setup` walks you through setting switchboard up, or what's left of it, full screen.

```bash
switchboard setup
```

It runs in the dashboard's chrome, a page a step: Accounts, Priming, the service, the `claude`
link and the skill, then Done. Each step says what's done already, and does only what isn't, so
`setup` is safe to run again. It needs a terminal at least 95 columns by 25 rows; smaller, it says
to make the terminal larger. Without a terminal, it lists what takes each step alone: a command
where there is one, and for the skill and the session hook, `setup` alone.

Each page says what its step found and did, a row a fact, and its keys are on the last row.
`enter` moves on once a step is done; there's no going back. A step that fails stops `setup`
there: its page says why, and `setup` leaves failing. `q` leaves at any step, and `setup` run again
carries on from where things stand. While a card is open, `q` does nothing, and while Priming's
day is typed in, it types: press `esc` first, or `ctrl+c`, which always leaves. At Done, `enter`
opens the dashboard, and `q` leaves without it.

However it's left, by `q`, a step that failed or Done, `setup` prints each step's line into the
scrollback once the screen has gone, as Done says it, one after another, with the line to add by
hand, or the directory to put first on `PATH`, under the `claude` link's where it isn't done, on a
line of its own, whole, however narrow the terminal, so it copies as one line. `c` and `d` copy
them on the screen where your terminal lets a program copy (OSC 52); where it doesn't, they're
there to copy. Where a token went unchecked, as when you leave before the checks answer, the
Accounts line counts each kind, as `3: 1 working, 1 unchecked, 1 refused`, or `3, all unchecked`,
or `1, unchecked` of one account. For example:

```text
  ✓  Accounts             2, all working · primary: work
  ✓  Priming              on, 08:00–23:00
  ✓  Service              installed · router healthy
  !  claude link          created, but it isn't first on your PATH. Add this line to ~/.zshrc:
     if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard
  ✓  Skill                up to date
```

Then, left before Done, it says `run setup again to carry on`; left at a step that failed, it
prints the step's error and ends with a status of 1.

The steps are:

1. **Accounts.** Shows every account as a table: its id, label, plan, reserve and token. Each
   token is checked with the API as `setup` opens, and shows the date its year is up, with the
   days left. A note under the table says what an account is missing, and the key that sees to
   it. `↑` and `↓` choose an account; `t` makes it a new token, `e` edits its label, plan, reserve
   and whether it's the primary, `r` removes it, asking first, and `p` makes it the primary: the
   account your browser and the Claude apps are signed in to. `+ add an account` adds one: its id,
   label and plan, then its token. With no account, the card adding one opens at once. With two
   or more and none the primary, `enter` waits for one. On a first run, where the primary has
   no reserve, `enter` asks for one first, once: the share of its windows kept back for the
   Claude apps, as a percentage, or `none`; `esc` leaves it with none, writing nothing.

   `setup` makes each token with `claude setup-token`, which it runs itself, opening its sign-in
   page in your browser. The first account's token, or the primary's on a run again, opens in
   the browser as it is. Every other's opens in a private window, where your browser can open
   one from the command line, as the Chrome family and Firefox can, so you sign in there to that
   account's subscription; otherwise, as for Safari, `setup` shows the address to open in one. The
   token's checked with the API, then saved, good for a year. A token for a subscription another
   account has already isn't saved. `p` on the token card pastes a token you made with `claude
   setup-token` instead; a pasted token shows as dots, never the token.
2. **Priming.** Asks for your day, as `HH:MM-HH:MM`, holding the day set, if any, and writes it to
   `[prime]` where it changed. Left empty, priming stays off; cleared, it's turned off. Where every
   account is on Ultra, setup skips this step and says so: a verified Ultra account has no 5-hour
   window to start. A day already set is left as it is.
3. **The service.** Runs as its page opens, asking nothing. Installs the LaunchAgent when it isn't
   installed and no router answers or runs. Installs it again when launchd hasn't loaded it and no
   router answers, when it runs another
   switchboard than the one `setup` runs as, when `setup` is given another config file than the one
   the router serves, or when its plist differs from the one `switchboard service install` would
   write with what it was installed with, as after an upgrade that adds a key, and says what
   differed, when the router's copy of switchboard it runs is missing, or when launchd still starts
   switchboard itself, having loaded the LaunchAgent before the router's copy, as after the router
   moved over to it. Installing it again keeps the `--log-level`, `--config` and environment it was
   installed with, every variable in it, a `HOME` too, as the environment decides where the router
   keeps its state, tokens included. A `--config` given to `setup` takes the place of the kept one
   only where it names another file than the one the router serves. Before it touches a running
   service, `setup` checks the router could start from what it keeps, its flags included. Where it
   couldn't, `setup` leaves the service as it is while a router answers, saying why, and fails
   while none does, saying how to install it once that's put right. A plist `setup` can't read
   back, or one that names no program, is installed afresh from `setup`'s own options and your
   shell's environment, once the router could start from them, only where launchd hasn't loaded the
   service and no router answers, as the next paragraph says. A plist that isn't XML, as the binary
   one `defaults write` leaves, is read back through `plutil`. A plist that sets `HOME` empty,
   where the router's state directory or config file is found under it, can't start a router again:
   `setup` fails, naming `switchboard service install`, and leaves a router that answers as it is,
   saying to stop it first where it isn't launchd's own, as the next paragraph says. A `--config`
   given to `setup` stands in for the config file. Where only Claude Code's config directory is
   under it, `setup` says the router won't keep the skill up to date, and carries on.

   `setup` watches the router where the service's plist keeps its state, and the one where your
   shell does, and asks launchd whether it runs the router. It leaves the service as it is, saying
   why. With no plist it can use, none or one it can't read back, `setup` never succeeds while a
   router answers or runs, or launchd has loaded a plist it can't read, nor sends you back to
   `setup`: a plist it can't read holds options it can't keep, so you choose. It leaves everything
   as it is and fails, saying to run `switchboard service install`, which writes the plist afresh,
   as no plist, or one launchd can't read, can't start the router at login. Where a router run some
   other way answers, it says to stop that router first, and what launchd has: the service not
   loaded, loaded but running no router, or running one, its pid named, that doesn't answer where
   `setup` looks. With a plist it can read, it leaves the service where a router answers that isn't
   launchd's own, run some other way, as another would clash with it, and fails the same way: stop
   that router, then run `switchboard service install`. A router that gives no pid, from before
   routers gave theirs, counts as launchd's only where launchd runs one; and where launchd says it
   runs one but `setup` can't read its pid, the router that answers counts as launchd's too, as it
   does where `launchctl print` says neither whether launchd runs one nor its pid. It leaves the
   service too where only the one at your shell's answers, so it can't tell which the service runs;
   and where launchd runs a router that answers at neither within 5 seconds, which may be starting,
   stopping, or keeping its state where `setup` doesn't look. Then the step fails, naming its pid
   where it's known: if it doesn't answer soon, as `switchboard service status` shows, `switchboard
   service install` installs it again: give it the `--config` and `--log-level` it ran with, if
   any. `setup` never sends you back to `setup`. It says where the router finds things apart from
   `setup` only once a router answers. Run `switchboard service install` from the shell the router
   should use to line them up. It restarts the router only when none answers and none runs, once it
   has checked the router could start, and fails where it couldn't, saying how to install it. A
   router started so is judged where it answers, once it does: where that's where your shell keeps
   its state, not where the plist does, `setup` says so. It never starts the service afresh while
   any router answers, and installs it again while one answers only where that one answers where
   the plist keeps its state, launchd having it loaded. When the router uses another state
   directory, config file or Claude Code config directory than `setup`, `setup` says which, never
   where: run `switchboard service install` from the shell the router should use to line them up,
   or, for the config file, `switchboard setup --config` naming the router's, after which `setup`
   finds them the same and changes nothing. Where the router reads the config and the tokens
   `setup` writes, it takes up their changes itself.
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

   Then `setup` asks your login shell, `$SHELL`, started as a new terminal starts it, which `claude`
   its `PATH` finds. The step is done once a `claude` typed there goes through the link. An alias
   named `claude` in front of it is read by a plain rule: its words are split at spaces and tabs,
   and only plain words are read, made of letters, digits and `_./:@%+,=-`, or a path from an
   unquoted `~/`, `$HOME/` or `${HOME}/`. Leading variables, plain but for a `$PATH` or `$HOME`
   within them, as `CLAUDE_CONFIG_DIR=~/.claude-work` or `PATH=/x:$PATH`, and `command`, are passed
   over. Then `claude` leaves it to `PATH`, though after a variable that sets `PATH`, as `PATH=…
   claude`, it doesn't count, as `setup` can't tell which `claude` that `PATH` finds, so it says to
   take that `PATH` setting out of the alias; and a clean path to a `claude`, with no trailing `/`
   and no `.` or `..` in it, such as Claude Code's old `~/.claude/local/claude`, goes through
   switchboard only where it's the link, with no `PATH` set before it, and otherwise `setup` says to
   remove the alias or point it at `claude`. Another command runs something else, as `npx …` does,
   where the alias is all plain words and names no `claude`; otherwise, as with `caffeinate -i
   claude` or `cd ~/work && claude`, it's a wrapper `setup` doesn't read. A function, or a wrapper,
   `setup` doesn't read, so it says it goes through switchboard where it runs `claude` by name.
   `q` while it asks stops `setup`. `setup` asks zsh, bash and fish alone: for another shell, it
   shows the bin directory, as `$HOME/…`, to put first on `PATH` by hand. Until it's done, for zsh,
   bash and fish, `setup` offers, once, in a card, to add one line to the end of your shell's
   startup file, naming the file and showing the line first: `enter` adds it, and `n` leaves it to
   add by hand. For zsh, the line is:

   ```bash
   if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard
   ```

   The file is `.zshrc` where zsh says it reads it, `$ZDOTDIR` or your home; for bash, the first
   there of `~/.bash_profile`, `~/.bash_login` and `~/.profile`, the one a login bash reads, else
   a new `~/.bash_profile`; and for fish, `config.fish` where fish says it reads it, as
   `$__fish_config_dir`. Where your shell couldn't be asked, `setup` writes nothing: it shows the
   line to add by hand, naming the file it usually goes in. Nor does it where zsh names a
   `$ZDOTDIR` without a `.zshrc` while your home has one, as that `.zshrc` may not be the one a
   new terminal reads: a `ZDOTDIR` set in `.zshenv` is, as is one set in `.zprofile` for a login
   shell, as macOS terminals start, but one set in `.zshrc` or after it isn't. It shows the line by
   hand, naming both. A startup file that's a link, as to your dotfiles, is written where it
   leads. The line is added once, ever: everything else switchboard does in your shell is in its
   own file. Should the file go, the line does nothing.

   When the line's there but the shell still runs another `claude`, something after the line
   changes `PATH`: `setup` says so, and to move the line to the end of the file, or what changes
   `PATH` after it to before it. Where the line is last already, something read after the file
   changes `PATH`, such as `.zlogin`, and `setup` says to put the line after it. A line there
   reading another switchboard's file, `setup` shows the line to put in its place. Declined,
   `setup` shows the line to add by hand, and the bin directory alone, as `$HOME/…`, to put first
   on `PATH` wherever you keep it another way: `c` copies the line and `d` the directory, and `r`
   asks your login shell again, as often as you like, until it runs the link. `enter` leaves it
   for now, which isn't a failure: `setup` carries on. A shell that exits
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
6. **Done.** A line a step, as it stands, the `claude` link's with the line to add by hand where
   it isn't done, then what to run next. `enter` opens the [dashboard](dashboard.md).

`setup` refuses a switchboard that won't last, such as `go run`'s, because the service and the link
both run the switchboard `setup` was run by.

`setup` takes a token only where it doesn't show as it's pasted, as on the token card. It never
takes one pasted where it would show.

### `accounts`

`accounts` lists the accounts, in config order. Its subcommands add an account, edit one, replace
an account's token, and remove an account.

```bash
switchboard accounts
switchboard accounts add <id> [--label <label>] [--primary]
switchboard accounts edit <id> [--label <label>] [--plan <plan>] [--primary] [--reserve <percent>]
switchboard accounts token <id> [--paste]
switchboard accounts remove <id>
```

| Flag | What it does |
|---|---|
| `--label <label>` | `add` and `edit`: show the account as `<label>`. Default: the id, which `--label ""` goes back to. |
| `--plan <plan>` | `edit`: the plan the account's subscription is on: `pro`, `max` or `ultra`, or `none` to leave it unknown. A plan's old name, `max5x` or `max20x`, works too, and the config is written with its new. |
| `--primary` | `add` and `edit`: make the account the primary, the account the browser and the Claude apps use, in place of the one that was. |
| `--reserve <percent>` | `edit`: the share of every window the router leaves unused on the account, as a percentage to two decimal places at most, such as `5%` or `2.5%`, from `0%` up to but not including `100%`. `0` or `0%` takes the reserve away. |
| `--paste` | `token`: take a token made with `claude setup-token`, pasted, in place of making one. |

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
    edges trimmed. Such a character is kept only where it makes an emoji or a flag, England's,
    Scotland's or Wales's among them, or shapes a joining script, such as Persian or Devanagari,
    and a variation selector only after a symbol, as in ❤️, non-ASCII punctuation, or a letter
    outside Latin, Greek and Cyrillic, or in a keycap, as in 1️⃣; it's dropped everywhere else,
    the tags after a black flag that name no other flag with them. Direction marks within it are
    kept, and so is every other character. One that's the id is no label, as it shows the same.
  - An id or a `--label` that holds anything shaped like a token is refused, before `add` asks for
    the token, and so is a `--label` over 100 characters as it shows, one that holds a control
    character, such as a newline or a tab, or a bidirectional embedding, override or isolate,
    which could reorder the row it's shown in, one that holds a sign that joins the character
    after it, such as an Arabic number sign, which can hide that character, one that would show
    as nothing, such as spaces or zero-width characters alone, and one that isn't UTF-8 text.
    Private-use glyphs, such as Nerd Font icons, show.
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
  - Where the config marks no account the primary, the first is the primary for want of one:
    `--primary` on it marks it so, and `edit` says it did.
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
- **`token`** replaces an account's token. At a terminal, it makes the token as `setup` does,
  saying each step as it goes:
  - It runs `claude setup-token`, which opens its sign-in page in your browser: as it is for the
    primary, as the browser is signed in to it, and in a private window for any other account, so
    you sign in to that one there. A private window opens where the default browser can open one
    from its command line, as the Chrome family's, Edge's and Firefox's can. Close any other
    private window first, as they all share one sign-in. Where `token` can't open the page, as
    for Safari, it shows the address to open by hand.
  - Once you approve it, `claude setup-token` creates the token, and `token` checks it with the
    API and saves it, a year from today.
  - The token isn't saved where the API refuses it, or where its subscription is another
    account's already, as the organization the API names it in says. So `token` checks the other
    accounts' tokens first, all at once, to know theirs, saying so: a moment, as long as the API
    takes to answer, up to about 10 seconds where it doesn't. Where an organization isn't known,
    the token is saved.
  - Ctrl-C before the token's created stops `claude setup-token` and saves nothing. Once it's
    created, as it lasts a year on the account, Ctrl-C waits: `token` says it saves the token
    first, checks it, and says what came of it, then exits as interrupted. The token is saved;
    shown to copy, where it was made but couldn't be read or saved; or not saved, where the API
    refuses it or it's another account's. It's saved unchecked, with a warning, where the API
    doesn't answer.
  - Where the token was created but couldn't be read off `claude setup-token`'s screen, `token`
    shows what that screen showed, the token in it, to copy. Where it was read but couldn't be
    saved, `token` shows it, to copy, and why. Either way it fails: paste it with `--paste`.
  - Where it saves nothing otherwise, it says why and fails: run it again, or paste a token with
    `--paste`.
- **`remove`** removes an account's table from the config, with the comments directly above it,
  and deletes its token file, then the record of when its token was taken (see
  [When each token was taken](configuration.md#when-each-token-was-taken)).
  - Of a token file or a record that's a link, it deletes the link alone, and leaves the file it
    leads to, even where that's at the record's own path. Where the token file led to the
    record's own path, and the file there has another name too, the record's path goes with the
    token, the file kept under its other name, and it says so.
  - The only account can't be removed.
  - Where the token file can't be deleted, `remove` fails, saying the account is taken out of the
    config, and the token file's path, and why it isn't deleted.
  - Removing the primary says which account is the primary now: the one marked
    `primary = true`, else the first.
  - Every session holds the primary's token. Sessions running on a removed account's token stay
    routed, as the primary's, for a week from when the router takes the change up, while the
    primary has a usable token. `remove` says when the primary has none.

**Tokens given.** At a terminal, `add`, and `token --paste`, ask for the token, which doesn't show
as it's pasted. Otherwise, they read it from stdin, as `token` does too. To make a token, run
`claude setup-token` while signed in to that subscription.

`add` and `token` check a token given with the API before they save it:

- A token the API refuses isn't saved.
- A token whose subscription is another account's already, as the organization the API names it
  in says, isn't saved, and `add` adds nothing. Once the API has named it, they check the other
  accounts' tokens, all at once, to know theirs. Where an organization isn't known, the token is
  saved.
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
switchboard accounts edit side --plan max --reserve 5%
switchboard accounts token side
pbpaste | switchboard accounts token side
switchboard accounts remove side
```

### `service`

`service` manages the LaunchAgent that keeps the router running. launchd starts
`switchboard serve`, from the router's own copy of switchboard, now, at every login, and again whenever it stops, without the light throttling
it gives a job that doesn't say what kind it is, as every Claude Code request waits on it. `setup`
installs it. launchd reads the LaunchAgent only as it loads it, so one an older switchboard wrote
keeps what it said through upgrades and restarts in place: `service install` writes it afresh, and
restarts the router to load it. One written before the router had its own copy is the exception:
its router moves over to the copy by itself as it starts, after the upgrade that brings the copy,
writing the LaunchAgent afresh to run it, and launchd runs the copy from the next login on (see
[Restarting in place](#restarting-in-place)). Where it can't, `service status` says so, and names
`service install`.

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
- **The binary.** The LaunchAgent runs a copy of switchboard of its own, which `install` makes
  from the switchboard you ran, following its links: `$XDG_DATA_HOME/switchboard/router/switchboard`,
  else `~/.local/share/switchboard/router/switchboard`. macOS keeps what you allow the router,
  such as a folder it writes into, against the path of the file it runs, and Homebrew puts each
  version at a path of its own, so the copy's path never changes. The LaunchAgent names the path
  you ran switchboard by too, with `serve --from`, so a Homebrew link stays the link
  `brew upgrade` moves on. A temporary build, such as `go run`'s, is refused.
- **The environment.** The LaunchAgent carries `--config`, and these variables where they're set:
  `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_DATA_HOME`, `SWITCHBOARD_CONFIG`,
  `SWITCHBOARD_LOG_LEVEL` and `CLAUDE_CONFIG_DIR`. It makes `--config`, `SWITCHBOARD_CONFIG` and `CLAUDE_CONFIG_DIR`
  absolute, so the router finds what the CLI does. The tokens are files, so the router needs
  nothing else of your environment. Any other variable the LaunchAgent in place sets, such as an
  `HTTPS_PROXY` you added to it by hand, even with `defaults write`, `install` keeps, saying so
  where it can't read the LaunchAgent in place and they're gone. A `HOME` it sets `install`
  drops, as launchd gives the router yours. It waits for the router where the environment it
  writes keeps its state, and for no other.
- **The tokens.** `install` warns when no account has a usable token, where the router it started
  answers. The router starts all the same, and routes as soon as a token file holds one.
- **Notifications.** In a release, the router writes out switchboard's own app as it starts, and
  macOS asks, the first time, whether `Switchboard` may send notifications. A switchboard built
  from source has no app, and macOS asks nothing. See
  [Notifications](configuration.md#switchboards-own-notifications).
- **Once it has begun.** From writing the file to launchd loading it, nothing stops `install`, a
  Ctrl-C included, so the router is never left with nothing to start it: a Ctrl-C then says it's
  finishing first, which can take a minute or two while the old router finishes its requests, and
  `install` stops once launchd has the new copy.

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
| Answering, with an upgrade it can't copy in | Where the router can't copy the upgrade due in beside its own copy, as on a full disk, it refuses, and `restart` says why. |
| Answering, mid-export | Where an [export on a schedule](configuration.md#export-keys) is under way, the router refuses, as it never restarts in place over one, and `restart` says `an export into <dir> is under way: try again once it ends`. |
| From before routers restarted in place | The router stops as at a signal, and launchd starts it again. |
| Not answering | launchd starts the service afresh at once. `restart` asks the router where the LaunchAgent keeps its state, and the one where your shell does, and starts the service afresh only when neither answers; the first to answer then, at either, is the one launchd started. |

When the router restarts, `restart` says so,
`the router is finishing its requests in flight, then it restarts in place`, or, for a router that
can't replace itself, `…, then launchd starts it again`. Then it waits up to 50 seconds for the new
router to answer.

When the router has a restart due, it waits for a moment with no request in flight, and no export
running. To have the restart now, run `switchboard service restart`, as `switchboard status` says.
While an export is under way, `status` names the export the restart waits for instead, as `service
restart` is refused until it ends. When the restart is held, as the binary the router would become
refuses to start, `status` gives the refusal instead: put the config right as it says.

#### `service uninstall` and `service status`

- **`uninstall`** stops the router, once it has finished its requests in flight, and removes the
  LaunchAgent and the router's copy of switchboard, then the notifications' app: it unregisters
  `Switchboard.app` from macOS and removes it, with the files beside it, its sum, its lock and any
  folders an interrupted write left. It waits up to 30 seconds while another switchboard writes
  the app, and leaves the app be where that takes longer. Where the app can't be unregistered, it
  removes it all the same. Whatever of this fails, it says so, and the service is uninstalled
  regardless.
- **`status`** shows whether the LaunchAgent is installed and loaded, whether the router's copy of
  switchboard it runs is missing, so launchd can't run the router, or not run at all, as the
  LaunchAgent was written before the copy and runs switchboard itself, naming `service install`
  for each, or run by launchd only from the next login, as it loaded the LaunchAgent before the
  copy and, reading it only as it loads it, starts switchboard itself till then, as `launchctl
  print` names the program, saying `service install` has it run the copy now, whether
  launchd runs a router, and its pid where `launchctl print` names one, or that its state
  is unread where it says neither, and the health of the router where the LaunchAgent keeps its state, else of the one
  where your shell does, saying so. Where it can't read the LaunchAgent's plist back, or work out
  from it where the router keeps its state, `status` and `restart` say so, and look for the router
  where your shell keeps its state. Where only the one at your shell's answers, or there's no
  plist, or it couldn't be read or placed, and the router can't be asked to restart, `restart`
  fails rather than signal launchd's, which may not be it.

When the router doesn't answer in time, `install` and `restart` fail, with the LaunchAgent left in
place. They point to `switchboard logs router` and `launchd.log` for why.

Examples:

```bash
switchboard service install
switchboard service install --log-level debug
switchboard service status
```

### `uninstall`

`uninstall` takes away what `setup` put on this Mac, so nothing switchboard leaves keeps working
on your behalf. Run it before `brew uninstall switchboard`: Homebrew has no way to run it for you.

```bash
switchboard uninstall [--all]
```

| Flag | What it does |
|---|---|
| `--all` | Takes the config, the accounts' token files and the recorded history too. |

It takes away, in this order:

1. **The service**, as [`service uninstall`](#service-uninstall-and-service-status) does: the
   router stopped once it has finished its requests in flight, the LaunchAgent, the router's copy
   of switchboard and the notifications' app.
2. **The startup line** `setup` added, taken out of each shell's startup file it's in, found by its
   `# switchboard` mark: `~/.zshrc`, and `.zshrc` in `$ZDOTDIR`; `~/.bash_profile`, `~/.bash_login`
   and `~/.profile`; and fish's `config.fish`. Only the line as `setup` writes it, at the start of
   its line, is taken out: an indented copy may be all there is of an `if` block of yours, which
   the shell couldn't read left empty, so it's left where it is, and `uninstall` names the file for
   you to take it out yourself; with switchboard's files gone, it does nothing. Every other line is
   kept as it was, and the file is rewritten in place, where a link to it leads, so it stays the
   file it was, hard links, owner and permissions with it. Then switchboard's own files for the
   shells, which the line loads, with bash's completions.
3. **The session hook**: switchboard's entry in Claude Code's SessionStart hooks, in
   `settings.json`, every other setting and hook left as it is. A settings file Claude Code
   couldn't read is left as it is, and `uninstall` says why.
4. **The skill**, in Claude Code's config directory.
5. **The `claude` link** in switchboard's bin directory, where it's switchboard's own, a link to a
   `switchboard`: anything else named `claude` there is left alone, and `uninstall` says so. Then
   the bin directory and switchboard's data directory, `~/.local/share/switchboard`, each once
   nothing else is in it: where something's still in the data directory, `uninstall` says it's
   kept.

The config, the accounts' token files and everything recorded in the state directory, the ledger,
the readings, the events, the logs and the router's state, stay, so a reinstall carries on where it
left off. `--all` takes them too: the config directory, with any themes of your own, and the state
directory, wherever the environment puts them, as the rest of switchboard finds them. A config file
`SWITCHBOARD_CONFIG` or `--config` puts outside the config directory goes alone; of a themes
directory `SWITCHBOARD_THEMES_DIR` puts there, only the `.theme` files go, then the directory where
nothing else is in it, as it may hold other files of yours. The history
nothing can bring back goes only when you ask: nothing asks first either way, as the flag is the
asking.

`uninstall` never touches the [export's folder](configuration.md#export-keys), wherever it is. Where
it's inside a directory `--all` removes, however its path names it, through a link included, it's
kept, with the directories on the way to it, and `uninstall` says so; where it's the directory
itself, the directory is kept whole. A config or state directory that's a link, as to your
dotfiles, is removed as a link, alone, and what it leads to is left whole. Where the config can't
be read to tell where the export's folder is, `--all` keeps the config and the state directory,
and says why. Nor does it touch switchboard's binary, which is
Homebrew's.

It prints a line for each piece, saying it's removed, not there, or failed and why, the service's
saying both where its LaunchAgent went but the notifications' app couldn't, then that the
accounts' setup tokens stay valid on Anthropic's side until their year is out, whether or not
their files are removed, and last, to run `brew uninstall switchboard`:

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

The paths it prints are whole; they're written from `~` here. A piece that fails doesn't stop the
rest: `uninstall` goes on to the last, then fails. Run again, it says each piece is not there, and
succeeds.

Examples:

```bash
switchboard uninstall
switchboard uninstall --all
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

- each window's utilization, when it resets and where it's heading, or `no 5-hour window` for an
  account without one. A week goes by its
  [forecast](how-it-works.md#where-a-week-runs-out) from the router, as `on pace for 92%` or
  `runs out ~Tue`; probed, or a model's own week, at its pace since it started; the 5-hour window
  by its recent rate, as `runs out ~17:25 at its rate over the last 30 min`. A run-out shows its
  time within a day, and its day alone further ahead, with its date on today's weekday;
- what holds an account back, such as a limit it reached or its reserve;
- whether an account is under pressure, with the rate it goes by, such as
  `under pressure: runs out ~18:21 at 5-hour's rate over the last 30 min, before its reset at 20:10`;
- the sessions routed in the last hour, a line each, with the account each of the session's models
  goes to;
- the priming schedule;
- the best account to use next; and
- last, where the usage came from, with the router's health. Under it comes a restart the router
  has due, if any, with how to have it now, or, while an export is under way, the export it waits
  for, as `service restart` refuses until that ends.

When its output isn't a terminal, such as in a pipe or an agent's shell, `status` prints the status
document as JSON instead. The document holds:

- every account's windows, each with its even pace, where even use would have put it by now, and
  its allowance, what can be spent of it an hour or a day and still last to its reset;
- each account's extra usage, where the API reports it;
- the pool: the room new sessions can go to, summed across its accounts, and their mean use;
- each account's plan, as the config gives it, and, from the router, the rates each week's
  forecast goes by: each account's windows' fast rates, and the pool's use as the forecast takes it;
- what's coming up, soonest first: each limit or cap lifting, window running out or resetting, and
  prime; and
- how the router's hourly export stands, where [`[export]`](configuration.md#export-keys) sets a
  folder: when one last succeeded, why the last failed, and when the one under way began.

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
- **A short week** is one in which some account hit its week's limit: its peak reached its cap,
  100% less its [`reserve`](configuration.md#account-keys), where it has one. It's the reserve the
  config has now, as a week's own isn't recorded beyond the moments an account was held at its cap.
  What a week had left at its reset is left of its limit, not of its cap. A replayed week is short
  where any account would reach its cap.
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

## Keeping history

### `export`

`export` copies this Mac's history into a folder of its own in `<dir>`. Keep `<dir>` where Dropbox
or iCloud Drive syncs it, and the history is somewhere else should the Mac be lost. Switchboard
writes the folder and nothing more: what keeps it safe, or carries it to another Mac, is up to you.

```bash
switchboard export <dir>
```

A relative `<dir>` is taken from the working directory. `<dir>` is made if it isn't there, and the
Mac's folder in it is `mac-<id>`, named by this install's id. So Macs that name the same `<dir>`,
as a config kept in a dotfiles repository does, never write each other's files.

The Mac's folder holds what switchboard can't build again, in the state directory's own names:

| File | Holds |
|---|---|
| `export.json` | `{"version": 1, "install": "<id>", "exported_at": "<time, UTC>"}`: whose folder it is, and when it was last exported to. |
| `ledger/requests-<date>.jsonl.gz` | The [request ledger](reference.md#the-request-ledger)'s lines, one file for each day. |
| `ledger/events-<date>.jsonl.gz` | The [router's events](reference.md#the-routers-events), one file for each day. |
| `ledger/day-<date>.json` | Each [day's summary](reference.md#the-days-summary). |
| `history/readings-<date>.jsonl.gz` | The [readings history](reference.md#the-readings-history), one file for each day. |

Each day's lines are in one compressed file, which `gzip -dc` reads. It leaves out the files the
router builds again from the lines, the days' digests, parts, heads and rows, and `state.json`,
`prefs.json`, the config, the tokens and the logs.

- **What it writes:** a day already in the folder, and unchanged since, is left as it is. A day
  compressed in the state directory is copied as it is. The last three days, not yet compressed,
  are compressed on their way, and copied again once a line is added. A line still being written is
  left for the next export.
- **Never half written:** every file is written beside where it goes and renamed into place, so a
  sync never carries one half written.
- **Nothing removed:** nothing in the folder is removed, a day past keeping included, but the
  hidden files an export cut short left as it wrote, `.<name>.<digits>`, which each export removes
  an hour after they last changed. The folder keeps what it's given until you remove it.

It reads the files where they lie, so it needs no router. It prints how many days it exported,
and how many of them it wrote:

```
exported 31 days to /Users/you/Dropbox/switchboard/mac-0123456789abcdef: 3 written, 28 as they were
```

A `<dir>` that's a file, or that can't be made or written, fails, saying so, and stops the export
there. A day whose files can't be opened or read in the state directory, as one whose file
switchboard isn't allowed to read, is passed over, and the rest exported. Then `export` names each
day it passed over, and why, and fails:

```
switchboard: couldn't export the request ledger of 2026-10-30: open /Users/you/.local/state/switchboard/ledger/requests-2026-10-30.jsonl.gz: permission denied
```

A folder on a disk that keeps coarser times works as well: one that keeps them to 100 nanoseconds,
as a share over SMB does, to the microsecond, to 10 milliseconds, as exFAT does, or to the second,
still leaves a day unchanged since as it is. On a FAT disk, which keeps them to 2 seconds, or one that rounds them, some unchanged files
are written again at each export.

The install's id is in `install-id`, in the state directory, made by the first export and never
replaced. A Mac whose state directory was restored from another's, as Migration Assistant restores
it, carries the other's id, and exports to its folder. Remove `install-id` to give it an id, and a
folder, of its own at its next export.

To have the router export every hour, set [`[export]`](configuration.md#export-keys)'s `dir`. How
its exports stand is the status document's `export`: see
[An export that fails](troubleshooting.md#an-export-that-fails).

Example:

```bash
switchboard export ~/Dropbox/switchboard
```

### `import`

`import` brings the history in Macs' folders, as [`export`](#export) writes them, into this
Mac's. Point it at the `<dir>` you export into, and it takes in each Mac's folder there; or point
it at one Mac's folder, and it takes in that one. So a new Mac takes in the last one's history,
and a Mac whose history was lost takes back its own.

```bash
switchboard import <dir>
```

A relative `<dir>` is taken from the working directory. A Mac's folder is one holding an
`export.json` that reads. This Mac's own is taken in like any other, and a folder without one is
passed over. A folder whose `export.json` doesn't read, as one damaged, is passed over too, and
`import` says so, and carries on with the rest:

```
switchboard: passed over /Users/you/Dropbox/switchboard/mac-0123456789abcdef: its export.json doesn't read
```

A `<dir>` that holds no Mac's folder fails, saying so, and so does one holding a folder a newer
switchboard exported, as this one may not read its files: upgrade switchboard to import it.

What it takes in:

- **A day this Mac doesn't hold** is copied in.
- **A day it holds already** gains the lines it doesn't hold. Each line is told apart from every
  other Mac's, so an import repeated, or of a Mac's own folder, adds nothing.
- **A day's summary** is copied in where this Mac holds no line of the day, and either no summary
  of it, or one that counts no line, as of a day it only probed, where the folder's counts some.
  Otherwise this Mac's stands, and the router summarises again each day that gained lines.
- **Days past keeping:** a day's lines older than [`[ledger] keep`](configuration.md#ledger-keys),
  or its readings older than [`[history] keep`](configuration.md#history-keys), are left out, as
  the router would remove them. Their summaries are taken in, as summaries are kept for good.

It needs the router, which writes the history, and fails without it. It copies the folders' files
into the state directory's `import/`, then has the router take them in, and waits for it, however
long that takes. The router then summarises the days at once, so `history`, the dashboard and the
other commands have them within minutes. It prints how many folders it took in, how it took in
their days, and how many days' summaries it copied in, whichever days they're of:

```
imported 1 folder: 27 days added, 3 merged, 12 past keeping left out, 39 days' summaries
```

Where it added and merged no day but copied in summaries, as of days whose lines are past keeping,
it says so:

```
imported 1 folder: 12 days' summaries, 12 days' lines past keeping left out
```

An import repeated adds nothing, and says `imported 1 folder: nothing new`, or, where the folder
holds days past keeping, tells of them left out again: `imported 1 folder: 12 days past keeping
left out`.

Against a router from before `import`, as one whose restart into an upgrade is still due, `import`
fails, saying to restart it with `switchboard service restart`, or, run by hand, to stop it and
run `switchboard serve` again, then import again.

One import runs at a time. An `import` run while another is running, as after you interrupt one
and the router carries on with it, is refused, and leaves the running one as it is:

```
Error: an import is running already
```

Where the router refuses an import, or a day can't be taken in, `import` fails, saying why. What
it took in before then stays, and importing again takes in the rest.

Example:

```bash
switchboard import ~/Dropbox/switchboard
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
- takes up the readings its history holds, so the recent rates it judges pressure by, and the
  rates the weeks' forecast goes by, carry on from before; and
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
- the binary it upgrades from: for the service's router, the Homebrew link `service install` was
  run by, which the LaunchAgent names with `--from`, and run by hand, the binary it was started
  as; and
- `/etc/localtime`, which says the Mac's time zone.

The service's router restarts when the config changes into one that's valid, when an upgrade leads
the link to another binary, or when the time zone changes. As it starts, it compares its own copy
with the binary the link leads to, by size and modification time, and, where the sizes agree but
the times don't, as on a disk that keeps coarser times, by their bytes. One that differs, as after
an upgrade made while launchd started it again, has an upgrade due from the start. `service
install` compares them the same way, and leaves a copy that matches as it is. To restart, it:

1. Waits until no [export](configuration.md#export-keys) is running, and for a moment with no
   request in flight.
2. For an upgrade, copies the binary the link leads to in beside its own copy, hidden, while no
   request is in flight.
3. Asks the binary it would become whether it can start from the config: the upgrade copied in,
   or else its own copy. Once an export ends, it can ask with a request still in flight, and keeps
   the answer.
4. Where a request is in flight by then, waits for the next moment with none, asking nothing
   again.
5. For an upgrade, renames the new version it copied in over its own copy.
6. Saves its state.
7. Replaces itself in place with its copy, handing it the sockets it listens on.

Some cases go differently:

- **A config that isn't valid** is logged, and the router carries on as it was.
- **A binary that refuses to start,** as a newer version can refuse a config the running one
  takes, holds the restart: the router carries on as it was, and says why in its log, its events
  and `status --json`'s `restart`. An upgrade copied in is removed, and the copy stays as it was.
  It asks again once the config or the binary changes. A binary from before the check can't
  answer, and the router restarts into it as before.
- **An upgrade it can't copy in,** as on a full disk, is logged once at warn: the router carries
  on as it was, the restart still due, and copies the upgrade again at its next moment with no
  request in flight.
- **A binary that isn't there,** as for a moment while `brew upgrade` moves the link on, is tried
  again for a couple of seconds. If replacing itself still fails, the router exits, and launchd
  starts it again.
- **Told to stop** while it finishes its requests to restart, the router stops then and there.
- **Run by hand,** the router logs, once, that a restart is due, and `switchboard status` says to
  run `serve` again.

A router launchd starts from a LaunchAgent written before the router had its own copy, with no
`--from`, moves over to the copy as it starts, once it has read the config and before it listens.
It makes the copy of the binary it was started as, following its links, writes the LaunchAgent
afresh to run the copy, as `service install` writes it, keeping its `--config`, its `--log-level`
and every variable it sets, and replaces itself in place with the copy, run as it was started with
`--from` naming that binary. The sockets a router before it handed over pass on to the copy
untouched, so after the upgrade that brings the copy, a request made meanwhile waits rather than
being refused. launchd isn't asked anything: it reads the LaunchAgent only as it loads it, so it
runs the copy from the next login on. Where the router can't move over, as when it can't write the
LaunchAgent or make the copy, it says why once at warn, leaves the LaunchAgent as it was, and
carries on from the binary it was started as, and `service status` names `service install`. It
tries again each time it starts without `--from`, as at the next upgrade or login. It puts moving
over off, saying so in its log, while `service install` is writing the LaunchAgent, which it leaves
to install, and where another upgrade has moved the link on as it started, so it never runs a
version that hasn't been asked whether it can start: it takes that upgrade up as any other, and the
new version moves over. It puts it off too where its binary couldn't be found as it started, as
for a moment while an upgrade moves the link on, as then which version it runs isn't known. A
router run by hand, or started with `--from`, never moves over.

## Help and version

### `help`

`switchboard help` lists the commands, and `switchboard help <command>` shows one command's help.

Every command also takes `-h` and `--help`. `claude --help` is Claude Code's own.

### `version`

`version` prints switchboard's version. `switchboard --version` and `switchboard -v` do the same.
