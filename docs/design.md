# Switchboard — design

**Status:** milestones 0 to 3, 5 and 6 are built, and this document describes the design as built,
but for milestone 7's, which it describes as designed, ahead of its build (see Milestones).
Milestones 0 to 2 built the usage dashboard (one-off, and in watch mode) and `status`, which read
the router while it runs and probe when it doesn't; logging; the router, with its scheduler, pins,
state, limit and refusal handling, health and desktop notifications; and launching (`run` and the
service). Milestone 3 added token files and the `accounts` commands, `setup`, every `claude` going
through switchboard in place of `init zsh`, the primary account and its reserve, priming the 5-hour
windows, and the router looking after itself; Milestones lists it in full, with what its final
review changed. Next is milestone 7 (see Milestones); milestone 4, the author's switch-over, happens
outside this repo. The release follows milestone 3. Milestone 5 rebuilt the dashboard as its owner
redesigned it, in three views, with themes, and with the router's history, events and request stream
to draw them from. Milestone 6 added the request ledger, a line for each request the router routes
and a summary of each day, with `requests` and `history` to read them back. Milestone 7 redesigns
the dashboard again, on what the ledger records: `usage` becomes a printout, `dashboard` the live
page, in six tabs, and the data verbs give an agent everything either shows. The Dashboard section
and The data verbs describe it.

## What it is

A local proxy that spreads Claude Code sessions across several Claude subscriptions. It uses each
account's quota right up to its limit, moves a session to another account when its account runs
out, and otherwise keeps every session on one account so its prompt cache stays warm. Any account
can keep back a share of its quota as a reserve, and each account's 5-hour window is started on a
schedule that spreads the resets through the day. It also ships a terminal dashboard showing every
account's usage.

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
  header switchboard's own launcher adds, which it strips, and the encodings the request accepts
  its answer in, narrowed to those the router can read (see Proxy rules), so the request is still
  genuinely Claude Code's. Switchboard never edits a request's body, which is also what keeps
  Claude's thinking valid across turns (see Cache and thinking facts).
- Claude Code's own token is the primary account's, so what it sends that isn't the conversation,
  such as publishing an artifact, and what it sends around the router, goes out on the primary,
  whichever account the conversation is on. See The primary account.
- Every response, success or 429, carries `anthropic-ratelimit-unified-*` headers: utilization
  and reset time for each window (`5h`, `7d`, and per-model weeklies such as `7d_oi`). Only a 429
  that refuses the request itself carries none (see Choosing an account, step 7). Switchboard
  reads them off real traffic, so it knows each account's usage without spending requests.
  Windows are parsed generically, not hard-coded. Of two readings of a window taken apart, one off
  a request sent after the other was taken in is current, whatever it reads: the API reckons use
  as it takes a request in, so no reordering can make that reading the older, and a reset made by
  hand on claude.ai, which drops use but may keep the reset, is seen. The answer to a request sent
  before may have been overtaken, so then the window's reset says which is current: a later reset
  is a new window; with the same reset the higher utilization stands, as use only rises within a
  window, so a slow response can't pull it back, nor lift a rejection either reading holds; an
  earlier reset is ignored.
- The router probes every account it has no reading for as it starts; its readings outlast a
  restart. After that, an account with no recent traffic is probed only when a decision needs fresh
  numbers, a dashboard asks for them, or it's due a prime, and never once its 5-hour window has
  lapsed, as the probe would start the window off the schedule (see Priming), but while it can take
  no request anyway, when starting the window costs nothing. An account never read, as
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
  The token switchboard swaps, the header it strips and the encodings it narrows aren't part of
  that, and it edits nothing else, so its requests pass. Anything that wrote into the
  conversation, such as a notice of a move, would fail it.

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
   window's room ends at the account's reserve (see Reserves): with a reserve of 0.1, a window
   reading 90% has none. That holds the router's own choices alone: a pin runs its accounts to
   their limits (see Pinning).
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
3. **Pressure:** several busy sessions on one account run its 5-hour window out together, then
   move at once, each rebuilding its cache on another account; scoring by the week alone doesn't
   see it coming. So the router keeps, of each of each account's windows, the levels its use was
   read at, each with when it was first read so and when last: its baseline, the level read last
   before the last 30 minutes, and each higher level since, within the window as it now runs. A
   later reset, a new window, clears them, and so does a reading taken as current that has fallen
   by a tenth of the window or more, as a reset made by hand leaves it (see `usage`, the printout);
   a smaller dip stands as the window's reading but is no level, so climbing back from it reads as
   no use. A window's recent rate is its rise from its baseline to its latest level, never less than
   0, over the last 30 minutes when the baseline was read again after they began, its use holding
   there till then; else over the time since the baseline was last read: an account is read only
   on its own traffic, its primes, and a probe before a choice once 15 minutes stale, so use
   outside the router, as in the Claude apps, arrives as one rise across a gap, which came at no
   telling when within it, and 20% read two hours on reads 10% an hour, not 40% for half an hour.
   A window with a baseline and no level since has been quiet, and reads 0, so a burst holds its
   rate for the 30 minutes after it, then drops to 0. Without a baseline, as for a window started
   less than 30 minutes ago, the rise is from its first level, over the time since it was first
   read, which must be 10 minutes back at least. The weekly windows' projections go by it too (see
   `usage`, the printout). The 5-hour window's rate is its recent rate, or, without a baseline or a
   level 10 minutes back, its use since it started, once 5% of it has passed, as the dashboard's
   projection measures it. An account is under pressure when, at that rate, its 5-hour window
   reaches where the account runs out before it resets: where its reserve starts, or its limit where
   the request may spend the reserve, as a pin spends its accounts' (see Pinning). A choice made
   afresh sets the candidates under pressure aside first, then scores the rest as step 2 says,
   keeping an idle session's own account unless another is well ahead; when every candidate is under
   pressure, pressure changes nothing, so it never leaves a request without an account. It weighs
   only choices made afresh: a new session's, a request's without a session, a session's idle past
   its cache's hour, or whose account can't serve the request, or whose own pin yields, and a
   session the global pin moves; and within the global pin's accounts first, so it never sends a
   request past the pin. A session's own pin is never weighed, and a session staying where it is,
   sticky or bound, never moves for it. An account the scoring can't rate, as one whose week's reset
   isn't known, is no relief from pressure: with the global pin naming it and one under pressure,
   requests go to the one under pressure, as though every candidate were, rather than to the one
   whose quota can't be judged, as they would once none of the pin's accounts can be scored at all
   (step 4 of the order below). The readings outlast a restart: as it starts, the router takes them
   up, baselines included, from its readings history, which holds each change of a window's use (see
   Files), but for a window that has reset since.
4. **New session:** the best candidate is assigned, keyed on the session id Claude Code sends
   (`x-claude-code-session-id`) and the model, and remembered as it's chosen, then forgotten unless
   the request is answered with success. Caches are per model anyway, so a session's Haiku calls can
   sit on a different account from its Opus calls at no cache cost. The id survives `--resume`, so a
   resumed session finds its account again. A new session's request that ends otherwise, in a 429, a
   refusal, another error or its client gone, leaves nothing remembered, unless another request of
   the session has been routed since, whose account stands: the quota check `--resume` sends as it
   starts goes under an id it never uses again, and refused, as on Claude Opus 5.5 today (step 7),
   would otherwise be kept, and listed among the sessions, for a week. A request the router knows
   for that quota check, `max_tokens` 1 and one message whose content is `quota`, is never
   remembered, whatever its answer: on Claude Haiku it's answered with success, and would otherwise
   claim the session's first account, and keep a `--resume` check's throwaway id for a week. A
   session already remembered keeps its account whatever its requests end in, but for a request
   every account it went out on refused, which leaves the session where it was before (see Requests
   that need special handling).
5. **Sticky:** the session stays on that account. It is only re-scored when:
   - it has been idle for more than an hour, the cache TTL, by the wall clock, which runs on while
     the Mac sleeps, so its cache is cold and a move costs nothing. Re-scoring prefers its own
     account, which another must beat by 20%, so near-equal accounts don't trade places; once one
     does, the choice is made as for a new session. A session on a model whose thinking is bound
     to its account (Claude Sonnet 5.5 today; `internal/claude` keeps the list) isn't re-scored
     for idling, as a move would lose its reasoning; or
   - its account can't serve the request. An account nothing has been read of counts as able, so
     neither a session nor a pin moves on no evidence.
6. **Limit hit:** a 429 whose overall status or any window's status reads `rejected` means real
   exhaustion. Switchboard replays the buffered request on the next candidate, among the accounts
   the request hasn't been tried on, before any response reaches Claude Code, and the session
   moves there and stays. Claude Code sees a normal, slower response. The account then has no
   room, whatever its windows read, for the requests the windows the 429 rejects count, whether
   or not it gives their utilization (every request when it names none), until the reset the 429
   gives: the overall reset, else the latest of the rejected windows', else 5 minutes on. The
   answer to a request sent after the limit was set lifts it sooner when it shows those windows
   with room, as a probe's does once the limit is reset by hand (see Priming), or, for a limit
   whose 429 named no window, when it's a success of a request that spends quota, a probe's
   included, and counting a message's tokens spends none. A limit reached while one holds is that
   limit reached again where it's in a window that one was named in, or either names none, and
   another limit otherwise; either way the account's limit holds as the latest 429 says, and a
   probe that reads it again changes nothing.
   A 429 to a request sent before the one whose answer showed a window it rejects reset by hand (see
   `usage`, the printout) is from before the reset: the limit holds in the windows it rejects that
   weren't, and when there are none, it's no limit, and the request goes out again on the same
   account, after the reset. One that rejects no window can't be told from a limit in a window that
   wasn't reset, and holds.
7. **Throttling:** a burst 429 without exhaustion gets a pause, as long as its `retry-after` asks
   (2 seconds when it doesn't say, 10 at most), and a retry on the same account, twice at most;
   then the 429 is passed through. It never triggers a move, because moving would throw the cache
   away for nothing. A 429 without the usage headers, neither the overall status nor any window's,
   isn't throttling: it says nothing of the account's quota, but refuses the request itself, as
   the API refuses the quota check Claude Code sends as it starts, on Claude Opus 5.5 today. Sent
   again, the request would fare no better, so the 429 is passed through at once, as it came, with
   nothing held against the account, for Claude Code to retry if it will.
8. **No forced return:** after the original account resets, the session isn't moved back; that
   would cost a cache rebuild for nothing. The idle rule brings it back when a move is free.
9. **Pool exhausted:** when no account has room, switchboard first re-probes those whose readings
   say they have none, each at most once a minute, but for one whose 5-hour window has lapsed (see
   Priming), waiting 5 seconds at most, as a reset may have passed with no traffic to show it;
   then it decides again. Failing that, a request replayed after a limit or a refusal goes out on no
   other account: it gets the last 429, passed through, or, refused last, the answer Requests that
   need special handling gives; any other falls back as step 5 of the order below says.

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
   Either way, those under pressure are set aside while another candidate isn't (see step 3 of
   Choosing an account). When none of the pin's accounts that can serve the request has a score,
   the first of them does.
5. With no candidate, the session's account, else the client's, else any other, for the upstream to
   say why, passing over those that refused the request lately, and those held back only by their
   reserve, which would serve it and spend the reserve. When that leaves none and an account is held
   back by its reserve alone, switchboard answers 429 itself, shaped as the API shapes its errors,
   with the usage headers Claude Code reads a limit from: `rejected`, and, when it's known, when the
   first of them is let go, each at the latest reset among the windows its reserve holds. When
   every account has refused the request lately, it goes out on the client's all the same, and,
   refused again, ends in the 502 of Requests that need special handling. A request replayed goes
   out on none of these: the upstream has said why on the accounts it was tried on, and the session
   is remembered on no account the request didn't go out on.

A request without a session id is never remembered: it goes to the launch pin it carries while that
account can serve it, and is otherwise decided afresh every time. Before deciding afresh, and never
for a sticky request, switchboard probes every account it hasn't read in 15 minutes, all at once,
but for one whose 5-hour window has lapsed and that can take a request (see Priming), and waits
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
| `…, <id> under pressure` | Any reason of a choice made afresh, or of a move by pin, when setting the accounts under pressure aside changed it (step 3 of Choosing an account): `<id>` is the account it would have chosen, or, when that one isn't under pressure, the highest scoring of those that are, whose score kept the account chosen out of reach. The log's `passed over under pressure` line, one a choice, gives `<id>`'s rate and when it runs out at it |

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
spent, passing over those under pressure, judged at their limits, while another of them isn't:
pressure never sends it past them. When none of them can serve the request, the pin yields: the
router chooses among every account as though nothing were pinned, the reserves of the accounts it
doesn't name held as ever. So a pin sets an order to spend the accounts in: those it names first,
the best of them first, then the rest. The best next that `status` gives, which the dashboard's
status line names after `new sessions →`, is where a new session goes, the pin's accounts first.
`--move` moves a running session on an account the pin doesn't name to the best of those it does;
one on an account it names stays.

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
own choices, and a pin is the user's. So when an account is at its reserve and every other is out,
`pin <id> --move` carries the running sessions on there, in place, and `pin auto` hands them back
to the router, reserve and all.

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
  token would only be refused again, its message giving the upstream's reason, cut to 200
  characters, with anything shaped like a token hidden.
- **A request refused everywhere bars no family:** when every account a request went out on refused
  it, the refusals of the request itself it met, the 403s, are taken back, as a refusal every
  account that judged the request gives says more of the request, such as a beta it carries, than
  of the accounts. Otherwise one such request would hold its model's family back on every account
  for 10 minutes, and every session of the family would fall back to the client's account, the
  primary, and move there. A refusal of an account's token, a 401, stands, as it says something of
  the account; so does a refusal another request met, and one met by a request another account
  served, or whose limit it reached. The request's session goes back where it was before the
  request, as the moves its replays made came to nothing, unless another request of the session
  has been routed since, whose account stands; the moves stand told, in the log and any
  notification of them. The client still gets the 502.
- **Replay:** request bodies, up to 64 MiB, are buffered so they can be replayed. A routed
  request whose body is larger is answered 413 (`request_too_large`), and one whose body can't
  be read 400, whoever is left to read it, as a client that closed its side of the connection
  still reads, neither going upstream nor counting towards the router's health, though each has
  its line in the request ledger (see The request ledger). Replay only happens before response
  headers have been sent; a failure mid-stream is passed through and Claude Code retries. Nor is
  a request that couldn't reach the upstream at all replayed elsewhere: that isn't the account's
  fault. Claude Code gets a 502 and retries.
- **Uploaded files (pending):** Claude Code uploads files on its own token, the primary's. Should a
  conversation request turn out to refer to one by id, which the artifact check will show (see
  Checks owed), such a request is to go to the primary, the only account that can read the file.
  Nothing does so yet: the rule waits on that check.
- **Storm control:** decided against, as the pause and retry on a throttled account (step 7
  above) already absorbs the burst limit many sessions moving onto one account at once can trip,
  where pacing them would slow every request.
- **Bypass traffic:** some requests (fast mode, WebFetch) ignore `ANTHROPIC_BASE_URL`, so the Claude
  Code process still needs a real token in its environment. That traffic goes out on the primary:
  see What doesn't go through the router.

## The primary account

One account is the primary: the one the browser and the Claude apps are signed into.
`primary = true` marks it; without it, the first account is the primary. So removing the primary
leaves the account marked, else the first, the primary, which `accounts remove` names; the
sessions running on the removed account's token, as every routed session holds the primary's, stay
routed, as the new primary's, while it has a usable token (see Accounts and tokens).

- **Claude Code's own token** is the primary's. `run` gives it to every routed session, whichever
  account the conversation goes to, a session pinned to another account included: the pin moves
  the conversation alone. So what Claude Code sends that isn't the conversation, such as
  publishing an artifact or uploading a file, goes out on the primary, and every session's
  artifacts open in a browser signed into it, whichever accounts its conversation went to. A
  session resumed later gets the same token, so its earlier artifacts stay within reach. While the
  primary's token isn't usable, a routed session gets `--account`'s, else the first account's with a
  usable token. Not routed, `run` gives Claude Code `--account`'s token, else the primary's, else
  the first account's with a usable token: see Launching.
- `accounts`, `status` and the dashboard mark the primary.

## Reserves

Any account can keep a reserve, which `reserve` in its table sets (see Config).

- **The reserve** is the share of every window, the 5-hour window, the shared weekly window and
  each model's own weekly, that the router leaves unused on an account: 0 unless set, on the
  primary as on the others. Once a window that applies to a request reads at or above 1 less the
  reserve, the router's own choices pass the account over: new sessions skip it, and a session on
  it moves as at a limit. Scoring counts only the room before the reserve. The router never sends
  a request to an account held back only by its reserve, even when no account has room, as that
  would spend it. A pin does spend it, running its account to its limit: the pin is the user's
  choice, where the reserve holds back the router's (see Pinning). So an account with a reserve
  keeps that share of every window for use outside the router, such as the Claude apps on the
  primary, where that use can take it past the reserve, as intended, and one without is used right
  up to its limits.
- Readings come off responses, so one large turn can take an account a point or two past its
  reserve before the router sees it. A launch that goes direct, without the router, can spend the
  reserve of the account it goes out on.
- The dashboard marks where each reserve starts on its bars, and `status` and the dashboard say
  when a reserve holds its account back, or, on an account the global pin names, that the pin is
  spending it; a session's own pin spending it reads as the reserve holding the account back.

## Priming

Anthropic describes the 5-hour window as starting at an account's first message after its last
window ended, and resetting five hours later. Every reset seen so far falls on a ten-minute mark,
as though the window's start is taken back to the ten minutes it falls in: a probe at 16:46 read a
reset at 21:40 (see Observed). Left alone, an account's first window starts with the
day's first request on it, so an 08:00–23:00 day meets three of its windows. Started earlier, a
fourth fits, the first and last partly outside the day. Started at staggered times, the accounts
come back one at a time rather than together: once all are spent, the wait for the next is at most
5 hours ÷ the number of accounts, rather than until the one reset they share.

- **The day:** `[prime] day = "08:00-23:00"`, in local time, turns priming on. An end before the
  start means past midnight.
- **The schedule:** with N accounts that have usable tokens, resets fall every 5 hours ÷ N; the
  first falls half a step after the day starts; each account, in config order, has its slot five
  hours before its first reset, taken back to the ten-minute mark that falls in, as the API takes
  back the start of the window a prime starts: so the slot is where the window starts, and the
  reset it reads five hours after, as the schedule shows. Taken back rather than rounded, a slot
  only ever moves earlier, so every prime still falls before the day starts, so the day's first
  requests don't disturb the schedule, and may fall the evening before; and every slot moves by
  the same rule, so the steps between them stay within ten minutes of each other, as even as the
  marks allow where 5 hours ÷ N isn't a whole number of ten minutes. A slot's prime goes 5 seconds
  after it, as a prime after a reset does (see below): sent on the mark, a prime the API took in
  a moment before it, by its own clock, would start the window ten minutes earlier. For a day
  starting at 08:00:

  | Accounts | Primed | Resets |
  |---|---|---|
  | 4 | 03:30, 04:50, 06:00, 07:20 | 08:30, 09:50, 11:00, 12:20, 13:30, 14:50, 16:00, 17:20, 18:30, 19:50, 21:00, 22:20 |
  | 3 | 03:50, 05:30, 07:10 | 08:50, 10:30, 12:10, 13:50, 15:30, 17:10, 18:50, 20:30, 22:10 |
  | 2 | 04:10, 06:40 | 09:10, 11:40, 14:10, 16:40, 19:10, 21:40 |

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
  is sent again five minutes on. An account that can take no request anyway (see No accidental
  windows), or whose token is refused, isn't primed while it's so, as a prime couldn't start its
  window, but the moment it could be, once its limit or refusal ends, or its spent window resets;
  it's still probed where the probe rules allow, which is how a reset made by hand is seen.
- **Through the day,** whenever an account's window resets, in use or not, the router primes it 5
  seconds on, so its windows stay back to back: the API's clock may be a little behind the Mac's,
  and a prime it took in before the reset would start nothing, and go again five minutes later,
  the window's start with it. After the day ends, it stops, so the windows lapse overnight and the
  next morning's primes start them afresh. An account's day of priming runs from 5 seconds after
  its slot until the day ends.
- A prime missed while the Mac slept, or the router was away, goes out when the router next can,
  unless the day has ended: the router looks at least once a minute, as a timer's clock stops
  while the Mac sleeps.
- Every time of the schedule is on the local clock, when an account is next primed included: a day
  the clocks change on keeps the slots and the day's end at their times of day, and a Mac taken to
  another time zone restarts the router, which keeps them at their times of day in that zone (see
  The router looking after itself).
- The router works the schedule out as it starts, and again whenever an account gains a usable token
  or loses it; a change to the accounts or the day is a change to the config, which restarts the
  router (see The router looking after itself). `status` shows the schedule. `status` shows
  the next reset among the 5-hour windows and, from the router, the next prime; the dashboard's
  COMING UP gives the next few resets and primes, and a lapsed window says when its account is next
  primed (see The Overview).
- Early starts, late nights and use in the Claude apps can start a window off the schedule, which
  shifts that account's slot for the day.
- The first primes confirmed the window's mechanics: each reset a prime read fell five hours after
  its slot, the ten-minute mark it goes 5 seconds after (see Observed).

**No accidental windows.** A probe is a request, so probing an idle account starts its 5-hour
window. The router never probes an account whose 5-hour window has lapsed, its last reading's reset
passed with nothing read since, except to prime it: that window reads empty, and the account's
weekly readings stand. This covers the probes as the router starts, before it decides afresh, when
no account has room, and for `POST /refresh`. The one exception is an account that can take no
request anyway: a limit holds back its every request, or a window every model shares reads spent, as
last read, as when its week is spent, the limit or not, as a restart keeps the reading but not the
limit. A probe that starts its window costs nothing then, and a probe is how a limit lifted before
its reset, as by a reset made by hand on claude.ai, is seen, the reading showing its windows with
room lifting the limit. A limit reached in one model's week alone is no exception, as the account
takes other models' requests, nor is a refused token or model. Readings persist in `state.json`,
with the model families each window has been seen to count, so a restart needs no probe. An account
never read has no window known to have lapsed: it's probed as the router starts, and, should that
probe fail, or the account first gain a token while the router runs, whenever a choice made afresh
or `POST /refresh` needs it, at any hour, which may start its window off the schedule, once. Probing
without the router, and with `--probe`, is unchanged: it's asked for; so is the probe that checks a
token `accounts add`, `accounts token` or `setup` is given.

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
  pinned account's when it finds the file there but empty, as a writer that empties it leaves it
  for a moment, before it starts Claude Code on another's, which would be Claude Code's own token
  for the whole session; a file that's missing, or isn't the user's alone, it looks at once. The
  LaunchAgent needs no token from the user's environment: it carries only what finds the config,
  the state and the skill, and the log level (see Launching).
- A token the router replaces stays its account's for 7 days: sessions started before hold it,
  and every session holds the primary's. The router keeps it as its SHA-256 hash, never the
  token, in `state.json`, and routes a request carrying it as the account's (see Proxy rules). A
  token replaced while the router was away counts too: `state.json` keeps the hash of the token
  the router held last, which it compares with the file's as it starts. It keeps the hashes of every
  configured account, with a usable token or not: of an account without one as the router starts,
  the hash of the token held last is compared with the token the account later gains, and a
  different one counts as replaced.
- An account removed from the config leaves its tokens, the one the router held last and those it
  had before, to the primary, as former tokens of the primary's: a session started with one, as
  every session holds the primary's, carries on sending it, and stays routed, its client account
  the primary. The primary takes them up once, as they stood when the router found the account
  gone, at the first look that finds the config without it, before the restart that takes the
  config up (see The router looking after itself), or, when the router was away, as it starts, from
  what `state.json` kept of the account's tokens. A token the account's file comes to hold after
  isn't the primary's. They count for 7 days from then, those replaced before from when they were
  replaced, and aren't taken up again once they're forgotten. `state.json` keeps them as the
  primary's alone, marked with the account they came from: should the config configure it again,
  as when an edit by hand is caught half made, it takes them back, an account like any other
  again. While the primary has no usable token, they count no more than its own do.
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
  alone, saying where it led, as the file there isn't switchboard's. Of the primary, it says which
  account is the primary now, and that the sessions running on the removed account's token stay
  routed, as the new primary's, for a week from when the router takes the change up, or, when the
  new primary has no usable token, that they aren't routed until it has one.
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
6. Prints `usage`, every account's windows as they stand (see `usage`, the printout).

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
| `status [--session <id>] [--json \| --pretty] [--probe] [--refresh]` | Accounts, windows, sessions, pin, what holds an account back, reserves, pressure, the priming schedule, and router health, read as `usage` reads them, and from the router, the sessions it has routed in the last hour: a line each, with its id cut short, the account each of its models goes to, its own pin, and when it was last seen. When the `claude` a shell runs from `PATH` isn't switchboard, so the sessions it starts don't go through the router, the first line says so, pointing to `setup`. Its JSON is the status document, which is what an agent reads (see The data verbs). `-r`, `--refresh` has the router first read every account it may, as `usage --refresh` does. `--session` prints one line, on a terminal or off one, as a statusline asks: the id of the account the router sends a session's requests to, the one its last-used model went to; or with `--json`, `/sessions/{id}`'s answer. `<id>` is the session's id, or as much of it as is unique among those sessions, as `pin --session` takes it, and it needs the router, taking neither `--probe` nor `--refresh` |
| `usage [--5h] [--week] [--<model>] [--by window\|account] [--probe] [--refresh] [--json \| --pretty]` | Every account's windows as they stand, printed once into the scrollback with the time they were read, to hold against one printed earlier: see `usage`, the printout. `--by` lays them out by window, each window's rows together, or by account, each account's windows together, over `[usage] by` (see Config); by window where neither says. `--5h`, `--week`, and a flag for each model's own window, named for its model, as `--fable` is for Fable's week, print the windows given alone, with the header and the key line. Its JSON is the status document, as `status --json` prints it, read as its flags say (see The data verbs). It reads the router while it answers; `--probe` probes instead. `-r`, `--refresh` has the router first read every account it may, as the dashboard's `R` does, and waits for it, ten seconds at most; without the router, or with `--probe`, every account is probed anyway |
| `dashboard [interval] [--no-notify] [--probe]` | The dashboard, full screen until `q`: see Dashboard. It needs a terminal, and refuses without one. It reads the router while it answers, and every interval has the router read the accounts it hasn't read in that time (see Live updates): 30m unless given, 5m at the least, as a duration, such as `15m`, or a number of minutes. Without the router, or with `--probe`, it probes every account instead, each interval. `--no-notify` has it post none of the notifications it posts while it probes without the router (see Notifications) |
| `requests [--session <id>] [--account <id>] [--since <when>] [--json \| --pretty]` | The request ledger's lines (see The request ledger): today's, unless `--since` reaches further back, oldest first, under the day each arrived on, a line each with the time, the session's id cut short, the model, the account, the status, the tokens in and out, and how long the request took. `--session` keeps a session's own, by its id or as much of it as is unique among the sessions read, failing, listing them, for as much as starts several; `--account` keeps an account's own; `--since` starts at a day (`2026-10-01`), a time today (`14:00`), as the clocks first read it that day, so `00:00` is the day's first instant, or a duration ago (`3h`, `2d`, a day being 24 hours), and one still to come is refused. Its JSON is each line as the ledger holds it, a JSON object a line, fields a later release added included (see The data verbs). It reads the ledger's files, so it needs no router |
| `history [--since <when>] [--windows] [--json \| --pretty]` | The request ledger's days: the last 30 unless `--since` says otherwise, from the day it falls on; of each with requests, a row for each account and model with its requests, those that went upstream and those the router answered itself (see The request ledger), its tokens, sessions and worth, at today's prices, then each account's sessions, moves, limits reached and windows' highest use; today's from its lines, as far as it's gone. Then, over those days, the totals Accounts and History count, each account's plan and its worth against it, each week's peaks, and Weeks' verdicts on the accounts (see Accounts, History). `--windows` prints in their place each account's windows' use over those days, a line each time a window's use, its reset or its status changed, as the readings history holds it (see Files). Its JSON (see The data verbs) is `{"prices_as_of": "2026-10-07", "days": [{"version": 2, "day": "2026-10-05", "lines": 424, "bytes": {"plain": 0, "compressed": 41208}, "accounts": […]}], "year": {…}, "months": […], "weeks": […], "tokens": […], "totals": {…}, "plans": […], "capacity": {…}}`, `prices_as_of` the day the price table was read from Anthropic's pricing page, and each day's summary as the ledger holds it, every day asked for from the first the ledger holds, so the last is today's, one of no requests without `accounts`, with each model's `worth` in US dollars added, exactly, left out where the model is unpriced, and beside it `unpriced`, the counts in its `usage` the worth leaves out, and `no_usage` where requests went upstream without usage, where there are any (see The request ledger). It reads the ledger's files and the readings history's, so it needs no router |
| `sessions [<id>] [--json \| --pretty]` | Today's sessions, as Sessions' List has them: those running, the oldest started first, then those that ended today, the latest ended first, a line each with its id cut short and its directory, the account it's on, its model, what it's doing, asking, answering or idle since it was last seen, or when it ended, when it started, its requests and their worth today, and how it came to its account; then a line summing them. `<id>`, a session's id or as much of it as is unique among the sessions read, as `requests --session` takes it, prints that session as its page has it: where it runs and why, each account it ran on, its turns, newest first, and its totals (see A session's page). Its JSON: see The data verbs. It reads the ledger's files, and the router's sessions where it answers |
| `events [--since <when>] [-f] [--json \| --pretty]` | The router's events, as the Log's Events have them (see The Log): today's, unless `--since` reaches further back, as `requests` takes it, oldest first, under the day each happened on, a line each, each event once, as it last stood, with the time, the session, if any, cut short, the kind, the account, and what happened. `-f`, `--follow` then prints a line each time the router files one, a changed event again, until interrupted, as `logs --follow` follows a log. Its JSON: see The data verbs. It reads the events' files, which outlast a restart (see The router's events), so it needs no router |
| `logs [router\|cli] [-n N] [-f] [--path]` | Print a log's last lines (`-n`, `--lines`: 50), or follow it (`-f`, `--follow`), or print where it is (`--path`): see Logging |
| `serve [--log-level <level>]` | Run the router in the foreground, normally started by the service. `--log-level` (debug, info, warn or error) overrides `SWITCHBOARD_LOG_LEVEL` |
| `pin <id>... [--move] [--force]`, `pin auto [--force]` | Set the global pin to the accounts given, replacing any before, or clear it: see Pinning. It needs the router |
| `pin <id> --session <session>`, `pin auto --session <session>` | Set or clear one running session's own pin, `<session>` being its id or as much of it as is unique. It needs the router |
| `run [--account <id>] [--direct] [-- <claude args>]` | Start Claude Code connected to the router, its conversation pinned to `--account`'s account if given. `--direct` skips the router and the token, so Claude Code uses its own login. See Launching |
| `service install [--log-level <level>]` | Install the LaunchAgent, which starts the router: see Launching |
| `service uninstall`, `service restart`, `service status` | Stop the router and remove the LaunchAgent; restart the router in place, letting it finish its requests in flight; report the plist, whether launchd has it loaded, and the router's health: see Launching |
| `version` | Print the version, as `--version` does |
| `help [command]` | List the commands, or print a command's help, as `-h` does |

The plain text `requests`, `history`, `sessions` and `events` print on a terminal is a first cut,
to be designed with their owner, as `status`'s text is (see Backlog); agents read their JSON, which
each prints wherever its output isn't a terminal (see The data verbs).

A command that needs the router fails without it, saying `the router isn't running: start it
with switchboard service install (or switchboard serve)`. Any notice a command gives on stderr,
rather than failing, reads `switchboard: …`.

No command quotes a token given where an id goes: `accounts token`, `accounts remove`, `pin`, to an
account or with `--session`, `status --session`, `requests --session` and `--account`, `sessions`
and `run --account` show an id shaped like a token as `[redacted]`, as `accounts add` does as it
refuses one, and `logs` a log's name. Nor does a command quote one in refusing what else it's given:
an argument it takes none of, a flag it doesn't know or a value the flag can't take, `dashboard`'s
interval, `--since`'s time, and the file `--config` names where there's none. Given where a command
goes, as `switchboard <token>`, or as `help`'s topic, as `switchboard help <token>`, a token is
quoted as given, by Cobra's own refusal of a command or a topic it doesn't know, which is left as it
is.

### The data verbs

`status`, `usage`, `requests`, `history`, `sessions` and `events` are the data verbs: each prints
what switchboard knows of one thing, in a form for a person to read or for an agent to take in.

- **Pretty on a terminal, JSON off one.** Where stdout is a terminal, each prints its form for a
  person; anywhere else, as in a pipe or Claude Code's Bash tool, its JSON. `--json` prints the
  JSON and `--pretty` the person's form wherever stdout is; given both, the command refuses them.
  `--pretty` off a terminal prints as on one, without colour, as wide as `COLUMNS` says, else 80
  columns (settled in the plan).
- **`status --session` keeps its bare id:** the id of the account the router sends the session's
  requests to, on a terminal or off one, as a statusline reads it; with `--json`,
  `/sessions/{id}`'s answer.
- **The person's form:** `usage`'s printout (see `usage`, the printout); `status`'s text; and, in
  plain text, first cuts to be designed with their owner (see Backlog): `requests` and `events`, a
  line each; `sessions`, a line each and one summing them; `history`, its days' tables and the
  totals over them; and `sessions <id>`, the session's page (settled in the plan).
- **One function a view, and every view's data in a verb.** What a verb prints is built by the
  function its view on the dashboard draws from, so a verb and its screen never disagree, and
  nothing a view shows is out of an agent's reach:
  - **`usage`, the printout, and the Overview's USAGE and its pool:** `usage` and `status`, the
    status document.
  - **The status line, ROUTING's states, COMING UP, Runway, and the accounts on the routing card:**
    `status`, with `events` for when a limit or a cap began.
  - **GRAPHS, and a day Runway steps back to:** `history --windows`, from the window's start or
    over the day; and the day's limits and highest use, `history`'s days.
  - **The traces and a session's story:** `requests`, a line a request, counted a minute at a time,
    by account or by session, as the traces count them.
  - **The Overview's SESSIONS, Accounts' Detail's sessions, and Sessions' Switchboard and List:**
    `sessions`, with `status` for the lines; the routing card's cost of a move, `sessions`'
    `move_cost`.
  - **Accounts' Compare and Detail, and History's Year, Weeks, Days and Tokens:** `history`, its
    blocks: days, year, months, weeks, tokens, totals, plans and capacity, with `status` for each
    account's windows and state now, and its extra usage.
  - **LATELY, the Log's Events, and Detail's LATELY:** `events`.
  - **The Log's Requests:** `requests`, and its day rule, `history`'s days.
  - **A session's page:** `sessions <id>`, and its requests, `requests --session <id>`.
  - **What's in flight:** `sessions`' `state`, which the dashboard shows as what travels a cord.

  A figure a view shows that no verb prints as drawn, as a trace's requests a minute, is worked out
  from what one prints, as the view works it out. Each reads where the dashboard reads: the router,
  or probing, for the status document, and the ledger's, the readings history's and the events'
  files where they lie, with no router (see Live updates).
- **Their JSON.** Fields may be added to any of them, never renamed, and a reader passes over those
  it doesn't know, as with the status document:
  - **`status` and `usage`:** the status document (see The status document), read as their flags
    say, each window's allowance and even pace, the pool's room and COMING UP among what it holds.
    `usage`'s window flags, `--5h`, `--week` and one for each model family with a window of its own,
    as the provider lists them (`internal/claude`), so they're known as the command line is parsed,
    and `--by` shape its printout alone (settled in the plan).
  - **`requests`:** a JSON object a line, each request's line as the ledger holds it, fields a later
    release added included.
  - **`history`:** one object, as Commands gives it, holding every block History's view and
    Accounts' build, over the days asked for, so an agent asks for a period by its first day
    (settled in the plan), as each view builds it: `prices_as_of`, and `days`, each day's summary as
    the ledger holds it, priced, with its worth by family, as Days draws it; then:
    - `year` and `months`: Year's grid, each day's requests and the level its shade is drawn at,
      with the key's cuts, and BY MONTH's rows, each month's requests, worth, plans' cost, limits
      and worth by family (see History).
    - `tokens`: Tokens' rows, a week each, its tokens by kind, `input`, `output`, `cache_write`,
      `cache_read` and `total`, its worth and against its price, by account and by model (see
      History).
    - `totals`, for all accounts and for each: `requests`; `sessions`, each once, by the
      summaries' session ids; `moved_on` and `moved_off`; `limits`, by window; `minutes_at_cap`
      and `minutes_at_limit`; `left_at_reset`, each window's share left at its last reset;
      `worth`, with `unpriced` beside it; `by_model`, each model version's `share` of use, weighed
      at API prices, and its `worth`; and `busiest_day` and `longest_run`: as Accounts and History
      count them (see Accounts, History).
    - `plans`: each account's `plan` (see Config) and its `price` a month, from the dated table
      (see The request ledger), with what it cost over those days, `cost`, prorated, and the
      worth `against` it, a ratio; left out of an account whose plan isn't known. An entry for all
      accounts sums them.
    - `weeks`: each week those days span, from the day `week_starts` names: each account's
      `peaks`, its own week's highest use of each window, placed as Weeks places it, and its
      `limits`; and `all accounts`' peaks: every account's, whatever was pinned (see History).
    - `capacity`: Weeks' verdicts over those weeks: how many were `short`, a week some account hit
      its limit, as the accounts stand, `with_one_fewer` and `with_one_more`, and the account to
      `drop`, where Weeks names one; and each account's `verdict`, in Weeks' words, with how many
      weeks it hit its limit, the share it had left at a reset, on average, and how many weeks the
      rest would have been short `without_it`.
  - **`history --windows`:** one object, `{"windows": [{account, window, readings}]}`, printed in
    place of the blocks (settled in the plan): each account's every window's readings over the days
    asked for, each `{at, utilization, resets_at, status, source}`, as the readings history holds
    them (see Files).
  - **`sessions`:** one object,
    `{"generated_at": …, "prices_as_of": …, "sessions": […], "today": {…}}` (settled in the plan).
    - `sessions` are the List's, in its order, each with `session`, its whole id; `dir`, its
      directory as the router or its lines give it, the home shown as `~`, left out where neither
      names one; and `account` and `model`, the account its latest request went to and that
      request's model, as the List's `on` and `model` show them, its last line's once it has ended.
    - `running` is set while the router lists it. One running also has `last_seen`; `models`, each
      model, the account it goes to now and why, `[{model, account, reason}]`; `state`, what its
      models' `in_flight` say (see Control API): `answering` where any model's is, else `asking`
      where any's is, and left out where no request of it is in flight; and `move_cost`, what moving
      it now would cost, its context written again, priced, as the routing card says it, left out
      where its cache would have run out anyway. One ended has `ended`, its last line's time.
    - `started` is its first line's time, today's or an earlier day's.
    - `requests` counts its requests today (see The request ledger). `worth` is theirs at API
      prices, in US dollars, exactly, with `unpriced` beside it where any count is unpriced.
    - `moved` is set where a request today moved it onto the account it's on:
      `{at, from, reason, cost}`, `cost` the worth of writing its context again there, left out
      where its cache would have run out anyway.
    - `today` sums them, `{sessions, requests, worth}`, as the List's `today` line does.
  - **`sessions <id>`:** one object, the session's page, a session of an earlier day found by its
    whole id, through the days the summaries name it in (settled in the plan; see The request
    ledger).
    - `session`, `dir`, `running`, `last_seen`, `models`, `state`, `move_cost` and `ended` are as
      `sessions` gives them, and `pin`, its own, where it has one.
    - `started` is its first line's time, and `resumed`, where it came back after an hour or more
      idle, the time it did.
    - `kept_until`, of one ended, is the last day its first day's lines are kept, as `[ledger] keep`
      says, the day its page shows, and is left out where that says `forever`.
    - `accounts` are each account it ran on, in the order it first did:
      `{account, from, to, requests, worth, points}`, `from` and `to` its first and last request
      there, and `points` each window's points its requests took, by key, an estimate (see A
      session's page).
    - `moves` are each move, `[{at, from, to, reason, written, cost}]`.
    - `turns` are newest first:
      `{turn, started, ended, requests, tools, read, written, out, worth, accounts}`. `tools` are
      the tools its answers called, each by name with how many times; `ended` is left out of one
      still going.
    - `totals` are `{turns, requests, from_cache, worth, unpriced}`, `from_cache` the share of its
      input read from the cache.
  - **`events`:** a JSON object a line, oldest first, each event once, as it last stood, as the
    router files it (see The router's events); with `--follow`, `-f` for short, as `logs`' is, it
    follows the events' files, as `logs --follow` follows a log, so needs no router
    (settled in the plan), a line more each time one is filed, a changed event again, with its `run`
    and `id`, so a reader takes the last.
- **What each needs:** `status` and `usage` read the router while it answers, and probe when it
  doesn't (see Live updates). `requests`, `history` and `events` read files alone, so need no
  router. `sessions` reads the ledger's files, and the router's sessions where it answers; without
  it, it lists today's sessions from the ledger alone, none as running (settled in the plan), and
  says on stderr that the router isn't running.

## Dashboard

**Milestone 7: designed.** This section is the dashboard as its owner redesigned it after 0.1.0,
area by area, each area signed off with frames in Paper between 4 and 8 October 2026, and as the
plan for building it settled what the frames leave open. It replaces milestone 5's, which the
history of this file describes. It's complete without any picture. Each area's frames are in a Paper
file, on a page named FINAL, which its section names, most in a file of their own, while Sessions
and Runway share *Switchboard · Sessions and Runway*, and a session's page and the Log *Switchboard
· Session and Log*; its references are kept in `testdata/vhs/reference/`, by the stems its section
gives. Where the frames and this section differ, this section holds, and Files lists every cell
where a reference differs from these rules, and why (see Files): most are key lines, drawn before
the keys were swept on 7 and 8 October 2026, and each section says where its own differ. The frames
are drawn in the `nord` theme at Thu 1 Oct 14:42:07, or at 16:05:41 and 16:05:42 for
`sessions-refused-160` and `sessions-moved-160`, with placeholder accounts (`work`, `personal`,
`side`, and for more, `client`, `spare`, `lab`, `team`, `extra`, `ops` and `studio`) and sample
numbers; they're 160 columns wide unless named otherwise, and 52 for a phone.

A person has two views of their accounts:

- **`usage`, the printout:** a compact, dated snapshot of every account's windows, printed into the
  scrollback and left there, to compare by eye with one printed an hour before (see `usage`, the
  printout).
- **`dashboard`, the live page:** it fills the terminal and redraws as things change, reading the
  router as Live updates says. It has six tabs, in this order in its title row:
  - **Overview,** the default: every account's windows and the router, understood at a glance
    (see The Overview);
  - **Accounts:** each account in depth, and all of them together: what each is, what it does,
    what it costs and whether it pays (see Accounts);
  - **History:** when: the weeks and days gone, and what used them (see History);
  - **Sessions:** where each running session's requests are going now, as a switchboard and as a
    list (see Sessions);
  - **Runway:** when each account has room, as a timeline (see Runway);
  - **Log:** every request and every event the router tells of, as they happen, followed or paused
    (see The Log).

  History the past, the Overview now, Runway the future; Accounts who, Sessions where. Beside the
  tabs, **a session's page** follows one session through its whole life, opened from any row that
  names a session (see A session's page), and the **routing card** changes where new sessions, or
  one session, go (see Routing by hand). The page, Keys and What moves give what every page shares.

`status` stays the detailed report. Everything either view shows reaches an agent as data through
the data verbs, each view and its verb sharing one function, so they never disagree (see The data
verbs).

Every view speaks as `usage` does:

- **A bar fills with what's used,** from the left, along the theme's ramp, and never drains; one
  window is compared across accounts on one aligned scale, never an account's windows alone.
- **`┃`,** in `viz.pace`, is even pace: where use would be now, spent evenly across the window.
  **`╎`,** in `viz.reserve`, is an account's cap. Neither is a forecast: no view draws where use is
  heading, and the one warning, `out ~16:05`, shows only where the recent rate runs a window out
  before its reset.
- **An account's cap** is where its reserve starts, 1 less its `reserve`: the dashboard and `usage`
  call it the cap, the config the reserve (see Reserves). `state.destructive` is kept for Claude's
  own limits, and for a token refused; a cap is movable, so it's `accent.attention`.
- **Percentages are coloured by level,** along the theme's ramp: its first stop below 70%, its
  second from 70%, its last from 90%.
- **Every account is numbered** by its place in the config, wherever its name heads a row, but on
  History and a phone's Compare, which number nothing: the number that picks it in the routing card.
  `◆` marks the primary, and `▲` the account new sessions go to now.
- **The pool** is the accounts new sessions can go to now: every account on auto, only those the
  global pin names when pinned. `Σ pool` totals them on the Overview, in `usage` and on Runway;
  `all accounts`, on Accounts and History, is every account, whatever is pinned. The two count room
  apart: the pool to each account's cap, the room new sessions can take, and `all accounts` to each
  account's limit.
- **A session is named by the first 4 characters of its id,** and, where switchboard knows it, the
  directory it was started in. A request of it in flight is `asking`, until its answer's first
  byte, then `answering`.
- **Times** read `16:30` for today and `Sun 21:00` for another day, `~` before one projected.

### The page

Every tab, and a session's page, is drawn in one frame, so the eye learns where to look:

| Row | Holds |
|---|---|
| 0 | The title row |
| 2 | The status line |
| 4 | The heading row; on the Overview, its first section |
| 5 | Blank |
| 6 to the last but two | The view |
| The last but one | Blank, or the view's state line |
| The last | The key line |

On a phone, under 100 columns, the tabs take row 1 of their own, the status line row 3 and the
heading row 5, the view starting at row 7, or at row 8 where a period takes row 6. **The breathing
room,** the blank row under the heading row and the row over the key line, never goes as rows run
short: the view gives up its rows instead, and scrolls within those it keeps where it must. The row
over the key line holds a state line only where a view has one, as the Log's following or paused,
the List's `today` line, or, while a view scrolls, what's out of view; else it's blank
(settled in the plan). A terminal too short for the fixed rows and three of the view's draws the
title row, the status line and the key line alone (settled in the plan).

**An account's name is white** wherever it's named, in `text.primary` unless a view gives it
another colour, and bold where it heads a row or leads a line, so it reads as an account and not a
word.

**Marks.** One mark a kind is the rule for events, in LATELY and the Log (see The Log). State words,
as `✕ no token` and `! can't read it`, and scroll lines, as `▲ 2 more accounts` in `text.subtle`,
have glyphs of their own, told apart from an event's mark by their colour and their words.

**The title row** (row 0):

- **The badge,** ` SWITCHBOARD ` with a space each side, in bold `canvas` on `accent.primary`, at
  column 0.
- **The tabs,** from two cells after it, a cell apart: ` Overview `, ` Accounts `, ` History `,
  ` Sessions `, ` Runway `, ` Log `. The one shown is bold `text.primary` on `bg.selection`; the
  rest are `text.subtle`. Two cells after the last, `tab ⇥` in `text.faint`.
- **The clock,** ending at the last column: the date, `Thu 1 Oct  `, in `text.muted`, then the time,
  `14:42:07`, in bold `text.secondary`, ticking each second.
- **On a phone,** the clock stays on row 0, and the tabs take row 1 from column 0 with no space
  between them and no `tab ⇥`: the six fill 52 columns exactly.

**The status line** (row 2) says how the router stands, and always shows, whatever a view hides, so
the mode is never out of sight. Its parts, each after `  ·  ` (two spaces, `·` and two spaces, in
`text.muted`):

1. **The router's state:** `●` in `state.positive` and `healthy` in `text.secondary`; or one of the
   states below.
2. **A restart due,** while the router has one: `restart due (config changed)` in
   `accent.attention`, the reason as the status document's `restart` gives it (`config changed`,
   `upgraded`, `time zone changed`) (settled in the plan).
3. **The mode:** `auto` in `text.secondary`; or with a global pin, `pinned to personal` in bold
   `accent.attention`, its accounts in the config's order (`pinned to personal and side`, `pinned to
   personal, side and work`).
4. **The sessions** the router has routed in the last hour: `6 sessions`, `1 session`, in
   `text.muted`, and `no sessions` in the count's place with none.
5. **Where new sessions go:** `new sessions → ` in `text.muted`, then `▲ side` in bold
   `accent.mode`; `no account has room` in bold `state.destructive` where none has; `nothing read
   yet` in `text.subtle` before anything has been read (settled in the plan).

The router's other states take the first part's place:

- **Unhealthy:** `● unhealthy` in `state.destructive`; the rest as its document has them
  (settled in the plan).
- **Not answering,** while its last document stays on screen (see Live updates): `■` in
  `state.destructive`, `the router isn't answering` in `text.secondary`, then `  ·  ` and `since
  14:40:55` in `text.muted`; the rest as that document had them, the mode above all.
- **Probed,** the router not running: `○ probed · router not running` in `text.subtle`; where
  something answered its socket but not as a router does, the status document's `fallback` says so
  in its place, `○ probed · router unhealthy: no answer within 500ms`; and probed as asked, with
  `--probe`, `○ probed, as asked`. A probed line has no mode, sessions or `new sessions →`, which
  are the router's (settled in the plan).

On a phone, one space either side of each `·`, and `new → ` for `new sessions → `. A line still too
long leaves out the sessions, then where new sessions go; the state and the mode always show, the
router's states shortened to `■ no router since 14:40` and `○ probed` (settled in the plan).

**The heading row** (row 4; a phone's row 5) is the tab's own, under the status line, on every tab
but the Overview, whose sections head themselves (see The Overview); a session's page has its own
heading there, ending in `esc back` (see A session's page). From column 0:

- **The tab's name** in capitals, `ACCOUNTS`, `HISTORY`, `SESSIONS`, `RUNWAY`, `LOG`, in
  `text.subtle`.
- **Its sub-tabs,** where it has them: three spaces, then each as ` Compare `, the one shown in bold
  `text.primary` on `bg.subtle`, the rest in `text.subtle`; then a space and `[ ]` in `text.faint`.
  Accounts has Compare and Detail; History, Year, Weeks, Days and Tokens; Sessions, Switchboard and
  List; the Log, Requests and Events.
- **State the screen can't otherwise show,** and only that: three spaces, then the state in
  `text.faint`, a key within it, as Compare's `←→` or History's `v`, in bold `accent.key` as
  everywhere, as the accounts Compare shows of more than fit, a filter on History or the Log, or
  Runway's day stepped back (`Wed 30 Sep`). No description follows a name: the tab and the sub-tab
  name the view, and `?` explains it.
- **A rule** of `─` in `border`, from two cells after `[ ]`, or after the name where there are no
  sub-tabs, or one after the state, to two cells short of what sits at the right, or to the last
  column where nothing does.
- **At the right,** where the tab has one: its period or window between its keys, `<` and `>` in
  bold `accent.key`, the period in `text.muted` with a space either side, `< this week >`,
  `< 5-hour >`; or the Log's count and its `f` (see The Log).

On a phone, as every phone frame draws it, a tab with sub-tabs has no `[ ]` and no rule; a period
goes to row 6, right-aligned, the view then starting at row 8; and Runway keeps its rule and its
window on row 5.

**The key line** (the last row) lists the keys that work where they are, each key in bold
`accent.key` and its word in `text.muted`: `tab views` first, then the page's own keys in groups,
then `p pin` where a pin can be set (see Routing by hand), and `? help  q quit` last. Most pages
space it as the Log does, three spaces after `tab views`, two between the keys of a group and four
after it; each page's section gives its line exactly, the Overview's with its toggles (see The
Overview).

- `t`, the theme picker, and `R`, refresh, work on every page and are never on the key line: `?`
  lists them.
- `[ ]`'s word is `view` everywhere; `enter`'s word says what it does on the page, as `enter its
  page` or `enter open`.
- A key that can't act now is drawn in `text.faint`, its key not bold, as `p pin` is while the
  router doesn't answer; pressed, it says why in the key line's place, for 4 seconds
  (settled in the plan).
- **On a phone,** one rule for every page: `? keys` and `q quit` always end the line; `tab views`
  leads it while it fits, and is the first to go when the line is full; between them, the keys the
  page chooses, three spaces apart. The global keys, `p`, `t` and `R`, are never on a phone's line,
  as `? keys` lists them. The Overview says `? keys and toggles`, as its toggles are listed there.

**A session's page opens over a tab, and `esc` goes back.** It opens with `enter` on a session's
row on Sessions, in either view, and on Accounts' Detail; with `s` on a line of the Log; and with a
click on a row of the Overview's SESSIONS. The tab it was opened from stays lit, and `esc` goes
back to it as it was left: its sub-tab, its period, the row it had chosen and its scroll, the Log
paused at the line it was on (settled in the plan). `tab` and `shift-tab` leave the page for the tab
after or before that one, and drop it (settled in the plan).

**A card over a page.** The routing card, help, and the Log's details and its `f` panel are drawn as
one card over whatever page shows, the page faded behind it (see Routing by hand, "The card"). One
is open at a time; while it is, its keys alone act, and `esc` closes it before anything else. The
theme picker is a slide-over at the right instead, so its themes preview on the page (see Themes).

**`?` opens the page's help** in the card, 128 cells wide and centred, titled `HELP · <PAGE>` as the
page is named, `HELP · OVERVIEW`, or `HELP · SESSION` on a session's page. It lists first every key
that works on the page, those the key line leaves out among them (`t`, `R`, `j`/`k`, PgUp, PgDn and
the wheel), worded as the key line words them; then what the page's rules explain, part by part in
the page's order: what each part shows and how to read it, the scale of a chart or a trace, and a
key to its marks and glyphs (settled in the plan). It's as tall as its content, up to the page's
height less two rows, and past that it scrolls within, with `j`/`k`. `?` or `esc` closes it. Each
page's section says what its help explains. The screen says only state; a page that needs its help
to be read is redesigned, not explained.

**Clicks.** The mouse is on while the dashboard runs (settled in the plan), so the terminal's own
selection takes its modifier: Option in iTerm2 and Terminal.app, Shift in most others. A click does
what a key does, where that changes only what's shown (settled in the plan, but for a row of the
Overview's SESSIONS):

- on a tab or a sub-tab, it shows it; on `<` or `>` by a period or a window, it steps it;
- on a key of the key line, it acts as that key does, but for `p pin` and `q quit`;
- on a row that names an account, a session, a line or a turn, or a Compare column's head, it
  chooses it, as the arrows would, and on the one already chosen, opens it, as `enter` does; a row
  of the Overview's SESSIONS opens its session's page at once, as the Overview's `↑↓` move the
  account focus;
- on the Overview, on a section's heading or its key, it hides the section; on an account's row in
  USAGE or ROUTING, it moves the focus there;
- on the Log's `▼ 214 new`, it follows again, and on a tick in its `f` panel, it ticks it;
- the wheel scrolls what's under the pointer, where that scrolls.

Routing takes keys alone: no click opens the routing card or acts in it, so a stray click never
moves a session.

**What's remembered.** The preferences file keeps, as they change: the theme or the pair; the tab
shown, which the dashboard opens on; the Overview's sections shown and its `b`; of each view, a
view at a time, its sub-tab, its period or window and `v`'s choice; and the Log's columns. It never
keeps a focus, a choice, a scroll, a card or help open, a session's page, a day Runway has stepped
back to, or the Log's filters, so the Log opens on everything (settled in the plan). `[usage] by` in
the config sets the default for `usage` and the dashboard alike; once `b` is pressed, the dashboard
keeps its own.

### `usage`, the printout

`usage` prints a compact, dated snapshot of every account's windows and exits, to stay in the
scrollback and be compared by eye with one printed an hour before. Its frames are in the Paper file
*Switchboard · usage*, on its page **FINAL · usage · signed off 4 Oct 2026 · revised 6 Oct**; its
references are `testdata/vhs/reference/usage-160` (the matrix), `usage-120`, `usage-80` and
`usage-52` (the windows stacked), `usage-5h-160` and `usage-5h-52` (`--5h`),
`usage-by-account-160` and `usage-by-account-52` (`--by account`), and `usage-one-account-160` (one
account). Each reference starts at the header and ends at the key line. The Overview's USAGE draws
the same block (see The Overview).

- **The windows:** `--5h`, `--week`, and one flag for each model's own window switchboard knows,
  named for its model (`--fable`), each print the header, that window alone in either layout, and
  the key line. Given together, they print the windows given, in Claude's order
  (settled in the plan). A flag for a window no account reports fails, naming the windows there are
  (settled in the plan).
- **The layout:** `--by window` or `--by account`. `[usage] by` in the config sets the default for
  `usage` and the dashboard alike, `window` unless set (see Config); the flag beats it.
- **Where it reads,** as `status` does: the router's status document while the router answers,
  else every account probed, `--probe` probing regardless, and `-r`, `--refresh` having the router
  read every account it may first (see Commands). Off a terminal, or with `--json`, it prints the
  status document instead, as every data verb prints its data (see The data verbs).
- **`setup` ends by printing it.**

The layout follows the terminal's width, `w` columns:

| What | Rule |
|---|---|
| The header on one line | `w ≥ 120`; below that, two |
| By window, the matrix | `w ≥ 150`; below that, the windows stacked |
| The name column | Fitted to the names: at least 14 cells, or 12 under 80 columns (see "The pool, the names and their numbers", below) |
| The words | The full form, 22 cells, from `w ≥ 70`; below that, the short form, 14 cells |
| By account, the name beside its first bar | `w ≥ 110`; below that, on a line of its own |
| The key line | One line at `w ≥ 150`; below that, two |

**The header:**

- **The badge,** ` SWITCHBOARD ` with a space each side, in bold `canvas` on `accent.primary`; two
  spaces; then the date and time it read at, `Thu 1 Oct  14:42`, in bold `text.primary`: the
  weekday, the day and the month, two spaces, then the time on the 24-hour clock.
- **The status** is the dashboard's status line (see The page),
  `● healthy  ·  6 sessions  ·  new sessions → ▲ side`, each part as it gives it, its states for an
  unhealthy router, a restart due and a reading probed among them. Its mode shows only while the
  router has a global pin, `pinned to personal`: on auto it's left out (settled in the plan).
- **Placement:** on one line, the status ends at the last column. On two, the status starts at
  column 0, with a blank line between it and the badge. Where it doesn't fit the width, it shortens
  as a phone's status line does.

**By window, the matrix** (`w ≥ 150`):

- **Headings,** in `text.subtle`, not bold: `5-HOUR`, `WEEK`, then each model's own window as its
  model's name in capitals and ` WEEK`, as `FABLE WEEK`; each over its group's first column, with a
  blank line under them.
- **A row an account,** in the config's order, with a blank line under each, then the pool, last,
  under its rule. The account's number and name are at column 0.
- **Each window is a group** at column `n + i·g`, where `n` is the name column and `g` is the
  bar's width and 32. In a group: the bar; a cell; the percentage, in 4 cells; two cells; the words,
  in 22 cells; then 3 cells before the next group.
- **The bars** are `min(90, ⌊(w − 14 − 32·k + 3) / k⌋) − ⌈(n − 14) / k⌉` cells for `k` windows:
  at 160 columns with the sample's 16-cell name column, 16 cells with three windows and 88 with one.
  Where that would be under 12 cells, as with four windows at 160 columns, the windows stack, as
  under 150 columns (settled in the plan).

**By window, stacked** (`w < 150`):

- **Each window is a section:** its heading at column 0, in `text.subtle`; a blank line; then a row
  an account with a blank line under each, and the pool last, under its rule. Between sections
  there's another blank line, so two separate a section's last row from the next heading.
- **A row:** the account's number and name at column 1, a cell in from the heading; the bar from the
  name column, `w − n − 8 − d` cells wide for words `d` cells wide: 74 at 120 columns, 34 at 80 and
  14 at 52, the name column at 16. Then a cell, the percentage in 4 cells, two cells, and the words,
  which end a cell before the terminal's edge.

**By account** (`--by account`), one layout at every width:

- **A block an account,** its windows as rows. Two blank lines separate the header from the first
  block, and each block from the next; a blank line separates a block's rows.
- **Wide** (`w ≥ 110`): the account's number and name at column 0, on its first window's row; the
  window's label at the name column, 10 cells; the bar 10 cells on, `w − n − 10 − 8 − d` wide: 104
  at 160.
- **Narrow:** the number and name on a line of their own at column 0, directly above the first
  window's row; the labels at column 1, 11 cells; the bars from column 12, `w − 1 − 11 − 8 − d`
  wide: 18 at 52.
- **After each bar,** as in the other layouts: a cell, the percentage, two cells and the words.
- **The labels,** in `text.subtle`: `5-hour`, `Week`, then each model's own window as its model's
  name and ` wk`, as `Fable wk`.
- **The pool's block** comes last, its rule in the second blank line above it.

**The key line** says what the marks mean:

- `┃` in `viz.pace`, as the bars draw it, then ` even pace: where you would be now, used evenly` in
  `text.muted`;
- where any account has a cap, four spaces, `╎` in `viz.reserve`, then ` cap: work at 95%` in
  `text.muted`; several capped accounts in the config's order, `work at 95%, side at 90%`.
- From `w ≥ 150`, one line starting at the name column; narrower, two lines at column 0, the cap's
  part on the second. A blank line separates it from the last row.

**Spacing:** `usage` prints a blank line, then the header, and ends at the key line, with no blank
line after it, as a shell's prompt spaces itself (settled in the plan).

**A row:**

- **The name:** the account's number, its place in the config, in `text.subtle` with a space after
  it; its label, its id unless the config gives another, in bold `text.primary`; then its marks,
  each after a space: `◆` in `text.tertiary` on the primary, and `▲` in `accent.mode` on the account
  new sessions go to. With one account, that account carries `▲`, and the header names it.
- **The bar:**
  - **Its track:** spaces on `viz.track`, a cell's background, `bg.subtle` unless the theme sets it:
    solid, never `░`, but in the `terminal` theme, which blends nothing.
  - **Its fill:** `e = round(used × bar × 8)` eighths. The first `⌊e / 8⌋` cells are spaces on the
    ramp's colour at that cell; where `e mod 8` isn't 0, the next cell is the eighth block
    (`▏▎▍▌▋▊▉`) in its cell's ramp colour, on the track. Use above 100% fills the bar.
  - **The ramp's colour** of cell `i` is the theme's ramp (`viz.ramp.1` to `viz.ramp.4`) at
    `t = round(8 · i / (bar − 1)) / 8`: nine steps, blended between the stops.
  - **The cap's mark,** `╎` in `viz.reserve`, `accent.attention` unless the theme sets it, at cell
    `round(cap × bar)`, where the cap is 1 less the account's `reserve`, if that cell is on the bar.
    It sits on the cell's own background, fill or track, with no tint after it.
  - **Even pace's mark,** `┃` in bold `viz.pace`, `text.primary` unless the theme sets it, at cell
    `min(round(pace × bar), bar − 1)`, drawn last, over the fill, the track or the cap's mark. Its
    cell has no background, so the terminal's own shows either side of the stroke: the mark reads as
    raised, and stays visible on the ramp's pale yellow.
  - **Rounding** halves to even throughout, as the references are drawn: at 90 cells, 95% is cell
    85.5, drawn at 86.
- **The percentage:** `round(used × 100)` and `%`, right-aligned in 4 cells, in bold, coloured by
  its level: the ramp's last stop (`viz.ramp.4`) from 90%, its second (`viz.ramp.2`) from 70%, and
  its first (`viz.ramp.1`) below, each judged on the percentage shown, so the colour always agrees
  with it (settled in the plan).
- **The words,** the first of these that holds; the full form from 70 columns, the short below:

  | When | Full | Short | Colour |
  |---|---|---|---|
  | The account has no usable token (settled in the plan) | `no token` | `no token` | `state.destructive` |
  | Nothing has been read of the account (settled in the plan) | `not read yet` | `not read` | `text.subtle` |
  | Its token is refused, or its last read failed: the last reading, faded (below) | `not read since 14:20` | `read 14:20` | `text.subtle` |
  | Claude's limit holds the window: it reads spent, or the router's limit names it, or names no window | `limit · back 15:54` | `back 15:54` | `state.destructive` |
  | Its use has reached the cap, which holds the account back | `at cap · back 17:10` | `back 17:10` | `accent.attention` |
  | It runs out before its reset at its recent rate | `out ~Sat 06:00`, `out ~16:05` | `out ~Sat`, `out ~16:05` | `accent.attention` |
  | A 5-hour window not started (settled in the plan) | `not started` | `not started` | `text.muted` |
  | The reset is less than an hour away for the 5-hour window, or a day for a week (settled in the plan) | `34% till 16:30` | `34% · 16:30` | as the allowance |
  | Otherwise, the allowance | `19%/day till Sun 21:00`, `34%/h till 16:30` | `19%/day · Sun`, `34%/h · 16:30` | the rate and the time in `text.muted`; ` till ` and ` · ` in `text.subtle` |

  A time reads `16:30` for today and `Sun 21:00` for another day; the short form keeps a time today,
  and only the weekday for another day. `back` is when room returns: when a limit lifts, or the
  capped window resets. Red is for Claude's own limit; a cap is movable, so it's amber.
- **A reading that's stale,** of an account whose token is refused or whose last read failed, shows
  as it was last read, faded: its bar, fill, track and marks keep 35% of their colour, which is 65%
  toward the canvas, nothing bold; its percentage and words in `text.subtle`.
- **An account with no usable token, or nothing read,** has its bars empty, no marks, and its
  percentages `—` in `text.faint`.

**How the numbers are worked out:**

- **Used:** the window's utilization, from 0 to 1. A window that has reset since it was read reads
  0, and a 5-hour window that has lapsed reads 0 until a request starts it (see Priming).
- **Even pace:** how much of the window has passed, (now − its start) ÷ its length, its start being
  its reset less its length, or its `restarted_at` for a window reset by hand. Where its length or
  reset isn't known, as of a 5-hour window not started, it has no mark.
- **The allowance:** the room left, to the cap where the cap holds the account back and to 100%
  otherwise, divided by the time to the reset: an hour's for the 5-hour window and a day's for a
  week, to a whole percent. Where the reset is less than that hour or day away, the allowance would
  be more than the room left (34% over six hours is `136%/day`), so the room itself shows,
  `34% till 16:30`. A window that has reset since it was read counts from full: all its room, over
  the time to its next reset, a length on. The allowance shows from the window's first minute
  (settled in the plan).
- **A global pin spends the caps** of the accounts it names (see Pinning): theirs count to 100%, as
  an account without a cap's do, and never read `at cap`; the cap's mark stays on their bars
  (settled in the plan).
- **Running out** goes by the window's projection. It goes at the pace its use since it started
  sets, once 5% of it has passed; or, from the router, at its recent rate, its rise over the last 30
  minutes (see Choosing an account), where that has it run out sooner or end more used. So a week at
  99%, on pace since it started to run out at 17:42 but used at 7% an hour lately, runs out at
  17:25, never later than its use lately says, and eases back as use slows. The 5-hour window, whose
  rate the router judges pressure by, goes at that recent rate whenever there is one, so `usage`
  shows where the router takes it to be heading. A probed reading has no recent rate, so it goes at
  the pace since its start, and still warns (settled in the plan). The projection goes from when the
  status document was built, its `generated_at`, so it stays put while a document stays on screen;
  countdowns count from now. A window runs out where its use reaches the cap, where the cap holds
  the account back, else its limit, and `out ~…` shows only where that comes before the window
  resets. The router's pressure goes by the same projection. Nothing draws it: no extension of a
  bar, no `→ 54%`.
- **`status` projects the same way,** its text keeping its words until it's designed afresh:
  `runs out ~Mon 17:25 at its rate over the last 30 min`, `over the last 18 min` for a window with
  no level from before the half hour, or `over the last 2h` across a gap in its readings; and its
  pressure line adds the rate it goes by and the reset it runs out before,
  `under pressure: runs out ~18:21 at Session's rate over the last 30 min, before its reset at
  20:10`, or where its reserve would hold it back, `under pressure: at its reserve ~18:21 at …`.
- **A window reset by hand** before its reset time, as claude.ai's banked reset does, dropping its
  use but keeping its reset (see Observed), has effectively started again: the router reads it with
  the same reset, taken as current (see How it works), fallen by a tenth of the window or more, and
  notes when as the window's start. A smaller dip, as a 429 reading a point below the use read just
  before, is noise: the reading stands, as the upstream's latest word, and the window runs on. Its
  even pace and its projection measure from its start again, rather than from a whole length before
  its reset, until its next reset, a later reset being a new window; otherwise a week reset at the
  end of its third day would have its even pace about three-sevenths of the way along, and be on
  pace for 0%. Once the router has read a window reset by hand, the answer to a request sent before
  the one whose answer showed it is from before the reset, and is passed over, where use only rising
  within a window would have it put back the use the reset took away, and so is the limit a 429 to
  it reaches in the window (see Choosing an account, step 6); the router keeps which request that
  was in memory alone, as the state file's readings count as read before any.
- **At cap:** the window's use has reached the cap, and the cap holds the account back; `back` is
  the window's reset, when room returns.
- **The pool,** wherever there's more than one account, totals the room new sessions can go to now:
  every account on auto, only those the global pin names when pinned. Each window's room is summed
  to each account's cap, `cap − used` and never below 0, with nothing from the 5-hour window of an
  account at its limit or its cap, nor from any window of an account refused or not read. Its bar
  shows `1 − room ÷ the accounts in the pool`, coloured as every bar is, with no cap's mark; its
  even pace is the mean of its accounts'; its percentage is its bar's. Its words are the room, to
  one decimal place: `1.5` in bold `text.secondary`, then ` of 3 left` in `text.muted`; pinned, the
  first window's words add ` · pinned`, in `text.muted`, and the smaller pool shows in `of 2`. The
  status document gives it (see The status document). With one account there's no pool: it would
  repeat the account.
- **Every window shows, used or not:** a model's own window, as Fable's week, on every account, at
  0% as at 40%, as probes read every model family's window before any use. Windows come in Claude's
  order: the 5-hour window, the week, then each model's week. An account that hasn't reported a
  window the others have shows it as nothing read.

**The pool, the names and their numbers:**

- **The pool is last,** after the accounts, in every layout: the matrix, each stacked window,
  `--5h`, and `--by account`, its block last.
- **One unbroken rule,** `─` in `border` across the whole row above it, label and all, in the blank
  line that would sit there anyway, so no layout changes size.
- **Its name is `Σ pool`:** `Σ` in `text.subtle`, in the numbers' column, and `pool` in
  `text.secondary`, not bold.
- **Every account is numbered,** wherever its name heads a row: its place in the config's order,
  which is its key in the routing card. The number is in `text.subtle`, `text.faint` where the
  account is out of the pool, right-aligned once there are ten, from ` 1` to `10`, `Σ` in the same
  column. With one account it's still `1`, so no layout changes with the number of accounts.
- **The name column fits the names:** the widest number, label and marks, plus two cells kept for
  `▲` on any account without it, as `▲` moves between accounts as routing does and no label may move
  with it, plus a gap of two cells; at least 14 cells, or 12 under 80 columns, and at most 24, or 16
  under 80. The bars give up the cells. One column serves every section that names accounts.
- **A longer label is cut** with `…` to fit, its number and marks kept, and the cells for `▲` kept
  too, so a label never changes when `▲` moves: `2 someone.else@exam… ◆`. Labels are free text, and
  an email address is the common long one.

**Colour.** The printout is drawn in the theme's colours and brought down to what the terminal shows
by `colorprofile`. It paints no background, and takes the light or dark half of the theme pair as
the terminal's background says (see Themes). With `NO_COLOR` set, or on a terminal that shows no
colour, a bar's fill is drawn in `█` and the eighth blocks in the terminal's own colour, its track
as spaces and its marks as they are; the names and percentages keep their bold
(settled in the plan).

### The Overview

The dashboard's default tab: every account's usage, read in combination with the router, at a
glance, live. Its frames are in the Paper file *Switchboard · Overview*, on its page **FINAL ·
dashboard overview · signed off 5 Oct 2026 · revised 6 Oct**; its references are
`testdata/vhs/reference/overview-*`: `overview-default-160` (on auto), `overview-pinned-160`,
`overview-bad-hour-160` (a limit, a refusal, a stale account and a pin of two accounts),
`overview-by-account-160`, `overview-usage-alone-160`, `overview-everything-160x39` (sections
folded), `overview-six-accounts-160`, `overview-ten-accounts-160`, `overview-default-216`,
`overview-sessions-216`, `overview-sessions-graphs-216`, `overview-everything-216x106` and
`overview-phone-52`. Frames 1 to 12 draw the key line as it was before the keys were swept (`0 auto
1-3 pin  m move`), and frames 2 and 3 ROUTING's `0 back to auto`. This section holds over them, with
`p pin` and `p back to auto`.

**Size never decides what shows.** Every section is a toggle, at every size, and the toggles are
remembered; size decides how much room each section shown gets. The page is one screen that never
scrolls: when rows run short, sections fold to their headings, and only USAGE scrolls, within
itself, when the accounts are many. Its sections, from row 4 (a phone's row 5), in an order that
never changes, a section hidden taking no room:

| Section | Key | Shown at first | What it is |
|---|---|---|---|
| USAGE | `u` | yes | Every account's windows, `usage`'s block |
| GRAPHS | `g` | no | The focused account's use so far in each window, under USAGE's columns |
| ROUTING | `r` | yes | How each account stands with the router, and its requests over the last 30 minutes |
| SESSIONS | `s` | no | The sessions running, by account |
| COMING UP | `c` | yes | What happens next, soonest first |
| LATELY | `l` | yes | What's happened, newest first, beside COMING UP |

A blank row separates two sections, but none separates two folded ones; a blank row comes before
the key line, which is the last row.

**A section's heading:** its name in `text.subtle`; three spaces and its description, where it has
one; a space, a rule of `─` in `border`, and a space; then the key that hides it, in bold
`accent.key`, in the section's last column. With no description, the rule starts two cells after
the name. Below the heading, a blank row, then the body. A description says only what the screen
can't, state or a key to marks; what a section is, and why it's where it is, are for `?` help:

| Section | Description |
|---|---|
| USAGE | None, as the pool's row says the room; with one account, which has no pool, the room left: `room left  ` in `text.faint`, `5h ` in `text.subtle`, `0.4` in bold `text.secondary`, ` of 1` in `text.subtle`, `  ·  ` in `text.faint`, then `week 0.6 of 1` the same way, summed as the pool's room is; on a phone, none |
| GRAPHS | The focused account, `work`, in bold `text.primary`; five spaces; then the key: `·` in `text.subtle` and ` even pace   `, `╌` in `viz.reserve` and ` cap   `, `┄` and ` limit`, the rest in `text.faint` |
| ROUTING | The mode (below) |
| SESSIONS, COMING UP, LATELY | None |

**USAGE** is `usage`'s block, exactly as `usage`, the printout gives it, but for its header: the
title row and the status line say what the header would. By window:

- **Two sizes from 150 columns:** compact, the matrix; and big, the windows stacked as sections at
  the full width, the bars from the name column and `w − n − 30` cells wide: 114 at 160 columns with
  the sample's 16-cell name column, 170 at 216. Room chooses which, as "Room", below, says. The
  matrix's bars follow `usage`'s rule: 16 cells for three windows at 160 columns, 35 at 216.
- **Under 150 columns,** `usage`'s stacked layout at that width; on a phone, its short words, its
  bars 14 cells beside a 16-cell name column.
- **By account,** as "By account", below, gives it.

Then, as only the Overview has them:

- **The focused account,** while GRAPHS shows, has the first 12 cells of its number and name on
  `bg.selection` in the matrix.
- **An account out of the pool,** where the global pin names others, is faint wherever the page
  names it, rather than struck through, as not every terminal draws strikethrough: its number, name
  and marks in `text.faint`; in USAGE, its bars faded as a stale reading's are, keeping 35% of their
  colour, which is 65% toward the canvas, and its percentages and words in `text.faint`; in ROUTING,
  its trace faded the same way and its rate in `text.faint`, its state's words as they are, being
  the account's own. A stale account out of the pool has the two fades compound, 35% of 35%, so it
  keeps 12% of its colour, and its text is `text.faint`, which wins over any colour the fade would
  leave.
- **The key line names the marks the accounts carry,** after the cap's: `◆` in `text.tertiary` and
  ` primary`, and `▲` in `accent.mode` and ` new sessions go here`, each four spaces after the part
  before, in `text.muted`; on a phone, three spaces, and `▲ new`.
- **Many accounts:** past what fits, USAGE scrolls within its section, its window headings and key
  line held, with `▼ 4 more accounts` in `text.subtle` under the last row shown, and `▲ 2 more
  accounts` over the first while some are above. `j`/`k`, PgUp, PgDn and the wheel scroll it, and
  moving the focus to an account out of view scrolls to it (settled in the plan).

**GRAPHS** draws the focused account's use so far, a chart a window, each under its window's
column in USAGE. `↑` and `↓` move the focus between the accounts, in the config's order.

- **Placement:** from the name column, a chart every `gw = ⌊(w − 14 + 3) / k⌋ − ⌈(n − 14) / k⌉`
  columns for `k` windows, each `gw − 3` wide: 45 cells every 48 at 160 columns, 64 every 67 at 216.
- **Its rows:** under the heading's blank row, the charts' top row: each chart's window, `5-HOUR`,
  `WEEK`, `FABLE WEEK`, in `text.subtle` and a space, then the limit's line along the rest of the
  chart's width, `┄` in `text.faint`; then the chart's `h` rows; then its axis row. GRAPHS takes
  `h + 4` rows, `h` from 5 to 19, as "Room", below, gives them; taller distorts.
- **A column a cell,** over the window's whole length, from its start to its reset; now's column is
  `min(round(pace × width), width − 1)`. Each column to now is filled to the use read at its
  middle, or at now for now's column, and none past now: `round(use × h × 8)` eighths, its full
  cells as background, its top cell the eighth block of its height (`▁`–`▇`) on the canvas. A row is
  coloured by its height, the ramp at `(r + 0.5) / h` for row `r` from the floor, mixed with the
  canvas so 70% of it shows.
- **Its lines,** each on empty cells alone, never over the fill, and where they meet in a cell, the
  cap's `╌` first, then the even pace's `·`, then now's `│`:
  - **the cap,** where the account has one: a row of `╌` in `viz.reserve`, `accent.attention` unless
    the theme sets it, `min(⌊cap × h⌋, h − 1)` rows up from the floor;
  - **even pace:** a `·` in `text.subtle` in each column `i`, `min(⌊i / (width − 1) × h⌋, h − 1)`
    rows up, from the bottom left to the top right, past now to the reset: a line to measure by,
    not a forecast (settled in the plan);
  - **now:** `│` in `text.faint` down now's column.
- **The axis row:** the window's start, `12:10` or `Sun 21:00`, in `text.faint` at the left; `now`
  in bold `text.subtle`, from the cell before now's column; the reset in `text.faint`, ending at the
  chart's right.
- **Its data** is the readings history, as `GET /history` gives it with each full read, each column
  the last reading at or before its time; with no router, the readings history's files and the
  readings the dashboard takes as it probes (settled in the plan; see Live updates).
- **By account,** GRAPHS draws as by window, its charts side by side under USAGE's blocks. **On a
  phone,** its charts stack, a window's under another's, each from the name column to the edge
  (settled in the plan).

**ROUTING** says how each account stands with the router, and how busy it's been.

- **The mode, as the heading's description:** `auto` in bold `text.secondary`; or `pinned to
  personal` in bold `accent.attention`, ` since 14:02` in `text.subtle`, three spaces, then `p` in
  bold `accent.key` and ` back to auto` in `text.faint`; with several accounts pinned, `pinned to
  personal and side since 13:40   p back to auto`. By account and on a phone, the mode alone. How
  each mode routes is for `?` help.
- **A row an account,** in the config's order, from 110 columns:
  - its number, name and marks at column 0, as in USAGE;
  - its state at the name column, its glyph coloured as its words (below), its words cut by whole
    clauses from the end so they end two cells before `● pinned` at column 68, or before the
    sessions at 79 where no pin shows, a name column wider than 16 moving their start and cutting
    them sooner, as Sessions' lines cut theirs (see Sessions);
  - `● pinned` in `accent.key` at column 68, on each account the global pin names;
  - its sessions at column 79: `2 sessions` or `1 session` in `text.muted`, `0 sessions` in
    `text.faint`;
  - its trace from column 92 to `w − 8`, then its rate right-aligned at the last column, `15/min` in
    bold `text.secondary`, or `0/min` in `text.subtle`.
- **Size 1:** a row an account, none between them, and under the last, at column 92, `requests a
  minute, last 30 minutes` in `text.faint`. **Size 2,** with room to spare: two rows of trace an
  account and a blank row, its words on the first row and its rate on the second, the caption on the
  blank row after the last account.
- **Under 110 columns, and on a phone:** two rows an account and a blank row. On the first, the
  number and name at column 1, the short state at the name column, and `● pinned` right-aligned;
  on the second, the trace from the name column to `w − 20` (16 cells on a phone, the name column
  at 16), and the sessions and rate right-aligned, `4 sessions  15/min` (settled in the plan).
- **An account's state,** the first of these that holds, its glyph and first words in the colour
  given, the rest in `text.muted`; the short form on a phone:

  | State | Words | Colour | Short |
  |---|---|---|---|
  | No usable token (settled in the plan) | `✕ no token · switchboard accounts token work` | `state.destructive` | `no token` |
  | A limit holds it back from every request | `■ limit reached · back 15:54` | `state.destructive` | `limit reached · 15:54` |
  | A limit holds back some models alone (settled in the plan) | `■ Fable wk limit · back Mon 21:00 · other models still come here` | `accent.attention` | `Fable wk limit` |
  | Its token refused | `■ refused (401) · its token needs renewing` | `state.destructive` | `refused (401)` |
  | One model's requests refused alone (settled in the plan) | `■ refused (403, opus) · other models still come here` | `accent.attention` | `refused (403)` |
  | Its usage can't be read, nothing read before (settled in the plan) | `! can't read it · timed out` | `state.destructive` | `can't read it` |
  | Nothing read yet | `○ not read yet` | `text.subtle` | `not read yet` |
  | At its cap, which holds it back | `● at its cap · back 17:10 · new sessions go elsewhere` | `accent.attention` | `at its cap · 17:10` |
  | At its cap, the global pin naming it (settled in the plan) | `● at its cap · pinned, so new sessions still come here` | `accent.attention` | `at its cap · pinned` |
  | Under pressure | `● under pressure · out ~16:05` | `accent.attention` | `under pressure` |
  | Its 5-hour window lapsed (settled in the plan) | `○ idle · window starts at its prime, 16:20`, or without priming `○ idle · window starts with its next request` | `text.subtle` | `idle` |
  | Open, new sessions coming here | `● open · new sessions come here` | `●` `state.positive`, `open` `text.secondary` | `open · next` |
  | Open | `● open` | as above | `open` |

  The states are the status document's (see The status document): a limit or a refusal is said even
  where the account's last read failed. Accounts' Detail and Sessions' lines say an account's state
  in these words too.

**A trace** draws requests a minute over the last 30 minutes, a column a cell, the newest at the
right:

- **Each cell** is the mean of four samples of requests a minute over its slice of the 30 minutes:
  30 seconds a cell at 160 columns. It fills `round(min(value ÷ scale, 1) × rows × 8)` eighths of
  its 1 or 2 rows, its full cells as background and its top cell an eighth block. Where a column is
  empty, its bottom row has a `▁` in `text.faint`, so the lane always shows.
- **One scale for every trace on the page,** accounts' and sessions' alike, so any two can be
  compared: the busiest column of them all over the 30 minutes fills a trace's height, never less
  than 8 requests a minute, so a quiet page isn't magnified (settled in the plan). A session's story
  scales by the same rule over its own columns (see A session's page).
- **Its colour:** `accent.key`, mixed with the canvas so 80% of it shows.
- **Its rate,** at the right, is the requests of the last minute.
- **The requests counted** are those that spend quota and went upstream (see The request ledger),
  each once, by its id, as it's sent, on the account it went out on; Claude Code's quota checks and
  token counts never. The last 30 minutes come from today's lines once, as the Overview is first
  shown (settled in the plan), then from the request stream (see Live updates). A session moved
  between accounts carries its whole 30 minutes on its own trace.

**SESSIONS** lists the sessions running, those the router has routed in the last hour, grouped by
account in the config's order, a session whose models go to two accounts on each:

- **The account's number, name and marks** at column 0, on its first session's row alone.
- **The session's id,** its first 4 characters, at the name column: bold `text.primary` while busy,
  with a request in flight or seen in the last minute, else `text.muted`. Then two spaces, its
  model's family padded to 7 in `text.muted`, and when it was last seen, right-aligned in 4 in
  `text.subtle`: `now` within the minute, `9m`, `2h`.
- **What it's doing,** three spaces on, padded to 9: `↓ ~1.2k` while it's answering, its tokens so
  far estimated at four characters a token, or `↑ ask` while it's asking, both in `accent.key`; else
  `idle` in `text.faint`.
- **Why it's there,** in `text.subtle`: its assignment's reason, in the views' words for the
  router's reasons (see The router's events), with when: `started here 14:41`, adding `, the best`
  while its account is still where new sessions go; `moved from work 14:31: work reached its cap`;
  `moved from work 14:39: pinned to side`; `pinned here 14:39`, its own pin; `its pin to side
  yields: side has no room` (settled in the plan).
- **Its own trace** at column 92, one row, on the shared scale, and its rate right-aligned at the
  last column. No blank rows between sessions; with none running, `no sessions running` in
  `text.faint` (settled in the plan).
- **A click on a row opens its session's page** (see A session's page): `↑` and `↓` stay on the
  account focus GRAPHS follows, so no key here opens one.
- **Under 110 columns, and on a phone,** a session takes two rows: the account's number and name at
  column 1 on its first, its id at the name column and what it's doing right-aligned; under them,
  why it's there, from the name column. No trace (settled in the plan).

**COMING UP and LATELY** sit side by side: COMING UP from column 0, `round(w × 0.45)` wide, and
LATELY from column `⌊w / 2⌋ + 2` to the edge, so 72 wide and from 82 at 160 columns, and 97 wide
and from 110 at 216. Either alone takes the full width; on a phone, COMING UP comes first, then
LATELY under it, each the full width, in their short words.

- **COMING UP,** soonest first: the time, padded to the longest, as `16:30`, `~Sat 06:00` or
  `Sun 02:00`, in `text.secondary`; two spaces; the account in bold `text.primary`, then what
  happens in `text.muted`; at the section's right, how long until it, `in 1h 48m`, in
  `text.subtle`. A run-out's row is all in `accent.attention`, its time and countdown too. It lists,
  as the status document gives them (see The status document):
  - a limit lifting, `personal back from its limit`, or a model's own,
    `back from its Fable wk limit`;
  - a cap lifting, as the capped window resets, `work back from its cap`;
  - a window running out before its reset, by its projection, the 5-hour window at its recent rate
    whenever there is one (see `usage`, the printout): the 5-hour window unnamed, `work runs out at
    its pace`, another named, `personal's week runs out at its pace`, or where the cap would hold
    the account back first, `work reaches its cap`;
  - a window resetting, `personal's 5-hour resets`, `side's week resets`, a model's own week only
    where its reset isn't its week's (settled in the plan);
  - an account's next prime, `spare is primed` (settled in the plan).
- **LATELY,** newest first: the time, the newest in `text.secondary` and the rest in `text.subtle`;
  two spaces; the event's mark in its colour and a space; then the subject in bold `text.primary`,
  the words in `text.muted` and the object in bold `text.secondary`, as `14:31  ╎ work reached its
  cap: 3 sessions moved to side`. The events are the router's, the newest the status document
  gives (see The router's events), and the marks are these, which the Log's Events share
  (settled in the plan):

  | Mark | Event | As |
  |---|---|---|
  | `▲` in `accent.mode` | `started` | `c61b started on side`, and why where it wasn't the router's best: `: personal is at its limit`; under a pin, `c61b started on personal, as pinned` |
  | `▸` in `accent.key` | `moved` | `db8a moved personal → work: personal came under pressure` |
  | `◔` in `accent.attention` | `pressure` | `personal came under pressure: its 5-hour runs out ~16:05` |
  | `╎` in `accent.attention` | `cap` | `work reached its cap: 3 sessions moved to side` |
  | `■` in `state.destructive` | `limit` | `personal reached its 5-hour limit: 2 sessions moved to side` |
  | `■` in `state.destructive` | `refused` | `work refused (401): 2 sessions moved to side` |
  | `◇` in `text.subtle` | `primed` | `side primed: its 5-hour window started` |
  | `✓` in `state.positive` | `room`, room again | `work open again: its 5-hour window reset` |
  | `●` in `accent.key` | `pin` | `pin set by hand: new sessions go to personal` |
  | `↩` in `accent.key` | `auto` | `new sessions back to auto` |
  | `!` in `accent.attention` | `restart` | `restart due: the config changed` |
  | `⊕` in `state.destructive`, or `state.positive` well again | `health` | `the router turned unhealthy: <why>`; `the router is healthy again` |

  A mark is one a kind, but for `■`, which a limit and a refusal share, their words telling them
  apart; the Log's Events mark each kind the same (see The Log). The sessions a limit, a cap or a
  refusal moved fold into its line, counted, as its event's `count` and `to` give them; a move shows
  alone only where the line it's folded into isn't among those shown. On a phone the words are
  short: `work at its cap: 3 moved to side`.

**By account** (`b`) pivots the page; `[usage] by` sets its default (see The page, "What's
remembered").

- **USAGE is a block an account,** as `usage --by account` draws it at 160 columns, but with bars
  `50 − (n − 14)` cells wide: the number and name at column 0; the window labels at the name column,
  10 cells; the bars 10 cells on; the percentage a cell after a bar, and the words two cells after
  the percentage: 26, 48 cells, 75 and 81 for the sample.
- **ROUTING moves beside each block,** at column `hx = n + 10 + bar + 7 + 22 + 4`, 107 for the
  sample. USAGE's heading is `hx − 3` wide, and ROUTING's, `ROUTING   auto` with the mode alone,
  runs from `hx` to the edge. On each block's first row, its state's words, cut by whole clauses to
  end two cells before the right edge; two rows down, `● pinned  ·  ` where the pin names it, then
  its sessions; four rows down, a one-row trace from `hx` to `w − 8`, and its rate right-aligned.
- **The pool's block is last,** under one rule across USAGE and ROUTING: its windows as each
  account's, and beside them its accounts' states counted, `2 open · 1 at cap`, each count in its
  state's colour (`open` in `text.secondary`, `pressure` and `at cap` in `accent.attention`, `limit`
  and `refused` in `state.destructive`) with ` · ` in `text.faint`; its accounts' sessions summed,
  `6 sessions`; and their requests a minute summed, as a trace and a rate, `16/min`.
- **The key line** follows the last block, from the name column.
- **Under 150 columns,** USAGE draws as `usage --by account` does at that width, and ROUTING stays a
  section of its own under it (settled in the plan).

**The key line**, the last row:

- **Wide:** `tab views`, five spaces; each toggle as its key and word, two spaces apart, `u usage  g
  graphs  r routing  s sessions  c coming up  l lately`, a key in bold `accent.key` and its word in
  `text.muted` while its section shows, else `text.faint`; two spaces, `b by: window`, the state in
  `text.secondary`; five spaces; then `↑↓ focus  p pin  ? help  q quit`. With one account,
  `↑↓ focus` doesn't show (settled in the plan), nor `p pin`, unless a pin set from the command line
  stands.
- **On a phone:** `tab views   ? keys and toggles   q quit`.

**Room.** Each section has a least and a most:

| Section | Least | Most | Rows |
|---|---|---|---|
| USAGE | The compact matrix | The big bars, by window from 150 columns | Compact: `2a + 7` for `a` accounts and the pool, 7 for one account |
| GRAPHS | 5 rows of chart | 19 | `h + 4` |
| ROUTING | A row an account | Two rows of trace an account, a blank row between | `a + 3`; `3a + 2` |
| SESSIONS | A row a session | The same | Its sessions and 2 |
| COMING UP and LATELY | 3 lines | 12 | Its lines and 2 |

Rows 0 to 3 (0 to 4 on a phone) hold the title row and the status line, and the last two the blank
row and the key line. The planner:

1. Every section shown starts at its least.
2. **Rows short:** sections fold, in turn, until the page fits: SESSIONS; then COMING UP and LATELY
   together; then GRAPHS; then ROUTING. USAGE never folds; with many accounts it scrolls within the
   rows left to it.
3. **While anything is folded, nothing grows,** and the rows over stay empty at the foot.
4. **Rows to spare,** while nothing is folded, go in turn: to GRAPHS, a row at a time to 19; to
   USAGE's big bars, if they fit; to ROUTING's two rows of trace, if they fit; then to COMING UP and
   LATELY, a line at a time to 12. GRAPHS comes before the big bars, so that a taller terminal never
   shrinks anything.
5. **Past every most,** the rest is empty at the foot, the key line on the last row.
6. **The layout changes only on a key or a resize,** never on its own. SESSIONS keeps the rows it
   was given: more sessions than fit show as many as fit, the last row saying `+2 more`
   (settled in the plan).

**A folded section keeps its heading,** saying what it holds, with no blank row between two folded
ones:

- `SESSIONS   6 running: 2 on personal, 4 on side   no room at 37 rows`, the count in
  `text.muted` and `no room at 37 rows` in `text.subtle`;
- `COMING UP   16:30  personal's 5-hour resets  …` and `LATELY   14:41  ▲ c61b started on side  …`,
  side by side, each its first row as drawn, then `  …` in `text.subtle`;
- `GRAPHS   work   no room at 35 rows`;
- `ROUTING   auto   no room at 33 rows`.

**Everything hidden,** row 4 says how to bring it back: `every section is hidden:` in `text.faint`,
then the toggles as the key line words them (settled in the plan).

**`?` help**, `HELP · OVERVIEW`, lists every key first, the toggles, `b`, `↑↓`, `p`, `t`, `R` and
`j`/`k` among them; then it explains each section, what it shows and why it's where it is; USAGE's
marks, the levels' colours, the allowance and `out ~…`, the pool against `all accounts`; GRAPHS'
lines; the modes and how each routes; ROUTING's states and the traces' scale; what a session is
doing; and LATELY's marks.

### Accounts

Each account in depth, and every account together: what each is, what it does, what it costs and
whether it pays. The Overview says how each account stands now, History when the accounts were used,
and Runway when they'll have room; Accounts says who. It has two views, as sub-tabs: **Compare**, a
column an account, and **Detail**, one account, or all of them, in full. They're drawn in the Paper
file *Switchboard · Accounts*, on its page **FINAL · accounts · signed off 5 Oct 2026 · revised 6
Oct**, and kept in `testdata/vhs/reference/` as `accounts-compare-160`, `accounts-compare-ten-160`,
`accounts-compare-month-160`, `accounts-compare-bad-hour-160`, `accounts-detail-all-160`,
`accounts-detail-work-160`, `accounts-detail-limit-160`, `accounts-detail-sessions-160`,
`accounts-detail-one-160`, `accounts-phone-compare-52` and `accounts-phone-detail-52`: 160 columns
by 37, and a phone's 52. The frames show the keys from before the sweep of 7 and 8 October 2026, `p`
for the period and `0 auto  1-3 pin  m move`; where they and this section differ, this section
holds.

**Rules Accounts and History share:**

- **The heading row** is the page's (see The page): `ACCOUNTS` or `HISTORY`, the sub-tabs, and at
  the right the period between its keys, `< this week >`. The state after `[ ]` is only what the
  screen can't show otherwise, as which accounts Compare shows of more than fit, or what History's
  Year is filtered to. On a phone, the sub-tabs follow the tab's name after two spaces, each `Name `
  with a space after it, with no `[ ]` or rule, and the period goes to row 6, at the right, the view
  then starting at row 8.
- **No description after a heading,** a tab's or a group's: the tab and the sub-tab name the view,
  and what a view is, and how to read it, is for `?` help. A dim line says only what the screen
  can't: a key to an encoding, as to the shades or the dots; a rule the eye can't see, as that worth
  is priced as the API prices it, and as of when, or that a verdict is an estimate; or state.
- **Labels:** a group's heading is uppercase in `text.subtle` (`ACTIVITY`, `BY MODEL`), its items
  lowercase in `text.secondary`, two cells in (`requests`).
- **Numbers:** wherever the accounts are numbered, as in Compare's heads and Detail's list and
  panel, each account's number, its place in the config, is before its name in `text.subtle`, and
  `0` before `all accounts`: the numbers are the routing picker's keys, and `0` is auto, which
  routes to every account (see Routing by hand). History and a phone's Compare number nothing.
- **`all accounts`,** never a count, is every configured account, always, whatever is pinned, at its
  limit or refused. Its totals count them all; a window's use is the mean of theirs and its pace the
  mean of their paces; and its room, `1.2 of 3 left`, is the sum of what each has left of it, to its
  limit, where the pool's is to each account's cap (see `usage`, the printout). With one account,
  `all accounts` goes, as does anything that would only repeat that account. The pool, the accounts
  new sessions can go to now, is routing, which only the Overview shows (see The Overview): Accounts
  and History show no routing, the status line carrying it onto every page. An account's own state,
  open, at its cap, at its limit or refused, is the account's, and shows.
- **Its accounts' states counted,** wherever an account's state goes for `all accounts`:
  `2 open · 1 at cap`, `1 open · 1 limit · 1 refused`, in the order open, pressure, at cap, limit,
  refused and not read, each count in its state's colour (`open` in `text.secondary`, `pressure` and
  `at cap` in `accent.attention`, `limit` and `refused` in `state.destructive`, `not read` in
  `text.subtle`), ` · ` in `text.faint`. Too wide for its room, it's how many are open, pressure
  counting as open, `8 of 10 open`, in `text.secondary`.
- **A view never changes shape with the number of accounts.** Where a view has two shapes, `v` picks
  one; more accounts than fit scroll. Only a phone's width changes a layout.
- **Models:** tables name versions, a row a version used in the period, as the version table names
  them (`Opus 5.5`, `Sonnet 5`; see The request ledger), each `●` and bar in its shade,
  `viz.version.1`–`viz.version.6`, or, for a version without one, its family's hue, mixed 30% toward
  the canvas for an older version (see Themes). Charts keep to families, in
  `viz.family.1`–`viz.family.4`: Opus, Sonnet, Fable and Haiku. A family's colour reads at a glance
  in a chart, and a table has room for every version.
- **Worth at API prices:** what the same tokens would have cost through the API, each version at its
  own prices, at today's prices, worked out as it's read (see The request ledger). Worth part
  unpriced, of a model the price table doesn't know or of requests that went upstream without usage,
  reads `$184+`, the `+` in `text.faint`, and wholly unpriced, `unpriced`, `?` help saying why
  (settled in the plan). **Share of use** weighs tokens as worth does, so cache reads, which run to
  millions and cost little, count for little.
- **Against its price:** worth over what the plans cost for the same period, a plan's monthly price
  prorated, a day being 12/365 of a month (settled in the plan), a week 12/52 of a month, 30 days a
  month and 12 weeks twelve weeks' (settled in the plan), and a period so far priced whole. It reads
  `4.0×`, bold, in `state.positive` at 1× or more and `accent.attention` under, then ` its $46 week`
  in `text.subtle`, the price to the dollar. A plan's price is the price table's, or the config's
  (see Config). An account without a `plan` shows `—` for it and no ratio, and `all accounts` sums
  the plans it knows, `$400/mo · 2 of 3 plans`, its ratio counting only their accounts' worth
  (settled in the plan).
- **Periods are rolling,** never billing months, as switchboard knows no renewal date: today, from
  midnight; this week, from its first day, as `week_starts` sets it (see Config); 30 days, today and
  the 29 before it; and 12 weeks, this week and the 11 before it, `<` and `>` stepping through them
  in that order, Compare and Detail sharing the period (settled in the plan).
- **Keys:** `[` and `]` move between the sub-tabs, and a click on one shows it; `<` and `>` step the
  period, `<` to a shorter and `>` to a longer, or on History's Year, `<` back a year and `>`
  forward; `v` cycles a view's shape, where it has more than one. The preferences file remembers
  each choice, a view at a time (see Themes). `p` opens new sessions' routing picker, whatever is
  chosen (see Routing by hand); `t`, the theme picker, and `R`, refresh, work as on every page, and
  neither is on the key line; `?` opens help for the page shown, and `q` quits.

**Compare** sets the accounts side by side, a column each, with `all accounts` last.

- **Columns:** the labels in columns 0 to 25; then a column an account and one for `all accounts`,
  from column 26, each `colw = (w − 26) // c` wide, `c` being the columns shown. As many accounts
  show as `(w − 26) // 19 − 1`, at least one: six at 160 columns. Past them, `←` and `→` move the
  focus along the accounts, the columns scrolling to keep it in view and `all accounts` staying put
  at the right, and the heading's state says which show:
  `accounts 1–6 of 10  ·  ←→ moves along them; all accounts stays put`, its `←→` bold `accent.key`.
- **The heads,** rows 6 and 7: an account's number, then its name and marks, bold, as `usage` draws
  a name (see `usage`, the printout); under them, its state in a word, as the Overview colours its
  state: `● open`, the dot `state.positive` and the word `text.secondary`; `● pressure` and
  `● at cap`, in `accent.attention`; `■ limit` and `■ refused`, in `state.destructive`;
  `○ not read`, in `text.subtle`. `all accounts`' head is `0`, in `text.subtle`, and `all accounts`,
  bold in `text.secondary`, with its accounts' states counted under it. A long name is cut with `…`
  to the column less a cell, keeping its number and marks, as `usage` cuts a name
  (settled in the plan). No column is focused until `←` or `→`; then the focused column's heads are
  `bg.selection` across its width, less a cell, and `enter` opens that account, or `all accounts`,
  in Detail (settled in the plan).
- **WEEK USED,** row 9: each account's week as `usage` draws a window's bar, its cap and even pace
  marked (see `usage`, the printout), `min(18, colw − 8)` wide, then, a cell on, its percentage,
  right-aligned in 4 and bold in its level's colour. `all accounts`' is every account's week
  together, with no cap. A stale account's bar is faded, as the Overview fades it, keeping 35% of
  its colour, which is 65% toward the canvas. The week is always as it stands now: over another
  period, the label adds `now`, in `text.faint`.
- **BY MODEL,** row 11, `share of use` after it in `text.faint`: on its heading row, each column's
  share of use as a stacked bar, `min(24, colw − 4)` wide, each version's part in its shade; then
  from row 12 a row a version used in the period, in the version table's order, `●` in its shade and
  the version's name in `text.secondary` from column 2, and in each column its share, right-aligned
  in 4, `—` in `text.faint` for none, and `<1%` for a share that rounds to nothing
  (settled in the plan).
- **Three groups** follow, a blank row before each, every value from its column's first cell in
  `text.secondary`, `—` and `none` in `text.faint`, and `all accounts`' bold:
  - **ACTIVITY:** `requests`, `1,204`; `sessions served`, `9`, each session counted once, however
    many days and accounts it ran on, so `all accounts`' is no sum of the rest; and
    `sessions moved`, `3 out` or `4 in`, or `2 in · 1 out` for an account with both
    (settled in the plan), and for `all accounts` every move, `4 moves`.
  - **LIMITS:** `limits hit`, by window, `5-hour ×2`, its windows joined by ` · `,
    `5-hour ×2 · week ×1` (settled in the plan), in `state.destructive`, `none` without, and for
    `all accounts` how many, `3`; `at cap or limit`, the time it spent at its cap or at a limit,
    `3h 10m`, and for `all accounts` their sum; and `left at its last reset`, what its week had left
    as it last reset, `15%`, or `0% (limit Fri)` where the week ran out, naming the day it did, and
    `—` for `all accounts`.
  - **VALUE,** with no description, as its rows say it: `plan`, `Max 20x`, and ` · $200/mo` in
    `text.subtle`, and for `all accounts` the plans summed, `$600/mo`, and ` · 3 plans`;
    `extra usage`, `off` in `text.muted`, or `on · billed past its limit` in `accent.attention`, and
    `—` before it's read, as the status document's `extra_usage` gives it (see The status document),
    and for `all accounts` `off on all`, `on on all` or `on for 1 of 3` (settled in the plan);
    `worth at API prices`, `$184`; and `against its price`, `4.0× its $46 week`.
- **With one account,** Compare goes, being Detail with one column: the heading shows ` Detail `
  alone.
- **On a phone,** Compare turns: a group at a time, an account a row, its name from column 1, and
  `all` last, unnumbered. More accounts than its height holds scroll with `j/k`, the last row saying
  `▼ 2 more below · j/k` (settled in the plan).
  - WEEK USED and BY MODEL: a blank row under the heading and between rows, so the bars don't merge;
    the bars 20 wide from column 13, WEEK USED's percentage after its bar, and at column 35 BY
    MODEL's account's largest version and its share, `● Opus 5.5 55%`.
  - ACTIVITY (`reqs` at 13, `sessions` at 23, `moved` at 35), LIMITS (`hit` at 13, `at cap` at 26,
    `left` at 36, `5-hour` cut to `5h`) and VALUE (`plan`, its price, at 13, `worth` at 26,
    `against it`, the ratio alone, at 36): the columns' heads on the group's heading row in
    `text.subtle`, a row an account under it, with no blank rows.

**Detail** shows one account in full, or all of them, chosen from a list at its left. The panel's
top half is now and the period; a rule, `the last six weeks`, heads the rest.

- **The list,** columns 0 to 19, a chooser and nothing else: from row 6, 2 rows apart and from
  column 1, `0 all accounts`, `0` in `text.subtle` and the name bold `text.secondary`, then each
  account's number, name and marks, and at column 15 its state's dot or square, in its state's
  colour. The chosen row is `bg.selection` across columns 0 to 16. Two rows under the last,
  `↑↓ choose`, `↑↓` bold `accent.key` and `choose` in `text.subtle`. A `│` in `border` at column 20,
  rows 6 to 32, or 33 with sessions; the panel starts at column 24. More accounts than it holds
  scroll with the choice (settled in the plan).
- **The panel's head,** row 6: the account's number, name and marks, three spaces, then its state in
  ROUTING's words without its `back …` time, saying where new sessions go (see The Overview): `● at
  its cap · new sessions go elsewhere`, `■ limit reached · new sessions go elsewhere`, `■ refused
  (401) · its token needs renewing`, `● under pressure · new sessions still come here`, `● open ·
  new sessions come here`. For `all accounts`, `0 all accounts`, three spaces, and its accounts'
  states counted. A model's own window at its limit, as Fable's week, heads it `■ Fable wk limit ·
  other models still come here`, in `accent.attention`, its USAGE row as `usage` draws a limit,
  while Compare's word stays the account's own (settled in the plan).
- **USAGE,** row 8, then every window, used or not, from row 9, 2 rows apart: the window's name in
  `text.subtle` (`5-hour`, `Week`, `Fable wk`); `usage`'s bar from 9 cells in, 40 wide; a cell on,
  its percentage, right-aligned in 4 and bold in its level's colour; and two cells on, `usage`'s
  full words (see `usage`, the printout). For `all accounts`, every account's window together, its
  words its room, `1.2 of 3 left`.
- **THIS WEEK,** on USAGE's row, rows 8 to 13, so its last row is level with the last window's:
  `requests`, `limits`, `plan`, `worth` with its ratio, `$184  4.0×`, and `extra use`, labels 10
  wide in `text.subtle`; for `all accounts`, `limits` is how many and `plan` the plans summed,
  `$600/mo`. It starts at the fifth week's column of BY DAY below, `cx + 4 × step`, column 108. Its
  title names the period: `THIS WEEK`, or `LAST 30 DAYS`.
- **BY MODEL,** row 15: the period's share of use as a stacked bar, from `cx`, 12 cells into the
  panel, at column 36, to the six weeks' right edge below, `6 × step − gap` wide, 104 cells; on row
  16, from `cx`, each version with a share, `● Opus 5.5 55%`, two cells apart, the smallest that
  don't fit the row left out and `+2 more` in `text.subtle` ending it (settled in the plan).
- **The rule,** row 18: `┄┄ ` in `border`, `the last six weeks` in `text.faint`, then `┄` in
  `border` to BY MODEL's right edge. Above it, the windows are now, and THIS WEEK and BY MODEL the
  period; below it, BY DAY and WEEKS always cover the last six weeks.
- **BY DAY,** row 20: the last six weeks of days, from `cx`, `dw` cells a day and `gap` cells
  between weeks, 2 and 4, so `step = 7 × dw + gap` is 18: each day the points of its week the
  account used, in History's five shades on the account's own scale (see History); a red `▪`, in
  `state.destructive`, in the middle of a day it hit a 5-hour limit; and `··` in `text.faint` for
  days still to come. On row 21, under each week, its first day, `24 Aug`, a `step` apart, this
  week's bold `text.secondary` and the rest `text.faint`. For `all accounts`, every account's days
  together, marked where any hit a limit.
- **WEEKS,** row 23: the last six weeks, a `step` apart from `cx`, each `usage`'s bar of the week's
  peak, `step − gap − 5` wide, 9 cells, and a cell on, the peak, `text.muted`, or at 100 bold
  `state.destructive`; for `all accounts`, each week the mean of the accounts' peaks. On row 25,
  from the panel's left, the verdict, as History's Weeks gives it (see History), bold in its colour,
  then `   limit hit 3 of the last 7 weeks · 9% left at a reset, on average` in `text.muted`. For
  `all accounts`, `room to spare together`, in `state.positive`, or `little to spare together`, in
  `accent.attention`, the first where its weeks left 25% or more at a reset on average
  (settled in the plan), then
  `   30% left at a reset, on average · side ends weeks with 59% left, work with 9%`, the accounts
  that end weeks with the most and the least left.
- **LATELY,** row 27: the account's last three events, newest first, from `cx`, in LATELY's marks,
  but told from the account's side, which they leave unnamed:
  `14:31  ╎ reached its cap: 3 sessions moved to side`; a move onto it, `▸ 3 sessions came from
  work: work reached its cap`; `14:12  ▲ 7f3a started here`, or, where it started on the router's
  best, `c61b started here, the best`; `◔ came under pressure: out ~16:05 at this pace`; a pin
  naming it with others, `● pinned with side, by hand`; and `12:10  ◇ primed: its 5-hour window
  started`. For `all accounts`, the last three of every account's, each naming its account, as the
  Overview's LATELY does: `c61b started on side, the best`.
- **With sessions running** on the chosen account: `SESSIONS`, and `now` in `text.faint`, at the
  panel's left on row 27, and from row 28 a row a session: `▸`, bold `accent.key`, before the one
  selected; `●` while busy, in `state.positive`, or `○`, in `text.subtle`; the id, bold,
  `text.primary` while busy and `text.muted` while idle; two spaces and the model, padded to 8, in
  `text.secondary`; what it's doing, padded to 9, bold `accent.mode` while busy and `text.subtle`
  while idle; and when it was last seen, right-aligned in 3, in `text.subtle`. The selected row is
  `bg.selection` from a cell before it to 42 cells on. On row 33, two cells in,
  `enter opens d28c's page`, `enter` bold `accent.key` and the rest `text.muted` (see A session's
  page). `→` takes the selection from the list into them, and `←` back; there `↑` `↓` select a
  session, and `enter` opens its page; more sessions than rows 28 to 31 hold end with `+2 more`
  (settled in the plan). LATELY moves beside them, 48 columns into the panel, at column 72, its rows
  from row 28 and its words cut to 52.
- **With one account,** there's no list and no Compare, and the stack grows into the width: the
  panel from column 0, `cx` 12, the windows' bars 50 wide, days 3 cells and weeks 1 apart, so `step`
  is 22, BY MODEL 131 wide, and THIS WEEK at column 100, over the fifth week as with the list.
- **On a phone,** stacked, each part under its heading, a blank row between: on row 8, `◂ 1 work ▸`,
  `◂` and `▸` bold `accent.key`, a picker that takes any number of accounts (settled in the plan),
  and at the right `↑↓ an account, or all` in `text.subtle`; under it, the state, its glyph and
  words; USAGE, every window, its bar 18 wide from column 9, the percentage at 28 and `usage`'s
  short words at 34, a blank row between windows; THIS WEEK in two columns of three, 26 apart,
  labels 10 wide: `requests`, `limits` and `plan`, then `worth`, `extra use` and `against`; BY
  MODEL, its bar 40 wide from column 10 on the heading's row, and its versions over two rows; BY
  DAY, the last three weeks from column 10, 2 cells a day, with their first days under them, 15
  apart; WEEKS, the last four peaks as percentages from column 10, 6 apart, `34%…` this week's; the
  verdict, and under it `limit hit 3 of 7 weeks · 9% left at a reset`; and LATELY, its words cut
  to 44. No rule.

**Keys.** `tab` views; `[ ]` view; `<` `>` the period, on both views; `←` `→` the focus along
Compare's accounts; `↑` `↓` the account in Detail's list, or the session in its SESSIONS; `enter`
opens, from Compare, the account in Detail, and from a session row, the session's page; `p` new
sessions' routing picker; `?` help; `q` quit. A click picks a sub-tab, an account, a column or a
session. The key lines, where the frames draw the keys from before the sweep of 7 and 8 October 2026
(settled in the plan):

- Compare:
  `tab views   [ ] view    < > period: this week    ←→ account  enter its page    p pin    ? help  q quit`.
- Detail:
  `tab views   [ ] view    < > period: this week    ↑↓ account  enter its page    p pin    ? help  q quit`,
  and in its SESSIONS `↑↓ session  ← accounts  enter its page` in place of
  `↑↓ account  enter its page` (settled in the plan).
- With one account, with no sub-tabs and so no `[ ] view`:
  `tab views   < > period: this week    ? help  q quit`, `↑↓ session  enter its page` joining it
  while it has sessions; no page shows `p pin` with one account, unless a pin set from the command
  line stands (see Routing by hand).
- On a phone: `tab views   [ ] view   ? keys   q quit`.

**`?` help** (see The page), `HELP · ACCOUNTS`, lists the page's keys first, `t`, `R`, `j`/`k` and
the wheel among them; then it explains each part in turn: Compare's groups and how each row is
counted, a session served once however many days and accounts it ran on, the time at cap or limit,
what its week had left at its last reset; worth at API prices and the price table's date; against
its price and how a period is priced; what `all accounts` counts, and why it isn't the pool;
Detail's windows, read as `usage` reads them, BY DAY's shades and its red `▪`, and WEEKS' peaks and
what each verdict means.

**Its figures.** The states, the windows, WEEK USED, extra usage, LATELY and SESSIONS are live, read
as the Overview reads them: the status document, the router's events, `GET /sessions` and the
request stream. The rest is the ledger's days, read with no router (see The request ledger):
requests, as the ledger counts them; sessions served, the distinct ids of the days' `session_ids`,
across accounts for `all accounts`; sessions moved, the days' `moved_off` and `moved_on`; limits
hit, the days' `limits`; time at cap or limit, the days' `minutes_at_cap` and `minutes_at_limit`
together; worth and share of use, the days' usage, priced; plan, each account's `plan` (see Config),
priced by the table. An account's weeks run from one reset of its week window to the next, as the
days' `resets` give them: a week's peak is the use read just before its reset, this week's its
highest use so far, and `left at its last reset` 100% less the peak of the week that ended last. BY
DAY is each day's `rise` of the week window. Days before today are read once and kept; today's lines
are read as they're added, from where the last read ended. An account added part-way through a
period shows what was recorded of it, priced against the whole period; a period with nothing
recorded, as on a fresh install, shows `—` in every row (settled in the plan).

### History

When: how use has gone over time, by day across a year, week by week against the accounts' limits,
day by day by model and by account, and in tokens as ccusage reports them. It has four views, as
sub-tabs: **Year**, **Weeks**, **Days** and **Tokens**. The rules Accounts and History share hold
here (see Accounts): the heading row, no description after a heading, the labels, `all accounts`,
versions in tables and families in charts, worth at API prices, against its price, the periods and
the keys. It reads the ledger's days alone, so it needs no router (see The request ledger). It's
drawn in the Paper file *Switchboard · History*, on its page **FINAL · history · signed off 5 Oct
2026 · revised 6 Oct**, and kept in `testdata/vhs/reference/` as `history-year-160`,
`history-year-opus-160`, `history-weeks-160`, `history-weeks-columns-160`, `history-weeks-ten-160`,
`history-days-160`, `history-days-ten-160`, `history-tokens-160`, `history-tokens-models-160` and
`history-phone-year-52`: 160 columns by 37, and a phone's 52. The frames show the keys from before
the sweep of 7 and 8 October 2026, `p` for the period; where they and this section differ, this
section holds.

**Five shades.** Days are drawn in five levels, far enough apart that neighbours can be told apart:

- **Level 0,** a day with nothing on it, in `viz.day.0`.
- **Levels 1 to 4,** the active days split into quarters by how much each did, on one scale for
  everything a view shows together: the cuts are the active days' values in order, taken a quarter,
  a half and three quarters of the way along (of `n` active days, the values at `n × q // 4`, `q`
  from 1 to 3), and a day's level is 1 more than the cuts it reaches. They're `viz.day.1` to
  `viz.day.4` (see Themes).
- **A family's strips,** and Year filtered to a version, take its own colour in four steps instead:
  `viz.day.0` mixed 40% and 70% toward it, the colour itself, and the colour mixed 55% toward white,
  or toward black on a light canvas (settled in the plan).
- **The Year shades requests,** how busy a day was, never its worth, which shifts with the models
  used and their prices. A request is a ledger line, so the grid needs nothing priced.

**Year** is the default view.

- **The grid:** 53 weeks, the last 12 months to this week, a column a week, 2 cells apart, from
  column 14; a row a day, rows 7 to 13, from the week's first day as `week_starts` sets it (see
  Config), each day `■` in its level's shade; days still to come this week left out. A day before
  the ledger recorded anything is an empty square like any other, with no line saying when it began.
  The first, third and fifth days' names, `Mon`, `Wed` and `Fri`, at column 0 in `text.subtle`; on
  row 6, each month's name over the column of its first week, in `text.faint`, the current month's
  in `text.secondary`.
- **The key,** row 15, from column 14: each shade's `■` and its bounds in requests a day, the cuts
  rounded to 50, in `text.subtle`, `■ none   ■ under 450   ■ 450–750   ■ 750–1,000   ■ over 1,000`,
  then two spaces and `requests a day` in `text.faint`.
- **The numbers,** at column 126: `LAST 12 MONTHS` on row 6, then from row 7 a row each, labels 13
  wide in `text.subtle` and values in `text.secondary`: `requests`, `236,410`; `sessions`, `1,688`,
  each counted once; `limits hit`, `5-hour ×402`, in `state.destructive`, those of more than one
  window joined by ` · `, `5-hour ×402 · week ×3` (settled in the plan); `worth`, `$27,480`;
  `plans cost`, `$7,200`, the plans' monthly prices over the months; `against it`, `3.8×`, bold, in
  `state.positive` at 1× or more and `accent.attention` under; `busiest day`, the day of the most
  requests, `Tue 22 Sep`; and `longest run`, the most days in a row with any request, `58 days`.
- **BY MONTH,** row 17, `worth at API prices; against what the plans cost that month` after it in
  `text.faint`. On row 18, its columns' heads in `text.subtle`: `requests` at 14, `worth` at 26,
  `against` at 38, `limits` at 48 and `by model` at 60. From row 19, a row a month, oldest first,
  its name at column 2, the first with its year, `Oct 2025`, the current month's bold; values in
  `text.secondary`; `against` bold, coloured as against its price is, and `—` in `text.faint` for
  the month so far; `limits` in `state.destructive`. At column 60, a bar as long as the month's
  worth against the largest month's, 30 cells at most and 1 at least, split by family, each cell `▅`
  in its family's colour, or in `text.muted` for a family with no colour of its own, as one the
  version table doesn't know (settled in the plan), so bars on neighbouring rows don't merge; two
  cells after a part month's bar, `so far`, or `from the 16th` for the month the ledger began
  recording in, in `text.faint`. Two rows under the last month, from column 60, the families' dots,
  each with its name in `text.subtle`, `● Opus   ● Sonnet   ● Fable   ● Haiku`: the bars' key,
  headed by its column's `by model`.
- **`v`** cycles what the grid and its numbers show: all use, the default; then each model version
  used in the 12 months, in the version table's order (see The request ledger); then each account;
  and round. The heading's state names it,
  `Opus 5.5 only, every account  ·  v for all, a model or an account`, with `v` bold `accent.key`.
  The shades and their key re-scale to what's shown, the key's unit naming it,
  `Opus 5.5 requests a day`, and a version's grid takes its shade in four steps.
  - Filtered to a version, the numbers are headed `OPUS 5.5 · 12 MONTHS`: `requests`, `sessions`,
    `worth`, `of all use`, its share of the year's worth, `first used`, the first day it was used in
    the 12 months, `Mon 22 Jun`, and `busiest day`. BY MONTH,
    `Opus 5.5 only: its worth at API prices, and its share of the month`, heads `requests` at 14,
    `worth` at 26 and `share` at 38, with `worth` over the bars at 60; a row a month it was used,
    its bar in its shade, 30 cells at most against its largest month, and `from the 22nd` after the
    bar of the month it was first used in, where that was part-way through; and above them a row for
    the months before it, `Oct 2025 – May` at column 2 and `none: Opus 5 then` at 26, naming its
    family's version used before it, both in `text.faint`.
  - Filtered to an account, `work only, every model`, the numbers, BY MONTH and the shades are the
    account's alone, plans cost its own plan's, and `←` `→` step to the next account, `←→ account`
    joining the key line; `v` passes over versions with no use in the 12 months
    (settled in the plan).
- **`<` and `>`** step from the last 12 months back through each calendar year the ledger holds, and
  forward again: a year's grid runs from the week of 1 January to the week of 31 December, the days
  outside it left out, and its numbers are headed by the year, `2025` (settled in the plan).
- **On a phone:** under the period's row, the latest 22 weeks that fit, from column 4 under their
  months' names, `M`, `W` and `F` at column 0; a blank row, and the key cut short,
  `■ 0 ■ <450 ■ 450–750 ■ 750–1k ■ >1k  requests a day`; a blank row, `LAST 12 MONTHS` and its
  numbers in two columns of three, 26 apart, labels 10 wide: `requests`, `sessions` and `limits`,
  then `worth`, `plans` and `against`; then a blank row and BY MONTH, its heads on its own row,
  `worth` at 12, `against` at 22 and `by model` at 32, and the last six months under them, their
  names at column 1 and their bars from column 32, 18 cells at most.

**Weeks** is how full each account's week got, and whether there are as many accounts as the use
needs. **Short** is a week in which some account hit its week's limit, its peak at 100%. Weeks takes
its form for many accounts, as `history-weeks-ten-160` draws it, where the verdict can name one to
drop, one fewer being short on no more weeks than now; otherwise its form for few.

- **The capacity verdict** leads, rows 6 to 8:
  - The headline, bold: `<n> is one more than you need`, in `state.positive`, where one fewer would
    be short on no more weeks than now, as `ten is one more than you need`; `<n> is one too few`, in
    `accent.attention`, where now is short on 4 or more of the 7 and one more would halve that; else
    `<n> is about right`, in `state.positive`, as `three is about right`. The count is a word to
    ten, then figures (settled in the plan).
  - Then `as now: ` and how many of the last 7 whole weeks were short, and who was short each time,
    `short on 3 of the last 7 weeks, each time work alone`; `   ·   with one fewer: ` and how many a
    replay with one account fewer finds, `short on 6 of 7`; and `   ·   with one more: `,
    `short on none`. The labels are `text.subtle` and the counts `text.muted`, the worst of them
    bold `accent.attention`. Where one fewer would be short on no more weeks than now,
    `with one fewer` says so, `short on 4 of 7, the same weeks`, `with two fewer` takes
    `with one more`'s place, and the third row names the account to drop,
    `spare is the one to drop: $100/mo, 5% used on average, and nothing ran short without it`, in
    `text.muted`. With one account, the row reads
    `as now: short on 3 of the last 7 weeks   ·   a second would have cleared them` where one more
    would leave none short (settled in the plan).
  - Last, `an estimate, from replaying these weeks with one account fewer or more`, in `text.faint`,
    with few accounts; with many, `an estimate, from replaying these weeks`, three spaces after the
    line naming the account to drop.
- **The replay** treats each of the last 7 whole weeks alone. Without an account, its peak, times
  its plan's size, is shared among the rest by their sizes, each one's peak rising by its share;
  with one more, of the plan most of the accounts have, every peak falls in proportion, the same use
  over more room. A replayed week is short where any account would reach 100%. `with one fewer`
  drops the account whose loss makes the fewest weeks short, and of those that tie, the one used
  least over the weeks shown. Plans' sizes are in Pros, `pro` 1, `max5x` 5 and `max20x` 20 (see The
  request ledger); an account without a `plan` counts as the plan most of the accounts have, and
  where none has one, every account counts as one (settled in the plan).
- **The table,** `v`'s first shape and the default. On row 10, the eight weeks' first days,
  `10 Aug`, from column 14, 8 apart, right-aligned in 6, in `text.faint`, this week's bold
  `text.secondary`, and at column 82 `over the last 7 weeks` in `text.faint`. On row 12,
  `all accounts`, bold `text.secondary`, each week the mean of the accounts' peaks, and at 82
  `short on 3 of 7 weeks`, bold `text.secondary`, and with many accounts `   with ten accounts`
  after it, in `text.subtle`. From row 14, a row an account: its name and marks at column 0; each
  week's peak, right-aligned in 5, in its level's colour, as `usage` colours a percentage, but bold
  only at 100%, in `state.destructive`; and this week's so far, `34%…`, bold `text.secondary`. At
  82, the account's verdict, bold, then, with few accounts, `  9% left` and
  `  without it: short 6 of 7`, in `text.subtle`, and with many, `   9% left at a reset`, with no
  `without it`.
- **An account's verdict,** over the last 7 whole weeks: `hits its limit most weeks`, in
  `state.destructive`, where it hit its limit in 3 or more of them; else
  `plenty to spare most weeks`, in `state.positive`, where its weeks ended with 40% or more left on
  average; else `ends most weeks with room`, in `text.secondary`. Where it could go, `could go: `
  bold `accent.attention`, then `5% used, no limit ever` in `text.muted`; where it's barely used,
  `barely used: ` and `80% left at a reset`; each alone in its cell. An account could go where the
  rest would cover it with no more weeks short, and it hit no limit in the 7 weeks and used 10% or
  less on average; it's barely used where its weeks ended with 75% or more left on average
  (settled in the plan).
- **The columns,** `v`'s second shape: a block an account, `all accounts` first, 6 rows apart, the
  name at column 0 on the block's first row. Each week is a column 5 wide and 3 rows tall, from
  column 14 and 8 apart, filled from the bottom to its peak, its top cell in eighths, each row in
  the ramp's colour at its height (`viz.ramp.1`–`viz.ramp.4`, as a bar's cells take it) mixed 75%
  from the canvas toward it, and 45% for this week; a week with no use is `▁▁▁▁▁` in `text.faint`.
  Under each column, its peak centred in 6, `text.muted`, 100% bold `state.destructive`, and this
  week's, `59%…`, in `text.secondary`. Beside each block at column 82: for `all accounts`, the
  headline, then `as now: …`, then `with one fewer: …   with one more: …`, then the estimate's line;
  for an account, its verdict, then `limit hit 3 of 7 weeks · 9% left at a reset, on average` in
  `text.muted`, then `without it, the rest: ` in `text.subtle` and `short on 6 of 7` in
  `text.muted`. The weeks' first days go under the last block.
- **The legend,** row 34:
  `each week at its peak   ·   100% it hit its limit   ·   34%… this week, so far`, each mark as
  it's drawn and the words in `text.faint`; under the columns,
  `short: a week some account hit its limit   ·   ` leads it.
- **Each column is one of the account's own weeks,** from one reset of its week window to the next,
  placed under the first day of the calendar week it began nearest. More accounts than fit scroll,
  `j/k` or the wheel.
- **On a phone,** Weeks shows the verdict and the table's last four weeks (settled in the plan).

**Days** is eight weeks, the week so far and the seven before it, a day at a time.

- **The weeks:** from column 14, 2 cells a day and a cell between weeks, so a week is 15 cells; on
  row 6, each week's first day, `10 Aug`, in `text.faint`, this week's bold `text.secondary`; on row
  7, over this week's days, their initials, `M T W T F S S`, in `text.faint`. A day still to come is
  `··`, in `text.faint`.
- **BY MODEL,** row 8, `every account, weighed at API prices, on one scale` after it in
  `text.faint`: from row 10, two rows apart, a strip a family used in the eight weeks, never a
  version (settled in the plan), `●` in its colour and its name in `text.secondary` at column 0,
  each day the family's worth that day, every account's, in the family's own steps, the strips on
  one scale; at column 136, its share of the eight weeks' worth, right-aligned in 4, bold
  `text.secondary`, then ` of use` in `text.subtle`, `59% of use`.
- **BY ACCOUNT,** three rows after the last family,
  `the share of its week each day used, on one scale` after it: two rows apart, a strip an account,
  its name and marks at column 0, each day the points of its week it used that day, in `viz.day.1`
  to `viz.day.4`, every account on one scale, and a red `▪`, `state.destructive`, in the middle of a
  day it hit a 5-hour limit; at column 136, how many it hit, right-aligned in 4, bold
  `state.destructive`, or `text.faint` at none, then ` 5-hour limits` in `text.subtle`. Past six
  accounts it scrolls within itself, with `j/k` or the wheel, and row 33 says what's out of view:
  `▼ 4 more accounts below   ·   j/k or the wheel to scroll them`, `▼` in `accent.key`, the words in
  `text.muted`, `j/k` bold `accent.key`.
- **The key,** row 34: `less `, the five shades as 2-cell swatches 3 apart, `more`, in
  `text.subtle`, then `  each shade a quarter of the active days` in `text.faint`; then a swatch of
  `viz.day.2` holding the red `▪`, and `a day it hit a 5-hour limit`; then `··` and ` days to come`.
- **On a phone,** Days shows the last three weeks (settled in the plan).

**Tokens** is ccusage's weekly report, inside switchboard, split by what ccusage can't split by: the
account.

- **Columns,** their heads on row 6 in `text.subtle`: `week` at 0, `models` (or `on`) at 14, then
  right-aligned in 10 cells `input` from 32, `output` from 44, `cache write` from 56, `cache read`
  from 70 and `total` from 84, then `worth` at 96 and `against its price` at 106. `cache write` is
  both lifetimes' writes together, and `total` the four kinds.
- **A period a row,** newest first, from row 8, a blank row between periods: named by its first day,
  `28 Sep`, by day its day, `Thu 1 Oct`, and by month its month, `Oct 2026` (settled in the plan),
  this week as any other, bold `text.primary`; how many model versions it used, `6 models`, in
  `text.muted`; its counts and worth, bold `text.primary`; and against its price,
  `4.6× its $138 week`.
- **`v` splits each period:** **by account,** the default, a row each, `└ ` in `text.faint` and the
  account in `text.secondary` at 0, its versions' count at 14 in `text.subtle`, and its counts and
  worth in `text.muted`; **by model,** a row each version, `└ opus-5-5`, its id without `claude-`,
  as ccusage names it, and at 14 the accounts it ran `on`, each version's worth at its own prices;
  **by account and model,** each account's versions nested under it, `└ work`, then `  └ opus-5-5`
  (settled in the plan); or **none,** the periods alone.
- **Counts:** under 100 million in millions to one place, `4.2M`, `0.8M`, `75.4M`; then in whole
  millions, `615M`; and from a billion in billions to two places, `1.06B`. Worth is to the dollar,
  `$635`.
- **`<` `>`:** by day, by week, the default, and by month. Older periods scroll, `j/k`, the row
  after the last shown saying `▼ 3 more weeks below   ·   j/k to scroll`, `▼` in `accent.key`, the
  words in `text.muted`, `j/k` bold `accent.key`.
- **The footnote,** row 34, in `text.faint`:
  `worth: what the same tokens would cost through the API, at its prices as of 7 Oct 2026`, the
  price table's date, 7 Oct 2026 for the built-in table and 1 Oct 2026 in the frames, whose fixtures
  inject a table dated as they were drawn, and split by model, `, each version at its own price`
  after it.
- **On a phone,** Tokens shows each period's `total` and `worth` alone (settled in the plan).

**Keys.** `tab` views; `[ ]` view; `<` `>` the period, on Year and Tokens; `v` the shape or split,
on Year, Weeks and Tokens; `j/k` and the wheel scroll, where a view runs past the screen; `p` new
sessions' routing picker; `?` help; `q` quit. A click picks a sub-tab, a period or a split. Weeks
and Days keep to eight weeks, which the heading row's right shows, `eight weeks`, in `text.muted`,
without `<` and `>` (settled in the plan). The key lines, where the frames draw the keys from before
the sweep of 7 and 8 October 2026 (settled in the plan):

- Year:
  `tab views   [ ] view    < > period: last 12 months    v split: all    p pin    ? help  q quit`,
  the split naming what's shown, `v split: Opus 5.5`.
- Weeks: `tab views   [ ] view    v split: table    p pin    ? help  q quit`, or `v split: columns`.
- Days: `tab views   [ ] view    p pin    ? help  q quit`.
- Tokens:
  `tab views   [ ] view    < > period: by week    v split: by account    p pin    ? help  q quit`.
- With one account, no line shows `p pin`, unless a pin set from the command line stands (see
  Routing by hand); on a phone: `tab views   [ ] view   ? keys   q quit`.

**`?` help** (see The page), `HELP · HISTORY`, lists the page's keys first, `t`, `R`, `j`/`k` and
the wheel among them; then it explains each part in turn: the five shades, each a quarter of the
active days, and that the Year counts requests; BY MONTH's bars and against what the plans cost;
what a week's peak is, what short means, how the replay estimates a week with an account fewer or
more, and what each verdict means; Days' two scales and its red `▪`; and Tokens' columns, as
ccusage's, and the price table's date.

**Its figures,** every one from the ledger's days (see The request ledger), read with no router:
requests a day, by version and by account; sessions, the distinct ids of the days' `session_ids`;
limits, the days' `limits`, by window; worth, the days' usage priced at today's prices; tokens, by
kind, the days' usage; plans cost, from each account's `plan` (see Config). An account's weeks run
from one reset of its week window to the next, as the days' `resets` give them: a week's peak is the
use read just before its reset, and this week's its highest use so far; Days' BY ACCOUNT is each
day's `rise` of the week window. History starts the day switchboard first recorded; nothing is
filled in from before, as from Claude Code's own transcripts, which can't tell the accounts apart.
Days before today are read once and kept; today's lines are read as they're added, from where the
last read ended. An empty history draws the grid's squares all at level 0 and the numbers `—`; Weeks
says `not enough weeks yet` until a whole week is recorded, and Days and Tokens
`nothing recorded yet` (settled in the plan).

### Sessions

Where each session's requests are going, now, in two views on sub-tabs: **Switchboard**, the
default, the sessions down the left as calls and the accounts down the right as lines, `usage`'s
rows, with a cord from each call to its account and its requests travelling it; and **List**, one
table of today's sessions, those running, then those ended. The frames are in the Paper file
*Switchboard · Sessions and Runway*, on the page **FINAL · Sessions and Runway · signed off · 8 Oct
2026**; the references are `testdata/vhs/reference/sessions-*`: `sessions-switchboard-160`,
`sessions-refused-160`, `sessions-moved-160`, `sessions-pinned-160`, `sessions-ten-accounts-160`,
`sessions-one-account-160`, `sessions-list-160`, `sessions-phone-52` and `sessions-list-phone-52`.
Columns and rows are at 160 by 37 unless they say otherwise.

**The heading row,** row 4: `SESSIONS` in `text.subtle`; three spaces; the sub-tabs, `Switchboard`
and `List`, each with a space either side, the one shown bold `text.primary` on `bg.subtle` and the
other `text.subtle`; a space and `[ ]` in `text.faint`; two spaces, and a rule of `─` in `border` to
the last column. `SESSIONS` takes 0–7, `Switchboard` with its spaces 11–23, `List` with its spaces
24–29 and `[ ]` 31–33, and the rule starts at 36. The view starts on row 6, and the row over the
key line holds its state line, where it has one (see The page). The sub-tab shown is kept in the
preferences file (settled in the plan).

**The Switchboard.** A call's session id is at column 0, what it's doing at 18, where it moved from
at 31, and its cord's plug at 48; the jacks are at 84, and the lines run from 86, 74 columns to the
edge.

- **The calls:** a row a running session, grouped by the account it's on, the accounts in config
  order, a blank row after each group, from row 6. A session whose models are on two accounts has a
  row in each group (settled in the plan). Rows keep their places from look to look: a new session,
  or one a move brings, joins the foot of its group. Each row:
  - the session's id, cut to 4, bold `text.primary` while the session works, else `text.muted`; a
    space, and its directory's last part, cut with `…` to 12 cells, in `text.muted`, as in
    `d28c switchboard`;
  - at 18, what it's doing, in a session's page's words: `↑ asking` or `↓ answering`, bold
    `accent.key`; `✕ 429` or `✕ 403`, bold `state.destructive`, while a refusal shows; `… 429` in
    `text.subtle` while a 429 is sent again on the account; else `idle 34s` in `text.faint`, the
    time since its last answer ended, in seconds under a minute, then as the dashboard writes spans,
    `9m`, `1h 05m`, `1d 3h`;
  - at 31, while the move that brought it to the account is its latest, `▸ from work`: `▸` in
    `accent.key`, `from` in `text.muted`, and the account it left in `text.secondary`, cut with `…`
    to 8 cells;
  - at 48, its cord's plug, `●`.

  A session works while it asks, while it's answering, and while a refused request of its is
  retried. Claude Code's quota check is never drawn.
- **The lines:** each account's line is `usage --by account`'s narrow block, 74 columns from column
  86 (see `usage`, the printout):
  - its name row: the number, name and marks; from the name column, fitted to the names as `usage`
    fits it (16 cells, column 102, for `2 personal ◆`), its state in the Overview's ROUTING words,
    `● open`, ` · new sessions come here` added on the account new sessions go to,
    `● at its cap · back 17:10 · new sessions go elsewhere`, `● under pressure · out ~16:05`,
    `■ limit reached · back 16:30`, `■ refused (401) · its token needs renewing` or `○ not read yet`
    (see The Overview); and ending at the last column, `● pinned` or `✕ out of the pool` (see
    Pinned, below);
  - a row a window, `5-hour`, `Week`, then each model's own week, `Fable wk`, a blank row between:
    its label at 87 in `text.subtle`, its bar at 98, 32 cells, a cell, its percentage in 4 cells at
    131, two cells, and its words, 22 cells, at 137.

  The lines start on row 6, each followed by two blank rows: 8 rows apart with three windows. A line
  whose group of calls starts below its first jack moves down to meet it, so cords only run right,
  then down. A closed account is never faded: its words and colours say why, amber for a cap and red
  for a limit or a refused token (see Routing by hand). No pool row: no session plugs into the pool,
  which `usage` and the Overview show.
- **The key,** the Overview's, under two blank rows after the last line, at column 87:
  `┃ even pace: where you would be now, used evenly` on its first line, and on its second,
  `╎ cap: work at 95%`, `◆ primary` and `▲ new sessions go here`, four spaces apart. Where a line of
  it would pass the last column, it starts at column 0.
- **The jacks,** a jack on each row of a line, at column 84: `○` in `text.faint` where no cord ends,
  `◉` in its cord's colour where one does. A session takes the jack it holds, or its line's first
  free one, in group order. A line with more sessions than rows grows a row for each more.
- **The cords:** from the plug, `━` along the call's row, `┓` turning down at its bend, `┃` down,
  `┗` turning along at the jack's row, and `━` along to the jack, `◉`; a call on its jack's row runs
  straight. A cord's colour says what's happening, never which account, as ten accounts would run
  out of colours: `accent.key` while the session works, and at rest, `accent.key` dimmed toward the
  canvas, `mix(canvas, accent.key, 0.45)`.
- **The bends,** so no cords cross: the bent cords are ordered by their jack's row, then their
  call's. The first bends 12 cells left of the jacks, at 72, clear of a 9-cell loose cord by 3
  cells, and each next 3 cells further left; where the last would then fall outside the bends' room,
  they bend 2 apart, or failing that, 1. The bends' room runs from the column after the plugs to the
  first bend, 49 to 72: 24 columns, so 3 apart holds 8 bent cords, 2 apart 12, and 1 apart 24. Where
  1 apart can't hold them all, the view draws lanes (see Lanes, below).

**What moves.** The cords' motion stays on this tab alone (see What moves). Its steps fall on the
clock's 80-millisecond marks, every cord in step, at up to 30 frames a second, and a frame is drawn
only for what moves on screen, never for a cord scrolled away or under `?`.

- **A request going out:** a pulse runs from the plug to the jack in 0.54 seconds, 5 cells long, its
  head bold `text.primary` and the 4 cells behind it the cord's colour glowing toward `text.primary`
  by 0.58, 0.46, 0.34 and 0.22. The row says `↑ asking`, the cord `accent.key`.
- **An answer coming back:** every fourth cell of the cord, but the plug and the jack, glows 0.75
  toward `text.primary`, bold, the lit cells stepping a cell toward the call at each mark, and the
  jack is lit, bold `text.primary`. The row says `↓ answering`.
- **An answer's end:** a pulse runs back from the jack to the plug in 0.54 seconds, lit
  `accent.key`; at once the row says `idle 0s`, counting up, the cord dims, and the jack goes out
  (settled in the plan).
- **A refusal,** a 429 at a limit, or a 401 or a 403: `✕` on the jack, bold `state.destructive`, and
  a pulse running back from it, its head bold `text.primary` and its trail `state.destructive`,
  glowing as a request's does. The row says `✕ 429`, or `✕ 403`, the id still bold, and the cord
  stays `accent.key`, as its request is to be retried. A 429 the router sends again on the account
  shows as `… 429`, with no pulse (settled in the plan).
- **The move** comes as the red pulse comes home, 0.54 seconds after the refusal: the `✕` clears,
  and the retried request's pulse sets out along the new cord as the session joins its new group
  (settled in the plan).
- **In the references,** the motion is frozen and every glow and fade is a colour mix, so each cell
  holds one glyph: a pulse's head at cell `⌊along × last⌋` of its cord, counted from where it set
  out, `last` being the cord's last cell; and the shimmer lighting the cells whose place along the
  cord, plus 2, divides by 4.

**Moves.** The session's row leaves its group, which closes up at once, and joins its new account's
group at the foot, `↑ asking  ▸ from personal`, its cord to the first free jack there.

- **Where a limit forced the move,** the old cord hangs from its old jack while the limit holds: `╌`
  in `state.destructive`, to the jack's left, 7 cells, then 6 for a second session, each a cell
  shorter, one for each session the limit's event moved, its `count`; red, as red on this page means
  a limit. Its jack shows `○`, and isn't free while the cord hangs. A session the limit moves at its
  next request keeps its row and its jack until then.
- **A move no limit forced,** by pressure, a pin or by hand: the old cord hangs 9 cells, in
  `text.subtle`, fading into the canvas over 3 seconds (settled in the plan).

**Pinned.** On each account the global pin names, `● pinned` in `accent.key` ends its name row at
the last column. On each it leaves out, `✕ out of the pool` in `text.faint`, cross and words, sits
in the same place, and its line is faded as the Overview fades an account out of the pool: its
number, name and marks in `text.faint`, its bars keeping 35% of their colour, which is 65% toward
the canvas, and its percentages and words in `text.faint`. Its state words stay unfaded, in their
own colours, as they're the account's own, cut by whole clauses from the end where
`✕ out of the pool` needs the room: `● at its cap · back 17:10`. An account closed by a cap or a
limit is never faded for it: one the pin names, at its limit, is red.

**Compaction.** No view changes shape by account count, so the Switchboard stays a switchboard: as
rows run short, its lines compact, as the Overview's sections fold. It takes the first step whose
lines end seven rows above the key line or higher, row h − 7 on a terminal h rows tall, which leaves
two blank rows, the key on the next two, and the row over the key line:

1. Each line as above.
2. No blank rows within a line: its name row and a row a window, then a blank row
   (settled in the plan).
3. Two rows a line, and no blank rows between lines: the name row with its state words; then
   `5-hour` at 87, its bar from 98, 22 cells, a cell and its percentage; three cells on, `Week` at
   128, its bar from 133, 22 cells, a cell and its percentage, ending at the last column. The
   even-pace and cap marks stay; the words and the model windows go, as `usage` and the Overview
   show them.
4. One row a line: the number, name and marks, cut to the name column; from it, the 5-hour bar, 14
   cells, a cell and its percentage; and 21 cells after it, the state words (settled in the plan).
5. Step 4's lines, scrolling, the calls with them, as Accounts scrolls (see Accounts): the key gives
   way, and the row over the key line says what's out of view, right-aligned: `▼ 3 more accounts`
   (settled in the plan).

Where no line grows past its rows and no group of calls moves its line down, each row so taken being
one fewer, steps 1 to 4 hold 3, 5, 12 and 25 accounts at 160 by 37. Where the lines are narrower
than 74 columns (below), step 3's two bars share the lines' width, `⌊(width − 30) ÷ 2⌋` cells each,
and step 4's state words take the short form, then lose whole clauses, as they must.

**One account** stays a switchboard: each cord ends at a jack of its own, a jack a row, the line
growing a row for each session past its rows. There's no pool row.

**Narrower and wider.** The lines stay at the right edge. Wider than 160 columns, the cords take the
extra. Narrower, the bends' room gives way first, the lines keeping their 74 columns, until it's 9,
at 145 columns; below that, it keeps 9 columns at least, and the calls and the lines' bars narrow
instead, a cell for each column lost. A step that frees several cells at once is taken where the
bends' room would otherwise fall below 9, and what it frees beyond that goes to the bends' room,
which gives it back a column at a time (settled in the plan):

| Columns | What gives |
|---|---|
| 159 to 145 | The bends' room, from 23 columns to 9 |
| 144 to 129 | The lines' bars, from 31 cells to 16 |
| 128 to 123 | The calls' directory, from 11 cells to 6 |
| 122 | `▸ from personal` becomes `▸ personal`, freeing 5 cells: the bends' room has 13 |
| 121 to 118 | The bends' room, from 12 columns to 9 |
| 117 | The lines' words take `usage`'s short form, 14 cells, freeing 8: the bends' room has 16 |
| 116 to 110 | The bends' room, from 15 columns to 9 |
| 109 to 102 | The lines' bars, from 15 cells to 8 |
| 101 and 100 | The account after `▸`, from 7 cells to 6 |

Each part keeps its distance from the part before it: at 100 columns, what a call is doing is at 12,
`▸` at 25 and the plug at 35, the jacks are at 56, and the lines run from 58, 42 columns. A line's
state words that don't fit take the short form, as on a phone, then lose whole clauses from the end.
Under 100 columns, or wherever the bent cords outnumber the bends' room's columns, the Switchboard
draws lanes.

**Lanes,** as on a phone, where cords would need width the terminal hasn't got: each account's line
with its sessions under it, in The page's phone frame under 100 columns (settled in the plan).

- **Each account:**
  - its name row: the number, name and marks, cut to the name column, fitted as `usage` fits it, 12
    to 16 cells under 80 columns and 14 to 24 from 80; from it, the Overview's short state words,
    `● at its cap · 17:10`, `● open · next`, `■ limit reached · 15:54`; and ending at the last
    column, `● pinned` or `✕ out of the pool`;
  - its windows on one row, as step 3 draws them: `5-hour` at column 1, its bar from 8, a cell and
    its percentage; three cells on, `Week`, its bar 5 cells after it, a cell and its percentage,
    ending at the last column; each bar `⌊(width − 26) ÷ 2⌋` cells, 13 at 52;
  - its sessions, a row each: the id at column 2, bold `text.primary` while the session works, else
    `text.muted`; a space and the directory, cut with `…` to 11 cells; what it's doing at column 20;
    and `▸ from work` at 33;
  - a blank row, which the pool's rule takes after the last account.
- **The foot:** the pool's rule across the whole row; `Σ pool` under it; on the row under that, from
  column 1, the room in each window, `5-hour 1.5 of 3 left   Week 1.2 of 3 left`; then, after a
  blank row, the Overview's phone key on two lines,
  `┃ even pace: where you would be now, used evenly`, and `╎ cap: work at 95%   ◆ primary   ▲ new`.
- **No cords, no travel:** the state words carry the motion, bold while the session works.
- **The heading:** `SESSIONS` in `text.subtle`, two spaces and the sub-tabs, with no `[ ]` and no
  rule, as on the Log's phone (see The Log).

**The List.** Today's sessions in one table, a row each, even one whose models are on two accounts
(settled in the plan); an earlier day's are found through the Log's `/` (see The Log). Its heads are
on row 6 in `text.subtle`, each at its column's start, or right-aligned where its column is, and its
rows run from row 7 (settled in the plan):

| Column | From | Width | Shows |
|---|---|---|---|
| `session` | 2 | 17 | The id, cut to 4, in `text.secondary`; a space and the directory, cut with `…` to 12, in `text.muted` |
| `on` | 21 | 8 | The account its latest request went to, in `text.primary` |
| `model` | 31 | 9 | The model of its latest request, its id less `claude-`, as `opus-5-5`, in `text.muted` |
| `state` | 42 | 11 | `asking` or `answering`, bold `accent.key`; `✕ 429`, bold `state.destructive`; `idle 34s` in `text.muted`; once over, `ended 11:21` in `text.muted` |
| `started` | 55 | 15 | When its first request came: `12:47` today, `yesterday 15:08`, `Tue 15:08` within the week, `28 Sep 15:08` before it (settled in the plan), in `text.muted` |
| `requests` | 72 | 8, right-aligned | Its requests today, as `1,240`, in `text.muted` |
| `worth` | 82 | 7, right-aligned | Theirs at API prices, as `$17.86`, in `text.muted`; unpriced as the Log shows it |
| `routing` | 91 | To the last column | How it came to the account it's on, in the Log's routing words: `from ` in `text.muted`, the account it left in `text.secondary`, `, at its cap` in `text.muted`, two spaces, and in `accent.attention` what writing its context again cost, `$1.82`; `pinned here` in `text.muted` for a session pinned on its own (settled in the plan); blank where it started there |

- **The marks,** at column 0, bold, the first that applies, as the Log's: `↑` in `accent.key` while
  it asks; `↓` in `accent.key` while it's answering; `▸` in `accent.key` where it moved to the
  account it's on. Else none.
- **The order:** the running sessions first, in the order they started, the oldest first, a new one
  joining their foot; then `ended today` in `text.subtle` from column 2, two cells, and a rule of
  `─` in `border` to the last column; under it, today's ended sessions, the newest end first. The
  order stays still, never reshuffled by activity, as rows would slide from under the cursor: a row
  moves only as its session ends, from running to the top of ended, or as it asks again, back to its
  place by start among the running.
- **The `today` line,** on the row over the key line, right-aligned to the last column:
  `today  14 sessions · 1,240 requests · $126.02 at API prices`, `today` in `text.subtle`, the
  counts in `text.secondary`, the worth bold `text.secondary`, and the rest in `text.muted`. It sums
  today's sessions, running and ended, their requests today and their worth.
- **Past a screen,** the table scrolls, its heads held, the row above the `today` line blank and the
  line held, as the Log's table does (settled in the plan).
- **Narrower,** `routing` is cut to the room left, as the Log cuts it, `…` in the colour of what it
  cuts and a move's cost kept whole, down to 24 cells, at 115 columns, and left out below that.
  Under 100 columns, as on a phone, the List keeps its mark and four columns: `session` from 2, 16
  wide, the directory cut to 11; `on` from 19, 8 wide; `state` from 29, 11 wide; and `requests` from
  44, 8 wide, right-aligned. Where the `today` line doesn't fit its row, it breaks after `requests`,
  the worth on the row over the key line and the rest on the row above, each right-aligned.

**Keys.** Where a reference's key row differs from these, these hold:

| Where | Key line |
|---|---|
| Both sub-tabs | `tab views   [ ] view    ↑↓ session  enter its page    p pin    ? help  q quit` |
| A phone | `↑↓ session   enter open   [ ] view   ? keys   q quit` |

- `[` and `]` switch between Switchboard and List.
- `↑` and `↓` choose a session, a call on the Switchboard or a row on the List, on `bg.selection`
  across the width, its cord unchanged. Nothing is chosen until they're pressed; the choice follows
  its session from look to look, and `esc` ends it. A session with a row in two groups is chosen on
  either (settled in the plan).
- `enter` opens the chosen session's page (see A session's page), the tab staying lit, and `esc`
  comes back. An ended session's page is as ended, with no `p` (settled in the plan).
- `p` opens new sessions' picker, whatever is chosen (see Routing by hand); a session's own routing
  is set from its page, `enter` then `p`. With one account, `p` does nothing and no key line shows
  `p pin`, unless a pin set from the command line holds, when `p` opens the picker to clear it.
- `j`, `k`, PgUp, PgDn and the wheel scroll a view that scrolls; `R` and `t` are as on every page
  (see Keys).
- `?` help (see The page), `HELP · SESSIONS`, lists the keys, `t`, `R` and `j`/`k` among them, and
  the marks: `●━━◉` a session's cord, `○` a free jack, `╌` a cord a limit let go, and `↑`, `↓` and
  `✕` what a request is doing (settled in the plan).

**Other states.**

- **The router not answering:** the status line says so (see The page); the Switchboard draws from
  the last document, nothing travelling, and `p pin` is faint (settled in the plan).
- **A look that can't blend,** the terminal's own colours or `NO_COLOR` (see Themes): an idle cord
  is drawn light, `─ ┐ │ └`, and a glow of 0.5 or more as `text.primary`. A light theme's glows and
  dimming are mixed against its own canvas (settled in the plan).
- **An account not read yet,** or whose token is refused: its line's state says `○ not read yet` in
  `text.subtle`, or `■ refused (401) · its token needs renewing` in `state.destructive`
  (settled in the plan).
- **Long names** are cut with `…` to the name column, the number and marks kept, as `usage` cuts
  them (settled in the plan).

**What it reads.**

- **The calls and the List's running sessions:** `GET /sessions`, the sessions routed in the last
  hour, each assignment's account, `last_seen` and `dir`, as the stream's events carry `dir` too
  (settled in the plan; see Control API); from a router that gives no `dir`, its lines' `dir`, else
  the id alone.
- **What each is doing, and what travels the cords:** the request stream, as the dashboard keeps it
  (see Live updates): a pulse out on `sent`, the shimmer from `first` to `done`, a pulse back on
  `done`, red on `limited` or `refused`, `… 429` on `throttled`, and the move on `moved`; and the
  `inflight` it opens with, so a view opened mid-answer shows it. Without the stream, the calls are
  drawn as each look reads them, nothing travelling. A router restarted, or another, has the
  dashboard forget what the last one's stream told.
- **`idle`:** since its last answer ended, as the stream told; else since the router's `last_seen`.
- **`▸ from`:** the stream's `moved`; for a move from before the view opened, the session's latest
  line today with a `from`, while the session is still on that line's account (settled in the plan).
- **The stubs:** the router's `limit` event, its `count`, while its limit holds.
- **The lines, their state, and `● pinned`:** the status document, as `usage` and the Overview read
  it, and its `pin`.
- **The List's requests, worth, start, end and routing:** the request ledger, read where it lies,
  with no router (see The request ledger). A session's requests are its lines today that spend
  quota; its worth, theirs priced by the dated table at today's prices; its start, its first line,
  on the first day the summaries' session ids give it; its end, its last line's. A session ends as
  `GET /sessions` stops listing it, an hour after its last request, and runs again as it asks again
  (settled in the plan). Today's lines are read as they're appended, never afresh at each look.
- **Agents** read the same through the data verbs: `sessions` lists the running sessions and today's
  ended ones, the List's own function (see The data verbs).

### Runway

When each account has room, as a timeline: a lane per account, then the pool's lane under the pool's
rule, over the window the view is named for. The frames are in the Paper file *Switchboard ·
Sessions and Runway*, on the page **FINAL · Sessions and Runway · signed off · 8 Oct 2026**; the
references are `testdata/vhs/reference/runway-*`: `runway-5-hour-160`, `runway-week-160`,
`runway-stepped-back-160`, `runway-ten-accounts-160` and `runway-phone-52`. Columns and rows are at
160 by 37 unless they say otherwise.

**The heading row,** row 4: `RUNWAY` in `text.subtle`; a rule of `─` in `border` from two cells
after it to two cells short of the window; and at the right edge, the window between its keys, as a
period sits between its keys on Accounts and History: `<` bold `accent.key`, the window's name,
`5-hour` or `week`, with a space either side, in `text.muted`, and `>` bold `accent.key`, ending at
the last column. Stepped back, `RUNWAY` is followed by three spaces and the day shown, `Wed 30 Sep`,
in `text.faint`, the rule starting a cell after it.

**The views,** named by their windows, `<` and `>` stepping between them, each account's use of the
view's window under its name:

- **5-hour,** the first the dashboard shows: from the hour before now, taken back to its ten
  minutes, 13:40 at 14:42, for 22 hours 40 minutes: 136 columns of 10 minutes.
- **week:** from a day before now, for a week: 136 columns of 74.1 minutes, its 10,080 minutes
  shared among them (settled in the plan).
- At other widths, the lanes keep their span, and the minutes a column scale with their width.

The window shown is kept in the preferences file; a step back never is, so the dashboard opens live
(settled in the plan).

**Stepping back.** `←` steps back: in the 5-hour view, first to today from its midnight, then a day
at a time; in the week view, first to this week from its first day, as `week_starts` begins weeks
(see Config), then a week at a time (settled in the plan). `→` steps forward, and from today, or
this week, to the live view. Nothing lies ahead to step to, as a 5-hour window that hasn't started
can't be forecast, and the week view holds what's ahead. `<` and `>` show the live view of the
window they step to (settled in the plan). `←` reaches back as far as the day summaries, which are
kept for good (settled in the plan; see The request ledger).

**The lanes and the axis.**

- **The lanes** run from column 24 to the edge, 136 columns.
- **The labels,** in the 22 cells before them: on a lane's row, the account's number, name and
  marks, cut with `…` to fit, the number and marks kept (see `usage`, the printout); on the row
  under it, from column 2, `5-hour ` or `week ` in `text.subtle`, and the account's use of that
  window, `95%`, bold in its level's colour, as `usage` colours it: red from 90%, yellow from 70%,
  else green.
- **The axis:** on row 6, `now`, bold `text.primary`, starting a cell left of now's column; on row
  7, the labels; on row 8, the ticks, on a rule of `─` in `border`.
  - **5-hour:** hours ticked `┬` in `border`, on the first of 1, 2, 3, 6, 12 and 24 hours that puts
    the ticks 3 cells apart or more, and labelled from their ticks, `16:00`, in `text.muted`, on the
    first that puts the labels 10 apart or more, where a label ends before the lanes' last column:
    every hour ticked and every other hour labelled at 160 columns. A day's midnight is labelled
    with its weekday, `Fri`, bold `text.secondary`, and no hour's label comes within 10 cells of it.
  - **week:** each midnight ticked `┬`, its weekday over it where that fits, today's bold
    `text.secondary` and the rest `text.muted`; every six hours between, where they're 3 cells apart
    or more, `╵` in `text.faint`.
  - An hour the clocks skip, going forward, is neither ticked nor labelled, and a day starts where
    the clocks put its midnight: where they went forward over it, at the hour they went forward to,
    and where they went back over it, at the first of its two.
- **Now's column** is picked out in `bg.subtle`, from the axis's first row to the pool's words, on
  every cell without a background of its own; a part-filled column's gradient takes it in place of
  the canvas.
- **The past is dimmed:** every colour before now's column is `mix(canvas, colour, 0.45)`.
- **A lane** is a cell a column, by the account's state at the column's middle, or at now for now's
  column, in the colours `usage` gives each state:

  | State | Glyph | Colour |
  |---|---|---|
  | Room | `▆` | The ramp's first stop |
  | Room, but it runs out at its pace before its window resets | `▆` | `accent.attention` |
  | At its cap | `─` | `accent.attention` |
  | At its limit | `─` | `state.destructive` |

  Every window that every model shares counts, but the 5-hour view leaves out a week running out:
  its lane is room until the week's limit. `│` in `text.subtle` marks where the view's window
  resets, on now's column or after it: each account's 5-hour window's reset in the 5-hour view, its
  week's in the week view. An account without a usable token has no room all along, and one not read
  yet is left blank (settled in the plan).

**The words under a lane** say where each stretch without room starts, from its column, 24 at the
least, or two cells after the words before:

- at a cap: `at its cap 14:31` bold `accent.attention`, then `  ·  back 17:10` in `text.muted`;
- running out ahead: `runs out ~16:05 at its pace` bold `accent.attention`, then `  ·  back 16:55`
  in `text.muted`;
- at a limit: `at its limit 13:58` bold `state.destructive`, then `  ·  back 15:30` in `text.muted`.

`, as its 5-hour resets`, or `, as its week resets`, follows a cap's or a run-out's `back …` where
its stretch ends as the view's window resets; a limit's `back …` takes none. In the week view, an
account whose week reaches no limit says `resets Sun 21:00` in `text.muted`, ending at the reset's
`│`, where that clears the words before. Else a lane says `room all day`, or `room all week`, in
`text.subtle`, from the cell after now's column. Words that don't fit drop whole parts from the end,
`, as its 5-hour resets` first. A stretch already over, or one that starts past the view, isn't
said.

**The pool's lane** sits under the lanes, under the pool's rule, one unbroken `─` in `border` across
the whole row, as `usage` draws it: with three rows an account, the rule takes the last account's
blank row; with two, it takes a row of its own.

- **Its columns,** two rows of them, a column for each of the lanes': how many of the pool's
  accounts have room, the column's height `with room ÷ accounts`, its full cells background and its
  top cell part-filled, as GRAPHS draws its columns (see The Overview). They're coloured as `usage`
  colours the pool's use, `1 − with room ÷ accounts`: green under 70%, yellow from 70% and red from
  90%; the past dimmed. An account at its cap or its limit counts without room; one running out at
  its pace has room until it does.
- **`Σ pool`** on its second row: `Σ` in `text.subtle` in the numbers' column, and `pool` in
  `text.secondary`.
- **Its room,** on the row under it, from column 2, the pool's room in the view's window, in
  `usage`'s words for the pool: `5-hour ` or `week ` in `text.subtle`, `1.5` bold `text.secondary`,
  and ` of 3 left` in `text.muted`.
- **Its words,** on that row, two cells after the room at the least: a phrase for each dip, a
  stretch where fewer than all the pool have room, in time order, each from its dip's first column,
  or two cells after the phrase before.
  - A phrase is the count, `8 of 10`, in `text.secondary`, then, in `text.muted`,
    ` with room till 15:30` for a dip under way, ` with room ~16:05 – 16:55` for one ahead, or
    ` with room 15:02 – 17:10` on a day stepped back to.
  - Each change of the count within a dip chains on, each count in `text.secondary` as the first:
    `8 of 10 with room till 15:30, 9 till ~16:05, 8 till 16:55, 9 till 17:10`.
  - `~` marks a moment a run-out projects, a cap or a limit still ahead; a reset is known, so takes
    none.
  - A dip too close to the phrase before for a phrase of its own, starting less than two cells after
    it, joins it with `and` where its count is the same: `2 of 3 with room 15:02 – 17:10 and 19:12 –
    23:58`.
  - Words that don't fit drop whole parts from the end, and a dip whose first stretch doesn't fit
    isn't said. In the live views, a dip already over isn't said.
- **The pool's accounts** are every account on auto, and the pin's while one holds: its room is then
  `of 2`, the room's words add ` · pinned`, as `usage`'s pool row does, and an account out of the
  pool has its lane, label and use faded as Sessions fades its line (see Sessions), the words under
  its lane not (settled in the plan).

**The legend,** on the row after the pool's words and a blank row, from column 0, keys only what the
view draws, four spaces apart: `▆` and ` room`; `▆` in `accent.attention` and
` room, but it runs out at its pace`; `─` in `accent.attention` and ` at its cap`; `─` in
`state.destructive` and ` at its limit`; and `│` and ` its 5-hour resets`, or ` its week resets`;
each glyph in its colour, its words in `text.muted`. Four spaces on, in `text.subtle`, a column's
length and what's dimmed: `a column 10 minutes · the past dimmed`; in the week,
`a column ≈ 74 minutes · the past dimmed`; stepped back,
`a column ≈ 10.6 minutes · under each name, the day's highest`. A column's length is in whole
minutes where it's whole, else after `≈`, to a tenth of a minute under 20 minutes and to the minute
from 20 (settled in the plan). Where the look can't blend, the past isn't dimmed, and the legend
doesn't say it is (settled in the plan).

**COMING UP,** in `text.subtle` at column 0, on the row after the legend and a blank row, with its
lines under it, the Overview's COMING UP rows, 72 wide (see The Overview): the time, padded to the
longest, in `text.secondary`; two spaces, the account bold `text.primary` and what happens in
`text.muted`; and, right-aligned to end at column 71, how long until it, `in 1h 48m`, in
`text.subtle`; a run-out's row all `accent.attention`, its time and countdown included. Its lines
are the accounts' spans and resets, soonest first:

- an account back from its cap or its limit, where one holds it now:
  `17:10  work back from its cap`;
- a window running out at its pace: `~16:05  lab's 5-hour runs out at its pace`;
- each other reset of the view's window, where its `│` stands: `16:30  personal's 5-hour resets`.

The 5-hour view lists the 5-hour windows'; the week view adds each week's run-out and reset. No
prime is listed (settled in the plan). The lines aren't capped: where they don't fit, Runway
compacts.

**A day stepped back to** is a calendar day, midnight to midnight across the 136 columns, about 10.6
minutes a column.

- Its hours are ticked and labelled by the rule above, every hour ticked and every other hour
  labelled at 160 columns; its first label is the day's name at its midnight, `Wed`, bold
  `text.secondary`.
- No `now`, nothing dimmed and no COMING UP, as all three are about now; no room line either, as the
  room is about now.
- Each lane is as the day had it, with `│` wherever its window reset that day, and the words of its
  stretches from column 24 at the least: `room all day` from column 24.
- Under each name, the day's highest use of the view's window, `5-hour 100%`, as the legend says.
- The pool's lane counts the pool as it is now, and its words take the past form:
  `2 of 3 with room 15:02 – 17:10 and 19:12 – 23:58`.
- The key line is the live view's.
- **Today, from its midnight,** the first step back, is drawn as a day stepped back to, but with
  now's column picked out, what's past dimmed, and the room line kept; still with no COMING UP.
- **A week stepped back to** runs from its first day, as `week_starts` begins it, to its last,
  across the 136 columns, ticked and named as the week view is, under each name the week's highest
  use; this week, the first step back, has now picked out, as today does (settled in the plan).

**Compaction.** As rows run short, Runway takes the first layout whose last row is two rows above
the key line or higher, row 34 at 37 rows, as the blank row under the heading and the one over the
key line never go. For N accounts and K lines of COMING UP, on a terminal h rows tall:

| Step | What changes | Holds if | At 160 by 37 |
|---|---|---|---|
| — | Three rows an account, its lane, its words and a blank row; blank rows under the axis and round the legend and COMING UP | 3N + K ≤ h − 19 | 4 accounts |
| (a) | Two rows an account; the pool's rule on a row of its own (settled in the plan) | 2N + K ≤ h − 20 | 5 |
| (b) | No blank rows round the legend and COMING UP (settled in the plan) | 2N + K ≤ h − 18 | 6 |
| (c) | COMING UP folded to its heading (settled in the plan) | 2N ≤ h − 18 | 9 |
| (d) | No blank row between the axis and the lanes | 2N ≤ h − 17 | 10 |
| (e) | The lanes scroll (settled in the plan) | | 11 and up |

The counts at 160 by 37 hold with a line of COMING UP an account, or one more. Ten accounts take
(d): the lanes on rows 9 to 28, the rule on 29, the pool's lane on 30 and 31, its words on 32, the
legend on 33, and COMING UP, folded, on 34.

- **COMING UP folded,** as the Overview folds a section, keeps its heading, saying what it holds, on
  the row under the legend: `COMING UP` in `text.subtle`, three spaces, the soonest's time in
  `text.secondary`, two spaces, the account bold `text.primary`, what happens in `text.muted`, and
  `  …` in `text.subtle`, as in `COMING UP   15:30  team back from its limit  …`.
- **At (e),** the lanes scroll as Accounts' do (see Accounts): the axis and the pool's foot hold,
  and the row over the key line says what's out of view, right-aligned: `▼ 1 more account`.

**One account:** its lane alone, with no pool's lane, rule, `Σ pool` or room line, as they'd only
repeat the account (settled in the plan).

**Under 100 columns,** in The page's phone frame (settled in the plan):

- the label column is 14 cells: names cut to 12, as `2 perso… ◆`, their use under them;
- the lanes take the other columns, 38 at 52: the 5-hour view's 22 hours 40 minutes at about 35.8
  minutes a column, ticked every 2 hours and labelled every 6, `18:00`, `Fri`, `06:00`, by the rule
  above;
- the words are cut by the rule above, as `at its cap 14:31  ·  back 17:10`;
- the pool's lane, then its words, with its room on the row under them, from column 2, as the label
  column is too narrow;
- the legend on two lines, the keyed glyphs, then `a column ≈ 36 minutes · the past dimmed`;
- COMING UP, the full width, after a blank row;
- the heading keeps its rule and the window at the right.

**Keys.** Where a reference's key row differs from these, these hold:

| Where | Key line |
|---|---|
| 5-hour, live or stepped back | `tab views   < > window  ←→ day    p pin    ? help  q quit` |
| week, live or stepped back | `tab views   < > window  ←→ week    p pin    ? help  q quit` |
| A phone | `tab views   < > window   ←→ day   ? keys   q quit`, `←→ week` in the week view |

`<` and `>` step between the 5-hour and the week views; `←` steps back and `→` forward; `j`, `k`,
PgUp, PgDn and the wheel scroll the lanes at (e); `p`, `R`, `t`, `?` and `q` are as on every page
(see Keys). `?` help (see The page), `HELP · RUNWAY`, lists the page's keys first, `t`, `R` and
`j`/`k` among them; then it explains the lanes' marks and colours, the pool's lane and its words,
stepping back and how far it reaches, the minutes a column, and COMING UP.

**Other states.**

- **A stale account:** its lane faded as the Overview fades a stale account, keeping 35% of its
  colour, which is 65% toward the canvas (see The Overview), its use under its name
  `not read since 14:20` (settled in the plan).
- **The router not answering:** Runway draws from the last document, its projections going from its
  `generated_at` (settled in the plan).

**What it reads.**

- **Room, a cap and a limit:** the status document's windows, their `utilization`, `resets_at` and
  `status`, and the account's `at_reserve` (see The status document), at each look. The `now`
  column, the clock and the countdowns move every second.
- **When a limit or a cap began:** the router's `limit` and `cap` events' `at`
  (settled in the plan). A hold whose start isn't known runs from before the timeline.
- **A run-out, `~16:05`:** the projection, as `usage` projects (see `usage`, the printout).
- **COMING UP:** the same, soonest first, as the Overview's COMING UP reads it.
- **The pool:** its accounts, every one on auto, or those the `pin` names.
- **A day stepped back to:** the readings history, each change of a window's use, its reset and its
  status, a limit running from a reading `rejected` to its window's reset; the day's summary's
  `limits`, each with its `at` and `resets_at`, and under each name its `highest`; today's, before
  it's summarised, its readings and lines as they're read (see The request ledger). A cap on a day
  gone is drawn from the reserve the config gives now, as the readings keep none
  (settled in the plan). A day older than the readings history's keep draws its lanes from its
  summary's limits and resets alone (settled in the plan).
- **Agents** read the room, the even pace, the pool's room and COMING UP in the status document,
  through the data verbs (see The data verbs).

### The Log

The router's traffic, as it happens and back through the days, in two tables under one tab:
**Requests**, a line for each request the router routes, and **Events**, a line for each thing the
router decides or notices (see The router's events). An event is a moment between requests, never a
container of them; every move the router makes is among them, with why. The newest line is at the
foot. The Log is drawn in the Paper file *Switchboard · Session and Log*, on its page **FINAL · Log
· signed off · 7 Oct 2026**, its frames kept as `testdata/vhs/reference/log-*`.

- **Its rows,** at 160×37: the title row (0) and the status line (2), as every page has them (see
  The page), the Log's tab lit; the heading row (4); the column heads (6); the table (7 to 33), 27
  lines; the state line (35); and the key line (36). Rows 5 and 34 stay blank, so the table has room
  above it and below. Only the table scrolls; at other heights it takes the rows the rest leave, 15
  at the least: the ten around the table, and five of it (settled in the plan). Below 15 rows, it
  keeps its 6 fixed rows, the title row, the status line, the heading row, the column heads, the
  state line and the key line, and 5 of its table at the least.
- **The heading row:** `LOG` in `text.subtle`; three spaces; the sub-tabs, ` Requests ` and
  ` Events `, the one shown bold `text.primary` on `bg.subtle`, the other `text.subtle`; a space and
  `[ ]` in `text.faint`, as Accounts' and History's sub-tabs are drawn (see The page). While a
  filter is on, three spaces and what it keeps, in `text.faint`: `only d28c switchboard`, several
  named after `only`, ` · ` between (`only d28c switchboard · side · failed`), and cut with `…`
  where they run long (settled in the plan). Nothing else describes the page, nor a column shown: a
  column shows by being there. Then a rule of `─` in `border`, from two cells after `[ ]`, or one
  after what's filtered, to two cells short of the count, and the count, ending at the last column:
  - on Requests, `35 hidden` in `text.secondary` and `: checks and counts` in `text.muted`, the
    lines hidden by default (below); filtered, `830 hidden` alone, as it is once more than one thing
    hides lines (settled in the plan);
  - on Events, `20` in `text.secondary` and ` today` in `text.muted`, today's events, or
    `N of M today` with some hidden (settled in the plan);
  - then three spaces, `f` bold `accent.key` and ` what it shows` in `text.muted`.
- **The column heads,** in `text.subtle`, each at its column's start, or ending at its end where the
  column is right-aligned.
- **The lines:** the oldest at the top and the newest at the foot, back through the days as the
  table scrolls. A line's mark, where its kind has one, is at column 0, bold, and the line starts at
  column 2. Times are local, to the second, `14:40:26`, the whole time on every line, in
  `text.muted`. The line chosen is on `bg.selection` across the whole width, its colours as they
  are. Scrolling back reads a day at a time, the day rule between, and at the oldest day kept, a
  line says how long the ledger keeps its lines, or that it keeps them for good
  (settled in the plan).

**Requests.** A line a request, its columns two cells apart:

| Column | From | Width | Aligned |
|---|---|---|---|
| `at` | 2 | 8 | left |
| `session` | 12 | 17 | left |
| `model` | 31 | 9 | left |
| `on` | 42 | 8 | left |
| `status` | 52 | 6 | left |
| `first` | 60 | 5 | right |
| `took` | 67 | 6 | right |
| `read` | 75 | 6 | right |
| `written` | 83 | 7 | right |
| `out` | 92 | 5 | right |
| `stopped` | 99 | 27 | left |
| `routing` | 128 | 30 | left |

- **`at`:** when it arrived. **`session`:** its id cut to 4 in `text.secondary`, a space, and the
  last part of its directory, the ledger's `dir`, cut with `…` to 12, keeping its start
  (settled in the plan), in `text.muted`: `d28c switchboard`. **`model`:** its id less `claude-`, in
  `text.muted`. **`on`:** the account whose answer the client got, in `text.primary`, cut with `…`
  past 8 cells (settled in the plan), as account names are white wherever they're named.
- **`status`:** as answered, in `text.muted`; 400 and over bold `state.destructive`. **`first`:**
  the time to its answer's first byte, `0.8s`; **`took`:** its whole time, `11.8s`, from a minute
  `1m 05s`; both in `text.muted`.
- **`read`, `written` and `out`:** the tokens it read from the cache, wrote to it, and put out, in
  `text.muted`, `—` in `text.faint` for none: under 1,000 as they are; under 10,000 in thousands to
  one place (`5.4k`); under a million in whole thousands (`958k`); then in millions to one place
  (`5.7M`, `16.1M`). A request that moved its session wrote its whole context on the new account:
  its `written` is bold `accent.attention`.
- **`stopped`:** why its answer stopped, in `text.muted`, `end_turn`; for `tool_use`, then ` · ` in
  `text.faint` and the tool it called in `text.secondary`: `tool_use · Bash`.
- **`routing`:** how the router chose its account, from the line's `reason` and `from`, in the
  views' words (see The router's events); blank for a request that stayed where its session was.
  A move, on the request that moved: `from ` in `text.muted`, the account it left in
  `text.secondary`, why in `text.muted` (`, at its cap`), two spaces, and in `accent.attention` what
  writing its context again is worth, `from work, at its cap  $1.82`: its cache write, priced
  exactly, so with no `≈`. A session's first request: `new session` in `text.muted`.
- **`worth`,** 6 wide, right-aligned, off until `f` shows it: what the request would have cost
  through the API, `$0.15`, `<$0.01` under half a cent, `unpriced` for a model the price table
  doesn't know, in `text.muted`. Shown, it goes between `out` and `stopped`, moving the two after it
  along by 8.
- **Nothing past the last column:** a column that would run past it is cut to the room left, as
  `routing` is with `worth` shown, to 24 cells from 136. What doesn't fit is cut with `…` in the
  colour of what it cuts, a move's cost kept whole: `from work, at it…  $1.82`. Below 160 columns,
  columns leave in this order: `routing` folds into `stopped`, as the phone's `new session` does;
  then `first`, `read`, `written`, `out`, `model` and `status`, a session's directory going before
  its id. Past 160, `stopped` and `routing` take the room (settled in the plan).
- **The marks,** bold, the first that applies: `↑` in `accent.key`, in flight and asking, before its
  answer's first byte; `↓` in `accent.key`, in flight and answering; `■` in `state.destructive`,
  failed, 400 and over; `▸` in `accent.key`, it moved its session (its line has a `from`); `▲` in
  `accent.mode`, a session's first request; none on a request that stayed and succeeded.
- **In flight,** a request sits where it arrived, as the request stream tells it, and fills in where
  it is as its line is written, so lines never move. `status`, `read`, `written` and `out` are
  blank, the stream giving a request's tokens only as it ends, and `first` blank while it asks;
  `took` counts, in `accent.key`; `stopped` says `asking`, bold `accent.key`, or `answering`, bold
  `accent.key`, then ` · ` and its characters so far in thousands to one place, `5.1k characters`,
  in `text.muted`.
- **Hidden until shown:** Claude Code's quota checks and its token counts, the ledger's kinds
  `check` and `count`. The heading counts them, and `f` shows them: then each has `·` in
  `text.faint` for a mark, `stopped` blank, and its tokens as they are (settled in the plan).
- **Requests no frame draws:** one sent again after a limit has its `tried` in `routing`
  (`after work's limit`); one every account refused, its 429 passed on, reads as failed; one
  answered by the router itself has no `first`; one cut off or given up, `status` 0, reads as
  failed; several tools show the first and `+1`; no session, `—`; no directory, the id alone
  (settled in the plan).

**A line's details.** `enter` opens the chosen line's details in the routing pickers' card (see
Routing by hand), over the page faded, the line it's of left bright; `esc` closes it. A line's
details open in a card; a line's own list opens in place, as a session's turns open into their
requests (see A session's page). A request in flight's card holds what the stream tells, the rest
filling in as its line comes (settled in the plan).

- **Opening pauses the table,** so the line stays where it is as lines come (the state line, below);
  the lines behind the card run on to the table's foot.
- **The card:** a rounded line, `╭─╮ │ ╰─╯` in `text.subtle`, round nothing filled, the page behind
  it faded 80% toward the canvas and nothing bold, but for the line it's of, which stays as the
  table drew it, lit on `bg.selection` across the width, its colours and its bold kept. It's 128
  cells wide, centred, columns 16 to 143. Its title is in its top edge after `╭─ `, in
  `text.subtle`, ` · ` between its parts: a request's `REQUEST · 14:31:13 · d28c switchboard`, the
  line's time and its session as the table names it; an event's `EVENT · 14:31:03 · work reached
  its cap`, what happened as its `event` item says it. An account a title names is `text.primary`.
- **Inside,** from column 19: a blank row under the edge; the tree; a request's foot; a blank row;
  the keys, on the card's last row but one; then its bottom edge. Its height is its contents': its
  tallest column of the tree, and 6 rows more with a foot, 5 without: 21 rows for a request that
  moved its session, whose tallest column is 15, and 11 for a cap, whose tallest is 6.
- **Where it sits:** under its line, a row between, where its bottom edge is on row 33 or above;
  else over its line, a row between. Stepping with `↑↓`, it's placed by the same rule at each step;
  where it fits neither under its line nor over it, the table scrolls the line up until it fits
  under (settled in the plan).
- **The tree,** in three columns, from columns 19, 69 and 94, each holding sections one under
  another, a blank row between. A section's heading is uppercase in `text.subtle`, as a group's is
  on every tab, an account it names `text.primary` (`SIDE'S WINDOWS AFTER IT`, `WORK THEN`). Its
  items follow, a row each: `├ ` in `text.faint` (`└ ` for its last), the item's name padded to 13
  in `text.muted`, its value in `text.secondary`, words around a value in `text.muted`. A value too
  long for its column's room is cut with `…`, as `req_011CQx…` is. The card's edge frames the tree,
  so there's no tree line down its left.
- **A request's sections,** each item from its line in the ledger (see The request ledger):

  | Column | Section | Items |
  |---|---|---|
  | 19 | `REQUEST` | `id`, the router's `request`; `Anthropic's`, `answer.id`; `from`, `agent` cut to its name and version (`claude-cli/2.1.0`); `betas`, `betas` without their dates (`context-1m, interleaved-thinking`) |
  | 19 | `ROUTED` | `to`, `account`; `from`, the account it left and why (`work, at its cap`); `upstream`, `attempts` in words, `once`, `twice`, then `3 times` (settled in the plan) |
  | 19 | `<ACCOUNT>'S WINDOWS AFTER IT` | from `limits`, as the answer left them: `5-hour` and `week`, each use bold in its level's colour (see `usage`, the printout), and a model's window where the answer gave it; `status` |
  | 69 | `ASKED` | `model`; `size`, `shape.bytes` (`482 KB`); `messages`, `system` (`3 blocks`) and `tools`, `shape`'s counts; `max_tokens`; `thinking`, its budget; `effort`, `output_config`'s; `stream`, `yes` or `no` |
  | 94 | `ANSWERED` | `status`; `model`, `answer.model`, the one that served it; `first byte`, `first_ms`; `ended`, `total_ms`; `stopped`, `answer.stop` and its tools as the table words them; `blocks`, `answer.blocks` (`2 thinking, 1 text, 1 tool use`) |
  | 94 | `TOKENS` | `in`, `usage.input_tokens`; `from cache`, the cache reads; `cached`, the cache writes, bold `accent.attention` where the request moved its session, then their lifetime in `text.muted` (` for an hour`, ` for 5 minutes`, from `usage.cache_creation`); `out`; `worth`, bold `text.secondary`, then ` at API prices` in `text.muted`; `the move`, its cache write priced, bold `accent.attention`, then ` of it` in `text.muted` |

  `from` in `ROUTED`, and `the move`, belong to a request that moved its session, and are left
  out of any other, which makes its card a row shorter.
- **A request's foot,** on the row under the tree, from column 19, in `text.faint`: `never kept: the
  messages, the system prompt, the tools' definitions, their inputs and results`, as the ledger
  keeps none of them.
- **An event's sections:** **WHAT HAPPENED**, its `event` (what happened, as its line says it) and
  what the kind adds; **`<ACCOUNT>` THEN**, the account's windows as they stood, their reset, and
  the sessions on it; and **WHAT FOLLOWED**, where something did. An event holds no request, and has
  no foot. Every kind's card follows the cap's, below; one that touched no request, as a prime, a
  window reopening, a pin or a restart, has no **WHAT FOLLOWED**, and `g` still shows its moment, at
  the first request at or after it; `sessions` in the account's section counts those whose lines
  went out on it in the hour before (settled in the plan). A cap's, work's here:
  - `WHAT HAPPENED`: `event`, `work reached its cap`; `its cap`, `95%` of its window, bold in its
    level's colour, then ` of its 5-hour window`; `known from`, the request whose answer first read
    the window at its cap, `7f3a's request at 14:30:51`;
  - `WORK THEN`: `5-hour` and `week`, each bold in its level's colour; `resets`, `17:10`;
    `sessions`, `3 on it`;
  - `WHAT FOLLOWED`: a row each session it moved, named by its id, `moved to side` and three
    spaces then what writing its context again was worth, in `accent.attention`; `new sessions`,
    `go elsewhere till 17:10`; `in all`, the moves' sum, bold `accent.attention`, then ` to cache
    them again` in `text.muted`.
- **The card's keys,** on its last row but one, right-aligned two cells in from its edge, three
  spaces apart, each key bold `accent.key` and its words in `text.muted`:
  - a request's: `↑↓ previous/next request   s its session   esc close`;
  - a session's event's: `↑↓ previous/next event   g in Requests   s its session   esc close`;
  - an account's event's: `↑↓ previous/next event   g in Requests   esc close`.

  `↑↓` steps to the line before or after, `↑` the older, its details in the card without closing,
  the lit line following and the card placed by the same rule; `s` opens the line's session's page
  (see A session's page); `g` shows Requests, paused at the event's moment; `esc` closes the card,
  leaving the table paused at the line chosen. While it's open the card owns the keys, and the
  page's key line behind it is faded with the page.
- **One open at a time:** the card shows one line's details, a request's or an event's; nothing else
  opens while it's open.

**What it shows (`f`).** A panel in the same card, 124 cells wide, as two sides don't fit the
pickers' 82, and 22 tall, centred: columns 18 to 141, rows 7 to 28. Its title, in its top edge:
`LOG · REQUESTS · WHAT IT SHOWS` or `LOG · EVENTS · WHAT IT SHOWS`. Inside, from column 21 (`x`):

- **Requests:**
  - row 9: `Pick the columns it shows, and which of today's 1,275 lines.` in `text.secondary`;
  - row 11: `COLUMNS` at `x` and `ONLY` at `x+40`, in `text.subtle`;
  - **COLUMNS,** from row 12, where the cursor starts (settled in the plan), a tick each: `■` bold
    `accent.key` and the name in `text.primary`, shown, or `□` in `text.subtle` and the name in
    `text.muted`, hidden. Those shown come first, in the table's order, six to a column at `x` and
    `x+13`; then those hidden, at `x+26`: `worth`, `input` (`usage.input_tokens`), `attempts`,
    `size` (`shape.bytes`), `messages` and `tools` (`shape`'s counts), `thinking`
    (`shape.thinking`), `agent`, and `request id`, the router's `request`. `at` always shows, and
    isn't listed;
  - **ONLY,** rows 12 to 16, its labels at `x+40` in `text.subtle` and their choices from `x+50`:
    `accounts`, every account's tick; `models`, the models of today's lines; `kinds`, `requests`,
    `checks` and `counts`, the ledger's `message`, `check` and `count`, each with today's count in
    `text.muted`; `failed`, `only those that failed`, 400 and over, cut off or given up, and today's
    count; `sessions`, `every one, unless some are ticked` in `text.muted`;
  - **today's sessions,** rows 17 to 22 from `x+52`, a row each: its tick; its id in
    `text.secondary`; two spaces and its directory's name padded to 12 in `text.muted`; two spaces
    and the account it's on padded to 9 in `text.primary`; its lines today right-aligned in 4 in
    `text.secondary`, then ` today` in `text.muted`; and at `x+96` what it's doing, `asking` in
    `accent.key`, or `started 14:41` or `idle 9m` in `text.muted`. The row the cursor's on is on
    `bg.selection`. Row 23: `▼` in `accent.key`, ` 8 more today   ` in `text.muted`, `/` bold
    `accent.key`, and ` finds one from another day` in `text.muted`. The list scrolls within the
    panel, and a session found with `/` joins it, ticked (settled in the plan);
  - row 25, what it will show: `It shows ` in `text.secondary`, `1,240` bold `text.primary`, ` of `,
    `1,275` bold, ` lines, in `, `11` bold, ` columns.`.
- **Events:**
  - row 9: `Pick which events it shows.`;
  - row 11: `EVENTS` at `x`, `today` at `x+14` and `ONLY` at `x+58`, in `text.subtle`;
  - rows 12 to 23, a kind each, in this order: its tick and name; today's count right-aligned in 5
    from `x+14`, in `text.secondary`, `text.faint` at 0; and at `x+21` what it is, in `text.muted`:
    `started` a session first seen; `moved` a session moved to another account; `pressure` an
    account under pressure; `cap` an account at its cap; `limit` an account at its limit; `refused`
    an account's token refused; `primed` a window started early; `open` an account with room
    again; `pin` routing set by hand; `auto` routing given back to the router; `restart` a restart
    of the router falling due; `health` the router unhealthy, or well again;
  - **ONLY:** `accounts` and their ticks (row 12); `sessions` and `every one, unless some are
    ticked` (13); today's sessions from `x+59`, rows 14 to 19, as Requests' list, counting their
    events today (`2 events`, `1 event`); then the `▼ 5 more today   / finds one from another day`
    line (20). No models and no `failed`, which an event has neither of;
  - row 25: `It shows all ` `20` ` of today's events.`
- **Its keys,** on row 27, right-aligned two cells in from the edge, three spaces apart:

  `←→ side   ↑↓ move   space tick   enter apply   esc cancel`

  `←→` moves between its two sides, `↑↓` through its rows, `space` ticks, `enter` applies and
  closes, and `esc` closes, changing nothing.
- **A choice ticked keeps those ticked; none ticked is every one,** for accounts, models and
  sessions alike.
- **What's remembered:** the columns shown, in the preferences file (see Themes). The filters are
  not, so the Log opens on everything.

**Events.** A line an event, as a table with heads: `at` from 2, 8 wide; `session` from 12, 17
wide; `event` from 31, 8 wide; `account` from 41, 16 wide; and `what happened` from 59 to the edge.

- **`at` and `session`** as Requests' are; `session` is blank for an account's event, and holds a
  session's own pin's id (settled in the plan).
- **`event`:** its kind, in `text.secondary`.
- **`account`:** the account, in `text.primary`, cut with `…` past 16 cells (settled in the plan);
  for a move, from and to, `personal → side`, with ` → ` in `text.muted`; for a pin, the accounts it
  names, joined with `, ` (settled in the plan); blank for routing given back to the router, a
  restart and the router's health, which name none.
- **`what happened`,** in `text.muted`: what the kind adds. Account names stay `text.muted` here,
  the table's own `account` column carrying the white: a details column is the one place names
  aren't white. A window's use is bold in its level's colour, and a move's cost in
  `accent.attention`.

| Mark | `event` | `account` | `what happened` |
|---|---|---|---|
| `▲` `accent.mode` | `started` | where it started | nothing |
| `▸` `accent.key` | `moved` | `work → side` | why, in the views' words: `work reached its cap`, `personal reached its limit`, `personal came under pressure, its 5-hour to run out at 15:10`, `pinned to side`, `rescored after 15h idle` (settled in the plan); and where it wrote its context again, three spaces and `182k cached again, $1.82`, in `accent.attention` |
| `◔` `accent.attention` | `pressure` | the account | `its 5-hour to run out at 16:05` (settled in the plan) |
| `╎` `accent.attention` | `cap` | the account | `95% of its 5-hour window: its 3 sessions move to side as their next requests come` |
| `■` `state.destructive` | `limit` | the account | `its 5-hour window, till 23:58: its session moves to side` |
| `■` `state.destructive` | `refused` | the account | `its token refused (401), till 14:00: its 2 sessions move to side`, or a request refused alone, `its opus requests refused (403), till 14:00` (settled in the plan) |
| `◇` `text.subtle` | `primed` | the account | `its 5-hour window started, resets at 12:05` |
| `✓` `state.positive` | `open` | the account | `its 5-hour window reset` |
| `●` `accent.key` | `pin` | the accounts it names | `new sessions go there, set from the command line`; with `--move`, `new and running sessions go there, …`; a session's own pin, `this session goes there, set from the dashboard` (settled in the plan) |
| `↩` `accent.key` | `auto` | blank | `new sessions back from personal to the router's choice, set from the dashboard`; a session's own pin cleared, `this session back from side to the router's choice, …` (settled in the plan) |
| `!` `accent.attention` | `restart` | blank | `due: the config changed`, `due: switchboard upgraded`, `due: the time zone changed` (settled in the plan) |
| `⊕` `state.destructive`, or `state.positive` well again | `health` | blank | `unhealthy: <its reason>`, or `healthy again` (settled in the plan) |

- **A mark a kind:** the marks are LATELY's (see The Overview) and the Log's own, each a cell wide
  in JetBrains Mono, regular and bold, one a kind but for `■`: a limit and a token refused both
  close their account, as ROUTING marks them, and their words tell them apart. `✕` is a single
  request's refusal, on a cord or a row (`✕ 429`), never an event's. `○` is never one: it says not
  read, or not live. `◔`, `!` and `⊕`, which no frame draws, are JetBrains Mono's, regular and bold,
  and `refused` keeps the `■` the Overview's frame 3 draws (settled in the plan).
- **`open`** is the Log's word for the router's `room`: room again, as a window that held the
  account back resets. A pin cleared is never called a pin: it's `auto`.
- **A limit's and a cap's sessions,** and a refusal's, are those its moves counted: `its 3 sessions
  move to side`, `its session moves to side`, or where they went to several accounts, `its 3
  sessions move to other accounts` (settled in the plan).

**The day rule.** Where one day ends and the next begins, on Requests and Events alike, a line
across the table, with no mark, a night's quiet unfolded (settled in the plan):

- from column 2, the day that ended bold `text.primary` (`Wed 30 Sep`); three spaces; its summary:
  `1,912` in `text.secondary` and ` requests · ` in `text.muted`, `17` and ` sessions · `, `2` bold
  `state.destructive` and ` limits reached`, ` · `, and `$212` bold `text.secondary` and ` at API
  prices` in `text.muted`;
- two spaces, a rule of `─` in `border`, two spaces, and ending at the last column the day that
  began, bold `text.primary` (`Thu 1 Oct`).
- **Its figures** are the day's summary's, as History reads it: requests as the ledger counts them
  (see The request ledger), the limits its accounts reached, its sessions, each once, and its worth
  at today's prices, exactly, with no `≈`. Today, and any day not summarised since lines came to be
  filed under it, is summarised from its lines as it's read.
- The lines above the rule carry the times of the day before, the day unnamed.

**The state line** (row 35):

- **Following:** `●` in `state.positive` and ` following` in `text.secondary`. New lines join at the
  foot, the table moving up.
- **Paused:** `‖` bold `accent.attention`, ` paused` in `text.secondary`, and ` at 14:31:13` in
  `text.muted`, the chosen line's time; three spaces; `▼ ` in `accent.key` and `214 new` bold
  `accent.key`, the lines come since, counted and not drawn; three spaces; `end` bold `accent.key`
  and ` to follow` in `text.muted`. With none come since, no count:
  `‖ paused at 14:31:03   end to follow`. Paused at a line of another day, it names that day
  (settled in the plan).
- **Not live:** `○` in `text.subtle`, ` not live` in `text.secondary`, and three spaces and `the
  ledger's lines to 14:40:52; those since come in as the router answers again` in `text.muted`.
- **At the right,** ending at the last column, how many lines the table holds of today, those shown:
  `1,240` in `text.secondary` and ` lines today` in `text.muted`; filtered, `445` and ` of d28c's
  today`; on Events, `20` and ` events today`. With the heading's hidden count, it makes today's
  lines.

**Without the router.** The status line says so in place of `● healthy`: `■ ` in
`state.destructive`, `the router isn't answering` in `text.secondary`, then `  ·  ` and `since
14:40:55` in `text.muted`; then the rest of the status line as the last status document kept has
it, the mode, the sessions and where new sessions go, as every page has it (see The page). The table
holds the ledger's lines to the last one written, read from its files, and nothing in flight; the
state line says `not live`, and to when. `p pin` is drawn all in `text.faint`, as no pin can be set
without the router; pressed, `p` says the router isn't answering. Events shows what the router's
files hold, to its last event filed.

**On a phone** (52×33), Requests alone:

- rows 0, 1 and 3: the title, the tabs and the status line, as every page's phone has them;
- row 5, the heading: `LOG` in `text.subtle`, two spaces, the sub-tabs as wide; at the right `35
  hidden` in `text.muted`, two spaces, `f` bold `accent.key`; no `[ ]` and no rule;
- row 7, the heads, in `text.subtle`: `at` 2, `session` 11, `on` 20, `took` ending at column 33,
  `stopped` 37;
- rows 8 to 29, the newest 22 lines: the mark at 0; the time at 2, in `text.muted`; the session's id
  at 11, in `text.secondary`, without its directory; the account at 20, in `text.primary`; `took`
  ending at column 33, in `text.muted`, counting in `accent.key` in flight; and at 37 how it
  stopped: the tool alone in `text.secondary` for `tool_use` (`Bash`), else why in `text.muted`
  (`end_turn`); `new session` in `text.muted` on a session's first; `asking` or `answering`, bold
  `accent.key`, in flight;
- row 31: the state line, `● following`, and at the right `1,240 lines today`;
- row 32, the keys: `↑↓ request   enter open   [ ] view   ? keys   q quit`, by the phone's rule for
  key lines (see The page).
- Events, the panel, paused and filtered take the wide page's rules in the phone's columns; a
  request that moved its session says `from work` at 37; and the card stacks its columns one under
  another, scrolling within it (settled in the plan).

**Keys.** `tab` the next tab; `[` `]` between Requests and Events; `↑↓` chooses a line, lit across
the width, the table scrolling to keep it in view, and from following, `↑` choosing the newest line
and pausing there (settled in the plan); `enter` opens its details, and `esc` closes them; `end`
follows again, the newest line at the foot and nothing chosen; `s` opens the chosen line's session's
page, and `esc` there comes back to the Log as it was, paused at its line; `f` opens the panel; `/`
finds a session by its id or its directory, today's or any day's the ledger keeps, through the days'
session ids, the table then showing its lines alone, headed `only <id> <name>`, and the panel
listing it among today's sessions, ticked (settled in the plan; see The request ledger); `p` opens
new sessions' routing picker, whatever line is chosen, as the page decides which picker (see Routing
by hand); `?` help for this page, `HELP · LOG`, listing every key first, `t`, `R` and `j`/`k` among
them, then explaining the marks, the columns, the kinds, why checks and counts are hidden, worth at
API prices and what the ledger never keeps (settled in the plan); `q` quits. `j` `k`, PgUp, PgDn and
the wheel scroll the table; `R` and `t` work as on every page, and aren't listed. A click on a
sub-tab shows it, on a line chooses it, and on the line chosen opens its details; on `▼ N new` it
follows again; on the panel's ticks it ticks; and the wheel scrolls (settled in the plan). Each key
line has each key bold `accent.key` and its word in `text.muted`, three spaces after `tab views`,
then four after a group and two within one:

- Requests:

  `tab views   [ ] view    ↑↓ request  enter open  f what it shows  / find    p pin    ? help  q quit`
- Events, the same with `↑↓ event`:

  `tab views   [ ] view    ↑↓ event  enter open  f what it shows  / find    p pin    ? help  q quit`
- Paused with a line chosen, and so behind a line's details, faded; Events' the same with
  `↑↓ event`, and on an account's event without `s its session`, as an account's event has no
  session:

  `tab views   [ ] view    ↑↓ request  enter open  s its session  end follow    f what it shows  / find    p pin    ? help  q quit`

**Where it reads.**

- **Requests:** the request ledger's lines, read where they lie through `internal/ledger`, with no
  router (see The request ledger), a day at a time as the table reaches it, today's followed as its
  file grows; and, while the dashboard runs, the request stream, `GET /stream`, for what's in
  flight (see Control API), as a line is written only as its request ends. A request in flight takes
  its time from `sent`, or `inflight`'s `sent_at`; its session's directory from the stream's `dir`;
  `first` from `first`, or `first_at`; and its characters from `progress`. A line arriving for a
  request already shown fills in its row, which keeps the place it took as it was sent, though the
  line's `at`, its arrival, is a moment earlier. Requests the router answers itself, never sent
  upstream, are never on the stream, and appear as their lines are written.
- **Worth** is worked out as it's read, at today's prices, from the dated price table, never stored;
  a model the table doesn't know is unpriced, never free (see The request ledger).
- **Counts,** today's: the heading's hidden, the panel's kinds and failed, and the state line's,
  from today's lines by `kind`, `status`, `canceled` and `cut_off`, and by `session`. The panel's
  sessions: today's lines by `session`, with their `dir`; the account each is on now, and when it
  was last seen, from `GET /sessions`; `asking` from the stream; `started` its first line's time
  today.
- **Events:** the status document's `events` as each look reads them, the newest 50 since the router
  started, and before them the router's files of events, through `internal/events`, back as far as
  `[ledger] keep` keeps them, 400 days unless it's set, and for good where it says `forever` (see
  The router's events). A move's cost, a cap's `known from` and the windows then are read off the
  ledger's lines of the moment. Days before the router filed its events show none.
- **Live:** the Log joins the request stream while it shows, as Sessions does, joining again a
  second after it ends (see Live updates). A reader 256 events behind is dropped by the router: the
  Log fills the gap from the ledger as it joins again. Within the motion budget (see What moves),
  following, a new line joins the foot and the table moves up a line, and a line in flight's `took`
  counts; paused, only `▼ N new` and `took` move (settled in the plan).
- **Not live:** the ledger's files and the router's files of events alone.
- **Text from elsewhere,** a directory, a label, an error's message, shows with its control
  characters as spaces.

### A session's page

One session, its whole life: what it's doing now and where it runs; how busy it's been, from its
first request to now; which accounts it ran on and what it took of each; and its turns, newest
first, each opening into its requests. The Overview shows every account now, Accounts who used
them and History when; a session's page follows one session. It's a full page, not a tab, and draws
no projection. It's drawn in the Paper file *Switchboard · Session and Log*, on its page **FINAL ·
session page · signed off · 7 Oct 2026**, its frames kept as `testdata/vhs/reference/session-*`.

- **Opening it:** `enter` on a session's row opens its page, on Sessions, in either view (see
  Sessions), and in Accounts' Detail (see Accounts); `s` opens it from the Log (see The Log); and a
  click opens it from a row of the Overview's SESSIONS, where `↑↓` moves the account focus (see The
  Overview). `esc` goes back to where it was opened from, as it was left. The tab it was opened from
  stays lit, and `tab` and `shift-tab` leave the page for the tab after or before it, as The page
  says (settled in the plan).
- **Named by its directory:** a session is named by the directory switchboard's own `claude` was
  started in, as `run` tells the router and the ledger keeps it (`dir`), and has no title. Nothing
  is read from Claude Code's own files, whose form is Claude Code's to change. A session with no
  directory, as one not started through `run`, or under a router from before `dir`, is named by its
  id alone; a long directory is cut from its start with `…`, keeping its name; one resumed in
  another directory shows the latest (settled in the plan).
- **Its rows,** at 160×37: the title row (0) and the status line (2), as every page has them (see
  The page); the heading (4); now (6 and 7); STORY (9 to 17): its heading (9), a move's label (10),
  its columns (11 to 15), the ribbon (16) and the axis (17); the accounts it ran on, a row each,
  from 19; then a blank row, TURNS' heading, a blank row, its column heads, and a row a turn, to row
  34; how many more turns, where some don't fit (35); and the key line (36). Each account it ran on
  takes a row from the turns: with one, the turns start at row 24, eleven of them. At other heights
  the turns take what's left, the rest fixed (settled in the plan), down to 29 rows at the least:
  its fixed rows and three turns, with one account.

**The heading** (row 4): `SESSION` in `text.subtle`; three spaces; the session's id cut to 4, as the
dashboard shows ids, bold `text.primary`; three spaces; its directory's last part, bold
`text.primary`; two spaces; the directory as the ledger holds it, the home as `~`
(`~/Code/switchboard`), in `text.faint`; one space; a rule of `─` in `border`; two spaces; and,
ending at the last column, `esc` bold `accent.key` and ` back` in `text.muted`.

**Now** (rows 6 and 7).

- **Row 6,** what it's doing and where it runs:
  - **its state,** bold: `↑ asking` in `accent.key` while a request of it waits for its answer;
    `↓ answering` in `accent.key` while an answer streams; `idle 9m` in `text.faint`, the time since
    its last answer ended; `ended yesterday 18:47` in `text.secondary` once it's over. The
    Overview's SESSIONS says the same, short;
  - **three spaces, then where and why,** in `text.muted`: `on `, the account bold `text.primary`
    and its marks (`▲` in `accent.mode`, `◆` in `text.tertiary`), then since when and how it came
    there, every account it names in `text.secondary`: ` since 14:31, moved from ` `work` `: `
    `work` ` reached its cap`; ` since it started: sticky`; ` since 09:12, rescored after ` `15h`
    (in `text.secondary`) ` idle: ` `personal` ` had the most room`; its own pin said as a move is,
    ` since 14:39, pinned here`, or ` since 14:31: its pin to side yields: side has no room`
    (settled in the plan);
  - **at the right,** ending at the last column: `started ` in `text.subtle`, the time in
    `text.secondary`, then in `text.muted` ` · 2h 40m`, how long since it started; ` · resumed
    09:12` once it came back after a fold; ` · ran 5h 27m` once it's over. A day other than today is
    named: `started yesterday 16:40`.
- **Row 7:** its models, three spaces apart, each by its id less `claude-` in `text.secondary`, then
  ` → ` in `text.muted` and the account it goes to now in `text.secondary`:
  `opus-5-5 → side   haiku-4-5 → side`. A session's models are routed apart, the session key being
  the session and its model, so each can go to an account of its own: row 6 names the account its
  last-used model goes to, and row 7 each, the ribbon and a turn's `on` following its main thread's
  (settled in the plan). At the right, while it runs, ending at the last column: the account in
  `text.secondary` and ` now  ` in `text.subtle`, then each of its windows, three spaces apart, its
  name in `text.muted` and its use bold in its level's colour (see `usage`, the printout):
  `side now  5-hour 13%   week 78%   fable week 4%`. Every window shows, used or not.
- **Account names are white** wherever named: bold `text.primary` where it runs (`on side`) and on
  an account's own row, `text.secondary` everywhere else, the story's labels and a turn's `on` among
  them, so a name reads as an account and not a word.

**STORY** (rows 9 to 17): its life, from its first request to now, across the page.

- **The heading** (row 9): `STORY`, three spaces, `requests a minute` in `text.faint`, one space,
  and a rule of `─` in `border` to two cells short of the edge.
- **The columns** (rows 11 to 15): its first minute at column 0 and now, or its end, at column 159,
  the minutes shared out evenly but for folds: `(minutes − folded) ÷ (160 − 5 × folds)` a column, so
  a minute a column for a session of 160 minutes, and a younger one never more than a column a
  minute, drawn from the left, its axis ending at its newest minute (settled in the plan). Each
  column is the mean requests a minute over the minutes it stands for, a request counting in the
  minute it arrived. Five rows tall, filled `round(rate ÷ scale × 40)` eighths, halves to even, as
  the Overview's traces are drawn: full cells are background and the top cell an eighth block, in
  `accent.key` mixed 80% from the canvas. A column whose minutes had no requests has `▁` in
  `text.faint` on its bottom row, so the lane always shows.
- **Its scale** is the traces' (see The Overview): the busiest column's rate fills all five rows,
  never less than 8 requests a minute, so a quiet session isn't magnified; the frames draw it at a
  fixed 8 (settled in the plan).
- **The turn chosen is lit:** its columns in `accent.key` mixed 95% from the canvas, so the list and
  the life it came from stay tied. As `↑↓` moves through the turns, the light moves along the story.
- **A move:** `┊` in `accent.attention` in its column, on row 10 and down the columns' rows wherever
  a cell is empty or holds the floor, never over a column. On row 10, a label ending at the marker:
  - what moved it and where to, in `text.muted`, its accounts in `text.secondary`: `work reached its
    cap → side`, `rescored after 15h idle → personal`;
  - where the move cost something, ` · `, the tokens it wrote again bold `accent.attention`
    (`182k`), ` cached again, `, and their worth at API prices, bold `accent.attention` (`$1.82`):
    its request's cache write, priced;
  - two spaces, its time in `accent.attention` (`14:31`), a space, and the marker.
  - With no room at the marker's left, the label starts two cells to its right, the time first:
    `┊ 09:12  rescored after 15h idle → personal`.
- **No cost where the cache would have run out anyway:** a move after the session idled longer than
  its cache lasts, an hour on a subscription, five minutes where its writes were five-minute ones,
  wrote nothing staying wouldn't have, so its label says what moved it and when, and nothing of what
  it wrote. Its turn's `written` still counts it.
- **A long idle gap folds,** any over an hour, as after an idle hour the router rescores the session
  anyway, `resumed` said after one (settled in the plan), into five columns: `╱ ╱` in `text.faint`
  in their middle three, on each of the columns' rows. On the axis, the gap's length in whole hours
  (`15h`) at the fold's first column, and the time the session came back (`09:12`) from the column
  after it.
- **The ribbon** (row 16): a stretch for each time on an account, `├`, `─` and `┤` in `text.faint`
  from its first column to its last, the account's name centred in it in `text.secondary`, a space
  either side, where its inside is wide enough. A stretch under three columns isn't drawn. Where two
  stretches meet in one column, the column is the later's, and the earlier ends a column before it:
  `┤├`.
- **The axis** (row 17), in `text.faint`: its first time at column 0 (`12:02`); then a time every
  half hour, or every hour once it has run more than four hours, folds left out, each at its own
  column, but none within 20 minutes of a fold, in its first 8 minutes or its last 10, fewer than 8
  columns after the one before, or starting past column 150; and at the right, `now`, or `ended`,
  in `text.subtle`.

**The accounts it ran on** (row 19 on), under the story with no heading, a row each, in the order it
first ran on them:

- at column 0, the account's number in config order, as the Overview numbers it, in `text.subtle`,
  and a space; then its name bold `text.primary` and its marks;
- at 14, its time there, from its first request on it to its last, in `text.secondary`: `12:02 –
  14:31`, `14:31 – now`; with its day where the session spans more than one: `yesterday 16:40 –
  18:05`, `today 09:12 – now`;
- at 40, its requests in `text.muted`: `404 requests`;
- at 56, what it took of the account's windows, the points bold `text.primary` and the rest
  `text.muted`: `≈ 41 points of its 5-hour · 6 of its week`, a model's own window it used adding
  `· N of its Fable week` (settled in the plan);
- right-aligned to the last column, its worth at API prices, in `text.secondary`: `$41.06`.
- **A turn that moved splits** between the two, each request counted on the account that answered
  it, the context written again on the account it moved to. The rows' requests and worth add up to
  TURNS', each rounded on its own, so the rows can come a cent apart from it. Back on an account it
  left, it keeps one row an account, spanning its first request there to its last
  (settled in the plan).
- **≈ points:** a window counts its account's whole use, every session's, so a session's share is
  judged by its tokens. For each account and window, each rise in the window's use from one answer
  to the next, as their lines' `limits` give it, is shared out among the requests that ended between
  the two, the later included, of whichever session, by their tokens, every kind counted alike. A
  session's points are its requests' shares summed, in whole points of the window, a reset starting
  the window afresh.

**TURNS.** A turn is a prompt to the answer that ended it.

- **Told from the lines:** the ledger keeps no content, so a session's turns are told from the shape
  of its `message` lines, in the order they arrived; checks and counts are left out.
  - A request's thread length is its `shape.messages`. Claude Code's main conversation is the
    longest thread; the side requests it makes, a subagent's or a small model's, start short.
  - A turn's main thread is its longest so far: a request is on it where its length is at least the
    main thread's last.
  - A turn ends at an answer on its main thread that stops other than at `tool_use`: `end_turn`,
    `max_tokens`, `stop_sequence` or `refusal`; or at a main-thread request its client canceled, as
    an interruption does. A side request's `end_turn` ends nothing.
  - The request after a turn's end starts the next turn. A conversation compacted or cleared starts
    a new turn short.
- **Checked against real lines before the page is built,** as no test and no agent reads a real
  ledger: `scripts/turns-check`, run by hand on the machine whose ledger it is, reads it through
  `internal/ledger` and prints counts alone, never an id, a directory, a time or anything of a
  line: the sessions and turns read; the requests a turn, the least, the median and the most; the
  turns ended by each stop reason, and by cancelling; the side requests' `end_turn`s passed over,
  by model; the turns whose first request is shorter than the main thread before them; and the turns
  that ran over an hour. Where the counts don't fit what its user knows of their sessions, the rule
  changes before the page ships.
- **The heading:** `TURNS`, with no description, its rule of `─` in `border` from column 9 to two
  cells short of the totals, and at the right the session's totals: `13 turns` in `text.secondary`;
  ` · 445 requests · 97% from cache · ` in `text.muted`; its worth bold `text.secondary` (`$48.20`);
  and ` at API prices` in `text.muted`. Its requests are its `message` lines; `from cache` is the
  share of the tokens it sent that the cache served, its cache reads over its input, cache reads
  and cache writes together, to a whole percent.
- **The column heads,** two rows under it, in `text.subtle`: `#` right-aligned in 3 from column 0;
  `started` at 5; `took` at 14; `requests` right-aligned in 8 from 21; `tools` at 32, 30 wide; then
  right-aligned, `read` in 7 from 64, `written` in 7 from 73, `out` in 6 from 82 and `worth` in 7
  from 90; and `on` at 100, 12 wide.
- **A row a turn,** newest first, from the row under the heads to row 34:
  - `#`, its number, from 1 at its first, bold `text.primary`;
  - `started`, its first request's time; `took`, its length in minutes, from the minute it started
    to the one it ended in, and while it runs, to the minute it's in, `…` after it (`3m…`);
    `requests`;
  - `tools`, the three its answers called most, most first: `Bash ×14, Edit ×9, Read ×8`;
  - `read`, `written` and `out`: the tokens it read from the cache, wrote to it, and put out,
    counted as the Log counts them (`5.4k`, `958k`, `5.7M`);
  - `worth`, at API prices (`$0.79`), `unpriced` for a model the price table doesn't know, a sum
    that leaves one out ending in `+` (settled in the plan);
  - `on`, the account, in `text.secondary`; for a turn that moved, `work → side`, its `written`,
    which holds the context written again, in `accent.attention`;
  - the rest in `text.muted`.
- **While a request waits for its answer,** the newest turn's row ends in `↑ asking`, bold
  `accent.key`, at column 114; while its answer streams, it has no word there.
- **The turn chosen** is on `bg.selection` from column 0 to 111, its words bold, and lit on the
  story. The page opens on the newest.
- **More than fit:** row 35, `▼` in `accent.key`, ` 3 more turns   ·   ` in `text.muted`, `j/k` bold
  `accent.key` and ` to scroll` in `text.muted`. The turns scroll within their rows, under the
  heading, now, the story and the accounts, which stay put, the turn chosen kept in view
  (settled in the plan).

**A turn opened.** `enter` opens the turn chosen into its requests, in place, the turns under it
moving down.

- **One request row with the Log, the Log's** (see The Log): its columns in the Log's order,
  colours, widths and words, but `session`, which a session's own page doesn't need. Its heads, on
  the row under the turn, in `text.faint`: `at` from column 5, `model` 15, `on` 26 and `status` 36;
  right-aligned, `first` in 5 from 44, `took` in 6 from 51, `read` in 6 from 59, `written` in 7
  from 67 and `out` in 5 from 76; `stopped` at 83, 27 wide; and `routing` at 112, 30 wide.
- **A row a request,** oldest first, a `│` in `border` at column 2, with no mark. The request that
  wrote the context again on the account it moved to has its `written` bold `accent.attention`, and
  in `routing`, as the Log words it, `from work, at its cap  $1.82`. A move at a cap refuses
  nothing: the session's next request goes to the account it moves to. A request sent again after a
  limit is one row, as it's one line, its times the whole request's and its `tried` in `routing`
  (`after work's limit`); a limit no account could take is its 429, as failed (settled in the plan).
- **Five at a time:** under them, `└` in `border` at column 2, then from column 5 `▼` in
  `accent.key`, ` 32 more of its requests   ·   ` in `text.muted`, `j/k` bold `accent.key` and
  ` through them` in `text.muted`. The requests scroll within their panel, `↑↓` moving through them.
- **`enter` on a request** opens its details in the Log's card, the turn staying open behind it; the
  first request is chosen as a turn opens, on `bg.selection` (settled in the plan).

**One that ended.**

- **It has ended** once the router no longer lists it, an hour without a request, at its last line's
  time, and runs again if it's resumed. One whose days run past `[ledger] keep` loses its first
  day's lines first, so `kept until` counts from its first day, and the story starts at its oldest
  line kept (settled in the plan).
- **Row 6:** `ended yesterday 18:47`, bold `text.secondary`; three spaces; in `text.muted`,
  `ran on `, each account it ran on in `text.secondary`, and when it moved and why, its account in
  `text.secondary` too: `ran on work, then side from 16:02: work reached its cap`. At the right:
  `started yesterday 13:20 · ran 5h 27m`.
- **Row 7:** its models, going nowhere now, joined by ` · ` in `text.faint`; three spaces; and in
  `text.faint`, `kept until 4 Nov 2027`, the day before the ledger removes the lines of its first
  day, as long after that day as `[ledger] keep` says, 400 days unless it's set, the year named
  where it isn't this year's (`kept until 29 Dec`); or `kept for good`, where it says `forever`. No
  windows at the right.
- **The story** ends at its last request, `ended` at the axis's right. No turn runs.
- **No `p`:** its key line is `esc back    ↑↓ turn  enter its requests    ? help  q quit`.

**One that came back.**

- **The night folds** on the story: its stretch on one account before the fold, on the next after.
- **The rescore is a move:** after an idle hour the router chooses afresh (the routed reason
  `rescored after <idle> idle`), so the story marks it, with no cost, as its cache had gone, and
  row 6 says `on personal ◆ since 09:12, rescored after 15h idle: personal had the most room`.
- **Row 6's right:** `started yesterday 16:40 · resumed 09:12`; the accounts name their days:
  `yesterday 16:40 – 18:05`, `today 09:12 – now`.

**On a phone** (52×32), each part shorter:

- **Rows 0, 1 and 3:** the title, the tabs and the status line, as every page's phone has them.
- **Row 5, the heading:** `SESSION` in `text.subtle`, two spaces, the id and the directory's name,
  bold `text.primary`, two spaces apart; `esc back` at the right. No path and no rule.
- **Rows 7 and 8:** `↑ asking  on side ▲ since 14:31`, coloured as row 6 is; then `moved from work:
  its cap` in `text.muted`, `work` in `text.secondary`, and at the right `2h 40m` in
  `text.secondary`. No models and no windows.
- **The story,** without a heading: the label on row 10, shortened to what moved it and its worth,
  `its cap → side, $1.82  14:31 ┊`, the account in `text.secondary` and the worth in
  `accent.attention`; four rows of columns, 11 to 14, on the same scale; the ribbon on 15, where
  side's stretch is too narrow for its name (`┤├──┤`); and the axis on 16.
- **The accounts,** from row 18: numbered, the name and its marks at 0, as the wide page's; at 11,
  `≈ 41 of 5-hour · 6 of week`, the points bold `text.primary` and the rest `text.muted`; and the
  worth at the right, in `text.secondary`.
- **TURNS,** after a blank row: the heading, its rule from column 9, and the session's worth at the
  right, bold `text.secondary`; no other totals.
- **The newest seven turns,** from two rows under the heading: the number right-aligned in 2, bold
  `text.primary`, and a space; its start in `text.secondary`; two spaces, its minutes right-aligned
  in 2 and `m`, `…` after the newest's; its requests right-aligned in 4 and ` req`; and at the
  right its worth, two spaces, and its account padded to 9 in `text.secondary`, `work→side` for a
  turn that moved, its `→` bold `accent.attention`; the rest in `text.muted`. The newest is chosen,
  on `bg.selection` across all 52 columns.
- **Row 31, the keys:** `esc back   ↑↓ turn   enter open   ? keys   q quit`, by the phone's rule for
  key lines (see The page).
- A turn opened, one that ended and one that came back follow the wide page's rules in the phone's
  columns, and `▼ N more turns` shows where some don't fit (settled in the plan).

**Keys.**

- **`↑↓`** moves the turn chosen, lit on the story (`↑↓ turn`); with a turn open, through its
  requests (`↑↓ request`).
- **`enter`** opens the turn chosen into its requests, in place (`enter its requests`; on a phone
  `enter open`).
- **`esc`** closes an open turn (`esc close`), else goes back to where the page was opened from
  (`esc back`).
- **`j/k`** scrolls the turns (`j/k to scroll`) and an open turn's requests (`j/k through them`), as
  `j` and `k` scroll wherever something does.
- **`p`** opens this session's routing picker, `ROUTING · THIS SESSION`: one account, or auto, which
  leaves it to the router (see Routing by hand). A session is pinned from its own page alone, and
  here the number keys and `m` do nothing. An ended session's page has no `p`, nor does any page
  with one account, unless a pin set from the command line stands (see Keys).
- **`?`** opens help for this page, `HELP · SESSION`, listing every key first, `t`, `R` and `j`/`k`
  among them, then explaining the story's scale, what a turn is, the ≈ points, worth at API prices
  and what `cached again` means (settled in the plan); and **`q`** quits. `t` and `R` work as on
  every page, and aren't listed.
- **The key line** (row 36) starts with `esc back` in `tab views`' place, `tab` still working: each
  key bold `accent.key` and its word in `text.muted`, four spaces after a group and two within one:
  - `esc back    ↑↓ turn  enter its requests    p pin    ? help  q quit`;
  - ended, without `p pin`: `esc back    ↑↓ turn  enter its requests    ? help  q quit`;
  - a turn open: `esc close    ↑↓ request  enter open    p pin    ? help  q quit`.

**Where it reads.**

- **Its lines:** the ledger's lines of its own days, read where they lie through `internal/ledger`,
  with no router (see The request ledger), the days' session ids saying which days those are; and
  today's followed as its file grows. The story, the turns, the accounts and the totals are worked
  out from them as they're read. Of a line, the page uses `at`, `model`, `account`, `status`,
  `first_ms` and `total_ms` for a request's row; `usage` for `read`, `written` and `out`, the
  five-minute and the hour writes priced apart, its input only in worth; `answer`'s `stop` and
  `tools`; `reason`, `from` and `tried`, for a move and why, in the views' words (see The router's
  events); `limits`, for the ≈ points, every session's lines of its accounts among them; and `dir`,
  for the heading.
- **What's in flight** has no line yet, as a line is written when its request ends: the request
  stream gives it, and with it `↑ asking` or `↓ answering`, as the Overview's SESSIONS reads them.
  The page joins the stream while it shows (see Live updates). Within the motion budget (see What
  moves), the newest turn grows, a new turn arrives at the top, and the story's newest column steps
  a minute at a time, its scale following its busiest column (settled in the plan).
- **Where it goes now:** `GET /sessions/{id}`, its models each with the account it goes to, its own
  pin, and when it was last seen; and the account's windows from the status document, as the
  Overview reads them. A session the router no longer remembers is drawn from its lines alone.
- **Worth** at API prices, at today's, from the dated price table, worked out as it's read and never
  stored; a model the table doesn't know is unpriced, never free (see The request ledger).
- **For agents,** `sessions <id>` prints the page's data, its accounts, turns and totals, and
  `requests --session <id>` its lines as the ledger holds them (see The data verbs).
- **Past sessions:** today's that have ended are on Sessions' List (see Sessions); an older one is
  found with the Log's `/`, and opened with `s` (see The Log). No page lists the days before.
- **Never read:** Claude Code's own files, or anything of a request's content, which the ledger
  never holds.

### Routing by hand

**The routing card is the only way the dashboard changes routing, and it changes nothing until
`enter`:** that's the confirmation. `p` opens it, with nothing picked but what's set now; `esc`
closes it with nothing changed. Neither a number key nor `m` does anything outside it, so a stray
key moves no session. Its frames are in the Paper file *Switchboard · Routing*, on its page **FINAL
· routing · signed off · 7 Oct 2026**; its references are `testdata/vhs/reference/routing-*`:
`routing-auto-160` (new sessions' card as it opens, on auto), `routing-two-160` (two accounts
picked), `routing-move-160` (`m` ticked), `routing-session-160` (a session's card) and
`routing-limit-160` (an account at its cap and one at its limit). No page's own frames draw it: it's
one card, drawn over any page.

Two things are routed by hand, and **the page decides which,** never what's focused or chosen:

- **New sessions:** the accounts the router sends them to, or auto; with `m`, the sessions already
  running elsewhere move there too. Every page opens this card, `ROUTING · NEW SESSIONS`, but a
  session's page.
- **One session:** the account it goes to, or auto, its own pin cleared, so it follows the router. A
  session's page opens this card, `ROUTING · THIS SESSION`, for its session, while it runs; an ended
  session's page has no `p`. A session chosen in a list is routed from its own page: `enter`, then
  `p`.

On the Overview, `↑↓ focus` picks whose charts GRAPHS draws, and `r` hides ROUTING: neither touches
routing.

**The card:**

- **A rounded line,** `╭─╮`, `│` and `╰─╯` in `text.subtle`, round nothing filled: the cells inside
  are the canvas. It's 82 cells wide, centred across the page and down it, a cell of either edge for
  its line and two more inside it, so its content is 76 cells from 3 in.
- **Its title in its top edge,** after `╭─ `, in `text.subtle`, the line running on after it:
  `ROUTING · NEW SESSIONS`, which becomes `ROUTING · NEW AND RUNNING SESSIONS` while `m` is ticked
  (not all sessions: one pinned on its own keeps its pin); `ROUTING · THIS SESSION` on a session's
  page.
- **The page behind it, faded:** every cell's colours, foreground and background, mixed 80% of the
  way to `canvas`, nothing bold, even pace's marks among them. A terminal draws it by drawing the
  page again in those colours. The page behind stands still while the card is open, the card's own
  figures following each read (settled in the plan).
- **It keeps its place:** new sessions' card holds room for its longest outcome, four lines, and a
  session's for two, so picking, which changes how long the outcome is, never moves it. New
  sessions' card is `15 + a` rows tall for `a` accounts, a session's `13 + a`.
- The Log's details and `?` help are drawn in the same card, 128 cells wide (see The page,
  The Log).

**What the card holds,** top to bottom, a blank row between each part:

1. **A line saying what it's for,** in `text.secondary`: `Pick the accounts new sessions go to.`,
   `Pick the accounts new and running sessions go to.` while `m` is ticked, or `Pick the account
   this session goes to.`
2. **The session,** on a session's card alone: its id, its first 4 characters, in bold
   `text.primary`; two spaces, its directory's own name in `text.muted`; two spaces, its directory,
   the home shown as `~`, in `text.secondary`; and at the right, `runs on ` in `text.muted` and the
   account it runs on, bold `text.primary`. Where it runs is never marked in the list below.
3. **The choices,** a row each, in columns from the content's start: the mark at 0, the name at 2,
   the state at 16, the 5-hour window at 26 and the week at 39, and at the right edge the account's
   reason or its sessions.
   - **`0 auto` first,** `0` in `text.subtle` and `auto` in bold `text.primary`, its description in
     `text.muted`: `the router chooses`; on a session's card, `the router chooses: side for now`,
     the account auto would send it to, bold `text.primary`. Picking `auto` clears the accounts;
     picking an account clears `auto`; nothing picked is auto.
   - **Then each account,** numbered by its place in the config, `1 ` in `text.subtle`, its name in
     bold `text.primary`.
   - **The mark:** new sessions' card ticks boxes, `■` in bold `accent.key` picked and `□` in
     `text.subtle` not; a session's card picks one, `●` and `○` the same. A row's mark is its one
     mark: an account's state has no dot.
   - **The state,** a word: `open` in `state.positive`, or `closed` in the colour of why.
   - **The windows:** `5-hour 39%` and `week 66%`, the words in `text.muted` and the figures in
     bold, coloured by level as `usage` colours them, in their columns so they read down; kept for a
     closed account, as it can still be picked or they say why it's closed.
   - **At the right edge:** a closed account's reason, in the colour of why, `at its cap till
     17:10`; else its sessions running, the count in bold `text.secondary` and ` running` in
     `text.muted`; else nothing.
4. **`m`,** on new sessions' card alone, a row of its own: its box, `m` in bold `accent.key`, and
   `also move the sessions running` in `text.secondary`. Never dimmed: its box says whether it's on.
5. **The outcome,** in short plain sentences in `text.secondary`, every account named in bold
   `text.primary` and the cost in bold `accent.attention`. It says only what changes, never what
   stays as it is, and reads the same at any number of accounts:
   - auto: `The router chooses an account for each new session.`
   - one account: `New sessions go to personal. When it's full, the router chooses.`
   - two: `New sessions go to personal or side. When both are full, the router chooses.`; three or
     more: `New sessions go to personal, side or work. When all are full, the router chooses.`
     (settled in the plan)
   - with `m`, then: `The 4 sessions running elsewhere move there too.` and `Rebuilding their caches
     costs ≈ $6.20.`, left out where none would move (settled in the plan).
   - a session's: `This session moves to side on its next request.` and `Rebuilding its cache costs
     ≈ $1.56.`; the account it runs on picked, `This session stays on personal.`; auto picked,
     `This session follows the router.` (settled in the plan)

   Its words are built from parts, an account's name, a count, a cost, never by joining clauses, so
   each outcome stays a sentence.
6. **Its keys,** right-aligned two cells in from its edge on its last row but one, each key in bold
   `accent.key` and its word in `text.muted`, three spaces apart, a blank row above them:

   `↑↓ move   space 0-3 tick   m move running   enter apply   esc cancel`

   On a session's card, `pick` for `tick`, and no `m`; the numbers named are the accounts there are,
   `0-3` for three, and `0-9` past nine.

**Its keys,** while it's open, alone: `↑` and `↓` move between its rows; `space` picks the row it's
on; a number picks its row directly, `0` auto; `m` ticks or clears moving the sessions running, on
new sessions' card; `enter` applies and closes it; `esc` closes it and changes nothing. A number
past the accounts there are does nothing. Past nine accounts, an account is picked with `↑↓` and
`space` alone: the key row reads `0-9`, and the accounts from 10 on have no number.

**Closed, and why:**

- **Amber where the account's own cap closed it:** the share it holds back from the router's own
  choices, which reopens itself as its window resets. A pin spends the caps of the accounts it names
  (see Pinning), so an account at its cap can be picked, and takes new sessions up to its limit.
- **Red where Anthropic closed it:** a limit holding back its every request, `at its limit till
  15:54`; or its token refused, `token refused (401)`; and red too an account with no usable token,
  `no token`, which the router refuses a pin to (settled in the plan).
- **An account that can't take a session can't be picked,** in either card: its row stays, with its
  reason and when it's back, in red, but no box or button, and its number does nothing. A session
  pinned to one would only yield. `switchboard pin` can still set such a pin, ahead of a reset, as a
  script might; the card then shows that account picked, its reason in red, until it reopens.
- **An account not read yet** can be picked: `open`, its figures `—` in `text.faint`. One refused a
  model's requests alone stays open (settled in the plan).

**The cost of a move** is an indicator, `≈`, to stop and consider a move by: rough, never billed. A
session that moves writes its conversation into the new account's cache, about as many tokens as its
last request's prompt, its input, cache reads and cache writes together. Its cost is that write,
priced at its model's cache-write price for as long as the session's writes last, five minutes or an
hour, as its last request's usage splits them, from the dated price table (see The request ledger),
in dollars to the cent: the same figure a session's page marks on its story, `182k cached again,
$1.82`. A session idle longer than its cache lasts would rebuild it on its next request anyway, so
its move costs nothing, and none is said; with `m`, the cost is that of every session that moves
(settled in the plan). The tokens come from the request stream's `done` for a request that ended
while the dashboard runs, else from the ledger's line, which splits the writes either way.

**Applying it:**

- New sessions' card sends `POST /pin` with `{"accounts": [...], "move": true, "by": "dashboard"}`,
  `move` as `m` is ticked, or `DELETE /pin?by=dashboard` for auto; a session's card sends
  `POST /sessions/{id}/pin` with `{"account": "side", "by": "dashboard"}`, or
  `DELETE /sessions/{id}/pin?by=dashboard` for auto. `by` names the dashboard as what set it, in the
  `pin` or `auto` event the router tells (see Control API, The router's events).
- On success the card closes and the router's status document is read again at once, so the status
  line and ROUTING show the change.
- Refused, as with a 400 for an account nothing can go out on, the card stays open, its outcome
  giving the router's reason in `state.destructive` (settled in the plan).
- While the router doesn't answer, `p pin` is drawn faint and `p` opens nothing, saying so in the
  key line's place (settled in the plan; see The page).

**With one account,** there's nothing to pin to: `p` does nothing, and no key line shows `p pin`,
unless a pin set from the command line stands, as `switchboard pin` still allows: then the
Overview's ROUTING heading says `p back to auto`, and `p` opens the card, its one account and
`0 auto`, to clear it.

**On a phone,** the card is the screen's width less a cell each side. An account's windows go to a
row of their own under its name, from the state's column, and its reason or sessions stay
right-aligned on its name's row, cut with `…` where they must; the key row keeps `enter apply` and
`esc cancel`, and as many of the rest as fit, `↑↓ move` the first to go (settled in the plan).

**More accounts than fit:** the card is at most the page's height less 4 rows; past that, its
accounts scroll within it, the row it's on kept in view, `▼ 3 more` under the last row shown and
`▲ 2 more` over the first while some are above (settled in the plan).

### Keys

Every key the dashboard takes, as swept on 7 and 8 October 2026. Each page's key line lists those
that work there (see The page); `?` lists every one, with what it does.

| Key | Where | Does |
|---|---|---|
| `tab`, `shift-tab` | Everywhere | The next tab, the one before, in the title row's order and round; from a session's page, those after and before the tab it was opened from (settled in the plan) |
| `p` | Everywhere, with more than one account | The routing card: a session's on its own page while it runs, new sessions' on every other page, whatever is focused or chosen (see Routing by hand) |
| `t` | Everywhere | The theme picker (see Themes) |
| `R` | Everywhere | Refresh (see Live updates) |
| `?` | Everywhere | Help for the page shown; `?` or `esc` closes it |
| `q` | Everywhere | Quit |
| `esc` | Everywhere | Close what's open, the newest first: a card, the theme picker, the Log's panel, a turn opened; else end a choice (settled in the plan); else, on a session's page, go back where it was opened from |
| `j` `k`, PgUp, PgDn, the wheel | Wherever something scrolls (settled in the plan) | Scroll |
| `u` `g` `r` `s` `c` `l` | Overview | Show or hide USAGE, GRAPHS, ROUTING, SESSIONS, COMING UP, LATELY |
| `b` | Overview | By window, or by account |
| `↑` `↓` | Overview | Move the account focus, which GRAPHS follows |
| `[` `]` | Accounts, History, Sessions, the Log | The sub-tab before or after |
| `<` `>` | Accounts, History | A shorter period, a longer one; on History's Year, the year before, the one after |
| `<` `>` | Runway | The 5-hour view, the week's |
| `v` | History | The next shape or split of the view |
| `←` `→` | Accounts' Compare | Move the focus along the accounts' columns |
| `←` `→` | History's Year, filtered to an account | The account before or after |
| `↑` `↓` | Accounts' Detail | Choose an account in the list, all accounts first; or, in its SESSIONS, a session |
| `→` `←` | Accounts' Detail | Into the account's SESSIONS, and back to the list |
| `←` `→` | Runway | Step back a day, or a week in the week view; step forward, to the live view |
| `↑` `↓` | Sessions | Choose a session, a call on the Switchboard or a row on the List |
| `↑` `↓` | The Log | Choose a line; from following, `↑` chooses the newest and pauses; with its details open, the line before or after |
| `end` | The Log | Follow again, the newest line at the foot and nothing chosen |
| `s` | The Log | The chosen line's session's page; not on an account's event |
| `g` | The Log, an event's details | Requests, paused at the event's moment |
| `f` | The Log | What it shows: the panel |
| `/` | The Log | Find a session, today's or another day's |
| `↑` `↓` | A session's page | Choose a turn; with one open, a request of it |
| `enter` | Accounts, Sessions, the Log, a session's page | Open what's chosen: from Compare, the account in Detail; a session's row, its page; a line of the Log, its details; a turn, its requests, in place |
| `↑` `↓`, `space`, `0`–`9`, `m`, `enter`, `esc` | The routing card | Move, pick, pick by number (`0` auto; past nine accounts, those from 10 on by `↑↓` and `space` alone), move the sessions running too, apply, cancel (see Routing by hand) |
| `←` `→`, `↑` `↓`, `space`, `enter`, `esc` | The Log's panel | A side, a row, tick, apply, cancel (see The Log) |
| `↑` `↓`, `enter`, `d`, `l`, `y` `n`, `esc` | The theme picker | Preview, set one theme, set the dark or light half, answer, put back (see Themes) |

**Routing takes these keys alone.** Outside the routing card, the number keys and `m` do nothing, so
a stray key moves no session; inside it, they act on its rows and nothing else. With one account
there's nothing to pin to, so `p` does nothing and no key line shows `p pin`, unless a pin set from
the command line stands, when `p` opens the card to clear it. While the router doesn't answer,
`p pin` is drawn faint and `p` says so instead (see The page).

**While a card is open,** the routing card, help, or the Log's details or its `f` panel, its keys
alone act, and `esc` closes it; so do the theme picker's while it's open, as Themes says.

**The mouse** acts as keys do where that changes only what's shown, and never routes (see The page).

**Keys that mean one thing on one page and another elsewhere** are kept apart by the page: `r` and
`s` hide sections on the Overview, where `R` refreshes on every page, and `s` opens a session's page
on the Log; `g` shows GRAPHS on the Overview, and Requests from an event's details on the Log.

### What moves

Every page keeps to one motion budget: motion that helps the eye follow a change where it happens,
and none that pulls it there for its own sake, as travelling motion in the corner of the eye
distracts most. What matters reads when still: nothing is said by motion alone.

- **Bars ease** to a new reading over a second, from where they stood, as each look reads one; a bar
  a look first shows rises from empty (settled in the plan).
- **A number that changes brightens** where it changed, its cells mixed toward `text.primary`, and
  fades back over a second (settled in the plan); the rest of its line stays still.
- **A new warning gets one cue,** then stays in its colour: as an account comes under pressure,
  reaches its cap or its limit, or is refused, or a window's words turn to `out ~…`, its words are
  drawn on `bg.attention` for 5 seconds, fading back; a new row in LATELY gets the same
  (settled in the plan). A state the clock changes, as a limit lifting at its time, gets its cue on
  the second's tick.
- **The traces step left** a column as their slice of the 30 minutes passes, the newest column
  filling as requests come.
- **The clock ticks** each second, and with it, where a cell changes, the countdowns, even pace's
  marks and `now` columns, and a session's `idle` time.
- **Nothing travels across a page** but Sessions' Switchboard, where requests travel the cords, by
  its own steps and timings (see Sessions).
- **Stale data says so, and only then:** a reading not refreshed is faded, keeping 35% of its
  colour, which is 65% toward the canvas, its words `not read since 14:20`, and a router that
  doesn't answer is named in the status line (see The page).

Each page's own motion is in its section, within this budget: the Log's table moving up a line as
a line joins its foot (see The Log); a session's story stepping a minute at a time, and its newest
turn growing (see A session's page); Runway's lanes moving on with the clock (see Runway).

**Frames** are drawn only for what moves on screen: never for what's scrolled out of view, under a
card or under `?`, so nothing new is drawn while nothing happens. They come at most 30 a second, a
motion with steps drawing at its steps, as the cords' shimmer does on the clock's 80-millisecond
marks. The page behind a card stands still, its card's figures following each read
(settled in the plan). Where the look can't blend, with `NO_COLOR`, a number that changes is bold
for its second instead of brightening, and a cue is bold for its 5 seconds (settled in the plan);
the `terminal` theme draws `bg.attention` as reverse video, as Themes says.

### Themes

The dashboard is drawn in a theme, which gives each of its tokens a colour. Themes are built in, or
written by the user as `.theme` files, and chosen in a picker drawn over the page.

- **Tokens:** 19 base tokens, which every theme file gives, for meaning and prominence, never a hue:
  `text.primary`, `text.secondary`, `text.tertiary`, `text.muted`, `text.subtle`, `text.faint`,
  `text.on-selection`, `accent.primary`, `accent.key`, `accent.mode`, `accent.attention`,
  `state.positive`, `state.destructive`, `canvas`, `bg.selection`, `bg.attention`, `bg.subtle`,
  `border` and `text.on-attention`. The charts' own, `viz.*`, are each optional, worked out from the
  base tokens where a file leaves it out:

  | Token | Is | Default |
  |---|---|---|
  | `viz.ramp.1`–`viz.ramp.4` | A bar's fill, from its first cell to its last; a use's colour, by its level; and a chart's fill, by its height (see `usage`, the printout) | `state.positive`, `accent.attention`, halfway from `accent.attention` to `state.destructive`, `state.destructive` |
  | `viz.track` | A bar's track: the background of the cells its fill doesn't reach (settled in the plan) | `bg.subtle` |
  | `viz.pace` | The even-pace mark | `text.primary` |
  | `viz.reserve` | The cap's mark on a bar, and its line on a chart: where an account's reserve starts (settled in the plan) | `accent.attention` |
  | `viz.family.1`–`viz.family.4` | The model families' hues, in the version table's order: Opus, Sonnet, Fable, Haiku (see The request ledger) (settled in the plan). History's charts draw a family in its hue, and Days' strips in its hue's shades | `accent.key`, `accent.mode`, `accent.primary`, `text.tertiary` |
  | `viz.version.1`–`viz.version.6` | The model versions' shades, which the version table gives them, a family's newest first (settled in the plan). A table names a version in its shade | A family's newest version, its family's hue; an older one, its family's hue mixed 30% toward `canvas` |
  | `viz.day.0`–`viz.day.4` | A day's shade by how busy it was, from a day with nothing on it to the busiest quarter of the active days (see History) (settled in the plan) | `bg.subtle`, then the ramp of shades below with `accent.key` as its hue |

  So a theme missing a base token is rejected, and a theme without any `viz.*` still draws every
  chart. `viz.track` and `viz.reserve` keep the keys they had, so a theme file that sets them still
  loads, with the meanings and defaults above; `viz.series.1`–`viz.series.6` are gone, with the
  built-ins' values of them, as the cords take their state's colour (see Sessions), and a file that
  still sets them loads, the keys unknown (settled in the plan). Every ramp of shades, the days' and
  each family's strips' on History's Days, is drawn by default by one rule, as `nord` alone sets its
  day shades explicitly: from `viz.day.0`, 40% and then 70% of the way to its hue, then the hue
  itself, then the hue mixed 55% toward white, or toward black on a light canvas
  (settled in the plan). The days' hue is `accent.key`, and a family's strips' its family's. A
  version the version table gives no shade, as an older one past the six, draws as its default
  would, in its family's hue mixed 30% toward `canvas`; and a family it gives no hue of its own, as
  Mythos, draws in `text.muted`, its versions with it (settled in the plan).
- **Files:** `<slug>.theme`, flat `key = #RRGGBB` lines, `#` starting a comment only at the start
  of a line, unquoted, a key once, unknown keys ignored, a missing base key rejecting the file; the
  slug matching `^[a-z0-9][a-z0-9-]*$`. They're read from `SWITCHBOARD_THEMES_DIR`, else
  `$XDG_CONFIG_HOME/switchboard/themes/`, else `~/.config/switchboard/themes/`, the top level alone,
  links followed, again each time the picker opens: regular files alone, once links are followed,
  as reading anything else may never end, and none whose name starts with a dot, as an editor's
  lock file's does. A theme is found by its name exactly, however the filesystem matches names. One
  that doesn't load is named, with why, in the picker and in the log. The built-ins' slugs are
  reserved.
- **Built in:** `nord`, today's palette, its base tokens Nord's, tuned to read against its canvas,
  and the dark half of the default pair; `tokyo-night` and `tokyo-night-day`, Tokyo Night's dark
  palette and its light one, each tuned against its own canvas, the latter the light half of the
  default pair; `amber`, an amber CRT; `exchange`, a telephone exchange's brass and walnut, its cap
  the default `accent.attention`, where its own `viz.reserve` drew it blue (settled in the plan);
  and `terminal`, for a terminal with a transparent or image background, which paints no background
  and uses the terminal's own 16 colours. `nord` gives the families, the versions and the days the
  shades the signed-off frames are drawn in, where its base tokens don't. `terminal` is built in,
  not a file, as `#RRGGBB` can't name the terminal's colours, and where the others paint a cell's
  background or blend into the canvas, it draws glyphs instead: a bar's fill in `█` and the eighth
  blocks, in the ramp's colours, its track in `░`, faint, as it paints no background for it to show
  on, and a chart's and a trace's columns in their colours, unblended; a day's shade by a glyph's
  density, `·` for none, then `░`, `▒`, `▓` and `█`, in its ramp's hue; a version in its family's
  colour, an older one faint; and what the others fade toward the canvas, as the page behind a card
  or a stale account's bars, faint (settled in the plan). What's dim, its text, borders and tracks,
  is the terminal's own foreground, faint, and `bg.selection` and `bg.attention` are reverse video,
  so it leans on no colour that's some palette's background. Themes are colour alone: a look that
  needs other glyphs or capitals, as the instrument cluster spiked in round 1 does, belongs to no
  theme. No theme has a sheet in `testdata/vhs/reference/`: milestone 5's left it with its frames
  (settled in the plan).
- **Choosing:** one theme, or a pair, one for a light terminal and one for a dark, the terminal's
  background asked once as the dashboard starts (OSC 11). The dashboard and `usage` both give the
  terminal 150 milliseconds to answer before they draw, one that doesn't taken for dark; the
  dashboard takes the answer whenever it comes, as the background found, and one that comes late
  and gives the other half of the pair has the dashboard drawn in that from then on. Nothing
  chosen means the pair: `tokyo-night-day` for light, `nord` for dark. `t`, on every page, opens a
  slide-over at the right, drawn over the page so it stays visible: the themes, each previewed live
  as the arrows reach it; `enter` sets one theme; `d` and `l` set the dark and light halves of the
  pair; `esc` closes it, putting back the theme in force. A row's badge says what it fills: `●`,
  `● light`, `● dark` or `● both`. Setting one theme clears the pair, and setting a half of the pair
  clears the one theme, asking `y`/`n` first. Each is set in the choice as it's kept now, as another
  dashboard may have changed it since the picker opened, and `y`/`n` is asked only where one theme
  is kept. While the picker is open, the key line lists its keys alone.
- **The preferences file:** `<state dir>/prefs.json`, which the dashboard writes and the user never
  needs to: the theme or the pair, and what the dashboard's keys choose that it keeps (see The
  page). It's written whole, as `state.json` is, in the state directory as it changes as the
  dashboard is used, never in the config directory, whose file is often a link into the user's
  dotfiles; and never the config file, which is the user's, and which the dashboard never rewrites.
  Its keys are written in sorted order, and those it doesn't know, as a newer build's, or an older
  one's that this build no longer keeps, are kept as they are; a value it doesn't know, as a page
  an older build kept, reads as the default. One that can't be read is set aside as
  `prefs.json.corrupt-<unix time>`, or, where it's a link, where it leads, the link kept, and the
  defaults stand.
- **The background:** the dashboard owns it: it paints `canvas` on every cell and sets the
  terminal's background to it, having asked for the old one first (OSC 11), and puts it back on
  every exit it can catch: quitting, an interrupt, a terminate signal, or a panic it recovers from;
  where the terminal didn't answer, it resets it instead (OSC 111), which restores the terminal
  profile's own. A kill leaves `canvas` as the background until the terminal's reset, so a
  background the terminal reports that's a dashboard's own canvas is never set back: it's reset
  with OSC 111 on exit, its half of the pair going by how dark it is all the same. A theme that
  paints none, shown mid-session, as from the picker, sets back the background found, or resets it
  with OSC 111 where none was found. The blends the charts and bars draw in are worked out against
  `canvas`. `usage` paints no background, printing into the scrollback: where it shows colour, and
  never from a job in the background, it asks the terminal for its background (OSC 11), through
  `/dev/tty`, picks the light or dark half by it, and blends against the colour it gets. It asks
  the terminal's device attributes after (DA1), which every terminal answers, so one that doesn't
  answer OSC 11 is known at once. Having given up on an answer, it keeps the terminal raw for a
  grace of 100 milliseconds, ending as the DA1 reply comes, reading and dropping what comes late,
  so no answer is left in the shell, and an answer that comes in the grace isn't taken. One theme
  chosen prints only on a background as dark or light as its own, else the default pair's half for
  that background. The `terminal` theme never paints, and blends nothing.
- **Fewer colours:** the frame is drawn in the theme's colours and brought down to what the
  terminal shows by `colorprofile`. With `NO_COLOR` set, there's no canvas and no colour: state is
  told by its glyphs and bold, a bar's fill is drawn in `█` and the eighth blocks, as `usage`, the
  printout says, a day's shade by a glyph's density, as the `terminal` theme draws it, and `t` does
  nothing.

### Live updates

- **Where it reads:** `dashboard`, `usage` and `status` read the router's status document whenever
  the router answers its health check within the half second `run` gives it, healthy or not: an
  unhealthy router's trouble is for them to show, and it still posts the notifications. Otherwise
  they probe every account, and the document's `fallback` says why the router's wasn't read:
  `{"router": "not running"}`, or `{"router": "unhealthy", "reason": "…"}` when something answered
  its socket, but not as a router does, or not within the half second (`no answer within 500ms`).
  `--probe` probes regardless, saying nothing of the router. The dashboard, and the data verbs that
  look back, read the ledger's, the readings history's and the events' files where they lie, the
  router there or not (see The data verbs).
- **The dashboard's reads** (`dashboard [interval]`). Reading the router, it looks at the router's
  document every 5 seconds, which costs nothing upstream, and every interval has the router
  `POST /refresh` with the interval as `max_age`, so idle accounts are probed no more often than
  the dashboard asks: sooner, backing off from 2 minutes to the interval, while an account can't be
  read. A minute after a window on screen resets, the next look has the router refresh first with
  a `max_age` of a minute, once a reset, so an idle account's window doesn't read `resets now`
  until the next interval; but not for the windows of an account whose 5-hour window has lapsed, as
  the router probes it only while it can take no request (see Priming): that window reads empty
  instead, and the account's others as read. Probing, it reads every interval, a minute after a
  window on screen resets, and sooner after a failure, backing off from 2 minutes to the interval.
  A look never probes: when the router stops answering one, the router's last document stays on
  screen, the status line saying since when the router hasn't answered (see The page), and the
  looks go on every 5 seconds, reading the router again as soon as it answers. A router away for a
  moment, as when it restarts or is slow on waking, so has no account probed directly, which would
  start every lapsed 5-hour window at once, off the priming schedule. Only the next full read, due
  an interval after the last, or `R`, probes instead. Probing, it asks after the router at each
  probe and once a minute between, and reads it again as soon as it answers, so it never goes back
  and forth faster than that.
- **`R`** refreshes, on every page: it has the router probe the accounts it hasn't read in the last
  minute, and those that can take no request anyway, however lately it read them, as a reset made
  by hand shows only to a probe, but for those whose 5-hour window has lapsed and that can take a
  request, and those it probed in the last minute; or, without the router, every account is
  probed, as `usage --refresh` does. It's `R`, shifted, as a probe costs upstream.
- **What each look reads:** the status document and, from the router, `GET /sessions`, for the
  sessions ROUTING counts, the Overview's SESSIONS, Accounts' Detail, Sessions, and the routing
  pickers' counts of sessions running; and on a session's page, `GET /sessions/{id}`, for where its
  models go now and its own pin. All are local, and cost nothing upstream. A router that can't
  list its sessions shows none, never those another router listed.
- **What moves** at each look, and every second with nothing read, is as What moves says.
- **The history** behind GRAPHS is read with each full read, not each look: `GET /history` (see
  Control API), each window's over its current length, the 5-hour window at 5-minute steps and the
  weeks at 30-minute steps. Without the router, GRAPHS draw from the readings history's files,
  read where they lie, and the readings the dashboard takes itself as it probes
  (settled in the plan).
- **The request stream:** while a page that draws what it tells is shown, the dashboard subscribes
  to the router's request stream, `GET /stream` (see Control API): the Overview, with ROUTING or
  SESSIONS shown, whose traces and sessions it moves; Accounts' Detail, with its sessions;
  Sessions; the Log; and a session's page (settled in the plan). A frame is drawn only for what
  moves on screen, never for what's scrolled out of view, under a card or under `?`, so nothing new
  is drawn while nothing happens; the motion's own pace is What moves'. The stream opens with the
  requests already in flight, so a page opened mid-answer shows it, `asking` or `answering`. A
  stream that ends is joined again a second later, and one that fails to open is tried again after a
  second, doubling to 30 seconds. A reader the router drops for falling 256 events behind joins
  again, and what it missed is taken from the ledger's lines as it does (settled in the plan). A
  router from before the stream has none, and the dashboard stops asking until another router
  answers: its pages draw the sessions as each look reads them, with nothing in flight and no
  requests travelling.
- **The traces and a session's story:** the Overview's traces take their last 30 minutes from
  today's lines once, as the Overview is first shown or the stream is joined again, and a session's
  story its life from its own days' lines, as its page opens; then each counts the requests the
  stream tells of as they're sent, each request once, by its id, whether the stream or its line
  tells of it first. No look reads the ledger for them (settled in the plan).
- **The ledger, read as it grows.** The dashboard reads the ledger's, the readings history's and
  the events' files itself, with no router, through the readers it's given (see Packages), and
  never reads them whole at a look:
  - **Today's lines** are read once, as a page first needs them, then tailed: each look reads only
    what the day's file gained since the last read ended, so a look costs what the day gained,
    never the day. A file found replaced, or shorter than where the last read ended, is read again
    from its start (settled in the plan).
  - **A past day's summary** is read once and held for the run, until its stamp or its sizes change
    (see The request ledger). **Today's** is a running tally of the lines tailed, never summarised
    afresh at a look (settled in the plan).
  - **A session's days** are those the summaries name it in, by their session ids (see The request
    ledger), so a session's page reads its own days' lines alone, newest first, as far back as the
    page shows.
  - **A period, or a day stepped back to,** as Accounts', History's and Runway's are, is read once
    as it's shown, then held; History's year is a year of summaries, read once a run.
  - So what a look costs grows with what the page shows, never with what's kept: each cost is
    measured against a year of heavy use before milestone 7's release (see Milestones).
- **Events:** live, the status document's `events`, the newest 50, read at each look, each new one
  known by its `id`; the past, the events' files, read as the Log's Events, LATELY or a session's
  page reach back past them (see The router's events). A router restarted tells its events afresh,
  while the files keep what it told before. Without the router, there are no live events, and the
  Log and LATELY show what the files hold (settled in the plan).
- **A router from before milestone 7,** as one still running between an upgrade and its restart:
  without `dir` on its sessions and its stream, a running session's directory comes from its lines'
  `dir`, else it's named by its id; without the new kinds of events, LATELY and the Log have none of
  them; and without the status document's new fields, the dashboard works out the allowance, the
  even pace, the pool's room and COMING UP itself, by the functions the router's come from
  (settled in the plan). A router from before milestone 5 has no `GET /history`, `events` or
  `GET /stream`: GRAPHS draw from the readings history's files, LATELY and the Log from the events'
  files alone, and nothing travels.

### Kept from the dashboard before milestone 5

- Desktop notifications: see Notifications.
- Text from elsewhere, such as labels and the upstream's errors, shows with its control characters
  as spaces, here and in `status` alike, so none can move the cursor or restyle what follows, a
  statusline's included.
- `status`'s text keeps the form and the words it has, as `usage`, the printout quotes them, until
  it's designed afresh with the dashboard's words, as the Backlog says; it projects as the
  dashboard does.
- Built with Bubble Tea v2 and Lip Gloss v2, its colour brought down by `colorprofile`, as the house
  rules say.

## Health

The launcher routes a new session only when the router answers its health check `ok`, saying where
its proxy listens; otherwise the session connects directly. The harder case is a router that is
running but failing requests: sessions already routed through it fail until they restart. The router
tracks the requests it has routed over the last 5 minutes, and those it failed itself: a 502 for an
upstream it couldn't reach, or for a refusal with no account left to fail over to. The upstream's
own 429s and 5xx, passed through, don't count against it, and nor does a failure of a request that
arrived before the router last noticed the Mac wake, which may have gone out on a connection the
sleep left dead (see The router looking after itself). It's unhealthy once it has failed 5 of
them at least, and half at least: `GET /health` then answers `ok: false` with a `reason`, the status
document's `router` object says the same, and the log notes the turn, and the turn back, at warn and
info. Its health is judged whenever it's asked for, and at the router's look at the accounts every
15 seconds too, so the turn back is told of as the failures leave the 5 minutes, though no request
comes. `status` and the dashboard show trouble loudly: they read an unhealthy router's document
all the same, `status`'s router line reading
`from the router: unhealthy, <reason>  ·  <sessions>  ·  <routing>`, the dashboard's status line
`● unhealthy`, in red (see The page), and the Log's Events the reason, in the router's `health`
event (see The Log).

Sessions don't fall back to going direct automatically. A running Claude Code can't change where
it sends its requests mid-session; the failures that count against the router, an upstream it
can't reach and refusals with no account left, would meet a direct connection too; and a new
launch already goes direct while the router is down or unhealthy.

## Notifications

The router sees each limit, move and return as it happens, whether or not a dashboard is open, so
while it runs, the desktop notifications are its own. `[notifications]` in the config says which
it posts: see Config.

- **Limits:** a limit's notification waits 5 seconds for the sessions the limit moves off its
  account, as it holds their next requests back there, then tells of them together:
  `work · Work hit its Session limit, back at Mon 18:10 — 3 sessions moved to side · Side`. With
  none moved, it says when no other account has room. When the account is back goes unsaid where
  it would make the message longer than a banner shows. One notification a limit, however many
  requests reach it: one reached again while it holds is the same limit, but another limit reached
  meanwhile, in windows the holding one doesn't name, has a notification of its own, with its own
  moves (see Choosing an account, step 6). The moves are gathered by the limit's identity, in
  whatever order the news comes: a move told of before its limit waits for it. They're gathered
  only while `limits` is on.
- **Room again:** an account whose quota for a request of any model ran out, under a limit, with
  a shared window spent, or at its reserve, and has come back: `work · Work has room again`. A
  refusal isn't quota, so one lifting is no news, or a revoked token would be announced every ten
  minutes; an account both out of quota and refused has room again once both are past. The router
  looks at the accounts on every event and every 15 seconds, so a limit lifting or a window
  resetting with no traffic is noticed.
- **Warnings:** a window passing the share given, once a reset: `work · Work: Week at 91%`.
- **Moves:** each move a limit's notification doesn't tell of, as one its limit forced once the
  limit's notification has gone out, or any while `limits` is off:
  `session 18bb978f moved from work · Work to side · Side (rescored after 1h 2m idle)`.

Room again and warnings compare an account with how it last stood, so neither tells of how the
accounts stood as the router started, nor of an account's first reading. A limit's notification
always goes out: as the router stops, those still gathering go out at once, within 3 seconds.
Any other goes out only a minute or more after the last posted about its account, a limit's
included: one due sooner is dropped, and the log says so at debug. One that fails to post is logged
at warn, and dropped too; not having gone out, it starts no quiet minute. The log names accounts by
id alone. Notifications never hold a request up: the router queues the limits and moves it tells
of, 256 at most, dropping any past that with a warning in the log, finds room again and warnings
at its looks, and posts from a goroutine of its own.

The dashboard posts its own only while it probes because the router isn't there to:
of an account with room again and a window passing the warning, as `room` and `warning` say, and
none with `--no-notify`. While the router answers, the dashboard posts none, so nothing is told
twice: probing with `--probe`, it asks after the router before it posts, and posts only when it
doesn't answer. It sees no limits or moves, which are the router's alone.

## The request ledger

The readings history says how each window's use changed; the request ledger says what used it. It
holds a line for each request the router routes, written as the request ends, and a summary of each
day once the day is over. History, Accounts, a session's page, the Log, Sessions' List and the
traces look back through it, as agents do through the data verbs (see The data verbs). It records
from the first day a router that has it runs, so nothing before then is in it.

- **A line a request:** each request the proxy routes, whether it went upstream or the router
  answered it itself, as when no account has room, or its body is over 64 MiB or can't be read
  (see Requests that need special handling); not those passed through, nor the router's probes
  and primes, which the readings history notes. It keeps everything about the request but its
  content, as a day not recorded can't be recorded after: how the router handled it, the
  request's shape, and everything the API said back. The line is written as the router finishes
  with the request, when it logs it `routed` (see Logging), or refuses its body, as one JSON
  object:

  ```json
  {"at": "2026-10-06T13:12:00.123Z", "request": "3f2a91c4", "kind": "message", "session": "5b0e…", "dir": "~/Code/project", "model": "claude-opus-5-5", "account": "work", "reason": "sticky", "status": 200, "attempts": 1, "first_ms": 812, "total_ms": 14230, "agent": "claude-cli/2.1.0 (external, cli)", "betas": ["context-1m-2025-08-07"], "shape": {"bytes": 482113, "messages": 214, "system": 3, "tools": 31, "max_tokens": 32000, "thinking": {"type": "enabled", "budget_tokens": 31999}, "stream": true}, "answer": {"id": "req_011C…", "model": "claude-opus-5-5", "stop": "tool_use", "blocks": {"thinking": 1, "text": 1, "tool_use": 2}, "tools": ["Bash", "Read"]}, "usage": {"input_tokens": 12, "cache_creation_input_tokens": 3120, "cache_read_input_tokens": 182340, "cache_creation": {"ephemeral_5m_input_tokens": 0, "ephemeral_1h_input_tokens": 3120}, "output_tokens": 845, "service_tier": "standard"}, "limits": {"status": "allowed", "5h-utilization": "0.23", "5h-reset": "1791320400", "7d-utilization": "0.41", "7d-reset": "1791590400"}}
  ```

  - `at`: when it arrived, before its body was read, in UTC, to the millisecond.
  - `request`: the router's id for it, as its `routed` line and the request stream give it, unique
    while the router runs but not across restarts.
  - `kind`: `message`, a request that spends quota; `check`, Claude Code's quota check (see
    Choosing an account); or `count`, a token count, which spends nothing.
  - `session`: Claude Code's session id, left out where a request carries none. `dir`: the
    directory `run` started `claude` in, as it tells the router (see Launching), the home directory
    at its start shown as `~`, as `~/Code/api`; left out where a request names none. A `claude`
    started within a session other than through `run`, such as the Agent SDK's bundled CLI in a
    script the session runs, or the real `claude` run by its path, inherits the session's
    environment, and with it the headers `run` set: its lines carry the directory of the session it
    was started within, as its requests carry that session's pin; the router can't tell the
    difference. It's the directory `claude` was started in, which a session doesn't keep: one
    resumed elsewhere carries the new one from then on. `model`: the model it asked for. Each is
    cut to 200 bytes once anything shaped like a token in it is hidden: a session's id and a
    model's at their end, as the request stream cuts them, and a directory at its start, `…`
    standing for what's cut, so it keeps its own name.
  - `account`: the account whose answer the client got, by its id. `reason` and `from` say how
    that account was chosen: `reason` why, as the `routed` line gives it (`sticky`, `new`,
    `pinned`, `moved: personal hit its limit`), and `from` the account the request's session was
    on before, where the request moved it and the move stands: a request every account refused
    takes its moves back, unless another request of the session has been routed since (see
    Requests that need special handling). Where the client got an answer held back, the 429 of the
    first account whose limit the request reached, as every account after it refused the request,
    they're as the request went out on that account, while the `routed` line gives the last
    account's; the moves after it are in `tried`.
    `tried`: the accounts it went out on that couldn't serve it, in the order tried, and why each
    was left, as `[{"account": "personal", "why": "hit its limit"}]`, where there were any: the
    account whose answer the client got may be among them, the last, where none was left to try
    after it, or the one whose answer was held back.
  - `status`: the status it was answered with, 0 where it ended before an answer came: the router
    answers a body it can't read whoever is left to read it, 400 or 413, but nothing else once a
    request has ended. `canceled`: true where its client went away before its end. `cut_off`:
    true where the router cut it off as it stopped, once the 30 seconds it gives requests in
    flight had passed; `canceled` is then false, as the client didn't go. Whichever came first is
    the one given. `attempts`: how many times it went upstream, 0 where the router answered it
    itself.
  - `first_ms` and `total_ms`: how long after it arrived its answer's first byte passed on to the
    client, and its end. `first_ms` is left out where the client got no answer of the upstream's,
    as where the router answered it itself.
  - `agent` and `betas`: Claude Code's user agent, which carries its version, and the features its
    `anthropic-beta` header asked for, some of which change what a request costs.
  - `shape`: the request's size in bytes; how many messages, system blocks and tools it carried, as
    counts; and those of its settings switchboard knows: `max_tokens`, `thinking`, `stream`,
    `tool_choice`'s type, `temperature`, `top_k`, `top_p`, `service_tier`, `output_config`'s
    `effort`, `speed`, `inference_geo`, and the types of `context_management`'s edits, nothing of
    their parameters. A field it doesn't know isn't kept, nor a setting's field it doesn't know, as
    one may carry a secret, as an MCP server's token does. The router reads the shape in the pass
    over the body it already makes for the model and the quota check, so it costs a request next to
    nothing.
  - `answer`: what came back, of the answer the client got: Anthropic's id for it, from its
    `request-id` header; the model that served it, the one it named, unless the API's server-side
    fallback handed it off partway to another, named by the `fallback` block that marks the handoff,
    the last such block's where there are several; why it stopped (`end_turn`, `tool_use`,
    `max_tokens`); how many blocks of each kind it held; the tools it called, by name alone; and, of
    an error, its type and message, cut to 200 bytes. Each is left out where the answer didn't give
    it, or gave it as another type than the API gives, the rest of the answer read all the same. A
    block a stream gives whole on a line over 1 MiB, as a web fetch's result of a large PDF can be,
    is passed over, uncounted, and the rest of the stream read.
  - `usage`: the answer's closing usage, as the API gave it, field for field, so whatever it counts
    is kept as it is: cache writes for five minutes and for an hour, which cost differently, web
    searches, the service tier, and anything it counts later. Left out where the answer gave none,
    as an error, a count or an answer cut short gives none.
  - `limits`: the answer's `anthropic-ratelimit-unified-*` headers, their prefix taken off, their
    values as given: each window's use, reset and status as the answer left them. Beside the
    request's usage, they say how many tokens a point of a window is worth.

  Of an answer whose body the router couldn't read as it passed, one in an encoding it can't
  decode, or one it fell over 4 MiB behind reading, only what the header gives is kept: `answer`'s
  id, and `limits`. Of a request whose body couldn't be read, the line holds what the router knows
  without it: no `model`, `account` or `shape`, `reason` empty, and `kind` a `count` where its path
  spends nothing, else a `message`.

  What's kept can be trimmed, or kept for less time, or packed tighter, at the review of what grows
  with time before milestone 7's release (see Milestones); what isn't kept can never be added for
  the days gone by. So it keeps all of this until then.

  Never written: the messages, the system prompt, the tools' definitions, inputs and results, or
  anything else of the conversation; `metadata.user_id`, which carries a device's id and an
  account's; and any header but those named here, a token's never. An account appears by its id
  alone, never by a token or a label. Fields may be added to a line, never renamed, and a reader
  passes over those it doesn't know.
- **Writing never holds a request up.** The request hands its line to a queue of 8,192 lines at
  most, longer than the readings history's, as a line dropped is a request History never counts,
  and goes on. A goroutine of the ledger's own writes the lines to the day's file, as the readings
  history writes its own (see Files). A line past the queue's end is dropped, logged once until the
  queue catches up; a write that fails is logged once until one succeeds, and its lines go
  unwritten. As the router stops, the goroutine writes what's queued once the requests it routes
  have finished: those still going after their 30 seconds are cut off, and get 5 seconds more to
  unwind, each until its line is noted; the lines of any still in flight then go unwritten, the
  log warning how many. Those passed through, which have no lines, aren't waited for. A benchmark
  of the proxy path, taken before the ledger joins it and again after, shows what the ledger costs
  a request (see Milestones).
- **A summary a day:** once a day has ended, on the first round an hour or more after, as a
  request still in flight as the day ends has its line filed under the day once it's done, the
  router writes the day's summary from the day's lines and the day's readings history, as one JSON
  object. The rounds are those that compress and prune the ledger's files, every hour and as the
  first line of a day is written, each made before its files are pruned; and the readings
  history's writer has the ledger summarise the days that have ended just before each prune of
  its own files, once a day, rather than at every round: so neither a line nor a reading a summary
  needs is pruned before its day is summarised, however long the router was stopped, or the Mac
  asleep, which holds the hourly rounds back. Summarising several days reads the readings history
  once for each run of them, days whose weeks before overlap the day before them, ahead, as the
  days ask for their readings, oldest first, holding a week and a day of them at a time: never
  the readings of a year between a day left unsummarised and yesterday. Its object:

  ```json
  {"version": 2, "day": "2026-10-05", "lines": 424, "bytes": {"plain": 0, "compressed": 41208}, "accounts": [{"account": "work", "models": [{"model": "claude-opus-5-5", "upstream": 412, "no_usage": 2, "unsent": 0, "checks": 3, "counts": 9, "sessions": 6, "usage": {"cache_creation": {"ephemeral_1h_input_tokens": 1180240, "ephemeral_5m_input_tokens": 0}, "cache_creation_input_tokens": 1180240, "cache_read_input_tokens": 61204410, "input_tokens": 8812, "output_tokens": 402113, "server_tool_use": {"web_search_requests": 4}}}], "sessions": 6, "session_ids": ["0c41…", "3e7a…", "5b0e…", "8d02…", "a9f3…", "e117…"], "moved_on": 2, "moved_off": 1, "highest": {"5h": 1, "7d": 0.41}, "rise": {"5h": 2.03, "7d": 0.12}, "resets": [{"window": "5h", "at": "2026-10-05T07:10:00Z", "before": 0.35}, {"window": "5h", "at": "2026-10-05T12:10:00Z", "before": 0.58}, {"window": "5h", "at": "2026-10-05T17:10:00Z", "before": 1}], "limits": [{"window": "5h", "at": "2026-10-05T14:37:12Z", "resets_at": "2026-10-05T17:10:00Z"}], "minutes_at_cap": 0, "minutes_at_limit": 153, "read_before": ["5h", "7d"]}]}
  ```

  - `version`: the summaries' version: 2 since milestone 7, which added `session_ids`, `rise`,
    `resets`, `minutes_at_cap` and `minutes_at_limit`, and 1 before it. A release that summarises a
    day otherwise, or keeps more of it, writes another, so the summaries written before it can be
    told: a day whose summary is of an older version is read, and summarised again from its lines
    while they're kept, once, on the first round under the new version, as the round's stamp holds
    the summary's version (see below); a reader that meets an older version's summary, its lines
    kept, summarises the day from its lines, as it does today's; and of the rest, whose lines are
    gone, a reader knows what they never counted, rather than taking it for none. Version 2 only
    adds fields, so a release from before it reads a summary of version 2 as one of its own, passing
    over what it doesn't know, and keeps those fields when it marks the summary afresh; where it
    summarises the day again, as lines come to be filed under it, it writes version 1, which the
    next release to run summarises again while the day's lines are kept. Of a summary of version 1
    whose lines are gone, a reader counts its accounts' `sessions` as distinct, as the nearest there
    is, and takes its rises, resets and minutes as never read, never as none.
  - `day`: the local date the day's lines are filed under.
  - `lines`: how many of the day's lines it was made from, those that didn't read as a line among
    them, which are warned of as it's made: what the summary is checked against, as below.
  - `bytes`: the sizes of the day's files, its plain file's and its compressed file's, as the
    summary was made from them, or last found to stand: a round that finds them so counts none of
    the day's lines, as below. A summary made as it's read, never written, has none.
  - `accounts`: each account's day, in the order of their ids; one without an `account` holds the
    requests the router answered without one, as when no account had room, or a body couldn't be
    read. A day of no requests has none. Its `models` are its requests by the model each asked
    for, one without a `model` holding those whose bodies couldn't be read, in the order of their
    names: `upstream` counts those that spend quota that went upstream, and `no_usage` those of
    them whose answers gave no usage, as one canceled, cut off or an error gives none, so what
    they spent is unknown, and their worth unpriced, never taken for none (see Worth); `unsent`
    counts those the router answered itself, never sending them; `checks` counts Claude Code's
    quota checks, and `counts` the counts of tokens, however each went; `sessions` counts the
    sessions they were of, but for the checks', as Claude Code
    sends one as it resumes a session under an id it never uses again; and `usage` is their usage
    summed, field for field as the API gives it, an object's fields within it, and the objects of a
    list that give their types, as an answer's `iterations` do, within it by their types, what
    isn't a count, as the service tier, left out. A model's requests that asked for an inference
    geo, as their shapes' `inference_geo` gives it, are a day of their own, after its others,
    naming it in `inference_geo`, as US-only inference costs more (see Worth): the usage summed
    keeps no text, so couldn't tell them apart, and a day of the requests Claude Code sends, which
    ask for none, reads as it would without them.
  - Of the account as a whole: `sessions`, counted as its models' are; `moved_on` and `moved_off`,
    the sessions its requests moved onto it and off it, as their lines' `from` says; `highest`,
    each window's highest use that day, by its key, counting the use it began the day at, as the
    last reading of it in the week before gave it, unless the window had reset by then; and
    `limits`, the limits it reached, each a window's status turning `rejected` that day, from
    another status or another window's, one with another reset: when it was read so, and when the
    window was to reset; and `read_before`, the windows the readings history read in the week
    before the day, by their keys, as the summary was made, which a summary made again goes by, as
    below.
  - Of the account as a whole too, since version 2:
    - `session_ids`: the ids of the sessions its `sessions` counts, in order, each once: so a span
      of days, or every account, counts each session once, however many days and accounts it ran on,
      and the days that hold a session's lines are known without reading them (see A session's
      page).
    - `rise`: how far each window's use rose that day, by its key, from the use it began the day at,
      as `highest` counts it, to its highest before each reset, and from nothing after it, the rises
      added together: the points of the window the day used.
    - `resets`: each reset of a window the readings history saw that day, in the order they came:
      the window's key; when it reset, its reset time where it was read, or when the reading that
      showed a reset made by hand came (see Choosing an account); and `before`, its use as last read
      before it reset. A week's peak is the `before` of the reset that ended it.
    - `minutes_at_cap` and `minutes_at_limit`: the minutes of the day the account spent at its cap,
      a window every model shares at or past its cap and short of its limit, and at a limit, such a
      window read `rejected` or used up, each from the reading that put it there to the one that
      took it away, its reset, or the day's end, a minute at both counting as at a limit. The cap is
      the config's as the summary is made, as the readings history keeps none. Windows of a model's
      own, as Fable's week, hold back only that model, and count toward neither.

  A summary is written whole, and written again while lines come to be filed under its day: a
  request in flight past the hour the day is given, a change of time zone, or a clock set ahead and
  set right again, files a line under a day already summarised, which `requests` lists, and which
  `history` would never count otherwise. So, at each round, while a day's lines are kept, a day
  whose files hold more lines than its summary's `lines` is summarised again, over it. None is
  taken away while they're kept, so fewer means some were lost since, as to a damaged file, and
  the summary made from more stands; once they're pruned, it's kept for good: a year of them runs
  to a few megabytes.

  A summary written again knows no less than the one it replaces. Each count it holds, a model's
  requests, sessions and usage, field for field, and an account's sessions and moves, is the one
  before's where that's more, as fewer means lines were lost since, and an account or a model the
  lines no longer give is kept as it was. Which lines were lost can't be told from those filed
  since, though: where some of a model's requests were lost, and others of it filed since, it
  counts the more of the two summaries', fewer than there were. Its `lines` are as many as the
  more of the two were made from, and never fewer than its requests, as those of lines lost count
  among them. Each window's highest use is the one before's where that's higher, and the limits
  and the `read_before` the one before held are among its own. Its `session_ids` are its own and the
  one before's together; each window's rise, and each of its minutes, are the one before's where
  that's more; and the resets the one before held are among its own, a reset being the same where
  its window and the time it reset are, its `before` the higher of the two. The readings history
  prunes the readings of a day past its own keep, whatever the ledger's, and a clock moved can leave
  it short of them, so what the readings gave can't be had again from them: a limit whose window has
  no reading before it, in the day or the week before, which a summary takes for one reached, is its
  own only where the summary before read none of that window in the week before either, as its
  `read_before` says, as the reading it read may be one pruned since, which would have said the
  limit held already. So one read first where the summary before read no readings, as of a day whose
  only line was torn, or while the history couldn't be read, is kept.

  A round counts a day's lines only where its files have changed since its summary was marked:
  the summary holds their sizes, as `bytes`, and its modification time, its stamp, is set to when
  the day's compressed file was last modified, as they were when it was made, or last counted and
  found to stand. The stamp holds the summary's version too: a day whose summary is of an older
  version is read, and summarised again from its lines while they're kept, once, on the first round
  under the new version. A day with no plain file, whose compressed file still gives that time,
  costs a look at its files and its summary's, reading none of them: compressing a day writes its
  compressed file anew, once a day at most. That look reads no summary, so it takes one damaged in
  place, or of another day, as by a restore or a copy that keeps its time, as it stands; what reads
  it, a reader, or a round that must, warns of it, and summarises its day from its lines, while
  they're kept. Any other has its summary read, and its lines counted only where its files' sizes
  differ from those it holds: lines are only ever appended to the plain file, which grows with each,
  however coarsely the file system keeps its times: one that keeps whole seconds, as HFS+ does,
  gives a line appended within the second the file was last modified in no time of its own. A
  summary found to stand where they differ is marked afresh: its `bytes` alone are rewritten, every
  other field kept as it's written, those a later release added among them, so a release before it
  never drops them from a summary that keeps that release's version. Read whole, a year of heavy
  days runs to gigabytes. The time must be the same, not merely no later, as a clock set back, or
  set ahead and right again, can give a file changed since its summary an earlier time. Fields may
  be added to a summary, never renamed. A day left without one, as when the router was stopped as
  the day ended, is summarised by the next round that finds its lines, compressed or not. A day one
  of whose files can't be opened isn't summarised from the rest, as that would be taken for the
  whole day, but left for a later round, as is one whose summary, or lines, can't be read to tell
  whether it stands, each warned of at each round until it can be; one whose summary, read, doesn't
  read as the day's, as one damaged or of another day, is warned of, and summarised again. A day
  whose files hold no line that reads is summarised as one of no requests, and again once a line
  comes that does. Views over weeks and months read the summaries; today, and any day not summarised
  since lines came to be filed under it, is summarised from its lines as it's read.
- **Worth** is what a request would have cost through the API, priced from a table built into
  switchboard, read from Anthropic's pricing page on 7 October 2026: each model's prices of input,
  output, cache reads, and cache writes for five minutes and for an hour, each as the page gives
  it, never worked out from another, as a cache read's share of the input's differs from model to
  model; a web search's, $10 a thousand beyond its tokens; and what US-only inference costs, a
  tenth more on each token's price, on Claude 4.6 and later, where a request's shape asks for
  `inference_geo` `us`. Each price carries the day it took effect, those of the table each model's
  launch day, as none has changed since; so a request can be priced as at its own day or at
  today's prices, and the views price at today's unless their design says otherwise. Worth is
  worked out as it's read and never stored, so a release with new prices prices the whole ledger
  afresh. A model the table doesn't know has no worth, and is shown as unpriced, never as free, as
  is a request that asked for an inference geo the table doesn't price its model in.
  - The table holds the models on the page, but those the API retired before 7 October 2026, the
    day the ledger began recording, as no answer of theirs is in it to price.
  - A count the usage holds that the table can't price is left out of the worth, which names it
    unpriced, rather than guessed at: a server tool's it doesn't know, writes to the cache the usage
    doesn't break down by how long they last, and the turns of another model, as an advisor's,
    which the API charges at that model's prices. Web fetches, which cost nothing beyond their
    tokens, the thinking within the output, and the turns of the model asked for, which the usage's
    own counts sum, cost nothing more.
  - A request that went upstream and spends quota, but whose answer gave no usage, as one
    canceled, cut off or an error gives none, spent what can't be known: the worth names it
    unpriced, as `no_usage`, a request's own or a summary's model's day's, never priced as free.
  - Every request the router routes runs at standard speed and the standard service tier, so the
    table holds no other prices: fast mode never reaches the router (see What doesn't go through
    the router), and a subscription's requests are served at the standard tier.
  - *No long-context rate (7 October 2026):* Claude 4.6 and later price their whole 1M-token
    context at their standard rates, since 13 March 2026, and the 1M-token beta that charged more
    on Claude Sonnet 4 and 4.5, which `context-1m-2025-08-07` asked for, was retired on 30 April
    2026, before the ledger began recording. So no request the ledger holds pays one.
  - **Plans:** beside the models, the table prices the plans a subscription is on, each a month, in
    US dollars, with its size in Pros, which History's replay of a week weighs an account by (see
    History): `pro`, Pro, $20 and 1; `max5x`, Max 5x, $100 and 5; and `max20x`, Max 20x, $200 and
    20. An account's plan is the config's `plan`, as switchboard can't read it: a setup token can't
    read the profile endpoint that knows it (see Requests that need special handling). An account
    without one has no price, and nothing is priced against it.
  - **Overrides:** the config can price any model, or any plan, otherwise (see Config). An override
    prices every day alike, rather than from a day of its own, and where any holds, the date shown
    beside worth says so: `at its prices as of 7 Oct 2026 and the config's`.
- **The version table,** beside the price table, names each model version as the views show it, and
  gives it its colour. A row a version, by its ids as the price table holds them: its name,
  `Opus 5.5`, `Sonnet 4.5`, `Haiku 4.5`, `Fable 5.1`; its family, as `internal/claude` reads it from
  the id, `opus`; and its shade. Its rows go by family, Opus, Sonnet, Fable, Haiku, then Mythos, and
  within a family newest first, by the day each launched, so `Opus 5.5` comes before `Opus 5`. The
  first four families are drawn in `viz.family.1` to `viz.family.4`; Mythos, the price table's
  `claude-mythos-5-1` and `claude-mythos-5`, is a fifth, with no hue of its own, drawn in
  `text.muted` with its versions, in charts and tables alike. The table assigns `viz.version.1` to
  `viz.version.6` in its rows' order: Opus 5.5 and Opus 5, Sonnet 5.5 and Sonnet 5, Fable 5.1, and
  Haiku 4.5; a release that adds a version assigns them afresh. A version without one draws in its
  family's colour, mixed 30% toward the canvas when it's older than the family's newest (see
  Themes). A version the table doesn't name is shown by its id, `claude-` taken off, `opus-6`, in
  its family's colour, or in `text.muted` where its family has none. Tables name versions; charts
  keep to families.
- **Requests,** wherever a view or a command counts them, are a summary's `upstream` and `unsent`
  together: every message, the kind of request that spends quota, whether it went upstream or the
  router answered it itself. Claude Code's quota checks and the counts of tokens, which spend
  nothing, are counted apart, as `checks` and `counts`, and never among the requests, so History,
  Accounts, the Log and `history` give a day the same number.
- **Reading needs no router.** The dashboard and the read commands read the ledger's files where
  they lie, through one package, `internal/ledger`, given the state directory and a clock, and the
  readings history beside them through `internal/readings`. A line that doesn't read as one, as one
  cut short, is passed over, and a damaged compressed file is read up to the damage, as the
  readings history is read; how many were passed over is logged. Lines are read oldest first, by
  when each arrived, whichever day's file each is in, as a day's are written as their requests
  end, and a change of time zone files a line under the date beside its own, never further:
  ordering them holds a day or two of those asked for at a time, a line of another time, as one of
  the day before the first asked for, whose file is read for those filed under it, passed over as
  it's read, never held; and so does ordering the readings history's.
  A day's summary is read as the ledger holds it while it stands: its lines are pruned, or are as
  its stamp, or its `bytes`, say, or number no more than its `lines`, or can't be counted, which is
  warned of. One of an older version, its day's lines kept, doesn't stand: the day is summarised
  from its lines as it's read, as today's is. Today, and any day not summarised since lines came to
  be filed under it, is summarised from its lines as it's read, knowing no less than any summary it
  would replace, the readings history read ahead for them all, as the router reads it, and never
  written, nor marked, as summaries are the router's to write. A summary that can't be read, or
  doesn't read as its day's, as one damaged in place or of another day, whatever its stamp, is
  warned of, and its day summarised from its lines, while they're kept: once they're pruned, there's
  nothing to give of the day. A read starts at the first day the ledger holds, the earliest its
  files of lines and its summaries are of, where that's later than the start asked for, as no day
  before it holds anything; at today where it holds none. Every day asked for from there is given,
  so the last is today, one of no requests as a summary of no accounts: none from before the ledger
  began. A session's page reads the lines of its own days; History and Accounts read the summaries.

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
  by its reserve, and let go at its reset; each choice that passes an account over as under
  pressure, with its rate and when it runs out at it (`passed over under pressure`), the routed
  line's reason naming it too; a limit a 429 to a request sent before a reset made by hand
  reaches, passed over as the request goes out again (`limit from before a reset passed over`); a
  token file read again after a 401, and whether it held a different token; a token replaced
  while the router was away, which it finds as it starts; a token file found holding another
  token, an account gaining a usable token, and one losing it, with why; the tokens directory made
  private as the router starts, and what bringing the skill up to
  date did; a config change, and an upgrade; the tokens of an account the config no longer
  configures counting as the primary's; a restart either makes due, once, and the restart as it
  goes: `replacing itself`, with the binary's path, why, and the listeners handed over, as the
  variable names them, and, from the router it becomes, after its `start`, with its version, each
  listener it takes up; each try again of a binary that isn't there; and a restart a signal stops.
  At `warn`: as the router starts, each account without a usable token, and, when none has one,
  that nothing will be routed until one has; a prime that failed, or didn't start the window; a
  config change refused, as invalid at two looks in a row; a restart that couldn't replace the
  process, and exits instead; and listeners handed over that couldn't be taken up. At `debug`, a
  token file found holding no usable token at one look, which the account's token outlasts, and a
  config file found making no valid config at one look. Of the readings
  history (see Files): at `info`, how many readings the router took up from it as it started
  (`took up the readings history`), and that it's written again after failing (`writing the
  readings history again`); at `warn`, that its directory can't be made private (`can't make the
  readings history private`), that it can't be written, once until it can be (`can't write the
  readings history`), that readings were dropped from it for its falling behind, once until it
  catches up (`readings history fell behind`), that a reading can't be put as a line, once
  (`readings history can't hold a reading`), how many of its lines couldn't be read as a reading
  as it was taken up (`readings history lines unread`), a file or its directory that can't be
  read (`can't read the readings history`), and a file cut short or damaged as it was read
  (`readings history read short`), each file once until it reads to its end again, as
  `GET /history` reads them again and again; a file that couldn't be compressed (`can't compress
  the readings history`); and a file, or the directory, that couldn't be pruned (`can't prune the
  readings history`). Of the request ledger (see The request ledger): at `info`, each day it
  summarises, and summarises again (`summarised a day of the request ledger`, with the day and its
  requests), and that it's written again after failing (`writing the request ledger again`); at
  `warn`, that its directory can't be made private, that it can't be written, once until it can be
  (`can't write the request ledger`), that lines were dropped from it for its falling behind, once
  until it catches up (`request ledger fell behind`), that a request's line can't be put as JSON,
  once, a file that can't be read or was read short, as the readings history's are warned of, how
  many of a day's lines couldn't be read as a line as it was summarised (`request ledger lines
  unread`), a summary that doesn't read as its day's, which is summarised again (`can't read the
  request ledger's summary of a day; summarising it from its lines`), a file that couldn't be
  compressed or pruned, a day that couldn't be summarised, as one of its files, or its summary,
  can't be read, at each round until it can be (`can't summarise the request ledger`), a summary
  that couldn't be marked afresh once it was found to stand, whose day's lines are counted again
  at the next round (`can't mark the request ledger's summary of a day; its lines are counted
  again at the next round`), or stamped, which is read again at the next round (`can't stamp the
  request ledger's summary of a day; it's read again at the next round`), and that
  the router stopped with requests it routed still in flight, whose lines go unwritten, saying how
  many (`requests still in flight as the router stops; their lines go unwritten`). Of the router's
  events' files (see The router's events), at `warn`, as the request ledger's are: lines dropped for
  the queue's falling behind, once until it catches up; a write that fails, once until one succeeds;
  a file read short; and a file that couldn't be compressed or pruned.
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

Every 3 seconds, the router looks at what it was started from: the token files, its config file,
its binary and the system's time zone. It notices the Mac waking from sleep as it looks.

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
  hold does (see Proxy rules), unless the account is gone from the config, when its tokens count as
  the primary's (see Config changes). While the primary's is gone, that's every session's request.
  A single look finding none keeps the token, noted at debug: a writer that empties the file before
  it writes the token, as a shell's redirect does, leaves it so for a moment. The priming schedule
  is worked out again whenever an account gains a usable token or loses it.
- **Config changes:** it restarts itself on a change to its config file that parses and
  validates, so `accounts add`, `accounts remove` and an edit by hand all take effect without a
  command. It follows links, so a config kept in a dotfiles repo and linked counts, and a change
  is the file's identity or modification time changing. A file that doesn't parse and validate at
  one look is no change, noted at debug, as an editor saving it can leave it so for a moment, and a
  restart already due waits for the next look; at two looks in a row, 3 seconds apart, as a token
  file is given two, the change is refused, logged at `warn`, and the router carries on with the
  config it has. Until the restart, which can be hours coming (see below), the router routes by the
  config it started with, but for the tokens of an account the new config is without, which count
  as the primary's it makes from the look that finds the change, as they stood then, so the
  sessions running on them stay routed once the account's token file goes, as `accounts remove`
  deletes it; a later change that configures the account again gives them back to it (see Accounts
  and tokens).
- **Upgrades:** it restarts itself when the binary it was started as, the Homebrew link its
  LaunchAgent runs, leads to a different file from the one running, or to the same file changed
  since, as after `brew upgrade`. A link that leads nowhere, as it may for a moment while an upgrade
  moves it on, isn't one.
- **Time zones:** Go reads the local time zone once, as a program starts, so a router carried on in
  the zone it started in would prime, and end the day, by that zone's clock. It restarts itself
  when `/etc/localtime` leads to another file than it did as the router started, as when the Mac
  is taken to another time zone, under the same rules as an upgrade.
- **Waking:** the wall clock runs on while the Mac sleeps, and the monotonic clock stops, so a look
  that finds the wall clock 5 seconds or more further on than the monotonic since the look before
  finds the Mac has slept, and the log notes the wake at `info`. A sleep can leave the upstream
  connections the router keeps dead, and a request sent on one would hang until its pings failed,
  some 45 seconds on, and then fail, so from then on requests go upstream on connections of their
  own; of those before, the idle close at once, and those carrying a request finish it, and close
  once idle for the idle timeout, 90 seconds, or sooner when a ping fails. HTTP/2 carries every
  request on one connection, so closing the idle alone wouldn't do: one carrying a stream through
  the sleep would take the next request too. A request that arrived before the wake was noticed, and
  fails, doesn't count against the router's health (see Health).
- `serve` notes how the config file, the binary and `/etc/localtime` stand before it reads the
  config, and the router compares them with that, so a change or an upgrade made while the router
  starts calls for a restart too.
- Any restart waits for a moment with no requests in flight, there being no hurry, and for the
  config file to make a valid config, which the router started again needs: an upgrade while the
  config file is invalid waits for it to be put right. A connection upgraded, such as a
  WebSocket, isn't a request in flight, as it can stay open for as long as its session runs.
- **In place.** The router then replaces itself. It stops taking requests, saves its state and
  sends the notifications still gathering, as it does at a signal, then `exec`s its binary, by the
  path it was started as, which an upgrade leads on to the new version, with the command line and
  the environment it was started with. The process keeps its id, so launchd sees no exit, and has
  nothing to start. Its listening sockets, the proxy's and the control socket, stay open
  throughout: they're held as their listeners close, pass through `exec` as descriptors it leaves
  open, and are named to the router it becomes in `SWITCHBOARD_LISTENERS` (`control=9,proxy=8`),
  which takes them up in place of listening afresh. A connection made meanwhile waits to be
  accepted, never refused; the control socket's path stays, so there's no stale socket to clear,
  nor another router to look for. A listener is taken up only where the config asks for it, so a
  config that moves the proxy has the one handed over closed and the new address listened on. A
  descriptor named that isn't a listening socket is left open, as it may be anything, but marked
  to close on `exec`, so nothing the router runs inherits it, and the router listens afresh for
  that listener, logged at `warn`. The variable stays in the router's environment, which
  switchboard never changes, but reaches nothing it runs: those start in no more of it than they
  need, as `internal/childenv` gives it, and the next `exec` sets it anew. Sessions keep their
  accounts (`state.json`), readings persist, and caches, being the API's, stay warm.
- **A variable it can't read.** An upgrade has one version set the variable and the next read it,
  so its form changes only in ways the next can still read. A value the router can't read names
  nothing it can trust, so it touches no descriptor: those handed over stay open, never accepted
  on, and it finds the control socket's path taken by one that doesn't answer, waits 5 seconds for
  it, and exits, failing to start, for launchd to start it afresh, with nothing inherited. The same
  befalls an `exec` into a binary from before routers restarted in place, as a rollback would make,
  which knows nothing of the variable. The connections made meanwhile are cut off as it exits.
- **Why not exit.** The router used to exit for launchd to start it again. On 30 September 2026,
  after a `brew upgrade` following a login, macOS refused launchd's start of the new binary
  (`xpcproxy exited due to OS_REASON_CODESIGNING | Launch Constraint Violation (Constraint not
  matched)`): switchboard is ad-hoc signed, so each release is signed afresh, and no longer
  matched what macOS noted of the LaunchAgent at login. launchd's 10-second throttle passed before
  a second start succeeded, and meanwhile nothing listened on the proxy's address: running
  sessions' requests failed, and a `claude` started then connected directly. Even a start that
  succeeds leaves a moment with nothing listening. Whether macOS lets the process `exec` the new
  binary is a check owed.
- **When it can't.** A binary that isn't there, as for a moment while `brew upgrade` moves the
  link on, is tried again half a second on, 5 times in all. Should the `exec` still fail, or the
  router not know its binary, it logs why at `warn`, removes the control socket, and exits for
  launchd to start it again, as before; one that dies as it `exec`s, as macOS refusing it would,
  is started again by launchd all the same.
- **Told to stop.** As a restart begins, the proxy stops taking requests first, its listener
  closed before the drain begins. A signal while it finishes its requests in flight has the router
  stop as at one after all, at once: the sockets it holds close, so nothing waits on the proxy's
  for a router that won't come, and the control socket is removed before the control API closes,
  as macOS can leave a connection made to a unix socket as it closes hanging, neither answered nor
  closed (proven by experiment). So a launcher that finds the router gone finds nothing listening,
  and connects directly. It exits for good once those requests are done. It looks for a signal
  again as late as it can, just before the `exec`: one taken after that is this process's alone,
  and never reaches the router it becomes, so launchd, booting the service out then, waits its 45
  seconds and kills it. That moment is the width of the `exec` itself, and of the signal's way to
  the router's own context, a hand-off between goroutines.
- Only the LaunchAgent's router restarts itself: launchd sets `XPC_SERVICE_NAME` to the label of
  the job it runs, which the router checks against the service's. Run by hand with `serve`, the
  router logs, once, that a restart is due instead of restarting.
- **A restart due shows.** With many long sessions, a moment with no request in flight can be
  hours coming, so the status document gives a restart due, why and since when, and how many
  requests are in flight (see The status document). `status` says so under its router line:
  `restart due since Mon 14:02 (config changed), once no request is in flight (3 now): switchboard
  service restart restarts it now, cutting off requests still in flight after 30 seconds`, or, run
  by hand, `…: run switchboard serve again to take it up`; and the dashboard's status line, `restart
  due (config changed)` (see The page). `service restart` asks the router to restart now
  (`POST /restart`): it gives its requests in flight up to 30 seconds, and those it cuts off then up
  to 5 more to unwind, answering on the control socket meanwhile, so a `claude` started then is
  routed, its requests waiting on the proxy's socket for the router it becomes, then replaces itself
  in place, or, as it says as it takes the request when it can't, as not knowing its binary, exits
  for launchd to start it again (see Launching).
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

It's short: `claude` runs through switchboard; `status --session`; the data verbs, which print JSON
in Claude Code's Bash tool with no flag, as anywhere their output isn't a terminal (see The data
verbs): `status`, and what the status document holds, for every account's usage, and
`status --refresh` once a limit is reset by hand; `sessions`, the sessions running and today's, and
`sessions <id>`, one session's accounts, turns and totals; and `requests`, `history` and `events`,
which look back through the request ledger, the readings history and the router's events, with or
without the router; `usage`, the user's printout of every account's windows, and `dashboard`, the
user's live dashboard, which Claude points to rather than runs, as `status` gives it what they show;
`pin`, to one account or the best of several; and `logs`; a move costs one slower turn, and a moved
Claude Sonnet 5.5 session carries on without its earlier reasoning; artifacts always live on the
primary; and `switchboard --help` for the rest.

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
- **A direct launch**, without the router, can spend the reserve of the account it goes out on.
- **Claude Code with an API key:** with `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` set, Claude
  Code may use the key in place of an account's token, so `claude` starts it as if switchboard
  weren't there, its requests going out on the key (see Launching).

## Architecture

### Packages

| Package | Owns |
|---|---|
| `cmd/switchboard` | `main`: builds the command tree from the real system (environment, clock, home, working directory, `claude`'s version, launchd, notifications, the terminal) and exits with its status. Run by the name `claude`, it hands every argument to `run` |
| `cmd/capturetool` | The capture harness's program, never a switchboard command: draws a fixture of `internal/capture` by its name, full screen until `q`, or once, as text (`--print`) or in colour (`--ansi`), at its frame's size or one given (`--size`), in a built-in theme or a `.theme` file given by its path (`--theme`), for `vhs` to screenshot; and plays a scenario of `internal/capture`'s in real time, for the README's demos (`--scenario`). Capture's tests hold each fixture to its frame's text and ANSI, cell for cell (see Files). Its import guard fails should anything `cmd/switchboard` builds import `internal/capture` |
| `internal/cli` | Cobra commands. Thin: parse flags, call the packages below, print; a data verb in its form for a person on a terminal, else its JSON (see The data verbs) |
| `internal/config` | Locating, parsing and validating the config file, and editing it: adding and removing accounts, and setting the primary and the priming day; and locating the state directory, and switchboard's bin directory |
| `internal/tokens` | The token files: reading them, checking their ownership and mode, writing them, and keeping their directory private. `tokens/tokenstest` stands in for the token files, for tests |
| `internal/accounts` | Adding accounts, replacing their tokens and removing them, for the `accounts` commands and `setup`: the config file and the token file together, and a token the user gives, typed unseen at a terminal or piped in, checked with the API before it's saved |
| `internal/dayfile` | Files a day of JSON lines, as the readings history, the request ledger and the router's events keep them: appending, compressing a day's file once its day ended two days ago, removing it once past keeping, leaving one named for a day after tomorrow until its day comes round, and reading them back, oldest first whichever day's file each is in, lines cut short and damaged files included, or a day's file on from where a read of it last ended, as a tail reads it; the days themselves, each from its first instant, and the times of a day, each as the clocks first read it, where they go forward over it, or back over it, too; and the queue and goroutine that write to them, so noting a line never waits, with the round they're kept on, hourly and before each prune, which what else is kept of a day, as the request ledger's summaries, is kept on too, at every round or just before each prune |
| `internal/ledger` | The request ledger: its lines and their writing, through `internal/dayfile`, the days' summaries, written again while lines come to be filed under their days, each knowing no less than the one it replaces, reading lines and summaries back, with no router, today and the days not summarised since lines came to be filed under them summarised as they're read; reading them as they grow, today's lines tailed, each day's summary held once read, and a session's days found by the summaries' session ids; and the price table, with the plans' prices and the version table, and the worth they give, a move's cache write's among it |
| `internal/readings` | The readings history's lines, as the router writes them and reads them back, and the request ledger summarises its days with them: a reading as a line, and the history's files in the state directory, through `internal/dayfile`, and the readings they hold of a time, in the order they were read |
| `internal/events` | The router's events kept across restarts, in a package of their own, as the readings history's lines are in `internal/readings` (settled in the plan): each as a line, filed in a file a day through `internal/dayfile`, beside the request ledger's and kept as long, and read back with no router, each event's versions merged by `run` and `id`, a day's read from its own file and the next 8 days', for the Log, LATELY, a session's page and `events` (see The router's events) |
| `internal/atomicfile` | Writing a file whole or not at all: beside where it goes, synced, then renamed into place; and where writing through a link leads, so a file that's a link is written where it leads, never replaced |
| `internal/linescan` | Reading text a line at a time, holding a line only as far as a most given: a longer one is passed over without being held, and counted, and the lines after it read, as the request ledger's, the readings history's and the router's events' files are read back and an answer's stream of events is counted |
| `internal/quota` | The provider-neutral usage model: windows, failures, per-account snapshots, and what a response says of its account |
| `internal/claude` | The Claude provider: usage-header parsing, extra usage's among it, probes, model families, response classification (a limit reached, throttling, a refused token, a request refused alone), which paths are routed, and which of them spend quota, the session header, Claude Code's environment variables, finding the installed `claude` and its version, whether the `claude` a shell runs from `PATH` is switchboard, Claude Code's local subcommands, and which models' thinking is bound to the account that produced it. `claude/claudetest` makes stand-ins of Claude Code, and of switchboard's binary, `claude` link and another build of it, for tests |
| `internal/score` | Pace, projection, a window's rate of use, the allowance, eligibility against the reserve, pressure, perishability, the 5-hour tiebreak and the best-account pick. Pure functions of a snapshot and a clock |
| `internal/prime` | The priming schedule: each account's slot from the day and the accounts, and when a prime is due. Pure functions of the day, the accounts, the window a request starts, which the `score.Policy` names, the readings and a clock |
| `internal/status` | The status document, each window's allowance and even pace, the pool's room and COMING UP among what it holds, and building it by probing every account; what the router says of a session; and their words: `status`'s text, the countdowns, clocks, titles and state words the dashboard shares, and one map from the router's reasons to the dashboard's words |
| `internal/views` | Each view's data, built by one function from what's read, the status document, the router's sessions and what its request stream tells, the ledger's, the readings history's and the events' readers, the price table and a clock: what a page of the dashboard draws, and its data verb prints, so the two never disagree (see The data verbs). It opens nothing itself; it reads through the readers it's given |
| `internal/dashboard` | Rendering the dashboard as frames (Lip Gloss): the page's chrome, its title row, status line, heading row and key line; the Overview, Accounts, History, Sessions, Runway, the Log and a session's page, each drawn from what `internal/views` builds; the usage block, which `usage` prints and the Overview's USAGE draws; the card a routing picker or a line's details is drawn in, over the page faded; the theme picker; and `?`'s help for each page |
| `internal/dashboard/watch` | The live dashboard (Bubble Tea): when to read the router or probe; the ledger's, the readings history's and the events' files, read as they grow through small interfaces it's given, which its tests and the capture harness fake; the history and the request stream; the pages and the stack `esc` goes back through, their keys and clicks, the routing pickers' orders, scrolling, easing and what moves; the theme picker; the preferences; and its desktop notifications while it probes without the router |
| `internal/theme` | Themes: the tokens, the built-ins, loading `.theme` files, working out `viz.*`, the families', versions' and days' shades among them, picking the light or dark half by the terminal's background, and the preferences file |
| `internal/capture` | The capture harness's fixtures, a fixture a signed-off frame, each a moment of the dashboard or a printout of `usage`: the specs' sample worlds; a fake router serving them, with its history and request stream; fakes of the ledger's, the readings history's and the events' readers, holding a year of days, a session's lines and the router's events, and of the price table; and the real watch model built through `watch.New` with every seam faked, or `usage`'s printout drawn through a path of its own, so it never dials the router, probes, runs a program, or reads or writes the real config, themes, state, ledger, preferences or tokens. Imported by `cmd/capturetool` alone |
| `internal/router` | The proxy and its replays, the scheduler, live account state, priming, the state file, keeping the readings history, in the lines `internal/readings` owns, each routed request's line handed to the request ledger, and the readings it summarises its days with, the router's health, the events it emits, filed through `internal/events` so they outlast a restart, and the notifications it posts, the control API and its client, and looking after itself: taking up the token files as they change, and restarting in place for a config change, an upgrade or a new time zone, or when asked |
| `internal/handover` | Handing listening sockets over across `exec`: holding them open as their listeners close, making them survive `exec`, naming them in `SWITCHBOARD_LISTENERS`, and taking up, on the other side, those it names that are listening sockets |
| `internal/launch` | `run`'s hand-over to `claude`, the real one, as `internal/claude` finds it on the `PATH` Claude Code starts with, and how a notice reads on stderr |
| `internal/setup` | `setup`'s steps, asked a line at a time at a terminal, the `claude` link in switchboard's bin directory among them, and the line that puts that directory on `PATH` |
| `internal/skill` | The Claude Code skill: its text and version, and writing and updating the installed copy |
| `internal/service` | The LaunchAgent: its plist, and driving `launchctl` |
| `internal/notify` | Posting desktop notifications, and the wording and warning threshold the router's and the dashboard's share |
| `internal/childenv` | The environment the programs switchboard runs for itself start in: no token, and for `claude --version`, the `claude`'s own directories first on `PATH` (`Beside`) |
| `internal/logs` | Logging: the handler every package logs through, the log files and their rotation, redaction, and reading logs back. `logs/logstest` captures what's logged, for tests |
| `internal/redact` | Hiding secrets: a token held, and anything shaped like a Claude token, as `[redacted]`, and telling text that holds one |
| `internal/prose` | Words shared across packages: a list run together as English does, a count as briefly as a row has room for, and text cut short |
| `internal/testguard` | Every package's `TestMain`: keeps tests off the real system, `~/.claude`, switchboard's bin directory and token files, and the directories on `PATH` included (see Test isolation in `CLAUDE.md`) |

Each view's data is built once, in `internal/views`, and drawn by `internal/dashboard` or printed
by `internal/cli`; neither works out a view's data of its own, so the CLI's JSON doesn't hang on how
a page is drawn (settled in the plan). `status` reads the router, or probes, through no code of the
dashboard's, and the watch takes the ledger's, the readings history's and the events' readers as
it's built, never opening the state directory itself, so its tests and the capture harness hand it
fakes, and its package keeps its name (settled in the plan). Of milestone 5's dashboard, the
redesign keeps the canvas it draws on, its looks and themes, the cords' motion, the request stream's
traffic, how the watch reads the router, Runway's lanes and timeline, COMING UP, the chart columns,
and the theme picker. It removes the rest: the cards, their charts and backs, the featured window,
the four-slot heading, Sessions' panels, plain list and `LOG`, Runway's day, and `usage`'s one-shot
frame and `usage -w` (see Milestones).

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
  account's last readings, with the model families each window has been seen to count, and when a
  window started again, as its reading's `restarted_at`, so a restart doesn't scatter sessions or
  need a probe, and the hashes of every configured account's tokens, with a usable token or not:
  see Accounts and tokens), `control.sock`, `tokens/`, `logs/`, `history/` and `ledger/`.
  `state.json` is versioned, the version changing only when a router couldn't read what another
  wrote: an older file, without readings, loads as having none, one whose readings lack
  `restarted_at` as windows that run a whole length before their resets, which a router from
  before, ignoring the field, takes them all for, and a pin that names its account
  alone, as pins did before they named several, as a pin to that one. It's rewritten whole (written
  beside it, synced, and renamed over it) a second after a change and on the way out, but once a
  minute at most while its only changes are those every request makes: its session's assignment used
  again, and the reading off its answer; and drops assignments unused for 7 days, with the pins of
  the sessions it forgets, and the hashes of tokens replaced 7 days before, at start and then
  hourly. At start it also drops the assignments and the sessions' own pins of accounts no longer
  configured, those accounts from the global pin, which goes with the last of them, and their
  readings, and keeps their token hashes as the primary's former tokens, which an account
  configured again takes back (see Accounts and tokens);
  those of a configured account whose token file can't be read are kept, as the file may only have
  been caught while it's rewritten, and choices pass the account over until it has a token. A
  corrupt one is set aside as `state.json.corrupt-<unix time>`, and the router starts without it.
- **Readings history:** `<state dir>/history/readings-<local date>.jsonl`, a file a day, 0600 in a
  0700 directory, for looking back at how the accounts were used. The router appends a line, as
  the file is opened to append, for each reading that changes how a window of an account reads,
  its use, its reset or its status, and for nothing else, so a request that moves nothing writes
  nothing: `{"at": "2026-09-28T13:12:00Z", "account": "work", "window": "5h", "utilization":
  0.23, "resets_at": "2026-09-28T18:10:00Z", "status": "allowed", "source": "answer"}`, `at` when
  the router took it in, in UTC, `resets_at` and `status` left out when the reading didn't give
  them, and `source` where it came from: `answer`, off the answer to a routed request; `probe`;
  or `prime`. An account appears by its id alone: never a token or a label. Fields may be added
  to a line, never renamed, and a reader passes over those it doesn't know. The router removes a
  day's file once its day ended as long ago as `[history] keep` says, 400 days unless it's set, and
  never where it says `forever` (see Config), as it starts and on each day after, as that day's
  first reading is written or its first hourly round comes, whichever is first, having first had
  the request ledger summarise the days that have ended, while their readings are there (see The
  request ledger), and leaves anything else in the directory alone. A
  file named for a day after tomorrow, as a clock once set ahead names one, stays, as the clock may
  be the one that's wrong, set back, and a clock set back must never delete real history: every
  read passes it over until its date comes round, and it's removed, as any other, once its day is
  past keeping. Writing never holds a request up: the lines queue, those of 1,024 answers or
  probes at most, dropping any past that, for a goroutine of their own to write; a write that
  fails is logged once until one succeeds, and the reading goes unwritten, as the history never
  stands in routing's way. As it starts, the router takes up the lines of its two newest days, by
  the dates their files are named for, as a change of time zone can name today's file for another
  day than the clock's, but for a day after tomorrow, into each window's recent readings, its
  baseline included (see Choosing an account): the history holds each change of a window's use, so
  they are as they were, but for when each level was last read again, which it takes as its line's
  time, and the recent rates outlast the restart. It reads them in the order they were read,
  whichever day's file each is in, holding a day or two of those it asks for at a time to order
  them, one of another time, or another window, passed over as it's read, as every read of the
  history does, and keeps what the recent readings need alone. A window quiet since
  before the older of the two has no baseline to
  take up. It passes over a line that doesn't read as a reading, as one cut short, or one over
  4 KiB, which no reading makes and which it skips without holding, one of an account no longer
  configured, and those of a window that has reset since, as `state.json` has it. A reading that
  can't be put as a line, as one whose use isn't a number, goes unwritten, logged once. A day's file
  is compressed, as `readings-<local date>.jsonl.gz`, once its day ended two days ago, as a year of
  them would otherwise run to hundreds of megabytes; the router reads either form back, as it starts
  and for `GET /history`, which the dashboard's charts draw from (see Control API), and so do the
  request ledger's readers, as they summarise a day (see The request ledger). A day's plain file
  is opened before its compressed file, and what the two hold is told from the files opened:
  compressing a day writes its compressed file before it removes the plain one, so a day another
  process compresses as they're opened is read whole, each line once. A day whose compressed file
  already ends with its plain file's lines, as when the router compressed the day between the two
  openings, or stopped between writing the one and removing the other, is read from the compressed
  file alone. A file that can't be read is passed over, and
  one damaged, as a compressed file cut short, read up to the damage, each warned of once, until it
  reads to its end again (see Logging). A line cut short at a
  file's end, as a crash or a power cut partway through a write leaves one, is ended before more
  lines follow it, whether appended or compressed after it, so the next starts a line of its own.
- **Request ledger:** `<state dir>/ledger/`, 0700: `requests-<local date>.jsonl`, a line a request,
  `day-<local date>.json`, a day's summary, and `events-<local date>.jsonl`, a line for each version
  of the router's events, each 0600 (see The request ledger, The router's events). The lines are
  appended, compressed and removed as the readings history's are, by the same rules, but kept as
  long as `[ledger] keep` says, 400 days unless it's set, or for good where it says `forever` (see
  Config). A day's summary is written whole, beside where it goes and renamed into place, written
  again while lines come to be filed under its day, and never removed; it holds the sizes of its
  day's files, and its modification time is its stamp, when the day's compressed file was last
  modified, which a round checks before it reads it, or counts the day's lines (see The request
  ledger). The router leaves anything else in the directory alone.
- **Events:** `<state dir>/ledger/events-<local date>.jsonl`, the router's events, kept across
  restarts, a file a day beside the request ledger's, and kept as long: see The router's events.
- **Preferences:** `<state dir>/prefs.json`: the dashboard's theme or pair of themes, and what its
  keys choose that it keeps (see The page), written whole by the dashboard alone, never by hand
  (see Themes).
- **Themes:** `<slug>.theme` files in `SWITCHBOARD_THEMES_DIR`, else
  `$XDG_CONFIG_HOME/switchboard/themes/`, else `~/.config/switchboard/themes/` (see Themes).
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
- **The capture harness,** in the repository: `testdata/vhs/`, where the dashboard is checked
  against its design. `reference/` keeps milestone 7's signed-off frames, 77 of them, each exported
  from Paper as a PNG, and as the text and ANSI of the generator that drew it, named by its stem:
  `usage-*` (9), `overview-*` (13), `accounts-*` (11), `history-*` (10), `routing-*` (5),
  `session-*` (6), `log-*` (9), `sessions-*` (9) and `runway-*` (5), each area's Paper page named
  in its section. A fixture of `internal/capture`'s, of the same name, draws each, and capture's
  tests hold it to the frame's `.txt` and `.ansi`, cell for cell. A fixture pins what its frame
  was drawn with, as it pins its frame's clock, Thu 1 Oct 2026 at 14:42:07, or at 16:05:41 and
  16:05:42 for `sessions-refused-160` and `sessions-moved-160`: the traces' scale at 16 requests a
  minute and a session's story's at 8, the frames' fixed scales, where the live page scales as its
  rule says (see The Overview, A session's page); `[ledger] keep` at `90d`, for the ended session's
  `kept until 29 Dec`; and the price table, injected (settled in the plan), dated 1 Oct 2026 as the
  frames were drawn, where the built-in table's date is 7 Oct 2026. The tests pass over the cells a
  frame drew otherwise than this document says:
  - the rows a frame drew with the keys as they stood before the sweep of 7 and 8 October 2026,
    which the dashboard draws as Keys has them: the key lines of every `overview-*` frame but
    `overview-phone-52`, and ROUTING's heading in `overview-pinned-160` and
    `overview-bad-hour-160`; the period's key on every `accounts-*` and `history-*` frame's heading
    row, or on a phone's the row under it, and the whole key line of each wider than a phone, as its
    period, its pin and its groups' spacing all differ; and the key lines of `session-phone-52`,
    which lacks `q quit`, `session-turn-opened-160`, which lacks `enter open`,
    `sessions-one-account-160`, which shows `p pin` with one account, and `runway-phone-52`, which
    lacks `tab views`;
  - the per-account `without it: short` counts in `history-weeks-160` and
    `history-weeks-columns-160`, drawn 6, 5 and 3 for `work`, `personal` and `side`, where the
    capacity replay gives 7, 7 and 6 (see History);
  - in `history-weeks-ten-160`, whose headline the replay bears out: its short weeks, drawn 4 as
    now, 4 with one fewer and 6 with two fewer, and 4 in `all accounts`' row, where the replay gives
    3 each; `spare`'s `5% used`, in the line naming it to drop and in its verdict, where its weeks
    average 6%; and `extra`'s `80% left at a reset`, where its weeks average 78%;
  - the mark of a pressure event in the LATELY of `accounts-detail-*`, drawn `●` before each event
    kind had one mark, where the Log's marks give `◔`.

  `testdata/vhs/README.md` lists each, with what the dashboard draws in its place, rather than their
  being patched in `reference/`, which stays the generators' output (settled in the plan). Beside
  `reference/`, `vhs` tapes screenshot fixtures `cmd/capturetool` draws: scaffolding, cleared, with
  their captures, as each milestone is signed off. `testdata/vhs/README.md` says how (see Visual
  capture harness in `CLAUDE.md`).
- **The turns check,** in the repository: `scripts/turns-check`, which the owner runs by hand
  against the real ledger, to check how turns are told from a session's lines (see A session's
  page): it prints counts alone, never anything of a request, and no test or agent runs it.

### Config

```toml
listen      = "127.0.0.1:4747"             # optional: the proxy's address
upstream    = "https://api.anthropic.com"  # optional: the API's base URL; overridden in tests
week_starts = "monday"                     # optional: the day the dashboard's weeks start on

[[account]]
id      = "work"       # permanent name: letters, digits, '-' and '_'; its token is tokens/work
label   = "Work"       # optional; defaults to the id
primary = true         # optional: the account the browser and the Claude apps use; else the first
reserve = 0.1          # optional, on any account: the share of every window the router leaves; else 0
plan    = "max20x"     # optional, on any account: its plan, pro, max5x or max20x; else unknown

[[account]]
id    = "side"
label = "Side"
plan  = "max5x"

[prime]                            # optional: start the 5-hour windows on a staggered schedule
day = "08:00-23:00"                # local time; an end before the start means past midnight

[notifications]                    # optional: which desktop notifications to post
limits  = true   # an account hits a limit, and the sessions it moved
room    = true   # an account has room again
warning = 0.9    # a window passing this share of its limit; 0 turns it off
moves   = false  # every other session move, such as after an idle hour or by pin

[history]                          # optional: the readings history
keep = "400d"                      # how long the readings history is kept: 8d or more, or "forever"

[ledger]                           # optional: the request ledger, and the router's events beside it
keep = "400d"                      # how long each request's line, and each day's file of the router's events, is kept: 8d or more, or "forever"; the days' summaries are kept for good

[usage]                            # optional: how usage and the dashboard lay out their windows
by = "window"                      # window or account: the accounts compared a window at a time, or each account's windows together

[prices.plans]                     # optional: what a plan costs, in US dollars a month, in place of the built-in price
max5x = 90

[prices.models.claude-opus-5-5]    # optional: a model's prices, in US dollars a million tokens, in place of the built-in ones
input          = 3.2
output         = 16
cache_read     = 0.16
cache_write_5m = 4
cache_write_1h = 6.4
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
  letters, digits, `-` and `_`, is unique, and isn't `auto`, in any case, which `pin auto` takes to
  mean routing. The id names the account's token file, which these rules keep safe as a file name;
  so no two ids differ only in case, as `work` and `Work` do, which macOS, ignoring case in file
  names, would give one token file. Neither the `id` nor the `label` may hold anything shaped like a
  token, as both show wherever the account does: the error never quotes it, and names an account
  whose id holds one by its place, as `account #2`.
- **`primary`** is true on one account at most.
- **`reserve`** is 0, or more than 0 and less than 1.
- **`[prime]`**: `day` is two times of day, `HH:MM`, joined by `-`, and not the same time twice.
- **`[notifications]`**: each key defaults as shown. `warning` is 0, or more than 0 and less than
  1.
- **`[history]`**: `keep` is a whole number of days, `<n>d`, `8d` or more, as the week's chart and a
  day's summary need a week and a day; or `forever`, which never removes a day's file. A count of
  more days than a duration holds, past `106751d`, some 292 years, is `forever` in effect, and read
  as it: refused, it would turn away a choice that's the user's to make, and counted as a duration,
  it would wrap round, to a keep that can remove every day at once.
- **`[ledger]`**: `keep` is as `[history]`'s is: `8d` or more, or `forever`. It keeps the router's
  events too, a file a day beside the ledger's lines, `events-<local date>.jsonl`, for as long as it
  keeps the lines (see The router's events).
- **`week_starts`** is a day of the week, by its name in English, in any case, as `monday` or
  `Sunday`; `monday` unless given. It's the day the dashboard's calendar weeks start on: the weeks
  History and Accounts count by, in History's grid, weeks, days and tokens and Accounts' this week
  and its days, and the weeks Runway steps back to. Runway's live week runs from a day before now,
  so isn't one of them (see History, Accounts and Runway). A window's own week, which resets when
  Anthropic resets it, is never moved by it.
- **`plan`,** on an account, is `pro`, `max5x` or `max20x`: the subscription it's on, which
  switchboard can't read from Anthropic. It prices the account against its worth, and sizes it for
  History's replay of a week (see The request ledger). Without it, the account's plan is unknown:
  Accounts and History show `—` for it, and price nothing against it.
- **`[usage]`**: `by` is `window`, the accounts compared a window at a time, or `account`, each
  account's windows together; `window` unless given. It's the layout `usage` prints and the
  dashboard's Overview opens with: `usage --by` overrides it for one printout, and the Overview's
  `b` for the dashboard, which remembers the choice (see `usage`, the printout, and The Overview).
- **`[prices]`** overrides the price table built into switchboard (see The request ledger).
  `[prices.plans]` gives a plan's price, by its name, `pro`, `max5x` or `max20x`, in US dollars a
  month. `[prices.models.<id>]` gives a model's, under its id as the API names it,
  `claude-opus-5-5`, in US dollars a million tokens, by the kind of token: `input`, `output`,
  `cache_read`, `cache_write_5m` and `cache_write_1h`. Each price is a number, 0 or more; one left
  out keeps the built-in table's; and a model the table doesn't know is priced only where all five
  are given, its web searches at the table's price and US-only inference unpriced. A plan's name, or
  a kind of token, the table doesn't know is an error, as unknown keys are.
- **No error quotes a token:** one that quotes a value, of `listen`, `upstream`, `prime.day`,
  `week_starts`, a `plan`, `[usage]`'s `by` or a price in `[prices]`, an unknown key's name, or the
  key a file that isn't TOML fails at, such as one without a value or given twice, shows anything in
  it shaped like a token as `[redacted]`, and an unknown key's value is never quoted.

### Proxy rules

- A request is routed only when its path is exactly `/v1/messages` or `/v1/messages/count_tokens`
  **and** its bearer token is one of the configured accounts' tokens, which Claude Code's, the
  primary's, is, or one an account had before the router took up another, or one of an account
  removed from the config, which counts as the primary's, for 7 days after (see Accounts and
  tokens): an account without a usable token has none that counts. Anything else passes through
  untouched but for switchboard's own headers (below), so batches, whose ids belong to one
  account, stay on it, and a local process that doesn't already hold a token can't borrow one.
- A token an account had before is known by its SHA-256 hash, which a request's token is hashed
  and compared with in constant time, as the current tokens are. A request carrying one is the
  account's, and goes out on the account's current token, as every routed request does. One
  carrying a token of an account removed is the primary's: the primary is its client account,
  which it falls back to when no account has room.
- Every header of switchboard's own, named `X-Switchboard-…`, is taken off a request going
  upstream, routed or passed through, by that prefix, whatever follows it, so none reaches the
  API: the pin and the directory, below, and any a newer `run` sends that this router doesn't
  know. The health check says so (see Control API), and `run` tells the directory only to a
  router that says it: one from before took the pin off alone, by its name.
- `X-Switchboard-Account: <id>`, set by `run --account` through `ANTHROPIC_CUSTOM_HEADERS`, pins
  that session. One naming an account that isn't configured, or has no token, is ignored, and the
  log warns of it once for each session and account, and notes it at debug after, until the
  account can be sent on again. The router keeps a thousand sessions of an account, and a thousand
  accounts, told of at most, forgetting them past that, and the log warns of each once more.
- `X-Switchboard-Dir: <dir>`, set by `run` through `ANTHROPIC_CUSTOM_HEADERS`, names the directory
  `run` started `claude` in, for the request ledger. Its value is percent-encoded where a header
  can't carry it as it is, and nowhere else: a control character, such as a newline, which would
  end the header's line; each byte of a character past ASCII; a space at either end, which would
  be trimmed off; and a percent sign. One that doesn't decode is passed over, and the request's
  line names no directory.
- A routed request's `Accept-Encoding` is narrowed to the encodings the router can read a copy of
  its answer in, to count it for the request stream (see Control API): gzip, deflate and identity,
  in the order the client gave them, or identity where it offered none of those. A request passed
  through keeps the encodings it offered.
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
| `GET /health` | `{ok, reason, listen, version, pid, started_at, strips_own_headers}`: the router is alive, and `ok` is its health, the judgment the status document's `router.healthy` gives, `false` while it's unhealthy, with a `reason` (see Health). `listen` is the address its proxy listens on. `strips_own_headers`, `true`, says it takes every `X-Switchboard-…` header off a request going upstream (see Proxy rules); a router from before leaves it out. `run` sends sessions to a router that answers `ok` and gives `listen`, and tells the directory a session starts in only to one that says `strips_own_headers`; `usage`, `status` and `dashboard` read the document of any router that answers at all |
| `GET /status` | The status document, as `status --json` prints it: see below |
| `GET /sessions/{id}` | For statuslines and the dashboard: `{"session": "<id>", "pin": "<id>", "assignments": [{model, family, account, dir, in_flight, pinned, yielded, pinned_at, reason, assigned_at, last_seen}], "account": {…}}`. `pin` is the session's own pin, left out when it has none: the one `pin --session` gave it, else the one `run --account` did, as its requests last carried it. `assignments` are the session's, a model each, the one used last first, each naming its model's family, such as `opus`, and its account by id; `dir`, the directory the session's last request of the model named, as the request ledger keeps it, the home as `~` and cut as a line's is (see The request ledger), kept with the assignment in `state.json`, and left out where none has named one; `in_flight`, what a request of its model in flight is doing, as the request stream tells of it: `answering` while an answer streams to one, else `asking` while one waits for its answer's first byte, and left out while none is in flight, so one read says what each running session is doing, as the request stream's first events don't say when they're all in; `yielded` set while the session's own pin has yielded, the account it names having had no room for a request of it, and it stays where it went; and `pinned_at` when it was given its own pin as it ran, left out for the one it was launched with, and while it has none. `account` at the top is the whole status of the account the last used went to, as the document gives it. 404 for a session never seen |
| `GET /sessions` | The sessions routed in the last hour, the one seen last first, each as `/sessions/{id}` gives it but for `account`. `status` lists them, and `pin --session` and `status --session` find a session from part of its id here |
| `POST /sessions/{id}/pin`, `DELETE /sessions/{id}/pin` | Set (`{"account": "work", "by": "dashboard"}`) or clear (`?by=dashboard`) one session's own pin, answering as `/sessions/{id}` does. `by`, `cli` or `dashboard`, says who did, for the `pin` or `auto` event it's told as (see The router's events); without it, the event names no one, and a router from before passes it over. 404 for a session never seen; pinning to an account nothing can go out on is a 400 |
| `POST /pin`, `DELETE /pin` | Set (`{"accounts": ["work", "side"], "move": false, "force": false, "by": "cli"}`) or clear (`?by=cli`, with `&force=true` to clear every session's own pin too) the global pin, answering with the status document. `account`, naming one account, is taken as well, as a switchboard from before pins named several sends it, and sent beside `accounts` with a pin of one account, as such a router reads a pin, one still running between an upgrade and its restart. `by`, `cli` or `dashboard`, says who did, for the `pin` or `auto` event it's told as (see The router's events); without it, the event names no one, and a router from before passes it over. Pinning no account, or any account nothing can go out on, is a 400, saying why (see Pinning), and pins nothing |
| `GET /history?window=<key>&step=<duration>` | Every account's use of a window over its current length, from the readings history and the readings since: `{"window": "7d", "step": "30m", "accounts": [{"id": "work", "start": …, "points": [{at, utilization}]}]}`, `start` when the account's window started, its reset less its length, or its `restarted_at`, and a point each step from it to now, at most 1,000, each the last reading at or before it, left out where none was; an account whose window isn't running, or wasn't read, has no points, and one whose window has reset since it was read has no `start` either. A window whose length can't be read, or a step that isn't a duration, isn't more than 0, or would take more than 1,000 steps over the window's whole length, is a 400, saying what to give, the least step included. The dashboard's charts ask for `5h` at 5-minute steps and the weeks at 30-minute steps, with each full read |
| `GET /stream` | The requests as they happen, for the dashboard's views of what's in flight: the Overview's ROUTING and SESSIONS, Accounts' Detail, Sessions, the Log and a session's page. Held open, `application/x-ndjson`, a line of JSON an event, every one `{at, kind, request, attempt, session, dir, model, account}` and what its kind adds, `request` the router's id for the request, counting up from a random start, and unique while it runs, `attempt` which time it went upstream, `session` and `model` cut to 200 bytes, `dir` the directory the request's session was started in, as its line in the ledger will give it, the home as `~` and cut as a line's is (see The request ledger), left out where the request names none, and `account` the account it goes out on, or, of `first` and `done`, the one whose answer the client got. It opens with an `inflight` for each request already in flight, adding `sent_at`, `first_at` and `chars`, and `verdict`, the last of `limited`, `throttled` or `refused` told of it on the account it went out on last, with that answer's `status`; then `sent` as one goes upstream; `first` at its answer's first byte; `progress`, with `chars`, the characters of its text, thinking and tool input so far, a quarter second after its answer streams more, then every quarter second while it does, as the API counts tokens only as an answer ends; `done` as it ends, told before the router has finished with the request, with `status`, its final `chars`, and `tokens`, the closing usage's counts, `{input, output, cache_read, cache_write}`, left out without one; `limited`, a 429 at a limit, `throttled`, a 429 sent again on the account, and `refused`, a 401 or 403, each with `status`; and `moved`, with `from`, `to` and `reason`, its `account` the `to`, which a request every account refused tells as it takes its session back, `back where it was before its request`. `limited` and `refused` are told once the router has judged the answer a limit or a refusal of the account: a 429 from before a reset made by hand, or a 401 to a token replaced since, which go out again on the same account, are neither. A request the router knows for Claude Code's quota check carries `check: true`. Requests that never go upstream, and those passed through, aren't on it. Reading the counts reads a copy of the answer's stream as it passes, decoded where it's gzip or deflate, as the request asked for one of those alone (see Proxy rules), never changing or holding the bytes passed on; an answer in another encoding goes uncounted, its bytes untouched. A reader that falls 256 events behind is dropped, and reconnects, and one that takes more than 10 seconds over a write is cut off. A stream ends as the control API closes, which a restart does after its drain, so its readers see the requests the router finished, and the dashboard reconnects to the router it becomes |
| `POST /refresh` | Probe the accounts nothing has been read of for longer than `{"max_age": "30m"}`, and those that can take no request anyway, however lately they were read, but for those whose 5-hour window has lapsed and that can take a request (see Priming), sharing the probes choices make and waiting a minute after one ended, as they do; wait 10 seconds at most for them, and answer with the status document. The dashboard asks every interval, and a minute after a window on screen resets |
| `POST /restart` | Restart now, as `service restart` asks: answer as `GET /health` does, with `in_place`, whether it means to replace itself in place rather than exit for launchd to start it again, as when it doesn't know its binary; then finish the requests in flight, within 30 seconds, those it cuts off then given 5 more to unwind, and restart as the router restarts itself (see The router looking after itself). A 409, saying why, from a router run by hand, which nothing would start again, and while its config file doesn't make a valid config, which it couldn't start again from |

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
| `best` | The id of the account to use next: of those with room in every window all models share, the one whose quota most needs using, judged by a week whose reset is known, and between near equals by the 5-hour window's reset, passing over those under pressure while another isn't, as Choosing an account says. From the router, with a global pin, it's where a new session goes, as step 4 of the order Choosing an account gives says: the best of the accounts the pin names while one has room, their reserves spent, or the first of them with room when none can be scored; else the best of every account. Left out when none qualifies, as when none has room, or none has been read yet |
| `primary` | The primary account's id |
| `prime` | The priming schedule, when the config sets a day and an account has a usable token: `{day, window, slots}`, `window` the key of the window a prime starts, such as `5h`, and `slots` giving each account with a usable token its daily prime, `{account, at, next}`, `at` a local `HH:MM`, in the order they fall. `next` is *router*: when it next primes the account, as its windows stand, left out when they can't say, as for a window read without a reset, and while the account's token is refused or it can take no request (see Priming). Left out otherwise |
| `pin` | *router* The global pin, `{accounts, account, since, move}`: `accounts` the ids of the accounts it names, in the config's order, and `account` the first of them, as a pin named its one account before pins named several; left out when there's none |
| `router` | *router* Its health: `{healthy, requests, failures, reason}`, over the last 5 minutes, `reason` left out while healthy |
| `restart` | *router* A restart it has due: `{reason, since, in_flight, by_hand}`, `reason` why, `config changed`, `upgraded` or `time zone changed`, `since` when it found it due, `in_flight` how many requests it had in flight as it gave the document, and `by_hand` set when it was run by hand, with `serve`, and restarts only when it's run again. Left out while none is due (see The router looking after itself) |
| `sessions` | *router* How many sessions have been routed in the last hour, each counted once, however many accounts its models went to; left out at 0 |
| `events` | *router* What's happened lately, newest first, 50 at most, kept in memory, so a restart starts them afresh, though the router files every one (see The router's events): `[{id, at, kind, account, accounts, session, model, from, to, reason, windows, until, since, count, limit, forced_by, reserve, status, family, move, force, by}]`, `id` rising by one an event, from 1 as the router starts, so a reader tells which are new; `kind` one of `started`, `pressure`, `cap`, `limit`, `moved`, `refused`, `primed`, `room`, `pin`, `auto`, `restart` and `health`, each with the fields The router's events gives it. The Overview's LATELY reads the newest, folding the moves a limit, a cap or a refusal counts into its line where that's among them, and the Log's Events lists each (see The Overview, The Log). Left out when there are none |
| `pool` | The accounts new sessions can go to now, as one, as `usage` and the Overview end their windows with it (see `usage`, the printout): `{accounts, pinned, windows}`. `accounts` their ids, in the config's order: those the global pin names, `pinned` then set, else every account; probing, every account, as the pin isn't known. `windows` a window each that any of them has, shortest first: `{key, label, room, used, pace}`, `room` its room summed in accounts' worth, `1.5` being an account and a half: each account's room to its cap, `1 − reserve` (to its limit without a reserve), and none in the 5-hour window of an account at its limit or its cap, nor in any window of one whose token is refused, or that hasn't been read; `used` what its bar fills to, `1 − room ÷` its accounts; and `pace` the even paces of its accounts' windows that are running, averaged. Left out with one account, whose own windows say the same |
| `coming_up` | What's coming to the accounts after the document was built, soonest first, as the Overview's and Runway's COMING UP list it: `[{at, account, kind, window, cap}]`, `kind` one of `back` (the account has room again: its limit lifting, or, with `cap` set, the window at its cap resetting; `window` set where a limit holds back only a model's requests, as a model's own week's), `runs_out` (`window` runs out before it resets, by its projection (see `usage`, the printout), the 5-hour window at its recent rate whenever there is one: at the account's cap, `cap` set, where its reserve holds it back, else at its limit), `reset` (`window` resets: each window's next, but one whose reset brings the account back, which `back` tells, and a model's own window only where its reset isn't its week's) and `prime` (*router* the account's next prime, as `prime` gives it). Left out when nothing is coming |
| `accounts` | Every configured account, in the config's order, as below |

Each account:

| Field | Is |
|---|---|
| `id`, `label` | As configured |
| `primary` | `true` on the primary; left out otherwise |
| `reserve` | Its reserve; left out at 0 |
| `token_set` | Whether its token file is present and usable |
| `fetched_at` | When its usage was last read; left out when it never was |
| `windows` | Its windows as last read, shortest first: `{key, label, utilization, resets_at, status, restarted_at, pace, allowance}`. `key` is the API's, such as `5h`, `7d` or `7d_oi`; `resets_at` is left out when unknown, and `status` (`allowed`, `allowed_warning` or `rejected`) when not given: a 5-hour window that has lapsed reads 0, with neither. `restarted_at` is *router*: when the window started again, as a reset made by hand that keeps its reset starts it, which its pace and projection measure from until its next reset; left out otherwise, when it runs a whole length before its reset. `pace` is its even pace: the share of its length that has passed since it started, its reset less its length, or its `restarted_at`, so where even use would have put its utilization now, as `usage` marks it; left out where it isn't running, as a 5-hour window that has lapsed, which `usage` says hasn't started, or where its reset isn't known. `allowance` is what can be spent of it and still last to its reset: `{share, per}`, the room left below its cap, where its reserve holds it back, else its limit, as a fraction of the window, divided by the time to its reset: `per` `hour` for a window of a day or less, as the 5-hour, and `day` for a longer one, as a week; and where its reset comes within one `per`, the room itself, `per` left out. Left out where it has no room left, isn't running, or its reset isn't known. A window whose reset has passed since it was read counts from empty, its pace from that reset and its next reset a length after it. Left out when none has been read |
| `lapsed` | The keys of its windows that have lapsed: the 5-hour window, once its reset has passed with nothing read since, which isn't running, and reads empty, until a request starts it (see Priming). Left out when none has |
| `at_reserve` | The keys of the windows at or past its reserve but short of their limit, that haven't reset since they were read; left out otherwise. The router's own choices pass the account over, for the requests those windows count, while there are any; a pin spends the reserve |
| `extra_usage` | Its extra usage, the API's overage, as the last answer or probe to give its `anthropic-ratelimit-unified-overage-*` headers left it: `{status, utilization, resets_at}`, from its `overage-status`, `overage-utilization` and `overage-reset` headers, as they've been seen: `status` as given (`allowed`, `allowed_warning` or `rejected`), `utilization` a fraction, and `resets_at` when its period resets. It's on, so the account can be served past its limit and billed for it, while its `status` isn't `rejected` (see What doesn't go through the router). The headers are read with the windows', and never count as a window. Left out where none has given them |
| `failures` | Windows a probe expected but couldn't read: `{label, window, error}`, `label` naming what should have read it, such as `Fable`. Left out when none |
| `error` | Why its usage couldn't be read, such as its token file missing, or readable by others, or, from the router, why its last probe read nothing; left out when there's nothing to say |
| `limit` | *router* A limit it reached, while it holds: `{id, windows, until}`, `id` the limit's identity, which it keeps while it holds, reached again, as the router counts its limits from 1 as it starts, and which the limit's event gives as its `limit`; `windows` the keys named as reached, left out when only the overall verdict said so |
| `refused` | *router* The upstream's refusal, while it holds: `{until, status, family}`. `status` 401 is its token refused, holding back every request; 403 a request refused alone, holding back its model's `family`. With both, the token's; with several families, the latest |
| `pressure` | *router* How fast its 5-hour window is being used, and where that's heading: `{window, rate, recent, since, runs_out, under}`. `window` is the window's key, such as `5h`; `rate` the share of it used an hour, never negative: its recent rate, as `rates` gives it, `recent` then set, and `since` when it's measured from, else its use since it started; `runs_out` when, at that rate, it reaches where the account runs out, where its reserve starts, or its limit without one or with the global pin naming the account, left out when it never does, as at a rate of 0, or has already; and `under` set when that comes before the window resets, while the account can take a request of some model, as one refused a model or held back in a model's own week still can: the account is under pressure (see Choosing an account). Of an account that can take no request, pressure isn't what passes it over, and `under` is left out. Left out when the rate can't be said, as when the window isn't running |
| `rates` | *router* How fast its windows have been used lately: `[{window, rate, since}]`, in `windows`' order, `window` a window's key, `since` when the rate is measured from, and `rate` its rise from the level of its use read last before the last 30 minutes to its latest, as a share of it an hour: over those 30 minutes when that level was read again after they began, else over the time since it was last read, a rise across a gap in its readings spread over the gap; or, with no level that far back, from its first, over the time since, 10 minutes at least. Never negative, and 0 for a window read but unused since (see Choosing an account). The projections go by them (see `usage`, the printout). Left out when no window has one |
| `sessions` | *router* How many sessions have been routed to it in the last hour; left out at 0 |

### The router's events

The router tells what it decides and notices as events, each a moment: a session started or moved;
an account under pressure, at its cap, at its limit, refused, primed, or with room again; routing
set by hand; a restart falling due; and its own health turning. The status document gives the newest
50, kept in memory (see The status document); the router also files every one, so they can be looked
back on across restarts. The Overview's LATELY reads the newest, the Log's Events lists them all,
those filed before them included (see The Overview, The Log), and `events` prints them (see The data
verbs).

- **An event** is `{id, at, kind}` and the fields its kind needs, each left out where it doesn't:
  - `started`: a session first remembered, as Choosing an account remembers one, told as its first
    answer of success comes, so never a quota check's: `session` and `model`; `account` the account
    whose answer started it; and `reason` why the session is there, as its assignment says, while
    it's still there, else why that request went there, another request having moved the session
    since.
  - `pressure`: an account came under it, but for its first reading, which is no news: `windows`
    the window under pressure, `until` when it runs out, and `since` when its rate is measured
    from. It's told once a reset of that window.
  - `cap`: a window of an account reached its reserve, so the router's own choices pass the account
    over for the requests the window counts, as its `at_reserve` comes to name the window, the
    account having had room before: `windows` the windows at its reserve; `until` when the last of
    them resets, and the account has room again; `reserve` its reserve as the cap came; `count`,
    the sessions it moved while it holds, each once, by the moves it forced; and `to`, the account
    they went to when they all went to one, left out when they went to several. It holds while
    `at_reserve` names a window of it.
  - `limit`: with `limit`, the limit's identity, which the account's `limit` gives as its `id`
    while it holds; `windows` the windows named as reached, and `until` when it lifts; and `count`
    and `to`, as a cap's are.
  - `moved`: a session's requests of a model moving to another account: `session` and `model`;
    `from`, `to` and `reason`; `limit`, the id of the limit's event, for a move that limit forced,
    holding the session's request back; and `forced_by`, the id of the event whose moves it counts
    in, a limit's, a cap's or a refusal's.
  - `refused`: with `status`; `family`, for a request refused alone; `until`, brought forward
    should the refusal lift early; and `count` and `to`, as a cap's are, of the sessions it moved
    while it holds.
  - `primed`: a prime that started its window: `windows` the window it started, and `until` its
    reset.
  - `room`: room again: an account that could take no request, at its limit or its cap, able to
    take one: `windows` the windows that held it back.
  - `pin`: routing set by hand. The global pin: `accounts` the accounts it names, in the config's
    order, and `account` the first of them, as the status document's `pin` gives them; `move` set
    where it moves the running sessions too, and `force` where it cleared every session's own pin.
    Or a session's own pin: `session`, and `account` the account it names. `by` says who set it:
    `cli`, the `pin` command, or `dashboard`, its routing pickers (see Routing by hand); left out
    where what set it didn't say, as a switchboard from before doesn't.
  - `auto`: routing given back to the router. The global pin cleared: `accounts` those it named,
    and `force` where every session's own pin went with it. Or a session's own pin cleared:
    `session`, and `account` the account it named. `by` as a pin's.
  - `restart`: one falling due: `reason` why, as the status document's `restart` gives it.
  - `health`: the router's health turning: `reason` as it turns unhealthy, none as it's well again.
- **Joining:** a limit reached again while it holds joins its event, keeping its `id`, in whatever
  order the news of it comes; another limit reached meanwhile is an event of its own. A cap reached
  again while it holds, in another window, joins its own the same way, and so does a refusal of the
  account renewed while it holds.
- **Kept in files,** so the Log and `events` look back across restarts:
  `<state dir>/ledger/events-<local date>.jsonl`, 0600 in the ledger's directory, a line an event,
  as the status document gives it, with `run`: when the router that told it started, as its
  `GET /health` gives `started_at`, so the ids of two runs are told apart. Each version of an event
  is appended, a line each, filed under the local day it's written on, with its `run` and `id`: one
  that changes after it's told, as one joined, one counting a move, or a refusal lifting early, is
  appended again, whole, as it then stands. An event changes for 8 days at most after it began, as a
  week's limit ends by its reset.
  - `internal/events` writes them through `internal/dayfile`, on a queue of 1,024 lines and a
    goroutine of its own, so filing never holds the router up: a line past the queue's end is
    dropped, logged once until the queue catches up, and a write that fails is logged once until
    one succeeds. As the router stops, what's queued is written.
  - A day's file is compressed, as `events-<local date>.jsonl.gz`, once its day ended two days
    ago, and removed once its day ended as long ago as `[ledger] keep` says, 400 days unless it's
    set, and never where it says `forever`, as the ledger's lines are, by the same rules; the
    ledger's own rounds and readers pass these files over, by their names.
  - They're read back where they lie, plain or compressed, with no router, through
    `internal/events`, given the state directory and a clock. A reader merges the lines by `run` and
    `id`, the last line winning, and shows each event under the day of its `at`, oldest first: so a
    day's events are read from its own file and the next 8 days', as `internal/events` reads them,
    rather than as `internal/dayfile` reads a day's lines, never further than the day beside it. A
    line that doesn't read as one is passed over, and a damaged file read up to the damage, as the
    ledger's lines are. The status document's list is the newest 50 since the router started; a
    reader that wants more reads the files.
  - Fields may be added to a line, never renamed, and a reader passes over those it doesn't know.
    A router from before files nothing, so the days before one that files them hold no events.
- **In the views' words:** a `reason`, as the routed line, the ledger's lines, a session's
  assignments and the events give it, reads on the dashboard as follows, every account it names
  white, as account names are wherever they're named (see The page):
  - `moved: work is at its reserve`: `work reached its cap`; in a request's `routing`, `from work,
    at its cap`;
  - `moved: work hit its limit`: `work reached its limit`; `from work, at its limit`;
  - `moved: work was refused`: `work refused the request`; `from work, refused`;
  - `moved: work has no room`: `work had no room`; `from work, with no room`;
  - `moved by pin`: `pinned to side`; `from work, pinned to side`, the pin's account the one moved
    to;
  - `rescored after 15h idle`: as it is; `from side, rescored after 15h idle`; on a session's page,
    with what the choice found, `personal had the most room`;
  - a reason ending `, personal under pressure`, naming an account the choice passed over:
    `personal came under pressure, its 5-hour to run out at 15:10`, the time its `pressure`
    event's `until`;
  - `pin yields: side has no room`: `its pin to side yields: side has no room`;
  - `new`: `started` on Events, `new session` in `routing`, `since it started` on a session's page;
  - `back where it was before its request`: `back where it was`;
  - `sticky`, `bound`, `pinned` and `pinned (global)` move nothing, so a request's `routing` is
    blank for them; a session's page says `sticky`, `kept for its thinking`, `pinned here` and
    `pinned to side`;
  - any other, as the router gives it.

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
  and with `--account`, the pin header added to any `ANTHROPIC_CUSTOM_HEADERS` already set. The
  directory header is added too, naming the directory `run` started in, which Claude Code starts in
  as well, the home directory at its start shown as `~`, as `~/Code/api`, unless `run` can't read
  it, as when it's been removed since, or the router's health doesn't say `strips_own_headers`: a
  router from before, which took the pin off alone and would send the directory on to the API,
  serves on after an upgrade until it restarts in place, at a moment with no request in flight,
  and one run by hand with `serve` until it's run again (see Proxy rules).
- **Direct:** otherwise it connects directly, on `--account`'s token, else the primary's, else the
  first account's with a usable token, without the base URL, the pin or the directory, and says why
  in a line on stderr: `switchboard: the router isn't running — connecting directly on work · Work`.
  Either way, and with `--direct`, a pin or a directory inherited from the environment, as from a
  session this one is started within, goes: what this launch pins is the only pin, and the directory
  it names the only directory. `--direct` removes the token and the base URL, so Claude Code uses
  its own login.
- **Never in the way:** switchboard never stands between the user and `claude`. When it can't take
  part at all, as when it can't read its config, can't locate its state directory, or no account has
  a usable token, `run` starts `claude` as if switchboard weren't there, environment and arguments
  untouched but for the mark every `claude` it starts carries (see Finding the real `claude`), an
  inherited pin or directory included, saying why in one line on stderr: `switchboard: couldn't
  read the config (…) — starting claude without it`. `run` fails only when `claude` can't be found
  or can't start, or on a misused command line, such as `--account` naming an account that isn't
  configured, or has no usable token.
- **An API key:** with `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` set, Claude Code may use the
  key in place of the token `run` gives it, and its requests would go unrouted, billed to the key.
  So `run`, and with it `claude`, starts Claude Code as if switchboard weren't there, environment
  and arguments untouched but for that mark, without reading the config, and says so on stderr,
  naming the variable: `switchboard: ANTHROPIC_API_KEY is set, so Claude Code uses it — starting
  claude without switchboard`. The log notes it at info, naming the variable, never the key.
  `--direct` and Claude Code's local subcommands aren't affected.
- **Claude Code's local subcommands:** `setup-token`, `update`, `upgrade`, `install`, `doctor`,
  `mcp`, `plugin`, `plugins`, `auth`, `import`, `project`, `auto-mode` and `gateway` look after
  Claude Code on this machine, or set up its login, and switchboard has no part in them. When the
  first of Claude Code's arguments names one, `run` starts `claude` as if switchboard weren't there,
  environment and arguments untouched but for that mark, an inherited pin or directory included,
  and says nothing; the log notes it at debug. `--direct` still starts it on Claude Code's own
  login. Only the first argument counts: `claude -p doctor` is a prompt. Everything else goes
  through `run` as any session does, background sessions (`claude --bg`), `agents`, `attach`,
  `respawn` and `ultrareview` among them. `internal/claude` keeps the list.
- **Finding the real `claude`:** `run` looks along `PATH`, then where its installers put it
  (`~/.local/bin`, `/opt/homebrew/bin`, `/usr/local/bin`, `~/.claude/local`), for a file that can
  be run, passing over any `claude` that's switchboard, which would start it again: one that
  leads, links followed, to switchboard's own executable, and another build of switchboard, as
  the Go build info it holds says. It replaces itself with the `claude` it finds (`exec`), so
  signals and the terminal behave as usual, and the `claude` keeps its process id. Every `claude`
  it starts, as if switchboard weren't there or not, starts with `SWITCHBOARD_STARTED` set to that
  id, the time and where the `claude` is. A switchboard started with it naming its own id, within
  30 seconds, was started again in that `claude`'s place, as by a wrapper named `claude` that
  `exec`s switchboard, which takes milliseconds, and looks past that `claude`, failing when there's
  none rather than start it again, and again. Any other, such as a `claude` started within a
  Claude Code session, has an id of its own, and looks everywhere, as does one whose id a mark
  older than that names: a later process that took the id of a `claude` long gone, the mark living
  on in something started within its session, such as a tmux server, or one running `claude` again
  in its own place much later.
  Probes claim the version of the `claude` found the same way, so the router, whose `PATH` is
  launchd's, finds it where its installers put it. Claude Code's arguments go after `--`,
  untouched and never logged; the log notes the decision: routed or direct, the router's state,
  and the account and why.
- **The service:** `service install` reads the config first, as the router will, and fails on one it
  can't read, rather than leave launchd restarting a router that can't start. It writes the
  LaunchAgent (`RunAtLoad`, `KeepAlive`, output to `launchd.log`, and an `ExitTimeOut` of 45
  seconds: the 30 the router gives requests in flight as it stops, 5 for those it cuts off to
  unwind, 3 for its last notifications to post, and 7 to save its state) to run this binary by the
  path it was run by, so a Homebrew link stays the link an upgrade moves on, with `serve`, any
  `--config` given, made absolute, and any `--log-level`. It refuses a temporary build, such as `go
  run`'s, judged by where the binary's links lead. It carries `XDG_CONFIG_HOME`, `XDG_STATE_HOME`,
  `SWITCHBOARD_CONFIG` and `CLAUDE_CONFIG_DIR`, those two made absolute, and `SWITCHBOARD_LOG_LEVEL`
  when they're set, so the service finds what the CLI does, and brings the skill up to date where
  `setup` wrote it. It runs switchboard directly: the tokens are files, so it needs nothing else of
  the user's environment. `install` warns when no account has a usable token; the router starts all
  the same, and routes once one has. Whether launchd has the service loaded is `launchctl print`'s
  to say, which exits 113 for one it hasn't: `install` boots out a loaded copy, bootstraps the new
  one into `gui/<uid>`, and waits up to 5 seconds for a router other than any running before to
  answer. launchd finishes booting a service out after `bootout` returns, and refuses to load it
  until then (`5: Input/output error`), so `install` tries bootstrapping 5 times, half a second
  apart, before it fails as the last try did. `uninstall` boots it out when loaded and removes the
  plist. `restart`, with a router answering, asks it to restart (`POST /restart`): it finishes its
  requests in flight, within 30 seconds, those it cuts off then given 5 more to unwind, then
  replaces itself in place, keeping its process, or, as it says when it can't, exits for launchd
  to start it again (see The router looking after itself). `restart` says which first, `the
  router is finishing its requests in flight, then it restarts in place`, or `…, then launchd
  starts it again`, then waits up to 50 seconds, the 45 launchd would give the router to stop and
  5 to start, for a router other than the one that took the request, as it answered it, to
  answer: one that restarted in place, keeping its process id, is told from the one before by
  when it started, and a router that restarted by itself just before it was asked is the one
  waited on to restart again, not taken for the one after. A router that refuses, as one run by
  hand, or whose config file doesn't make a valid config, which it couldn't start again from,
  fails `restart`, saying why. One that can't be asked, as one from before routers restarted when
  asked, which answers `POST /restart` 404, or one that doesn't answer it, has launchd send it
  SIGTERM (`launchctl kill`): it stops as at any signal, finishing its requests in flight, and
  launchd, keeping the service alive, starts it again, `restart` saying `…, then launchd starts it
  again`, and waiting as long. With none answering, there's nothing to finish, and `restart` is
  `launchctl kickstart -k`, waiting up to 5 seconds. Without the service loaded, `restart` is an
  error saying so. `status` reports the plist, whether launchd has it loaded, and the router's
  health. When no router answers in time, `install` and `restart` fail, the LaunchAgent in place,
  pointing to `switchboard logs router` and `launchd.log` for why. Each run of `launchctl` is cut
  off after 10 seconds, but a bootout, which waits for the router to stop, after 55: the 45 and 10
  more. Any other failure of `launchctl` is an error that quotes it.

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

**5. The dashboard — done.** The dashboard as its owner redesigned it, in Paper, over five rounds,
signed off on 2 October 2026, which milestone 7 replaces (see Dashboard). Built in stages, a pull
request each, each leaving a working dashboard, judged by eye against the final page's frames
through the capture harness, a pull request of its own after the first (see Files):

1. **The router's side:** `[history] keep`, the history's compression, `GET /history`, and the
   status document's `events`.
2. **Themes:** `internal/theme`, the built-ins, `.theme` files, the preferences file, owning the
   background, `NO_COLOR`, and the picker, `t`; testguard watching the real themes directory, as it
   watches the config.
3. **Accounts:** the title row with its tabs, the heading, the cards and their states in words, the
   burn-down charts of the 5-hour window and the week, the featured window and `w`, the layout rules
   and scrolling, the key and `?`; and the one-shot `usage` to match.
4. **Flipping and hand-patching:** the focus, `space` and `s`, the cards' backs, and selecting and
   moving a session.
5. **Runway:** the day and the week.
6. **Sessions,** in two pull requests: the request stream, `GET /stream`, reviewed as a change of
   its own, as the request ledger's idea asks of code in every answer's path, with Claude Code's
   quota check never remembered; then Sessions with requests travelling the cords, and the cards'
   backs saying what each session is doing.
7. **More charts:** burn rate and hourglass, `g`.

Building it settled what the design left open, and found the design, or the frames, wrong in
places: the Dashboard section, and the parts of the others it touched, said what was built, until
milestone 7's design took its place. Its final review, a deep one of the whole stack, changed: a
limit has an identity, which its event, the moves it forced and the account's `limit` give, and
another limit reached meanwhile is news of its own; a routed request accepts only the encodings the
router can count; the request stream ends after the requests a restart finishes; a config file
caught mid-save is looked at again before it's refused; a history file dated ahead is kept; and a
router told to stop as it restarts leaves nothing to connect to.

`status`'s text follows, designed with its owner first (see Backlog).

**6. The request ledger — done.** The redesigned dashboard's History and Accounts tabs, and a
session's page, look back on requests nothing records yet, and every day the ledger isn't recording
is a day History will never show; so the ledger comes first, ahead of the dashboard it feeds (see
The request ledger). A pull request a stage, each merged as it's ready, so the ledger records as
early as it can:

1. **A benchmark of the proxy path:** a streamed answer from an `httptest` upstream through the
   proxy, timed and its allocations counted, before anything joins the path.
2. **`internal/dayfile`:** the readings history's files a day, and the queue and goroutine that
   write them, taken out of `internal/router`, what they do unchanged, for the ledger to share.
3. **The lines:** `[ledger] keep`; the ledger's queue, goroutine and files; the line each routed
   request hands it as it ends: its shape, read with its model, and its answer's usage, blocks,
   tools and limits, read as the answer passes; and the benchmark taken again, with a body the size
   of a long session's.
4. **The summaries,** written on the hourly round for each day that ended an hour or more before,
   and kept.
5. **Reading:** `internal/ledger`'s readers, with the readings history's lines taken out of
   `internal/router` into `internal/readings`, so a reader summarises today without the router; the
   price table and worth, and the summaries keeping apart what US-only inference prices otherwise;
   `requests` and `history`; and the skill telling agents of them.

**7. The dashboard redesigned — next.** Its owner redesigned the dashboard area by area, in Paper,
from 4 to 8 October 2026, each area signed off with its frames and a spec: `usage`, the Overview,
Accounts and History, the routing pickers, a session's page and the Log, and Sessions and Runway.
The Dashboard section is that design, and The data verbs the commands it brought. It's built in 14
stages, a pull request each, each judged against the signed-off frames through the capture
harness, cell for cell once stage 4 has rebuilt it, and what moves by eye (see Files). Each
replaces rather than layers: what a stage makes dead goes in that stage, with no alias, no shim and
no code left commented out. After every stage, every gate in `CLAUDE.md` passes, and
`golangci-lint`'s `unused` finds nothing.

1. **The router records what the redesign reads,** first, and released early, as the ledger was,
   since a day not recorded can't be recorded after. The router files its events across restarts,
   a line each, a file a day beside the ledger's, under `[ledger] keep`, through `internal/events`.
   It tells new kinds: `cap`, with the sessions it moved; `pin` and `auto`, each with who set it;
   `refused` with the sessions it moved; and `room` with its windows. Its sessions and its request
   stream carry each session's `dir`. Extra usage is read off the `overage` headers into the status
   document. The days' summaries gain each account's session ids, each window's rise, and its resets
   with the use before each, and the minutes at its cap and at its limit, each merged as a summary
   written again knows no less (see The request ledger). The overage headers and `dir` join every
   answer's path, so the stage is reviewed as a change of its own, as the request ledger's idea asks
   of code in every answer's path, and the proxy's benchmark is taken again. It deletes nothing.
2. **The data layer.** `status`'s reading moves out of the dashboard's packages, so `status` builds
   on no dashboard code. The ledger's, the readings history's and the events' readers reach the
   watch as small interfaces, which its tests and the capture harness fake. The files are read as
   they grow: today's lines tailed, each day's summary held once read, and a session's days found
   by the summaries' session ids (see Live updates). Prices gain a move's cache-write cost, the
   plans' prices in the dated table, with a `plan` for each account, the version table's names, and
   overrides of the table in the config. One map takes the router's reasons to the dashboard's
   words. The config gains `week_starts` (see Config). Turns are worked out from a session's lines,
   the rule checked first against real lines by `scripts/turns-check`, which the owner runs, and
   which prints counts alone, never anything of a request (see Files). Nothing anyone sees changes.
   Its tests show the incremental reads give what whole reads give, over a day appended to,
   compressed and pruned as it's read.
3. **The data verbs** (see The data verbs): pretty on a terminal and JSON off one, with `--json`
   and `--pretty`, for `status`, `usage`, `requests` and `history`; `history`'s blocks, every one
   History and Accounts build, with the capacity replay `capacity` needs, built in `internal/views`,
   and `--windows`; `sessions [<id>]` and `events [--since] [--follow]`, new; the status document's
   allowance, even pace, pool's room and COMING UP, from the router and probing alike; the router's
   sessions telling what's in flight; "requests" meaning one thing everywhere, as what the verbs
   print changes here (see The request ledger); `internal/views`, the functions the verbs print and
   the pages will draw; and the skill telling agents of them, its version moved on. Tests hold each
   verb's JSON to its shape.
4. **The capture harness, rebuilt.** Its samples are the specs' worlds, and its fakes hold a year of
   days, a session's lines and the router's events, the prices injected and each frame's fixed
   scales pinned. `reference/` holds the 77 signed-off stems in place of milestone 5's frames. A
   test holds each fixture to its frame's `.txt` and `.ansi`, cell for cell, passing over only the
   cells `testdata/vhs/README.md` lists; from here on, each stage's fixtures must pass it.
   `usage`'s printout gets a draw path of its own, outside the watch. `CLAUDE.md`'s paragraph on
   the harness changes with it, here, as the harness does.
5. **`usage` and `dashboard`.** The usage block is one component, which the printout draws, and the
   Overview's USAGE will. `usage` takes its window flags, `--by` and `[usage] by`;
   `dashboard [interval] [--no-notify] [--probe]` runs milestone 5's views until their stages
   replace them; and `setup` ends by printing `usage`. It deletes `usage -w` and the one-shot path,
   `watch.Once` and the frame drawn once, with their tests and goldens, and changes every word of
   them: the README, `CHANGELOG.md`, the skill, the commands' help and setup's text, `demo/` and
   `testdata/vhs/README.md`. Its fixtures draw `usage-*`.
6. **The page.** The six tabs and the status line, with the router's states; the heading row, the
   key lines and the rule for a phone's; the card, the page faded behind it, the routing pickers
   and their confirmation; `?` help for each page, the stack `esc` goes back through, clicks, the
   preferences, and `R`. Milestone 5's views are re-seated under it until their stages replace
   them. It deletes the four-slot heading, and the digits, `a` and `m` on the pages, as routing by
   hand moves into the pickers. Tests hold the pickers' orders to the router's endpoints; the
   routing frames are drawn over the Overview, in stage 7.
7. **The Overview,** the default page, with the routing frames over it. It deletes milestone 5's
   Accounts view: the cards, the featured window and its use of `w`, the burn-down, burn rate and
   hourglass and `g`, the flip and the moves made from a card's back, with their fixtures and tests.
   Its fixtures draw `overview-*` and `routing-*`.
8. **History,** new: Year, Weeks, Days and Tokens; and the `viz.family.1`–`viz.family.4`,
   `viz.version.1`–`viz.version.6` and `viz.day.0`–`viz.day.4` tokens, which History is the first to
   use, added to `internal/theme` and the built-ins (see Themes). Its fixtures draw `history-*`.
9. **The Log,** new: Requests and Events, following and paused, `f`, `/`, and a line's details in
   the card. Its fixtures draw `log-*`.
10. **A session's page,** new, with the session's own routing picker, opened from the Overview's
    SESSIONS, by a click, and from the Log, by `s`. Its fixtures draw `session-*`.
11. **Accounts:** Compare and Detail, `enter` on a session in Detail opening its page. Its fixtures
    draw `accounts-*`.
12. **Sessions:** the Switchboard and the List, `enter` on a session opening its page. It deletes
    milestone 5's panels, plain list and `LOG`, and the cords coloured by account, with their
    fixtures and tests. Its fixtures draw `sessions-*`.
13. **Runway:** the 5-hour and the week, stepping back, and the pool's lane. It deletes milestone
    5's day, and `w` itself with it, with their fixtures and tests. Its fixtures draw `runway-*`.
14. **The demos and the clean-up.** The README's demos and art are recorded again through capture's
    scenarios: `usage`'s, renamed `dashboard`, and `routing`, `themes`, `repatch`, `runway-week`,
    `phone` and `help`; `flipped`, with no flip left to show, goes. `testdata/vhs/`'s milestone 5
    tapes and captures are cleared, and `viz.series.1`–`viz.series.6` go, with the built-ins'
    values of them. No code of milestone 5's dashboard is left but what the redesign kept (see
    Packages).

Stage 1 merges and is released at once, as v0.1.3. Stages 2 to 4 merge as each is ready, with
`usage -w` still there, so main can be released through them, and stage 3 is released as it merges,
as stage 1 is, so agents have the data verbs before the dashboard. Stages 5 to 14 are a stack,
merged once it has been reviewed whole: from stage 5, `usage -w` is gone, so main can't be released
partway. Then, before the release, a review of everything milestone 7 and the request ledger changed
for costs that grow with time or with what's kept, each measured against a year of heavy use: the
kind of cost a round's recount of every kept day was until it learned to look only at the days whose
lines changed. Then the release.

The release, through GoReleaser, a Homebrew tap and mint, follows milestone 3.

## Observed

What the real API, Claude Code and macOS were seen to do, dated, as Anthropic and Apple may change
any of it. Times are the Mac's, UTC+1.

- **29–30 September 2026: Claude Code's quota check.** Each `claude` sends, before any prompt, a
  request of its own on its main model: `max_tokens` 1, the content `quota`, sent without retries.
  On Claude Opus 5.5 the API answers it with a 429 carrying no `anthropic-ratelimit-unified-*`
  header, only `x-should-retry: true` and a `rate_limit_error` whose message is `Error`, sent
  directly as through the router, on every account; on Claude Haiku 4.5 it's answered 200, with
  every usage header. Claude Code ignores the failure. `--resume` sends it under a session id
  never used again. Hence step 7 of Choosing an account, and a session remembered only once
  answered, and never for a quota check (step 4).
- **30 September 2026, 15:37: a weekly limit.** An account's shared week reached 100% under
  traffic. The 429 carried the usage headers: the week `rejected`, and its reset, Monday 10:00,
  as the overall reset. The router held the account back until then, replayed the request on the
  pinned account with room before Claude Code saw any of it, moved the session there, and posted
  the notification "hit its Week limit, back at Mon 10:00 — 1 session moved to 1".
- **30 September 2026, 16:46: a banked weekly reset.** The account's owner used the free reset
  claude.ai offered, on the account held back by that limit. `usage --refresh` probed it at once,
  its limit holding back every request, and the reading lifted the limit, "room again" posting:

  | Window | Before | After |
  |---|---|---|
  | Week | 100%, `rejected`, resets Mon 10:00 | 0%, resets Mon 10:00 |
  | Fable's week | 0%, resets Mon 10:00 | 0%, resets Mon 10:00 |
  | 5-hour | 25%, resets 20:20 | 0%, resets 21:40 |

  The reset kept the week's reset time, and dropped its use: a reading lower than the one before,
  with the same reset, which the router takes as current only as it came off a request sent after
  the one before was taken in (see How it works). It cleared the 5-hour window too; the probe
  started the next, off the priming schedule, its reset at 21:40, a ten-minute mark, for a probe
  at 16:46. Hence a window read so, fallen by a tenth or more, runs from then, for its pace and
  projection (see Dashboard).
- **30 September 2026, 17:26: a second weekly limit.** Another account's shared week ran out under
  traffic. The router replayed the request on the pinned account with room, and moved the
  account's three sessions there within 16 seconds, notifying "hit its Week limit — 2 sessions
  moved to 3": the notification told of the two its 5 seconds gathered, the third moving 13
  seconds after it.
- **30 September 2026, 17:31: a second banked weekly reset.** That account's banked reset read as
  the first did: the week's use dropped to 0%, its reset time, Monday 21:00, kept; the 5-hour
  window cleared, and the refresh's probe started the next, its reset at 22:30; and "room again"
  posted 13 seconds after the refresh.
- **30 September 2026: resets on ten-minute marks.** Every 5-hour reset seen falls on one: 00:20,
  05:20, 10:10, 10:20, 20:10, 20:20, 21:40, 22:30.
- **30 September 2026, 11:15: macOS refused an upgraded binary.** After a login, `brew upgrade`
  replaced the ad-hoc-signed binary; the router exited for launchd to start the new one, and
  macOS refused it (`Launch Constraint Violation`), launchd starting it ten seconds later and
  macOS posting that switchboard can run in the background (see The router looking after itself).
  Before that login, the same exit and start after an upgrade took 40 ms. At 18:50 the next
  upgrade, the last to restart by exiting, was refused the same way, ten seconds before launchd's
  second start went through.
- **30 September 2026, 21:34: the first restart in place.** The 0.0.4 router, its restart due
  since `brew upgrade` two hours before, as requests kept coming, found a moment with none in
  flight and replaced itself with 0.0.6 under the same pid: `replacing itself` with the proxy's
  and the control socket's descriptors, then the new version's `start` 27 ms later, both listeners
  taken up, the skill brought up to date, and the requests after answered. launchd started
  nothing, and the system log held no `Launch Constraint Violation`, nor a notice that switchboard
  can run in the background.
- **1 October 2026: the first primes, after a quiet night.** With every session routed, and none
  from 22:44 until 09:34, each account was primed at its slot, 5 seconds on, and its 5-hour window
  started then, resetting five hours after the slot: primed at 03:50, 05:30 and 07:10, they read
  resets at 08:50, 10:30 and 12:10, and the first, primed again at its reset, read 13:50. A window
  starts with the first request after the last lapsed, not back to back whatever the use, so
  priming staggers the resets as designed.
- **1 October 2026, 10:00: pressure passed an account over.** A session idle for twelve hours was
  chosen for afresh; the primary had used 17% of its 5-hour window since 09:34, at 34% an hour,
  which would reach its reserve at 12:09, before its reset at 13:50, so the session went to the
  account with room instead (`rescored after 12h 14m idle, 1 under pressure`).
- **29–30 September 2026: no burst limit seen.** Over two thousand requests on one account in a
  night, from a session and up to five subagents at once, drew no 429 but the quota check's.

## Checks owed

What's built but hasn't been seen against the real thing:

- What a burst 429 looks like, and that it, or one at the edge of a limit, carries the usage
  headers: passing a 429 without them through at once rests on one observation, the quota check
  Claude Code sends on Claude Opus 5.5 (see Observed).
- Whether a reset made by hand clears a model's own week, such as Fable's: the one seen cleared
  the shared week and the 5-hour window, but Fable's week read 0% before it.
- How far below the reading before it a 429, or any answer, can read a window's use with the same
  reset: a fall of a tenth or more is taken for a reset made by hand, which rests on such dips
  being a point or so, as the one the limit's tests model, and on a reset made by hand emptying
  the window, as the two seen did (see Observed).
- That the API reports usage in the order it takes requests in: a reading off a request sent after
  another was taken in counting, whatever it reads, rests on it.
- `service install`, `service restart` and setup's service step against the real launchd:
  `restart` of a router from before routers restarted when asked relies on launchd's `KeepAlive`
  starting it again once it stops at the SIGTERM `launchctl kill` sends.
- An artifact published from a session the router has moved opening in a browser signed into the
  primary, and whether a conversation request ever refers to an uploaded file by id.
- `claude doctor` with the link in place.
- Background sessions (`claude --bg`) going through the router.
- *Milestone 5:* asking the terminal its background colour and setting it (OSC 11, putting it back
  with OSC 111) in the terminals the owner uses, and through tmux, which answers for itself or
  passes them on only as it's configured.
- *Milestone 5:* that every answer's stream ends with a usage event carrying the token counts
  `GET /stream`'s `done` gives, that none comes sooner, which `progress` would use in place of its
  estimate, and that reading a copy of the stream as it passes holds nothing up; and whether Claude
  Code asks for the stream compressed, which the copy then has to undo.
- *Milestone 7:* the overage headers `extra_usage` reads, `overage-status`, `overage-utilization`
  and `overage-reset`, and the values they carry, once an account's extra usage is on: every account
  tested read it as off (see What doesn't go through the router).

## Open-source hygiene

- No personal data in the repo or its history, ever: no account emails or labels, tokens, token
  file paths or machine names. Examples use placeholders.
- Public, under the MIT licence, and released through GoReleaser to a Homebrew tap. Built for its
  author, and general only where that's free.

## Backlog

Ideas live one to a file in `ideas/`, named for the day each was captured
(`2026-09-30--request-ledger.md`); this table is their index. An idea taken up leaves the table
for its pull request and the design proper; one dropped leaves it with a line in its file saying
why.

| Idea | Status | File |
|---|---|---|
| A reserve spent as a last resort, when no account has room outside one | next, a fast follow | [reserve-last-resort](../ideas/2026-10-03--reserve-last-resort.md) |
| The plain text the data verbs print, `status`'s and the first cuts of `requests`, `history`, `sessions` and `events`, designed to match the dashboard | next, to design with its owner | [dashboard-layout](../ideas/2026-09-30--dashboard-layout.md) |
| Releases signed with a Developer ID, so macOS stops noticing each upgrade | next, once the certificate is in hand | [developer-id-signing](../ideas/2026-10-01--developer-id-signing.md) |
| Three things the router keeps that grow without bound, each only when something rare happens, and a restart's request stream that can end before its last events | open: small fixes, any time | [router-loose-ends](../ideas/2026-10-05--router-loose-ends.md) |
| Judgments with Jev, beside or in place of fixed rules | to storm | [judgments-with-jev](../ideas/2026-09-30--judgments-with-jev.md) |
| OAuth logins in place of setup tokens, kept fresh | later | [oauth-logins](../ideas/2026-09-30--oauth-logins.md) |
| A billing month: each account's renewal day in the config, so Accounts' periods can follow its bill | later, after milestone 7 | [billing-month](../ideas/2026-10-08--billing-month.md) |
| Detail's token line: an account's tokens, and the share read from the cache, on Accounts' Detail, a key from History's Tokens for it | later, after milestone 7 | [detail-token-line](../ideas/2026-10-08--detail-token-line.md) |
| History's Days in strips per model version, beside its strips per family | later, after milestone 7 | [days-strips-per-version](../ideas/2026-10-08--days-strips-per-version.md) |
| TOON for the data verbs' long tables, beside their JSON | later, after milestone 7 | [toon-tables](../ideas/2026-10-08--toon-tables.md) |
| A faster gate before a release's tag | later, once the redesign ships | [faster-release-gate](../ideas/2026-10-08--faster-release-gate.md) |
| Spending extra usage: a way to say carry on, and be billed, once every account is at its limit | later | [spending-extra-usage](../ideas/2026-10-08--spending-extra-usage.md) |
| A notice when a session moves, through a hook | deferred | [move-notice](../ideas/2026-09-30--move-notice.md) |
| Other agents than Claude Code | deferred | [other-agents](../ideas/2026-09-30--other-agents.md) |
| Prompt-cache keep-warm | parked | [prompt-cache-keep-warm](../ideas/2026-09-30--prompt-cache-keep-warm.md) |
| An artifact proxy, one browser for every account's artifacts | open | [artifact-proxy](../ideas/2026-09-30--artifact-proxy.md) |
| Intercepting traffic that ignores `ANTHROPIC_BASE_URL` | not planned | [local-ca-interception](../ideas/2026-09-30--local-ca-interception.md) |
| An MCP server exposing switchboard to agents, such as to stream them live events | not planned | [mcp-server](../ideas/2026-09-30--mcp-server.md) |
