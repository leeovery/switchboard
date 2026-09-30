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
