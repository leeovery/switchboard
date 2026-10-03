# Intercepting traffic that ignores ANTHROPIC_BASE_URL

A local certificate authority, trusted by the Mac, would let switchboard see the traffic that
doesn't honour `ANTHROPIC_BASE_URL`. What it would catch is listed in the design's "What doesn't
go through the router".

Not planned: a trusted local CA costs more, and risks more, than what it would catch. Moved here
from the design's backlog.
