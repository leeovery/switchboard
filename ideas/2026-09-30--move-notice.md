# A notice when a session moves

A `UserPromptSubmit` hook that shows a line in Claude Code's TUI after the router has moved the
session to another account, and gives Claude the same line as context. It's never written into the
conversation itself, which the thinking check rules out: the API refuses a conversation changed
before a thinking block.

Deferred since milestone 3's design; moved here from the design's backlog.
