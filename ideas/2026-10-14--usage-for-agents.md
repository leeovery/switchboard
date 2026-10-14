# Usage limits for agents, read and watched

A verb, or flags on one, for an agent to read and watch the usage limits as its own work moves them,
with nothing left for it to work out.

- **Read:** for each account and window, the use now, the recent rate, when it runs out at that
  rate, and when it resets; and the pace that would bring a window to a chosen share by its reset,
  so an agent can size its work. Plain fields, read as given, with no sums of the agent's own.
- **Watch:** a stream, a line per change that matters: the rate rising, the run-out moving before
  the reset, a threshold crossed. Pressure and run-outs follow recent use, so they move as agents
  start and stop; an agent following the stream can pace itself as its work changes.

`usage --json`'s `pressure` and `rates` already hold most of this, but finding and reading them took
interpretation. The watch is what `2026-09-30--mcp-server.md` names as what would earn a server its
place; a stream on the command line may do instead.

Raised by the owner on 13 October 2026, beside `2026-10-14--advice-from-the-ledger.md`.
