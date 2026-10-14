<div align="center">

# 🎛️ Switchboard

**Every Claude subscription you have, behind one `claude`**

A local router for Claude Code. It spreads your sessions across your Claude subscriptions, so a limit on one
<br>doesn't stop your work, and records every request, so you and Claude both see where your usage stands.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27+-00ADD8.svg)](https://go.dev)
[![Platform: macOS](https://img.shields.io/badge/platform-macOS-lightgrey.svg)](#install)

[Install](#install) · [Quick Start](#quick-start) · [How It Works](#how-it-works) · [Dashboard](#the-dashboard) · [Claude](#claude-and-switchboard) · [Commands](#commands) · [Configuration](#configuration) · [Reference](#reference)

<br>

<img src="art/routing.webp" alt="Switchboard's dashboard routing three accounts: the Overview's usage, routing and LATELY, a new session starting on side, then the routing card pinning new and running sessions to side, personal's two moving there" width="840">

</div>

---

## Why Switchboard?

Every Claude subscription has a 5-hour limit and a weekly one. Hit one and Claude Code stops. `/usage` says where you stand when you think to ask, but not where you're heading, and Claude can't see it at all. Moving to another subscription by hand means exiting, switching, resuming and rebuilding the prompt cache from cold, while the weekly quota you didn't reach before its reset is lost.

- **Claude sees its own limits.** A skill lets Claude read every account's usage, pace and resets, and plan its work around them.
- **Limits stop interrupting you.** A request that hits a limit is replayed on another account before Claude Code sees it, and the session carries on.
- **Sessions stay put.** A session keeps its account while its prompt cache is warm, and moves only when it must.
- **No quota goes to waste.** New sessions go to the account whose weekly quota would be lost soonest unused.
- **Resets come one at a time.** Priming staggers the accounts' 5-hour windows, so once all are spent, the next is back soon.
- **Every request on record.** Its session, model and account, why it went there, its tokens and timing, for a year: what your use was worth at API prices against what your plans cost, and each session's story.
- **Your usage, always in view.** A live dashboard: what's left in each window, when it runs out at your pace, when it resets, and the year behind it.
- **You and Claude both steer.** Pin new sessions to an account, or move running ones, from the command line, the dashboard, or by asking Claude.
- **Told in time.** Desktop notifications when a window passes a share you choose, an account hits a limit, or has room again.
- **Never in the way.** Without the router, `claude` connects directly; when switchboard can't take part, `claude` starts as it would without it.

Routing needs two subscriptions or more. With one, Claude seeing its limits, the record of every request and the dashboard are all still there.

Switchboard is built for its author's setup: macOS, Claude Code, and a Claude subscription or several. It's public because it can be, and general only where that costs nothing.

## Install

You'll need macOS, Claude Code, and a long-lived token for each subscription, made with `claude setup-token` while signed in to it. `switchboard setup` asks for the tokens.

**Homebrew**

```bash
brew install leeovery/tools/switchboard
```

**From source**

```bash
go install github.com/leeovery/switchboard/cmd/switchboard@latest
```

## Quick Start

```bash
brew install leeovery/tools/switchboard
switchboard setup       # accounts and their tokens, priming, the service, the claude link, the skill
claude                  # Claude Code, through switchboard
switchboard usage       # every account's windows, as they stand
switchboard dashboard   # every account's usage, live
```

`setup` walks through the rest a step at a time, and is safe to run again: each step says what's done already, and does only what isn't. It asks for each account's id, label and token, which account is the primary, and your day for priming; installs the service that keeps the router running; links `claude` to switchboard, and gives you the one line to add to your shell's startup file; and installs a Claude Code skill that tells Claude what switchboard does. See [`setup`](#setup).

## How It Works

```
Claude Code ──ANTHROPIC_BASE_URL──▶ switchboard ──▶ api.anthropic.com
                                      │
                                      ├─ chooses the account: sticky per session and model
                                      ├─ swaps in that account's token
                                      ├─ reads every response's usage headers
                                      └─ records the request, and what the router did
```

- **Every `claude` goes through switchboard,** which starts the real Claude Code connected to the router.
- **The body is untouched.** Switchboard swaps in the account's token and never edits a request's body, so it stays Claude Code's own.
- **Usage comes off real traffic.** Every answer says how full each window is, so knowing costs next to nothing.
- **Each session gets an account,** the one whose quota most needs using, and keeps it while its cache is warm.
- **A limit moves the session,** replaying its request on another account before Claude Code sees it.
- **You can pin,** from the command line, the dashboard or Claude, and every pin yields at a limit.
- **Everything is recorded:** each reading, request and event, a file a day, for the dashboard and the verbs to read back.
- **It looks after itself,** taking up new tokens and config, and restarting in place after an upgrade.

The detail of each is in [How it works, in depth](#how-it-works-in-depth), after the dashboard.

## The Dashboard

<div align="center">
<img src="art/dashboard.webp" alt="One account's dashboard: work alone on the Overview, its usage, routing trace and LATELY, then GRAPHS, then Runway's 5-hour, its lane running out at its pace to its cap, back at 17:10" width="840">
</div>

One account: work alone on the Overview, its usage, its routing trace and LATELY; then `g` to GRAPHS, and `tab` on to Runway's 5-hour, its lane running out at its pace to its cap, back at 17:10.

`switchboard dashboard` draws every page in one frame: the title row, with the tabs and the clock; under it, the status line; then, on every tab but the Overview, the tab's heading row, its name and a rule; the view; and the key line last.

- **The tabs:** `tab` and `shift-tab` move between six, in the title row's order: the Overview, Accounts, History, Sessions, Runway and the Log. The next dashboard opens on the tab last shown. A session's page opens over a tab, and `esc` goes back to it.
- **The status line** says how the router stands, always, whatever a tab shows: `● healthy`; `■ the router isn't answering  ·  since 14:40:55` while its last view stays on screen; or `○ probed · router not running`; then how it routes, `auto` or `pinned to personal`, the sessions it has routed in the last hour, and where new sessions go, `new sessions → ▲ side`.

**The Overview** is one screen that never scrolls, in six sections, in an order that never changes: USAGE, GRAPHS, ROUTING, SESSIONS, COMING UP and LATELY, each shown and hidden by its key, `u`, `g`, `r`, `s`, `c` and `l`, at any size, GRAPHS and SESSIONS hidden at first. Size never decides what shows, only how much room each section gets: where rows run short, sections fold to their headings, each still saying what it holds, SESSIONS first, then COMING UP and LATELY, GRAPHS and ROUTING; USAGE never folds, and with many accounts scrolls within itself. Rows to spare grow the sections shown, to their most.

- **USAGE** is [`usage`](#usage)'s block, every account's windows and the pool's: the matrix from 150 columns, or given the room, the windows stacked at the full width, their bars as long; narrower, stacked as `usage` stacks them. An account out of the pool, under a pin, is faint, and the key line under the block names the marks: `┃` even pace, `╎` the cap, `◆` the primary and `▲` where new sessions go.
- **GRAPHS** charts the focused account's use so far in each window, each chart under its window's column, from the window's start to its reset, with its cap, even pace and now. `↑` and `↓` move the focus, and USAGE lights the account with it.
- **ROUTING** says how each account stands with the router, in words: `● under pressure · out ~16:05`, `■ limit reached · back 15:54`, `○ idle · window starts at its prime, 16:20`; `● pinned` on each account the pin names; its sessions; and its requests a minute over the last 30 minutes as a trace, with the last minute's rate. Its heading says the mode, `auto`, or `pinned to personal since 14:02   p back to auto`. Every trace on the page, a session's too, shares one scale, so any two compare.
- **SESSIONS** lists the sessions the router has routed in the last hour, by account: each one's id, bright while it's busy; its model; when it was last seen; what it's doing, `↑ ask` or `↓ ~1.2k`; why it's on its account, as `moved from work 14:31: work reached its cap`; and its own trace.
- **COMING UP and LATELY** sit side by side: what happens next, soonest first, as `16:30  personal's 5-hour resets  in 1h 48m`, a run-out's row in the theme's attention colour; and what the router has done, newest first, each event marked, as `14:31  ╎ work reached its cap: 3 sessions moved to side`, the sessions a limit, a cap or a refusal moved folded into its line.
- **By account:** `b` makes USAGE a block an account, its windows as rows, with its ROUTING beside it, and the pool's block last.

**Accounts** is each account in depth and every account together, in two views on sub-tabs, `[` and `]` moving between them.

- **Compare** is a column an account and one for `all accounts`: its week as a bar, its share of use by model version, its requests, sessions served and moved, its limits hit, the time it spent at its cap or a limit and what it had left at its last reset, and its plan, its extra usage, and what the plan costs against what its use was worth at API prices. `←` and `→` move a focus along the columns, and `enter` opens the one focused in Detail.
- **Detail** is one account, or all of them, chosen from a list at its left: its windows now, the period's requests, limits, plan, worth and extra usage, its use by model, the last six weeks a day and a week at a time, its verdict as History's Weeks gives it, its SESSIONS now, `enter` opening one's page, and its LATELY.
- **The period:** `<` and `>` step the period both count over, today, this week, 30 days and 12 weeks, rolling, never billing months.

<div align="center">
<img src="art/accounts-compare.webp" alt="Accounts' Compare over this week: a column each for work, personal and side and one for all accounts, each with its week used, its use by model version, its requests, sessions and moves, its limits and time at its cap, and what its plan costs against the worth of its use at API prices, 4.0× to 5.2×" width="840">
</div>

**History** is how use has gone over time, in four views on sub-tabs, `[` and `]` moving between them.

- **Year,** the default, is a square a day over the last 12 months, a column a week, each shaded by how many requests it made, never its worth: a day with nothing on it in a shade of its own, and the active days in quarters by how much each did. Beside it, the 12 months' requests, sessions, limits hit, worth, what the plans cost and the worth against it, the busiest day and the longest run; under it, BY MONTH, a row a month, its bar its worth split by family. `v` shows one model version alone, in four steps of its own colour, or one account, `←` and `→` stepping between them; `<` and `>` step back through each calendar year the ledger holds. Shown for one version, the Year counts each of its sessions once, but for a day whose summary was written before the ledger kept a model's sessions by their ids, its lines since gone, it adds that day's count, as nothing tells those sessions from another day's.
- **Weeks** is how full each account's own week got over eight weeks, and whether there are as many accounts as the use needs, its verdict as [`history`](#history) words it, from replaying the last 7 whole weeks with an account fewer or more; `v` draws the weeks as columns.
- **Days** is the eight weeks a day at a time, each family's worth a day, and how much of its week each account used each day, the days it hit a 5-hour limit marked.
- **Tokens** is ccusage's report inside switchboard, each period's tokens by kind and its worth at API prices against what its plans cost, by day, week or month with `<` and `>`, split with `v` by account, by model, by both or not at all.

<div align="center">
<img src="art/history-year.webp" alt="History's Year: a square a day over the last 12 months, shaded by its requests; beside it, 212,885 requests worth $24,736 at API prices against $7,200 of plans; and BY MONTH under it, each month's requests, worth, ratio, limits and use by model family" width="840">
</div>

**Sessions** is where each session's requests are going now, in two views on sub-tabs.

- **The Switchboard,** the default, is a switchboard: the sessions down the left as calls, the accounts down the right as lines, and a cord from each call to its account, its requests travelling it, out as they go and back as their answers stream, a refusal bouncing back red, `✕ 429`. A session a limit moves joins its new account's group, and its old cord hangs red from the old jack, one cell shorter for each session the limit moved; a move made for any other reason hangs faint for 3 seconds. Where the cords don't fit, as on a phone, each account's line carries its sessions under it.
- **The List** is one table of today's sessions, those running first, then those ended: each one's account, model, state, when it started, its requests and their worth at API prices, and how it came to its account.

<div align="center">
<img src="art/repatch.webp" alt="Sessions' Switchboard as personal reaches its limit: the 429 back red along 41e0's cord, its old cord left hanging, and personal's sessions re-patched to side" width="840">
</div>

**Runway** is when each account has room, a lane each, in the 5-hour view, from the hour before now for 22 hours 40 minutes, or, with `<` and `>`, the week: a cell a column, room, or at its cap, or at its limit, the past dimmed, with words under a lane that say where a stretch without room starts and when it's back, `at its cap 14:31  ·  back 17:10`, or `runs out ~16:05 at its pace`. Under the lanes, with more than one account, the pool's: how many accounts have room, a column at a time. `←` steps back a day, or a week, and `→` forward to the live view; a step back is never kept.

<div align="center">
<img src="art/runway-week.webp" alt="Runway over the week: a lane an account to next Wednesday, work at its cap till 17:10, personal's and side's weeks running out at their pace before they reset, and the pool's room under them" width="840">
</div>

**The Log** is the router's traffic, as it happens and back through the days, in two tables under one tab, `[` and `]` moving between them, the newest line at the foot.

- **Requests** is a line for each request the router routes: when, its session, model and account, its status, how long it took, its tokens by kind, why its answer stopped, and how the router chose its account, a move's cost with it; `f` shows more columns, its worth at API prices, its input tokens, its attempts, its size, messages, tools and thinking budget, the client that sent it and the router's id for it, and Claude Code's quota checks and counts of tokens, hidden until then, as neither is work done for you.
- **Events** is a line for each thing the router decides or notices, every move it makes with why.
- **Its keys:** `↑` chooses a line and pauses the table, `end` follows again, `enter` opens a line's details, `s` its session's page, `f` sets what it shows, its columns and which lines, and `/` finds a session.

<div align="center">
<img src="art/log-requests.webp" alt="The Log's Requests, following the newest: a line a request, with its time, session and directory, model, account, status, time to first byte and in all, tokens read from the cache, written to it and put out, why its answer stopped, and a new session's routing, the requests in flight at the foot" width="840">
</div>

**A session's page** follows one session through its whole life: what it's doing now and where it runs; its STORY, its requests a minute from its first to now, each move marked and costed; the accounts it ran on and what it took of each; and its turns, newest first, `enter` opening one into its requests. It opens with `enter` on a session's row on Sessions, or on Accounts' Detail, with `s` on a line of the Log, and with a click on a row of the Overview's SESSIONS; `esc` goes back to the tab as it was left. While the session runs, `p` on its page pins it.

<div align="center">
<img src="art/session.webp" alt="A session's page for d28c in ~/Code/switchboard: its STORY of requests a minute, its move from work to side at 14:31 marked with the $1.82 its cache cost, its time and worth on each account, and its turns, the one that moved it opened into its requests" width="840">
</div>

**Under 100 columns,** the dashboard is a phone's: the tabs on a row of their own, under the clock. The Overview's USAGE stacks its windows in short words, ROUTING and SESSIONS take two rows an account or a session, and LATELY comes under COMING UP.

<div align="center">
<img src="art/phone.webp" alt="The Overview in a phone's terminal, 52 columns wide" width="260">
</div>

It reads, and takes its keys, as [`dashboard`](#dashboard) says, and `?` on any page shows every key that works there, and what its marks mean.

<div align="center">
<img src="art/help.webp" alt="The Overview's help: every key that works there, and what its marks mean" width="840">
</div>

### Themes

<div align="center">
<img src="art/themes.webp" alt="The theme picker over the Overview, the dashboard redrawn in each theme the cursor reaches: nord, terminal, tokyo-night, tokyo-night-day, exchange and amber, then nord again" width="840">
</div>

The dashboard is drawn in a theme. Themes name colours by what they mean and how prominent they are, never by hue.

- **Built in:** `nord`, the dashboard's palette from the start; `tokyo-night`, and `tokyo-night-day` for a light terminal; `amber`, an amber CRT; `exchange`, a telephone exchange's brass and walnut; and `terminal`, for a terminal with a transparent or image background, which paints no background and draws in the terminal's own sixteen colours.
- **One, or a pair:** choose one theme for every terminal, or a pair: one for a light background and one for a dark. The dashboard asks the terminal what its background is as it starts, and takes a terminal that doesn't say for a dark one. Choosing nothing gives the pair of `tokyo-night-day` and `nord`.
- **The picker:** in the dashboard, `t` opens a panel at the right, over the dashboard, listing every theme. `↑` and `↓` move through them, the dashboard redrawn in each as it's reached; `enter` sets the one theme; `d` and `l` set the dark and the light half of the pair, asking first, with `y` or `n`, to clear a single theme; and `esc` closes it, putting back the theme in force. A badge says what each theme fills: `●` the one theme, `● light`, `● dark`, or `● both`. A theme file that doesn't load is listed with why, such as `bad colour` or `missing tokens`, and the log has the detail.
- **Your own:** a theme is a file named `<slug>.theme`, its slug lower-case letters, digits and hyphens, in the themes directory (see [Where things live](#where-things-live)), its top level alone; links are followed, and it's read again each time the picker opens. Each line is `key = #RRGGBB`, a line starting `#` a comment, and every key once:

  ```
  # Lake: a dark theme.
  text.primary = #ECEFF4
  canvas = #102030
  ...
  ```

  It gives all 19 base tokens, `text.primary`, `text.secondary`, `text.tertiary`, `text.muted`, `text.subtle`, `text.faint`, `text.on-selection`, `accent.primary`, `accent.key`, `accent.mode`, `accent.attention`, `state.positive`, `state.destructive`, `canvas`, `bg.selection`, `bg.attention`, `bg.subtle`, `border` and `text.on-attention`, or it doesn't load. The charts' own, `viz.ramp.1` to `viz.ramp.4` (a bar's fill, first cell to last), `viz.track`, `viz.pace`, `viz.reserve`, `viz.family.1` to `viz.family.4` (the model families', Opus, Sonnet, Fable and Haiku), `viz.version.1` to `viz.version.6` (the model versions', a family's newest in its family's colour and an older one fainter) and `viz.day.0` to `viz.day.4` (History's shades of a day, from one with nothing on it to the busiest), are each worked out from those where a file leaves them out. A key switchboard doesn't know is passed over. A file can't take a built-in's slug.
- **Kept:** the choice is kept in `prefs.json` in the state directory, which the dashboard writes and you never need to; one that can't be read is set aside as `prefs.json.corrupt-<unix time>`, and the defaults stand.
- **The background:** the dashboard paints the theme's background on every cell, and sets the terminal's own to it, putting it back as it was when the dashboard stops: on `q`, an interrupt or a terminate signal, or a crash it catches. A terminal that didn't say what its background was is reset to its profile's own instead. [`usage`](#usage) paints none, printing into the scrollback: it asks the terminal what its background is, and takes the light or dark half of the pair by it; one theme chosen prints only on a background as dark or light as its own, else the default pair's half for that background.
- **Fewer colours, and none:** the colours are brought down to what the terminal shows. With `NO_COLOR` set to anything, the dashboard has no colour, nor background: what colour would say, its glyphs and bold say, and `t` does nothing. `usage` has none either, there or on a terminal that shows no colour, its bars filled in `█`; printed off a terminal, as `usage --pretty` prints it, it has none unless `CLICOLOR_FORCE` asks for it.

## Notifications

While it runs, the router posts desktop notifications, whether or not a dashboard is open. [`[notifications]`](#notifications-keys) says which:

- **Limits:** an account hits a limit. The notification waits 5 seconds for the sessions the limit moves, then tells of them together: `work · Work hit its Session limit, back at Mon 18:10 — 3 sessions moved to side · Side`.
- **Room again:** `work · Work has room again`.
- **Warnings:** a window passing the share given, once a reset: `work · Work: Week at 91%`.
- **Moves**, off unless asked for: every other move, such as `session 18bb978f moved from work · Work to side · Side (rescored after 1h 2m idle)`.

A limit's notification always goes out. Any other goes out only a minute or more after the last posted about its account, and is dropped otherwise. Notifications never hold a request up.

The dashboard, probing without the router, posts its own, of an account with room again and a window passing the warning, unless `--no-notify`. While the router runs, the dashboard posts none, so nothing is told twice, even probing with `--probe`.

## Claude and switchboard

`setup` installs a Claude Code skill that tells Claude what switchboard does under `claude`, so no session needs it explained (see [`setup`](#setup)).

- **Claude reads the limits itself.** `switchboard status` gives it every account's windows: how much of each is used, how fast it's going, when it runs out at that pace, and when it resets. So Claude can plan its work around them. Ask it to use what's left of a week before the week resets at 08:00 without reaching 100%, and it paces its work through the night by the rate and run-out switchboard reports. It's worth it with a single account.
- **It looks back.** `sessions`, `requests`, `history` and `events` give it your sessions, their requests, your days and what the router did, with what each was worth.
- **It steers when you ask.** It pins new sessions to an account with [`pin`](#pin), moves running ones there with `--move`, or pins one session alone with `--session`; inside a session, `$CLAUDE_CODE_SESSION_ID` holds its own id, and `switchboard status --session` names the account it's on.
- **JSON without asking.** Run by an agent, `status`, `usage`, `sessions`, `requests`, `history` and `events` print JSON with no flag, as anywhere their output isn't a terminal. What each prints is built by the same code its view on the dashboard draws from, so the two never disagree, and nothing a view shows is out of Claude's reach.

## How it works, in depth

### Routing

- **Every `claude` goes through switchboard.** `setup` puts a link named `claude` in switchboard's own bin directory, which you put on `PATH` ahead of the real one. Run by that name, switchboard starts the real Claude Code connected to the router: from a shell, a tmux pane, a script, or a tool that runs `claude -p`.
- **A token swapped, the body untouched.** Switchboard replaces the `Authorization` header of each conversation request (`/v1/messages` and its `count_tokens`) with the chosen account's token, and never edits a request's body, so the request is still Claude Code's own, and Claude's thinking stays valid from turn to turn.
  - It narrows the request's `Accept-Encoding` to gzip, deflate and identity, the encodings it can read a copy of the answer in, to count the answer as it streams.
  - Everything else passes through on Claude Code's own token, as it came.
  - Either way, every header of switchboard's own, named `X-Switchboard-…`, is taken off on the way, so the API never sees one: the pin `run --account` sets, the directory `run` names for the ledger, and any a newer `run` sends.
- **Usage off real traffic.** Every response carries each window's utilization and reset: the 5-hour window, the weekly window every model shares, and a model's own weekly window where it has one. Only a 429 that refuses the request itself carries none. Switchboard reads them off every response, so looking costs next to nothing.
  - It probes an account, one request per model family, each capped at one output token, only when nothing has been read of it, a decision needs fresher numbers than traffic has given, a dashboard asks, it's due a [prime](#priming), or on the router's own [rounds](#priming): once nothing has read it in half an hour, and 5 minutes before each of its weeks resets.
- **Choosing an account.** A new session goes to the account whose quota most needs using, among those with room in every window its model counts against: the room left in the shared weekly window, divided by the hours until it resets.
  - So quota that resets tomorrow is used before quota that resets next week, and an account with little left scores low unless its week resets soon.
  - Among accounts scoring at least 0.8 of the best, the one whose 5-hour window resets soonest wins, as what's left in a window at its reset is lost; with [priming](#priming), the accounts' resets are spread through the day.
- **Sticky, for the cache.** Prompt caches are per account, and the first turn after a move costs around 40× a warm one.
  - A session is remembered by its session id and model once a request of it is answered with success, so a resumed session finds its account again, and a request under an id never used again that fails, as the quota check `claude --resume` sends as it starts can, leaves nothing behind.
  - It stays on that account while its cache is warm, for an hour after its last request, and the account has room.
  - Idle past the hour, its cache is cold and a move costs nothing, so it's re-scored, keeping its own account unless another beats it by 20%. A Claude Sonnet 5.5 session isn't re-scored for idling: its thinking works only on the account that produced it, and a move would lose it.
- **Pressure.** Several busy sessions on one account run its 5-hour window out together, then all move at once, each rebuilding its cache elsewhere. So the router watches each account's 5-hour window, and steers sessions off one heading for its limit:
  - **The rate** is how fast its use rose over the last 30 minutes, or across a longer gap in its readings, spread over the gap, as use outside the router shows only when the router next reads the account, so an account gone quiet reads quiet; for a window with no reading from 10 minutes back, it's its use since the window started.
  - **Under pressure** is an account whose window, at that rate, reaches its reserve before it resets, or its limit, without a reserve or on an account pinned.
  - **What moves:** a new session, a request without one, or a session chosen afresh after an idle hour, because its account can't serve it or because its own pin yields, goes to the best of the others, the pin's first. Running sessions stay put, a session's own pin holds, and when every account is under pressure, it changes nothing.
- **Limits and refusals.**
  - **A limit:** a 429 that says a limit is reached is replayed on the next candidate before any of the answer reaches Claude Code, and the session moves there and stays, with one slower turn while its cache rebuilds there, and no exiting or resuming. The account sits out of the requests the limit counts, so a limit on Fable's own week leaves its other models' sessions where they are, until the reset the 429 gives, or for five minutes when it gives none, or sooner when a request sent since shows it lifted, as after you reset a limit by hand.
  - **Throttling:** a 429 that's only throttling waits and retries on the same account, twice at most, as moving would throw the cache away for nothing.
  - **A 429 without usage headers** says nothing of the account, but refuses the request itself, so it reaches Claude Code at once, as it came.
  - **A refusal:** a request the API refuses is replayed elsewhere too, and the refusal never relayed, as Claude Code drops its login on a 403. An account whose token the API refuses sits out every request for ten minutes, and one that refuses the request itself sits out that model's requests as long, unless every account the request went out on refused it, which says more of the request than of the accounts, so none sits out for it, and its session stays where it was.
  - **Nowhere to go:** when no account has room, Claude Code gets a 429, as it would from one account at its limit; when every account has refused the request, a 502 that gives the API's reason and tells it not to retry.
- **Pins.** `switchboard pin` sends new sessions to one account, or to the best of several, or moves running ones there; `pin --session` pins one running session; `switchboard run --account` pins a session as it starts. The dashboard's routing card sets the same pins, and Claude does when you ask. Every pin yields at a limit rather than failing. See [`pin`](#pin).

### What it records

Switchboard keeps a record of everything the router reads and does, in the state directory, so the dashboard and the verbs can look back, with or without the router. The formats are in the [Reference](#reference).

- **State that outlasts restarts.** The router keeps each session's account, the pins and each account's last readings in `state.json`, so a restart doesn't scatter sessions or need a probe.
- **A readings history.** Each reading that changes how a window of an account reads, and nothing when nothing moved, is appended to a file a day in `history/`, kept for 400 days, or as long as [`[history]`](#history-keys) says, `forever` included. It's there for you to look back at how the accounts were used.
  - As it starts, the router takes up the readings of its two newest days, so the recent rates outlast a restart.
  - It never stands in routing's way: a line it can't write goes unwritten, logged once.
- **A request ledger.** A line for each request the router routes, as it finishes with it, whether it went upstream or the router answered it itself, is appended to a file a day in `ledger/`, kept for 400 days, or as long as [`[ledger]`](#ledger-keys) says, `forever` included, and never pruned before its day's summary is written.
  - It keeps everything about a request but its content: its session, model and account, why it went there, its timing, its shape and settings, and the answer's usage, but never its messages, its system prompt or its tools.
  - It never holds a request up: the lines wait, 8,192 at most, for a goroutine of their own to write them, and a line it can't write goes unwritten, logged once.
  - An hour after each day ends, giving the requests still in flight then time to end, the router summarises the day beside its lines, each account's requests, sessions, moves, limits and highest use, and the summary is kept for good.
  - [`requests`](#requests), [`history`](#history) and [`sessions`](#sessions) read it all back, with or without the router.
- **The router's events.** Beside the ledger's lines, the router files its events, a JSON line for each version of one, so they outlast a restart: a session started or moved, an account under pressure, at its cap, at its limit, refused, primed or with room again, routing set by hand, a restart falling due, and its own health turning. [`events`](#events) reads them back.

### Looking after itself

- **Tokens.** The router takes up a change to a token file as it comes, and a token file caught empty while it's rewritten doesn't cost its account its token.
- **Sleep.** Once your Mac wakes, it sends the requests it routes upstream on fresh connections, as a sleep can leave those it kept dead; probes go out on connections of their own, each given 5 seconds.
- **Restarts.** The service's router restarts itself, once no request is in flight, when its config changes, `brew upgrade` replaces it or your Mac's time zone changes, so `accounts add`, an edit by hand, an upgrade and a new time zone all take effect without a command; a router started by hand with `serve` logs that a restart is due instead.
  - It restarts in place, running its new binary in the same process and handing it the sockets it listens on, so a request made meanwhile waits a moment rather than being refused, and launchd isn't asked to start an upgraded binary afresh, which macOS has been seen to refuse, where it lets the router run it in place, as the first such upgrade showed.
  - With many long sessions, a moment with no request in flight can be hours coming: `switchboard status` and the dashboard say while a restart is due, and `switchboard service restart` has it now. See [`serve`](#serve).

### The primary

One account is the primary: the one your browser and the Claude apps are signed into. `primary = true` marks it; without it, the first account is the primary. Remove the primary, and the account marked, else the first, is the primary from then on, as `accounts remove` says; the sessions already running, which hold the removed one's token, stay routed while the new primary has a usable token.

Artifacts live on the primary. Claude Code's own token is the primary's in every routed session, whichever account the conversation goes to, a pinned session included: a pin moves the conversation alone. (While the primary's token isn't usable, it's another account's: see [`claude`](#claude).) So what Claude Code sends that isn't the conversation, such as publishing an artifact or uploading a file, goes out on the primary, and every session's artifacts open in a browser signed into it.

### Reserves

Any account can keep a reserve, leaving a share of it for use outside the router, as the primary might for the Claude apps.

- **The reserve** is the share of every window the router leaves unused: `0` unless the account's `reserve` sets another, so with none set, the router runs every account to its limit. Once a window reaches its reserve, at 90% with a reserve of `0.1`, the router's own choices pass the account over: new sessions skip it, and sessions on it move as at a limit. The router never spends a reserve, even when no other account has room.
- **Pins spend it.** The reserve holds back the router's choices, and a pin is yours: a pinned account runs to its limit. So when an account is at its reserve and every other is out, `switchboard pin <account> --move` carries the running sessions on there, and `switchboard pin auto` hands them back to the router, reserve and all.

Readings come off responses, so one large turn can take an account a point or two past its reserve before the router sees it, and use outside the router, such as in the Claude apps on the primary, takes it further, as intended.

### Priming

Anthropic's 5-hour window starts with an account's first message after its last window ended, and resets five hours later. Left alone, an account's first window starts with the day's first request on it, so an 08:00–23:00 day meets three of its windows, and accounts started together come back together. With a day set in [`[prime]`](#prime-keys), switchboard starts each account's window at a time of its own, so they come back one at a time: once all are spent, the wait for the next is at most 5 hours ÷ the number of accounts.

With N accounts that have usable tokens, resets fall every 5 hours ÷ N, the first half a step after the day starts, and each account, in config order, is primed five hours before its first reset, taken back to the ten-minute mark that falls in, and the resets it predicts with it: every reset seen falls on one, as the API takes a window's start back to its mark, so the schedule shows where each window really starts and resets. With two accounts, the first is primed at 04:10 and resets at 09:10, not at 04:15 and 09:15. Taken back, never on, a slot stays before the day starts, and the slots stay as evenly apart as the marks allow. Every prime falls before the day starts, so your first requests don't disturb the schedule, and goes 5 seconds after its slot, so it lands within the slot's ten minutes though the Mac's clock runs a little ahead of the API's. For a day starting at 08:00:

| Accounts | Primed | Resets |
|---|---|---|
| 4 | 03:30, 04:50, 06:00, 07:20 | 08:30, 09:50, 11:00, 12:20, 13:30, 14:50, 16:00, 17:20, 18:30, 19:50, 21:00, 22:20 |
| 3 | 03:50, 05:30, 07:10 | 08:50, 10:30, 12:10, 13:50, 15:30, 17:10, 18:50, 20:30, 22:10 |
| 2 | 04:10, 06:40 | 09:10, 11:40, 14:10, 16:40, 19:10, 21:40 |

Each account then meets four windows in an 08:00–23:00 day. The cost is short windows at the day's edges: with three accounts, the first has 50 minutes of its first window left at 08:00, and the third's last window starts at 22:10.

- **A prime** is a probe, one request per model family, Haiku and Fable (falling back to the previous Fable), each capped at one output token, sent to an account whose 5-hour window isn't running. An account whose window is already running, as after a late night, gets none, and its slot shifts for the day. A prime that reads nothing, or doesn't start the window, is logged as a warning and sent again five minutes on. An account that can take no request anyway, or whose token is refused, isn't primed while it's so, as a prime couldn't start its window.
- **Through the day,** whenever an account's window resets, in use or not, the router primes it 5 seconds on, so its windows run back to back, a prime being sure to land after the reset though the Mac's clock runs a little ahead of the API's. After the day ends it stops, and the windows lapse overnight, but for those the router's rounds start where they reset before the account's slot.
- **A prime missed** while the Mac slept, or the router was away, goes out as soon as it can, unless the day has ended.
- **No accidental windows.** A probe is a request, so probing an idle account starts its window. The router never probes an account whose 5-hour window has lapsed except to prime it, on its rounds where the window it starts resets before the account's slot, or while it can take no request anyway, as while a limit holds back its every request or its week reads spent: starting its window costs nothing then, and a probe is how the router sees a limit you've reset by hand. An account nothing has been read of has no window known to have lapsed, so the router probes it as it starts, and, should that fail or the account be given its first token while the router runs, on its rounds, or sooner when a decision or a dashboard needs it, at any hour, which may start its window off the schedule, once. `usage --probe` and `dashboard --probe`, and `usage`, `status` or `dashboard` without the router, probe every account, as asked, and `accounts add`, `accounts token` and `setup` probe a token they're given, to check it.
- **The router's rounds.** Every answer the router relays carries its account's whole usage, from wherever it was used: your phone, claude.ai, Claude Code run directly. So an account in use through the router is always current, and the router reads the others itself, around the clock, dashboard or not: each account nothing has read, by an answer, a probe or a prime, in the last half hour. One whose 5-hour window has lapsed and that can take a request is probed, starting its window, only where its slot is more than 5 hours off, so that window resets before its prime is due; without priming, it's probed. One that can take no request anyway is probed at any hour, so a reset you make by hand is seen within the half hour. Each account is also probed once in the 5 minutes before each of its weeks resets, whatever the schedule, unless something read that week in them, so a week's last use is read before it goes; where that starts a lapsed window, the account's prime is skipped as for a window already running, and its schedule settles back by itself. Routing comes first, then the readings' accuracy, then priming. A probe that reads nothing is tried again 2 minutes on, doubling to the half hour, until it reads the account or the account goes out on another token, and a probe due while your Mac slept goes out within a minute of its waking.

`status` shows the schedule. `status` and the dashboard show the next reset among the 5-hour windows and, from the router, the next prime, and a 5-hour window that hasn't started says when its account is next primed.

### What doesn't go through the router

Claude Code's own token is the primary's, so what isn't routed lands there:

- **Fast mode** ignores `ANTHROPIC_BASE_URL`, and isn't supported. **WebFetch's site checks** ignore it too; they're small.
- **Claude Code's own `/usage` and `/status`** report the primary, not the accounts the conversation went to: `switchboard usage` and `switchboard status --session` do.
- **Account-bound features**, such as remote sessions and file uploads, pass through untouched, on the primary.
- **Programs whose `PATH` lacks the link's directory**, such as launchd jobs and some GUI apps, find the real `claude`, and aren't routed.
- **Extra usage:** an account with extra usage turned on may serve, and bill, the request that crosses its limit, rather than refuse it. The router moves sessions off once the headers show the window spent.
- **A direct launch**, without the router, can spend the reserve of the account it goes out on.
- **Claude Code with an API key:** with `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` set, Claude Code may use the key rather than an account's token, so `claude` starts without switchboard, and its requests go out on the key (see [`claude`](#claude)).

The full design, including the cache facts it rests on, is in [docs/design.md](docs/design.md).

## Commands

Every command takes `--config <file>`, naming the config file in place of the one switchboard finds (see [Configuration](#configuration)), and `-h`, `--help`. `switchboard --version`, `-v` and [`switchboard version`](#version) print the version. A command that needs the router fails without it, saying how to start it. No command repeats a token pasted where an account's or a session's id goes, nor in refusing an argument or a flag's value: it shows as `[redacted]`. Pasted where a command goes, as `switchboard <token>`, or as `help`'s topic, as `switchboard help <token>`, it's repeated as given, in saying there's no such command or topic.

### Everyday

#### `usage`

Every account's windows as they stand, printed once into the scrollback with the time they were read, to hold against one printed an hour before. A header, with when it read and the router's status, `● healthy  ·  6 sessions  ·  new sessions → ▲ side`, with `pinned to personal` while a global pin holds; then a bar for each account's every window, and last, the pool; then a line saying what the marks mean. It reads the router's status document while the router answers, else probes each account. Where its output isn't a terminal, as in a pipe or an agent's shell, it prints the status document as JSON instead, as [`status --json`](#status) does, read as its flags say.

```bash
switchboard usage [--5h] [--week] [--<model>] [--by window|account] [--probe] [-r] [--json | --pretty]
```

| Flag | Description |
|---|---|
| `--5h`, `--week` | print that window alone, with the header and the key line; given together, the windows given, in Claude's order: the 5-hour window, the week, then each model's own |
| `--<model>` | print a model's own window, as `--week` does the week: a flag for each model with a window of its own, named for it, as `--fable` is for Fable's week. A flag for a window no account reports fails, naming the windows there are |
| `--by window\|account` | lay the windows out by window, the accounts compared a window at a time, or by account, each account's windows together; over [`[usage] by`](#usage-keys), and by window where neither says |
| `--probe` | probe every account, even while the router runs |
| `-r, --refresh` | have the router first read every account it may, as the dashboard's `R` does, and wait for it, ten seconds at most; without the router, or with `--probe`, every account is probed anyway |
| `--json` | print the status document as JSON, as it does off a terminal anyway, even on one. The window flags and `--by` shape the printout alone, and the JSON is the whole document whatever they say |
| `--pretty` | print the bars, as it does on a terminal, even off one: without colour, unless `CLICOLOR_FORCE` asks for it, as wide as `COLUMNS` says, else 80 columns |

- **A row** an account, in the config's order: its number, its place in the config, its label, `◆` on the primary and `▲` on the account new sessions go to; the window's bar, filled along the theme's ramp, `┃` where even use would have put it by now and `╎` where its cap starts, 1 less its `reserve`; its percentage; and the first of these that holds: `limit · back 15:54` while Claude's limit holds the window; `at cap · back 17:10` once its use reaches the cap and the cap holds the account back; `out ~Sat 06:00` where it runs out before its reset, at its recent rate; `not started`, of a 5-hour window not yet started; else its allowance, what can be spent of it an hour or a day and still last to its reset, `19%/day till Sun 21:00`, or under an hour from the 5-hour window's reset, or a day from a week's, the room itself, `34% till 16:30`. An account with no usable token reads `no token`, and one nothing has been read of `not read yet`, their bars empty; one whose token is refused, or whose last read failed, shows its last reading faded, `not read since 14:20`.
- **The pool,** wherever there's more than one account, is the room new sessions can go to, summed across the accounts in it: every account on auto, only those pinned when there's a global pin. Its row, `Σ pool`, comes last, under a rule, its words the room left in accounts' worth, `1.5 of 3 left`, and pinned, ` · pinned`, an account out of it numbered faintly.
- **Every window shows,** used or not: a model's own window, as Fable's week, on every account, at 0% as at 40%.
- **The width** shapes it. By window, from 150 columns the windows sit side by side, a column each, and narrower, or where a bar would be under 12 cells, as with four windows at 160 columns, they're stacked, a section each. By account, each account is a block, its windows as rows, its name beside its first bar from 110 columns, else on a line of its own above it. The header is one line from 120 columns, else two, and the key line one from 150, else two. Under 70 columns the words shorten, to `back 15:54`, `out ~Sat` and `19%/day · Sun`. A label too long for the name column is cut with `…`, never moving as `▲` moves between accounts.
- **Colour:** it's drawn in the dashboard's theme, brought down to what the terminal shows, with no background of its own (see [Themes](#themes)). With `NO_COLOR` set, or on a terminal that shows no colour, a bar's fill is drawn in `█`, its track left as spaces, and the names and percentages keep their bold.

```bash
switchboard usage                  # every window, every account
switchboard usage --5h             # the 5-hour window alone
switchboard usage --by account     # each account's windows together
switchboard usage --probe          # read every account from the API, whatever the router says
switchboard usage -r               # have the router read every account it may first, as after a reset made by hand
```

#### `dashboard`

Every account's usage, full screen, until `q`, on six tabs: the Overview, every account's windows read with the router's, in sections you show and hide; Accounts; History; Sessions, where each session's requests go; Runway, when each account has room; and the Log (see [The Dashboard](#the-dashboard)). It needs a terminal, and refuses without one. It reads the router while it answers, else probes each account. The interval, given after the command, is how often it reads in full: `30m` unless given, `5m` at the least; a duration such as `15m` or `1h`, or a number of minutes.

```bash
switchboard dashboard [interval] [--no-notify] [--probe]
```

| Flag | Description |
|---|---|
| `--no-notify` | post none of the desktop notifications it posts while it probes without the router (see [Notifications](#notifications)) |
| `--probe` | probe every account each interval, even while the router runs |

Reading the router, it looks at the router's view every 5 seconds, which costs nothing upstream, and every interval has the router probe the accounts it hasn't read in that time, beside the router's own [rounds](#priming), every half hour. Should the router stop answering, or a read of it fail, its last view stays on screen, the status line saying `■ the router isn't answering  ·  since 14:40:55`, until it answers again or the interval's read, or `R`, probes the accounts directly. Without the router, it probes every account every interval, sooner after a window on screen resets or an account couldn't be read, and goes back to the router once it answers.

| Key | Does |
|---|---|
| `tab` | the next tab, in the title row's order and round: the Overview, Accounts, History, Sessions, Runway and the Log (see [The Dashboard](#the-dashboard)); `shift-tab` the one before; from a session's page, the tab after or before the one it was opened from; the next dashboard opens on it |
| `p` | the routing card, where new sessions go, or on the page of a session that's running, where that session goes: see below. With one account there's nothing to pin to, and `p` does nothing, unless a pin set with [`pin`](#pin) stands, to clear it |
| `R` | refresh now: the router probes the accounts it hasn't read in the last minute, and those that can take no request however lately it read them, but for those whose 5-hour window has lapsed and that can take a request, and none twice in a minute; without it, every account is probed |
| `t` | the theme picker: see [Themes](#themes) |
| `u` `g` `r` `s` `c` `l` | on the Overview, show or hide a section: USAGE, GRAPHS, ROUTING, SESSIONS, COMING UP and LATELY; the next dashboard shows those shown |
| `b` | on the Overview, group the windows by window or by account, as [`[usage] by`](#usage-keys) has them until `b` is pressed; the next dashboard groups them so |
| `↑` `↓` | on the Overview, move the focus between the accounts, in the config's order: GRAPHS charts the account with it, and USAGE scrolls to it |
| `[` `]` | on Accounts, History, Sessions and the Log, the sub-tab before or after, round: Accounts' Compare and Detail, History's Year, Weeks, Days and Tokens, Sessions' Switchboard and List, and the Log's Requests and Events; the next dashboard opens on the one last shown, but for the Log's |
| `<` `>` | on Accounts, the period both its views count over, a shorter or a longer: today, this week, 30 days and 12 weeks; on History's Year, back through each calendar year the ledger holds and forward again to the last 12 months; on Tokens, the period each row is, a day, a week or a month; on Runway, the 5-hour or the week view; the next dashboard opens on the period or window each last showed, but for a calendar year |
| `v` | on History, what the Year shows, all use, a model version or an account; Weeks' shape, a table or columns; and how Tokens splits each period, by account, by model, by both or not at all; the next dashboard shows them so |
| `←` `→` | on Accounts' Compare, the focus along the accounts' columns, scrolling them where they don't all fit; on Accounts' Detail, `→` into the chosen account's sessions and `←` back to the list; on History's Year, shown for one account, the account before or after, round; on Runway, back to today, then a day at a time, or to this week, then a week at a time, and on again to the live view |
| `↑` `↓` | on Accounts' Detail, the account chosen in the list, `all accounts` first, or, in its sessions, the session; on Sessions, a session, a call on the Switchboard or a row on the List; on the Log, a line, `↑` from following choosing the newest and pausing, and with its details open, the line before or after; on a session's page, a turn, or with one open, a request of it |
| `enter` | on Accounts, Sessions, the Log and a session's page, open what's chosen: from Compare, the account in Detail; a session's row, its page; a line of the Log, its details; a turn, its requests, in place, and with a turn open, the chosen request's details |
| `end` | on the Log, follow again: the newest line at the foot, and nothing chosen |
| `s` | on the Log, the page of the chosen line's session, but not of an account's event |
| `g` | on the details of an event in the Log, the Requests, paused at the event's moment |
| `f` | on the Log, what it shows: the panel, where `←` `→` move between its sides, `↑` `↓` through a side's ticks, `space` ticks the one the cursor's on, `enter` applies them, `esc` closes it unchanged, and `/` closes it for the find |
| `/` | on the Log, find a session by the start of its id or its directory's name, today's first, then back a day at a time; `↑` `↓` and `enter` pick one, filtering both tables to it |
| `j`, `k`, `PgDn`, `PgUp`, the wheel | scroll the view, where it doesn't fit: a row, a page, or three rows a turn of the wheel. On the Overview, USAGE scrolls, where its accounts don't fit; and on History, the Year, where the screen is shorter than it, Weeks' accounts, Days' BY ACCOUNT, past six accounts, and Tokens' periods: USAGE, Weeks and Days an account at a time, and Tokens a period, a turn of the wheel too |
| `?` | the page's help: every key that works on it, what each part of it shows and how to read it, and what its marks mean, in a card over the page; `j` and `k` scroll it, and `?` or `esc` closes it |
| `esc` | close what's open, the newest first: a card, the theme picker, the Log's panel, the Log's find, or a turn opened; else end a choice; on the Log, with a filter on and nothing open, clear it; else, on a session's page, go back to where it was opened from |
| `q` | quit, but while a card, the Log's panel or its find is open, whose keys alone act; `ctrl-c` quits wherever it is |

- **The routing card** is the only way the dashboard changes routing, and it changes nothing until `enter`. `p` opens it over the page, with nothing picked but what's set now: a row for `0 auto` and one for each account, numbered by its place in the config, with whether it's open, its 5-hour window and week, and why it's closed, as `at its cap till 17:10`, or its sessions running. `↑` and `↓` move between the rows, `space` picks the one it's on, and a number picks its row directly, `0` auto, which clears the accounts, as an account clears auto; `m` ticks moving the sessions running elsewhere there too, the card saying how many would move and what rebuilding their caches costs, `≈ $6.20`, roughly; under it, in a sentence, what the picks do. `enter` applies them, as [`pin`](#pin) does, the router's event naming the dashboard as what set it, and `esc` closes it with nothing changed. An account that can't take a session, at a limit, its token refused, or with none, can't be picked. Outside the card, the number keys and `m` do nothing, so a stray key moves no session; while the router doesn't answer, `p pin` is drawn faint, and `p` says so instead. On a running session's page, `p` opens that session's own card, to pin the session to an account or back to auto, with no `m`.
- **A card over a page,** the routing card or help, is drawn with the page faded behind it, standing still while the card is open, the card's own figures following each read.
- **The key line,** the last row, lists the keys that work on the page: `tab views` first, then the page's own, `p pin` where a pin can be set, and `? help  q quit` last. A key that can't act now is faint, and pressed, says why in the line's place for 4 seconds. `t`, `R` and the scrolling keys work everywhere and are left to `?`. The Overview's lists its toggles, each word faint while its section is hidden, then `b by: window`, then `↑↓ focus  p pin  ? help  q quit`. On a phone, or wherever the line doesn't fit whole, `? keys` and `q quit` end it, `? keys and toggles` on the Overview, and `tab views` leads it while it fits.
- **Clicks:** the mouse is on while it runs, so selecting text takes the terminal's modifier: Option in iTerm2 and Terminal.app, Shift in most others. A click on a tab or a sub-tab shows it, on `<` or `>` by a period or a window steps it, and on a key of the key line does what the key does, a pair's, as `[ ]` or `< >`, its second's, but `p pin` and `q quit`; on a row that names an account, a session, a line or a turn, or a Compare column's head, it chooses it, as the arrows would, and on the one already chosen opens it, as `enter` does, but a row of the Overview's SESSIONS opens its session's page at once; on the Overview, a click on a section's heading hides it, and on an account's row in USAGE or ROUTING moves the focus there; on the Log, a click on `▼ 214 new` follows again, and on a tick in its `f` panel ticks it; the wheel scrolls what's under the pointer. No click opens the routing card or acts in it, so a stray click never moves a session.
- **What's remembered,** in `prefs.json` in the state directory, as it changes: the theme or the pair, the tab shown, the Overview's sections shown and its `b`, and of each view what its keys last left, the sub-tab, the period or window and `v`'s choice: Accounts' view and period, History's view, what its Year shows, Weeks' shape, and Tokens' period and split, Sessions' view and Runway's window, and the Log's columns, the Year showing all use again where the version or account it showed is no longer there to show. Never a focus, a choice, a card or help open, a session's page, a calendar year or Runway's day or week stepped back to, the Log's filters, or a scroll.

```bash
switchboard dashboard           # reading every 30 minutes
switchboard dashboard 15m       # every 15 minutes
switchboard dashboard --probe   # every account from the API each interval, whatever the router says
```

#### `pin`

Tell the router where to send sessions. `pin <account>...` sends every new session, and any other session whose account is chosen afresh, to the accounts given while one has room: to the one, or to the best of several, as the router would choose were they the only accounts. It replaces any pin before it. When none of them has room, the router chooses among every account as though nothing were pinned. `pin auto` goes back to routing. It needs the router.

```bash
switchboard pin <account>... [--move] [--force]
switchboard pin auto [--force]
switchboard pin <account>|auto --session <id>
```

| Flag | Description |
|---|---|
| `--move` | move running sessions on other accounts there too, each on its next request, at the cost of a cache rebuild each; not with `auto` |
| `--force` | clear every session's own pin too, the one `run --account` gave it included |
| `--session <id>` | pin one running session alone to one account, from its next request, in place of any pin it had; takes neither `--move` nor `--force` |

| Command | New sessions | Running sessions |
|---|---|---|
| `pin work` | go to `work` | stay where they are while their caches are warm and their accounts have room |
| `pin work --move` | go to `work` | move to `work` on their next request, but those with a pin of their own |
| `pin work --move --force` | go to `work` | all move to `work`, their own pins cleared |
| `pin work side` | go to the best of `work` and `side` | stay where they are while their caches are warm and their accounts have room |
| `pin work side --move` | go to the best of `work` and `side` | those on neither move to the best of them on their next request, but those with a pin of their own |
| `pin auto` | routed | stay where they are while their caches are warm and their accounts have room |
| `pin auto --force` | routed | as `pin auto`, their own pins cleared |
| `pin work --session 18bb978f` | unaffected | that session moves to `work` on its next request, and stays pinned there |
| `pin auto --session 18bb978f` | unaffected | that session's own pin is cleared, and it's routed like any other |

A session's own pin beats the global one. Every pin yields at a limit: a pinned session that hits one moves by the usual rules rather than failing. A pin spends the reserves of the accounts it names, and no other's (see [Reserves](#reserves)).

Pinning several accounts sets an order to use them up in: with `pin work side`, `work` and `side` take the new sessions, the better of the two first, until both are out, and only then does anything go to the rest. Say two accounts have a weekly reset banked on claude.ai and a third hasn't: pin the two, let them run out, and reset them by hand. The router sees a reset the next time it reads the account: on its own rounds, within half an hour, before a choice it makes afresh once its reading is 15 minutes old, or at once with `switchboard usage -r`, or `R` on the dashboard, which probe an account that can take no request however lately the router read it, unless it probed the account in the last minute.

Name a session by its id, or as much of it as is unique among the sessions routed in the last hour: `switchboard status` lists them, Claude Code's `/status` shows a session's own, and inside a session, `$CLAUDE_CODE_SESSION_ID` holds it.

```bash
switchboard pin side                  # new sessions go to side
switchboard pin side --move           # and running ones move there too
switchboard pin work side             # new sessions go to the better of work and side
switchboard pin auto --force          # back to routing, every session's own pin cleared
switchboard pin work --session 18bb   # one session, by the start of its id
```

#### `claude`

Once `setup` has linked it, `claude` is switchboard: a link named `claude` in switchboard's bin directory, ahead of the real one on `PATH`. Run by that name, switchboard starts the real Claude Code connected to the router, handing it every argument, so `claude --help` is Claude Code's. The real one is the first `claude` on `PATH`, else where its installers put it, that doesn't start switchboard again: past switchboard's link, any other build of switchboard, and a wrapper named `claude` that `exec`s switchboard.

```bash
claude [<claude args>]
```

- **Routed.** When the router answers within half a second, healthy, Claude Code starts with `ANTHROPIC_BASE_URL` pointing at it and the primary's token, or, while that isn't usable, the first account's that is, and the router chooses an account for each request. It's given `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1` too, unless your environment sets it already, so it tells the router what it tells the API of each request, such as the prompt the request serves, for the request ledger.
- **Direct.** When the router isn't running, or isn't healthy, Claude Code connects straight to the API on the primary's token, or, while that isn't usable, the first account's that is, and a line on stderr says so: `switchboard: the router isn't running — connecting directly on work · Work`.
- **Without switchboard.** When switchboard can't take part at all, as when it can't read its config or no account has a usable token, `claude` starts as if switchboard weren't there, and a line on stderr says why: `switchboard: couldn't read the config (…) — starting claude without it`.
- **With an API key.** When `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set, Claude Code may use that key rather than an account's token, and its requests would go unrouted, billed to the key. So `claude` starts as if switchboard weren't there, and says so: `switchboard: ANTHROPIC_API_KEY is set, so Claude Code uses it — starting claude without switchboard`.
- **Local subcommands** go straight to Claude Code, untouched and without a word: `setup-token`, `update`, `upgrade`, `install`, `doctor`, `mcp`, `plugin`, `plugins`, `auth`, `import`, `project`, `auto-mode` and `gateway`. Only the first argument counts: `claude -p doctor` is a prompt.

The ways round switchboard are `switchboard run --direct`, which starts Claude Code on its own login, and the real `claude` by its path. Should the switchboard binary itself break, every `claude` breaks with it, until its directory comes off `PATH`.

Switchboard defines no per-account launchers: an alias for `switchboard run --account <id> --` is one.

```bash
claude                                             # routed
claude --resume                                    # the session finds its account again
claude doctor                                      # a local subcommand: straight to Claude Code
alias cxside='switchboard run --account side --'   # a launcher pinned to side
```

### Setting up

#### `setup`

Walk through setting switchboard up, or what's left of it. Each step says what's done already, and does only what isn't, so `setup` is safe to run again. It asks as it goes, a line at a time, so it needs a terminal: without one, it lists what takes each step alone: a command where there is one, and for the skill, `setup` alone. It stops at a step that fails, at an interrupt, or when its input ends, and run again carries on from there.

```bash
switchboard setup
```

1. **Accounts.** Lists them, each with whether its token is usable, and asks for a missing token (Enter leaves it for later). Offers to add accounts, one at a time, asking each one's id, label and token, and with none configured, asks for the first straight away. With more than one and none marked, asks which is the primary.
2. **Priming.** With no day set, asks for one, `HH:MM-HH:MM`, and writes it to `[prime]`; Enter leaves priming off.
3. **The service.** Installs the LaunchAgent when it isn't installed or loaded, or runs another switchboard than the one setup runs as; restarts the router only when it doesn't answer. The router takes up setup's changes to the config and the tokens itself.
4. **The `claude` link.** Makes `claude` in switchboard's bin directory, `$XDG_DATA_HOME/switchboard/bin`, else `~/.local/share/switchboard/bin`, leading to the switchboard setup was run by, and checks the directory is on `PATH` ahead of the real `claude`. When it isn't, setup gives the one line to add to your shell's startup file, after anything else there that changes `PATH`. It never edits the file itself; run again, in a new terminal, it checks it.

   ```bash
   export PATH="$HOME/.local/share/switchboard/bin:$PATH"
   ```

5. **The skill.** Writes a Claude Code skill, or brings it up to date, which tells Claude what switchboard does under `claude`, so no session needs it explained: `skills/switchboard/SKILL.md` in `~/.claude`, or in `$CLAUDE_CONFIG_DIR`. Switchboard owns the file: `setup`, and the router as it starts, replace it, edits and all, when a new switchboard brings a new version of it, and leave it be otherwise.
6. **Usage.** Prints [`usage`](#usage), every account's windows as they stand.

Setup refuses a switchboard that won't last, such as `go run`'s, as the service and the link both run the one it was run by. It asks for tokens where they don't show as they're pasted, and never takes one pasted where it would.

#### `accounts`

List the accounts, in config order: each one's id and label, whether it's the primary, and whether its token file holds a token switchboard can use, or why not and what would put it right.

```bash
switchboard accounts
switchboard accounts add <id> [--label <label>] [--primary]
switchboard accounts token <id>
switchboard accounts remove <id>
```

| Flag | Description |
|---|---|
| `--label <label>` | `add`: show the account as `<label>` (default: the id) |
| `--primary` | `add`: make it the primary, the account the browser and the Claude apps use |

- **`add`** registers an account: an `[[account]]` table in the config, making the config if there's none. The id names the account for good, and its token file: letters, digits, `-` and `_`. When the account's token file holds no usable token, `add` takes one, and writes it over whatever the file held. An id, or a `--label`, that holds anything shaped like a token is refused before `add` asks for the token.
- **`token`** replaces an account's token.
- **`remove`** removes an account's table from the config, with the comments directly above it, and deletes its token file: of a token file that's a link, the link alone, leaving the file it leads to. The only account can't be removed. Removing the primary says which account is the primary now, the one marked `primary = true`, else the first. Sessions running on a removed account's token, as every session holds the primary's, stay routed, as the primary's, for a week from when the router takes the change up, while the primary has a usable token: `remove` says when it has none.

At a terminal, `add` and `token` ask for the token, which doesn't show as it's pasted; otherwise they read it from stdin. Make one with `claude setup-token`, run while signed in to that subscription. The token is checked with the API before it's saved: one the API refuses isn't saved, and one the API doesn't answer for is, with a warning. An interrupt while they wait for it saves nothing.

The config is edited as text, keeping its comments and layout, and a config that's a link is written through, to where it leads. The router takes the change up itself: the service's restarts to read it, and one started by hand with `serve` logs that a restart is due. See [`serve`](#serve).

```bash
switchboard accounts add work --label Work --primary
switchboard accounts add side --label Side
pbpaste | switchboard accounts token side
switchboard accounts remove side
```

#### `service`

Manage the LaunchAgent that keeps the router running: launchd starts `switchboard serve` now, at every login, and again whenever it stops. `setup` installs it.

```bash
switchboard service install [--log-level <level>]
switchboard service uninstall
switchboard service restart
switchboard service status
```

| Flag | Description |
|---|---|
| `--log-level <level>` | `install`: have the router log at `debug`, `info`, `warn` or `error` and above (default `$SWITCHBOARD_LOG_LEVEL`, else `info`) |

- **`install`** writes `~/Library/LaunchAgents/io.github.leeovery.switchboard.plist`, has launchd load it in place of any copy it had loaded, and waits up to 5 seconds for the router to answer. It reads the config first, as the router will, and fails on one the router couldn't serve. The LaunchAgent runs switchboard by the path you ran it by, so a Homebrew link stays the link `brew upgrade` moves on; a temporary build, such as `go run`'s, is refused. It carries `--config`, and `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `SWITCHBOARD_CONFIG`, `SWITCHBOARD_LOG_LEVEL` and `CLAUDE_CONFIG_DIR` where they're set, `--config`, `SWITCHBOARD_CONFIG` and `CLAUDE_CONFIG_DIR` made absolute, so the router finds what the CLI does. The tokens are files, so it needs nothing else of your environment. It warns when no account has a usable token; the router starts all the same, and routes as soon as a token file holds one.
- **`uninstall`** stops the router, and removes the LaunchAgent.
- **`restart`** restarts the router, which reads the config and the tokens afresh. A router that's answering is asked to restart: it finishes its requests in flight first, up to 30 seconds, and 5 more for those it cuts off then to unwind, then restarts in place, as it restarts itself, and `restart` says so, `the router is finishing its requests in flight, then it restarts in place`, or, for one that can't replace itself, `…, then launchd starts it again`, then waits up to 50 seconds for the new one to answer. A router run by hand refuses, as does one whose config isn't valid, which it couldn't start again from, and `restart` says why. One from before routers restarted in place stops as at a signal, and launchd starts it again. With none answering, launchd starts the service afresh at once. It's how to have a restart the router has due, waiting for a moment with no request in flight, now, as `switchboard status` says.
- **`status`** shows whether the LaunchAgent is installed and loaded, and the router's health.

When the router doesn't answer in time, `install` and `restart` fail, the LaunchAgent in place, pointing to `switchboard logs router` and `launchd.log` for why.

```bash
switchboard service install
switchboard service install --log-level debug
switchboard service status
```

### Checking in

#### `status`

Every account's usage as text: each window's utilization, when it resets and where it's heading; what holds an account back, such as a limit it reached or its reserve, and whether it's under pressure, with the rate it goes by (`under pressure: runs out ~18:21 at 5-hour's rate over the last 30 min, before its reset at 20:10`); the sessions routed in the last hour, a line each, with the account each of its models goes to; the priming schedule; the best account to use next; and last, where the usage came from, with the router's health, and under it, a restart the router has due, with how to have it now. It reads the router while it runs, else probes each account. When the `claude` a shell runs from `PATH` isn't switchboard, the first line says so.

Where its output isn't a terminal, as in a pipe or an agent's shell, it prints the status document as JSON instead: every account's windows, each with its even pace, where even use would have put it by now, and its allowance, what can be spent of it an hour or a day and still last to its reset; each account's extra usage, where the API reports it; the pool, the room new sessions can go to, summed across its accounts; and what's coming up, each limit or cap lifting, window running out or resetting, and prime, soonest first. [The status document](#the-status-document) lists its fields.

```bash
switchboard status [--json | --pretty] [--probe] [-r]
switchboard status --session <id> [--json | --pretty]
```

| Flag | Description |
|---|---|
| `--json` | print the status document as JSON, what an agent or a script reads, as it does off a terminal anyway, even on one; with `--session`, the session's every model and why it went where it did |
| `--pretty` | print the text, as it does on a terminal, even off one |
| `--probe` | probe every account, even while the router runs |
| `-r, --refresh` | have the router first read every account it may, as [`usage -r`](#usage) does, and wait for it, ten seconds at most; without the router, or with `--probe`, every account is probed anyway |
| `--session <id>` | print the id of the account the router sends that session's requests to, the one its last-used model went to, as a statusline asks, on a terminal or off one; it needs the router, and takes no `--probe` or `--refresh` |

```bash
switchboard status
switchboard status --json | jq '.accounts[] | {id, windows}'
switchboard status --json -r   # have the router read every account it may first, as after a reset made by hand
switchboard status --session "$CLAUDE_CODE_SESSION_ID"   # inside a session: the account it's on, such as work
```

#### `logs`

Show the last lines of a log: the router's, or the one every other command shares. Without one named, it's the router's once the router has logged, else the CLI's.

```bash
switchboard logs [router|cli] [-n <lines>] [-f] [--path]
```

| Flag | Description |
|---|---|
| `-n, --lines <n>` | show the last `n` lines (default 50), reaching into the rolled-over file when the log is shorter |
| `-f, --follow` | go on printing lines as they're logged, across rotations, until interrupted |
| `--path` | print the log's path, and nothing else |

```bash
switchboard logs -f              # follow the router: a line per request, with its account and why
switchboard logs cli -n 200
switchboard logs router --path
```

### Looking back

`requests`, `history` and `sessions` read the [request ledger](#the-request-ledger)'s files where they lie, and `events` the files the router keeps its events in beside them, so none needs the router, though `sessions` asks it which sessions are running where it answers. Their plain text is a first cut; agents read their JSON, which each prints wherever its output isn't a terminal.

#### `requests`

The ledger's requests: today's, unless `--since` reaches further back, oldest first, under the day each arrived on, a line each with the time, the session's id cut short, the model, the account, the status, the tokens in and out, and how long it took.

```bash
switchboard requests [--session <id>] [--account <id>] [--since <when>] [--json | --pretty]
```

| Flag | Description |
|---|---|
| `--session <id>` | keep that session's requests, named by its id, or as much of it as is unique among the sessions read |
| `--account <id>` | keep that account's requests |
| `--since <when>` | start at a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`) |
| `--json` | print each line as the ledger holds it, a JSON object a line (see [The request ledger](#the-request-ledger)), as it does off a terminal anyway, even on one |
| `--pretty` | print the text, as it does on a terminal, even off one |

```bash
switchboard requests --since 14:00
switchboard requests --session 18bb --json | jq '.usage'
```

#### `history`

The ledger's days: the last 30, unless `--since` says otherwise, a row for each account and model with its requests, tokens, sessions and worth, what they'd have cost through the API at the prices switchboard carries, read from Anthropic's pricing page, or the config's, in [`[prices]`](#prices-keys); then each account's sessions, moves, limits reached, weeks reset, as `week reset 10:00`, and windows' highest use, of an account that served no request that day only where it reached a limit or one of its weeks reset. Where none of the days has requests, it says so, then those notes alone, without the totals under them. Its requests are messages, those that went upstream and those the router answered itself: Claude Code's quota checks and its counts of tokens, which spend next to nothing, are never among them, nor make a row of their own, though their tokens and worth are counted. Today's is summed up from its lines, as far as it has gone. A model the price table doesn't know shows as unpriced, never free.

A plan's price a month, of the [`plan`](#account-keys) the config gives an account, is prorated as the dashboard prices its periods: a day at 12/365 of a month, a week at 12/52, 30 days at a month and 12 weeks at twelve weeks', a period still running priced whole, and a month or week the days asked for begin part-way through priced by its days from then. The plans' own period runs from the later of the day asked for and the first the ledger holds. An account without a `plan` has no cost and no ratio, and every account's sums those that have one.

Under the days, over all of them: each account's totals, its plan and its worth against it, a column each and one for every account; each week's peaks; and whether there are as many accounts as the use needs.

- **An account's own week** runs from one reset of its week to the next, as the readings history saw them, a reset by hand among them, and its peak is the use read just before the reset that ended it.
- **Placed** by the date it began on: under its calendar week, from the config's [`week_starts`](#top-level-keys), where it began up to half the week in, as a Thursday is in a week from Monday, else under the next; but the week still running, its peak its highest so far, is always this week's, the reset that began it read from the week before the days asked for where it came before them.
- **Short:** a week is short where some account hit its week's limit.
- **The replay:** each whole week in which every account's week is known and ended is replayed with an account fewer, its peak times its plan's size shared among the rest by theirs, and with one more, of the plan most of the accounts have, every peak falling in proportion: an account without a `plan` counts as that plan, and where none has one, each counts as one.
- **The verdict,** as the dashboard's History words it: `one too few`, where now is short on 4 or more of every 7 and one more would halve that; `too few`, where now is short on as many and one more wouldn't halve that, the replay saying how short one more would still leave it; else `one more than you need`, where now is short on fewer than 4 of every 7 and one fewer would be short on no more weeks, naming the one to drop, and, of three accounts or more, how many a replay without it and the next by the same rule finds; else `about right`.
- **Each account's,** over its own whole weeks: `hits its limit most weeks`, 3 or more of every 7, `plenty to spare most weeks`, 40% or more left at a reset on average, `ends most weeks with room`, `barely used`, 75% or more left, or, where the verdict is one more than you need, `could go`, no limit hit, 10% or less used, and the rest covering it.
- **Never guessed:** what a day never counted, as one recorded before the ledger counted resets and time at a limit, is left out, never taken for none.

`--windows` prints in their place each account's windows over those days, a line each time a window's use, its reset or its status changed, as the readings history holds it.

```bash
switchboard history [--since <when>] [--windows] [--json | --pretty]
```

| Flag | Description |
|---|---|
| `--since <when>` | start on the day of a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`) |
| `--windows` | print each account's every window's readings over those days, in place of the days |
| `--json` | print the blocks as JSON, as it does off a terminal anyway, even on one: the days' summaries, priced, then the year, months, weeks, tokens, totals, plans and capacity over them, or with `--windows`, the windows' readings. See [`history`'s JSON](#historys-json) |
| `--pretty` | print the text, as it does on a terminal, even off one |

```bash
switchboard history --since 7d
switchboard history --json | jq '.days[-1]'      # today's, as far as it has gone
switchboard history --json | jq '.capacity'      # whether there are as many accounts as the use needs
switchboard history --windows --since 14:00      # today's readings of every window
```

#### `sessions`

Today's sessions: those running, the oldest started first, then those that ended today, the latest ended first. A line each with its id cut short and its directory, the account it's on, its model, what it's doing, asking, answering, idle or when it ended, when it started, its requests today and what they'd have cost through the API, and how it came to its account, such as `from work, at its cap` and what writing its context again there cost; then a line summing them. It reads the ledger's files, and the router's sessions where it answers; without the router, it lists today's sessions from the ledger alone, none as running, and says so.

Given a session, by its id or as much of it as is unique among today's sessions, it shows that one's page: what it's doing, where it runs and why, when it started, and when it came back after an hour or more without a request; each account it ran on, with its time there, its requests, the points of each of the account's windows they took and their worth; then its turns, newest first, a turn being a prompt and every request that serves it, its subagents', compactions and side requests included, as Claude Code's own headers tell them, each with when it started and how long it took, its requests, the tools its answers called most, the tokens it read from the cache, wrote to it and put out, its worth, and the accounts its own conversation went to, as `work → side` where it moved, a subagent on another account being no move; and its totals. The points are an estimate: a window counts its account's every session, so each rise in its use between two answers is shared out among the requests that ended between them, by their tokens. A session of an earlier day is found by its whole id. Without the router, it's shown from its lines alone, as ended.

```bash
switchboard sessions [<id>] [--json | --pretty]
```

| Flag | Description |
|---|---|
| `--json` | print the sessions, or the session's page, as JSON, as it does off a terminal anyway, even on one. See [`sessions`' JSON](#sessions-json) |
| `--pretty` | print the text, as it does on a terminal, even off one |

```bash
switchboard sessions
switchboard sessions --json | jq '.sessions[] | select(.running)'   # those running now
switchboard sessions 5b0e --json | jq '.turns[0]'                    # its newest turn
```

#### `events`

What the router decided and noticed: sessions started and moved, accounts under pressure, at their caps and limits, refused, primed and open again, pins set and cleared, restarts falling due and the router's health. Today's, unless `--since` reaches further back, oldest first, under the day each happened on, each once, as it last stood, a line each with the time, the session's id cut short, if it's of one, the kind, the account and what happened. Its files outlast a restart, so it looks back across them.

```bash
switchboard events [--since <when>] [-f] [--json | --pretty]
```

| Flag | Description |
|---|---|
| `--since <when>` | start at a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`) |
| `-f, --follow` | then go on printing a line each time the router files one, a changed event again, whole, until interrupted |
| `--json` | print each event as the router files it, a JSON object a line, with its `run` and `id` (see [The router's events](#the-routers-events)), as it does off a terminal anyway, even on one: of an event printed again, the last line is how it stands |
| `--pretty` | print the text, as it does on a terminal, even off one |

```bash
switchboard events --since 2d
switchboard events -f --json | jq 'select(.kind == "limit")'
```

### Plumbing

#### `run`

What `claude` runs: start Claude Code through the router, with Claude Code's own arguments after `--`. `--account` pins the session's conversation to an account while that account can take it; Claude Code's own token stays the primary's, or is `--account`'s while the primary's isn't usable. Without the router, Claude Code connects directly on `--account`'s token.

```bash
switchboard run [--account <id>] [--direct] [-- <claude args>]
```

| Flag | Description |
|---|---|
| `--account <id>` | pin the session's conversation to account `<id>`, which must be configured and have a usable token |
| `--direct` | start Claude Code on its own login, without the router or a token, for what needs the login; not with `--account` |

```bash
switchboard run -- --resume
switchboard run --account side -- -p "tidy the changelog"
switchboard run --direct
```

#### `serve`

Run the router in the foreground, until interrupted or terminated: the proxy, on `listen`, and its control API, on a unix socket in the state directory. The service normally runs it. It logs to the router's log, and to the terminal when it runs in one. A second router fails while one is running.

It starts even when no account has a usable token, warning in its log that nothing will be routed until one has. Every 3 seconds, the router reads the token files again and takes up what's changed in place: an account goes out on the token its file holds now, one without a usable token gains the one its file comes to hold, and one whose file holds none it can use at two looks in a row has nothing to send on until it's back. A single look finding none, as a file caught while it's rewritten, keeps the account its token. It also looks at its config file, following links, at the binary it was started as, the Homebrew link the service runs, and at `/etc/localtime`, which says the Mac's time zone. Once the config changes into one that's valid, an upgrade leads the link to another binary, or the time zone changes, the service's router waits for a moment with no request in flight, saves its state, and replaces itself in place with the binary the link leads to, handing it the sockets it listens on; a config that isn't valid is logged, and the router carries on as it was. A binary that isn't there, as for a moment while `brew upgrade` moves the link on, it tries again for a couple of seconds; should replacing itself still fail, it exits, and launchd starts it again. Told to stop while it finishes its requests to restart, it stops then and there. Run by hand, the router logs, once, that a restart is due, and `switchboard status` says to run `serve` again. As it starts, it makes the `tokens`, `history` and `ledger` directories private, takes up the readings its history holds, so the recent rates it judges pressure by carry on from before, and brings the skill up to date.

```bash
switchboard serve [--log-level <level>]
```

| Flag | Description |
|---|---|
| `--log-level <level>` | log at `debug`, `info`, `warn` or `error` and above; overrides `SWITCHBOARD_LOG_LEVEL` |

### Help and version

#### `help`

`switchboard help` lists the commands, and `switchboard help <command>` shows one's help; every command also takes `-h`, `--help`. `claude --help` is Claude Code's own.

#### `version`

Print switchboard's version. `switchboard --version` and `-v` do the same.

## Configuration

`$SWITCHBOARD_CONFIG`, else `$XDG_CONFIG_HOME/switchboard/config.toml`, else `~/.config/switchboard/config.toml`; `--config <file>` names another for one command. `setup` and the `accounts` commands edit it, and it's yours to edit too.

```toml
week_starts = "monday"  # optional: the day calendar weeks start on

[[account]]
id      = "work"        # for good: letters, digits, '-' and '_'; its token is tokens/work
label   = "Work"
primary = true          # the account the browser and the Claude apps are signed into
reserve = 0.1           # optional, on any account: the share of every window the router leaves unused
plan    = "max20x"      # optional, on any account: its plan, pro, max5x or max20x

[[account]]
id    = "personal"
label = "Personal"
plan  = "max5x"

[[account]]
id    = "side"
label = "Side"

[prime]
day = "08:00-23:00"     # your day, in local time: priming spreads the 5-hour resets over it

[notifications]         # optional: these are the defaults
limits  = true
room    = true
warning = 0.9
moves   = false

[history]               # optional: this is the default
keep = "400d"

[ledger]                # optional: this is the default
keep = "400d"

[usage]                 # optional: this is the default
by = "window"

[prices.plans]          # optional: a plan's price, in US dollars a month, in place of switchboard's
max5x = 90

[prices.models.claude-opus-5-5]   # optional: a model's prices, in US dollars a million tokens
input          = 3.2
output         = 16
cache_read     = 0.16
cache_write_5m = 4
cache_write_1h = 6.4
```

The accounts keep their file order, which is their order everywhere they're shown, and the order priming gives them their slots in. Unknown keys are errors, and a file that parses has every problem reported at once. No error quotes a token pasted into the config, not even where a file that doesn't parse fails: it shows as `[redacted]`, or not at all.

### Top-level keys

| Key | Default | Description |
|---|---|---|
| `listen` | `127.0.0.1:4747` | the proxy's address: `host:port`, the host a loopback IP address, `127.0.0.1` or `::1` (as `[::1]:4747`), never a name such as `localhost` |
| `upstream` | `https://api.anthropic.com` | the API's base URL: `https` unless its host is a loopback IP address |
| `week_starts` | `monday` | the day calendar weeks start on, as [`history`](#history) counts them: a day of the week, by its name in English, in any case, as `monday` or `Sunday`. An account's own week, which resets when Anthropic resets it, never moves with it |

### `[[account]]` keys

One table per subscription, and at least one.

| Key | Default | Description |
|---|---|---|
| `id` | required | the account's name for good, and its token file's: a letter or digit, then letters, digits, `-` and `_`; unique, in any case, as macOS gives `work` and `Work` one token file; not `auto`; nothing shaped like a token |
| `label` | the id | what it's shown as; nothing shaped like a token |
| `primary` | the first account | the account the browser and the Claude apps are signed into; one at most |
| `reserve` | `0` | the share of every window the router leaves unused: `0`, or more than `0` and less than `1` |
| `plan` | none: unknown | the plan its subscription is on, which switchboard can't read from Anthropic: `pro`, `max5x` or `max20x`. It prices the account against its worth, and sizes it in [`history`](#history)'s replay of a week; without it, the account's plan shows as `-`, and nothing is priced against it |

### `[prime]` keys

| Key | Default | Description |
|---|---|---|
| `day` | none: priming off | your day, `HH:MM-HH:MM`, in local time; an end before the start runs past midnight |

### `[notifications]` keys

Which desktop notifications the router posts: see [Notifications](#notifications).

| Key | Default | Description |
|---|---|---|
| `limits` | `true` | an account hits a limit, and the sessions it moved |
| `room` | `true` | an account has room again |
| `warning` | `0.9` | a window passing this share of its limit; `0` turns it off |
| `moves` | `false` | every other session move, such as after an idle hour or by pin |

### `[history]` keys

How long the router keeps the readings history: see [What it records](#what-it-records).

| Key | Default | Description |
|---|---|---|
| `keep` | `400d` | how long a day's file is kept once its day has ended: a whole number of days, `<n>d`, `8d`, a week and a day, or more; or `forever`, which never removes one |

### `[ledger]` keys

How long the router keeps the request ledger's lines: see [What it records](#what-it-records).

| Key | Default | Description |
|---|---|---|
| `keep` | `400d` | how long a day's file of lines is kept once its day has ended: a whole number of days, `<n>d`, `8d`, a week and a day, or more; or `forever`, which never removes one. The day's summary is kept for good |

### `[usage]` keys

How [`usage`](#usage) and the dashboard's Overview lay out their windows.

| Key | Default | Description |
|---|---|---|
| `by` | `window` | `window`, the accounts compared a window at a time, or `account`, each account's windows together; `usage --by` overrides it for one printout, and the dashboard's `b` for good, once pressed |

### `[prices]` keys

Prices in place of those switchboard carries, read from Anthropic's pricing page, for [`history`](#history) and [`sessions`](#sessions) to price with. Each price is a number, `0` or more, and holds for every day alike; one left out keeps switchboard's.

| Key | Default | Description |
|---|---|---|
| `[prices.plans]` | switchboard's prices | a plan's price, in US dollars a month, by its name: `pro`, `max5x` or `max20x` |
| `[prices.models.<id>]` | switchboard's prices | a model's prices, in US dollars a million tokens, under its id as the API names it, as `claude-opus-5-5`, by the kind of token: `input`, `output`, `cache_read`, `cache_write_5m` and `cache_write_1h`. A model switchboard doesn't know is priced only where all five are given, its web searches at switchboard's price, and its US-only inference unpriced; given fewer, it's left unpriced, and the log says so |

A plan's name, or a kind of token, switchboard doesn't know is an error, as any unknown key is.

### Tokens

Each account's token is a file of its own, holding the token alone: `<state dir>/tokens/<id>`, as in `~/.local/state/switchboard/tokens/work`. The file must be yours, and neither readable nor writable by anyone else (`chmod 600`); otherwise the account counts as having no token, and `accounts` and `status` say why and how to fix it. Whitespace around the token is ignored. Switchboard keeps the `tokens` directory `0700` when it writes there, and the router makes it so as it starts; it reads no token from the environment.

`setup`, `accounts add` and `accounts token` write the files, but anything can, such as a secrets manager's file export or a dotfiles step, best by writing a temporary file beside the token file and renaming it into place, as switchboard does. A token file can be a link to one kept elsewhere: switchboard writes a token through it, to where it leads. An account whose token file already holds a usable token is added without asking for one.

The router reads the tokens as it starts, every account's file again every 3 seconds, and an account's file again when the API refuses the token it holds, so a changed token file needs no restart. A file found without a usable token at one look, as while it's being rewritten, keeps its account's token: only two looks in a row, 3 seconds apart, take it away. `claude` too looks again, a moment on, at the primary's file, and a pinned account's, when it finds one there but empty, before it counts it as holding none. Sessions started before a token was replaced still carry the old one, which the router routes as its account's for 7 days, and those started on the token of an account you've since removed carry that, which the router routes as the primary's for 7 days. It keeps none of these tokens, only their SHA-256 hashes.

## Reference

What switchboard writes, and what its verbs print, field by field. Fields may be added to any of them, never renamed, and a reader passes over those it doesn't know. Accounts appear by their ids alone, never a label or a token.

### Where things live

| What | Where |
|---|---|
| The config | `$SWITCHBOARD_CONFIG`, else `$XDG_CONFIG_HOME/switchboard/config.toml`, else `~/.config/switchboard/config.toml` |
| The dashboard's themes | `$SWITCHBOARD_THEMES_DIR`, else `$XDG_CONFIG_HOME/switchboard/themes/`, else `~/.config/switchboard/themes/` |
| The state directory: `state.json`, `control.sock`, `prefs.json`, `tokens/`, `logs/`, `history/` and `ledger/` | `$XDG_STATE_HOME/switchboard/`, else `~/.local/state/switchboard/` |
| The `claude` link | `$XDG_DATA_HOME/switchboard/bin/claude`, else `~/.local/share/switchboard/bin/claude` |
| The LaunchAgent | `~/Library/LaunchAgents/io.github.leeovery.switchboard.plist` |
| The skill | `skills/switchboard/SKILL.md` in `$CLAUDE_CONFIG_DIR`, else in `~/.claude` |

In the state directory:

| File | Holds |
|---|---|
| `state.json` | the router's state that outlasts a restart: each session's account, the pins, and each account's last readings |
| `control.sock` | the router's control API, which the commands and the dashboard talk to |
| `prefs.json` | what the dashboard remembers: see [`dashboard`](#dashboard) and [Themes](#themes) |
| `tokens/<id>` | each account's token: see [Tokens](#tokens) |
| `logs/` | `router.log`, `cli.log` and `launchd.log`: see [The logs](#the-logs) |
| `history/readings-<date>.jsonl` | the [readings history](#the-readings-history), a file a day |
| `ledger/requests-<date>.jsonl` | the [request ledger](#the-request-ledger)'s lines, a file a day |
| `ledger/day-<date>.json` | each [day's summary](#the-days-summary) |
| `ledger/events-<date>.jsonl` | the [router's events](#the-routers-events), a file a day |

A day's file of lines is compressed, to `<name>.jsonl.gz`, which `gzip -dc` reads, once its day ended two days ago, and kept as long as its `keep` says (see [`[history]`](#history-keys) and [`[ledger]`](#ledger-keys)), the events as long as the ledger's lines.

### The readings history

A JSON line each, `{"at", "account", "window", "utilization", "resets_at", "status", "source"}`, in `history/readings-<date>.jsonl`:

| Field | Is |
|---|---|
| `at` | when it was read |
| `account` | the account, by its id alone |
| `window` | the window's key, as the API gives it, such as `5h`, `7d` or `7d_oi` |
| `utilization` | how much of the window is used, as a fraction |
| `resets_at` | when the window resets |
| `status` | `allowed`, `allowed_warning` or `rejected` |
| `source` | whether it came off an `answer`, a `probe` or a `prime` |

### The request ledger

A JSON line for each request the router routes, in `ledger/requests-<date>.jsonl`. It keeps everything about a request but its content: never its messages, its system prompt or its tools, its metadata's ids, a setting or header the line doesn't name, or anything shaped like a token.

| Field | Is |
|---|---|
| `at` | when the request arrived, in UTC, to the millisecond |
| `request` | the router's id for it, as its routed line in the log gives it |
| `kind` | `message`, Claude Code's quota `check`, or a `count` of tokens |
| `session` | its session's id |
| `dir` | the directory `run` started Claude Code in, your home shown as `~`, which `run` tells the router in a header the API never sees, and which a `claude` started within that session other than through `run` carries too |
| `model` | the model it asked for |
| `account` | the account whose answer the client got, by its id alone; left out where the router answered it itself |
| `reason` | why it went to that account, in the words of the log's routed line |
| `from` | the account its session was on before, where the request moved it |
| `tried` | the accounts it went out on that couldn't serve it, in the order tried, each `{account, why}` |
| `status` | its answer's status, `0` where it ended before one came |
| `canceled` | set where the client went away before the end |
| `cut_off` | set where the router cut the request off as it stopped |
| `attempts` | how many times it went upstream, `0` where the router answered it itself |
| `first_ms`, `total_ms` | how long after it arrived its answer's first byte passed on to the client, and its end, in milliseconds |
| `agent` | the client's user agent |
| `betas` | the features it asked the API for beyond its version |
| `prompt` | the prompt it serves, by Claude Code's random id, the same on every request of one prompt, its subagents' included |
| `class` | `main`, `subagent`, `workflow`, `compaction` or `auxiliary` |
| `agent_id`, `parent_agent_id` | the subagent that sent it, and the one that started that, by Claude Code's ids for them, random for a subagent, from its name for a member of an agent team |
| `agent_type` | the subagent's kind, a built-in agent's name, `custom`, `teammate` or `fork`, never a name you chose |
| `compaction`, `compacted` | what started the compaction it is, `auto`, `manual` or `reactive`, or, on the first request after one, what started that |
| `tool_ms` | how long each tool call whose result it carries ran, each `{tool, ms}` |
| `shape` | the request's size in `bytes`, how many `messages`, `system` blocks and `tools` it carried, and those of its settings switchboard knows: `max_tokens`, `thinking`, `stream`, `tool_choice`'s type, `temperature`, `top_k`, `top_p`, `service_tier`, `output_config`'s `effort`, `speed`, `inference_geo`, and the types of `context_management`'s edits |
| `answer` | of the answer the client got: Anthropic's `id` for it, the `model` that served it, why it stopped, `stop`, how many `blocks` of each kind it held, the `tools` it called, by name, and an `error`'s `type` and `message` |
| `usage` | the answer's closing usage, as the API gave it, field for field |
| `limits` | the answer's `anthropic-ratelimit-unified-*` headers, the prefix taken off |

`prompt` to `tool_ms` are what Claude Code's own headers say of the request, each left out where it sends none: the agents' ids Claude Code sends the router always, and the rest as `run` has it send them, with `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1` (see [`claude`](#claude)).

### The day's summary

An hour after each day ends, the router summarises it beside its lines, in `ledger/day-<date>.json`: `{"version", "day", "lines", "bytes", "accounts"}`, `lines` how many lines it was made from, and `bytes` how large its files were then. Each of `accounts` is an account's day:

| Field | Is |
|---|---|
| `account` | the account, by its id alone; left out of the requests no account answered |
| `models` | its requests by the model each asked for, each counted by how it went: `upstream`, those that went upstream, `no_usage`, those of them that got no usage back, `unsent`, those the router answered itself, `checks`, Claude Code's quota checks, and `counts`, its counts of tokens; with their `sessions` and `session_ids`, their `usage` summed field for field, and `inference_geo` where they asked for one |
| `sessions`, `session_ids` | the sessions its requests were of, by their ids too |
| `moved_on`, `moved_off` | the sessions its requests moved onto it, and off it |
| `highest` | each window's highest use that day, by its key |
| `rise` | how far each window's use rose that day, from the history |
| `resets` | its windows' resets that day, each `{window, at, before}`, `before` the use before it |
| `limits` | the limits it reached, each a window turning `rejected`: `{window, at, resets_at}` |
| `minutes_at_cap`, `minutes_at_limit` | the minutes it spent at its cap, and at a limit |
| `read_before` | the windows the history read in the week before |

- **Which accounts:** every account the history read that day has a day of its own, whether or not it served a request, as has one whose window reset that day by the reset its last reading gave, so a week that ended on a quiet day keeps its peak.
- **Which days:** a day the router served nothing on, but read an account, or saw a window reset, is summarised too, once the history holds the week before it, as a window already at its limit as the history began would otherwise read as one reached that day. A day that ended while the router was stopped is summarised as it starts again, before the history's readings of it can go.
- **Summarised again:** a day a line comes to be filed under later, as a request in flight past that hour files one, is summarised again, knowing no less than before, though the history has since let the day's readings go, or a damaged file some of its lines, which can't be told from those filed since, so its counts are then the least there were.
- **Kept for good:** once its lines go, its summary is kept for good, summarised again only by a release that summarises days otherwise, from the day's readings while the history keeps them.

### The router's events

A JSON line for each version of an event, in `ledger/events-<date>.jsonl`, so a reader of one that changed takes the last:

| Field | Is |
|---|---|
| `run`, `id` | when the router that told it started, and its number in that run, rising from 1 |
| `at` | when it happened |
| `kind` | `started`, a session first on an account; `moved`; `pressure`; `cap`, a window at its reserve; `limit`; `refused`; `primed`; `room`, an account with room again; `pin` and `auto`, routing set by hand and given back; `restart`, a restart falling due; or `health`, the router's own turning |
| `account`, `accounts` | the account it befell, and the accounts a global pin names |
| `session`, `model` | the session that started or moved, or whose own pin changed, and the model whose requests did |
| `from`, `to` | the accounts a session moved from and to, `to` where the sessions a limit, a cap or a refusal moved went, when they all went to one |
| `reason` | why: why a session went to its account, why a restart is due, or why the router turned unhealthy |
| `windows` | the keys of the windows it's of |
| `until`, `since` | when a limit, cap or refusal ends, or a window runs out or resets; and when a pressure's rate is measured from |
| `count` | how many sessions a limit, a cap or a refusal moved while it held |
| `limit` | of a limit, its identity, kept while it holds; of a move, the id of the limit's event that counts it among the sessions it moved |
| `forced_by` | of a move, the id of the limit's, cap's or refusal's event that counts it among the sessions it moved |
| `reserve` | of a cap, the account's reserve |
| `status`, `family` | of a refusal, the status the API refused with, and for a request refused alone, the model family it holds back |
| `move`, `force` | of a pin, that it moves the running sessions too, and that it cleared every session's own pin |
| `by` | what set or cleared a pin, `cli` or `dashboard` |

### The status document

What [`status`](#status) and [`usage`](#usage) print as JSON: `{"generated_at", "source", "fallback", "best", "primary", "prime", "pin", "router", "restart", "sessions", "events", "pool", "coming_up", "accounts"}`. Times are in UTC, but the priming schedule's times of day, which are local; utilizations are fractions, and can pass 1. A field the router alone knows is left out of a document built by probing.

| Field | Is |
|---|---|
| `generated_at` | when it was built |
| `source`, `fallback` | `router`, or `probe` where it was built by probing every account, and why, where the router was asked first |
| `best` | the account new sessions go to next |
| `primary` | the primary's id |
| `prime` | the priming schedule: the day, the window a prime starts, and each account's slot |
| `pin` | the global pin: the accounts it names, since when, and whether it moved the running sessions |
| `router`, `restart` | the router's health over the last 5 minutes, and a restart it has due |
| `sessions`, `events` | how many sessions it has routed in the last hour, and what's happened lately, newest first |
| `pool` | the room new sessions can go to, summed across its accounts |
| `coming_up` | each limit or cap lifting, window running out or resetting, and prime, soonest first |
| `accounts` | each account, in the config's order: its `id`, `label`, whether it's the `primary`, its `reserve`, whether its token is set, when it was read, and its `windows`, each with its use, reset, status, even `pace` and `allowance`; the windows `lapsed` and `at_reserve`; its `extra_usage`; what couldn't be read and why; from the router, a `limit` or refusal that holds it, its `pressure` and `rates`, and its `sessions` |

### `history`'s JSON

`{"prices_as_of", "days", "year", "months", "weeks", "tokens", "totals", "plans", "capacity"}`, each block over the days asked for since the ledger began, oldest first, today's last:

- **`prices_as_of`:** the day the prices were read, and `prices_from_config` where the config's `[prices]` price a model or a plan.
- **`days`:** each day's summary as the ledger holds it (see [The day's summary](#the-days-summary)), each model's `worth` in US dollars added, `unpriced` naming what it leaves out, as `no_usage`, requests that got no usage back, and `by_family`, the day's worth by model family. A day of no requests, the history telling of no account that day, has no `accounts`, and an account has a day there where its quota checks, its counts of tokens or the history give it one, requests or none.
- **`year`:** each day's `requests` and the `level`, 0 to 4, it's shaded at, and the `cuts` between the levels, the active days' quarters.
- **`months`:** each month's `requests`, `worth`, `limits` reached by window, `by_family`, and its plans' `cost`, the whole month's, or its days' from the first asked for, with the worth of their accounts `against` it, a ratio, but for the month so far.
- **`weeks`:** each week's, from the day the config's `week_starts` names, Monday unless it says otherwise: each account's `peaks`, the highest use of its week and of each model's own week, as Fable's, by window, the windows whose week is still running, `open`, its peak so far, and the `limits` it reached in that week, by window; then `all` of them, each window's peak the mean of theirs.
- **`tokens`:** each week's by kind, `input`, `output`, `cache_write`, `cache_read` and `total`, its `worth`, its plans' `cost` and `against`, then `by_account`, in the config's order, and `by_model`, each version by its id, with the accounts it ran `on`, the one it was worth most on first.
- **`totals`:** each account's, then `all` of them: `requests`; `sessions`, each once however many days and accounts it ran on; `moved_on` and `moved_off`; `limits` by window; `minutes_at_cap` and `minutes_at_limit`; `left_at_reset`, each window's share left at its last reset; `worth`; `by_model`, each version's `share` of use, weighed at API prices, and its `worth`; `busiest_day`; `longest_run`, the most days in a row with any request; and `partial`, naming a count some of the days never counted.
- **`plans`:** each account's `plan`, its `price` a month, what it cost over those days and `against` it, and its `worth`, then `all` of them, of those on a plan.
- **`capacity`:** over the whole `weeks` in which every account's week is known and ended: how many were `short`, `with_one_fewer` and `with_one_more`; the `verdict`, `one too few`, `too few`, `one more than you need` or `about right`, and where it's one more than you need, the account to `drop`, and of three or more `with_two_fewer`, without it and the account whose loss then makes the fewest weeks short; and each account's `verdict`, over its own whole `weeks`, the weeks it hit its `limits` in, what it had `left_at_reset` on average and how many the rest would have been short `without_it`, then `all` of them, `room to spare together` or `little to spare together`.
- **Worth:** every `worth` is exact, with `unpriced` beside it naming what it leaves out: the counts a usage holds that the price table can't price; `no_usage`, requests that got no usage back; and of requests it prices none of, a model it doesn't know by its id, a model it knows in a geo it doesn't price it in, as `inference_geo:eu`, and requests that named no model as `no_model`. `worth` is left out where nothing was priced, as by `sessions`. Every share is exact.
- **With `--windows`:** `{"windows": [{"account", "window", "readings"}]}`, each reading `{at, utilization, resets_at, status, source}`.

### `sessions`' JSON

`{"generated_at", "prices_as_of", "sessions", "today"}`:

- **`sessions`:** each session with its whole id, its directory, account and model, whether it's `running`; of one running, when it was last seen, each of its models' account and why, what it's doing and its `move_cost`, what moving it now would cost; of one ended, when it `ended`; when it `started`, its `requests` today, their `worth` in US dollars and what it leaves `unpriced`, and the move that brought it to its account, as `moved`.
- **`today`:** summing them.

Given a session, its page:

- **`session`, `dir`, `account`, `model`, `running`, `last_seen`, `models`, `state`, `move_cost` and `ended`** as the list has them, its own `pin`, `started`, `resumed`, and of one ended, `kept_until`, the last day the ledger keeps the lines of its first day.
- **`accounts`:** each `{account, from, to, requests, worth, points}`, `points` by window.
- **`moves`:** each `{at, model, from, to, reason, written, cost}`.
- **`turns`:** newest first, each `{turn, started, ended, requests, tools, read, written, out, worth, accounts}`, `ended` left out of one still going.
- **`totals`:** `{turns, requests, from_cache, worth, unpriced}`.

Every `worth` is exact, and left out where nothing was priced; `unpriced` names what it leaves out as `history` names it.

## Troubleshooting

- **Where things stand:** [`switchboard status`](#status) says how each account stands and why, the router's health, and a restart it has due; [`switchboard service status`](#service) whether the LaunchAgent is installed and loaded.
- **Why a request went where it did:** `switchboard logs -f` follows the router's log, a line a request with its account and why; [`requests`](#requests) and [`events`](#events) look back over the days.
- **A limit reset by hand:** `switchboard usage -r`, `status -r` or `R` on the dashboard has the router read the accounts now.
- **Not routed:** some of what Claude Code sends never reaches the router: see [What doesn't go through the router](#what-doesnt-go-through-the-router).
- **Round switchboard:** `switchboard run --direct` starts Claude Code on its own login, and the real `claude` runs by its path. Should the switchboard binary itself break, every `claude` breaks with it, until its directory comes off `PATH`.

### The logs

Logs are for working out, after the fact, why a session went to an account, why a request failed, or why the dashboard showed what it did. They're in `<state dir>/logs/`: the router writes `router.log`, every other command `cli.log`, and launchd writes the service's own output, such as a crash's, to `launchd.log`. Each record is a line of logfmt, and the router notes every request it routes, with its account and why:

```
time=2026-09-28T14:12:00.123+01:00 level=INFO msg=routed component=router pid=4242 id=5f3a9c2e session=18bb978f model=claude-opus-5-5 account=side reason="moved: work hit its limit" status=200 attempts=2 duration=9.412s
```

- **Level:** `SWITCHBOARD_LOG_LEVEL` is `debug`, `info`, `warn` or `error`, and `info` unless set. `serve --log-level`, and `service install --log-level`, which passes it on, override it for the router.
- **Rotation:** a log rolls over before it would pass 10 MiB, keeping five old files.
- **Redaction:** nothing logs a token or an account's label: accounts appear by id, and anything shaped like a token is replaced with `[redacted]`.
- **Never in the way:** no command but `serve`, at a terminal, logs to stdout or stderr, so `--json` and the dashboard stay clean, and none fails because it couldn't log.

[`switchboard logs`](#logs) prints and follows them.

## Contributing

`CLAUDE.md` holds the house rules, and [docs/design.md](docs/design.md) the design. Every change passes the gates:

```bash
gofmt -l .               # must print nothing
go vet ./...
scripts/test-isolated    # every test, race detector on, sandboxed
golangci-lint run ./...
go build ./...
```

The clips and stills in this README are recorded from scenarios played through the real dashboard, and from the fixtures of its signed-off frames, every seam faked: [demo/README.md](demo/README.md) says how to record them again.

No test touches the real network, environment, home directory, config, state, binaries or notifications. `scripts/test-isolated` runs the suite inside a macOS sandbox (`scripts/isolation.sb`) that denies it the network beyond loopback, and the router's default port, 4747; unix sockets outside the temporary directory; writes into the home directory but Go's caches, switchboard's real config, themes, state and bin directories, its `prefs.json` and themes, Claude Code's config directory and the directories on `PATH`; and running the real `claude`, `osascript`, `launchctl`, `tmux` and `open`. Each run first proves the sandbox holds, and `scripts/test-isolated --self-check` does only that: among the rest, that it denies running the real `claude`, found as switchboard finds it. Every package's tests also run through `internal/testguard`, which points `HOME` and the XDG directories at a throwaway root, clears the variables that can hold a token, puts stubs on `PATH`, and fails the run when a test reaches the real system, such as a real token file appearing, changing or going, even if every test passed. A test that needs something the guards block injects it instead.

## License

MIT
