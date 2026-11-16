# A refused read's hold in the Log

Since #351 the router holds an account back while Anthropic refuses its read, as when its
subscription has ended or its token is refused, until a read comes back clean. The status document
states the hold, as the account's `RefusedRead`, and the cards and `status` say why. The Log
doesn't. A session moved off a held account reads there as any other leaving one without room:
`moved off work: has no room`, as the router's `unable` falls back to it when no request went
wrong on the account and no reserve holds it. Nothing in the Log says when the hold began, why,
or when it lifted, so looking back, a hold reads as a limit that never came.

## What it could do

- **An event when a hold starts,** naming the account and the cause as the cards word it:
  `subscription lapsed` or `token refused`. It joins the Log's Events, LATELY and `switchboard
  events`, as `refused` and `limit` do.
- **An event when it lifts,** as a clean read comes back, as `open` tells of room again.
- **The cause on the moves it makes:** a session moved off a held account says why, such as
  `moved off work: subscription lapsed`, in place of `has no room`.

## To settle

- Whether the start is a new kind of event, or the `refused` event the router writes when the
  upstream refuses requests on an account, with the read's cause beside it.
- Whether a notification follows, as limits and room again have one.
- A hold found at a restart, carried in the state file: whether the router tells of it again.

Found with #351 on 16 November 2026.
