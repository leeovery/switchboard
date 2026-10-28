# OAuth logins in place of setup tokens

Each account signed in with its claude.ai login, as a browser is, rather than given a setup token,
with switchboard keeping every account's refresh token fresh, refreshing each before it lapses, so
no login ever does. It would take away the chore of logging in again, which is why setup tokens
are used today.

Later, not now; moved here from the design's backlog.

## Not wanted

The owner, on 27 October 2026, who wouldn't use them: a login means signing in again all the time.
Research that day found why, and that no refreshing could prevent it:

- A claude.ai login lapses about 30 days after it's signed in. Refreshing doesn't move that; only
  signing in again does, and Claude Code warns three days before (its docs: "Renew an expiring
  login"). With several accounts, that's a sign-in every few weeks for each.
- Each refresh hands out a new refresh token and refuses the old one at once. Two holders of one
  login, as switchboard and Claude Code would be, lock each other out, and one refresh token sent
  twice, as a retry after a timeout does, loses the login: other multi-account tools have lost
  accounts that way (CLIProxyAPI #6288, October 2026).
- Claude Code's legal and compliance page, read on 27 October 2026, says tools may not "collect,
  store, or intermediate Claude.ai credentials or session tokens", and that signing in "must
  complete through Anthropic's own flow". A setup token, which Claude Code's own `claude
  setup-token` makes, is the documented way for scripts.

Switchboard keeps the setup token as its one credential, and makes it painless instead: see
`2026-10-14--auth-and-onboarding.md`.
