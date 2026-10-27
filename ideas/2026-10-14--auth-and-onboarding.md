# Signing in, one account, and a first setup

Switchboard asks for a setup token for each account, made with `claude setup-token`: a token that
lasts a year, so nobody signs in again. It was built that way because its owner works that way. A
review of that choice, to discuss with the owner after milestone 7's release (raised 14 October
2026).

## Signing in

- Many people sign in to Claude Code with their claude.ai login (OAuth) rather than a setup token,
  and switchboard should still serve them. `2026-09-30--oauth-logins.md` is one way: switchboard
  holds each account's login and refreshes it before it lapses.
- With several accounts, how switchboard would take each through its login is open. The owner's
  experience of OAuth across several accounts: signing in again, often, on every one of them.
- Switchboard could recommend setup tokens, to people with several accounts and with one: a year
  without signing in.

## One account

Switchboard should be worth running with one account. There, the router routes nothing, and its
worth is what it records and shows: the request ledger, the history, sessions, the dashboard, and
Claude reading its own usage limits.

That last one isn't said loudly enough. On 13 October 2026 the owner asked Claude to pace its
work so an account's week would be spent by its reset at 08:00 the next morning, without reaching
100% first. Claude read the window's use, the recent rate, when it would run out at that rate and
when it reset, from switchboard, and paced its agents by them. Without switchboard, Claude couldn't
have done that accurately. `2026-10-14--usage-for-agents.md` makes it easier still.

## A first setup

Setting switchboard up for the first time should be quick and slick: onboarding that gets someone
from install to routed in moments.

- It could tell whether someone signs in with a login or a setup token from their environment:
  Claude Code reads a setup token from `CLAUDE_CODE_OAUTH_TOKEN`, whatever the person's own
  variable is called before it, and keeps a login in the keychain. To check. Failing that, it asks.
- Then, for each account, the shortest path to a working token.

To settle with the owner: whether switchboard takes logins at all, or recommends setup tokens and
helps make them; and what `setup` asks, detects and does for one account and for several.

## Each account's plan

Nothing asks for an account's plan: `plan` is an optional config key, so most people will have
none, and see `-` for the plan and no worth against it on Accounts, History and in `history`.

- Switchboard can't read the plan. Responses tell each window's use and reset, never the plan or
  the size of a limit, and a setup token is refused by the profile endpoint that knows it (403).
- A login (OAuth), where Claude Code reads the plan, might be allowed to: to check, if switchboard
  takes logins.
- Otherwise `setup` should ask for each account's plan as it adds the account, and say what it's
  used for.

## Managing accounts after setup

Adding, updating and removing accounts should be as easy as setting up: changing an account's
label, plan, primary or reserve, replacing its token, adding one or taking one away, without
editing the config by hand. `accounts add`, `accounts token` and `accounts remove` exist; nothing
updates an account. Raised by the owner on 15 October 2026, who wants this, and `setup` asking
for the plan, to follow milestone 7's release: "users wouldn't know to put that in".

## The owner, 26 October 2026

Next to discuss, after the cleanliness release: making switchboard work with Claude's OAuth logins, and the setup process. First to find out: whether Claude Code and switchboard refreshing one login lock each other out, as single-use refresh tokens would.

## Decided, 27 October 2026

The owner chose, from research that day (`2026-09-30--oauth-logins.md` says why logins aren't
wanted):

- **No logins.** Switchboard holds no claude.ai login: its one credential is the setup token, a year
  long and never refreshed. Someone who signs in to Claude Code with a login can still use it, as
  setup makes them a token; their login stays Claude Code's own, for what goes direct.
- **Setup makes each token.** `setup` runs `claude setup-token` itself, on a pseudo-terminal, and
  takes the token from what it draws, so the person approves in the browser and copies nothing.
  Pasting a token stays, for where that fails.
- **The browser, an account at a time.** For each account, setup says which the browser must be
  signed in to, and gives the address to open in a private window, so the browser's own session
  is left alone. Whether `setup-token` can be told the account, as `claude auth login --email` can,
  is to check.
- **What setup asks.** Each account's plan, Pro, Max 5x or Max 20x, saying what it's for, and its
  reserve. It catches an account added twice, by the organization id the API answers with: reported
  of setup tokens, to check.
- **After setup.** `accounts edit` changes an account's label, plan, primary or reserve, and
  `accounts token` renews its token. Switchboard keeps when it took each token, and warns weeks
  before its year is up.
- **No pass-through.** Claude Code's own login isn't let through for the account it's signed in to:
  one approval in setup gets someone with one account to the same place, and the router keeps one
  kind of account.

### Setup's flow

The owner agreed it on 27 October 2026, step by step. Setup's other steps stay as they are, but for
the service's, below.

- **A new account:** its id and label, as now. Then its token, which setup makes: it runs `claude
  setup-token` on a pseudo-terminal wide enough that the token is drawn whole, with `BROWSER` set
  to switchboard for that run, so switchboard is handed the sign-in address. The first account's
  opens in the browser as it is; each after opens in a private window, which setup opens itself
  where the default browser allows (the Chrome family, Firefox), and otherwise prints the address
  to paste into one (Safari). Over SSH it opens nothing: it prints the address setup-token shows for
  pasting, to open on any device, and asks for the code that page gives, which it passes to
  setup-token. It takes the token from setup-token's screen once it says it's made, checks it with
  the API as `accounts add` does, and saves it, saying it's good until a year from then. `p` pastes
  a token instead, and where reading the screen fails, setup shows setup-token's own and pasting
  goes on as before. Then its plan: Pro, Max 5x or Max 20x, saying History prices its use against
  it; Enter leaves it unset.
- **After the accounts:** the primary, as now, then its reserve: the share of its windows the router
  leaves for the Claude apps, as a percentage; Enter for none. Other accounts' reserves are set with
  `accounts edit`.
- **Run again, as on a Mac restored with its config and tokens in place,** setup reviews each
  account, a line each: its token and how long it's good for, its plan or that it has none, whether
  it's the primary, its reserve. Enter keeps it; `e` edits it, `t` makes a new token, `r` removes it,
  asking first. Each token is checked with the API as it's reviewed, so one refused shows here rather
  than at the first request; one the API doesn't answer for is kept, saying so. Nothing is asked of
  an account already set up, as its plan: its line says what's missing, and `e` sets it. Then "Add
  another account?", then the other steps. Enter all the way through changes nothing.
- **The service:** setup also reinstalls it where the plist differs from what `service install`
  would write now, keeping the options it was installed with, such as `--log-level`, and says what
  differed. Nothing asks whether to reinstall: `service install` stays the way to force one.
- **A token's year:** switchboard keeps when it took each token, beside the token, so a restore
  brings it back too. A pasted token's year counts from the day it was given, as nothing says when
  it was made. Accounts shows a line once a token has under 30 days left, a notification goes out
  at 30 days and at 7, and `usage` and `status --json` carry the date.
- **`accounts edit <id>`** asks each of label, plan, primary and reserve in turn, showing what it is,
  or takes them as flags. **`accounts token <id>`** makes a new token as setup does, or takes one
  pasted or on stdin.

Next: the build, in small slices.

### What other tools do (27 October 2026)

TeamClaude, better-ccflare, CLIProxyAPI, claude-relay-service and sub2api, claude-swap and clauth all
spread Claude Code across several subscriptions. Most hold logins: they run Claude Code's own
sign-in, under its client id, with a localhost callback and a pasted code as the fallback; refresh
ahead of expiry, one refresh at a time an account; and read each account's identity and plan from
the profile endpoint. Their trackers show the cost: logins lapsing monthly, accounts lost to a
refresh token sent twice, lockouts from sharing Claude Code's own login. None gets the browser onto
each account in turn: each takes whatever account the browser is signed in to.
