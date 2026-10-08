# The dashboard's layout

The graphs work well. Three things don't:

- **Narrow terminals in watch mode.** `usage -w` falls back to the compact layout on a phone-sized
  terminal, where a one-shot `usage` at the same size shows the full cards with their graphs. The
  full cards are the ones worth seeing.
- **The top section.** The heading packs the router's state, its sessions, the pin, best next and
  more into a line or two that's hard to take in at a glance.
- **More dynamic.** The dashboard could show more of what's happening as it happens: sessions
  moving, an account under pressure, a limit reached, a restart due.

The plain text `switchboard status` prints is to be redesigned alongside it: its owner finds it a
mess. It's for people alone now, as agents read `status --json`, which the skill points them to,
and `usage` prints the same where its output isn't a terminal; so the text can change as freely as
the dashboard, where the status document only ever gains fields.

Next up after the 5-hour pressure work; to be designed with its owner before anything is built.

**The dashboard's part is taken up:** designed with its owner in Paper over five rounds and signed
off on 2 October 2026 as milestone 5, then redesigned as milestone 7, in the design's Dashboard
section. What's left here is the plain text the data verbs print on a terminal: `status`'s; the
first cuts of `requests` and `history`, which exist; and those of `sessions` and `events`, which
milestone 7's stage 3 adds: all to be redesigned in the dashboard's words, with its owner.
