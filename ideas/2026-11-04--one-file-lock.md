# One owner of switchboard's file locks

Switchboard takes a `syscall.Flock` on a file in four places, each with its own few lines of open,
lock, retry on an interrupted call and release: the logs (`internal/logs`), the token files
(`internal/tokens`), an import's lock (`internal/export`), and the router's copy, which `service
install` and a router moving over to its copy share (`internal/routercopy`). Found as the router's
copy was built, 4 November 2026.

Suggested: one small package owning a file lock, blocking or trying without waiting, its file left
in place, that the four use. A refactor alone: nothing a user sees changes.
