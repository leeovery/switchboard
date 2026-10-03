# Other agents than Claude Code

Claude-specific knowledge sits behind a provider interface (`internal/claude`, behind
`router.Provider`, `router.Prober` and `status.Prober`), so another coding agent used with several
subscriptions could be routed the same way without reshaping the rest.

Deferred since the design began; moved here from the design's backlog.
