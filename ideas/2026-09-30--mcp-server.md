# An MCP server

An MCP server could expose switchboard to agents as tools: every account's usage, the account a
session is on, pinning.

Not planned unless the command line falls short. The agent interface today is the command line:
`switchboard status --json` prints the status document, and the skill tells Claude when to run it,
and the rest. What would earn a server its place is what a command can't do well, such as
streaming live events to an agent as they happen: a session moving, a limit reached, an account
coming under pressure.
