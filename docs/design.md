# Switchboard — design

**Status:** the usage dashboard (one-off, and in watch mode with its desktop notifications),
`status`, logging, the router, with its scheduler, pins, state, limit handling, health and
desktop notifications, and launching (`run`, `init zsh` and the service) are built. The dashboard
reading the router comes next.

## What it is

A local proxy that spreads Claude Code sessions across several Claude subscriptions. It uses each
account's quota right up to its limit, moves a session to another account when its account runs
out, and otherwise keeps every session on one account so its prompt cache stays warm. It also
ships a terminal dashboard showing every account's usage.

Scope for v1: Claude Code, authenticated with long-lived setup tokens (`claude setup-token`).
Browser logins and other agents are out of scope. Everything Claude-specific sits behind one
provider interface, so another agent can be added later without reshaping the rest.

## How it works

```
Claude Code ──ANTHROPIC_BASE_URL──▶ switchboard ──▶ api.anthropic.com
                                      │
                                      └─ swaps the Authorization header for the chosen account
```

- Claude Code sends every request to switchboard. Switchboard replaces the `Authorization` header
  with the chosen account's token and forwards the request. Nothing else changes, so the request
  is still genuinely Claude Code's.
- Every response, success or 429, carries `anthropic-ratelimit-unified-*` headers: utilization
  and reset time for each window (`5h`, `7d`, and per-model weeklies such as `7d_oi`). Switchboard
  reads them off real traffic, so it knows each account's usage without spending requests.
  Windows are parsed generically, not hard-coded. A window's reset says which reading is current:
  a later reset is a new window; with the same reset the higher utilization stands, as use only
  rises within a window, so a slow response can't pull it back; an earlier reset is ignored.
- An account with no recent traffic is refreshed with a 1-token probe, and only when a decision
  needs fresh numbers.

## Prompt cache facts the design rests on

- Caches are isolated per organization, and each subscription is its own organization. The first
  request after moving a session rewrites its whole context into the new account's cache.
- Claude Code uses a 1-hour cache TTL on a subscription within its limits, 5 minutes in overage.
- A 1-hour cache write costs 2× base input; a cache read on Opus 5.5 costs 0.05×. The first turn
  after a move therefore costs roughly 40× a warm turn. How the subscription limits weigh cache
  tokens is undocumented.
- Caches don't carry across models.

The spike measured the first and last points directly. Resuming a session on the same account read
35k tokens from cache; resuming it on a second account read nothing and wrote 35k; a second resume
there read 35k again; switching model on that account read nothing and wrote 44k.

Hence: move a session only when its cache is already cold or its account can't serve it.

## Choosing an account

1. **Candidates:** accounts where every window that applies to the request's model has headroom:
   the 5-hour window, the shared weekly window, and that model's own weekly window if it has one.
2. **Score:** perishability = remaining share of the shared weekly window ÷ time until it resets.
   The highest score wins, so quota that resets tomorrow is used before quota that resets next
   week, and a nearly empty account scores low whatever its reset.
3. **New session:** the best candidate is assigned and remembered, keyed on the session id Claude
   Code sends (`x-claude-code-session-id`) and the model. Caches are per model anyway, so a
   session's Haiku calls can sit on a different account from its Opus calls at no cache cost.
   The id survives `--resume`, so a resumed session finds its account again.
4. **Sticky:** the session stays on that account. It is only re-scored when:
   - it has been idle for more than an hour, the cache TTL, so its cache is cold and a move costs
     nothing. Re-scoring prefers its own account, which another must beat by 20%, so near-equal
     accounts don't trade places; or
   - its account can't serve the request. An account nothing has been read of counts as able, so
     neither a session nor a pin moves on no evidence.
5. **Limit hit:** a 429 whose overall status or any window's status reads `rejected` means real
   exhaustion. Switchboard replays the buffered request on the next candidate, among the accounts
   the request hasn't been tried on, before any response reaches Claude Code, and the session
   moves there and stays. Claude Code sees a normal, slower response. The account then has no
   room, whatever its windows read, for the requests the rejected windows count (every request
   when the 429 names none) until the reset the 429 gives: the overall reset, else the latest of
   the rejected windows', else 5 minutes on. A later reading showing those windows with room
   lifts it sooner.
6. **Throttling:** a burst 429 without exhaustion gets a pause, as long as its `retry-after` asks
   (2 seconds when it doesn't say, 10 at most), and a retry on the same account, twice at most;
   then the 429 is passed through. It never triggers a move, because moving would throw the cache
   away for nothing.
7. **No forced return:** after the original account resets, the session isn't moved back; that
   would cost a cache rebuild for nothing. The idle rule brings it back when a move is free.
8. **Pool exhausted:** when no account has room, switchboard first re-probes those whose readings
   say they have none, each at most once a minute, waiting 5 seconds at most, as a reset may have
   passed with no traffic to show it; then it decides again. Failing that, the last 429 is passed
   through.

Each request's account is decided in this order, and the routed line in the log gives the reason:

1. The session's own pin, while its account can serve it (`pinned`); else the pin yields to the
   rest (`pin yields: <id> has no room`). A session that yielded stays where it went while its
   cache is warm, and goes back once it's cold.
2. A global pin set with `--move`, for a session assigned before it: once each (`moved by pin`).
3. The session's account, while its cache is warm and the account can serve it (`sticky`).
4. Afresh: the global pin's account while it can serve the request (`pinned (global)`), else the
   best candidate (`new`, `rescored after <idle> idle`, `moved: <id> has no room`, and when a
   replay moves it, `moved: <id> hit its limit` or `moved: <id> was refused`).
5. With no candidate, the session's account, else the client's (`no account has room`).

A request without a session id is decided afresh every time and not remembered (`unsessioned`).
Before deciding afresh, and never for a sticky request, switchboard probes every account it hasn't
read in 15 minutes, all at once, and waits for them 8 seconds at most. Choices made together share
a probe, and an account whose probe failed waits a minute for the next.

## Pinning

| Pin | Scope | Running sessions |
|---|---|---|
| `run --account <id>` | That session only | Unaffected |
| `pin <id>` | Every new session | Stay where they are |
| `pin <id> --move` | Every new session and every running one | Move on their next request (one cache rebuild each) |
| `pin auto` | Back to routing | n/a |

A per-session pin beats a global pin. Every pin yields at a limit: a pinned session that hits
one moves by the normal rules rather than failing. The per-session pin reaches the proxy as a
request header the launcher sets through `ANTHROPIC_CUSTOM_HEADERS`.

## Requests that need special handling

Learned from TeamClaude (MIT, Node) and taken as ideas, not code:

- **Only `/v1/messages` and its `count_tokens` are swapped.** Everything else passes through
  untouched. That includes the identity-bound paths TeamClaude found (`/v1/code/…`, file uploads,
  the session-ingress WebSocket), and message batches, which must keep the client's own token.
- **`metadata.user_id` is left alone.** It carries a device id, the session id and an account UUID,
  but the UUID is Claude Code's cached account from its last browser login, not the token's
  account, so a mismatch is already normal without switchboard. Setup tokens can't read the
  profile endpoint (403), so the right UUID isn't available to rewrite it with anyway. The spike's
  mismatched requests were all accepted.
- **401s and 403s:** an upstream refusal of a routed request's token is never relayed, because
  Claude Code drops its login on a 403. The account counts as having no room for 10 minutes, and
  the request is replayed on another, as at a limit. With none left, switchboard returns 502
  instead, marked not to be retried, as the same token would only be refused again.
- **Replay:** request bodies are buffered so they can be replayed. Replay only happens before
  response headers have been sent; a failure mid-stream is passed through and Claude Code retries.
  Nor is a request that couldn't reach the upstream at all replayed elsewhere: that isn't the
  account's fault. Claude Code gets a 502 and retries.
- **Storm control:** decided against, as the pause and retry on a throttled account (step 6
  above) already absorbs the burst limit many sessions moving onto one account at once can trip,
  where pacing them would slow every request.
- **Bypass traffic:** some requests (fast mode, WebFetch) ignore `ANTHROPIC_BASE_URL`, so the Claude
  Code process still needs a real token in its environment. That traffic goes out on that account.

## Accounts and tokens

- Accounts are declared in `~/.config/switchboard/config.toml`: an id, a label, and the name of
  the environment variable that holds the token.

  ```toml
  [[account]]
  id    = "work"
  label = "Work"
  token_env = "CLAUDE_TOKEN_WORK"
  ```

- Switchboard never stores tokens. It reads them from its environment, so wherever they already
  live (a password manager, a generated env file) stays the source of truth.
- The background service is a LaunchAgent, and a LaunchAgent doesn't see the shell's environment.
  `service install --env-file <path>` has zsh source that file, one the shell sources too, each
  time the router starts. After tokens change, `service restart` picks them up.

## Commands

| Command | Job |
|---|---|
| `serve` | Run the proxy in the foreground (normally started by the service) |
| `service install [--env-file <path>]\|uninstall\|restart\|status` | Manage the LaunchAgent: see Launching |
| `run [--account <id>] [--direct] [-- <claude args>]` | Start Claude Code connected to the router. Checks the router is healthy first and connects directly if not. `--direct` skips the router and the token, so Claude Code uses its own login |
| `usage [-w]` | The dashboard; `-w` keeps it on screen |
| `status [--session <id>] [--json]` | Accounts, windows, sessions, pins and router health. A statusline asks it for its session's account |
| `pin <id> [--move]`, `pin auto` | Global pin |
| `accounts` | List configured accounts and whether each token is present |
| `logs [router\|cli] [-n N] [-f] [--path]` | Print a log's last lines, or follow it: see Logging |
| `init zsh [--prefix cx]` | Print shell integration: a `claude` wrapper that goes through `run`, and one pinned launcher per account, `<prefix><id>` |

## Dashboard

- One card per account. Any number of accounts; the layout adapts to the terminal.
- Bars with a pace marker (where even use across the window would put you) and a projection
  ("on pace for 92%", "runs out ~Fri 19:40").
- For an exhausted account, a live countdown until it's back.
- Which sessions are on each account, and the current global pin.
- Desktop notification when an account comes back, or passes 90%.
- Keys: `r` refresh, `1`–`9` pin, `a` auto, `q` quit.
- Reads live state from the router. When the router isn't running, it probes directly.
- Built with Bubble Tea v2 and Lip Gloss v2.

## Health

The launcher routes a new session only when the router answers its health check. The harder case
is a router that is running but failing requests: sessions already routed through it fail until
they restart. The router tracks the requests it has routed over the last 5 minutes, and those it
failed itself: a 502 for an upstream it couldn't reach, or for a refusal with no account left to
fail over to. The upstream's own 429s and 5xx, passed through, don't count against it. It's
unhealthy once it has failed 5 of them at least, and half at least: `GET /health` then answers
`ok: false` with a `reason`, the status document's `router` object says the same, and the log
notes the turn, and the turn back, at warn and info. `status` and the dashboard show trouble
loudly. Whether it should also fall back automatically is an open question.

## Notifications

The router sees each limit, move and return as it happens, whether or not a dashboard is open, so
while it runs, the desktop notifications are its own. `[notifications]` in the config says which
it posts, each key defaulting as shown:

```toml
[notifications]
limits  = true   # an account hits a limit, and the sessions it moved
room    = true   # an account has room again
warning = 0.9    # a window passing this share of its limit; 0 turns it off
moves   = false  # every other session move, such as after an idle hour or by pin
```

- **Limits:** a limit's notification waits 5 seconds for the sessions the limit moves off its
  account, as their next requests come, then tells of them together:
  `work · Work hit its Session limit, back at Mon 18:10 — 3 sessions moved to side · Side`. With
  none moved, it says when no other account has room. When the account is back goes unsaid where
  it would make the message longer than a banner shows. One notification a limit, however many
  requests reach it.
- **Room again:** an account whose quota for a request of any model ran out, under a limit or
  with a shared window spent, and has come back: `work · Work has room again`. A refusal isn't
  quota, so one lifting is no news, or a revoked token would be announced every ten minutes; an
  account both out of quota and refused has room again once both are past. The router looks at
  the accounts on every event and every 15 seconds, so a limit lifting or a window resetting with
  no traffic is noticed.
- **Warnings:** a window passing the share given, once a reset: `work · Work: Week at 91%`.
- **Moves:** each move a limit's notification doesn't tell of:
  `session 18bb978f moved from work · Work to side · Side (rescored after 1h 2m idle)`.

Room again and warnings compare an account with how it last stood, so neither tells of how the
accounts stood as the router started, nor of an account's first reading. A limit's notification
always goes out. Any other goes out only a minute or more after the last about its account, a
limit's included: one due sooner is dropped, and the log says so at debug. One that fails to post
is logged at warn, and dropped too. The log names accounts by id alone. Notifications never hold a
request up: the router queues what happens, and posts from a goroutine of its own. `warning` must
be 0, or more than 0 and less than 1.

The dashboard in watch mode posts its own, of an account's return and a window passing 90%,
until it reads the router.

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
  `--log-level` given to `serve` overrides it. Every process logs its `start` (role, version and
  command path, never the arguments, which can hold a prompt) and `exit` (status and duration):
  at `info` for the router, at `debug` for commands, so a statusline running `status` every few
  seconds doesn't flood `cli.log`.
- **Redaction:** nothing logs a token or an account's label; accounts appear by id. As a
  backstop, the handler replaces anything shaped like a token (`sk-ant-…`) in the message or in
  any attribute's text, and the whole value of any attribute keyed `Authorization`, with
  `[redacted]`.
- **Never in the way:** commands never log to stdout or stderr, so `--json` and the dashboard
  stay clean, and a command never fails because it couldn't log: its records go nowhere
  instead. `serve` also writes each record to its terminal, when it runs in one.
- **Reading:** `switchboard logs` prints the router's log once there is one, else the CLI's, or
  the one named. `-n` sets how many lines (50), reaching into `.1` when the log is shorter; `-f`
  follows it across rotations; `--path` prints where it is.

## Architecture

### Packages

| Package | Owns |
|---|---|
| `cmd/switchboard` | `main`: builds the command tree and exits with its status |
| `internal/cli` | Cobra commands. Thin: parse flags, call the packages below, print |
| `internal/config` | Locating, parsing and validating the config file; reading tokens from the environment |
| `internal/quota` | The provider-neutral usage model: windows, failures, per-account snapshots, and what a response says of its account |
| `internal/claude` | The Claude provider: usage-header parsing, probes, model families, response classification (a limit reached, throttling, a refused token), which paths are routed, the session header |
| `internal/score` | Pace, projection, eligibility, perishability and the best-account pick. Pure functions of a snapshot and a clock |
| `internal/dashboard` | Rendering (Lip Gloss), and watch mode (Bubble Tea) with its desktop notifications |
| `internal/notify` | Posting desktop notifications, and the wording the router's and the dashboard's share |
| `internal/logs` | Logging: the handler every package logs through, the log files and their rotation, redaction, and reading logs back |
| `internal/router` | The proxy and its replays, the scheduler, live account state, the router's health, the events it emits and the notifications it posts, and the control API |
| `internal/launch` | `run` and `init zsh` |
| `internal/service` | The LaunchAgent |

Claude-specific knowledge lives only in `internal/claude`. Other packages depend on small
interfaces they define themselves, which `internal/claude` satisfies.

### Files

- **Config:** `$SWITCHBOARD_CONFIG`, else `$XDG_CONFIG_HOME/switchboard/config.toml`, else
  `~/.config/switchboard/config.toml`.
- **State:** `$XDG_STATE_HOME/switchboard/`, else `~/.local/state/switchboard/`. Holds `state.json`
  (pins and session assignments, so a restart doesn't scatter sessions), `control.sock` and
  `logs/`. `state.json` is versioned, rewritten whole (a temporary file renamed over it) a second
  after a change and on the way out, and drops assignments unused for 7 days, at start and then
  hourly. A corrupt one is set aside as `state.json.corrupt-<unix time>`, and the router starts
  without it.
- **Logs:** `<state dir>/logs/`: `router.log`, `cli.log` and their rolled-over files (see
  Logging), and `launchd.log`, where the service's raw stdout and stderr, such as crash output,
  go.
- **Service:** `~/Library/LaunchAgents/io.github.leeovery.switchboard.plist`, the LaunchAgent's
  plist, 0644, named after its label.

```toml
listen   = "127.0.0.1:4747"             # optional: the proxy's address
upstream = "https://api.anthropic.com"  # optional: overridden in tests

[[account]]
id        = "work"                 # permanent name: letters, digits, '-' and '_'
label     = "Work"                 # optional; defaults to the id
token_env = "CLAUDE_TOKEN_WORK"    # environment variable holding the setup token
```

Unknown keys, duplicate ids and a config without accounts are errors, and so is the id `auto`, in
any case, which `pin auto` takes to mean routing. An optional `[notifications]` table says which
desktop notifications the router posts: see Notifications.

### Proxy rules

- A request is routed only when its path is exactly `/v1/messages` or `/v1/messages/count_tokens`
  **and** its bearer token is one of the configured accounts' tokens. Anything else passes through
  untouched: batches, whose ids belong to one account, stay on it, and a local process that
  doesn't already hold a token can't borrow one.
- `X-Switchboard-Account: <id>`, set by `run --account` through `ANTHROPIC_CUSTOM_HEADERS`, pins
  that session. It is stripped before the request goes upstream.
- The session key is `X-Claude-Code-Session-Id` plus the request's model. A request without the
  header is placed on the best account and not remembered.
- Which windows apply to a request: `5h` and `7d` apply to every model. Any other window applies
  to the model families it has been seen on (responses and probes reveal this), and to every model
  until it has been seen. A family is read from the model id: haiku, sonnet, opus or fable.

### Control API

HTTP over `control.sock` (mode 0600, so file permissions are the authentication):

| Endpoint | Job |
|---|---|
| `GET /health` | Liveness, with `ok: false` and a `reason` while the router is unhealthy (see Health) |
| `GET /status` | Accounts, windows, sessions, pin, health: the same JSON `status --json` prints, with the router's `pin` (`{account, since, move}`), its health, `router` (`{healthy, requests, failures, reason}`), and each account's `sessions`, those used in the last hour, and `limit` (`{windows, until}`) while one holds |
| `GET /sessions/{id}` | For statuslines: `{"session", "assignments": [{model, account, pinned, reason, assigned_at, last_seen}], "account"}`, the assignment used last first, and `account` its account's status; 404 for a session never seen |
| `POST /pin`, `DELETE /pin` | Set (`{"account": "work", "move": false}`) or clear the global pin, answering with the status document. Pinning an account nothing can go out on is a 400 |
| `POST /refresh` | Probe accounts whose data is older than `{"max_age": "30m"}` |

A request the API refuses is answered `{"error": "<why>"}`.

### Launching

- `run` gives the router half a second to answer `GET /health` with `ok`. When it does, `run`
  starts Claude Code with `ANTHROPIC_BASE_URL` pointing at the proxy, `CLAUDE_CODE_OAUTH_TOKEN` set
  to a configured account's token (`--account`'s, else the router's best, else the first with a
  token), and with `--account`, the pin header added to any `ANTHROPIC_CUSTOM_HEADERS` already set.
  Otherwise it connects directly on that same token, without the base URL or the pin, saying why in
  one line on stderr: `switchboard: the router isn't running — connecting directly on work · Work`.
  A pin inherited from the environment never survives: what this launch pins is the only pin.
  `--direct` removes the token and the base URL, so Claude Code uses its own login.
- Switchboard never stands between the user and `claude`. When it can't take part at all, as when
  it can't read its config or no account has a token, `run` starts `claude` as if switchboard
  weren't there, environment and arguments untouched, saying why in one line on stderr:
  `switchboard: couldn't read the config (…) — starting claude without it`. Only a misused command
  line fails, such as `--account` naming an account that isn't configured, or has no token.
- It finds `claude` on `PATH`, else where its installers put it, and replaces itself with it
  (`exec`), so signals and the terminal behave as usual. Claude Code's arguments go after `--`,
  untouched and never logged; the log notes the decision: routed or direct, the router's state, and
  the account and why.
- `init zsh` prints, for `.zshrc` to `eval`, a `claude` function that goes through `run` by this
  binary's absolute path, one pinned launcher per account (`<prefix><id>`, prefix `cx` by default),
  and, for tools that call `claude` directly, an export of the first account's token by its
  variable's name (`export CLAUDE_CODE_OAUTH_TOKEN="${CLAUDE_TOKEN_WORK}"`, when that's set), so
  the output never holds a token. A shell function isn't on `PATH`, so `run` finds the real
  `claude`, never the function. Without a config it can read, it prints the `claude` function
  alone, and warns on stderr: the `eval` never fails.
- `service install` writes the LaunchAgent (`RunAtLoad`, `KeepAlive`, output to `launchd.log`) to
  run this binary, its symlinks resolved, with `serve` and any `--config` given, and refuses a
  temporary build, such as `go run`'s. It carries `XDG_CONFIG_HOME`, `XDG_STATE_HOME` and
  `SWITCHBOARD_CONFIG` when they're set, so the service finds what the CLI does. With
  `--env-file`, it runs `/bin/zsh -c 'source "$1" && exec "$2" serve "${@:3}"'`, the paths as
  arguments, never in the script, and warns when others can read the file. It then boots out any
  loaded copy, bootstraps the new one into `gui/<uid>`, and waits up to 5 seconds for a router
  other than any running before to answer. `uninstall` boots it out and removes the plist;
  `restart` is `launchctl kickstart -k`, waiting the same way; `status` reports the plist, whether
  launchd has it loaded, and the router's health. macOS only for now.

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

Still unseen: what a quota 429 and a burst 429 look like (needs a real limit), the identity-bound
paths (not exercised by a plain session), and what Claude Code's own `/usage` and `/status`
report under the router.

**1. Core.** Config and accounts, header parser, probe, scoring, `usage`, `status`. Useful on its
own before the router exists.

**2. Router.** `serve`, stickiness, replay, special handling, pins, `service`, `run`, `init`.

**3. Launch.** Switch the shell over to it. That work happens outside this repo.

## Open-source hygiene

- No personal data in the repo or its history, ever: no account emails or labels, tokens, env
  file paths or machine names. Examples use placeholders.
- Private until it's ready. Licence and a Homebrew release (goreleaser) come with going public.

## Not in v1

- Browser (OAuth) login support
- Intercepting traffic that ignores `ANTHROPIC_BASE_URL` (a local-CA mode)
- Keep-warm: starting idle accounts' 5-hour windows early
- Other agents
