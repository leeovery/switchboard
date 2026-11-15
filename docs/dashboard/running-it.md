# Running it

`switchboard dashboard` shows the usage of a pool's accounts, every account's where there's one
pool, full screen, until you press `q`. It needs a terminal, and refuses to run without one. It
reads the router while the router answers, and otherwise probes each account. Just after the
upgrade that made account ids, while the router moves the history onto them, it says what the move
waits on, a line each time that changes, and opens once it's done (see
[Upgrading to account ids](../upgrades-and-restarts.md)), and the same while it moves
the history to pools.

```bash
switchboard dashboard [interval] [--no-notify] [--probe] [--pool ID]
```

The interval, given after the command, is how often the dashboard probes every account when it
probes for itself, without the router or with `--probe`:

- It's `30m` unless you give one, and `5m` at the least.
- Give it as a duration, such as `15m` or `1h`, or as a number of minutes.

| Flag | Description |
|---|---|
| `--no-notify` | Posts none of the desktop notifications the dashboard posts while it probes without the router. See [Notifications](../notifications.md). |
| `--probe` | Probes every account each interval, even while the router runs. With no router read, there's no routing card, and `p` does nothing. |
| `--pool` | Shows the pool with the id given, as `switchboard pool` lists them, else the one the verbs act on. See [In a pool](#in-a-pool). |

```bash
switchboard dashboard           # probing every 30 minutes without the router
switchboard dashboard 15m       # every 15 minutes
switchboard dashboard --probe   # every account from the API each interval, whatever the router says
switchboard dashboard --pool q7mt   # pool q7mt, whatever pool the verbs act on
```

## In a pool

Where the config has [pools](../pools.md), the dashboard shows one pool: the one
`--pool` names, else the one the verbs act on, the only pool, else the unfenced one (see
[Reading a pool](../commands.md)). A pool the config doesn't have is refused, naming
those it does. What it reads from the router is that pool's: its accounts, windows, room, sessions
running and pin, and the router's events, the Overview's and Runway's LATELY and the Log's Events
among them. Accounts and Trends list the pool's accounts alone. What it reads from the request
ledger is the pool's work alone, a request made before pools the first pool's: the Log's requests,
today's and each day it reads back, with each day's figures, and its find, which finds the pool's
sessions; Sessions, each session's requests, worth and tokens those it made in the pool, the List
over every period; Projects, each project's work in the pool, a project's page and its SESSIONS
among them; a session's page, its points among them; Trends, its YEAR among them; and the worth
and tokens on Accounts. A session resumed in another pool, once its directory's fence changes,
shows in each pool with the work it did there, and a project worked on in two pools shows each
pool's work alone. The days begin where the pool's work does, so a pool added to a ledger months
old shows no empty days or months before it on Trends, Projects, a project's WORTH BY among them,
the List, or Accounts' WORTH and Tokens, and WHEN YOU WORK averages its own days alone. Accounts'
weeks, on Compare's WEEKS and on Detail, are the accounts' own windows, so they reach back before
the pool's work, as an account's windows are its own whatever pool it worked in.

The pool shown follows the config as it changes: where a pool or an account is added, removed,
relabelled, fenced, moved or reordered, or an account's plan, primary or reserve changes, in any
pool, the view starts afresh on the pool shown, as `P` does, keeping the tab and what each tab
shows. A change to nothing of those, such as a comment, starts nothing. The dashboard looks at the
config file it reads, the one `--config` names, at each read, and reads the config again only where
the file has changed, or the accounts the router routes have: a look costs a stat of the file and a
compare of two lists of ids. Where the pool shown is removed, it turns to the first. A dashboard
opened on a config without pools, or where it couldn't tell the pools, looks at the config file
alike, so once the config names pools the view starts afresh on the pool the commands act on, every
tab that pool's alone; where every pool is fenced, there's none, and it shows none, as below.

Once there's a second pool:

- The title row names the pool shown after the badge, by its label, at every size.
- `P` turns to the next pool, in the config's order, the first after the last, on every tab,
  Projects and the List among them. It reads the pools afresh, so a pool added or removed while the
  dashboard is open shows at the next `P`. The view starts afresh, as when another router answers:
  nothing of the pool before carries over, its pages, what's chosen, a project's or a session's
  page, the routing card or its notifications. The tab shown stays, the period the dial shows, and
  what each tab shows as the dashboard remembers it (see [What's remembered](#whats-remembered)).
- Notifications lead with the pool's label, the router's and the dashboard's own, as
  `Acme · k3f9 · Acme 1 has room again`.

With one pool, `P` isn't listed, but reads the pools afresh all the same: once a second is added,
it turns to it, and otherwise says there's one pool.

A pool with no accounts draws the same page on every tab, Projects among them: its status line
says `no accounts`, and
the page says that a session started in its directories doesn't start, how to add an account to it
or move one in, and that its directories stay fenced until the pool is removed. The key line keeps
`tab`, `P`, `?` and `q` alone. The dashboard judges this from the router's view of the pool: an
account added to it shows at the next read, as does its last account taken out. The router takes
up a pool added to the config within about 3 seconds, but an account added only as it restarts,
at a quiet moment: till then, the status line says how the router stands, the restart it has due
and `no accounts routed yet`, and the page says that `switchboard service restart` takes it up
now.

Where every pool is fenced and no `--pool` names one, there's no pool the verbs act on: the
dashboard opens on none, with no accounts listed. Its status line says it couldn't read usage, as
every pool is fenced, naming the pools, and tries again at each read; the pages drawn from the
router's document stay empty, and it reads none of the router's events. `P` turns to the first
pool.

## How it reads the router

While the router answers:

- The dashboard looks at the router's view every 5 seconds. This costs nothing upstream.
- It doesn't have the router probe each interval. The router reads the idle accounts itself, on its
  own [rounds](../reference/probes-and-priming.md), each one nothing has read in the last 5 minutes.
- `R` has the router read them at once.

If the router stops answering, or a read of it fails, the router's last view stays on screen. The
status line says `■ the router isn't answering  ·  since 14:40:55`. This lasts until the router
answers again, or until the interval's read, or `R`, probes the accounts directly.

Without the router, the dashboard probes every account every interval. It probes sooner after a
window on screen resets, or after an account couldn't be read. It goes back to the router once the
router answers.

## What's remembered

The dashboard keeps some choices in `prefs.json`, in the state directory, as they change. The next
dashboard opens with them.

It remembers:

- the theme, or the pair
- the tab shown: a file from before History left the tabs that kept it opens Trends, with YEAR
  shown too, and keeps it so
- the Overview's sections shown, and its `b` grouping
- whether `w` shows each model's own window
- the period, one dial for the dashboard
- Accounts' view, and Tokens' split, which takes History's Tokens' split from a file from before
  History left the tabs, where it keeps none of its own
- Sessions' view
- whether `a` collapsed Runway's labels
- Trends' sections shown
- the Log's columns.

It never remembers:

- a focus or a choice
- a card or help, open
- a session's page
- what Trends' `v` shows, which lasts the run, or Runway's week stepped back to
- the Log's sub-tab or its filters
- a scroll.
