# The dashboard's layout

The graphs work well. Three things don't:

- **Narrow terminals in watch mode.** `usage -w` falls back to the compact layout on a phone-sized
  terminal, where a one-shot `usage` at the same size shows the full cards with their graphs. The
  full cards are the ones worth seeing.
- **The top section.** The heading packs the router's state, its sessions, the pin, best next and
  more into a line or two that's hard to take in at a glance.
- **More dynamic.** The dashboard could show more of what's happening as it happens: sessions
  moving, an account under pressure, a limit reached, a restart due.

Next up after the 5-hour pressure work; to be designed with its owner before anything is built.
