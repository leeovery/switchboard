# How it works

This page explains how switchboard routes each request, what it records, and how it keeps itself
running. It also covers the primary account, reserves, priming, what bypasses the router, and how
Claude uses switchboard.

```
Claude Code ──ANTHROPIC_BASE_URL──▶ switchboard ──▶ api.anthropic.com
                                      │
                                      ├─ chooses the account: sticky per session and model
                                      ├─ swaps in that account's token
                                      ├─ reads every response's usage headers
                                      └─ records the request, and what the router did
```

Every `claude` you run starts the real Claude Code connected to the router. The router chooses an
account for each session, swaps in that account's token, and reads the usage off every answer. It
records each reading, request and event, so the dashboard and the commands can read them back.

- [Routing](#routing)
- [What it records](#what-it-records)
- [Looking after itself](#looking-after-itself)
- [The primary](#the-primary)
- [Reserves](#reserves)
- [Priming](#priming)
- [What doesn't go through the router](#what-doesnt-go-through-the-router)
- [Claude and switchboard](#claude-and-switchboard)

## Routing

The router decides which account each request goes out on. It keeps a session on one account while
the session's cache is warm, and moves the session when that account reaches a limit.

Anthropic measures each account's use in windows. A window fills as the account is used and resets
at a set time. When a window is full, the account has reached its limit for the requests that window
counts.

### How requests reach the router

`setup` puts a link named `claude` in switchboard's own bin directory. You put that directory on
`PATH` ahead of the real `claude`.

When switchboard runs by the name `claude`, it starts the real Claude Code connected to the router.
This works wherever `claude` runs: from a shell, a tmux pane, a script, or a tool that runs
`claude -p`.

### What the router changes in a request

A conversation request is a request to `/v1/messages` or its `count_tokens`. For each conversation
request, switchboard replaces the `Authorization` header with the chosen account's token.

Switchboard never edits a request's body. The request is still Claude Code's own, so Claude's
thinking stays valid from turn to turn.

On a conversation request, switchboard also narrows `Accept-Encoding` to gzip, deflate and identity.
These are the encodings it can read a copy of the answer in, so it can count the answer as it
streams.

Every other request passes through on Claude Code's own token, as it came.

On every request, conversation or not, switchboard removes its own headers, named `X-Switchboard-…`,
so the API never sees one. These are its headers:

- The pin that `run --account` sets.
- The directory that `run` names for the ledger.
- Any that a newer `run` sends.

### Reading usage off real traffic

Every response carries the utilization and reset time of each of the account's windows:

- The 5-hour window.
- The weekly window that every model shares.
- A model's own weekly window, where the model has one.

Only a 429 that refuses the request itself carries none. Switchboard reads the windows off every
response, so looking costs next to nothing.

To read an account without traffic, switchboard probes it. A probe is one request per model family,
each capped at one output token. Switchboard probes an account only in these cases:

- Nothing has been read of the account.
- A decision needs fresher numbers than traffic has given.
- A dashboard asks.
- The account is due a [prime](#priming).
- The router's own [rounds](#the-routers-rounds) call for it: once nothing has read the account in
  5 minutes, and 5 minutes before each of the account's weeks resets.

### Choosing an account

A new session goes to the account whose quota most needs using. The candidates are the accounts with
room in every window that the session's model counts against.

Each candidate's score is the room left in its shared weekly window, divided by the hours until that
window resets. This has two effects:

- Quota that resets tomorrow is used before quota that resets next week.
- An account with little left scores low, unless its week resets soon.

Among the accounts that score at least 0.8 of the best, the one whose 5-hour window resets soonest
wins. Whatever is left in a window when it resets is lost. With [priming](#priming), the accounts'
resets are spread through the day.

### Sticky sessions, for the cache

Prompt caches are per account. The first turn after a session moves costs around 40 times a warm
turn. So the router keeps each session on its account:

- **Remembering a session:** the router remembers a session by its session id and model once a
  request of that session is answered with success. A resumed session finds its account again. A
  request that fails, under an id never used again, leaves nothing behind. The quota check that
  `claude --resume` sends as it starts can be such a request.
- **While the cache is warm:** the session stays on its account for an hour after its last request,
  as long as the account has room.
- **After an hour idle:** the session's cache is cold, and a move costs nothing. The router scores
  the session again. The session keeps its own account unless another account beats it by 20%.
- **After a compaction:** a busy session never idles for an hour. Without this check, it would stay
  on its account until that account ran out, while another account's week reset with room nobody
  used. A compaction is the cheap moment to move, as only the system prompt and tools have to be
  cached again. So at the first request after a compaction, the router looks at the account it would
  choose for a new session. The session moves there if that account's week resets sooner than its
  own account's, and, at the rate it's been used lately, would reset with more than 2% of the week
  unused, above any reserve.

A Claude Sonnet 5.5 session isn't scored again for idling, nor moved after a compaction. Its
thinking works only on the account that produced it, and a move would lose it. A session's own pin
holds it through a compaction too.

### Pressure

Several busy sessions on one account run its 5-hour window out together. Then they all move at once,
and each one rebuilds its cache elsewhere. To avoid this, the router watches each account's 5-hour
window and steers sessions off an account that is heading for its limit.

- **The rate:** how fast the window's use rose over the last 30 minutes. Where the readings have a
  longer gap, the rise is spread over the whole gap. Use outside the router shows only when the
  router next reads the account, so an account gone quiet reads quiet. For a window with no reading
  from 10 minutes back, the rate is its use since the window started.
- **Under pressure:** an account is under pressure when, at that rate, its window reaches its
  reserve before the window resets. For an account without a reserve, or a pinned account, the
  mark is its limit instead.
- **What moves:** a request that needs an account chosen afresh goes to the best of the accounts
  not under pressure, the pin's accounts first. These are the requests that move:
  - A new session.
  - A request without a session.
  - A session chosen afresh after an idle hour, because its account can't serve it, or because its
    own pin yields.
  - A busy session that moves after its compaction.
- **What stays:** running sessions stay where they are, and a session's own pin holds. When every
  account is under pressure, pressure changes nothing.

### Limits and refusals

The API can refuse a request, or answer that an account has reached a limit. The router handles each
case so that Claude Code sees as little of it as possible.

- **A limit:** a 429 can say that a limit is reached. The router then replays the request on the
  next candidate account, before any of the answer reaches Claude Code. The session moves to that
  account and stays there. You get one slower turn while the session's cache rebuilds, and you don't
  need to exit or resume.

  The account then sits out the requests that the limit counts. For example, a limit on Fable's own
  weekly window leaves the account's sessions on other models where they are. The account sits out
  until the reset that the 429 gives, or for five minutes when the 429 gives none. It comes back
  sooner when a request sent since shows the limit has lifted, as happens after you reset a limit by
  hand.
- **Throttling:** a 429 that's only throttling waits and retries on the same account, at most twice.
  Moving would throw the cache away for nothing.
- **A 429 without usage headers:** this says nothing about the account, but refuses the request
  itself. It reaches Claude Code at once, as it came.
- **A refusal:** when the API refuses a request, the router replays the request on another account
  too. It never relays the refusal, because Claude Code drops its login on a 403.
  - An account whose token the API refuses sits out every request for ten minutes.
  - An account that refuses the request itself sits out that model's requests for ten minutes.
  - When every account the request went out on refused the request itself, that says more about
    the request than about the accounts. No account sits out for it, and its session stays where
    it was.
- **Nowhere to go:** when no account has room, Claude Code gets a 429, as it would from one account
  at its limit. When every account has refused the request, Claude Code gets a 502 that gives the
  API's reason and tells it not to retry.

### Pins

A pin sends sessions to an account you choose, instead of the one the router would choose. You can
set a pin in these ways:

- `switchboard pin` sends new sessions to one account, or to the best of several. It can also move
  running sessions there.
- `pin --session` pins one running session.
- `switchboard run --account` pins a session as it starts.
- The dashboard's **routing card** sets the same pins.
- Claude sets them when you ask.

Every pin yields at a limit rather than failing. For the details, see [`pin`](commands.md#pin).

## What it records

Switchboard keeps a record of everything the router reads and does, in the state directory. The
dashboard and the commands read it back to look at the past, with or without the router. The file
formats are in the [Reference](reference.md).

### State that outlasts restarts

The router keeps these in `state.json`, so a restart doesn't scatter sessions or need a probe:

- Each session's account.
- The pins.
- Each account's last readings.

### A readings history

Switchboard appends each reading that changes how one of an account's windows reads to the day's
file in `history/`. When nothing moved, it writes nothing. The history is there for you to look
back at how the accounts were used.

- **How long it's kept:** 400 days, unless [`[history]`](configuration.md#history-keys) sets another
  length. That can be `forever`.
- **At startup:** the router takes up the readings of the two newest days, so the recent rates
  outlast a restart.
- **Never in routing's way:** a line switchboard can't write goes unwritten, and is logged once.

### A request ledger

The router writes a line for each request it routes, when it finishes with the request. This
includes requests that the router answered itself, as well as those it sent upstream. The lines go
to one file a day in `ledger/`.

- **How long it's kept:** 400 days, unless [`[ledger]`](configuration.md#ledger-keys) sets another
  length. That can be `forever`. A day's file is never pruned before the day's summary is written.
- **What a line holds:** everything about a request except its content. That is the request's
  session, model and account, why it went there, its timing, its shape and settings, and the
  answer's usage. A line never holds the request's messages, its system prompt or its tools.
- **Never in a request's way:** the lines wait, 8,192 at most, for a goroutine of their own to write
  them. A line the router can't write goes unwritten, and is logged once.
- **The day's summary:** an hour after each day ends, the router summarises the day beside its
  lines. The hour gives requests still in flight time to end. The summary holds each account's
  requests, sessions, moves, limits and highest use, and is kept for good.
- **The day's digest:** once a day's summary is written, the router writes a digest beside it,
  `digest-<date>.json.gz`. It holds each session that ran that day as it stood at the day's end,
  its page among it. Beside it, `parts-<date>.gz` holds what each session's page gained that day:
  the turns it added to and the subagents it first saw, and apart from them, when its requests
  went upstream, which only a page being shown reads. The dashboard reads a session from its
  newest digest and the days after it, and a session's page from its newest digest and the parts
  before it, so Sessions' list and a session's page open without reading every day a session has
  run. A digest holds no prices: worth is worked out as it's shown. The day's lines stay the
  truth. A digest or a parts file that's missing, out of date or damaged is passed over, and an
  older digest or the lines read instead, which is slower but shows the same.
- **The day's head and rows:** beside the digest, `logday-<date>.json.gz` lists each session that
  ran that day, with the directory it ran in, so the Log's find goes back a month in a moment,
  never reading the days' lines. It also says where the day's rows lie in `rows-<date>.gz`, which
  holds the Log's row of each of the day's requests, 256 to a chunk, so the Log goes back a day,
  and a turn opens, without reading the day's lines. Neither holds a price, and both are kept as
  long as the day's lines. Where either is missing, out of date or damaged, the Log reads the
  day's lines instead, which is slower but shows the same.
- **Reading it back:** [`requests`](commands.md#requests), [`history`](commands.md#history) and
  [`sessions`](commands.md#sessions) read it all back, with or without the router.

### The router's events

Beside the ledger's lines, the router files its events, so they outlast a restart. It writes a JSON
line for each version of an event. These are the events it files:

- A session started or moved.
- An account under pressure, at its cap, at its limit, refused, primed, or with room again.
- Routing set by hand.
- A restart falling due.
- The router's own health turning.

[`events`](commands.md#events) reads them back.

## Looking after itself

The router takes up changes to tokens and config, and recovers from sleep, without a command from
you.

- **Tokens:** the router takes up a change to a token file as it comes. A token file caught empty
  while it's rewritten doesn't cost its account its token.
- **Sleep:** once your Mac wakes, the router sends the requests it routes upstream on fresh
  connections, as a sleep can leave the connections it kept dead. Probes go out on connections of
  their own, and each is given 5 seconds.

### Restarts

The service's router restarts itself, once no request is in flight, when any of these happen:

- Its config changes.
- `brew upgrade` replaces it.
- Your Mac's time zone changes.

So `accounts add`, `accounts edit`, `accounts remove`, an edit by hand, an upgrade and a new time
zone all take effect without a command. A router you started by hand with `serve` logs that a
restart is due instead.

The router restarts in place. It runs its new binary in the same process and hands that binary the
sockets it listens on. A request made meanwhile waits a moment rather than being refused, and a
`claude` started while the router finishes up is still routed.

Before it restarts, the router asks the binary it's about to become whether it can start from your
config, as a newer version's rules can be stricter than the running one's, such as the 100-character
limit on labels. If the new version refuses, the router carries on as it is, and says why once: in
its log, in its events, and as `restart.held` in `switchboard status --json`. `switchboard status`
reads `restart due since Mon 14:02 (upgraded), held:` and the reason, and the dashboard's status
line `restart held (upgraded)`. The `switchboard` you run is the new version too, so it can't read
the config either: `switchboard status` shows the router's document all the same, then fails with
the config's error, `usage` and a dashboard you open then fail on it, and each `claude` starts
without switchboard until you put the config right. The router asks again once you change the
config, or the binary changes again. A version from before this check can't answer, and the router
restarts into it as it always has.

The new version judges your config for an upgrade: an edit to the config that only it takes, such
as a key it brings, doesn't hold the upgrade off. A version too old to answer can't judge it, so
the running version does: where it refuses your config, the restart is held with its reason
until you change the config or the binary.

Restarting in place also means launchd isn't asked to start an upgraded binary afresh. macOS has
been seen to refuse that, while it lets the router run the new binary in place, as the first such
upgrade showed.

With many long sessions, a moment with no request in flight can be hours coming. While a restart is
due, `switchboard status` and the dashboard say so. To restart now, run `switchboard service
restart`. See [`serve`](commands.md#serve).

## The primary

One account is the primary: the one your browser and the Claude apps are signed into.

- **Choosing it:** to mark an account as the primary, run
  `switchboard accounts edit <id> --primary`, or set `primary = true` on it. Without it, the first
  account is the primary. The sessions already running keep the token they started on as Claude
  Code's own until they restart: the former primary's, pinned or not, or, where it had no usable
  token, the first account's with one, or a pinned session's pinned account's. So what they send
  outside the conversation still goes out on that account, as the edit says.
- **Removing it:** when you remove the primary, the account marked `primary = true`, or else the
  first account, is the primary from then on, as `accounts remove` says. The sessions already
  running hold the removed account's token. They stay routed while the new primary has a usable
  token.

Artifacts live on the primary. In every routed session, Claude Code's own token is the primary's,
whichever account the conversation goes to. That includes a pinned session, because a pin moves the
conversation alone. While the primary's token isn't usable, Claude Code's own token is another
account's: see [`claude`](commands.md#claude).

So what Claude Code sends that isn't the conversation goes out on the primary, such as publishing an
artifact or uploading a file. Every session's artifacts open in a browser signed into the primary.

## Reserves

Any account can keep a reserve: a share of every window that the router leaves unused, for use
outside the router. For example, the primary might keep one for the Claude apps.

- **The default:** an account's reserve is `0` unless its `reserve` sets another. With none set,
  the router runs every account to its limit.
- **At the reserve:** once a window reaches its reserve, the router's own choices pass the account
  over. With a reserve of `0.1`, that's at 90%. New sessions skip the account, and sessions on it
  move as they would at a limit.
- **Never spent by the router:** the router never spends a reserve, even when no other account has
  room.

### Pins spend the reserve

The reserve holds back only the router's choices. A pin is your choice, so a pinned account runs to
its limit.

Suppose an account is at its reserve and every other account is out. To carry the running sessions
on there, run `switchboard pin <account> --move`. To hand them back to the router, reserve and all,
run `switchboard pin auto`.

Readings come off responses, so one large turn can take an account a point or two past its reserve
before the router sees it. Use outside the router, such as in the Claude apps on the primary, takes
it further, as intended.

## Priming

Anthropic's 5-hour window starts with an account's first message after its last window ended. It
resets five hours later.

Left alone, an account's first window starts with the day's first request on it. So an 08:00–23:00
day meets three of the account's windows, and accounts started together come back together.

With a day set in [`[prime]`](configuration.md#prime-keys), switchboard starts each account's window
at a time of its own, so the accounts come back one at a time. Once all are spent, the wait for the
next is at most 5 hours ÷ the number of accounts. Starting an account's window this way is called
priming it.

### The schedule

With N accounts that have usable tokens, switchboard works out the schedule in these steps:

1. Resets fall every 5 hours ÷ N. The first falls half a step after the day starts.
2. Each account, in config order, is primed five hours before its first reset. That time is the
   account's slot.
3. Each slot is taken back to the ten-minute mark it falls in, and the resets it predicts move back
   with it.
4. Each prime goes out 5 seconds after its slot. The Mac's clock runs a little ahead of the API's,
   and the 5 seconds make sure the prime lands within the slot's ten minutes.

Every reset seen falls on a ten-minute mark, because the API takes a window's start back to its
mark. So the schedule shows where each window really starts and resets. With two accounts, the
first is primed at 04:10 and resets at 09:10, not at 04:15 and 09:15.

Because slots are taken back and never on, every slot stays before the day starts. The slots stay as
evenly apart as the marks allow. Every prime falls before the day starts, so your first requests
don't disturb the schedule.

For a day starting at 08:00:

| Accounts | Primed | Resets |
|---|---|---|
| 4 | 03:30, 04:50, 06:00, 07:20 | 08:30, 09:50, 11:00, 12:20, 13:30, 14:50, 16:00, 17:20, 18:30, 19:50, 21:00, 22:20 |
| 3 | 03:50, 05:30, 07:10 | 08:50, 10:30, 12:10, 13:50, 15:30, 17:10, 18:50, 20:30, 22:10 |
| 2 | 04:10, 06:40 | 09:10, 11:40, 14:10, 16:40, 19:10, 21:40 |

Each account then meets four windows in an 08:00–23:00 day. The cost is short windows at the day's
edges. With three accounts, the first has 50 minutes of its first window left at 08:00, and the
third's last window starts at 22:10.

### Primes

A prime is a probe: one request per model family, Haiku and Fable, each capped at one output token.
The Fable request falls back to the previous Fable when the newest doesn't answer. A prime goes to an account whose
5-hour window isn't running.

- **A window already running:** an account whose window is already running, as after a late night,
  gets no prime. Its slot shifts for the day.
- **A prime that fails:** a prime that reads nothing, or doesn't start the window, is logged as a
  warning. It's sent again five minutes later.
- **An account that can't start a window:** an account that can take no request anyway, or whose
  token is refused, isn't primed while it stays that way. A prime couldn't start its window.
- **Through the day:** whenever an account's window resets, in use or not, the router primes it 5
  seconds later. So the account's windows run back to back. The 5 seconds make sure the prime lands
  after the reset, though the Mac's clock runs a little ahead of the API's.
- **After the day ends:** priming stops, and the windows lapse overnight. The exception is the
  windows that the router's rounds start, where they reset before the account's slot.
- **A prime missed:** a prime missed while the Mac slept, or while the router was away, goes out as
  soon as it can, unless the day has ended.

### No accidental windows

A probe is a request, so probing an idle account starts its window. The router never probes an
account whose 5-hour window has lapsed, except in these cases:

- To prime it.
- On its rounds, where the window the probe starts resets before the account's slot.
- While the account can take no request anyway, such as while a limit holds back its every request,
  or its week reads spent. Starting its window costs nothing then, and a probe is how the router
  sees a limit you've reset by hand.

An account that nothing has been read of has no window known to have lapsed. The router probes such
an account as it starts. If that probe fails, or the account is given its first token while the
router runs, the router probes the account on its rounds, or sooner when a decision or a dashboard
needs it. This can happen at any hour, so it may start the account's window off the schedule, once.

Some commands probe every account, as you asked them to:

- `usage --probe` and `dashboard --probe`.
- `usage`, `status` or `dashboard` without the router.

`accounts add`, `accounts token` and `setup` probe a token they're given, to check it.

### The router's rounds

Every answer the router relays carries its account's whole usage, from wherever the account was
used: your phone, claude.ai, or Claude Code run directly. So an account in use through the router is
always current.

The router reads the other accounts itself, around the clock, whether a dashboard is open or not.
On its rounds, it reads each account that nothing has read in the last 5 minutes, by an answer, a
probe or a prime. Two kinds of account are handled differently:

| The account | What the round does |
|---|---|
| Its 5-hour window has lapsed, and it can take a request | Probes it, starting its window, only where its slot is more than 5 hours off, so that window resets before its prime is due. Without priming, probes it. |
| It can take no request anyway | Probes it at any hour, so a reset you make by hand is seen within 5 minutes. |

The router also probes each account once in the 5 minutes before each of its weeks resets, whatever
the schedule. It skips the probe if something read that week in those 5 minutes. So a week's last
use is read before it goes. Where this probe starts a lapsed window, the account's prime is skipped,
as for a window already running, and its schedule settles back by itself.

When these needs pull against each other, routing comes first, then the readings' accuracy, then
priming.

A probe that reads nothing is tried again 2 minutes later. The wait doubles each time, up to the
half hour. This goes on until the probe reads the account, or the account goes out on another
token. A probe due while your Mac slept goes out within a minute of its waking.

How each account's probes went outlasts a restart, kept in `state.json` beside its readings: when
one last ended, why the last read nothing, and how many in a row did. So a restarted router waits
out the backoff as though it had run on, and an account's error still reads as its last probe said.
An account given another token while the router was stopped starts afresh.

### Seeing the schedule

- `status` shows the schedule.
- `status` and the dashboard show the next reset among the 5-hour windows. From the router, they
  also show the next prime.
- A 5-hour window that hasn't started says when its account is next primed.

## What doesn't go through the router

Claude Code's own token is the primary's, so what isn't routed lands on the primary:

- **Fast mode:** ignores `ANTHROPIC_BASE_URL`, and isn't supported.
- **WebFetch's site checks:** ignore `ANTHROPIC_BASE_URL` too. They're small.
- **Claude Code's own `/usage` and `/status`:** report the primary, not the accounts the
  conversation went to. `switchboard usage` and `switchboard status --session` report those.
- **Account-bound features:** features such as remote sessions and file uploads pass through
  untouched, on the primary.
- **Programs whose `PATH` lacks the link's directory:** programs such as launchd jobs and some GUI
  apps find the real `claude`, and aren't routed.
- **Extra usage:** an account with extra usage turned on may serve, and bill, the request that
  crosses its limit, rather than refuse it. The router moves sessions off once the headers show the
  window spent.
- **A direct launch:** a launch without the router, which `claude` makes only when told to, with
  `when_router_down = "direct"` or `SWITCHBOARD_WHEN_ROUTER_DOWN=direct`, can spend the reserve of
  the account it goes out on.
- **Claude Code with an API key:** with `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` set, Claude
  Code may use the key rather than an account's token. So `claude` starts without switchboard, and
  its requests go out on the key. See [`claude`](commands.md#claude).

The full design, including the cache facts it rests on, is in [the design document](design.md).

## Claude and switchboard

`setup` installs a Claude Code skill that tells Claude what switchboard does under `claude`. So no
session needs it explained. See [`setup`](commands.md#setup).

- **Claude reads the limits itself:** `switchboard status` gives Claude every account's windows. For
  each window, it says how much is used, how fast it's going, when it runs out at that pace, and
  when it resets. So Claude can plan its work around them. It's worth it with a single account.

  For example, ask Claude to use what's left of a week before the week resets at 08:00, without
  reaching 100%. Claude paces its work through the night by the rate and run-out that switchboard
  reports.
- **Claude looks back:** `sessions`, `requests`, `history` and `events` give Claude your sessions,
  their requests, your days and what the router did, with what each was worth.
- **Claude steers when you ask:** it pins new sessions to an account with
  [`pin`](commands.md#pin). It moves running sessions there with `--move`, or pins one session alone
  with `--session`. Inside a session, `$CLAUDE_CODE_SESSION_ID` holds the session's own id, and
  `switchboard status --session` names the account the session is on.
- **JSON without asking:** when an agent runs `status`, `usage`, `sessions`, `requests`, `history`
  or `events`, they print JSON with no flag, as they do anywhere their output isn't a terminal. What
  each command prints is built by the same code that its view on the dashboard draws from. So the
  two never disagree, and nothing a view shows is out of Claude's reach.
