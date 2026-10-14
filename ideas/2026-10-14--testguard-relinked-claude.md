# testguard and Claude Code's relinked `claude`

Claude Code's updater re-creates the link at `~/.local/bin/claude` every so often, during long runs
too, often to the very target it had. testguard sees anything named `claude` on the real `PATH`
change and fails the run, though every test passed and the sandbox denies tests any write into the
home. It fails whole gate runs, the release gate's included, several times a day.

Until this is settled, a run whose only failure is testguard naming that link, with no `--- FAIL`
anywhere, is counted as passed by hand.

Ways out, keeping the guard:

- compare a link's resolved target and the file it leads to, not the link's own `lstat`, so a link
  re-created to the same target is no change, and one to a new target still fails the run;
- or pause Claude Code's updates while the gates run.

Never loosen the guard further than that: a `claude` the tests made or changed must still fail it.
