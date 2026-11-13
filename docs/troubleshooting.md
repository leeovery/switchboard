# Troubleshooting

This page shows where to look when switchboard doesn't do what you expect, and how to read its logs.

## Where to start

- **Where things stand:** to see how each account stands and why, the router's health, any
  restart the router has due, and a move onto the accounts' ids or to pools it owes, run
  [`switchboard status`](commands.md#status). To see whether the LaunchAgent is installed and
  loaded, run [`switchboard service status`](commands.md#service).
- **Why a request went where it did:** to follow the router's log, run `switchboard logs -f`. The log
  has a line for each request, with its account and why it went there. To look back over the days,
  use [`requests`](commands.md#requests) and [`events`](commands.md#events).
- **A limit reset by hand:** to have the router read the accounts now, run `switchboard usage -r` or
  `status -r`, or press `R` on the dashboard.
- **Not routed:** some of what Claude Code sends never reaches the router. See
  [What doesn't go through the router](how-it-works.md#what-doesnt-go-through-the-router).
- **`claude` in a new terminal isn't switchboard's:** run `switchboard setup`. It asks your login
  shell which `claude` it runs, and says what it found. With switchboard's line in your startup
  file but another `claude` still run, something after the line changes `PATH`: move the line
  after it, to the end of the file, or what changes `PATH` to before it. Where you keep `PATH`
  another way, as in a file of your own or a tool that writes it, put switchboard's bin
  directory, `$XDG_DATA_HOME/switchboard/bin`, else `~/.local/share/switchboard/bin`, first on
  it there. `r` on `setup`'s `claude` link page checks again once you have. An alias named `claude` goes through switchboard
  where its command, after any plain variables, is `claude`, `\claude` or `command claude`, or
  switchboard's `claude` link by its path, absolute or from an unquoted `~/`, `$HOME/` or
  `${HOME}/`, with no variable before it that sets `PATH`, by `PATH=` or `PATH+=`, or in zsh
  `path=` or `path+=`. A function, or an alias `setup` doesn't read, such as one that runs
  `claude` after another command, as `caffeinate -i claude` or `cd ~/work && claude`, or one with
  more than plain words, letters, digits and `_./:@%+,=-`, goes through switchboard where it runs
  `claude` by name.
- **Completions:** switchboard's file for your shell loads them, for commands and flags. zsh takes
  them once `compinit` has run, which a usual `.zshrc` does before switchboard's line at its end;
  bash takes them where the bash-completion package is loaded; fish always does.
- **"This session isn't going through switchboard":** the session was started by a `claude` that
  isn't switchboard's, so its requests go straight to Anthropic on Claude Code's own login,
  unrouted and unrecorded. Started from a terminal, something has put the real `claude` ahead of
  switchboard's on `PATH`, such as a startup file changed: run `switchboard setup`, which puts
  switchboard's `claude` first, then start the session again in a new terminal. To see which
  `claude` your shell runs, run `command -v claude`. Started by an app that runs a Claude Code of
  its own, such as the Claude desktop app's Code sessions or an editor's bundled Claude Code, it
  can't be routed: `setup` won't change that. See
  [What doesn't go through the router](how-it-works.md#what-doesnt-go-through-the-router).
  See [A session that isn't going through switchboard](how-it-works.md#a-session-that-isnt-going-through-switchboard).
- **`setup` carries on without the session hook:** Claude Code's `settings.json` isn't the JSON
  object Claude Code reads, or its `hooks` isn't an object, or `hooks.SessionStart` isn't an array.
  `setup` leaves the file as it is, says what's wrong, with the line and column it can't read, or
  that the file starts with a byte order mark, and carries on. Put the file right, then run `setup`
  again. Where it says the file kept changing, Claude Code was writing it as `setup` did: run
  `setup` again. Where it says `switchboard is running as sb here`, you ran switchboard by
  another name, such as a link of your own or a build given another name: run `setup` as `switchboard`, by the path
  you installed it at.
- **`setup` says to make the terminal larger:** its screen needs 95 columns by 25 rows, so on a
  phone's narrow terminal, turn it sideways or use a larger one.
- **`c` on `setup`'s screen copies nothing:** it copies by OSC 52, which some terminals don't
  allow, or allow only once it's turned on, as tmux's `set-clipboard`. As `setup` leaves, it prints
  each step's line into the scrollback, with the line to add to your shell's startup file, or the
  directory to put first on `PATH`, to copy from there.
- **A token `setup` made but couldn't read or save:** it prints the token as `claude setup-token`
  showed it, once its screen has gone, as the token lasts a year on the account. Run `setup` again,
  and press `p` on its token card to paste it.
- **The ledger's digests:** the `ledger/digest-<date>.json.gz`, `ledger/parts-<date>.gz`,
  `ledger/logday-<date>.json.gz` and `ledger/rows-<date>.gz` files are safe to delete. They're
  worked out from the day's lines, and the router writes them again on its next round, or when it
  next starts. Until then, the dashboard reads the lines instead, which is slower but shows the
  same.
- **A session's page slow to open:** a damaged parts file whose size hasn't changed isn't noticed
  until a page reads it. The page then rests on an older digest, or reads the session's days whole,
  at every open, until the router writes the day again, as after a release that changes the
  digests' form. To have it written now, delete that day's `digest-<date>.json.gz` and
  `parts-<date>.gz`: the router writes both again on its next round.
- **The Log's find slow to go back:** the find reads each past day's head, and a day's lines only
  where its head isn't written yet. After the upgrade that brings heads in, the router writes one
  for every day kept, yesterday's first, 1 to 1.5 s of work a day with its rows, once it has written
  every digest due, which on a long ledger takes some minutes; until it reaches a day, the find
  reads that day's lines, half a second or more. Yesterday's head is written about an hour after
  midnight, just after its digest. A head damaged where its size hasn't changed is never written
  again on its own, and the find reads that day's lines at every walk: delete that day's
  `logday-<date>.json.gz`, and the router writes it again on its next round.
- **The Log slow to go back a day, or a turn slow to open:** the Log reads each past day from its
  rows, and a turn its own past days, all but yesterday until about an hour after midnight, which
  read the day's lines, as today's do. A day whose rows file or head is deleted, or damaged so its
  size changed, is safe: that day reads its lines instead, half a second or more, never showing
  otherwise, until the router writes both again on its next round. After the upgrade that brings
  rows in, the router writes them for every day kept, yesterday first, as it writes heads. A rows
  file damaged where its size hasn't changed isn't noticed by the router, and that day reads its
  lines at every day back: delete that day's `rows-<date>.gz`, and the router writes it again on its
  next round.
- **A restart held after an upgrade:** when the new version refuses your config, as it can where
  its rules are stricter, the router keeps running the old version rather than restart into one
  that would stop. The `switchboard` you run is the new version too, so it can't read the config
  either. `switchboard status` shows the router's document all the same, the restart given as
  held with the refusal (`restart.held` with `--json`), then fails with the config's error;
  `usage` and a dashboard you open now fail on the config, while one already open shows
  `restart held (upgraded)` on its status line; and the router's log has it at warn. Until the
  config is put right, each `claude` you start says it couldn't read the config and is routed
  through the running router all the same, on the accounts the router names; only where the router
  doesn't answer does it start without switchboard. Put the config right as it says, and the
  router restarts at its next quiet moment. See [Restarts](how-it-works.md#restarts).
- **An upgrade not taken up, as the disk is full:** the router copies a new version in beside its
  own copy before it restarts into it, and where it can't, it carries on as it is, the restart
  still due (`restart due (upgraded)`), and says why once at warn in its log. It copies the
  upgrade again at each moment with no request in flight, so once there's room it takes the
  upgrade up by itself; `switchboard service restart` meanwhile fails, saying why.
- **`service status` says the router's copy is not run:** the LaunchAgent was written before the
  router had its own copy of switchboard, and runs switchboard itself, so macOS asks again what
  you allow the router after each upgrade. The router moves over to its copy by itself as it
  starts, and where it can't, as when it can't write the LaunchAgent, its log says why at warn
  (`can't move over to the router's own copy of switchboard`). Run
  `switchboard service install` to write the LaunchAgent afresh. See
  [Restarting in place](commands.md#restarting-in-place).
- **`service status` says launchd runs the router's copy from the next login:** launchd reads the
  LaunchAgent only as it loads it, at login, so after the router moved over to its copy, launchd
  starting the router again, as after a crash, starts switchboard itself, as before the copy, until
  the next login: an upgrade landed since, never asked whether it could start, may refuse the
  config. The next login sets it right by itself. To have launchd run the copy now, run
  `switchboard service install`, which has launchd load the LaunchAgent afresh; `setup` runs it
  too.
- **`claude` won't start, the router down:** with the router not running, not answering, or not
  saying where it listens, `claude` starts nothing and says so. An unhealthy router doesn't stop it:
  `claude` goes through it, saying so. Run [`switchboard service status`](commands.md#service) to
  see whether the LaunchAgent is installed and loaded and how the router answers, then `switchboard
  service install` or `switchboard service restart`. To start Claude Code without the router this
  once, unrouted and unrecorded, run `SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude`; to have it do so
  always, set [`when_router_down = "direct"`](configuration.md#top-level-keys).
- **A pin that may or may not have been made:** a router stopping or restarting as `pin`, or a pin
  on the dashboard, asks it closes without answering, though it may have made the pin first. Run
  `switchboard status` to see whether it did, and pin again if not.
- **Getting round switchboard:** to start Claude Code on its own login, run
  `switchboard run --direct`, or run the real `claude` by its path. If the switchboard binary itself
  breaks, every `claude` breaks with it, until you take switchboard's directory off `PATH`: remove
  the line marked `# switchboard` from your shell's startup file, and open a new terminal.
- **Pools:** with pools, `claude`, the router and the verbs can refuse for a pool's sake. See
  [Pools](#pools).

## The upgrade to account ids

The first time the router starts on a version that makes account ids, it moves your accounts and
their history onto them (see [Upgrading to account ids](how-it-works.md#upgrading-to-account-ids)).

### A move onto the ids that waits

While the move is owed, the dashboard says what it waits on and opens once it's done, and
`requests`, `history`, `sessions`, `events`, `export` and `import` refuse, saying the same.
`switchboard status` says it on its last line, and `switchboard status --json | jq .moving` gives
its `stage`, and, where it waits on the API, the `account`, by the id it had, and `why`.

- **`the API hasn't named the organization of <account>'s token`:** the API didn't answer for
  that account's token, or answered in a way that says to ask again, as 429 or a 5xx does, or a
  proxy between answered in its place, without Anthropic's request id. The router asks again, 30
  seconds after the first time, then twice as long each time, 5 minutes apart at the most, and
  routes meanwhile. Check the network, and any proxy or VPN between you and `api.anthropic.com`;
  `switchboard logs -f` shows each ask. Where the API answers but names no organization, as when it
  refuses the token, the router asks for 10 minutes before it takes that as the answer, and gives
  the account a random id.
- **`the router makes the rest once it restarts onto the new ids its config holds`:** the config
  holds the new ids, and the router is still routing under the old. The service's router restarts
  once no request is in flight; run `switchboard service restart` to have it now. A router you
  started by hand with `serve`: stop it and start it again.
- **`no router answers to make it`:** the move is the router's to make. Start it with
  `switchboard service install`, or `switchboard serve`.
- **`the router running is of a version from before the ids`:** the router hasn't restarted into
  the upgrade yet, as when a restart is held or waits on a request in flight. It makes the move
  once it does: see [Restarts](how-it-works.md#restarts).
- **`the router couldn't make it, and tries again in a minute`:** a step failed, as a file it
  couldn't write, or an accounts record that doesn't read. The reason follows, and the router's
  log has it at warn. Put that right and it carries on, as nothing is lost part way.
- **Two Macs sharing a config:** a Mac still on an earlier version finds the config naming ids it
  has no token files for. Upgrade every Mac that shares the config. A Mac that upgrades while
  its network is down waits to join its accounts to the ids the first Mac made: it carries on
  once the API answers.

A day's compressed file the move finds damaged is repaired as it's rewritten, its lines before the
damage kept, and the file set aside beside it as `<name>.damaged`, warned of in the router's log
(see [Where things live](reference.md#where-things-live)).

### An import refused across the upgrade

`import` names each Mac's folder it passes over, and why, and takes in the rest:

- **`another Mac exported it before switchboard made account ids`:** that folder's days name the
  other Mac's old ids, which this Mac can't tell. On that Mac, upgrade switchboard, let its move
  finish, then export again, and import here once the folder has synced. This Mac's own folder
  from before the upgrade is taken in as it is.
- **`its accounts record hasn't synced yet`:** the folder's `export.json` says it holds the
  accounts record, which isn't there yet. Import again once the sync is done.
- **`it holds an export without its export.json`:** an export cut short, or a sync not done, left
  the days without the file that says whose they are. Export again on that Mac, or wait for the
  sync, then import again.
- **`its accounts record doesn't read`:** the folder's `accounts.json` is damaged. Export again on
  that Mac.
- **`there's nothing to import in <dir>: each Mac's folder in it was passed over`:** every folder
  was passed over, each for the reason named above it.
- **`the history waits on its move onto the accounts' new ids`:** this Mac's own move isn't done
  yet: see above, and import once it is.

### A damaged accounts record

`accounts.json` that doesn't read, as one edited wrongly by hand, is left as it is: adding or
removing an account fails, naming it, and History and the verbs name each removed account by its
id meanwhile. Put it right, or move it aside: see
[The accounts record](reference.md#the-accounts-record).

## Pools

What a pool can refuse, and what to do about it. See [Pools](how-it-works.md#pools) for what they
are.

- **`claude` starts nothing in a directory:** it says why, ending `— not starting claude`: no pool
  covers the directory, two pools' fences hold it through its links, the pool has no accounts, or
  none of them has a usable token, or it's inside a fence and would start on Claude Code's own
  login. Each is in [`claude`](commands.md#claude)'s table, with what to do.
- **`the router doesn't route by the config's pools (…)`:** or `the router routes by other pools
  (…)`, where the pools come from the router or the accounts record. With two pools or more,
  `claude` counts a router down where its pools, their directories, accounts or primaries aren't
  those it launches by, as a router from before pools would spread a session across every
  account. It waits 5 seconds for
  the router to take a change up first, so this is a router that can't: one from before pools,
  still running after an upgrade, or one that routes none of a pool's accounts yet, as they were
  all added since it started. Run `switchboard service restart`. Meanwhile it stops, or goes
  direct on the pool's accounts, as `when_router_down` says.
- **A request answered `switchboard: no account in pool Client can take the request; …`:** none of
  the pool's accounts can take it, and nothing falls back to another pool's. What follows the `;`
  says why of each account, as `account m4zq: …`, the problem it has, or `taken up when the router
  restarts` for one added since it started. Give the account what it lacks, or wait for a window
  to reset, as with one pool whose accounts are all out.
- **A session answered `switchboard: this session's account has left pool Client — exit and resume
  it to carry on in the pool its directory is in now`:** the account whose token the session
  started on is no longer in the session's pool, as when the pool's primary moved to another pool,
  or the pool was removed (`this session's pool isn't configured`). Claude Code can't change that
  token while it runs. Exit the session and resume it: it starts in the pool its directory is in
  now, on that pool's primary.
- **A verb says `every pool is fenced: name a pool, q7mt (Acme) or r2d4 (Client), with --pool`:**
  with every pool fenced, there's no pool to read without being told. Pass `--pool <id>`. A verb
  never takes its pool from the directory you run it in. See
  [Reading a pool](commands.md#reading-a-pool).
- **A verb says `can't tell which pool to read: neither the config, the router nor the accounts
  record names any pools`:** once the history has moved to pools, the config names none, or can't
  be read, the router isn't running or routes by none, and the accounts record keeps none, so no
  verb can read one pool's work apart from another's. Put the config's pools back, or start the
  router.
- **A verb says `there's no pool "q7mt": name …` of a pool you removed:** `status`, `usage` and
  `pin` read the router, which routes nothing in a pool the config no longer has. Its history
  stays: `requests`, `history`, `sessions` and `events` read it with `--pool q7mt`, naming it
  `pool Acme (removed 6 Oct)`. They read a pool another Mac's history brought in by `import` so
  too, naming it `not configured here`, as `pool Client (not configured here)`.
  See [Reading a pool](commands.md#reading-a-pool).
- **`status` says `probed directly: the router doesn't route by pools yet`,** or the dashboard
  `○ probed · router started before pools`: the config has pools and the router reports none, as
  one from before pools still running after an upgrade, so its document and sessions are every
  account's, never a pool's: `status`, `usage` and the dashboard probe the pool's accounts instead,
  `sessions` lists the ledger's alone, and `pin` and `status --session --pool` refuse. Run
  `switchboard service restart`.
- **The dashboard says `couldn't read usage: every pool is fenced: …`:** with every pool fenced,
  there's no pool for it to show without being told, and this release's dashboard takes no
  `--pool`. Read a pool with `switchboard status --pool <id>`, or leave one pool unfenced with
  `pool edit <id> --unfence`.
- **`setup` stops adding an account, saying `every pool is fenced, so which the account goes in is
  yours to say`:** it adds an account to the pool `accounts add` takes without `--pool`, and with
  every pool fenced there's none. It stops before making a token. Add the account with
  `switchboard accounts add <label> --pool <id>`, then run `setup` again for the rest.
- **A client's session in a directory you've just fenced still runs on your accounts:** a
  session's pool is decided as it starts, so one already running when `pool add` or `pool edit`
  made the fence keeps its old pool till it's resumed. Resume the sessions running there.
- **`pin` says `pin accounts of one pool: …`:** a pin is its pool's, so the accounts given must be
  in one. Pin each pool's accounts apart. A session can be pinned only to an account of its own
  pool.
- **`pool add` says the config names no pools:** the router makes the first pool as it moves the
  history to pools, the first time it starts on a version with them, and writes a config's pools
  back once the move is done, where it lost them. Once the config names them, add the rest.
- **`pool remove` is refused:** the pool still has accounts, which it names, or it's the last pool.
  Move each account to another pool with `accounts edit <id> --pool <id>`, or remove it, first.
- **`pool edit --dirs` says to set them by hand:** the config writes the pool's `dirs` across
  lines, which it can't change in place. Edit them in the config, or write them on one line.
- **The config is refused for its pools:** the error names what's wrong: two pools without `dirs`,
  as only one can be unfenced; directories that are the same, or one inside another; an account
  without a `pool`, or naming one that isn't configured; two primaries in one pool. Put the config
  right as it says. See [`[[pool]]` keys](configuration.md#pool-keys).
- **A config put back without its pools:** once the history has moved to pools, the router gives a
  config that names no pools its pools back from the accounts record, and routes by them
  meanwhile, whether or not it can write them, so no session crosses a fence. Where it can't, as
  where every pool is fenced and an account added since has no pool to go to, it says why once in
  its log, and tries again each minute: give that account a pool, or put the config's pools back
  yourself. Where the accounts record can't be read either, and the router never routed within any
  pool, it routes nothing, answering every request with switchboard's refusal saying why, and
  reports itself unhealthy, till the config has its pools or the record reads again. `claude`
  routes through it all the same, its sessions refused till then, rather than going direct past
  the fences.
- **The move to pools waits, `awaiting restart`:** the router writes the first pool into the
  config, then takes it up before it moves any file. Where it can't read the config as it takes it
  up, the move waits: put the config right, as `switchboard status` says. The move never waits on
  the API.

## A `wrong token` account

An account that reads `wrong token`, or `its token is another subscription's`, holds in its token
file a token the API says is for another subscription than the account's, as where the file was
replaced by hand with another account's token, or a token was made while the browser was signed in
to the wrong account. The router sends nothing on it: the account takes no requests, its sessions
go to the others, and what Claude Code sends to other paths passes through as before. Where no
other account can take a request, Claude Code gets a 503 saying the same.

- **The token was meant for this account:** make it a token of its own with
  `switchboard accounts token <id>`, approving it in the browser signed in to that subscription, or
  with setup's `t`. `accounts token` refuses a token of another subscription, so the right one is
  all it saves.
- **The token was meant to be used:** add its subscription as an account of its own, with
  `switchboard accounts add <label>`, then give this account its own token.
- **It stays after a restart:** the finding is kept in `state.json` until the account takes up
  another token, which the router reads within 3 seconds of the file changing.
- **`switchboard status --json`** marks the account `token_foreign`, and `switchboard events`
  holds a `foreign` event of when it was found.

## An export that fails

With [`[export]`](configuration.md#export-keys)'s `dir` set, the router exports every hour. To see
how its exports stand, run `switchboard status --json | jq .export`: `at` is when the last
succeeded, `error` why the last failed, and `running` when the one under way began. The router's
log has a failure at warn, once until an export succeeds, and the first that succeeds after it.

- **`macOS refused access to <dir>`:** macOS asks before the router writes into iCloud Drive,
  Dropbox or `~/Documents`, and you didn't allow it. Allow switchboard in System Settings →
  Privacy & Security, and the next export, within the hour, goes through. macOS asks about a
  router you run by hand, with `switchboard serve`, as part of the terminal you run it in, so
  then it says to allow that terminal app.
- **`<path> is locked`:** a file or folder in the export is locked in the Finder. Unlock it in its
  Get Info window.
- **The folder isn't there:** the router makes `dir` only where the folder it goes in is there, so
  a disk that isn't mounted isn't written onto the Mac's own disk in its place. Mount the disk, or
  make the folder `dir` goes in.
- **`running` for long:** an export waits while macOS's prompt asking whether to allow switchboard
  is unanswered, and no other into the folder starts until it's answered. Nor does the router
  restart in place meanwhile, so an upgrade or a config change waits, `switchboard status` says
  the restart waits for the export, and `service restart` says `an export into <dir> is under
  way: try again once it ends`. Answer the prompt, or allow switchboard in System Settings →
  Privacy & Security.
- **A day passed over:** a day's file in the state directory that can't be read is warned of in
  the router's log, as `couldn't export the request ledger of 2026-10-30; passed over`, and the
  rest exported. That doesn't fail the export.
- **`can't remove a file an export cut short left; trying again at each round`:** an export cut
  short, as by the router stopping mid-export, left a hidden `.<name>.<digits>` file in the
  export, which the router can't remove. That doesn't fail the export. The line names the file:
  check its permissions, and whether it or its folder is locked in the Finder, or remove it by
  hand.
- **To export now:** run [`switchboard export <dir>`](commands.md#export), which needs no router.
  macOS asks about your terminal apart from the router, so an export that works from the terminal
  says nothing of what the router is allowed.

## Notifications that don't show

Notifications come from switchboard's own app, `Switchboard` (see
[Switchboard's own notifications](configuration.md#switchboards-own-notifications)).

- **Turned off:** macOS asked once whether `Switchboard` may send notifications, and they were
  turned off. Turn them on in System Settings → Notifications → Switchboard. Until you do, the
  router's log says so at warn, once until a notification goes out:
  `macOS won't let Switchboard post notifications: turn them on in System Settings → Notifications → Switchboard, or, where they're on, switchboard service restart registers its app again`.
- **On, but not allowed all the same:** macOS can lose track of the app. Run
  [`switchboard service restart`](commands.md#service-restart): the router registers the app again
  as it starts.
- **Never asked:** macOS asks when the router first starts on a release with the app. Run
  [`switchboard service restart`](commands.md#service-restart) to have it ask. Where the question
  can't be put, the router's log says
  `can't ask macOS whether Switchboard may post notifications; posting through its app all the same`,
  with macOS's reason.
- **Posted as Script Editor:** where switchboard can't write out or register its app, its log says
  `can't write out the notifications' app, so notifications are posted through osascript`, with
  why, and it posts through `osascript` until it next starts. A switchboard built from source has
  no app, and always does.
- **A notification that fails:** the router's log has it at warn as `notification failed`, and
  drops it. See [How often notifications go out](configuration.md#how-often-notifications-go-out).

## The logs

The logs are for working out, after the fact, why a session went to an account, why a request
failed, or why the dashboard showed what it did. They're in `<state dir>/logs/`:

| File | What writes it |
|---|---|
| `router.log` | The router. |
| `cli.log` | Every other command. |
| `launchd.log` | launchd, with the service's own output, such as a crash's. |

Each record is a line of logfmt. The router notes every request it routes, with its account and why
it went there:

```
time=2026-09-28T14:12:00.123+01:00 level=INFO msg=routed component=router pid=4242 id=5f3a9c2e session=18bb978f model=claude-opus-5-5 account=q4tx reason="moved: b6s7 hit its limit" status=200 attempts=2 duration=9.412s
```

- **Level:** `SWITCHBOARD_LOG_LEVEL` is `debug`, `info`, `warn` or `error`, and is `info` unless
  set. For the router, `serve --log-level` overrides it, and so does `service install --log-level`,
  which passes it on.
- **Rotation:** `router.log` and `cli.log` roll over before they'd pass 10 MiB, and each keeps five
  old files. `launchd.log`, which launchd writes, isn't rolled over: once it passes 10 MiB, it's
  cut to its newest 5 MiB as the service starts the router.
- **Redaction:** nothing logs a token or an account's label. Accounts appear by id, and anything
  shaped like a token is replaced with `[redacted]`.
- **Never in the way:** no command logs to stdout or stderr, except `serve` at a terminal. So
  `--json` and the dashboard stay clean. No command fails because it couldn't log.

To print and follow the logs, use [`switchboard logs`](commands.md#logs).
