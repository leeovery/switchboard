# How it works: what happens under `claude`

Switchboard sits between Claude Code and Anthropic, on your Mac. Every request goes through a
small program there, the router, which picks an account, swaps in its token, and reads the usage
off the answer. This page explains the parts, and what the router changes in a request and what
it never touches. It ends with where each topic is covered.

## The parts

- **The `claude` link.** `switchboard setup` puts a link named `claude` first on your `PATH`,
  ahead of the real one. When you run `claude`, the link starts the real Claude Code, pointed at
  the router. That works wherever `claude` runs, from a script or `claude -p` too.
- **The router.** A program on your Mac that Claude Code sends every request to. It listens on
  `127.0.0.1:4747` by default. The service, a LaunchAgent, starts the router at login, and again
  if it stops.
- **The state directory,** `~/.local/state/switchboard/`, or under `$XDG_STATE_HOME` if you set
  it. The router keeps the token files there, and the state it needs after a restart. It also
  keeps the record: every request without its content, every change in a window, what the router
  decided, and a summary of each day.
- **The dashboard and the verbs.** `switchboard dashboard` shows your accounts, sessions and record
  in the terminal. Commands such as `status`, `sessions` and `requests` print the same facts, as
  JSON for Claude or as text for you. One piece of code builds each view for both, so the two
  never disagree.

## The request path

```
claude, the link
  │ starts Claude Code, pointed at the router
  ▼
Claude Code ──▶ router ──▶ Anthropic
                  │
                  ├─ picks the account
                  ├─ swaps in its token
                  ├─ reads usage off the answer
                  ├─ on a limit, sends it again
                  └─ records the request
```

Claude Code talks to the router as it would to Anthropic. The router sends each request on with
the picked account's token, and passes the answer back as it streams.

## What the router changes

A conversation request is one to `/v1/messages`, or to `/v1/messages/count_tokens`. On each one,
the router changes two things:

- **The token.** It replaces the `Authorization` header with the picked account's token.
- **The encodings.** It narrows `Accept-Encoding` to gzip, deflate and identity. The router can
  read a copy of the answer in those, so it can count the answer as it streams.

The router swaps the token only on a request that already carries the token of one of your
accounts. So a program on your Mac that holds no token of yours can't borrow one.

What the router never touches:

- **The body of a request.** The request stays the one Claude Code built, so Claude's thinking
  stays valid from one turn to the next.
- **Every other request,** such as a file upload. It passes through as it came, on the token
  Claude Code holds: the token of the [primary](glossary.md) in its pool.

On every request, the router also takes off switchboard's own headers, named `X-Switchboard-…`, so
Anthropic never sees one. `switchboard run` uses them to pass a pin, and the folder a session
started in.

## Reading the usage

Every answer from Anthropic carries headers that say how full each window of the account is, and
when each resets:

- the week that every model shares;
- the 5-hour window, on Pro and Max plans, though verified Ultra plans don't have one;
- a week of its own for some models, such as Fable.

The router reads these off every answer, so knowing the usage costs nothing extra. Only a 429 that
refuses the request itself carries none. Each answer gives the account's whole usage, from
wherever it was used: your phone, claude.ai, or Claude Code run without switchboard.

Switchboard learns which windows an account has from the answers, never from the plan. If the
answers stop reporting a 5-hour window, the router drops it for that account and logs
`no longer reports the window`. The dashboard then says `no 5-hour limit`. If the window comes
back, the log says `reports the window again`.

To read an account with no traffic, the router probes it. A probe is one request for each model
family, Haiku and Fable, each capped at one output token. The router probes an account in these
cases:

- nothing has read the account yet, for example as the router starts;
- nothing has read it in the last 5 minutes, or its week resets in the next 5;
- a decision, or a dashboard you opened, needs fresher numbers;
- the account is due a prime.

A probe starts a 5-hour window that has lapsed, so the router rarely probes such an account. The
full rules are in [Probes and priming](reference/probes-and-priming.md).

## What skips the router

Some of what Claude Code does never reaches the router, or passes through untouched. In a routed
session, that goes out on the token Claude Code holds, the primary in its pool:

| What | What happens |
|---|---|
| Fast mode | It ignores `ANTHROPIC_BASE_URL`, so it isn't routed. |
| WebFetch's site checks | They ignore `ANTHROPIC_BASE_URL` too. They're small. |
| `/usage` and `/status` in Claude Code | They report the primary. |
| Remote sessions and file uploads | They pass through untouched. |
| Extra usage, if turned on | Anthropic may bill the request that crosses a limit. |

Switchboard doesn't support fast mode. For the accounts the conversation went to, run
`switchboard usage` or `switchboard status --session`. With extra usage, Anthropic may serve the
request that crosses a limit rather than refuse it. The router moves sessions off once the headers
show the window spent.

Other sessions never go through the router's choice at all:

| What | Where it goes out |
|---|---|
| `switchboard run --direct` | On Claude Code's own login, whatever it's signed into. |
| `when_router_down = "direct"` | On an account of its pool, without the router. |
| `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` set | On the key, as if switchboard weren't there. |
| A program whose `PATH` lacks the link | On Claude Code's own login. |
| An app with a Claude Code of its own | On Claude Code's own login. |

A session that goes direct can spend the reserve of the account it goes out on. A launchd job is
one program that may lack the link on its `PATH`. The Claude desktop app's Code sessions and an
editor's bundled Claude Code never look on `PATH`, so nothing can route them.

## A session outside switchboard

`setup` adds a hook to Claude Code that runs as every session starts or resumes. If switchboard
didn't start the session, the hook shows a line that begins
`This session isn't going through switchboard`. That session's requests go straight to Anthropic
on Claude Code's own login, unrouted and unrecorded.

| If you started it | Then |
|---|---|
| From a terminal | Run `switchboard setup`, then start the session again in a new terminal. |
| From an app with its own Claude Code | It can't be routed, and `setup` won't change that. |

From a terminal, something put the real `claude` ahead of the link, such as a changed startup
file. The hook tells you, not Claude. A message to Claude would go into every prompt of a headless
`claude -p` run, such as one from cron.

The hook routes nothing and changes nothing. It always succeeds at once, so it never holds a
session up, and does nothing once switchboard is gone. A session that `switchboard run --direct`
starts belongs to switchboard, so the hook stays quiet.

## Where each part is covered

- **Routing and limits.** A new session goes to the account most at risk of going unused, and
  moves when a limit hits. See [Limits and routing](limits-and-routing.md), and the full rules in
  [Routing rules](reference/routing.md).
- **Pins.** A pin sends sessions to an account you choose. See
  [Pinning and moving](pinning-and-moving.md).
- **Pools.** A pool ring-fences a set of accounts to one client's folders. See [Pools](pools.md).
- **The primary and reserves.** A reserve keeps part of every window free for the Claude app. See
  [Room for the Claude app](room-for-the-claude-app.md).
- **Staggered resets.** Accounts with a 5-hour window can reset one after another. See
  [Staggered resets](staggered-resets.md).
- **Your record.** Every request without its content, and what it was worth. See
  [Your record](your-record.md) and [Several Macs and backups](several-macs.md).
- **Claude and switchboard.** A skill teaches Claude to read the limits and pin. See
  [Claude and switchboard](claude-and-switchboard.md).
- **Upgrades and restarts.** The router restarts itself, and refuses no request. See
  [Upgrades and restarts](upgrades-and-restarts.md).
- **Everything else:** [Commands](commands.md), [Configuration](configuration.md),
  [Reference](reference.md) and [Troubleshooting](troubleshooting.md). Contributors will find
  every rule and its reasons in [the design](design.md).
