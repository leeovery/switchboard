# A config checked under the new binary's rules before a handover

Found on 23 October 2026, in the release review after the fluid layout pass, which brought the
first rule a config could break across an upgrade: a label of at most 100 characters (#142). Its
owner chose to keep the limit, with a line in the release notes, and this idea for what's left.

## What happens

When the binary moves, as an upgrade moves it, the router hands over to the new one once nothing
is in flight. Before it does, it checks that its config file makes a valid config, so the router
it hands over to can start from it (`restartable`, and `look` before that). But it checks under
its own rules, the old binary's. A config the old binary takes and the new one refuses passes the
check:

- **The new router** fails to load the config and stops, and launchd starts it again, and it
  fails again, for as long as the config stands.
- **Each `claude`** says on stderr that it couldn't read the config, and starts unrouted: its
  requests go on one account, unrecorded.
- **The dashboard and the verbs** fail on the config too.

Editing the config to the new rules puts it right at once.

## Why it matters

Rarely: it takes a config the old rules took and the new ones don't, and #142's limit is the
first such rule. But routing is switchboard's first job, and an upgrade is the moment a user is
least likely to be watching for it.

## Ways it could go

- **Ask the new binary before handing over:** the old router runs the new binary to check the
  config, with a verb it would need, such as `switchboard config check`, and hands over only
  when it passes; when it doesn't, it carries on as it is, says why in its log and its health,
  and `status` shows the restart held.
- **Have the new binary take what the old one took:** a rule that tightens reads an old config
  leniently, as cutting an over-long label with a warning, and holds the rule only where a
  config is written, as in `accounts add`.
- Leave it, and give each tightened rule a line in its release's notes, as #142's has.

Not decided; to discuss.
