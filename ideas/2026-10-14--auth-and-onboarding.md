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
