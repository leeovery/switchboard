# History kept beyond one Mac

Switchboard's history, the request ledger, the readings history and the router's events, lives in
its state directory on one Mac. A backup of that Mac keeps it for that Mac, but nothing brings it to
a new one. The owner agreed on 27 October 2026 that switchboard keeps its history safe beyond any one
Mac itself: the tool that restores the owner's settings on a new Mac keeps settings in a git
repository, the wrong place for data that grows by 20–38 MB a day before it's compressed. The
owner's next Mac arrives in about four weeks, and is to have this Mac's history.

What stays as it is:

- The config file is the user's, written by hand, and theirs to keep.
- `prefs.json` is a settings file, which a tool that restores settings can carry like any app's.
- Tokens are never backed up: `setup` makes them on each Mac (see
  `2026-10-14--auth-and-onboarding.md`).

## Suggested, to settle with the owner

- **Where:** iCloud Drive, built into macOS, so every Mac on one Apple ID has it from its first login,
  with nothing to install. A folder of switchboard's there, and in it one for each Mac.
- **How:** never the live state in the synced folder. Switchboard keeps working in its state
  directory, and copies a day's finished files there once they're compressed and never change again,
  so each Mac writes only its own folder and no file is ever written twice: no sync conflicts. A
  day not yet compressed is the Mac's own backup's to keep.
- **Size:** a compressed day comes to about 4–8 MB, about 2–3 GB a year.
- **Restore:** on a new Mac, `setup` offers to bring a Mac's history back.

To settle: which files go (the ledger's own lines, or its derived files too, which can be built
again); how a Mac is named in the folder, never by its machine name in anything switchboard shows
publicly; how the copying is kept, as the router's housekeeping after a day compresses; what a
restore does with days the new Mac has already, as when both Macs run through the same days; and
what's said when iCloud Drive isn't there, or is full.
