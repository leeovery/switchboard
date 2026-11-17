# Completions for a shell that already reaches switchboard's `claude`

Setup offers the shell line only where the shell runs another `claude` (see Setup's claude link in
the design). The line hands off to switchboard's own file, which puts its `claude` first and loads
its completions. A shell that already reaches switchboard's `claude` some other way, as through a
`PATH` kept in another file, is offered nothing, so it gets no completions.

## To settle with its owner

Whether setup should offer the line for completions alone in that case, saying so: the line as it
is, with a reason that names the completions and not the `claude`. Or whether those users add the
line themselves, as the by-hand page shows it, and setup leaves them be.

Found on 17 November 2026, reading the design's claude link step.
