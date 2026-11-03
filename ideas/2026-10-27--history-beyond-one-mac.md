# History kept beyond one Mac

Switchboard's history, the request ledger, the readings history and the router's events, lives in
its state directory on one Mac. A backup of that Mac keeps it for that Mac, but nothing brings it to
a new one. The owner agreed on 27 October 2026 that switchboard keeps its history safe beyond any one
Mac itself: the tool that restores the owner's settings on a new Mac keeps settings in a git
repository, the wrong place for data that grows by 20–38 MB a day before it's compressed. The
owner's next Mac arrives in about four weeks, and is to have this Mac's history.

## Taken up, 3 November 2026

The owner chose an export and an import first, then the export on a schedule, into a folder the
user chooses, iCloud Drive, Dropbox, an external disk, anywhere: switchboard runs the export and
nothing more, and what keeps the folder safe, or carries it elsewhere, is the folder's own
business. The design's Export and import says how; what was measured is there too.

What stays as it is:

- The config file is the user's, written by hand, and theirs to keep.
- `prefs.json` is a settings file, which a tool that restores settings can carry like any app's.
- Tokens are never backed up: `setup` makes them on each Mac (see
  `2026-10-14--auth-and-onboarding.md`).

## Left: one history across several Macs

Not now, the owner's call on 3 November 2026: one history read across several Macs at once, each
Mac's History counting the others' requests. It's only worth it where two Macs run switchboard on
the same accounts at the same time, as each records only the requests it routed, and History,
worth and the totals on either would count short.

Suggested, should it come up: each Mac exports on the schedule into one `[export] dir`, a folder a
Mac as they do already, and each Mac's readers read the other Macs' folders beside its own state
directory, never writing them. The others' days run as far behind as their exports, today's an
hour at most, and the readings of one account, read by both Macs, are one account's however many
Macs read them.

Later, too: `setup` on a new Mac offering to import a folder.
