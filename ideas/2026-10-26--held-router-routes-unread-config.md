# New sessions through a held router while the config can't be read

Found by the cleanliness release's review, 26 October 2026. To land before any release tightens a
config rule.

## What happens

A restart held after an upgrade is mostly a newer binary refusing the config the running router
took. The CLI is that newer binary too, so it can't read the config either, and each `claude`
started then, finding the config unreadable, starts unaided, as #178 settled: outside a session,
unrouted and unrecorded, spending the primary past its reserve, though the router runs on and
would route it. `status` shows the router's document beside the config's error (built in the same
review's fixes), but nothing routes the sessions started meanwhile.

## Why it matters

The held restart exists to keep routing going through an upgrade whose rules refuse the config.
It keeps the sessions already running routed, but every session started until the user notices
and edits the config goes unrouted, which is the outcome the hold was built to avoid. Each config
rule a release tightens, as the label's 100 characters (#142) did, meets it.

## Ways it could go

- **Ask the router first:** where the config can't be read, but the router's state directory can
  be found, as it can without a config, `run` asks the router, and while it answers healthy, routes
  through it, saying on stderr that the config can't be read. Only where the router doesn't answer
  does it start unaided. Which token Claude Code starts on, with no config to name the primary, is
  to settle. This changes #178's rule that a config that can't be read means unaided, so it's the
  owner's call.
- **Read the config leniently for `run`:** the launcher needs only the accounts and
  `when_router_down`; a key it doesn't know, or a rule only the router holds, needn't stop it.
- Leave it, and give each tightened rule a line in its release's notes.
