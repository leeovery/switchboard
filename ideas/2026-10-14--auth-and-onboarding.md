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
