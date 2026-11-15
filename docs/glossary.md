# Glossary

The words switchboard uses in a sense of its own. Every page uses them in this sense and no other.

**Account.** One Claude subscription that you've added to switchboard. You see it by its label,
such as `work`, and commands name it by the id switchboard gives it, such as `b6s7`.

**Cap.** The point where an account's reserve begins. With a reserve of 10%, the cap is at
90% of each window.

**Direct.** A session that starts without the router. `switchboard run --direct` starts one on
the account Claude Code itself is signed into. If you set `when_router_down = "direct"`, a session
started while the router is down goes direct on its pool's primary.

**Fence.** The folders a pool is tied to. A session belongs to that pool if it starts in one of
those folders, in a folder beneath one, or in a git worktree of a repository inside one.

**Label.** The name you give an account or a pool, shown on every screen. You can change it
whenever you like.

**Ledger.** Every request the router handles, kept without its content in a file a day. The ledger is
part of the record, kept for 400 days unless you set another length.

**Limit.** The most of a window that Claude lets an account use. An account at a limit can't take
more requests in that window until it resets.

**Open accounts.** The accounts new sessions can go to right now: every account in the pool, or
only the pinned ones while a pin holds. The dashboard and `usage` still call these the pool, for
example in `Σ pool`. This term replaces that second meaning of the word.

**Pin.** Your choice of account for new sessions, or for one running session, in place of the
router's choice. A pin gives way when its account hits a limit.

**Pool.** A set of accounts that sessions are routed across. Every account is in exactly one
pool, and nothing crosses from one pool to another. See [Pools](pools.md).

**Primary.** The account whose token Claude Code holds in a pool. Each pool has one. The router
picks the account for each conversation. What Claude Code sends outside the conversation goes out
on the primary, for example artifacts, uploads and WebFetch's site checks. So artifacts open in a browser
signed into the primary.

**Priming.** Starting an account's 5-hour window at a time switchboard chooses, so your
accounts reset one after another rather than together.

**Project.** The repository a session runs in, with its worktrees folded in. A session in a
worktree or a folder beneath the repository counts as the repository's. A session started outside
any repository counts as its own folder's. A `claude` that another session starts outside any
repository counts as that session's project, while it's in that session's pool. `run` records the
project as the session starts. See [the ledger](reference/ledger.md).

**Record.** Everything switchboard keeps of your use, on this Mac and from any you import:

- the ledger of requests;
- the readings of each window;
- the router's events;
- a summary of each day.

**Reserve.** A share of every window that the router leaves unused on an account, for you to use
outside switchboard, for example in the Claude apps. The router never spends it, but a pin
or a direct session can.

**Room.** How much of a window an account has left before its limit or its cap.

**The router.** The program on your Mac that Claude Code sends every request through. It picks
the account, swaps in its token, and records the request. The service keeps it running.

**Session.** One conversation in Claude Code, from start to exit. A resumed session carries on the
same conversation, but starts a new process.

**The service.** The LaunchAgent that runs the router at login and restarts it when needed.

**The unfenced pool.** The pool with no fence. A config holds at most one, and it takes every session
started outside all the fences. If every pool is fenced, a session started outside them doesn't
start.

**Window.** A span of time Claude counts usage over. Each account has a week. Pro and Max
accounts have a 5-hour window too. A verified Ultra account doesn't. Some models also have a week
of their own. When a window resets, its usage starts again from zero.

**Worth.** What a request would have cost at Anthropic's API prices. Switchboard sets it against
what your plans cost.
