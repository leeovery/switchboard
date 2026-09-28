# Switchboard — design

**Status:** draft. The spike (milestone 0) has run and the core mechanism works; results are under
Milestones. Nothing else is built yet.

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
  Windows are parsed generically, not hard-coded.
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
   - it has been idle longer than the cache TTL, so its cache is cold and a move costs nothing, or
   - its account can't serve the request.
5. **Limit hit:** a 429 whose window status reads `rejected` means real exhaustion. Switchboard
   replays the buffered request on the next candidate before any response reaches Claude Code,
   and the session moves there and stays. Claude Code sees a normal, slower response.
6. **Throttling:** a burst 429 without exhaustion gets a short pause and a retry on the same
   account. It never triggers a move, because moving would throw the cache away for nothing.
7. **No forced return:** after the original account resets, the session isn't moved back; that
   would cost a cache rebuild for nothing. The idle rule brings it back when a move is free.
8. **Pool exhausted:** the 429 is passed through. Switchboard re-probes at most once a minute to
   notice resets.

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

- **Only `/v1/messages` is swapped.** Everything else passes through untouched. That includes the
  identity-bound paths TeamClaude found (`/v1/code/…`, file uploads, the session-ingress
  WebSocket), which must keep the client's own token.
- **`metadata.user_id` is left alone.** It carries a device id, the session id and an account UUID,
  but the UUID is Claude Code's cached account from its last browser login, not the token's
  account, so a mismatch is already normal without switchboard. Setup tokens can't read the
  profile endpoint (403), so the right UUID isn't available to rewrite it with anyway. The spike's
  mismatched requests were all accepted.
- **403s:** an upstream 403 is never relayed, because Claude Code drops its login on a 403.
  Switchboard returns 502 instead.
- **Replay:** request bodies are buffered so they can be replayed. Replay only happens before
  response headers have been sent; a failure mid-stream is passed through and Claude Code retries.
- **Storm control:** after a move, concurrency to the new account ramps from 1 to unlimited over
  about 30 seconds, so many sessions switching at once don't trip its burst limit.
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
  `service install --env-file <path>` gives it a launcher that loads that file before starting.
  After tokens change, `service restart` picks them up.

## Commands

| Command | Job |
|---|---|
| `serve` | Run the proxy in the foreground (normally started by the service) |
| `service install\|uninstall\|restart` | Manage the LaunchAgent |
| `run [--account <id>] [--direct] [-- <claude args>]` | Start Claude Code connected to the router. Checks the router is healthy first and connects directly if not. `--direct` skips the router and the token, so Claude Code uses its own login |
| `usage [-w]` | The dashboard; `-w` keeps it on screen |
| `status [--session <id>] [--json]` | Accounts, windows, sessions, pins and router health. A statusline asks it for its session's account |
| `pin <id> [--move]`, `pin auto` | Global pin |
| `accounts` | List configured accounts and whether each token is present |
| `init zsh` | Print shell integration: a `claude` wrapper that goes through `run`, and one pinned launcher per account (name pattern configurable) |

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
they restart. The router tracks its own upstream error rate, and `status` and the dashboard show
trouble loudly. Whether it should also fall back automatically is an open question.

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
