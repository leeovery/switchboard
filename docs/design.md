# Switchboard — design

**Status:** milestones 0 to 3 are built, and this document describes the design as built. Milestones
0 to 2 built the usage dashboard (one-off, and in watch mode) and `status`, which read the router
while it runs and probe when it doesn't; logging; the router, with its scheduler, pins, state, limit
and refusal handling, health and desktop notifications; and launching (`run` and the service).
Milestone 3 added token files and the `accounts` commands, `setup`, every `claude` going through
switchboard in place of `init zsh`, the primary account and its reserve, priming the 5-hour windows,
and the router looking after itself; Milestones lists it in full, with what its final review
changed. Next is milestone 4, the author's switch-over, which happens outside this repo. The release
follows milestone 3.

## What it is

A local proxy that spreads Claude Code sessions across several Claude subscriptions. It uses each
account's quota right up to its limit, moves a session to another account when its account runs
out, and otherwise keeps every session on one account so its prompt cache stays warm. One account,
the primary, keeps back a share of its quota for the Claude apps, and each account's 5-hour window
is started on a schedule that spreads the resets through the day. It also ships a terminal
dashboard showing every account's usage.

It's built for its author's setup, and stays general only where that costs nothing. It runs on
macOS only.

Scope: Claude Code, authenticated with long-lived setup tokens (`claude setup-token`). Browser
logins and other agents are deferred. Claude-specific knowledge sits behind a provider interface,
so another agent can be added later without reshaping the rest; the few pieces that don't are
listed under Architecture.

## How it works

```
Claude Code ──ANTHROPIC_BASE_URL──▶ switchboard ──▶ api.anthropic.com
                                      │
                                      └─ swaps the Authorization header for the chosen account
```

- Every `claude` goes through switchboard. A link named `claude`, in switchboard's own directory
  on `PATH`, ahead of the real one, runs switchboard, which starts the real Claude Code connected
  to the router: from a shell, a tmux pane, a script or a tool that runs `claude -p` alike. See
  Launching.
- Claude Code sends every request to switchboard. Switchboard replaces the `Authorization` header
  with the chosen account's token and forwards the request. Nothing else changes but the pin
  header switchboard's own launcher adds, which it strips, so the request is still genuinely
  Claude Code's. Switchboard never edits a request's body, which is also what keeps Claude's
  thinking valid across turns (see Cache and thinking facts).
- Claude Code's own token is the primary account's, so what it sends that isn't the conversation,
  such as publishing an artifact, and what it sends around the router, goes out on the primary,
  whichever account the conversation is on. See The primary account.
- Every response, success or 429, carries `anthropic-ratelimit-unified-*` headers: utilization
  and reset time for each window (`5h`, `7d`, and per-model weeklies such as `7d_oi`). Only a 429
  that refuses the request itself carries none (see Choosing an account, step 6). Switchboard
  reads them off real traffic, so it knows each account's usage without spending requests.
  Windows are parsed generically, not hard-coded. A window's reset says which of two readings
  taken apart is current: a later reset is a new window; with the same reset the higher
  utilization stands, as use only rises within a window, so a slow response can't pull it back,
  nor lift a rejection either reading holds; an earlier reset is ignored.
- The router probes every account it has no reading for as it starts; its readings outlast a
  restart. After that, an account with no recent traffic is probed only when a decision needs fresh
  numbers, a dashboard asks for them, or it's due a prime, and never once its 5-hour window has
  lapsed, as the probe would start the window off the schedule (see Priming), but while a limit
  holds back its every request, when starting the window costs nothing. An account never read, as
  one whose probe failed as the router started, or one first given a token while the router runs,
  has no window known to have lapsed: it's probed whenever a decision or a dashboard needs it, at
  any hour, which may start its window off the schedule, once. A probe is one request per model
  family, each capped at one output token: Haiku, for the windows every model shares, and Fable, for
  its own week, falling back to the previous Fable when the newest reads nothing. Where both report
  a window, the higher reading stands, as the two are taken together. A probe claims the version of
  the Claude Code installed here, as `claude --version` gives it, asked at most once an hour and
  given 5 seconds, output and all, so a CLI that hangs, or leaves a program running that holds its
  output open, holds the probes up no longer. A probe follows no redirect, which would carry its
  token along.

## Cache and thinking facts the design rests on

- Caches are isolated per organization, and each subscription is its own organization. The first
  request after moving a session rewrites its whole context into the new account's cache.
- Claude Code uses a 1-hour cache TTL on a subscription within its limits, 5 minutes in overage.
- A 1-hour cache write costs 2× base input; a cache read on Opus 5.5 costs 0.05×. The first turn
  after a move therefore costs roughly 40× a warm turn. How the subscription limits weigh cache
  tokens is undocumented.
- Caches don't carry across models.
- Since 28 September 2026, the thinking blocks Claude Sonnet 5.5 produces work only in the account
  that produced them, or one linked to it. Linking is automatic only between accounts under one
  Claude Platform parent organization, or one Google Cloud organization, so separate
  subscriptions aren't linked. A request from another account has them dropped before the model
  sees them, silently, and succeeds: the model carries on without that reasoning, and the next
  response may be slower and use more tokens. Claude Sonnet 5.5 keeps every earlier turn's
  thinking in context, so a move loses all of it. Claude Opus 5.5's and Claude Fable 5.1's
  thinking isn't bound to an account.
- On Claude Fable 5.1, Claude Opus 5.5 and Claude Sonnet 5.5, the API checks that nothing before
  a thinking block, the system prompt, the tools and every earlier message, has changed since the
  block was produced; for accounts created on or after 31 August 2026, a change is a 400 error.
  The token switchboard swaps and the header it strips aren't part of that, and it edits nothing
  else, so its requests pass. Anything that wrote into the conversation, such as a notice of a
  move, would fail it.

The spike measured cache isolation and the separate caches of models directly. Resuming a session
on the same account read 35k tokens from cache; resuming it on a second account read nothing and
wrote 35k; a second resume there read 35k again; switching model on that account read nothing and
wrote 44k. The thinking facts are Anthropic's:
[Preserved thinking](https://platform.claude.com/docs/en/build-with-claude/preserved-thinking)
and
[What's new in Claude Sonnet 5.5](https://platform.claude.com/docs/en/models/sonnet-5-5/whats-new-sonnet-5-5).

Hence: move a session only when its cache is already cold or its account can't serve it, and one
on a model whose thinking is bound to its account only when its account can't serve it.

## Choosing an account

1. **Candidates:** accounts where every window that applies to the request's model has room: the
   5-hour window, the shared weekly window, and that model's own weekly window if it has one. A
   window's room ends at the account's reserve (see The primary account): with a reserve of 0.1,
   a window reading 90% has none. That holds the router's own choices alone: a pin runs its
   accounts to their limits (see Pinning).
2. **Score:** perishability = the room left in the shared weekly window ÷ time until it resets,
   the room ending at the reserve: (1 − reserve − utilization) ÷ hours to reset. Hours to reset
   count as 1 at the least, a week whose reset has passed counts as its full room, to the reserve,
   over the week's length, and an account whose weekly reset isn't known has no score, so it's
   never the best candidate, though a global pin naming it still sends requests there while none
   of the accounts it names has a score. The highest score wins, so quota that resets tomorrow is
   used before quota that resets next week, and an account with little left scores low unless its
   week resets soon. Among candidates scoring at least 0.8 of the highest, the one whose 5-hour
   window resets soonest wins: whatever is left in a window at its reset is lost, and with the
   windows staggered (see Priming), the accounts' resets are spread through the day. One whose
   5-hour window has lapsed, its reset passed with nothing read since, or whose reset isn't known,
   comes after those whose reset is to come. Between equal resets, or two of those, the higher
   score wins, then the account that has used less of its 5-hour window, then the first
   configured.
3. **New session:** the best candidate is assigned, keyed on the session id Claude Code sends
   (`x-claude-code-session-id`) and the model, and remembered once the request is answered with
   success. Caches are per model anyway, so a session's Haiku calls can sit on a different account
   from its Opus calls at no cache cost. The id survives `--resume`, so a resumed session finds
   its account again. A new session's request that ends otherwise, in a 429, a refusal, another
   error or its client gone, leaves nothing remembered, unless another request of the session has
   been routed since, whose account stands: the quota check `--resume` sends as it starts goes
   under an id it never uses again, and refused, as on Claude Opus 5.5 today (step 6), would
   otherwise be kept, and listed among the sessions, for a week. A session already remembered
   keeps its account whatever its requests end in.
4. **Sticky:** the session stays on that account. It is only re-scored when:
   - it has been idle for more than an hour, the cache TTL, by the wall clock, which runs on while
     the Mac sleeps, so its cache is cold and a move costs nothing. Re-scoring prefers its own
     account, which another must beat by 20%, so near-equal accounts don't trade places; once one
     does, the choice is made as for a new session. A session on a model whose thinking is bound
     to its account (Claude Sonnet 5.5 today; `internal/claude` keeps the list) isn't re-scored
     for idling, as a move would lose its reasoning; or
   - its account can't serve the request. An account nothing has been read of counts as able, so
     neither a session nor a pin moves on no evidence.
5. **Limit hit:** a 429 whose overall status or any window's status reads `rejected` means real
   exhaustion. Switchboard replays the buffered request on the next candidate, among the accounts
   the request hasn't been tried on, before any response reaches Claude Code, and the session
   moves there and stays. Claude Code sees a normal, slower response. The account then has no
   room, whatever its windows read, for the requests the windows the 429 rejects count, whether
   or not it gives their utilization (every request when it names none), until the reset the 429
   gives: the overall reset, else the latest of the rejected windows', else 5 minutes on. A later
   reading showing those windows with room lifts it sooner, as a probe's does once the limit is
   reset by hand (see Priming). A limit reached again while it holds is the same limit, and holds
   as the latest 429 says; a probe that reads it again changes nothing.
6. **Throttling:** a burst 429 without exhaustion gets a pause, as long as its `retry-after` asks
   (2 seconds when it doesn't say, 10 at most), and a retry on the same account, twice at most;
   then the 429 is passed through. It never triggers a move, because moving would throw the cache
   away for nothing. A 429 without the usage headers, neither the overall status nor any window's,
   isn't throttling: it says nothing of the account's quota, but refuses the request itself, as
   the API refuses the quota check Claude Code sends as it starts, on Claude Opus 5.5 today. Sent
   again, the request would fare no better, so the 429 is passed through at once, as it came, with
   nothing held against the account, for Claude Code to retry if it will.
7. **No forced return:** after the original account resets, the session isn't moved back; that
   would cost a cache rebuild for nothing. The idle rule brings it back when a move is free.
8. **Pool exhausted:** when no account has room, switchboard first re-probes those whose readings
   say they have none, each at most once a minute, but for one whose 5-hour window has lapsed (see
   Priming), waiting 5 seconds at most, as a reset may have passed with no traffic to show it;
   then it decides again. Failing that, a request replayed after a limit gets the last 429, passed
   through; any other falls back as step 5 of the order below says.

Each request's account is decided in this order:

1. The session's own pin, while its account can serve it: the one `pin --session` gave it while it
   ran, else the one `run --account` gave it as it started; else the pin yields to the rest. A
   session that yielded stays where it went while its cache is warm, and goes back once it's
   cold, unless its model's thinking is bound to the account it went to. A pin it's given while
   it runs moves it on its next request, back to the account it yielded included.
2. A global pin set with `--move`, for a session assigned before it to an account the pin doesn't
   name: once each, to the best of the accounts it names, as step 4 chooses among them.
3. The session's account, while its cache is warm, or for a model whose thinking is bound to it,
   and the account can serve it.
4. Afresh: the best candidate among the global pin's accounts, their reserves spent, while one can
   serve the request, else the best candidate among every account, as though nothing were pinned.
   When none of the pin's accounts that can serve the request has a score, the first of them does.
5. With no candidate, the session's account, else the client's, else any other, for the upstream to
   say why, passing over those that refused the request lately, and those held back only by their
   reserve, which would serve it and spend the reserve. When that leaves none and an account is held
   back by its reserve alone, switchboard answers 429 itself, shaped as the API shapes its errors,
   with the usage headers Claude Code reads a limit from: `rejected`, and, when it's known, when the
   first of them is let go, each at the latest reset among the windows its reserve holds. When
   every account has refused the request lately, it goes out on the client's all the same, and,
   refused again, ends in the 502 of Requests that need special handling.

A request without a session id is never remembered: it goes to the launch pin it carries while that
account can serve it, and is otherwise decided afresh every time. Before deciding afresh, and never
for a sticky request, switchboard probes every account it hasn't read in 15 minutes, all at once,
but for one whose 5-hour window has lapsed and that no limit holds back (see Priming), and waits
for them 8 seconds at most. Choices made together share a probe, and an account whose probe ended,
read or not, waits a minute for the next.

The routed line in the log gives the reason for each request's account, one of:

| Reason | Why |
|---|---|
| `pinned` | The session's own pin (step 1) |
| `pin yields: <id> has no room` | The session's pin to `<id>` yielded, as `<id>` has no room for the request: its windows leave none, a limit or a refusal from before this request holds it back, or it has no usable token |
| `pin yields: <id> hit its limit`, `pin yields: <id> was refused` | The same, as `<id>` answered this request with its limit, or refused it |
| `moved by pin` | A global pin with `--move` (step 2) |
| `sticky` | The session's account, its cache warm (step 3), a session that yielded its pin included (step 1) |
| `bound` | The session's account, idle past its cache's hour, kept as its model's thinking is bound to it (step 3), a session that yielded its pin included (step 1) |
| `pinned (global)` | One of the global pin's accounts, chosen afresh (step 4) |
| `new` | A new session's first account |
| `unsessioned` | A request without a session id |
| `rescored after <idle> idle` | A session idle past its cache's hour, chosen afresh, its model's thinking not bound to its account |
| `moved: <id> has no room` | The session's account, `<id>`, can't serve the request |
| `moved: <id> is at its reserve` | The same, as `<id>` has reached its reserve |
| `moved: <id> hit its limit`, `moved: <id> was refused` | The same, as `<id>` answered this request with its limit, or refused it, and it was replayed |
| `no account has room` | None can take it (step 5): it goes where the upstream will say why, or switchboard answers 429 itself |
| `client` | The scheduler named an account nothing can go out on, so the request kept the client's own token: a bug, which the log reports as an error |

## Pinning

| Pin | Scope | Running sessions |
|---|---|---|
| `run --account <id>` | That session's conversation; Claude Code's own token stays the primary's | Unaffected |
| `pin <id>...` | Every new session, and any other whose account is chosen afresh: to the best of the accounts given | Stay where they are while their caches are warm and their accounts have room |
| `pin <id>... --force` | As `pin <id>...`, every session's own pin cleared | Stay where they are while their caches are warm and their accounts have room |
| `pin <id>... --move` | Every new session and every running one but those with their own pin | Those on an account not given move to the best of those given on their next request (one cache rebuild each, and a session on a model whose thinking is bound to its account loses its reasoning) |
| `pin <id>... --move --force` | Every session, their own pins included, which it clears | Those on an account not given move on their next request |
| `pin auto` | Back to routing (`auto` in any case) | Stay where they are while their caches are warm and their accounts have room |
| `pin auto --force` | Back to routing, every session's own pin cleared too | As `pin auto` |
| `pin <id> --session <session>` | That one session, to one account, its own pin from now on, replacing any it had | Moves on its next request |
| `pin auto --session <session>` | That one session's own pin cleared | Routed like any other from its next request |

Where the table has a running session stay while its cache is warm, one on a model whose thinking
is bound to its account stays while the account has room, however long it idles: see Choosing an
account.

The global pin names one account or several, replacing any pin before it, and the router keeps
them in the config's order, each once. A request it decides afresh goes to the best of them, by
perishability and the 5-hour tiebreak as Choosing an account scores them, each one's reserve
spent. When none of them can serve the request, the pin yields: the router chooses among every
account as though nothing were pinned, the reserves of the accounts it doesn't name held as ever.
So a pin sets an order to spend the accounts in: those it names first, the best of them first,
then the rest. `--move` moves a running session on an account the pin doesn't name to the best of
those it does; one on an account it names stays.

A per-session pin beats a global pin. The pin `run --account` sets reaches the proxy as a request
header the launcher sets through `ANTHROPIC_CUSTOM_HEADERS`. A pin set with `--session` outranks
it: the router remembers the new pin in `state.json`, with the session's assignments, and passes
over the launch pin's header from then on, as it does once `pin auto --session` has cleared the
session's pin. It holds as long as the router remembers the session, so a session resumed with
`--resume` keeps it. A session is named by its id, or as much of it as is unique among the
sessions routed in the last hour: `status` lists them with their ids, Claude Code's `/status`
shows a session's own, and inside a session, `$CLAUDE_CODE_SESSION_ID` holds it; one idle for
longer is named by its whole id. `--session` pins one session alone, to one account, so it takes
neither `--move` nor `--force`, nor more than one account.

`--force` clears every session's own pin, launch pins included, which the router passes over from
then on, so `pin <id>... --move --force` puts everything on the accounts given, and `pin auto
--force` hands everything back to the router. A session first seen afterwards has the pin it's
launched with, as usual.

A pin needs accounts requests can go out on. `pin` naming one that isn't configured fails, naming
the accounts that can be pinned, or, when no account has a usable token, saying that none has one
to pin; naming one without a usable token, it fails, saying why it has none. Either way, it pins
nothing. `run --account` fails for either too, saying what `pin` does of one that isn't configured
(see Launching).

Every pin yields at a limit: a pinned session that hits one moves by the normal rules rather than
failing. A pin spends the reserves of the accounts it names: the reserve holds back the router's
own choices, and a pin is the user's. So when every other account is out and the primary is at its
reserve, `pin <primary> --move` carries the running sessions on there, in place, and `pin auto`
hands them back to the router, reserve and all.

Switchboard defines no per-account launchers: the user's own aliases for
`switchboard run --account <id> --` serve.

## Requests that need special handling

Learned from TeamClaude (MIT, Node) and taken as ideas, not code:

- **Only `/v1/messages` and its `count_tokens` are swapped.** Everything else passes through
  untouched, on Claude Code's own token, the primary's. That includes the identity-bound paths
  TeamClaude found (`/v1/code/…`, file uploads, the session-ingress WebSocket), and message
  batches, which must keep the client's own token.
- **`metadata.user_id` is left alone.** It carries a device id, the session id and an account UUID,
  but the UUID is Claude Code's cached account from its last browser login, not the token's
  account, so a mismatch is already normal without switchboard. Setup tokens can't read the
  profile endpoint (403), so the right UUID isn't available to rewrite it with anyway. The spike's
  mismatched requests were all accepted.
- **401s and 403s:** an upstream refusal of a routed request is never relayed, because Claude Code
  drops its login on a 403. A 401 refuses the account's token: switchboard reads the account's token
  file again, and when it holds a different token, replays the request with it, as after a rotation;
  otherwise the account has no room for any request for 10 minutes. A file holding no usable token
  then, missing or empty as for a moment while it's rewritten, is no news: the account keeps its
  token, held back those 10 minutes as for the token refused. An account that goes out on another
  token, as this re-read or the token files' watch finds it (see The router looking after itself),
  is no longer held back by a 401 refusal of the one before. A 403 refuses the request alone, as for
  a model or beta the plan lacks: the account has no room for requests of that model's family for 10
  minutes, whatever token it goes out on. Either way the request is replayed on another account, as
  at a limit. With none left, switchboard answers with the 429 of the first account whose limit the
  request reached, as it came, when one did: that's why there's no account left. Otherwise it
  returns 502, shaped as the API shapes its errors and marked `X-Should-Retry: false`, as the same
  token would only be refused again.
- **Replay:** request bodies, up to 64 MiB, are buffered so they can be replayed. A routed
  request whose body is larger is answered 413 (`request_too_large`), and one whose body can't
  be read 400, neither going upstream nor counting towards the router's health. Replay only
  happens before response headers have been sent; a failure mid-stream is passed through and
  Claude Code retries. Nor is a request that couldn't reach the upstream at all replayed
  elsewhere: that isn't the account's fault. Claude Code gets a 502 and retries.
- **Uploaded files (pending):** Claude Code uploads files on its own token, the primary's. Should a
  conversation request turn out to refer to one by id, which the artifact check will show (see
  Checks owed), such a request is to go to the primary, the only account that can read the file.
  Nothing does so yet: the rule waits on that check.
- **Storm control:** decided against, as the pause and retry on a throttled account (step 6
  above) already absorbs the burst limit many sessions moving onto one account at once can trip,
  where pacing them would slow every request.
- **Bypass traffic:** some requests (fast mode, WebFetch) ignore `ANTHROPIC_BASE_URL`, so the Claude
  Code process still needs a real token in its environment. That traffic goes out on the primary:
  see What doesn't go through the router.

## The primary account

One account is the primary: the one the browser and the Claude apps are signed into.
`primary = true` marks it; without it, the first account is the primary.

- **Claude Code's own token** is the primary's. `run` gives it to every routed session, whichever
  account the conversation goes to, a session pinned to another account included: the pin moves
  the conversation alone. So what Claude Code sends that isn't the conversation, such as
  publishing an artifact or uploading a file, goes out on the primary, and every session's
  artifacts open in a browser signed into it, whichever accounts its conversation went to. A
  session resumed later gets the same token, so its earlier artifacts stay within reach. While the
  primary's token isn't usable, a routed session gets `--account`'s, else the first account's with a
  usable token. Not routed, `run` gives Claude Code `--account`'s token, else the primary's, else
  the first account's with a usable token: see Launching.
- **The reserve** is the share of every window, the 5-hour window, the shared weekly window and
  each model's own weekly, that the router leaves unused on an account: 0.1 on the primary unless
  set, 0 on the others. Once a window that applies to a request reads at or above 1 less the
  reserve, the router's own choices pass the account over: new sessions skip it, and a session on
  it moves as at a limit. Scoring counts only the room before the reserve. The router never sends
  a request to an account held back only by its reserve, even when no account has room, as that
  would spend it. A pin does spend it, running its account to its limit: the pin is the user's
  choice, where the reserve holds back the router's (see Pinning). So the primary keeps a share
  of every window for the Claude apps, where use can take it past the reserve, as intended, and
  the other accounts are used right up to their limits.
- Readings come off responses, so one large turn can take an account a point or two past its
  reserve before the router sees it. A launch that goes direct, without the router, can spend the
  reserve.
- `accounts`, `status` and the dashboard mark the primary. The dashboard marks where each reserve
  starts on its bars, and `status` and the dashboard say when a reserve holds its account back, or,
  on an account the global pin names, that the pin is spending it; a session's own pin spending it
  reads as the reserve holding the account back.

## Priming

Anthropic describes the 5-hour window as starting at an account's first message after its last
window ended, and resetting five hours later; resets aren't rounded to the hour (two accounts'
resets were ten minutes apart in the tests). Left alone, an account's first window starts with the
day's first request on it, so an 08:00–23:00 day meets three of its windows. Started earlier, a
fourth fits, the first and last partly outside the day. Started at staggered times, the accounts
come back one at a time rather than together: once all are spent, the wait for the next is at most
5 hours ÷ the number of accounts, rather than until the one reset they share.

- **The day:** `[prime] day = "08:00-23:00"`, in local time, turns priming on. An end before the
  start means past midnight.
- **The schedule:** with N accounts that have usable tokens, resets fall every 5 hours ÷ N; the
  first falls half a step after the day starts; each account, in config order, is primed five
  hours before its first reset, to the minute. Every prime falls before the day starts, so the
  day's first requests don't disturb the schedule, and may fall the evening before. For a day
  starting at 08:00:

  | Accounts | Primed | Resets |
  |---|---|---|
  | 3 | 03:50, 05:30, 07:10 | 08:50, 10:30, 12:10, 13:50, 15:30, 17:10, 18:50, 20:30, 22:10 |
  | 2 | 04:15, 06:45 | 09:15, 11:45, 14:15, 16:45, 19:15, 21:45 |

  Each account still meets four windows in an 08:00–23:00 day. The cost is short windows at the
  day's edges: in the three-account schedule, the first account has 50 minutes of its first
  window left at 08:00, and the third's last window starts at 22:10.
- **A prime** is a probe, under the probe's rules (a request per model family, each capped at one
  output token), to an account whose 5-hour window isn't running: it has lapsed, or nothing of the
  account has been read. One read without the 5-hour window, or without its reset, may be running,
  and isn't primed. An account whose window is already running, as after a late night, gets none,
  and its slot shifts for that day. The log notes each prime at `info`, with the reset it read. A
  prime fails when it reads nothing (`prime failed`), or when the 5-hour window still reads as
  lapsed once it's done (`prime didn't start the window`): either is noted at `warn`, and the prime
  is sent again five minutes on.
- **Through the day,** when an idle account's window resets, the router primes it at once, so its
  windows stay back to back. After the day ends, it stops, so the windows lapse overnight and the
  next morning's primes start them afresh. An account's day of priming runs from its slot until
  the day ends.
- A prime missed while the Mac slept, or the router was away, goes out when the router next can,
  unless the day has ended: the router looks at least once a minute, as a timer's clock stops
  while the Mac sleeps.
- Every time of the schedule is on the local clock, when an account is next primed included: a day
  the clocks change on keeps the slots and the day's end at their times of day.
- The router works the schedule out as it starts, and again whenever an account gains a usable token
  or loses it; a change to the accounts or the day is a change to the config, which restarts the
  router (see The router looking after itself). `status` shows the schedule. `status` and the
  dashboard show the next reset among the 5-hour windows and, from the router, the next prime, and a
  lapsed window says when its account is next primed (see Dashboard).
- Early starts, late nights and use in the Claude apps can start a window off the schedule, which
  shifts that account's slot for the day.
- The first primes confirm the window's mechanics: the 5-hour reset a prime reads should be five
  hours on (see Checks owed).

**No accidental windows.** A probe is a request, so probing an idle account starts its 5-hour
window. The router never probes an account whose 5-hour window has lapsed, its last reading's reset
passed with nothing read since, except to prime it: that window reads empty, and the account's
weekly readings stand. This covers the probes as the router starts, before it decides afresh, when
no account has room, and for `POST /refresh`. The one exception is an account a limit holds back
from every request, as when its week is spent: it can take no request anyway, so a probe that
starts its window costs nothing, and a probe is how a limit lifted before its reset, as by a reset
made by hand on claude.ai, is seen, the reading showing its windows with room lifting the limit.
A limit reached in one model's week alone is no exception, as the account takes other models'
requests, nor is a refused token or model. Readings persist in `state.json`, with the model
families each window has been seen to count, so a restart needs no probe. An account never read has
no window known to have lapsed: it's probed as the router starts, and, should that probe fail, or
the account first gain a token while the router runs, whenever a choice made afresh or `POST
/refresh` needs it, at any hour, which may start its window off the schedule, once. Probing without
the router, and with `--probe`, is unchanged: it's asked for; so is the probe that checks a token
`accounts add`, `accounts token` or `setup` is given.

## Accounts and tokens

- Accounts are declared in the config file: an id, a label, which is the primary, and each one's
  reserve. See Config.
- Each account's token is a file of its own, holding the token alone: `<state dir>/tokens/<id>`.
  Whitespace around the token is ignored. Switchboard keeps the directory 0700, creating it or
  tightening it, and the router tightens it as it starts. A token file must be the user's, and
  neither readable nor writable by anyone else; otherwise the account counts as having no token,
  and `accounts` and `status` say why and how to fix it. Switchboard reads no token from the
  environment.
- Anything can write the files, such as a password manager's file export or a dotfiles secrets step.
  `accounts add` and `accounts token` write them for everyone else. A writer does best to write a
  temporary file beside the token file and rename it into place, as switchboard does: one that
  empties the file before it writes the token, as a shell's redirect does, leaves it empty for a
  moment, which the router can catch (see below). A token file may be a link to one kept
  elsewhere, as by a dotfiles step: switchboard reads it, and writes a token, through the link, to
  where it leads, never in its place.
- The router reads the tokens as it starts, every account's file again every 3 seconds, and an
  account's file again when a request on it gets a 401 (see Requests that need special
  handling), so a changed token file needs no restart: an account whose file holds another token
  goes out on that one, one that had no usable token gains the one its file comes to hold, and
  one whose file holds no usable token at two looks in a row, 3 seconds apart, has nothing to send
  on until it's back. A single look finding none, as a file caught while it's rewritten, keeps the
  account its token (see The router looking after itself). The router starts whether or not any
  account has a usable token, and routes as soon as one has. `run`, `usage` and `status` read the
  files as they need them; `run` looks again, 200 milliseconds on, at the primary's file and a
  pinned account's when it finds no usable token there, as it can while the file's rewritten,
  before it starts Claude Code on another's, which would be Claude Code's own token for the whole
  session. The LaunchAgent needs no token from the user's environment: it carries
  only what finds the config, the state and the skill, and the log level (see Launching).
- A token the router replaces stays its account's for 7 days: sessions started before hold it,
  and every session holds the primary's. The router keeps it as its SHA-256 hash, never the
  token, in `state.json`, and routes a request carrying it as the account's (see Proxy rules). A
  token replaced while the router was away counts too: `state.json` keeps the hash of the token
  the router held last, which it compares with the file's as it starts. It keeps the hashes of every
  configured account, with a usable token or not: of an account without one as the router starts,
  the hash of the token held last is compared with the token the account later gains, and a
  different one counts as replaced.
- The programs switchboard runs for itself, `claude --version`, `osascript` and `launchctl`, get
  none of its environment but `PATH`, `HOME`, `TMPDIR` and `LANG`. `claude --version` gets the
  `claude`'s own directory first on `PATH`, then the one its links lead to, so a script, such as
  npm's `claude`, run by `env node`, finds a `node` installed beside it under launchd's `PATH`,
  which holds the system's directories alone.
- `accounts add <id>` registers an account. It refuses an id, or a `--label`, holding anything
  shaped like a token before it asks for the token, saving nothing, and shows the id as
  `[redacted]`. When the account's token file holds no usable token, it asks for the token, hidden,
  pointing to `claude setup-token` run while signed in to that subscription, or reads it from stdin
  when stdin isn't a terminal, and writes it in place of whatever the file held. An interrupt while
  it waits puts the terminal back as it was, showing what's typed again, and the command exits 130,
  as a shell expects of an interrupt, saving nothing. It probes the token before saving it, and
  doesn't save one the API refuses (401 or 403); when the probe fails otherwise, as when it can't
  reach the API, it saves it with a warning. `--label` sets the label, and `--primary` makes the
  account the primary.
- `accounts token <id>` replaces an account's token, under the same rules. `accounts remove <id>`
  removes the account from the config, and deletes its token file: of one that's a link, the link
  alone, saying where it led, as the file there isn't switchboard's.
- The config is edited as text, keeping its comments and layout, and read back to check it. A
  config file that's a link is written through, never replaced. The router picks the change up
  itself (see The router looking after itself).

## Setup

`switchboard setup` walks through a first run, and is safe to run again: each step says what's
already done, and does only what's missing. It asks as it goes, a line at a time, so it needs a
terminal: without one, it fails, saying how each step is taken alone: by the command that takes it,
where there is one, priming by a line in the config's `[prime]` table, and the `claude` link by
making it and putting its directory on `PATH`; the skill, only setup writes. It stops at a step that
fails, when its input ends, or at an interrupt; run again, it carries on from there. Before it asks
anything, it refuses a switchboard that won't last, such as `go run`'s, as `service install` does:
the service and the `claude` link both run the one setup was run by. An answer shaped like a token,
pasted where it shows, is never taken: setup asks for tokens where they don't show.

1. **Accounts:** lists them, each with whether its token is usable, and asks for a missing token
   as `accounts token` does; Enter leaves it for later. It offers to add accounts, one at a time,
   asking each one's id, label and token as `accounts add` does, and with none configured, asks
   for the first straight away: there's nothing to set up without one. With more than one and
   none marked, it asks which is the primary, the one the browser and the Claude apps are signed
   into, and marks it.
2. **Priming:** with no day set, asks for one, `HH:MM-HH:MM`, and writes it to `[prime]`; Enter
   leaves priming off.
3. **Service:** installs the LaunchAgent when its plist isn't there, launchd hasn't loaded it, or
   the plist runs another program than the switchboard setup runs as, saying which it ran; restarts
   the router when it doesn't answer; and otherwise says it's up. It never restarts the router for
   setup's changes to the config or a token: the router takes them up itself (see The router looking
   after itself), and setup says so.
4. **The `claude` link:** setup writes into no directory but its own. It makes the `claude` link
   in switchboard's bin directory (see Files), leading to switchboard by the path it was run by,
   as the LaunchAgent names it, so an upgrade moves the link on; a link there leading anywhere
   else it replaces, as the directory is switchboard's, and anything there that isn't a link it
   leaves alone, and says so. Then it finds the real `claude` as `run` does, and checks the
   directory is on `PATH` ahead of it: the step is done when the link is right and the
   directory is ahead of the real `claude`, and no other place counts. Otherwise it says the one
   line to add to the shell's startup file, after anything else there that changes `PATH`, so
   the directory stays ahead of the real `claude`'s, written from `$HOME` when it's in the home:
   `export PATH="$HOME/.local/share/switchboard/bin:$PATH"`. It never edits the file; run
   again, setup checks it.
5. **The skill:** writes it, or brings it up to date (see The skill).
6. Shows `usage`.

## Commands

Every command takes `--config <file>`, which names the config file in place of the one found as
Files says, and `-h`, `--help`. `switchboard --version`, or `-v`, prints the version, as
`switchboard version` does. Run by the name `claude`, switchboard is `switchboard run --` with every
argument Claude Code's own, so `claude --help` is Claude Code's (see Launching).

| Command | Job |
|---|---|
| `accounts` | List the accounts: id, label, whether its token is present and usable, and why not, and which is the primary |
| `accounts add <id> [--label <label>] [--primary]` | Register an account, asking for its token or reading it from stdin: see Accounts and tokens |
| `accounts token <id>` | Replace an account's token |
| `accounts remove <id>` | Remove an account, and its token file |
| `setup` | Walk through setting up, or what's left of it: see Setup |
| `status [--session <id>] [--json] [--probe]` | Accounts, windows, sessions, pin, what holds an account back, reserves, the priming schedule, and router health, read as `usage` reads them, and from the router, the sessions it has routed in the last hour: a line each, with its id cut short, the account each of its models goes to, its own pin, and when it was last seen. When the `claude` a shell runs from `PATH` isn't switchboard, so the sessions it starts don't go through the router, the first line says so, pointing to `setup`. `--json` prints the status document. `--session` prints one line, as a statusline asks: the id of the account the router sends a session's requests to, the one its last-used model went to; or with `--json`, `/sessions/{id}`'s answer. `<id>` is the session's id, or as much of it as is unique among those sessions, as `pin --session` takes it, and it needs the router |
| `usage [--watch [interval]] [--no-notify] [--probe] [--refresh]` | The dashboard. `-w`, `--watch` keeps it on screen, reading every interval (30m unless given, 5m at the least; a duration such as `15m`, or a number of minutes). `--no-notify` has a watch post no notifications. It reads the router while it runs; `--probe` probes instead. `-r`, `--refresh` has the router first read every account it may, as the dashboard's `r` does, and waits for it, ten seconds at most; without the router, or with `--probe`, every account is probed anyway. It reads once, so it takes no `--watch` |
| `logs [router\|cli] [-n N] [-f] [--path]` | Print a log's last lines (`-n`, `--lines`: 50), or follow it (`-f`, `--follow`), or print where it is (`--path`): see Logging |
| `serve [--log-level <level>]` | Run the router in the foreground, normally started by the service. `--log-level` (debug, info, warn or error) overrides `SWITCHBOARD_LOG_LEVEL` |
| `pin <id>... [--move] [--force]`, `pin auto [--force]` | Set the global pin to the accounts given, replacing any before, or clear it: see Pinning. It needs the router |
| `pin <id> --session <session>`, `pin auto --session <session>` | Set or clear one running session's own pin, `<session>` being its id or as much of it as is unique. It needs the router |
| `run [--account <id>] [--direct] [-- <claude args>]` | Start Claude Code connected to the router, its conversation pinned to `--account`'s account if given. `--direct` skips the router and the token, so Claude Code uses its own login. See Launching |
| `service install [--log-level <level>]` | Install the LaunchAgent, which starts the router: see Launching |
| `service uninstall`, `service restart`, `service status` | Stop the router and remove the LaunchAgent; restart the router, letting it finish its requests in flight; report the plist, whether launchd has it loaded, and the router's health: see Launching |
| `version` | Print the version, as `--version` does |
| `help [command]` | List the commands, or print a command's help, as `-h` does |

A command that needs the router fails without it, saying `the router isn't running: start it
with switchboard service install (or switchboard serve)`. Any notice a command gives on stderr,
rather than failing, reads `switchboard: …`.

No command quotes a token given where an id goes: `accounts token`, `accounts remove`, `pin`, to an
account or with `--session`, `status --session` and `run --account` show an id shaped like a token
as `[redacted]`, as `accounts add` does as it refuses one.

## Dashboard

- One card per account. Any number of accounts; the layout adapts to the terminal, down to a line
  per account when the cards don't fit its width, or, in watch mode, its height.
- Bars with a pace marker (where even use across the window would put you) and a projection
  ("on pace for 92%", "runs out ~Fri 19:40"), and on an account with a reserve, a mark where the
  reserve starts.
- For an exhausted account, a live countdown until it's back. A 5-hour window that has lapsed
  shows empty, as not started, until something uses it or a prime starts it, and, from the
  router, when its account is next primed: `not started · next prime Tue 04:15`.
- Under the heading, where the usage came from, then `best next: …`, each part set apart by a dot
  wider than the one within an account's title: the router, how many sessions it has and where
  it sends new ones (`router  ·  3 sessions  ·  pinned to 2 · two  ·  best next: …`,
  `…  ·  pinned to 1 · one and 2 · two  ·  …`, or `…  ·  routing automatically  ·  …`);
  `router unhealthy — <reason>`, in red; or, dim, `probing directly (router not running)`.
  Probing as asked says nothing of the router. With no account to use next, `no account has room
  right now` stands in for `best next`, in red, or, while nothing has been read of any account,
  `nothing read yet`, dim. With priming on, a line under it gives the next reset among the
  accounts' 5-hour windows, and, from the router, the next prime, each with its account, rather
  than the daily schedule, which is `status`'s:
  `next reset: work · Work, Mon 18:10  ·  next prime: side · Side, Tue 06:45`.
- Each account the global pin names carries a `● pinned` badge beside the best's `▲ best`, and
  the primary a `◆ primary` badge, and cards are wide enough for all three, so pinning never
  reflows them. What the router holds an account back by shows at the top of its card, in red,
  while it holds: a limit it reached, `limit until Mon 21:00`, and under it a refusal,
  `refused (403, opus) until 21:40`. An account held back by its reserve says so there, in the
  warning colour: `at its reserve (90%)`, or, with the global pin naming it, `spending its reserve
  (pinned)`. The account's sessions, `2 sessions`, show at its foot.
  A line per account carries the primary's, the pin's and the best's marks, `◆`, `●` and `▲`,
  and, where there's room, how its reserve stands and its sessions.
- **Where it reads:** `usage` and `status` read the router's status document whenever the router
  answers its health check within the half second `run` gives it, healthy or not: an unhealthy
  router's trouble is for them to show, and it still posts the notifications. Otherwise they
  probe every account, and the document's `fallback` says why the router's wasn't read:
  `{"router": "not running"}`, or `{"router": "unhealthy", "reason": "…"}` when something
  answered its socket, but not as a router does, or not within the half second (`no answer
  within 500ms`). `--probe` probes regardless, saying nothing of the router.
- **Watch mode** (`usage -w [interval]`). Reading the router, it looks at the router's document
  every 5 seconds, which costs nothing upstream, and every interval has the router `POST
  /refresh` with the interval as `max_age`, so idle accounts are probed no more often than the
  watch asks: sooner, backing off from 2 minutes to the interval, while an account can't be read.
  A minute after a window on screen resets, the next look has the router refresh first with a
  `max_age` of a minute, once a reset, so an idle account's window doesn't read `resets now`
  until the next interval; but not for an account whose 5-hour window has lapsed, which the
  router doesn't probe while no limit holds it back (see Priming): that window reads empty
  instead, and the account's others as read. Probing, it reads every interval, a minute after a
  window on screen resets, and sooner after a failure, backing off from 2 minutes to the
  interval. When the router stops answering, the next look probes instead, and the footer says
  since when there's been no router. Probing, it asks after the router at each probe and once a
  minute between, and reads it again as soon as it answers, so it never goes back and forth
  faster than that.
- **Keys:** `r` refresh: the router probes the accounts it hasn't read in the last minute, but for
  those whose 5-hour window has lapsed and that no limit holds back, or, without it, every account
  is probed, as `usage --refresh` does. `q` quit. While it reads the router, `1`–`9` toggle the
  account in that place, as configured, in the global pin: one it doesn't name joins those it
  does, new sessions going to the best of them, and one it names leaves, the last to leave
  routing automatically again; `a` routes automatically again; `m` moves running sessions to the
  pinned accounts, or says nothing's pinned. A digit sets a pin that doesn't move running
  sessions, as `pin` without `--move` does. Each says in the footer what it did, or why it
  couldn't, for a few seconds, and the router's document is read again at once. The footer lists
  only the keys that work:
  `r refresh · 1–3 toggle pin · a auto · m move · q quit` reading the router, and
  `r refresh · q quit` probing.
- Desktop notifications: see Notifications.
- Text from elsewhere, such as labels and the upstream's errors, shows with its control characters
  as spaces, here and in `status` alike, so none can move the cursor or restyle what follows, a
  statusline's included.
- Built with Bubble Tea v2 and Lip Gloss v2.

## Health

The launcher routes a new session only when the router answers its health check `ok`, saying where
its proxy listens; otherwise the session connects directly. The harder case is a router that is
running but failing requests: sessions already routed through it fail until they restart. The router
tracks the requests it has routed over the last 5 minutes, and those it failed itself: a 502 for an
upstream it couldn't reach, or for a refusal with no account left to fail over to. The upstream's
own 429s and 5xx, passed through, don't count against it. It's unhealthy once it has failed 5 of
them at least, and half at least: `GET /health` then answers `ok: false` with a `reason`, the status
document's `router` object says the same, and the log notes the turn, and the turn back, at warn and
info. `status` and the dashboard show trouble loudly: they read an unhealthy router's document all
the same, `status`'s last line reading
`from the router: unhealthy, <reason>  ·  <sessions>  ·  <routing>` and the dashboard heading its
cards `router unhealthy — <reason>`, in red.

Sessions don't fall back to going direct automatically. A running Claude Code can't change where
it sends its requests mid-session; the failures that count against the router, an upstream it
can't reach and refusals with no account left, would meet a direct connection too; and a new
launch already goes direct while the router is down or unhealthy.

## Notifications

The router sees each limit, move and return as it happens, whether or not a dashboard is open, so
while it runs, the desktop notifications are its own. `[notifications]` in the config says which
it posts: see Config.

- **Limits:** a limit's notification waits 5 seconds for the sessions the limit moves off its
  account, as their next requests come, then tells of them together:
  `work · Work hit its Session limit, back at Mon 18:10 — 3 sessions moved to side · Side`. With
  none moved, it says when no other account has room. When the account is back goes unsaid where
  it would make the message longer than a banner shows. One notification a limit, however many
  requests reach it: one reached again while it holds is the same limit.
- **Room again:** an account whose quota for a request of any model ran out, under a limit, with
  a shared window spent, or at its reserve, and has come back: `work · Work has room again`. A
  refusal isn't quota, so one lifting is no news, or a revoked token would be announced every ten
  minutes; an account both out of quota and refused has room again once both are past. The router
  looks at the accounts on every event and every 15 seconds, so a limit lifting or a window
  resetting with no traffic is noticed.
- **Warnings:** a window passing the share given, once a reset: `work · Work: Week at 91%`.
- **Moves:** each move a limit's notification doesn't tell of:
  `session 18bb978f moved from work · Work to side · Side (rescored after 1h 2m idle)`.

Room again and warnings compare an account with how it last stood, so neither tells of how the
accounts stood as the router started, nor of an account's first reading. A limit's notification
always goes out: as the router stops, those still gathering go out at once, within 3 seconds.
Any other goes out only a minute or more after the last posted about its account, a limit's
included: one due sooner is dropped, and the log says so at debug. One that fails to post is logged
at warn, and dropped too; not having gone out, it starts no quiet minute. The log names accounts by
id alone. Notifications never hold a request up: the router queues what happens, 256 events at most,
dropping any past that with a warning in the log, and posts from a goroutine of its own.

The dashboard in watch mode posts its own only while it probes because the router isn't there to:
of an account with room again and a window passing the warning, as `room` and `warning` say, and
none with `--no-notify`. While the router answers, the dashboard posts none, so nothing is told
twice: probing with `--probe`, it asks after the router before it posts, and posts only when it
doesn't answer. It sees no limits or moves, which are the router's alone.

## Logging

Logs are for working out, after the fact, why a session went to an account, why a request failed,
or why the dashboard showed what it did. Each record is one line of logfmt:

```
time=2026-09-28T14:12:00.123+01:00 level=WARN msg="probe failed" component=status pid=4242 account=side duration=120ms error="HTTP 401 · Invalid bearer token"
```

- **Files:** in `<state dir>/logs/`. The router writes `router.log`; every other command writes
  `cli.log`, often several at once, hence the `pid` on every line. Each record is one unbuffered
  append, so none is lost to an `exec` or exit, and none splits another.
- **Rotation:** before a record would take a log past 10 MiB, it rolls over to `.1`, shifting
  older files up to `.5` and dropping the oldest. Processes sharing `cli.log` roll it over one
  at a time, under a lock on the directory, and a process whose file was rolled over moves on to
  the new one. The directory is 0700, the files 0600.
- **Levels:** `SWITCHBOARD_LOG_LEVEL` is `debug`, `info`, `warn` or `error`, in any case; `info`
  when unset, and `info` with a warning naming the value when it's anything else. A
  `--log-level` given to `serve`, or to `service install`, which passes it on, overrides it.
  Every process logs its `start` (role, version and command path, never the arguments, which can
  hold a prompt) and `exit` (status and duration): at `info` for the router, at `debug` for
  commands, so a statusline running `status` every few seconds doesn't flood `cli.log`. `run`
  becomes `claude`, by `exec`, before it would log its exit: its last record is the launch.
- **The router's own events:** at `info`, each prime, with the reset it read; an account held back
  by its reserve, and let go at its reset; a token file read again after a 401, and whether it held
  a different token; a token replaced while the router was away, which it finds as it starts; a
  token file found holding another token, an account gaining a usable token, and one losing it, with
  why; the tokens directory made private as the router starts, and what bringing the skill up to
  date did; a config change, and an upgrade; a restart either makes due, once, and the restart as it
  goes. At `warn`: as the router starts, each account without a usable token, and, when none has
  one, that nothing will be routed until one has; a prime that failed, or didn't start the window;
  and a config change refused, as invalid. At `debug`, a token file found holding no usable token at
  one look, which the account's token outlasts.
- **Redaction:** nothing logs a token or an account's label; accounts appear by id. As a
  backstop, the handler replaces anything shaped like a token (`sk-ant-…`) in the message or in
  any attribute's text, and the whole value of any attribute keyed `Authorization`, with
  `[redacted]`. What the standard library's `log` package is given goes through it too, to the
  log rather than stderr: under `GODEBUG=http2debug=1`, Go's HTTP/2 client prints every header it
  sends, the token's included.
- **Never in the way:** commands never log to stdout or stderr, so `--json` and the dashboard
  stay clean, and a command never fails because it couldn't log: its records go nowhere
  instead. `serve` also writes each record to its terminal, when it runs in one.
- **Reading:** `switchboard logs` prints the router's log once there is one, else the CLI's, or
  the one named. `-n` sets how many lines (50), reaching into `.1` when the log is shorter; `-f`
  follows it across rotations; `--path` prints where it is.

## The router looking after itself

Every 3 seconds, the router looks at what it was started from: the token files, its config file
and its binary.

- **Token files:** it reads each account's token file again, and takes up what it holds in place,
  with no restart, logging each change, but never a token. An account whose file holds another token
  goes out on that one, the one before still counting as the account's, as after a 401 (see Accounts
  and tokens), and a 401 refusal of the one before no longer holds it back; a 403's refusal of a
  model family stands. One that had no usable token gains the one its file comes to hold, and
  requests can go out on it. The router starts even when no account has one, warning once in its log
  that nothing will be routed until one has, and routes from the look that finds the first. One
  whose file holds no usable token at two looks in a row, 3 seconds apart, has nothing to send on
  until it's back, as though it had none as the router started: the sessions on it move, and a
  request carrying its token passes through untouched, as one carrying a token the router doesn't
  hold does (see Proxy rules). While the primary's is gone, that's every session's request. A single
  look finding none keeps the token, noted at debug: a writer that empties the file before it writes
  the token, as a shell's redirect does, leaves it so for a moment. The priming schedule is worked
  out again whenever an account gains a usable token or loses it.
- **Config changes:** it restarts itself on a change to its config file that parses and
  validates, so `accounts add`, `accounts remove` and an edit by hand all take effect without a
  command. It follows links, so a config kept in a dotfiles repo and linked counts, and a change
  is the file's identity or modification time changing. A change that doesn't parse and validate
  is logged at `warn`, and the router carries on with the config it has.
- **Upgrades:** it restarts itself when the binary it was started as, the Homebrew link its
  LaunchAgent runs, leads to a different file from the one running, or to the same file changed
  since, as after `brew upgrade`. A link that leads nowhere, as it may for a moment while an upgrade
  moves it on, isn't one.
- `serve` notes how the config file and the binary stand before it reads the config, and the router
  compares them with that, so a change or an upgrade made while the router starts calls for a
  restart too.
- Either restart waits for a moment with no requests in flight, there being no hurry, and for the
  config file to make a valid config, which the router started again needs: an upgrade while the
  config file is invalid waits for it to be put right. A connection upgraded, such as a
  WebSocket, isn't a request in flight, as it can stay open for as long as its session runs. The
  router then stops as it does at a signal, its state saved and the notifications still gathering
  sent, and exits, and launchd starts it again. Sessions keep their accounts (`state.json`),
  readings persist, and caches, being the API's, stay warm. A `claude` started in the second or
  so the router is away connects directly.
- Only the LaunchAgent's router restarts itself: launchd sets `XPC_SERVICE_NAME` to the label of
  the job it runs, which the router checks against the service's. Run by hand with `serve`, the
  router logs, once, that a restart is due instead of exiting.
- As it starts, the router brings the installed skill up to date (see The skill), and makes the
  tokens directory private when it's there.

## The skill

A Claude Code skill tells Claude what switchboard does under `claude`, so no session needs it
explained. Switchboard carries the skill in its binary, with a version, which an HTML comment in its
body gives, as Claude Code takes only the frontmatter keys it knows; the version moves on whenever
the text changes. `setup` writes it where Claude Code reads skills: `skills/switchboard/SKILL.md` in
its config directory, `$CLAUDE_CONFIG_DIR`, else `~/.claude`. `setup`, and the router as it starts,
rewrite an installed copy whose version is older than switchboard's, or can't be read, and leave any
other as it is: switchboard owns the file, and an edit to it stands only until the version moves on,
which replaces it. The router never creates one `setup` didn't.

It's short: `claude` runs through switchboard; `status --session`; `usage`, and `usage --refresh`,
or the dashboard's `r`, once a limit is reset by hand; `pin`, to one account or the best of
several; and `logs`; a move costs one slower turn, and a moved Claude Sonnet 5.5 session carries
on without its earlier reasoning; artifacts always live on the primary; and `switchboard --help`
for the rest.

## What doesn't go through the router

Claude Code's own token is the primary's, so what isn't routed lands there.

- **Fast mode**, which runs on paid extra usage, ignores `ANTHROPIC_BASE_URL`, as TeamClaude found.
  It isn't supported.
- **WebFetch's site checks** ignore it too. They're small.
- **Claude Code's own `/usage` and `/status`** report the primary, not the accounts the
  conversation went to.
- **Account-bound features**, such as remote sessions and file uploads, pass through untouched, on
  the primary.
- **Extra usage:** an account with extra usage turned on may be served past its limit, and billed,
  rather than refused. The router moves sessions off it once the headers show the window spent,
  but the request that crossed the limit is billed. The accounts tested read extra usage as off.
- **Programs whose `PATH` lacks the link's directory**, such as launchd jobs and some GUI apps,
  find the real `claude`, and aren't routed.
- **A direct launch**, without the router, can spend the primary's reserve.
- **Claude Code with an API key:** with `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` set, Claude
  Code may use the key in place of an account's token, so `claude` starts it as if switchboard
  weren't there, its requests going out on the key (see Launching).

## Architecture

### Packages

| Package | Owns |
|---|---|
| `cmd/switchboard` | `main`: builds the command tree from the real system (environment, clock, home, `claude`'s version, launchd, notifications, the terminal) and exits with its status. Run by the name `claude`, it hands every argument to `run` |
| `internal/cli` | Cobra commands. Thin: parse flags, call the packages below, print |
| `internal/config` | Locating, parsing and validating the config file, and editing it: adding and removing accounts, and setting the primary and the priming day; and locating the state directory, and switchboard's bin directory |
| `internal/tokens` | The token files: reading them, checking their ownership and mode, writing them, and keeping their directory private. `tokens/tokenstest` stands in for the token files, for tests |
| `internal/accounts` | Adding accounts, replacing their tokens and removing them, for the `accounts` commands and `setup`: the config file and the token file together, and a token the user gives, typed unseen at a terminal or piped in, checked with the API before it's saved |
| `internal/atomicfile` | Writing a file whole or not at all: beside where it goes, synced, then renamed into place; and where writing through a link leads, so a file that's a link is written where it leads, never replaced |
| `internal/quota` | The provider-neutral usage model: windows, failures, per-account snapshots, and what a response says of its account |
| `internal/claude` | The Claude provider: usage-header parsing, probes, model families, response classification (a limit reached, throttling, a refused token, a request refused alone), which paths are routed, the session header, Claude Code's environment variables, finding the installed `claude` and its version, whether the `claude` a shell runs from `PATH` is switchboard, Claude Code's local subcommands, and which models' thinking is bound to the account that produced it. `claude/claudetest` makes stand-ins of Claude Code, and of switchboard's binary, `claude` link and another build of it, for tests |
| `internal/score` | Pace, projection, eligibility against the reserve, perishability, the 5-hour tiebreak and the best-account pick. Pure functions of a snapshot and a clock |
| `internal/prime` | The priming schedule: each account's slot from the day and the accounts, and when a prime is due. Pure functions of the day, the accounts, the window a request starts, which the `score.Policy` names, the readings and a clock |
| `internal/status` | The status document, building it by probing every account, what the router says of a session, and their words: `status`'s text, and the countdowns, clocks and titles the dashboard shares |
| `internal/dashboard` | Rendering the status document as a frame (Lip Gloss): cards, or a line per account |
| `internal/dashboard/watch` | Watch mode (Bubble Tea): when to read the router or probe, its keys, easing the bars, and its desktop notifications while it probes without the router |
| `internal/router` | The proxy and its replays, the scheduler, live account state, priming, the state file, the router's health, the events it emits and the notifications it posts, the control API and its client, and looking after itself: taking up the token files as they change, and restarting for a config change or an upgrade |
| `internal/launch` | `run`'s hand-over to `claude`, the real one, as `internal/claude` finds it on the `PATH` Claude Code starts with, and how a notice reads on stderr |
| `internal/setup` | `setup`'s steps, asked a line at a time at a terminal, the `claude` link in switchboard's bin directory among them, and the line that puts that directory on `PATH` |
| `internal/skill` | The Claude Code skill: its text and version, and writing and updating the installed copy |
| `internal/service` | The LaunchAgent: its plist, and driving `launchctl` |
| `internal/notify` | Posting desktop notifications, and the wording and warning threshold the router's and the dashboard's share |
| `internal/childenv` | The environment the programs switchboard runs for itself start in: no token |
| `internal/logs` | Logging: the handler every package logs through, the log files and their rotation, redaction, and reading logs back. `logs/logstest` captures what's logged, for tests |
| `internal/redact` | Hiding secrets: a token held, and anything shaped like a Claude token, as `[redacted]`, and telling text that holds one |
| `internal/prose` | Words shared across packages: a list run together as English does, and text cut short |
| `internal/testguard` | Every package's `TestMain`: keeps tests off the real system, `~/.claude`, switchboard's bin directory and token files, and the directories on `PATH` included (see Test isolation in `CLAUDE.md`) |

Claude-specific knowledge lives in `internal/claude`. The router and `status` depend on small
interfaces they define themselves (`router.Provider`, `router.Prober`, `status.Prober`), which
`internal/claude` satisfies, and the windows that matter, the one a request starts among them,
reach them, the scoring and the watch as a `score.Policy`, which the CLI fills in from
`internal/claude`. `cmd/switchboard` and `internal/cli`, which wire the rest together,
`internal/launch`, which exists to start Claude Code, and `internal/setup`, which links `claude` to
switchboard ahead of it, use `internal/claude` directly: for the `claude` command, where it's
installed and its version, and the environment variables Claude Code reads. A few pieces stay
outside it, deliberately, each where the code that needs it can't reach `internal/claude`, or where
hiding it behind the provider would take a wider interface than it's worth:

- **The token's shape**, `sk-ant-…`, is in `internal/redact`: the logs hide it, and
  `internal/claude` logs through `internal/logs`, which therefore can't import it. The router
  reads and sets the token as an `Authorization: Bearer` header, as the setup tokens go.
- **The window statuses** `quota` names, `allowed`, `allowed_warning` and `rejected`, are the
  API's own words, which `score` judges a spent window by; and the shape of the API's window
  keys, `<n>h` or `<n>d`, as in `5h` and `7d_oi`, is what `quota` reads a window's length from,
  for pace, projection and perishability.
- **The config's defaults** name the API, `upstream` defaulting to `https://api.anthropic.com`,
  and `accounts add` points to `claude setup-token` for a token.
- **The proxy's own answers** are shaped as the Messages API shapes its errors, with its error
  types, and its 502 for a refusal with no account left is marked `X-Should-Retry: false`, which
  the API's clients, Claude Code among them, honour. That no refusal is relayed is Claude Code's
  doing too: it drops its login on a 403.
- **The cache's life**, an hour, is Claude Code's prompt-cache TTL on a subscription, which the
  scheduler keeps a session sticky for.
- **The skill** is written for Claude Code, and where Claude Code keeps skills.

### Files

- **Config:** `$SWITCHBOARD_CONFIG`, else `$XDG_CONFIG_HOME/switchboard/config.toml`, else
  `~/.config/switchboard/config.toml`. A relative `XDG_CONFIG_HOME`, `XDG_STATE_HOME` or
  `XDG_DATA_HOME` is ignored, as the XDG spec says.
- **State:** `$XDG_STATE_HOME/switchboard/`, else `~/.local/state/switchboard/`. Holds `state.json`
  (the global pin, session assignments and the pins sessions were given while they ran, and each
  account's last readings, with the model families each window has been seen to count, so a restart
  doesn't scatter sessions or need a probe, and the hashes of every configured account's tokens,
  with a usable token or not: see Accounts and tokens), `control.sock`, `tokens/` and `logs/`.
  `state.json` is versioned, the version changing only when a router couldn't read what another
  wrote: an older file, without readings, loads as having none, and a pin that names its account
  alone, as pins did before they named several, as a pin to that one. It's rewritten whole
  (written beside it, synced, and renamed over it) a second after a change and on the way out,
  and drops assignments unused for 7 days, with the pins of the sessions it forgets, and the
  hashes of tokens replaced 7 days before, at start and then hourly. At start it also drops the
  assignments and the sessions' own pins of accounts no longer configured, those accounts from
  the global pin, which goes with the last of them, and their readings and token hashes; those of
  a configured account whose token file can't be read are kept, as the file may only have been
  caught while it's rewritten, and choices pass the account over until it has a token. A corrupt
  one is set aside as `state.json.corrupt-<unix time>`, and the router starts without it.
- **Tokens:** `<state dir>/tokens/<id>`, a file per account, 0600 in a 0700 directory: see Accounts
  and tokens.
- **Logs:** `<state dir>/logs/`: `router.log`, `cli.log` and their rolled-over files (see
  Logging), and `launchd.log`, where the service's raw stdout and stderr, such as crash output,
  go.
- **Service:** `~/Library/LaunchAgents/io.github.leeovery.switchboard.plist`, the LaunchAgent's
  plist, 0644, named after its label.
- **The `claude` link:** a link named `claude` to switchboard, by the path it was run by, alone in
  switchboard's bin directory, `$XDG_DATA_HOME/switchboard/bin`, else
  `~/.local/share/switchboard/bin`, which `setup` makes, and the user puts on `PATH` ahead of the
  real `claude`'s directory.
- **The skill:** `skills/switchboard/SKILL.md` in Claude Code's config directory,
  `$CLAUDE_CONFIG_DIR`, else `~/.claude`, once `setup` has written it.

### Config

```toml
listen   = "127.0.0.1:4747"             # optional: the proxy's address
upstream = "https://api.anthropic.com"  # optional: the API's base URL; overridden in tests

[[account]]
id      = "work"       # permanent name: letters, digits, '-' and '_'; its token is tokens/work
label   = "Work"       # optional; defaults to the id
primary = true         # optional: the account the browser and the Claude apps use; else the first
reserve = 0.1          # optional: the share of every window the router leaves; 0.1 on the primary, else 0

[[account]]
id    = "side"
label = "Side"

[prime]                            # optional: start the 5-hour windows on a staggered schedule
day = "08:00-23:00"                # local time; an end before the start means past midnight

[notifications]                    # optional: which desktop notifications to post
limits  = true   # an account hits a limit, and the sessions it moved
room    = true   # an account has room again
warning = 0.9    # a window passing this share of its limit; 0 turns it off
moves   = false  # every other session move, such as after an idle hour or by pin
```

The accounts keep their file order, which is their order everywhere they're shown, and the order
priming gives them their slots in. A file that isn't TOML, or holds a value of the wrong type,
fails as it is; one that parses has every problem reported at once:

- **Unknown keys** are errors. A `token_env` from before milestone 3 is one, and the error says the
  token now lives in a file.
- **`listen`** is `host:port`, its host a loopback IP address, such as `127.0.0.1` or `::1`
  (`[::1]:4747`), never a name such as `localhost`, which a client can look up to `::1` while the
  proxy listens on `127.0.0.1`, and send its token to whatever listens there; and a port from 1 to
  65535.
- **`upstream`** is an absolute `http` or `https` URL, and `https` unless its host is a loopback
  IP address: tokens never cross a network in plaintext.
- **At least one `[[account]]`.** Each needs an `id`: it starts with a letter or digit, holds only
  letters, digits, `-` and `_`, is unique, and isn't `auto`, in any case, which `pin auto` takes
  to mean routing. The id names the account's token file, which these rules keep safe as a file
  name; so no two ids differ only in case, as `work` and `Work` do, which macOS, ignoring case in
  file names, would give one token file. Neither the `id` nor the `label` may hold anything shaped like a token, as both show
  wherever the account does: the error never quotes it, and names an account whose id holds one by
  its place, as `account #2`.
- **`primary`** is true on one account at most.
- **`reserve`** is 0, or more than 0 and less than 1.
- **`[prime]`**: `day` is two times of day, `HH:MM`, joined by `-`, and not the same time twice.
- **`[notifications]`**: each key defaults as shown. `warning` is 0, or more than 0 and less than
  1.
- **No error quotes a token:** one that quotes a value, of `listen`, `upstream` or `prime.day`, an
  unknown key's name, or the key a file that isn't TOML fails at, such as one without a value or
  given twice, shows anything in it shaped like a token as `[redacted]`, and an unknown key's value
  is never quoted.

### Proxy rules

- A request is routed only when its path is exactly `/v1/messages` or `/v1/messages/count_tokens`
  **and** its bearer token is one of the configured accounts' tokens, which Claude Code's, the
  primary's, is, or one an account had before the router took up another, for 7 days after (see
  Accounts and tokens): an account without a usable token has none that counts. Anything else
  passes through untouched: batches, whose ids belong to one account, stay on it, and a local
  process that doesn't already hold a token can't borrow one.
- A token an account had before is known by its SHA-256 hash, which a request's token is hashed
  and compared with in constant time, as the current tokens are. A request carrying one is the
  account's, and goes out on the account's current token, as every routed request does.
- `X-Switchboard-Account: <id>`, set by `run --account` through `ANTHROPIC_CUSTOM_HEADERS`, pins
  that session. It is stripped before the request goes upstream. One naming an account that isn't
  configured, or has no token, is ignored, and the log warns of it.
- The session key is `X-Claude-Code-Session-Id` plus the request's model. A request without the
  header is never remembered, and but for a launch pin it carries, is decided afresh every time,
  as a new session's is.
- Which windows apply to a request: `5h` and `7d` apply to every model. Any other window applies
  to the model families it has been seen on (responses and probes reveal this), and to every model
  until it has been seen. A family is read from the model id: haiku, sonnet, opus or fable.
- Whether a model's thinking is bound to the account that produced it is read from the model id
  too, against the list in `internal/claude`: an id starting `claude-sonnet-5-5`, Claude Sonnet
  5.5's, today.

### Control API

HTTP over `control.sock` (mode 0600, so file permissions are the authentication):

| Endpoint | Job |
|---|---|
| `GET /health` | `{ok, reason, listen, version, pid, started_at}`: the router is alive, and `ok` is its health, the judgment the status document's `router.healthy` gives, `false` while it's unhealthy, with a `reason` (see Health). `listen` is the address its proxy listens on. `run` sends sessions to a router that answers `ok` and gives `listen`; `usage` and `status` read the document of any router that answers at all |
| `GET /status` | The status document, as `status --json` prints it: see below |
| `GET /sessions/{id}` | For statuslines: `{"session": "<id>", "pin": "<id>", "assignments": [{model, family, account, pinned, reason, assigned_at, last_seen}], "account": {…}}`. `pin` is the session's own pin, left out when it has none: the one `pin --session` gave it, else the one `run --account` did, as its requests last carried it. `assignments` are the session's, a model each, the one used last first, each naming its model's family, such as `opus`, and its account by id; `account` at the top is the whole status of the account the last used went to, as the document gives it. 404 for a session never seen |
| `GET /sessions` | The sessions routed in the last hour, the one seen last first, each as `/sessions/{id}` gives it but for `account`. `status` lists them, and `pin --session` and `status --session` find a session from part of its id here |
| `POST /sessions/{id}/pin`, `DELETE /sessions/{id}/pin` | Set (`{"account": "work"}`) or clear one session's own pin, answering as `/sessions/{id}` does. 404 for a session never seen; pinning to an account nothing can go out on is a 400 |
| `POST /pin`, `DELETE /pin` | Set (`{"accounts": ["work", "side"], "move": false, "force": false}`) or clear (`?force=true` to clear every session's own pin too) the global pin, answering with the status document. `account`, naming one account, is taken as well, as a switchboard from before pins named several sends it. Pinning no account, or any account nothing can go out on, is a 400, saying why (see Pinning), and pins nothing |
| `POST /refresh` | Probe the accounts nothing has been read of for longer than `{"max_age": "30m"}`, but for those whose 5-hour window has lapsed and that no limit holds back (see Priming), sharing the probes choices make and waiting a minute after one ended, as they do; wait 10 seconds at most for them, and answer with the status document. The watch asks every interval, and a minute after a window on screen resets |

A request an endpoint refuses is answered `{"error": "<why>"}`; any other path or method gets the
standard library's plain 404 or 405. Times are given in UTC.

### The status document

What `status --json` prints and `GET /status` answers, whether the router built it or probing
did. Times are RFC 3339, in UTC, but the priming schedule's times of day, which are local;
utilizations are fractions (`0.23` is 23%), and can pass 1. Fields may be added to it, never
renamed. A field marked *router* is the router's alone, and left out of a document built by
probing.

| Field | Is |
|---|---|
| `generated_at` | When the document was built |
| `source` | `"router"`, or `"probe"` when built by probing every account |
| `fallback` | Why a probed document isn't the router's, when the router was asked first: `{router: "not running"}`, or `{router: "unhealthy", reason}`. Left out otherwise, and when probing was asked for |
| `best` | The id of the account to use next: of those with room in every window all models share, the one whose quota most needs using, judged by a week whose reset is known, and between near equals by the 5-hour window's reset, as Choosing an account says. Left out when none qualifies, as when none has room, or none has been read yet |
| `primary` | The primary account's id |
| `prime` | The priming schedule, when the config sets a day and an account has a usable token: `{day, window, slots}`, `window` the key of the window a prime starts, such as `5h`, and `slots` giving each account with a usable token its daily prime, `{account, at, next}`, `at` a local `HH:MM`, in the order they fall. `next` is *router*: when it next primes the account, as its windows stand, left out when they can't say, as for a window read without a reset. Left out otherwise |
| `pin` | *router* The global pin, `{accounts, account, since, move}`: `accounts` the ids of the accounts it names, in the config's order, and `account` the first of them, as a pin named its one account before pins named several; left out when there's none |
| `router` | *router* Its health: `{healthy, requests, failures, reason}`, over the last 5 minutes, `reason` left out while healthy |
| `sessions` | *router* How many sessions have been routed in the last hour, each counted once, however many accounts its models went to; left out at 0 |
| `accounts` | Every configured account, in the config's order, as below |

Each account:

| Field | Is |
|---|---|
| `id`, `label` | As configured |
| `primary` | `true` on the primary; left out otherwise |
| `reserve` | Its reserve; left out at 0 |
| `token_set` | Whether its token file is present and usable |
| `fetched_at` | When its usage was last read; left out when it never was |
| `windows` | Its windows as last read, shortest first: `{key, label, utilization, resets_at, status}`. `key` is the API's, such as `5h`, `7d` or `7d_oi`; `resets_at` is left out when unknown, and `status` (`allowed`, `allowed_warning` or `rejected`) when not given: a 5-hour window that has lapsed reads 0, with neither. Left out when none has been read |
| `lapsed` | The keys of its windows that have lapsed: the 5-hour window, once its reset has passed with nothing read since, which isn't running, and reads empty, until a request starts it (see Priming). Left out when none has |
| `at_reserve` | The keys of the windows at or past its reserve but short of their limit, that haven't reset since they were read; left out otherwise. The router's own choices pass the account over, for the requests those windows count, while there are any; a pin spends the reserve |
| `failures` | Windows a probe expected but couldn't read: `{label, window, error}`, `label` naming what should have read it, such as `Fable`. Left out when none |
| `error` | Why its usage couldn't be read, such as its token file missing, or readable by others, or, from the router, why its last probe read nothing; left out when there's nothing to say |
| `limit` | *router* A limit it reached, while it holds: `{windows, until}`, `windows` the keys named as reached, left out when only the overall verdict said so |
| `refused` | *router* The upstream's refusal, while it holds: `{until, status, family}`. `status` 401 is its token refused, holding back every request; 403 a request refused alone, holding back its model's `family`. With both, the token's; with several families, the latest |
| `sessions` | *router* How many sessions have been routed to it in the last hour; left out at 0 |

### Launching

- **The `claude` link:** a link named `claude` to switchboard sits in switchboard's bin directory
  (see Files), which `setup` makes, and the user puts on `PATH` ahead of the real `claude`'s
  directory, with the line `setup` says to add to the shell's startup file. `status` says when
  the `claude` a shell runs from `PATH` isn't switchboard. Run by that name, switchboard behaves
  as `switchboard run --` with every argument passed through, so `claude --help` is Claude Code's.
  Everything that runs `claude` from `PATH` is routed: shells, tmux panes, scripts, and tools
  that run `claude -p`. Programs whose `PATH` lacks the link's directory find the real `claude`,
  and aren't. The ways round it are `switchboard run --direct`, and the real `claude` by its path.
  A broken switchboard breaks every `claude` until the directory is taken off `PATH`. `claude
  doctor` may report the link as a second installation (unverified).
- **No shell integration:** switchboard defines no shell function or launcher. Per-account
  launchers are the user's own aliases for `switchboard run --account <id> --`.
- **Routed:** `run` gives the router half a second to answer `GET /health` with `ok`. When it
  does, `run` starts Claude Code with `ANTHROPIC_BASE_URL` pointing at the proxy, where the router
  says it listens, not where the config says, which may have changed since,
  `CLAUDE_CODE_OAUTH_TOKEN` set to the primary's token, whatever account the conversation goes to,
  or, while the primary's isn't usable, `--account`'s, else the first account's with a usable token,
  and with `--account`, the pin header added to any `ANTHROPIC_CUSTOM_HEADERS` already set.
- **Direct:** otherwise it connects directly, on `--account`'s token, else the primary's, else the
  first account's with a usable token, without the base URL or the pin, saying why in one line on
  stderr: `switchboard: the router isn't running — connecting directly on work · Work`. Either way,
  and with `--direct`, a pin inherited from the environment, as from a session this one is started
  within, goes: what this launch pins is the only pin. `--direct` removes the token and the base
  URL, so Claude Code uses its own login.
- **Never in the way:** switchboard never stands between the user and `claude`. When it can't
  take part at all, as when it can't read its config, can't locate its state directory, or no
  account has a usable token, `run` starts `claude` as if switchboard weren't there, environment
  and arguments untouched, an inherited pin included, saying why in one line on stderr:
  `switchboard: couldn't read the config (…) — starting claude without it`. `run` fails only
  when `claude` can't be found or can't start, or on a misused command line, such as `--account`
  naming an account that isn't configured, or has no usable token.
- **An API key:** with `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` set, Claude Code may use the
  key in place of the token `run` gives it, and its requests would go unrouted, billed to the key.
  So `run`, and with it `claude`, starts Claude Code as if switchboard weren't there, environment
  and arguments untouched, without reading the config, and says so on stderr, naming the variable:
  `switchboard: ANTHROPIC_API_KEY is set, so Claude Code uses it — starting claude without
  switchboard`. The log notes it at info, naming the variable, never the key. `--direct` and Claude
  Code's local subcommands aren't affected.
- **Claude Code's local subcommands:** `setup-token`, `update`, `upgrade`, `install`, `doctor`,
  `mcp`, `plugin`, `plugins`, `auth`, `import`, `project`, `auto-mode` and `gateway` look after
  Claude Code on this machine, or set up its login, and switchboard has no part in them. When the
  first of Claude Code's arguments names one, `run` starts `claude` as if switchboard weren't
  there, environment and arguments untouched, an inherited pin included, and says nothing; the log
  notes it at debug. `--direct` still starts it on Claude Code's own login. Only the first argument
  counts: `claude -p doctor` is a prompt. Everything else goes through `run` as any session does,
  background sessions (`claude --bg`), `agents`, `attach`, `respawn` and `ultrareview` among them.
  `internal/claude` keeps the list.
- **Finding the real `claude`:** `run` looks along `PATH`, then where its installers put it
  (`~/.local/bin`, `/opt/homebrew/bin`, `/usr/local/bin`, `~/.claude/local`), for a file that can
  be run, passing over any `claude` that's switchboard, which would start it again: one that
  leads, links followed, to switchboard's own executable, and another build of switchboard, as
  the Go build info it holds says. It replaces itself with the `claude` it finds (`exec`), so
  signals and the terminal behave as usual, and the `claude` keeps its process id. Every `claude`
  it starts, as if switchboard weren't there or not, starts with `SWITCHBOARD_STARTED` set to that
  id and where the `claude` is. A switchboard started with it naming its own id was started again
  in that `claude`'s place, as by a wrapper named `claude` that `exec`s switchboard, and looks past
  that `claude`, failing when there's none rather than start it again, and again. Any other, such
  as a `claude` started within a Claude Code session, has an id of its own, and looks everywhere.
  Probes claim the version of the `claude` found the same way, so the router, whose `PATH` is
  launchd's, finds it where its installers put it. Claude Code's arguments go after `--`,
  untouched and never logged; the log notes the decision: routed or direct, the router's state,
  and the account and why.
- **The service:** `service install` reads the config first, as the router will, and fails on one
  it can't read, rather than leave launchd restarting a router that can't start. It writes the
  LaunchAgent (`RunAtLoad`, `KeepAlive`, output to `launchd.log`, and an `ExitTimeOut` of 45
  seconds, over the 30 the router gives requests in flight as it stops) to run this binary by the
  path it was run by, so a Homebrew link stays the link an upgrade moves on, with `serve`, any
  `--config` given, made absolute, and any `--log-level`. It refuses a temporary build, such as
  `go run`'s, judged by where the binary's links lead. It carries `XDG_CONFIG_HOME`,
  `XDG_STATE_HOME`, `SWITCHBOARD_CONFIG` and `CLAUDE_CONFIG_DIR`, those two made absolute, and
  `SWITCHBOARD_LOG_LEVEL` when they're set, so the service finds what the CLI does, and brings
  the skill up to date where `setup` wrote it. It runs switchboard directly: the tokens are files,
  so it needs nothing else of the user's environment. `install` warns when no account has a usable
  token; the router starts all the same, and routes once one has. Whether launchd has the service
  loaded is `launchctl print`'s to say, which exits 113 for one it hasn't: `install` boots out a
  loaded copy, bootstraps the new one into `gui/<uid>`, and waits up to 5 seconds for a router other
  than any running before to answer. `uninstall` boots it out when loaded and removes the plist.
  `restart`, with a router answering, has launchd send it SIGTERM (`launchctl kill`): it stops as at
  any signal, finishing its requests in flight, and launchd, keeping the service alive, starts it
  again. `restart` says so first, `the router is finishing its requests in flight, then launchd
  starts it again`, then waits up to 50 seconds, the 45 launchd gives the router to stop and 5 to
  start, for a router other than that one to answer. With none answering, there's nothing to finish,
  and `restart` is `launchctl kickstart -k`, waiting up to 5 seconds. Without the service loaded,
  `restart` is an error saying so. `status` reports the plist, whether launchd has it loaded, and
  the router's health. When no router answers in time, `install` and `restart` fail, the LaunchAgent
  in place, pointing to `switchboard logs router` and `launchd.log` for why. Each run of `launchctl`
  is cut off after 10 seconds, but a bootout, which waits for the router to stop, after 55: the 45
  and 10 more. Any other failure of `launchctl` is an error that quotes it.

## Milestones

**0. Spike — done.** A throwaway pass-through proxy that swapped the token on `/v1/messages` and
logged every exchange, with one session driven through it in print mode and interactively.
Confirmed:

- Claude Code honours `ANTHROPIC_BASE_URL` with a setup token.
- `x-claude-code-session-id` is on every `/v1/messages` request and survives `--resume`.
- A header set through `ANTHROPIC_CUSTOM_HEADERS` reaches the proxy.
- The only paths seen were `HEAD /api/hello` (a connectivity check, sent without a token) and
  `POST /v1/messages?beta=true`.
- A forced switch mid-session works: the conversation carries on, and the cache rebuild shows in
  usage exactly as predicted (figures under the cache facts above).
- The usage headers come from whichever account served the request, and so do the rate limits
  Claude Code hands its statusline. Switching accounts mid-session moved the statusline's figures
  to the new account's on the next response.

**1. Core — done.** Config and accounts, header parser, probe, scoring, `usage`, `status`. Useful
on its own before the router exists.

**2. Router — done.** `serve`, stickiness, replay, special handling, pins, `service`, `run`,
`init`, and the dashboard and `status` reading the router.

**3. Setup, tokens and priming — done.** Token files and the `accounts` commands; `setup`; every
`claude` through switchboard, and `init zsh` gone; the primary account and its reserve; priming, and
no accidental windows; the 5-hour tiebreak; account-bound thinking; pins for one running session,
and `--force`; the router looking after itself; the skill; `status --session` in one line, and
`status` listing the running sessions; macOS-only builds, and the README to match. Its final review
changed: the router starts with no usable token, and routes once a token file holds one; a token
file caught empty while it's rewritten keeps its account's token; `state.json` keeps every
configured account's token hashes, and a start drops only what belongs to accounts no longer
configured; an account that goes out on another token loses its 401 refusal at once; `serve` notes
the config file and the binary before it reads the config; the tiebreak takes scores at least 0.8 of
the highest; a prime that doesn't start the window counts as failed, and the next prime is worked
out in local time; a notification that fails to post starts no quiet minute; the dashboard says
`nothing read yet` while nothing has been read; no id or label may hold a token, and no command or
config error quotes one; setup reinstalls a service that runs another switchboard, and restarts the
router only when it doesn't answer; `service restart` lets the router finish its requests; `claude`
steps aside for an API key; and `switchboard version`.

**4. Switch-over — next.** The author's shell and dotfiles move onto switchboard, outside this repo.

The release, through GoReleaser, a Homebrew tap and mint, follows milestone 3.

## Checks owed

What's built but hasn't been seen against the real thing:

- What a quota 429 and a burst 429 look like (needs a real limit).
- A desktop notification posting.
- `service install`, `service restart` and setup's service step against the real launchd: `restart`
  relies on launchd's `KeepAlive` starting the router again once it stops at the SIGTERM `launchctl
  kill` sends.
- The 5-hour window's mechanics, on the first primes: the reset a prime reads should be five hours
  on.
- An artifact published from a session the router has moved opening in a browser signed into the
  primary, and whether a conversation request ever refers to an uploaded file by id.
- `claude doctor` with the link in place.
- Background sessions (`claude --bg`) going through the router.

## Open-source hygiene

- No personal data in the repo or its history, ever: no account emails or labels, tokens, token
  file paths or machine names. Examples use placeholders.
- Public, under the MIT licence, and released through GoReleaser to a Homebrew tap. Built for its
  author, and general only where that's free.

## Backlog

- **Deferred:** browser (OAuth) logins, and other agents.
- **Prompt-cache keep-warm (parked):** before a session's hour lapses, replay its last request with
  `max_tokens` of 1 on its account, renewing its cache for a fraction of a rebuild. Open: how the
  limits count cache reads; stopping for sessions that have closed (`run`'s pid is Claude Code's,
  which could tag its requests); and the conflict with re-scoring an idle session. `max_tokens`
  isn't part of what the thinking check covers.
- **Artifact proxy:** serve an account's artifacts locally, so one browser sees every account's.
  It hinges on whether a setup token can read an artifact through the API the artifact tool uses,
  and claude.ai's live features wouldn't work through it. The primary makes it less needed.
- **Move notice (deferred):** a `UserPromptSubmit` hook that shows a line in the TUI after a move,
  and gives Claude the same line as context. Never written into the conversation, which the
  thinking check rules out.
- **Intercepting traffic that ignores `ANTHROPIC_BASE_URL`** (a local-CA mode): not planned. What
  it would catch is in What doesn't go through the router.
