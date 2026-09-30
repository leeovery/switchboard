# An artifact proxy

Serve an account's artifacts locally, so one browser sees every account's, wherever the session
that published one ran.

It hinges on whether a setup token can read an artifact through the API the artifact tool uses,
and claude.ai's live features wouldn't work through it. The primary account makes it less needed:
its token is every Claude Code's own, so a session's artifacts go out on the primary whichever
account its conversation is on.

Moved here from the design's backlog.
