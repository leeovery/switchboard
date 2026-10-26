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

## Built, 26 October 2026

The first way, chosen by its owner on 26 October 2026, built in the cleanliness release (see the
design's The router looking after itself):

- **The check:** before every restart it makes, and before one `service restart` asks for, the
  router runs the binary it's about to become as it would run it, with `--check` added, a hidden
  flag of `serve` that loads what `serve` would start from, the config above all, and exits:
  0 when it could start, and 78, with why on its standard output, when it couldn't. A flag the
  service passes that the new binary no longer takes is a refusal too. The protocol is
  `internal/handover`'s (`Ask`, `CheckFlag`, `Refused`), as the listeners' variable is.
- **Held only on a refusal:** any other answer is restarted into as before: a binary from before
  the check, which exits 1 at the flag it doesn't know, one that crashes, and one that takes more
  than 10 seconds. A binary that isn't there, as for a moment mid-upgrade, isn't asked: the
  restart waits until it's back, and `service restart` meanwhile is refused.
- **An answer clears the restart** for the config file and the binary as they stood when it was
  asked: a request begun while the binary answers puts the restart off to the next quiet moment,
  which asks again only where either file differs by then. An answer given while either file
  changed, refusing or not, clears and holds nothing, and `service restart` is refused then, to be
  tried again.
- **While it's held,** the old router serves on, and says why once: at warn in its log, in a
  `restart` event with `held`, and in the status document's `restart.held`. It asks again only
  once the config file or the binary moves on, there and another file or changed: one gone for a
  moment, as `brew` relinks the binary or an editor saves the config, hasn't. `service restart`
  meanwhile is refused with the binary's reason.
- **How it's worded,** as the owner chose on 26 October 2026: the Log's Events `held: <reason>`;
  LATELY `restart held: <reason>`; `status`'s text `restart due since Mon 14:02 (upgraded), held:
  <reason>`, with no `service restart` advice; and the dashboard's status line `restart held
  (upgraded)`.
- **Its event's card** in the Log follows those words: headed `restart held`, with a `held` item
  giving the reason.
- **Left:** the running router still judges a config edited after an upgrade, before the
  handover, by its own rules. So it refuses an edit only the new version takes, and the restart
  waits until the config suits the old rules again. A fix would let the new binary's answer decide
  once the binary has moved on. Not built.
- **Left:** while the binary isn't there, the status document shows the restart due with nothing
  saying it waits for the binary: a moment mid-upgrade, but for good after a `brew uninstall`
  with the service still running. The words for it are the owner's to choose.
