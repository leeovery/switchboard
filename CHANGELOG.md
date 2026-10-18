# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.5] - 2026-10-18

✨ Added
- A busy session now moves to a different account at its compaction — when that account's week would otherwise reset with room unused, the session switches there since little cache is left to lose.
- The routing reason for such a move reads "compacted: side would reset with 35% of its week left".
- Debug logging explains why a session stayed on its account after a compaction.

🔧 Changed
- Only the main conversation's compaction triggers a move — a subagent's compaction is ignored, so the session's cache is kept.

## [0.2.4] - 2026-10-18

✨ Added
- `w` shows and hides each model's own window, such as Fable's week, on the Overview, Accounts' Detail, Sessions and a session's page — hidden at first and remembered; a model window nearing or at its limit shows either way.
- `a` on Runway collapses the account column to each account's number, marks and use, giving the chart more room; COMING UP then numbers its accounts. Remembered.

🔧 Changed
- Windows are listed the week first, then the 5-hour, then each model's own — on the Overview, in `usage`, GRAPHS, Sessions, the routing card, a session's page, Accounts, `status`, `history --windows` and the Log's cards. The JSON keeps its order.
- Runway opens on the week.
- Σ pool's percentage is its accounts' mean use, so a reserve no longer reads as use; its words still count the room to their caps. `status --json`'s `pool.windows[].used` is the mean, left out where no account's use counts.
- USAGE's blocks are a blank row apart, two only where GRAPHS' charts sit beside them.
- Runway's pool lane is named `Σ pool` on its top row, without the rule above it or the step-by-step words.
- The heading's clock keeps time, bright, while a card is open, so a card left open doesn't read as a hang.
- A phone's view tabs have a cell either side, and a session's path is drawn brighter.

🐛 Fixed
- An account without a usable token adds no room to the pool, as its row shows none.

## [0.2.3] - 2026-10-17

🐛 Fixed
- The dashboard holds far less memory for today's request lines — the ledger follower now keeps only what was gained since the last read instead of every line of the day.
- The Overview's traces now count today's lines from every read of the ledger, so requests that ended while the request stream was reopening are no longer missed.

🔧 Changed
- The Overview's traces no longer run their own separate read of today's lines when the request stream opens — a single read of the ledger's tail now feeds them.

## [0.2.2] - 2026-10-17

🔧 Changed
- The dashboard keeps one shared store of sessions, fed once per read of the ledger, that the List, the Log, a session's page and its points all read — the same session now tells the same story everywhere.
- The Log's Requests table builds its lines on demand from the read instead of copying them each look — far less memory and work on busy days.
- The Log lets go of days its tables no longer show, and re-reads them only if needed — memory stays flat as you scroll back and return.
- Ledger lines decode once into compact held requests, with repeated names shared — a fraction of the previous footprint.
- A session's page keeps what it tallies of sealed requests and folds in only the recent ones — pages on long sessions stay fast.
- Reads of today's or a day's lines no longer hold each line's raw JSON.
- A read of a single session decodes only that session's lines, even on a busy day.

🐛 Fixed
- Opening a turn on a session's page now reads its requests in the background and shows "loading…" instead of stalling.
- A request sent just after midnight no longer makes the dashboard re-read the whole previous day at every look.

## [0.2.1] - 2026-10-16

✨ Added
- GRAPHS can draw a burn-down — press `v` to flip every chart between the burn-up and the burn-down, which shows room left falling from full, where each window is heading, and ✕ where it runs out; the choice is kept.
- By account from 150 columns, GRAPHS charts every account beside its USAGE block, and the pool's charts sit beside Σ pool.
- Pages say "loading…" in their heading while a read of the ledger or readings is under way, including a session's page as it opens.
- The `dashboard` help now lists every page's keys, generated from the same source as the in-app help so the two never drift.
- The router now records a limit lifting early, and the Log's limit and cap events show windows that joined after they began.

🔧 Changed
- Where some accounts' plans aren't known, "against its price" is shown over the plans known, e.g. "4.4× its $92 · 2 of 3 plans known".
- Quota checks are worth nothing and a model used only for quota checks no longer appears in any view.
- A move on a model the price table can't price now says "unpriced" instead of leaving its cost blank.
- Session and turn rows now add up to the cent against the session's total.
- Runway marks room with a full cell, and the pool's lane draws a line along its foot where no account has room.
- Runway's words say "lifts" for a hold that another holds on past, and "back" only for the last.
- COMING UP names an account back from a refused token.
- Detail's list fits its name column to the account names, and the chosen row shows which of the list and SESSIONS holds the focus.
- When the router stops answering, sessions in flight are shown as last known, faint, rather than as working now.
- The Switchboard's cords no longer cross, and its chosen call lights only its own columns.
- A plan is priced from the ledger's first day where the ledger began part-way through the period.

🐛 Fixed
- Short terminals no longer cut the routing card's keys, and the Year, Weeks, Days and Switchboard scroll when they don't fit.
- The Switchboard no longer cuts an account's calls when scrolling.
- Traces and rates no longer run under the theme picker while it's open.
- A request's row in the Log stays put as its line is filed, and a request's line is no longer lost when it was filed on the day before.
- A session's page no longer rebuilds on every tick, and opening one no longer reads other sessions' days.
- A week-long session's page and Runway's lanes are much faster to judge.
- A cap or limit lifted by a reset made by hand now ends at the reset rather than at the window's old reset time.

## [0.2.0] - 2026-10-15

The dashboard, redesigned to be read at a glance: six tabs, each account's usage and routing on one screen, every request on record, and a page for each session.

✨ Added
- `switchboard dashboard` is a command of its own, on six tabs: the Overview, Accounts, History, Sessions, Runway and the Log.
- The Overview shows every account's windows, how the router stands with each, its sessions, what's coming up and what it did lately, in sections you show and hide.
- Accounts compares every account side by side for a period, and shows one in depth: its windows, use by model, the last six weeks, and its plan against what its use was worth.
- History shows the last 12 months a day at a time, each account's weeks, each day's use, and tokens by day, week or month, as ccusage does.
- Sessions draws each session corded to the account it runs on, its requests travelling the cord, and lists today's sessions.
- Runway shows when each account has room, over the next day or the week, and steps back through past days and weeks.
- The Log shows every request and every router decision as they happen, with a details card, a filter and a find, and looks back through earlier days.
- A session's page tells one session's story: its requests over time, each move and what it cost, the accounts it ran on, and its turns.
- Routing from the dashboard goes through one picker, `p`, and every pin waits for you to confirm it.
- `?` on any page shows every key that works there; the mouse picks tabs, rows and keys.
- Under 100 columns the dashboard takes a phone's layout.
- `usage` prints a redesigned block, by window or with `[usage] by = "account"`.
- The router's events record when each window joined a limit or a cap, and a session's moves name their model.

🔧 Changed
- Every view and every JSON verb reads the same judgement of each account, so the dashboard, `status`, `sessions` and `history` always agree.
- The README is a short front page with a tour of the dashboard; the depth moved to `docs/`.

🐛 Fixed
- A new session's account is chosen again once the probes it waited on land, so it never goes out on readings older than a probe that just came in.
- An account at its weekly limit no longer records a false 5-hour reset at every probe.
- A clock change no longer moves when a day's files are kept and compressed.

## [0.1.6] - 2026-10-11

🔧 Changed
- `events --follow` picks up exactly where the listing ends — no line is printed twice or missed.
- The dashboard's COMING UP hides window run-outs and resets for windows no card shows.
- The dashboard's LOG words a token swap as "had a new token".
- A router restart now lets `/stream` readers finish writing what they were told before the control API closes.
- Pricing table refreshed as of 11 October 2026, with Claude Sonnet 5.5's cache-read price change on 7 October applied.

✨ Added
- Claude Haiku 5.5 pricing and version listing.

🐛 Fixed
- A request is no longer treated as having a refused token when the account has since moved to a new token — the account isn't wrongly barred.
- Failed probes sent on a token an account has since replaced no longer count against it — they log at debug and leave its error state and backoff alone.
- A prime's verdict is now judged by when the last probe the account took anything in from ended.
- A request the router reroutes after a new-token 401 is no longer counted as a refusal of the request by every account.
- An unreadable request ledger directory is warned about once, rather than every round.

## [0.1.5] - 2026-10-11

✨ Added
- `switchboard sessions` lists today's sessions with their account, model, state, requests and API-price worth, and `sessions <id>` shows one session's page — its accounts, moves, turns and totals.
- `switchboard events` lists what the router decided and noticed, with `-f` to follow new events as they're filed.
- `history` now totals each account over the period, with plan cost against worth, each week's peaks, and a verdict on whether you have as many accounts as your use needs.
- `history --windows` prints every window's readings over the days asked for.
- Set `plan` on an account and `[prices]` for plans and models in the config to price worth against what you pay.
- `week_starts` config key sets the day calendar weeks start on.
- The router now probes accounts on its own rounds — any account nothing has read in half an hour, and each account 5 minutes before its week resets — so readings stay current without a dashboard open.
- The status document now includes each window's even pace and allowance, the pool of room new sessions can go to, and what's coming up.

🔧 Changed
- `status`, `usage`, `sessions`, `requests`, `history` and `events` print JSON by default whenever output isn't a terminal, with `--json` and `--pretty` to choose the form either way.
- `usage --pretty` prints the dashboard off a terminal, without colour unless `CLICOLOR_FORCE` is set.
- The `history` request count now counts messages only, leaving out Claude Code's quota checks and token counts.
- Day summaries now cover every account the readings history read that day, including days with no requests, so a quiet day keeps a week's peak.
- Request ledger files are no longer pruned before their day's summary is written.
- The skill now teaches Claude the new data verbs.

🐛 Fixed
- A token replaced or newly added now resets an account's probe backoff, so its readings resume at once.
- Readings that arrive late from a window that has since reset no longer count toward the new run's rise.

## [0.1.4] - 2026-10-09

✨ Added
- Request ledger lines now record what Claude Code says of each request — the prompt it serves, its class (main, subagent, compaction and so on), the subagent that sent it, compaction triggers and tool-call durations.
- `claude` started by switchboard now sets `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1` so the router receives those details, unless your environment already sets it.
- The router's request stream now carries each request's prompt, class and agent id.
- `plan` on an account (`pro`, `max5x` or `max20x`) and a `[prices]` table in the config — set plan prices and override per-model token prices.
- `week_starts` config key — choose the day the dashboard's calendar weeks start on, Monday by default.
- `switchboard history` reports when the config's prices stand in place of the built-in ones, and `--json` gains `prices_from_config`.
- The dashboard can now follow the request ledger, readings history and router events as they grow, reading only what each file gained — groundwork for the redesigned views.
- The price table now carries plan prices and a model version table for naming models, Opus 5.5 for example.
- Cache-write cost can be priced on its own — the cost of the context a session rewrites when it moves accounts.

🔧 Changed
- Account day summaries now record session ids, rises and resets of each window with the use before each, and the minutes spent at the cap and at a limit.
- Router events are filed in an `events-<date>.jsonl` file beside the ledger, kept and compressed like the ledger's, so they outlast a restart.
- Config errors now hide anything token-shaped in the key as well as the value.

## [0.1.3] - 2026-10-09

✨ Added
- The router now keeps a persistent record of its events across restarts, with a reader that merges each event's versions back together.
- Usage readings now include extra usage — its status, how much is used and when it resets — and carry it through probes, the state file and the status document.
- New "cap" events tell when an account reaches its reserve, and the sessions that cap moved are counted on it.
- Pin and unpin events record who set or cleared routing (CLI or dashboard), for the global pin and for a session's own.
- Moves forced off an account now name what held the request back — a limit, a reserve or a refusal — and are linked to the event that counted them.
- "Room again" events list the windows that had held an account back.
- Day summaries in the request ledger now record session ids, how far each window rose, resets, and the minutes each account spent at its cap and at a limit.
- Sessions report the directory they were started in and whether a request is asking or answering.

🔧 Changed
- Refusals that renew while one is in force now join the same event, and moves they forced are counted on it.
- The `requests` command now needs a config, since it summarises days with each account's caps.
- Older request-ledger summaries are rebuilt once from their lines, so they gain the new fields.

## [0.1.2] - 2026-10-08

✨ Added
- `switchboard requests` lists the routed requests from the request ledger — filter by session or account, start from a day, a time or "3h ago", and read it as JSON.
- `switchboard history` shows each day by account and model with requests, tokens, sessions, moves, limits reached and highest window use.
- `history` prices every model's usage at Anthropic's API rates, shown as "worth" in US dollars — models the price table doesn't know show as unpriced, never free.
- The router writes a summary of each finished day beside the ledger's lines, kept for good even after the lines are pruned.
- `keep = "forever"` for `[history]` and `[ledger]` — a day's files are never removed.
- The skill teaches Claude about `requests` and `history`, so agents can look back at past use.

🔧 Changed
- The readings history and the request ledger now keep 400 days by default, up from 14 and 90.
- `keep` accepts any number of days from `8d`, no longer capped at `400d`.
- Token counts show in millions and billions as well as thousands.
- Day boundaries follow the local clocks, including days that begin or skip hours when summer time starts, in the dashboard's charts and in the new commands.

🐛 Fixed
- A token pasted as a command, a flag value or an argument is shown as `[redacted]` in error messages instead of being repeated.
- A day's files compressed while being read no longer lose lines.

## [0.1.1] - 2026-10-07

✨ Added
- Request ledger — the router writes one JSON line per routed request (session, directory, model, account, why it was chosen, status, timings, request shape, answer, token usage and rate-limit headers) to a file a day under `ledger/`, never holding message content or tokens.
- `[ledger] keep = "90d"` config key sets how long the ledger's day files are kept; they're compressed after two days.
- `switchboard run` tells the router which directory Claude Code started in, shown with `~` for your home, so ledger lines carry it.

🔧 Changed
- The router strips every `X-Switchboard-…` header before sending a request upstream, so the API never sees one, including those a newer `run` sends.
- Stopping or restarting the router now cuts off requests still running after 30 seconds and gives them 5 more to unwind, so their ledger lines and state changes are kept.
- Dashboard themes and docs no longer reference Portal.

🐛 Fixed
- A request whose body can't be read now gets a proper error and a ledger line, instead of going unrecorded.
- Answers with an overlong event line no longer stop token counting for the rest of the stream.
- A day file left ending in a half-written line is closed off before new lines are appended, so neither line is lost.

## [0.1.0] - 2026-10-03

✨ Added
- A redesigned dashboard: a card per account with big-digit readouts, charts and a heading summing up the router, its sessions, where new sessions go, room left and what's coming up.
- Three dashboard views, switched with `tab`: Accounts, Sessions (a switchboard of cords from each session to its account, with a live log) and Runway (when each account has room, over the day or the week).
- Card charts you cycle with `g` — a burn-down, a burn rate or an hourglass — and `w` to choose which window each card features.
- Flip a card (`space`, or `s` for all) to see its sessions, pick one out with the arrow keys and pin it to another account with a digit key.
- Live request tracking in the dashboard: pulses along cords as requests go out, answers stream back with token counts, and refusals bounce back red.
- Dashboard themes: built-in `nord`, `tokyo-night`, `tokyo-night-day`, `amber`, `exchange` and `terminal`, a live theme picker on `t`, light/dark pairs chosen by the terminal's background, and support for your own `.theme` files.
- A `?` help panel listing every key and what the dashboard's glyphs mean.
- `GET /history` and `GET /stream` on the router, serving the usage history and a live stream of request events that feed the dashboard.
- The router now keeps a list of recent events — sessions started and moved, limits, pressure, refusals, primes, restarts — shown in the dashboard and the status document.
- `[history] keep` config option sets how long the readings history is kept (8d to 400d, default 14d).
- Readings history files are compressed once their day ended two days ago.
- Animated demos and stills for the README, recorded from scripted scenarios with the new `capturetool`.

🔧 Changed
- Reserves are now opt-in on every account, the primary's included — with none set, the router runs every account to its limit.
- `usage` printed once draws the full Accounts view, with its charts, in your chosen theme.
- The dashboard remembers your view, featured window, chart style and theme between runs.
- The dashboard's colours are brought down to what the terminal shows, and `NO_COLOR` draws it with no colour at all.
- README rewritten to lead with tracking and routing usage across accounts.

🐛 Fixed
- Router restarts no longer leave a connecting launcher hanging while it stops.
- A config file caught half-saved no longer causes a refused or premature restart.
- A projection no longer reports a window running out when it only reaches its limit just as it resets.

## [0.0.6] - 2026-09-30

🔧 Changed
- `switchboard usage` prints the status document as JSON when its output isn't a terminal, such as in a pipe or an agent's shell — data instead of a dashboard's boxes and bars.
- The switchboard skill now points agents to `switchboard status --json` for usage instead of the dashboard.

✨ Added
- `switchboard status --refresh` (`-r`) — has the router read every account it may first, waiting up to ten seconds, so a limit reset by hand shows up straight away.

## [0.0.5] - 2026-09-30

✨ Added
- Pressure awareness for the 5-hour window — the router watches each account's recent rate of use and, when one would run out before its window resets, sends new sessions to the best of the others; running sessions and pinned sessions stay put.
- Readings history — each change to an account's window readings is appended to a daily file under `history/` in the state directory, kept for 14 days, so you can look back at how your accounts were used.
- Recent rates survive router restarts — on startup the router reloads the last readings from the history.
- `status` and `usage` show when an account is under pressure, with when it runs out and the rate it goes by (`under pressure: runs out ~18:21`).
- Projections use the last 30 minutes' rate when that has a window running out sooner, labelled `(last 30 min)` — a burst of use shows at once.

🔧 Changed
- Prime slots fall on ten-minute marks, matching where the API actually starts windows, so the schedule shows real resets (two accounts now prime at 04:10 and 06:40, not 04:15 and 06:45).
- Primes now go 5 seconds after their slot, so they land within the slot's ten minutes even when the Mac's clock runs slightly ahead.
- "Best next" and the router's choice of account both pass over accounts under pressure while another isn't.
- The log notes when a choice passed over an account for pressure, and the choice's reason says so.

🐛 Fixed
- A window you reset by hand on claude.ai is now recognised — its marker and projection measure from the reset rather than the window's start.
- A limit reached before a hand reset no longer sticks — the request is sent again on the same account instead of moving the session away.
- Stale readings from requests sent before a hand reset no longer put the dropped use back.

## [0.0.4] - 2026-09-30

✨ Added
- In-place router restarts — the router replaces itself with its new binary in the same process and hands over its listening sockets, so requests made during a restart wait a moment instead of being refused.
- `POST /restart` on the router's control API — asks a running router to restart once its in-flight requests finish, and reports whether it will restart in place.

🔧 Changed
- `switchboard service restart` now asks the router to restart in place and says which way it will go (`it restarts in place` or `launchd starts it again`) — older routers still stop and are restarted by launchd.
- Config, upgrade and time-zone changes now trigger an in-place restart rather than an exit, so launchd no longer has to start an upgraded binary afresh, which macOS has been seen to refuse.
- A router whose binary is briefly missing during `brew upgrade` retries for a couple of seconds before falling back to exiting for launchd.

🐛 Fixed
- `service restart` now refuses with a reason for a router run by hand or one whose config isn't valid, instead of stopping it with nothing to start it again.
- A stop signal received while the router is finishing its requests to restart now stops it immediately instead of restarting.

## [0.0.3] - 2026-09-30

🐛 Fixed
- Removing an account no longer strands running sessions — sessions holding a removed account's token stay routed as the primary's for a week.
- A request refused by every account it went out on no longer bars accounts or moves its session — the refusal says more about the request than the accounts.
- Sessions whose request was refused now return to the account they were on before it.
- Once every account has refused a request, it isn't tried again elsewhere, and Claude Code gets a 502 giving the API's actual reason.
- Per-request refusal tracking means one request taking back its refusals no longer erases those placed by other requests.
- `best next` now respects the global pin — it shows where a new session would actually go.

✨ Added
- `switchboard status` and the dashboard report a pending router restart — with the reason, since when, and requests in flight — e.g. `restart due (config changed)`.
- `switchboard service restart` is now the pointed-to way to apply a pending restart immediately instead of waiting for a quiet moment.
- `switchboard accounts remove` reports which account becomes primary when the primary is removed, and warns if the new primary has no usable token.

🔧 Changed
- Removed-account tokens are kept only as SHA-256 hashes, and are restored to the account if it's configured again.
- A router started by hand with `serve` now tells `status` to run `serve` again to take up the restart.

## [0.0.2] - 2026-09-30

✨ Added
- Pin several accounts at once with `switchboard pin work side` — new sessions go to the best of them until all are out, and the dashboard's `1`–`9` keys now toggle accounts in and out of the pin.
- `switchboard usage --refresh` (`-r`) has the router read every account it may first, so a limit you reset by hand on claude.ai is seen at once.
- The router notices the Mac waking from sleep and sends requests upstream on fresh connections, so connections a sleep left dead no longer fail requests.
- The router restarts itself when the Mac's time zone changes, so the priming schedule keeps to the clock.
- Token files can be links to files kept elsewhere — switchboard writes a token through the link to where it leads.

🔧 Changed
- Accounts are primed 5 seconds after their 5-hour window resets, whether or not they were in use, so a prime always lands after the reset.
- Accounts that can take no request (limit reached, week spent, token refused) are no longer primed, and are probed instead so a manual reset is noticed.
- Account ids that differ only in case are refused, as macOS would give them one token file.
- The dashboard keeps showing the router's last view, saying since when, when the router stops answering, rather than probing every account at once.
- Usage readings off responses and repeated use of a session are saved to disk at most once a minute instead of every second or so.
- `claude` looks a moment later at a token file it finds empty before counting the account as having no token.
- `claude` version checks run with the real Claude Code's own directories first on `PATH`, and no longer hang on a CLI that leaves a process running.

🗑️ Removed
- `accounts remove` no longer deletes the file a linked token file leads to — only the link goes, and the command says so.

🐛 Fixed
- A 429 without usage headers is passed straight to Claude Code instead of being treated as the account's limit, retried or replayed.
- A session is remembered only once a request of it succeeds, so `claude --resume`'s startup quota check no longer leaves a stray assignment.
- A limit lifted early — by a manual reset or a request that then succeeds — is now noticed instead of holding the account back until its stated reset.
- A wrapper named `claude` that `exec`s switchboard, or another build of switchboard, no longer makes it start itself in a loop when looking for the real Claude Code.
- Repeated warnings about an ignored pin are logged once per session and account rather than on every request.
- Installing the service retries when launchd is still finishing unloading the old one, avoiding spurious "Input/output error" failures.
- Request failures that predate a wake from sleep no longer count against the router's health.
- Unknown log names are redacted in the error `switchboard logs` prints.

## [0.0.1] - 2026-09-29

Initial release.
