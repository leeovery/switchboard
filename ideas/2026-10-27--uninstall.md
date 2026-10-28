# An uninstall

A command that takes away everything switchboard put on the Mac, the way `setup` put it there, so
nothing it leaves behind keeps working on the user's behalf or breaks their shell or Claude Code.
Today only `service uninstall` exists, which stops the router and removes the LaunchAgent.

The owner asked for it on 27 October 2026, as setup took on the shell's startup line and the session
hook: removing the hook is part of it.

What setup puts in place, each for the uninstall to take away:

- the LaunchAgent, as `service uninstall` does, first, so no router brings the skill back up to
  date as it goes;
- the line setup added to the shell's startup file, by its mark, and the file of switchboard's own
  it hands off to;
- the session hook in Claude Code's settings, its own entry alone, every other setting left as it
  is;
- the skill, in Claude Code's config directory;
- the `claude` link in switchboard's bin directory.

To settle with the owner:

- whether the config, the tokens and the recorded history (the ledger, the readings, the events and
  the logs) go too, or stay unless asked;
- what it says of the accounts' setup tokens, which stay valid on Anthropic's side until their year
  is out;
- what a `brew uninstall` without it leaves: the startup line does nothing once its file is gone,
  and the hook nothing once switchboard is, so neither breaks the shell or a session, but both stay
  until removed; the formula's caveats could name the uninstall.
