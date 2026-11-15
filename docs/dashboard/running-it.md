# Running it: one pool at a time, as you left it

The dashboard shows one pool at a time, and every tab shows only the work done in that pool. It
follows your config as you change it, and opens where you left it last time. This page covers
pools, config changes and what the dashboard remembers.

```bash
# Open it on the pool the verbs act on
switchboard dashboard
# Open it on the pool with id q7mt
switchboard dashboard --pool q7mt
```

The dashboard needs a terminal. Its flags, its interval and how it reads the router are on
[`switchboard dashboard`](../commands/dashboard.md).

## What each tab shows in a pool

If your config has [pools](../pools.md), the dashboard shows one: the pool `--pool` names, or else
the one the verbs act on. Each tab shows only that pool:

| Tab | Shows, in the pool |
|---|---|
| Overview, Runway | The accounts, their windows and room, the sessions running, the pin, and the router's events in LATELY. |
| Accounts | The accounts, and the worth and tokens of the work done in the pool. |
| Projects | The work on each project, the chosen project and its SESSIONS among it. |
| Sessions | The requests, worth and tokens of each session, and the List over every period. |
| The session page | The work the session did in the pool, in STORY and in TURNS. |
| Trends | The accounts, and the work in YEAR. |
| The Log | Today's requests and each day read back, with the figures for each day, and the router's events. Find finds only the pool's sessions. |

- A request made before you had pools counts as the first pool's.
- A session resumed in another pool, after you change its folder's fence, shows in each pool
  with the work it did there.
- A `claude` that a session starts, such as a test running `claude -p`, shows in the session's
  pool, unless a fence holds its folder.
- A project worked on in two pools shows only the work in the pool shown.
- The days begin where the work in the pool does. A pool added to a ledger months old shows no
  empty days or months before it.
- WHEN YOU WORK on Trends averages only the pool's own days.

The weeks on Accounts, in WEEKS on Compare and in Detail, are the accounts' own
windows. They reach back before the work in the pool, because an account's windows are
its own, whatever pool it worked in.

## Turning between pools

Once there's a second pool:

- The title row names the pool shown, by its label, after the badge, at every size.
- `P` turns to the next pool in the config's order, and from the last to the first, on every
  tab.
- `P` reads the pools afresh, so a pool you add or remove while the dashboard is open shows at the
  next `P`.
- Notifications lead with the pool's label, from the router and from the dashboard alike, such
  as `Acme · k3f9 · Acme 1 has room again`.

Turning to another pool starts the view afresh, as when another router answers:

| Carries over | Starts afresh |
|---|---|
| The tab shown | The pages, and what's chosen on them |
| The period the dial shows | The chosen project on Projects, or the session page if it's open |
| What each tab shows, as the dashboard remembers it | The routing card, and its notifications |

With one pool, `P` isn't listed, but it still reads the pools afresh. Once you add a second pool,
`P` turns to it. Until then, it says there's one pool.

## When the config changes

The dashboard follows the config file as it changes. At each read, it checks the file it reads,
which is the one `--config` names. It reads the config again only if the file has changed, or the
accounts the router routes have.

If any of these changes, in any pool, the view starts afresh on the pool shown, as `P` does:

- a pool or an account is added, removed, relabelled, fenced, moved or reordered;
- an account's `plan`, primary or reserve changes.

The tab and what each tab shows stay as they were. A change to none of those, such as a comment,
starts nothing. If the pool shown is removed, the dashboard turns to the first pool.

A dashboard opened on a config without pools does the same, and so does one that couldn't tell
the pools. Once the config names pools, the view starts afresh on the pool the verbs act on.

## A pool with no accounts

A pool with no accounts draws the same page on every tab:

- The status line says `no accounts`.
- The page says that a session started in its folders doesn't start.
- It says how to add an account to the pool, or move one in.
- It says that the folders stay fenced until you remove the pool.
- The key line keeps only `tab`, `P`, `?` and `q`.

The dashboard judges this from what the router reports. An account moved into the pool shows at
the next read, and so does the last account moved out. The router takes up a new pool within about
3 seconds, but a new account only when it restarts, at a quiet moment.

Until the router restarts, the status line says how the router stands, the restart it has due,
and `no accounts routed yet`. The page says that `switchboard service restart` takes the new
account up now.

## When every pool is fenced

If every pool is fenced and no `--pool` names one, there's no pool the verbs act on. The dashboard
opens on no pool:

- It lists no accounts.
- The status line says it couldn't read usage because every pool is fenced, and names the pools.
- It tries again at each read.
- The pages drawn from the router stay empty, and it reads none of the router's events.

`P` turns to the first pool.

## What the dashboard remembers

The dashboard keeps some of your choices in `prefs.json`, in the state directory, as they change.
The next dashboard opens with them. It remembers:

- the theme, or the pair;
- the tab shown;
- the sections shown on the Overview, and its `b` grouping;
- whether `w` shows each model's own window;
- the period, one dial for the whole dashboard;
- the view on Accounts, and the split on Tokens;
- the view on Sessions;
- whether `a` collapsed the labels on Runway;
- the sections shown on Trends;
- the columns on the Log.

It never remembers:

- a focus or a choice;
- an open card, or help;
- the session page, if it's open;
- what `v` shows on Trends, which lasts the run;
- a week you stepped back to on Runway;
- the sub-tab on the Log, or its filters;
- a scroll.

A file from before the History tab left opens on Trends, with YEAR shown too, and keeps it so.
If that file kept no Tokens split of its own, Accounts takes the one History kept.

If `prefs.json` can't be read, see [Themes](themes.md) for what happens to it.

## Related

- [`switchboard dashboard`](../commands/dashboard.md): the flags, the interval, and how it reads
  usage.
- [Pools](../pools.md): ring-fence a set of accounts to your folders.
- [Upgrades and restarts](../upgrades-and-restarts.md): what the dashboard waits on after an
  upgrade.
