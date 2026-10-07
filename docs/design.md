# Switchboard — design

**Status:** milestones 0 to 3 and 5 are built, and this document describes the design as built.
Milestones 0 to 2 built the usage dashboard (one-off, and in watch mode) and `status`, which read
the router while it runs and probe when it doesn't; logging; the router, with its scheduler, pins,
state, limit and refusal handling, health and desktop notifications; and launching (`run` and the
service). Milestone 3 added token files and the `accounts` commands, `setup`, every `claude` going
through switchboard in place of `init zsh`, the primary account and its reserve, priming the 5-hour
windows, and the router looking after itself; Milestones lists it in full, with what its final
review changed. Next is milestone 4, the author's switch-over, which happens outside this repo. The
release follows milestone 3. Milestone 5 rebuilt the dashboard as its owner redesigned it, in three
views, with themes, and with the router's history, events and request stream to draw them from:
the Dashboard section describes it.

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
   by a tenth of the window or more, as a reset made by hand leaves it (see Dashboard); a smaller
   dip stands as the window's reading but is no level, so climbing back from it reads as no use.
   A window's recent rate is its rise from its baseline to its latest level, never less than 0,
   over the last 30 minutes when the baseline was read again after they began, its use holding
   there till then; else over the time since the baseline was last read: an account is read only
   on its own traffic, its primes, and a probe before a choice once 15 minutes stale, so use
   outside the router, as in the Claude apps, arrives as one rise across a gap, which came at no
   telling when within it, and 20% read two hours on reads 10% an hour, not 40% for half an hour.
   A window with a baseline and no level since has been quiet, and reads 0, so a burst holds its
   rate for the 30 minutes after it, then drops to 0. Without a baseline, as for a window started
   less than 30 minutes ago, the rise is from its first level, over the time since it was first
   read, which must be 10 minutes back at least. The weekly windows' projections go by it too (see
   Dashboard). The 5-hour window's rate is its recent rate, or, without a baseline or a level 10
   minutes back, its use since it started, once 5% of it has passed, as the dashboard's projection
   measures it. An account is under pressure when, at that rate, its 5-hour window reaches where
   the account runs out before it resets: where its reserve starts, or its limit where the
   request may spend the reserve, as a pin spends its accounts' (see Pinning).
   A choice made afresh sets the candidates under pressure aside first, then scores the rest as
   step 2 says, keeping an idle session's own account unless another is well ahead; when every
   candidate is under pressure, pressure changes nothing, so it never leaves a request without an
   account. It weighs only choices made afresh: a new session's, a request's without a session, a
   session's idle past its cache's hour, or whose account can't serve the request, or whose own
   pin yields, and a session the global pin moves; and within the global pin's accounts first, so
   it never sends a request past the pin. A session's own pin is never weighed, and a session
   staying where it is, sticky or bound, never moves for it. An account the scoring can't rate, as
   one whose week's reset isn't known, is no relief from pressure: with the global pin naming it
   and one under pressure, requests go to the one under pressure, as though every candidate were,
   rather than to the one whose quota can't be judged, as they would once none of the pin's
   accounts can be scored at all (step 4 of the order below). The
   readings outlast a restart: as it starts, the router takes them up, baselines included, from
   its readings history, which holds each change of a window's use (see Files), but for a window
   that has reset since.
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
   A 429 to a request sent before the one whose answer showed a window it rejects reset by hand
   (see Dashboard) is from before the reset: the limit holds in the windows it rejects that
   weren't, and when there are none, it's no limit, and the request goes out again on the same
   account, after the reset. One that rejects no window can't be told from a limit in a window
   that wasn't reset, and holds.
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
the best of them first, then the rest. The best next that `status` gives, and the dashboard's NEW
SESSIONS GO TO, is where a new session goes, the pin's accounts first. `--move` moves a running
session on an account the pin doesn't name to the best of those it does; one on an account it names
stays.

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
  primed (see Dashboard).
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
| `status [--session <id>] [--json] [--probe] [--refresh]` | Accounts, windows, sessions, pin, what holds an account back, reserves, pressure, the priming schedule, and router health, read as `usage` reads them, and from the router, the sessions it has routed in the last hour: a line each, with its id cut short, the account each of its models goes to, its own pin, and when it was last seen. When the `claude` a shell runs from `PATH` isn't switchboard, so the sessions it starts don't go through the router, the first line says so, pointing to `setup`. `--json` prints the status document, which is what an agent reads (see The skill). `-r`, `--refresh` has the router first read every account it may, as `usage --refresh` does. `--session` prints one line, as a statusline asks: the id of the account the router sends a session's requests to, the one its last-used model went to; or with `--json`, `/sessions/{id}`'s answer. `<id>` is the session's id, or as much of it as is unique among those sessions, as `pin --session` takes it, and it needs the router, taking neither `--probe` nor `--refresh` |
| `usage [--watch [interval]] [--no-notify] [--probe] [--refresh]` | The dashboard. Where stdout isn't a terminal, as in a pipe or Claude Code's Bash tool, it prints the status document instead, as `status --json` does, read as its flags say; `--watch` needs a terminal, and refuses without one. `-w`, `--watch` keeps it on screen, reading every interval (30m unless given, 5m at the least; a duration such as `15m`, or a number of minutes). `--no-notify` has a watch post no notifications. It reads the router while it runs; `--probe` probes instead. `-r`, `--refresh` has the router first read every account it may, as the dashboard's `r` does, and waits for it, ten seconds at most; without the router, or with `--probe`, every account is probed anyway. It reads once, so it takes no `--watch` |
| `requests [--session <id>] [--account <id>] [--since <when>] [--json]` | The request ledger's lines (see The request ledger): today's, unless `--since` reaches further back, oldest first, under the day each arrived on, a line each with the time, the session's id cut short, the model, the account, the status, the tokens in and out, and how long the request took. `--session` keeps a session's own, by its id or as much of it as is unique among the sessions read, failing, listing them, for as much as starts several; `--account` keeps an account's own; `--since` starts at a day (`2026-10-01`), a time today (`14:00`), as the clocks first read it that day, so `00:00` is the day's first instant, or a duration ago (`3h`, `2d`, a day being 24 hours), and one still to come is refused. `--json` prints each line as the ledger holds it, a JSON object a line, fields a later release added included. It reads the ledger's files, so it needs no router |
| `history [--since <when>] [--json]` | The request ledger's days: the last 30 unless `--since` says otherwise, from the day it falls on; of each with requests, a row for each account and model with its requests, tokens, sessions and worth, at today's prices, then each account's sessions, moves, limits reached and windows' highest use; today's from its lines, as far as it's gone. `--json` prints `{"prices_as_of": "2026-10-07", "days": [{"version": 1, "day": "2026-10-05", "lines": 424, "bytes": {"plain": 0, "compressed": 41208}, "accounts": […]}]}`, `prices_as_of` the day the price table was read from Anthropic's pricing page, and each day's summary as the ledger holds it, every day asked for from the first the ledger holds, so the last is today's, one of no requests without `accounts`, with each model's `worth` in US dollars added, exactly, left out where the model is unpriced, and beside it `unpriced`, the counts in its `usage` the worth leaves out, and `no_usage` where requests went upstream without usage, where there are any (see The request ledger). It reads the ledger's files, so it needs no router |
| `logs [router\|cli] [-n N] [-f] [--path]` | Print a log's last lines (`-n`, `--lines`: 50), or follow it (`-f`, `--follow`), or print where it is (`--path`): see Logging |
| `serve [--log-level <level>]` | Run the router in the foreground, normally started by the service. `--log-level` (debug, info, warn or error) overrides `SWITCHBOARD_LOG_LEVEL` |
| `pin <id>... [--move] [--force]`, `pin auto [--force]` | Set the global pin to the accounts given, replacing any before, or clear it: see Pinning. It needs the router |
| `pin <id> --session <session>`, `pin auto --session <session>` | Set or clear one running session's own pin, `<session>` being its id or as much of it as is unique. It needs the router |
| `run [--account <id>] [--direct] [-- <claude args>]` | Start Claude Code connected to the router, its conversation pinned to `--account`'s account if given. `--direct` skips the router and the token, so Claude Code uses its own login. See Launching |
| `service install [--log-level <level>]` | Install the LaunchAgent, which starts the router: see Launching |
| `service uninstall`, `service restart`, `service status` | Stop the router and remove the LaunchAgent; restart the router in place, letting it finish its requests in flight; report the plist, whether launchd has it loaded, and the router's health: see Launching |
| `version` | Print the version, as `--version` does |
| `help [command]` | List the commands, or print a command's help, as `-h` does |

The plain text `requests` and `history` print is a first cut, to be designed with their owner as
`status`'s is (see Backlog); agents read their `--json`.

A command that needs the router fails without it, saying `the router isn't running: start it
with switchboard service install (or switchboard serve)`. Any notice a command gives on stderr,
rather than failing, reads `switchboard: …`.

No command quotes a token given where an id goes: `accounts token`, `accounts remove`, `pin`, to an
account or with `--session`, `status --session` and `run --account` show an id shaped like a token
as `[redacted]`, as `accounts add` does as it refuses one, and `logs` a log's name. Nor does a
command quote one in refusing what else it's given: an argument it takes none of, a flag it doesn't
know or a value the flag can't take, `usage --watch`'s interval, `--since`'s time, and the file
`--config` names where there's none. Given where a command goes, as `switchboard <token>`, or as
`help`'s topic, as `switchboard help <token>`, a token is quoted as given, by Cobra's own refusal
of a command or a topic it doesn't know, which is left as it is.

## Dashboard

**Milestone 5: built.** This section describes the dashboard milestone 5 built, as signed off with
its owner on 2 October 2026, and as building it settled what the design left open; the dashboard
before it is described in the history of this file. This section is the design, complete without
any picture. The designs are also drawn, frame by frame, in the owner's Paper file *Switchboard
dashboard spikes*, on its page **FINAL · signed off · 2 Oct 2026**: the only page there that holds,
its frames kept in `testdata/vhs/reference/` too (see Files). The pages named
`superseded · round 1` to `round 5` are the iterations that led to it, kept as history; where they
differ from the final page, they're wrong, and where the final page differs from this section,
this section holds: the page's own read-me lists the differences known as it was signed off, and
`testdata/vhs/README.md` lists them in full, those building it found among them. The frames use
placeholder accounts (`work`, `personal`, `side`, `client`, `spare`, `lab`, `team`, `extra`) and
sample numbers; the mocks are 160 columns wide unless named otherwise, and 52 for a phone.

The dashboard is three views of the same document, a heading that sums them up, and the keys to act
on them. It runs in watch mode (`usage -w`), filling the terminal and redrawing as things change, or
once (`usage`), printing the Accounts view and exiting.

### Views

- **Accounts**, the default: a card per account, showing how much of it is left and where that's
  heading. It answers *how much do I have left?*
- **Sessions:** where each running session's requests go, a cord from each to its account, with
  requests travelling them as they happen. It answers *where is everything going, and why?*
- **Runway:** when each account has room over the next day, or with `w`, the week. It answers
  *when will I have room?*

`tab` and `shift-tab` move between them, in that order and round, as the digits are the pin's. The
title row names them as tabs, the one shown picked out in `bg.selection`, with `tab ⇥` beside them
while there's another view to move to: `SWITCHBOARD  Accounts  Sessions  Runway  tab ⇥`, and the
date and the time at its right. The view shown is remembered in the preferences file (see Themes),
so a watch opens where it was left. The one-shot `usage` prints the Accounts view alone, without
the tabs.

### The heading

Under the title row, every view shows the same heading: four slots, each a label and three lines
under it, at fixed places, so the eye learns where to look:

- **ROUTER:** its health, how it routes, and what it has due:
  - from a router that answers: `● healthy`, in `state.positive`, or `● unhealthy`, in
    `state.destructive`; then, on the second line, its sessions and its routing, `5 sessions ·
    auto`, `pinned to work` or `pinned to work and side`; then, on the last, a restart it has due,
    in `accent.attention`, `restart due (config changed)`, which always shows there, else an
    unhealthy router's reason, in `state.destructive`. With both, the reason follows `● unhealthy`,
    wrapping onto the lines under it, and the sessions and routing follow where there's room.
  - while the router's last document stays on screen, the router not answering: `○ no router since
    14:40`, dim, and the rest as that document had it, an unhealthy router's reason included.
  - probing because the router isn't there: `○ probing`, dim, then `router not running`; or, where
    something answered its socket but not as a router does, `router unhealthy: no answer within
    500ms`, in `state.destructive`, as the document's `fallback` gives it.
  - probing as asked, with `--probe`: `○ probing, as asked`, saying nothing of the router.
- **NEW SESSIONS GO TO:** the account the next session goes to, `▲ 3 side`, in `accent.mode`, and
  under it how many could take one, those the router chooses a new session among, as the status
  document's `best` is chosen: `1 of 3 open`. With none, `no account has room right now`, in
  `state.destructive`, or while nothing has been read of any account, `nothing read yet`, dim.
- **ROOM LEFT, IN ACCOUNTS:** a row each for the 5-hour window and the week, a short bar per
  account, in its configured order and numbered under them, filled to the share it has left, and the
  rooms summed as accounts' worth: `5h ▆▆▆░░ … 1.3 of 3`. An account that can take no request counts
  0 in the 5-hour row. The bars narrow as accounts are added, from 8 cells to 3 at the least, so
  the slot stays put.
- **COMING UP:** the next three things to happen, soonest first, each with its time, its account
  and what happens, and on the right how long until it: `15:54  personal back from its limit  in 1h
  12m`, `16:05  work runs out at its pace  in 1h 23m`, `17:10  work's session resets  in 2h 28m`,
  and primes, `16:20  spare is primed`. A run-out goes by the floor the account runs out at: its
  reserve, where the reserve would hold it back, `15:45  work reaches its reserve`, else its limit,
  `16:05  work runs out at its pace`; it's in `accent.attention`. This takes the place of the line
  of next reset and next prime; the priming schedule itself stays `status`'s.

The pin isn't in the heading but on the cards, as their `● pinned` badge, and ROUTER's routing
word. With **one account**, there's nothing to choose between, so the heading is a single line:
`ROUTER ● healthy · 3 sessions · priming 08:00–22:00`, and at its right `ROOM LEFT 5h 42% week
66%`; NEW SESSIONS GO TO, the `▲ next` badge and the pin's keys go. The four slots sit in a row from
150 columns; from 100 to 150, in two rows of two, ROUTER beside NEW SESSIONS GO TO and ROOM LEFT
beside COMING UP. COMING UP starts two cells clear of ROOM LEFT's sums at the least, however many
accounts' bars they follow. On a **phone**, under 100 columns, the tabs take a line of their own
and the heading two: `● healthy · 5 sessions · auto`, and `new → ▲ side` with `room 5h 1.3 wk
1.4` at its right.

### Accounts: the card

A card, top to bottom:

- **Its top edge:** its number and id (or label), and at the right its badges: `◆ primary`, in
  `text.tertiary`; `● pinned`, in `accent.primary`, while the global pin names it; `▲ next`, in
  `accent.mode`, on the account new sessions go to, whose border is `accent.mode` too; and
  `sessions`, in `accent.key`, while the card is flipped. Cards are wide enough for every badge, so
  pinning never reflows them; a flipped card too narrow for them all, `sessions` among them, drops
  `◆ primary` first, then `▲ next`, as `● pinned` and `sessions` always show.
- **Its state, in words**, the coloured dot or square leading, wrapping onto three rows where
  they're long: what holds it back or what it's doing, and what that means for new sessions. In
  order of precedence:
  - `✕ no token · switchboard accounts token work`, in `state.destructive`, without a usable token.
  - `■ limit reached · back 15:54, in 1h 12m`, in `state.destructive`, while a limit holds back
    every request: the router's limit and every window read spent hold it back together, and it's
    back as the last of them lifts. `■ refused (401) · until 21:40` while its token is refused. One
    holding back some models alone, as a limit in a model's own week or a 403 does, is in
    `accent.attention`, and says what still goes: `■ Fable wk limit · back Mon 21:00 · other models
    still come here`, `■ refused (403, opus) · until 21:40 · other models still come here`; but a
    window every model shares at the reserve that holds the account back outranks it, and is said
    as the reserve's line below says. A limit or a refusal that holds is said even where the
    account's last read failed, so a probe timing out under a limit leaves the card at its limit.
  - `! can't read it · <why>`, in `state.destructive`, when its usage can't be read, over the
    numbers last read of it; `… not read yet`, dim, before anything has been.
  - `● at its reserve (90%)`, or with the global pin naming it, `● spending its reserve (pinned)`,
    in `accent.attention`; a model's own window at its reserve, as Fable's week, says what still
    goes: `● Fable wk at its reserve (90%) · other models still come here`.
  - `● under pressure · new sessions go elsewhere`, in `accent.attention`, while another account
    takes them; where new sessions still come here, as with one account or every account under
    pressure, `● under pressure · runs out ~16:05 at this pace`; where its reserve would hold it
    back, `● under pressure · at its reserve ~18:21`.
  - `○ idle · window starts at its prime, 16:20`, while its 5-hour window has lapsed; without
    priming, or probing, which can't say when it's next primed, `○ idle · window starts with its
    next request`.
  - `● open · new sessions come here` on the account new sessions go to; `● open` on the rest;
    `● open · its week nears its reserve` once a week is within 10 points of it.
- **The featured window** (see below): its use in big digits, three cells tall, drawn in `▀▄█`, in
  the colour of the account's state, or for a limited 5-hour window the time until it lifts, `1:12`,
  as `h:mm`, and `mm:ss` in its last ten minutes, in `state.destructive`; beside them the window's
  name, `SESSION 5-hour window` or `WEEK 7-day window`, where it's heading, `→ runs out ~16:05 at
  its last-30-min rate`, or at its reserve, `→ reaches its reserve ~15:45`, or `→ 54% by its
  reset`, and when it resets, `resets 17:10 · in 2h 28m`, or, read without a reset, `reset time
  unknown`. At a limit, where it's heading reads `limit reached at 14:12`, dated by the router's
  event of the limit where one names the window held, else `limit reached`; and when it resets,
  `its window resets 15:54`. Lapsed, they read `not started`, and `next prime 16:20 · in 1h 38m`,
  or `starts with its next request`. Too long for the card, the words keep their time whole,
  leaving off the rate's words first, then cutting the verb to `→ out ~`, then the date, its
  weekday left off within a day of now; and a countdown to a time just passed reads `now`. Under
  them its chart (see Charts), and under that its axis: the 5-hour window's start, `now` and its
  reset; or the week's days, a tick in the column each midnight falls in, or the instant the clocks
  went forward over it, `╵Tue ╵Wed ╵Thu`, a lone `╵` where the day's name doesn't fit, today's
  picked out.
- **The other windows**, a line each, in the status document's order, shortest first: the name, a
  bar, the use and where it's heading: `Week ██████┃███▋╎░ 34% → 87%`, where it's heading turning
  `accent.attention` within 10 points of the floor it runs out at; `→ out Fri` in `accent.attention`
  where it runs out before its reset, `back 15:54` in `state.destructive` for a limit, `not started`
  for a lapsed window; and a window a probe couldn't read has its row say so, as the document's
  `failures` give it: `<label>  can't read · <why>`. A bar fills along `viz.ramp`, from its first
  stop at its first cell to its last at its last, so its colour says how far along it is; the share
  it's heading for by its reset follows in the ramp's colours blended halfway into the canvas; `┃`
  in `viz.pace` marks where even use across the window would be now; `╎` in `viz.reserve` where
  the reserve starts. A window reset by hand measures its pace and projection from its start
  again, as Pace and projection says.
- **Its bottom edge:** a dot per session on it, `●` lit in `state.positive` while busy, `○` while
  idle, and the count at its right, `3 sessions`, or `no sessions`, its sessions counted as the
  router counts them, those active in the last hour. A session is busy while a request of it is in
  flight, or it was seen in the last minute.

The 5-hour window and the shared week always show. A model's own window, as Fable's week, that no
account has used this period, nor is heading to use, is hidden from every card, and the line over
the footer says so: `Fable wk hidden: unused on every account`. It comes back as soon as any account
uses it. Every card shows the same windows in the same order, so their rows line up across the grid.

**The featured window.** A card features one window, with the big digits, the chart and the axis;
the rest are the one-line bars. `w` cycles which, for every card at once, so the cards stay
comparable: `auto`, then the 5-hour window, then the week, then any other window in use, such as
Fable's week, and round to `auto`. The footer says which, `w window: auto`, and the choice is
remembered in the preferences file. A window remembered that's no longer in use reads as `auto`,
in the footer and `?` alike, and `w` moves on from `auto`. On `auto`, each card features what will
stop its account first:

1. The window holding it back now, under a limit or at its reserve. A limit that names no window,
   its overall verdict alone, holds every window: the card features the 5-hour window as limited,
   the time until it lifts in its digits and its level on the floor.
2. Else the window that runs out soonest, at the projection Charts gives, where one runs out
   before it resets.
3. Else its most-used window, by share; a lapsed 5-hour window counts as unused.

So, as drawn, `work` features its 5-hour window, which runs out at 16:05, `personal` its 5-hour
window, at its limit, and `side`, `client` and `lab` their weeks. The readout always names the
window it shows, so a grid of mixed windows reads plainly: the mix is the point, as it shows what's
really binding each account.

**Charts.** The featured window's chart is a burn-down: the room left in the window, falling
toward the floor as it's used.

- **The past** is a level drawn in eighth blocks (`▁▂▃▄▅▆▇█`), a column a cell, filled to the room
  left at that time, in the account's state colour blended halfway into the canvas. It's drawn from
  the readings history (see Files), each column the room the last reading before its time gave.
  Any room left shows as its lowest eighth at the least, and none as a line along the floor, `▁` in
  `state.destructive`, so a limit shows as the level reaching the floor and that line along it,
  until the reset.
- **Now** is a thin line, `│` in `border`, at the column for the time.
- **Where it's heading** is a dotted line, in braille, from the level now: to `✕`, in
  `state.destructive`, on the floor where it runs out, where it runs out before its reset; else to
  the room it will have left at its reset. It goes at the same projection as the words do: the pace
  its use since it started sets, or from the router, its recent rate, where that has it run out
  sooner or end more used; and for the 5-hour window, the recent rate whenever there is one (see
  Choosing an account). The floor it runs out at is its reserve, drawn as a faint dotted line in
  `viz.reserve`, where the reserve would hold it back, else its limit.
- **A lapsed 5-hour window** shows a full level, dim, and `full · window starts at its prime, 16:20`
  across it.
- **The week** draws the same way over its seven days, a column covering about four hours at 160
  columns, so it steps down through each working day and runs flat overnight: the owner's own
  rhythm, and how many working days are left before the `✕`. On a fresh install it fills in as the
  history grows.

Braille was tried for the level and rejected: a large area of braille reads as a grid of dots, not
a level. `g` cycles the chart style, for every card, remembered in the preferences file: burn-down,
then burn rate and hourglass, which were sketched, not drawn on the final page:

- **Burn rate** is the window's use per 10 minutes, as bars in eighth blocks: a bar each 10
  minutes, or each column where a column covers more, as over the week, measured per 10 minutes
  all the same, in the state's colour blended halfway into the canvas. A dotted line in `viz.pace`
  runs at the even pace from now to the floor it runs out at, the fastest it could burn and still
  last to its reset, and the bars above it are in `accent.attention`. The bars and the line share
  one scale, each card its own. At a limit, a line runs along the floor until the limit lifts, as a
  burn-down's does, and there's no dotted line.
- **Hourglass** is the window as an hourglass in quadrant blocks, as tall as the chart, standing
  over `now`: the sand above the room left, the pile below the use. The stream between them is 2,
  4 or 6 grains wide as the recent rate stands against the rate that would last to the reset, and
  falls a grain every 125 milliseconds while the account is busy, drawn at that pace rather than 30
  frames a second, and is still while it isn't. At its reset it's turned over, full and still. It
  draws no reserve, and in the `terminal` theme draws as it does without colour.

A fourth, a heartbeat of the account's requests from the request stream (see Live updates), wasn't
built with them.

**The key.** Where there's room, a line over the footer explains the glyphs, the chart's in the
style the cards draw: for a burn-down, `▆ room left`, the dotted `heading`, `✕ runs out`, the
dotted `reserve`; then the bars' `used`, `heading`, `┃ even pace` and `╎ reserve`, `● session, lit
while busy`. It shows only where all of it fits: where it doesn't, as for a style whose glyphs need
more width than there is, or there's no row for it, it's behind `?`, and the line says so: `? for
the key`.

### Pace and projection

- **A window's projection** goes at the pace its use since it started sets, or, from the router,
  at its recent rate, its rise over the last 30 minutes (see Choosing an account), where that has it
  run out sooner, or end more used: so a week at 99%, on pace since it started to run out at 17:42
  but used at 7% an hour lately, reads as running out at 17:25, never later than its use lately
  says, and eases back as use slows. The 5-hour window's, whose rate the router judges pressure by,
  goes at that recent rate whenever there is one, so the screen shows where the router takes it to
  be heading. The words, the charts' dotted lines and the bars' projected share all go by it, and
  `status` projects as the dashboard does. The dashboard projects from when the document was built,
  its `generated_at`, so a projection stays put while a document stays on screen, as when the
  router has stopped answering; its countdowns count from now.
- **It says the span it measures over** when it goes at the recent rate: on a card, `→ runs out
  ~16:05 at its last-30-min rate`; `last-18-min` for a window without a level from before the half
  hour, or `last-2h` across a gap in its readings; the event, `work came under pressure: its session
  runs out ~16:05 at its last-30-min rate`, or where its reserve would hold it back, `…: its session
  reaches its reserve ~15:45 at its last-30-min rate`. `status`, whose text keeps its form until
  it's designed afresh, says `runs out ~Mon 17:25 at its rate over the last 30 min`, `over the last
  18 min` or `over the last 2h` as the span is, and its pressure line adds the rate it goes by and
  the reset it runs out before: `under pressure: runs out ~18:21 at Session's rate over the last 30
  min, before its reset at 20:10`, or where its reserve would hold it back, `under pressure: at its
  reserve ~18:21 at Session's rate over the last 30 min, before its reset at 20:10`.
- **A window reset by hand** before its reset time, as claude.ai's banked reset does, dropping its
  use but keeping its reset (see Observed), has effectively started again: the router reads it with
  the same reset, taken as current (see How it works), fallen by a tenth of the window or more, and
  notes when as the window's start. A smaller dip, as a 429 reading a point below the use read just
  before, is noise: the reading stands, as the upstream's latest word, and the window runs on. Its
  pace marker, its projection and its chart measure from its start again, rather than from a whole
  length before its reset, until its next reset, a later reset being a new window; otherwise a week
  reset at the end of its third day would show the marker about three-sevenths of the way along,
  and be on pace for 0%. Once the router has read a window reset by hand, the answer to a request
  sent before the one whose answer showed it is from before the reset, and is passed over, where
  use only rising within a window would have it put back the use the reset took away, and so is the
  limit a 429 to it reaches in the window (see Choosing an account, step 6); the router keeps which
  request that was in memory alone, as the state file's readings count as read before any.

### Accounts: the layout, for any number of accounts

The layout follows rules, so every number of accounts and every terminal size gets one, and none
falls back to a line per account until the terminal is too short for any card (rule 11). The final
page draws 1, 3, 4, 6 and 8 accounts, 8 scrolled, and a phone; a panel there lists these rules.

1. **Columns:** as many 50-column cards as fit across, never more than there are accounts, 4 columns
   apart, or 2 where that fits another card; the cards share the spare width. So 160 columns hold 3,
   as the frames lay them out, 210 hold 4, and a phone 1.
2. **One account** gets one wide card, about 104 columns, with its chart 6 rows tall, and COMING UP
   and RECENT in a column beside it, once the terminal is 150 columns wide, RECENT giving each
   event's subject a line, and the rest of it the lines under.
3. **One size:** rows of cards wrap, every card in the grid is the same size and density, and they
   show the same windows, so their rows line up.
4. **Recent events** (see Live updates) take the first empty cell of the last row, as with 4, 5 or
   8 accounts at 160 columns; with no empty cell, as with 3 or 6, a strip under the grid, its label
   `RECENT` and up to four lines.
5. **Unused windows hide**, as the card says.
6. **Cards get the space first.** The richest card that fits wins, of, in order:
   - **full:** the state line, the big readout, a 4-row chart and its axis, with a blank row
     between each part;
   - **mid:** the state line, a one-line header (`Session 58% → out ~16:05 resets 17:10`), the chart
     at 6, 5, 4 or 3 rows, and its axis;
   - **compact:** the state line, the header and a 3- or 2-row chart, no axis.

   The windows' bars and the edges come with every one. A card fits when every row of cards does,
   with one line of recent events under them where they take a strip.
7. **Then the extras:** rows left over grow the recent strip to four lines, then show the key line,
   where all of it fits; else the key is behind `?`, and the line over the footer says `? for the
   key`.
8. **Scroll last**, in watch mode, only once 2-row charts don't fit. The title row, the heading, a
   recent strip under the cards and the footer stay put, and the rows of cards scroll between them,
   a recent cell with its row: `j`/`k`, PgUp and PgDn, and the wheel, and moving the focus to a card
   out of view scrolls to it. A scrollbar runs down the right edge, `┃` the part shown on a `│`
   track, and the line over the footer counts the accounts out of view, above and below: `▲ 1 more
   account above · ▼ 2 more accounts below · j/k or wheel to scroll`.
9. **The one-shot `usage`** never scrolls: it has no height to fit, so it prints every card at the
   full density, as wide as the terminal allows, and the terminal's scrollback holds what doesn't
   fit the screen. It reads `GET /history` once, where the router answers, and `GET /sessions`, for
   the cards' dots, and keeps the preferences file's theme, featured window and chart style.
   Without history, as probing, a chart draws the room the window has now as a flat level from its
   start, dim, marked `no history yet`.
10. **Narrow:** under 50 columns of room for a card, a card takes the full width; the phone layout
    stacks compact cards with 2-row charts, which fits three accounts in 36 rows.
11. **Too short:** a terminal too short for one row of compact cards with 2-row charts shows the
    title row, a line per account, its place, its name and its state in words, as many as fit, and
    the footer; under three rows, the title row alone.

The rules count rows: the title row, a blank, the heading's four rows and a blank above the cards,
7 rows; a blank and the footer below them, 2; the key line and a blank, 2 more; a recent strip, a
blank and its lines; and a blank between rows of cards. A **full** card is 12 + c + w rows, c its
chart's rows and w its windows' bars, as the edges, the state line, the readout's three rows and the
blanks between them take 12; a **mid** card 6 + c + w; a **compact** card 4 + c + w. With the 5-hour
window featured, and a week and Fable's week as bars, w is 2, so a full card is 18 rows. With **one
account** under 150 columns, its card takes the width, and COMING UP and RECENT are strips under
it.

What the rules choose at 160 columns:

| Accounts | 28 rows | 40 rows | 50 rows |
|---|---|---|---|
| 2 or 3 | mid, 6-row chart | full | full |
| 4 or 5 | compact, 3-row | mid, 6-row | full |
| 6 | compact, 3-row | mid, 6-row | full |
| 8 | compact, 2-row, scrolls | compact, 3-row | mid, 6-row |

### Accounts: flipping a card

A card turns over to show the sessions on its account, rather than squeezing them onto its front.

- **Focus:** no card has it until an arrow, `space` or `s` gives it to the first card wholly in
  view; then it stays. The arrow keys move it between cards, `←` `→` along a row of them, stopping
  at its ends, `↑` `↓` between rows, over a flipped card's sessions first, `↓` to a shorter last row
  landing on its last card; the focused card's border turns heavy (`┏━┓┃┗━┛`) and `accent.key`.
  `space` flips the focused card; `s` flips every card, or back. A flipped card keeps its size and
  its place, and says `sessions` on its top edge.
- **Its back:** `3 sessions · 2 busy`, and a blank; then a row per session and model active on
  the account in the last hour, as the router counts them, a session whose models go to two
  accounts showing on both, its other half noted: a dot, lit while busy; the session's id, cut to
  4; its model; and what it's doing. While a request of it is out, that's `streaming ↓ ~1.2k`, in
  `accent.mode`, an estimate of its tokens so far at four characters a token, or `waiting 38s`,
  sent with nothing back yet, in `accent.attention`; for the 2 seconds an answer is held as it
  ends, its exact count, `↓ 1.3k`; otherwise `idle 38s` under a minute, and after it, to the
  nearest minute, as Sessions' calls say when they were last seen, `idle 9m`. Under each row, where
  there's room for every row's, a note, the first of these that holds: its own pin sending it
  elsewhere, `╰ goes to side from its next request`, or, where that pin has yielded, the account it
  names having had no room, and the session stays where it went, as the router says, `╰ its pin to
  side yielded here`; its other models, `╰ its opus is on side`; when it was given its pin as it
  ran, `╰ pinned here at 14:39`; `╰ moved from personal at 14:12`; `╰ here since 13:20`. A session
  the request stream told of moving here shows here, on the back and among the card's dots, as in
  Sessions' plain list, until a listing of the sessions does, noting when it moved, `╰ here since
  14:43`. Times show as the cards date them, and accounts by their labels. A back too crowded for
  every row drops the blank under its count first, then shows as many rows as fit and `+2 more`,
  the selected row among them. Then, where there's room, `LATELY`, the account's own events, told
  from its side: a limit takes two lines, `■ reached its session limit`, then `▸ 3 sessions moved
  to side`, and the card they went to says `▸ 3 sessions arrived from personal`. Last, a line of
  the keys: `↑↓ select · 1-3 move it · space flip`. An account with no sessions says so, and why
  where it can: `3 moved to side at 14:12, when personal reached its limit`.
- **Hand-patching:** on the focused flipped card, `↑`/`↓` select a session, its row in
  `bg.selection`, `▸` before it, and carry the focus on to the card above or below past its first
  or last. A digit then pins that session to the account in that place, every model of it, as
  `pin <id> --session <session>` does, so its next request goes there, its other rows following;
  `a` clears its own pin, as `pin auto --session` does; `esc` ends the selection.
  While a session is selected, the footer gives that mode's keys: `↑↓ select · 1-3 move 5b19 to
  that account · space flip back · esc done`, and at its right `5b19 selected on work`.
- **Without the request stream**, as from a router from before it, a session is busy when it was
  seen in the last minute, and its row says `seen now`, which only a back without the stream says,
  or `idle 9m`, rather than what it's doing.

### Sessions

Where each session's requests go, and why, as a switchboard draws it: **calls** on the left, the
sessions, and **lines** on the right, the accounts, with a cord from each call to its line.

- **Calls:** a row per session and model, grouped by the account it's on, in the accounts' order,
  a blank row between groups: its id, cut to 4, bold while busy; its model; when it was last seen,
  the later of the router's `last_seen` and what the stream told of it, which outlasts its answer.
  A session split across accounts has a row in each group. Rows keep their places from look to look,
  a new one joining its group's foot.
- **Lines:** a panel per account, its name and state on its top edge, `┌─ 1 · WORK ─ ● under
  pressure ──── ◆ primary ─┐`, a row per window under it, its bar, use and where it's heading,
  and its resets on its bottom edge, `└─ resets session 17:10 · weeks Mon 21:00 ─┘`; an account
  not read yet has its panel say `not read yet`, dim, as its card does. A jack, `◉`, at the panel's
  left on each row a cord ends at, `○` where none does.
- **Cords** run from the call, `●`, along its row, and down to a jack of its account's panel, in
  `━ ┃ ┓ ┗`; a cord that starts higher bends further right, so none cross, the bends starting 12
  cells left of the lines and stepping 5 apart, closer when crowded. They only ever run right,
  then down: a panel has a jack for each of its window rows, and grows a row for each call past
  those; and where a group of calls would start below its panel's first jack, the panels move down
  to meet it. Every cord to an account is that account's colour, from `viz.series`, in its
  configured order; an idle session's cord is dimmed. Sessions a limit moved off an account leave
  dashed stubs, `╌`, hanging at its jacks while the limit holds.
- **Requests travel the cords** (see Live updates): a bright pulse runs from the call to the jack
  in 0.54 seconds as a request goes out, the call row saying `↑ ask`; while the answer streams
  back, the cord shimmers toward the call, every fourth cell lit, stepping on the clock's
  80-millisecond marks, every cord in step, and the row counts the tokens, estimated, `↓ ~1.2k`; a
  pulse runs back as the answer ends. A limit or a refusal shows as `✕` on the jack, in
  `state.destructive`, and a red pulse bouncing back, the row saying `✕ 429` or `✕ 403`;
  throttling, a 429 the router sends again on the account, as a dim `… 429` and no pulse. An
  answer's end, and a `✕`, hold for 2 seconds, a `✕` even as its 429 is passed on to the client.
  Claude Code's quota check isn't drawn.
- **A move re-patches:** the session's row leaves its old group, a faint placeholder, `5b19 ↪
  moved to side`, keeping its row until a listing of the sessions shows the session on the account
  it went to, through the stream joined again or another view shown meanwhile, so the other cords
  stay put; it joins its new account's group, `↪ new`, its cord running to a free jack there; its
  old cord hangs loose from its old jack, `╌`, and fades over 3 seconds, or where a limit moved it,
  stays as a stub while the limit holds. A move at a limit or a refusal re-patches once the red
  pulse has bounced back, the retried request's pulse setting out along the new cord as it does.
  The log line says why: `14:43 ▸ 5b19 moved work → side: work reached its limit, so its request
  was retried on side`. A router restarted, or another, has the watch forget what the last one's
  stream told.
- **Under the calls,** `LOG`, the recent events, every move among them, and why, as the re-patch's
  line above, those a limit forced too, which RECENT and the cards' LATELY fold into the limit's
  line where that's among theirs.
- **Panels shrink** with the accounts: three window rows and the edges, so five accounts fit in 40
  rows; beyond what fits, the view scrolls as Accounts does, the line over the footer saying what's
  out of view only while something is. On a terminal too short for them, the line over the footer,
  and the labels over the calls and lines, are left out.
- **With one account,** every cord would end at the same jack, so the view is a plain list of the
  sessions instead: as the back of a card lists them, with the account's panel above.
- **Narrower terminals:** the lines keep their width, about 64 columns, and the cords shorten; under
  110 columns, or where the bends can't fit 1 apart beside panels with bars, the panels' bars give
  way to the windows' percentages; under 90, or where the bends can't fit 1 apart even then, the
  view is the plain list, a panel per account each with its sessions under it. The plain list's
  panels are boxed, without jacks.

### Runway

When each account has room, as a timeline: one lane per account, and a strip above them counting
how many of the accounts read have room at each moment, with a line along its floor, in the ramp's
last stop, where none has; with none read, it draws nothing.

- **The labels**, at the lanes' left, each lane's place and account, and at their right, over the
  day, `▲ next` on the account new sessions go to, or over the week, its week's use, take half the
  width at most: where they don't fit, the account's name is cut short, and `▲ next` gives way to
  `▲`.
- **The day**, by default: from the hour before now, taken back to its ten-minute mark, for 22
  hours and 40 minutes, across the width the labels leave: 136 columns of 10 minutes at 160
  columns, the minutes a column scaling with the lanes' width. The hours run along the top, ticked
  at least 3 cells apart and labelled at least 10 apart, an hour the clocks went forward over left
  out, and a day's start, its midnight, or wherever the clocks put it, ticked and labelled with its
  weekday: where they went forward over midnight, the hour they went forward to, and where they
  went back over it, the first of its two; `now` and its column are picked out in `bg.subtle`, and
  the past dimmed. A lane is thick, `▆`, where the account can take a session; `▆` in
  `accent.attention` where it can but is heading to run out, from now until the last time it runs
  out before a reset; and a thin line, `─` in `state.destructive`, where it can't. The 5-hour
  window, its limit, and its week running out all count, and over the day a refusal of every
  request too: an account's room is all of them. A limit's stretch starts at the router's event of
  it, and a hold whose start isn't known runs from before the timeline.
  An account without a usable token has no room all along; so has one whose usage can't be read,
  its words saying why, as `can't read it · timed out`, and one not yet read is left blank, each
  even while a router limit holds it, which its card shows instead.
- **Words where it changes**, on the line under the lane, where each stretch without room starts:
  `runs out ~16:05 · back 17:10, as it resets`; `reaches its reserve ~15:45 · back 17:10, as it
  resets`, or already there, `at its reserve · back 17:10, as it resets`; `limit reached 14:12 ·
  back 15:54`; `refused (401) · back 14:00`; `week runs out ~Fri 04:06 · back Sun 02:00`. Where
  causes overlap, each is told where it starts, and `back …` once, where room really returns.
  Words that don't fit drop whole parts from the end, `, as it resets` first. A lane with room all
  day says `room all day`, and where it's heading to run out after the day, when: `room all day ·
  week runs out ~Fri 04:06`.
- **`w`, the week:** from yesterday to six days ahead, a column about every 75 minutes, the days
  along the top, each midnight ticked in the column it falls in, as on a card's week, and each
  day's quarters ticked where it's wide enough, the first, partial day's among them: the weeks'
  room alone, a `┃` where each week resets, labelled `resets Mon 21:00 ·
  87% used by then`, or `runs out ~Fri 04:06 · back Sun 02:00, as it resets`, and a week that
  resets beyond the view, `room all week · resets …`. The strip shows the stretch when fewer
  accounts have weekly room, as the final page's Friday to Sunday. The footer says which shows,
  `w window: day` or `w window: week`, which, unlike the featured window, the preferences file
  doesn't keep.
- **The legend**, over the footer: `▆ has room · ▆ has room, but running out · ─ no room`, and for
  the week, `┃ week resets`.

Primes and the 5-hour windows' resets that change nothing about room aren't drawn: the timeline
says when there's room, not why. COMING UP and the cards say why.

### Themes

The dashboard is drawn in a theme, which gives each of its tokens a colour. Themes are built in, or
written by the user as `.theme` files, and chosen in a picker drawn over the view.

- **Tokens:** 19 base tokens, which every theme file gives, for meaning and prominence, never a hue:
  `text.primary`, `text.secondary`, `text.tertiary`, `text.muted`, `text.subtle`, `text.faint`,
  `text.on-selection`, `accent.primary`, `accent.key`, `accent.mode`, `accent.attention`,
  `state.positive`, `state.destructive`, `canvas`, `bg.selection`, `bg.attention`, `bg.subtle`,
  `border` and `text.on-attention`. The charts' own, `viz.*`, are each optional, worked out from the
  base tokens where a file leaves it out:

  | Token | Is | Default |
  |---|---|---|
  | `viz.ramp.1`–`viz.ramp.4` | A bar's fill, from its first cell to its last | `state.positive`, `accent.attention`, halfway from `accent.attention` to `state.destructive`, `state.destructive` |
  | `viz.track` | A bar's empty cells | `border` |
  | `viz.pace` | The even-pace marker | `text.primary` |
  | `viz.reserve` | The reserve's mark and floor | `accent.key` |
  | `viz.series.1`–`viz.series.6` | The accounts' cords, in order, round again past six | `accent.key`, `state.positive`, `accent.primary`, `accent.mode`, `text.secondary`, `accent.attention`: never `state.destructive`, which a limit or refusal draws in |

  So a theme missing a base token is rejected, and a theme without any `viz.*` still draws every
  chart.
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
  default pair; `amber`, an amber CRT; `exchange`, a telephone exchange's brass and walnut; and
  `terminal`, for a terminal with a transparent or image background, which paints no background and
  uses the terminal's own 16 colours. `terminal` is built in, not a file, as `#RRGGBB` can't name
  the terminal's colours, and where the others blend into the canvas, it draws shade glyphs instead:
  a bar's projection `▒`, a chart's past level in its account's colour, unblended. What's dim, its
  text, borders and tracks, is the terminal's own foreground, faint, and `bg.selection` and
  `bg.attention` are reverse video, so it leans on no colour that's some palette's background. The
  final page draws a sheet for each, its tokens as swatches beside a card. Themes are colour alone:
  a look that needs other glyphs or capitals, as the instrument cluster spiked in round 1 does,
  belongs to no theme.
- **Choosing:** one theme, or a pair, one for a light terminal and one for a dark, the terminal's
  background asked once as the dashboard starts (OSC 11). The watch and the one-shot `usage` both
  give the terminal 150 milliseconds to answer before they draw, one that doesn't taken for dark;
  the watch takes the answer whenever it comes, as the background found, and one that comes late
  and gives the other half of the pair has the dashboard drawn in that from then on. Nothing
  chosen means the pair: `tokyo-night-day` for light, `nord` for dark. `t` opens a slide-over at
  the right, drawn over the view so it stays visible: the themes, each previewed live as the arrows
  reach it; `enter` sets one theme; `d` and `l` set the dark and light halves of the pair; `esc`
  closes it, putting back the theme in force. A row's badge says what it fills: `●`, `● light`, `●
  dark` or `● both`. Setting one theme clears the pair, and setting a half of the pair clears the
  one theme, asking `y`/`n` first. Each is set in the choice as it's kept now, as another dashboard
  may have changed it since the picker opened, and `y`/`n` is asked only where one theme is kept.
  While the picker is open, the footer lists its keys alone.
- **The preferences file:** `<state dir>/prefs.json`, which the dashboard writes and the user never
  needs to: the theme or the pair, the view shown, the featured window and the chart style. It's
  written whole, as `state.json` is, in the state directory as it changes as the dashboard is used,
  never in the config directory, whose file is often a link into the user's dotfiles; and never the
  config file, which is the user's, and which the dashboard never rewrites. Its keys are written in
  sorted order, and those it doesn't know, as a newer build's, are kept as they are. One that can't
  be read is set aside as `prefs.json.corrupt-<unix time>`, or, where it's a link, where it leads,
  the link kept, and the defaults stand.
- **The background:** in watch mode the dashboard owns it: it paints `canvas` on every cell and
  sets the terminal's background to it, having asked for the old one first (OSC 11),
  and puts it back on every exit it can catch: quitting, an interrupt, a terminate signal, or a
  panic it recovers from; where the terminal didn't answer, it resets it instead (OSC 111), which
  restores the terminal profile's own. A kill leaves `canvas` as the background until the terminal's
  reset, so a background the terminal reports that's a dashboard's own canvas is never set back:
  it's reset with OSC 111 on exit, its half of the pair going by how dark it is all the same. A
  theme that paints none, shown mid-session, as from the picker, sets back the background found,
  or resets it with OSC 111 where none was found. The blends the charts and bars draw in are worked
  out against `canvas`. The one-shot `usage` paints no background, printing into the scrollback:
  where it shows colour, and never from a job in the background, it asks the terminal for its
  background (OSC 11), through `/dev/tty`, picks the light or dark half by it, and blends against
  the colour it gets. It asks the terminal's device attributes after (DA1), which every terminal
  answers, so one that doesn't answer OSC 11 is known at once. Having given up on an answer, it
  keeps the terminal raw for a grace of 100 milliseconds, ending as the DA1 reply comes, reading
  and dropping what comes late, so no answer is left in the shell, and an answer that comes in the
  grace isn't taken. One theme chosen prints only on a background as dark or light as its own,
  else the default pair's half for that background. The `terminal` theme never paints, and blends
  nothing.
- **Fewer colours:** the frame is drawn in the theme's colours and brought down to what the
  terminal shows by `colorprofile`, as now. With `NO_COLOR` set, there's no canvas and no colour:
  state is told by its glyphs and bold, a bar's projection in `▒`, and `t` does nothing.

### Live updates

- **Where it reads:** `usage` and `status` read the router's status document whenever the router
  answers its health check within the half second `run` gives it, healthy or not: an unhealthy
  router's trouble is for them to show, and it still posts the notifications. Otherwise they probe
  every account, and the document's `fallback` says why the router's wasn't read:
  `{"router": "not running"}`, or `{"router": "unhealthy", "reason": "…"}` when something answered
  its socket, but not as a router does, or not within the half second (`no answer within 500ms`).
  `--probe` probes regardless, saying nothing of the router.
- **Watch mode** (`usage -w [interval]`). Reading the router, it looks at the router's document
  every 5 seconds, which costs nothing upstream, and every interval has the router `POST /refresh`
  with the interval as `max_age`, so idle accounts are probed no more often than the watch asks:
  sooner, backing off from 2 minutes to the interval, while an account can't be read. A minute
  after a window on screen resets, the next look has the router refresh first with a `max_age` of a
  minute, once a reset, so an idle account's window doesn't read `resets now` until the next
  interval; but not for the windows of an account whose 5-hour window has lapsed, as the router
  probes it only while it can take no request (see Priming): that window reads empty instead, and
  the account's others as read. Probing, it reads every interval, a minute after a window on screen
  resets, and sooner after a failure, backing off from 2 minutes to the interval. A look never
  probes: when the router stops answering one, the router's last document stays on screen, the
  footer saying since when there's been no router, and the looks go on every 5 seconds, reading the
  router again as soon as it answers. A router away for a moment, as when it restarts or is slow
  on waking, so has no account probed directly, which would start every lapsed 5-hour window at
  once, off the priming schedule. Only the next full read, due an interval after the last, or `r`,
  probes instead. Probing, it asks after the router at each probe and once a minute between, and
  reads it again as soon as it answers, so it never goes back and forth faster than that.
- **What each look reads:** the status document and, from the router, `GET /sessions`, for the
  cards' dots and backs and the calls; both are local and cost nothing upstream. A router that
  can't list its sessions shows none, never those another router listed.
- **What moves, at each look:** bars ease to their new readings, as now; charts take their newest
  column; session dots light and dim; new events join RECENT and the cards' LATELY; COMING UP and
  the Runway's lanes move on. A card whose state changes, as to under pressure or a limit, has its
  state line, and an event newer than the last look saw, by its `id`, its row, in `bg.attention`
  for 5 seconds, fading back; a state the clock changes, as a limit lifting at its time, is picked
  out so on the second's tick.
- **What moves every second,** with nothing read: the clock, the countdowns (`1:12`, `in 1h 12m`),
  the `now` columns, and the waiting times on the cards' backs.
- **The history** behind the charts is read with each full read, not each look: `GET /history`
  (see Control API), each window's over its current length. Probing without the router, there's no
  history: the charts draw from the readings the watch itself has
  seen, so they fill in as it runs.
- **The request stream:** while Sessions is shown, a card is flipped, or the cards draw
  hourglasses, the watch subscribes to the router's request stream, `GET /stream` (see Control
  API). While Sessions' switchboard shows, it animates the cords as the stream tells, at up to 30
  frames a second; the cards' backs and the plain list redraw as the stream tells, and on the
  second's tick. A frame is drawn only for what moves on screen, never for a cord or an hourglass
  scrolled out of view or under `?`, so nothing new is drawn while nothing happens. The shimmer
  steps on the clock's 80-millisecond marks, every cord in step, its frames coming at its steps
  where it's all that moves, as an hourglass's come at its sand's. The stream opens with the
  requests already in flight, so a view opened mid-answer shows it. A stream that ends is joined
  again a second later, and one that fails to open is tried again after a second, doubling to 30
  seconds. A router from before it has none, and the watch stops asking until another router
  answers: the cords and the cards' backs fall back to the sessions as each look reads them, with
  no requests travelling.
- **Recent events:** the router's own, from the status document's `events` (see The status
  document): a session starting, `14:41 ▲ 3e7a started on side, the best`; coming under pressure,
  `14:38 ● work came under pressure: its session runs out ~16:05 at its last-30-min rate`, or where
  its reserve held it back as the event came, no pin spending it, `…: its session reaches its
  reserve ~15:45 at its last-30-min rate`; a limit, with the sessions it moved, `14:12 ■ personal
  reached its session limit; 3 sessions moved to side`, its line taking in the moves it forced,
  which Sessions' `LOG` lists, and which show alone only where the limit's line isn't among
  RECENT's; a move by pin, `14:39 ▸ 9e21 moved side → client (pin)`; a refusal; a prime, `13:50 ◇
  side primed: its 5-hour window started, resetting 18:50`; room again; a restart due; the router's
  health turning. RECENT, as LATELY does, shows times as the cards date them, and accounts by their
  labels. Probing without the router, the dashboard has none, and RECENT says `the router isn't
  running`.
- **A router from before milestone 5**, as one still running between an upgrade and its restart:
  without `GET /history`, the charts draw as without history; without `events`, RECENT says
  `restart the router for recent events`; without `GET /stream`, as above.

### Keys

| Key | Where | Does |
|---|---|---|
| `tab`, `shift-tab` | everywhere | The next view, the one before |
| `w` | Accounts, Runway | Cycle the featured window: `auto`, `5h`, `week`, any other in use; in Runway, the day or the week. Nothing before the first read |
| `g` | Accounts | Cycle the chart style: burn-down, burn rate, hourglass |
| `←` `→` `↑` `↓` | Accounts | Move the focus between cards; on a flipped card, `↑` `↓` select its sessions first |
| `space` | Accounts | Flip the focused card |
| `s` | Accounts | Flip every card, or back |
| `esc` | everywhere | End a selection; close the theme picker or the help |
| `j` `k`, PgUp, PgDn, wheel | wherever it scrolls | Scroll |
| `1`–`9` | everywhere, while the router answers | Toggle the account in that place in the global pin; with a session selected, pin that session there |
| `a` | everywhere, while the router answers | Route automatically again; with a session selected, clear its own pin |
| `m` | everywhere, while the router answers | Move running sessions to the pinned accounts |
| `r` | everywhere | Refresh |
| `t` | everywhere | The theme picker |
| `?` | everywhere | Every key there is, and the key to the glyphs, over the view; `esc` or `?` closes it |
| `q` | everywhere | Quit |

`r` has the router probe the accounts it hasn't read in the last minute, and those that can take no
request anyway, however lately it read them, as a reset made by hand shows only to a probe, but for
those whose 5-hour window has lapsed and that can take a request, and those it probed in the last
minute; or, without it, every account is probed, as `usage --refresh` does. While it reads the
router, and the router answers, a digit toggles the account in that place, as configured, in the
global pin: one it doesn't name joins those it does, new sessions going to the best of them, and
one it names leaves, the last to leave routing automatically again; `a` routes automatically again;
`m` moves running sessions to the pinned accounts, or says nothing's pinned. What a digit or `m`
sends leaves out an account the pin names that has no usable token, as the document shows, having
lost it since, as the router refuses a pin naming one. A digit sets a pin that doesn't move running
sessions, as `pin` without `--move` does. Each says in the footer what it did, or why it couldn't,
for a few seconds, and the router's document is read again at once. Pressed while the router's last
document stays on screen, the router not answering, each says so instead. With one account, the
digits, `a` and `m` do nothing and aren't listed.

The footer lists the keys that work where they are, the most used first, in this order, as many as
fit, and `? keys` and `q quit` always; and at its right when the document was read: `tab views · w
window: auto · ←→ focus · space flip · g chart · 1-3 pin · a auto · m move · ? keys · q quit`,
then `read 4s ago`. What doesn't fit is behind `?`, as `r`, `t`, `s` and `j`/`k` always are.

### Kept from the dashboard before milestone 5

- Desktop notifications: see Notifications.
- Text from elsewhere, such as labels and the upstream's errors, shows with its control characters
  as spaces, here and in `status` alike, so none can move the cursor or restyle what follows, a
  statusline's included.
- `status`'s text keeps the form and the words it has, as Pace and projection quotes them, until
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
`from the router: unhealthy, <reason>  ·  <sessions>  ·  <routing>` and the dashboard's ROUTER
slot `● unhealthy`, in red, with the reason under it.

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

The dashboard in watch mode posts its own only while it probes because the router isn't there to:
of an account with room again and a window passing the warning, as `room` and `warning` say, and
none with `--no-notify`. While the router answers, the dashboard posts none, so nothing is told
twice: probing with `--probe`, it asks after the router before it posts, and posts only when it
doesn't answer. It sees no limits or moves, which are the router's alone.

## The request ledger

The readings history says how each window's use changed; the request ledger says what used it. It
holds a line for each request the router routes, written as the request ends, and a summary of each
day once the day is over. The dashboard's History and Accounts tabs, and a session's own page, look
back through it, as agents do through `requests` and `history` (see Commands). It records from the
first day a router that has it runs, so nothing before then is in it.

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

  What's kept can be trimmed, or kept for less time, or packed tighter, once the views that read it
  are designed; what isn't kept can never be added for the days gone by. So it keeps all of this
  until then.

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
  once, ahead, as the days ask for their readings, oldest first, holding a week and a day of them
  at a time, and starting again past days none asks for, rather than read the readings of a year
  between a day left unsummarised and yesterday. Its object:

  ```json
  {"version": 1, "day": "2026-10-05", "lines": 424, "bytes": {"plain": 0, "compressed": 41208}, "accounts": [{"account": "work", "models": [{"model": "claude-opus-5-5", "upstream": 412, "no_usage": 2, "unsent": 0, "checks": 3, "counts": 9, "sessions": 6, "usage": {"cache_creation": {"ephemeral_1h_input_tokens": 1180240, "ephemeral_5m_input_tokens": 0}, "cache_creation_input_tokens": 1180240, "cache_read_input_tokens": 61204410, "input_tokens": 8812, "output_tokens": 402113, "server_tool_use": {"web_search_requests": 4}}}], "sessions": 6, "moved_on": 2, "moved_off": 1, "highest": {"5h": 1, "7d": 0.41}, "limits": [{"window": "5h", "at": "2026-10-05T14:37:12Z", "resets_at": "2026-10-05T17:10:00Z"}]}]}
  ```

  - `version`: the summaries' version, 1 so far. A release that summarises a day otherwise, or
    keeps more of it, writes another, so the summaries written before it can be told: those whose
    lines are still kept can be summarised again, and a reader knows what the rest never counted,
    rather than taking it for none.
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
    window was to reset.

  A summary is written whole, and written again while lines come to be filed under its day: a
  request in flight past the hour the day is given, a change of time zone, or a clock set ahead and
  set right again, files a line under a day already summarised, which `requests` lists, and which
  `history` would never count otherwise. So, at each round, while a day's lines are kept, a day
  whose files hold more lines than its summary's `lines` is summarised again, over it. None is
  taken away while they're kept, so fewer means some were lost since, as to a damaged file, and
  the summary made from more stands; once they're pruned, it's kept for good: a year of them runs
  to a few megabytes.

  A summary written again knows no less than the one it replaces. Each count it holds, a model's
  requests, sessions and usage, field for field, an account's sessions and moves, and `lines`, is
  the one before's where that's more, as fewer means lines were lost since, and an account or a
  model the lines no longer give is kept as it was; each window's highest use is the one before's
  where that's higher; and the limits the one before held are among its own. The readings history
  prunes the readings of a day past its own keep, whatever the ledger's, and a clock moved can
  leave it short of them, so what the readings gave can't be had again from them: and a limit
  whose window has no reading before it, in the day or the week before, which a summary takes for
  one reached, is its own only where the summary before held it, as the reading before it may be
  one pruned since, which would have said the limit held already.

  A round counts a day's lines only where its files have changed since its summary was marked:
  the summary holds their sizes, as `bytes`, and its modification time, its stamp, is set to when
  the day's compressed file was last modified, as they were when it was made, or last counted and
  found to stand. A day with no plain file, whose compressed file still gives that time, costs a
  look at its files and its summary's, reading none of them: compressing a day writes its
  compressed file anew, once a day at most. Any other has its summary read, and its lines counted
  only where its files' sizes differ from those it holds: lines are only ever appended to the
  plain file, which grows with each, however coarsely the file system keeps its times: one that
  keeps whole seconds, as HFS+ does, gives a line appended within the second the file was last
  modified in no time of its own. Read whole, a year of heavy days runs to gigabytes. The time
  must be the same, not merely no later, as a clock set back, or set ahead and right again, can
  give a file changed since its summary an earlier time. Fields may be added to a summary, never
  renamed. A day left without one, as when the router was stopped as the day ended, is summarised
  by the next round that finds its lines, compressed or not. A day one of whose files can't be
  opened isn't summarised from the rest, as that would be taken for the whole day, but left for a
  later round, as is one whose summary, or lines, can't be read to tell whether it stands, each
  warned of at each round until it can be; one whose summary doesn't read as the day's, as one of
  another day, is warned of, and summarised again. A day whose files hold no line that reads is
  summarised as one of no requests, and again once a line comes that does. Views over weeks and
  months read the summaries; today, and any day not summarised since lines came to be filed under
  it, is summarised from its lines as it's read.
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
  its stamp, or its `bytes`, say, or number no more than its `lines`, or can't be counted, which
  is warned of. Today, and any day not summarised since lines came to be filed under it, is
  summarised from its lines as it's read, knowing no less than any summary it would replace, the
  readings history read ahead for them all, as the router reads it, and never written, nor
  marked, as summaries are the router's to write. A summary that can't be read, or doesn't read
  as its day's, as one of another day, is warned of, and its day summarised from its lines, while
  they're kept. A read starts at the first day the ledger holds, the earliest its files of lines
  and its summaries are of, where that's later than the start asked for, as no day before it
  holds anything; at today where it holds none. Every day asked for from there is given, so the
  last is today, one of no requests as a summary of no accounts: none from before the ledger
  began. A session's page reads the lines of its own days; History and Accounts read the
  summaries.

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
  token, an account gaining a usable token, and one losing it, with why; the tokens directory made private as the router starts, and what bringing the skill up to
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
  many (`requests still in flight as the router stops; their lines go unwritten`).
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
  by hand, `…: run switchboard serve again to take it up`; and the dashboard's heading, `restart due
  (config changed)`. `service restart` asks the router to restart now (`POST /restart`): it gives
  its requests in flight up to 30 seconds, and those it cuts off then up to 5 more to unwind,
  answering on the control socket meanwhile, so a `claude` started then is routed, its requests
  waiting on the proxy's socket for the router it becomes, then replaces itself in place, or, as
  it says as it takes the request when it can't, as not knowing its binary, exits for launchd to
  start it again (see Launching).
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

It's short: `claude` runs through switchboard; `status --session`; `status --json`, and what the
status document holds, for every account's usage, and `status --json --refresh` once a limit is
reset by hand; `usage`, the user's dashboard, which Claude points to rather than reads; `pin`, to
one account or the best of several; `requests --json` and `history --json`, which look back through
the request ledger, with or without the router; and `logs`; a move costs one slower turn, and a
moved Claude Sonnet 5.5 session carries on without its earlier reasoning; artifacts always live on
the primary; and `switchboard --help` for the rest.

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
| `cmd/capturetool` | The capture harness's program, never a switchboard command: draws a fixture of `internal/capture` by its name, full screen until `q`, or once, as text to diff with a frame's (`--print`), in a built-in theme or a `.theme` file given by its path (`--theme`), for `vhs` to screenshot and the eye to judge against the final page (see Files). Its import guard fails should anything `cmd/switchboard` builds import `internal/capture` |
| `internal/cli` | Cobra commands. Thin: parse flags, call the packages below, print |
| `internal/config` | Locating, parsing and validating the config file, and editing it: adding and removing accounts, and setting the primary and the priming day; and locating the state directory, and switchboard's bin directory |
| `internal/tokens` | The token files: reading them, checking their ownership and mode, writing them, and keeping their directory private. `tokens/tokenstest` stands in for the token files, for tests |
| `internal/accounts` | Adding accounts, replacing their tokens and removing them, for the `accounts` commands and `setup`: the config file and the token file together, and a token the user gives, typed unseen at a terminal or piped in, checked with the API before it's saved |
| `internal/dayfile` | Files a day of JSON lines, as the readings history and the request ledger keep them: appending, compressing a day's file once its day ended two days ago, removing it once past keeping, leaving one named for a day after tomorrow until its day comes round, and reading them back, oldest first whichever day's file each is in, lines cut short and damaged files included; the days themselves, each from its first instant, and the times of a day, each as the clocks first read it, where they go forward over it, or back over it, too; and the queue and goroutine that write to them, so noting a line never waits, with the round they're kept on, hourly and before each prune, which what else is kept of a day, as the request ledger's summaries, is kept on too, at every round or just before each prune |
| `internal/ledger` | The request ledger: its lines and their writing, through `internal/dayfile`, the days' summaries, written again while lines come to be filed under their days, each knowing no less than the one it replaces, reading lines and summaries back, with no router, today and the days not summarised since lines came to be filed under them summarised as they're read, and the price table and the worth it gives |
| `internal/readings` | The readings history's lines, as the router writes them and reads them back, and the request ledger summarises its days with them: a reading as a line, and the history's files in the state directory, through `internal/dayfile`, and the readings they hold of a time, in the order they were read |
| `internal/atomicfile` | Writing a file whole or not at all: beside where it goes, synced, then renamed into place; and where writing through a link leads, so a file that's a link is written where it leads, never replaced |
| `internal/linescan` | Reading text a line at a time, holding a line only as far as a most given: a longer one is passed over without being held, and counted, and the lines after it read, as the request ledger's and the readings history's files are read back and an answer's stream of events is counted |
| `internal/quota` | The provider-neutral usage model: windows, failures, per-account snapshots, and what a response says of its account |
| `internal/claude` | The Claude provider: usage-header parsing, probes, model families, response classification (a limit reached, throttling, a refused token, a request refused alone), which paths are routed, and which of them spend quota, the session header, Claude Code's environment variables, finding the installed `claude` and its version, whether the `claude` a shell runs from `PATH` is switchboard, Claude Code's local subcommands, and which models' thinking is bound to the account that produced it. `claude/claudetest` makes stand-ins of Claude Code, and of switchboard's binary, `claude` link and another build of it, for tests |
| `internal/score` | Pace, projection, a window's rate of use, eligibility against the reserve, pressure, perishability, the 5-hour tiebreak and the best-account pick. Pure functions of a snapshot and a clock |
| `internal/prime` | The priming schedule: each account's slot from the day and the accounts, and when a prime is due. Pure functions of the day, the accounts, the window a request starts, which the `score.Policy` names, the readings and a clock |
| `internal/status` | The status document, building it by probing every account, what the router says of a session, and their words: `status`'s text, and the countdowns, clocks, titles and state words the dashboard shares |
| `internal/dashboard` | Rendering the views as frames (Lip Gloss): the title row and heading, the Accounts layout rules, the cards, front and back, and their charts, Sessions' calls, lines and cords, Runway's lanes, and the key |
| `internal/dashboard/watch` | Watch mode (Bubble Tea): when to read the router or probe, the history and the request stream, its keys, the focus, flipping and selection, scrolling, easing the bars and animating the cords, the theme picker, and its desktop notifications while it probes without the router |
| `internal/theme` | Themes: the tokens, the built-ins, loading `.theme` files, working out `viz.*`, picking the light or dark half by the terminal's background, and the preferences file |
| `internal/capture` | The capture harness's fixtures, each a moment of the dashboard: the frames' sample accounts and sessions, a fake router serving them, with its history and request stream, and the real watch model built through `watch.New` with every seam faked, so it never dials the router, probes, runs a program, or reads or writes the real config, themes, state, preferences or tokens. Imported by `cmd/capturetool` alone |
| `internal/router` | The proxy and its replays, the scheduler, live account state, priming, the state file, keeping the readings history, in the lines `internal/readings` owns, each routed request's line handed to the request ledger, and the readings it summarises its days with, the router's health, the events it emits and the notifications it posts, the control API and its client, and looking after itself: taking up the token files as they change, and restarting in place for a config change, an upgrade or a new time zone, or when asked |
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
  and `day-<local date>.json`, a day's summary, each 0600 (see The request ledger). The lines are
  appended, compressed and removed as the readings history's are, by the same rules, but kept as
  long as `[ledger] keep` says, 400 days unless it's set, or for good where it says `forever` (see
  Config). A day's summary is written whole, beside where it goes and renamed into place, written
  again while lines come to be filed under its day, and never removed; it holds the sizes of its
  day's files, and its modification time is its stamp, when the day's compressed file was last
  modified, which a round checks before it reads it, or counts the day's lines (see The request
  ledger). The router leaves anything else in the directory alone.
- **Preferences:** `<state dir>/prefs.json`: the dashboard's theme or pair of themes, the view it
  shows, the featured window and the chart style, written whole by the dashboard alone, never by
  hand (see Themes).
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
- **The capture harness,** in the repository: `testdata/vhs/`, where the dashboard is checked by
  eye against its design, which a test can't do. `reference/` keeps the final page's frames,
  exported from Paper as PNGs, and as the text and ANSI of the generator that drew them, which a
  capture, or `capturetool --print`'s text, is held against; it stays, as the design the dashboard
  was built to, though no code points at it. Beside it, `vhs` tapes screenshot fixtures
  `cmd/capturetool` draws: scaffolding, cleared, with their captures, once milestone 5 is signed
  off. `testdata/vhs/README.md` says how, and lists where the dashboard differs from the frames, as
  this document holds over them (see Visual capture harness in `CLAUDE.md`).

### Config

```toml
listen   = "127.0.0.1:4747"             # optional: the proxy's address
upstream = "https://api.anthropic.com"  # optional: the API's base URL; overridden in tests

[[account]]
id      = "work"       # permanent name: letters, digits, '-' and '_'; its token is tokens/work
label   = "Work"       # optional; defaults to the id
primary = true         # optional: the account the browser and the Claude apps use; else the first
reserve = 0.1          # optional, on any account: the share of every window the router leaves; else 0

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

[history]                          # optional: the readings history
keep = "400d"                      # how long the readings history is kept: 8d or more, or "forever"

[ledger]                           # optional: the request ledger
keep = "400d"                      # how long each request's line is kept: 8d or more, or "forever"; the days' summaries are kept for good
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
- **`[ledger]`**: `keep` is as `[history]`'s is: `8d` or more, or `forever`.
- **No error quotes a token:** one that quotes a value, of `listen`, `upstream` or `prime.day`, an
  unknown key's name, or the key a file that isn't TOML fails at, such as one without a value or
  given twice, shows anything in it shaped like a token as `[redacted]`, and an unknown key's value
  is never quoted.

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
| `GET /health` | `{ok, reason, listen, version, pid, started_at, strips_own_headers}`: the router is alive, and `ok` is its health, the judgment the status document's `router.healthy` gives, `false` while it's unhealthy, with a `reason` (see Health). `listen` is the address its proxy listens on. `strips_own_headers`, `true`, says it takes every `X-Switchboard-…` header off a request going upstream (see Proxy rules); a router from before leaves it out. `run` sends sessions to a router that answers `ok` and gives `listen`, and tells the directory a session starts in only to one that says `strips_own_headers`; `usage` and `status` read the document of any router that answers at all |
| `GET /status` | The status document, as `status --json` prints it: see below |
| `GET /sessions/{id}` | For statuslines: `{"session": "<id>", "pin": "<id>", "assignments": [{model, family, account, pinned, yielded, pinned_at, reason, assigned_at, last_seen}], "account": {…}}`. `pin` is the session's own pin, left out when it has none: the one `pin --session` gave it, else the one `run --account` did, as its requests last carried it. `assignments` are the session's, a model each, the one used last first, each naming its model's family, such as `opus`, and its account by id; `yielded` set while the session's own pin has yielded, the account it names having had no room for a request of it, and it stays where it went; and `pinned_at` when it was given its own pin as it ran, left out for the one it was launched with, and while it has none. `account` at the top is the whole status of the account the last used went to, as the document gives it. 404 for a session never seen |
| `GET /sessions` | The sessions routed in the last hour, the one seen last first, each as `/sessions/{id}` gives it but for `account`. `status` lists them, and `pin --session` and `status --session` find a session from part of its id here |
| `POST /sessions/{id}/pin`, `DELETE /sessions/{id}/pin` | Set (`{"account": "work"}`) or clear one session's own pin, answering as `/sessions/{id}` does. 404 for a session never seen; pinning to an account nothing can go out on is a 400 |
| `POST /pin`, `DELETE /pin` | Set (`{"accounts": ["work", "side"], "move": false, "force": false}`) or clear (`?force=true` to clear every session's own pin too) the global pin, answering with the status document. `account`, naming one account, is taken as well, as a switchboard from before pins named several sends it, and sent beside `accounts` with a pin of one account, as such a router reads a pin, one still running between an upgrade and its restart. Pinning no account, or any account nothing can go out on, is a 400, saying why (see Pinning), and pins nothing |
| `GET /history?window=<key>&step=<duration>` | Every account's use of a window over its current length, from the readings history and the readings since: `{"window": "7d", "step": "30m", "accounts": [{"id": "work", "start": …, "points": [{at, utilization}]}]}`, `start` when the account's window started, its reset less its length, or its `restarted_at`, and a point each step from it to now, at most 1,000, each the last reading at or before it, left out where none was; an account whose window isn't running, or wasn't read, has no points, and one whose window has reset since it was read has no `start` either. A window whose length can't be read, or a step that isn't a duration, isn't more than 0, or would take more than 1,000 steps over the window's whole length, is a 400, saying what to give, the least step included. The dashboard's charts ask for `5h` at 5-minute steps and the weeks at 30-minute steps, with each full read |
| `GET /stream` | The requests as they happen, for the dashboard's Sessions, its cards' backs and its hourglasses: held open, `application/x-ndjson`, a line of JSON an event, every one `{at, kind, request, attempt, session, model, account}` and what its kind adds, `request` the router's id for the request, counting up from a random start, and unique while it runs, `attempt` which time it went upstream, `session` and `model` cut to 200 bytes, and `account` the account it goes out on, or, of `first` and `done`, the one whose answer the client got. It opens with an `inflight` for each request already in flight, adding `sent_at`, `first_at` and `chars`, and `verdict`, the last of `limited`, `throttled` or `refused` told of it on the account it went out on last, with that answer's `status`; then `sent` as one goes upstream; `first` at its answer's first byte; `progress`, with `chars`, the characters of its text, thinking and tool input so far, a quarter second after its answer streams more, then every quarter second while it does, as the API counts tokens only as an answer ends; `done` as it ends, told before the router has finished with the request, with `status`, its final `chars`, and `tokens`, the closing usage's counts, `{input, output, cache_read, cache_write}`, left out without one; `limited`, a 429 at a limit, `throttled`, a 429 sent again on the account, and `refused`, a 401 or 403, each with `status`; and `moved`, with `from`, `to` and `reason`, its `account` the `to`, which a request every account refused tells as it takes its session back, `back where it was before its request`. `limited` and `refused` are told once the router has judged the answer a limit or a refusal of the account: a 429 from before a reset made by hand, or a 401 to a token replaced since, which go out again on the same account, are neither. A request the router knows for Claude Code's quota check carries `check: true`. Requests that never go upstream, and those passed through, aren't on it. Reading the counts reads a copy of the answer's stream as it passes, decoded where it's gzip or deflate, as the request asked for one of those alone (see Proxy rules), never changing or holding the bytes passed on; an answer in another encoding goes uncounted, its bytes untouched. A reader that falls 256 events behind is dropped, and reconnects, and one that takes more than 10 seconds over a write is cut off. A stream ends as the control API closes, which a restart does after its drain, so its readers see the requests the router finished, and the dashboard reconnects to the router it becomes |
| `POST /refresh` | Probe the accounts nothing has been read of for longer than `{"max_age": "30m"}`, and those that can take no request anyway, however lately they were read, but for those whose 5-hour window has lapsed and that can take a request (see Priming), sharing the probes choices make and waiting a minute after one ended, as they do; wait 10 seconds at most for them, and answer with the status document. The watch asks every interval, and a minute after a window on screen resets |
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
| `events` | *router* What's happened lately, newest first, 50 at most, kept in memory, so a restart starts them afresh: `[{id, at, kind, account, session, model, from, to, reason, windows, until, since, count, limit, status, family}]`, `id` rising by one an event, from 1 as the router starts, so a reader tells which are new; `kind` one of `started` (a session first remembered, as Choosing an account remembers one, told as its first answer of success comes, so never a quota check's: `account` the account whose answer started it, and `reason` why the session is there, as its assignment says, while it's still there, else why that request went there, another request having moved the session since), `pressure` (an account came under it, but for its first reading, which is no news: `windows` the window under pressure, `until` when it runs out, and `since` when its rate is measured from; told once a reset of that window), `limit` (with `limit`, the limit's identity, which the account's `limit` gives as its `id` while it holds; `count`, the sessions it moved while it holds, each once, by the moves it forced; and `to`, the account they went to when they all went to one, left out when they went to several. Reached again while it holds, it joins its event, keeping its `id`, in whatever order the news of it comes; another limit reached meanwhile is an event of its own), `moved` (with `from`, `to` and `reason`, and `limit`, the id of the limit's event, for a move that limit forced, holding the session's request back), `refused` (with `status`, `family` for a request refused alone, and `until`, brought forward should the refusal lift early), `primed` (a prime that started its window: `windows` the window it started, and `until` its reset), `room` (room again), `restart` (one falling due) and `health` (the router's turning, with `reason` as it turns unhealthy), and the rest as each kind needs. The dashboard's RECENT reads the newest, and each card's LATELY its account's among the 50, both folding the moves a limit counts into its line where that's among theirs, while Sessions' `LOG` lists each. Left out when there are none |
| `accounts` | Every configured account, in the config's order, as below |

Each account:

| Field | Is |
|---|---|
| `id`, `label` | As configured |
| `primary` | `true` on the primary; left out otherwise |
| `reserve` | Its reserve; left out at 0 |
| `token_set` | Whether its token file is present and usable |
| `fetched_at` | When its usage was last read; left out when it never was |
| `windows` | Its windows as last read, shortest first: `{key, label, utilization, resets_at, status, restarted_at}`. `key` is the API's, such as `5h`, `7d` or `7d_oi`; `resets_at` is left out when unknown, and `status` (`allowed`, `allowed_warning` or `rejected`) when not given: a 5-hour window that has lapsed reads 0, with neither. `restarted_at` is *router*: when the window started again, as a reset made by hand that keeps its reset starts it, which its pace and projection measure from until its next reset (see Dashboard); left out otherwise, when it runs a whole length before its reset. Left out when none has been read |
| `lapsed` | The keys of its windows that have lapsed: the 5-hour window, once its reset has passed with nothing read since, which isn't running, and reads empty, until a request starts it (see Priming). Left out when none has |
| `at_reserve` | The keys of the windows at or past its reserve but short of their limit, that haven't reset since they were read; left out otherwise. The router's own choices pass the account over, for the requests those windows count, while there are any; a pin spends the reserve |
| `failures` | Windows a probe expected but couldn't read: `{label, window, error}`, `label` naming what should have read it, such as `Fable`. Left out when none |
| `error` | Why its usage couldn't be read, such as its token file missing, or readable by others, or, from the router, why its last probe read nothing; left out when there's nothing to say |
| `limit` | *router* A limit it reached, while it holds: `{id, windows, until}`, `id` the limit's identity, which it keeps while it holds, reached again, as the router counts its limits from 1 as it starts, and which the limit's event gives as its `limit`; `windows` the keys named as reached, left out when only the overall verdict said so |
| `refused` | *router* The upstream's refusal, while it holds: `{until, status, family}`. `status` 401 is its token refused, holding back every request; 403 a request refused alone, holding back its model's `family`. With both, the token's; with several families, the latest |
| `pressure` | *router* How fast its 5-hour window is being used, and where that's heading: `{window, rate, recent, since, runs_out, under}`. `window` is the window's key, such as `5h`; `rate` the share of it used an hour, never negative: its recent rate, as `rates` gives it, `recent` then set, and `since` when it's measured from, else its use since it started; `runs_out` when, at that rate, it reaches where the account runs out, where its reserve starts, or its limit without one or with the global pin naming the account, left out when it never does, as at a rate of 0, or has already; and `under` set when that comes before the window resets, while the account can take a request of some model, as one refused a model or held back in a model's own week still can: the account is under pressure (see Choosing an account). Of an account that can take no request, pressure isn't what passes it over, and `under` is left out. Left out when the rate can't be said, as when the window isn't running |
| `rates` | *router* How fast its windows have been used lately: `[{window, rate, since}]`, in `windows`' order, `window` a window's key, `since` when the rate is measured from, and `rate` its rise from the level of its use read last before the last 30 minutes to its latest, as a share of it an hour: over those 30 minutes when that level was read again after they began, else over the time since it was last read, a rise across a gap in its readings spread over the gap; or, with no level that far back, from its first, over the time since, 10 minutes at least. Never negative, and 0 for a window read but unused since (see Choosing an account). The projections go by them (see Dashboard). Left out when no window has one |
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
signed off on 2 October 2026 (see Dashboard). Built in stages, a pull request each, each leaving a
working dashboard, judged by eye against the final page's frames through the capture harness, a
pull request of its own after the first (see Files):

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
places: the Dashboard section, and the parts of the others it touched, say what was built. Its
final review, a deep one of the whole stack, changed: a limit has an identity, which its event,
the moves it forced and the account's `limit` give, and another limit reached meanwhile is news of
its own; a routed request accepts only the encodings the router can count; the request stream
ends after the requests a restart finishes; a config file caught mid-save is looked at again
before it's refused; a history file dated ahead is kept; and a router told to stop as it restarts
leaves nothing to connect to.

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

**7. The dashboard redesigned — next.** Its design is being settled area by area with its owner,
each signed off with frames and a spec: the usage view, the Overview, Accounts, History, the
routing pickers, a session's page and the Log so far; then Sessions and Runway. Building starts
once the whole design is signed off, as an implementation plan of its own. Before it's released, a
review of everything it and the request ledger changed for costs that grow with time or with what's
kept, each measured against a year of heavy use: the kind of cost a round's recount of every kept
day was until it learned to look only at the days whose lines changed.

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
| The plain text `switchboard status` prints, redesigned to match milestone 5's dashboard | next, to design with its owner | [dashboard-layout](../ideas/2026-09-30--dashboard-layout.md) |
| Releases signed with a Developer ID, so macOS stops noticing each upgrade | next, once the certificate is in hand | [developer-id-signing](../ideas/2026-10-01--developer-id-signing.md) |
| Three things the router keeps that grow without bound, each only when something rare happens, and a restart's request stream that can end before its last events | open: small fixes, any time | [router-loose-ends](../ideas/2026-10-05--router-loose-ends.md) |
| Judgments with Jev, beside or in place of fixed rules | to storm | [judgments-with-jev](../ideas/2026-09-30--judgments-with-jev.md) |
| OAuth logins in place of setup tokens, kept fresh | later | [oauth-logins](../ideas/2026-09-30--oauth-logins.md) |
| A notice when a session moves, through a hook | deferred | [move-notice](../ideas/2026-09-30--move-notice.md) |
| Other agents than Claude Code | deferred | [other-agents](../ideas/2026-09-30--other-agents.md) |
| Prompt-cache keep-warm | parked | [prompt-cache-keep-warm](../ideas/2026-09-30--prompt-cache-keep-warm.md) |
| An artifact proxy, one browser for every account's artifacts | open | [artifact-proxy](../ideas/2026-09-30--artifact-proxy.md) |
| Intercepting traffic that ignores `ANTHROPIC_BASE_URL` | not planned | [local-ca-interception](../ideas/2026-09-30--local-ca-interception.md) |
| An MCP server exposing switchboard to agents, such as to stream them live events | not planned | [mcp-server](../ideas/2026-09-30--mcp-server.md) |
