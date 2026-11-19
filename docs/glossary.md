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

**Hold.** The router sends an account nothing because Anthropic refused its last usage read. The
cause is `subscription lapsed` or `token refused`. A hold lasts until a read of the account comes
back clean, or the account gets a new token, and no time ends a hold. The Log shows a hold starting
as a `held` event and ending as `open`.

**Label.** The name you give an account or a pool, shown on every screen. You can change it
whenever you like.

**Lapsed.** A 5-hour window that has passed its reset with nothing read since. It reads empty,
because it isn't running until a request starts it. Only the 5-hour window lapses: a week past its
reset keeps its last reading until the next one. An account whose subscription has ended shows
`subscription lapsed`, or `lapsed` if space is short, which is not this.

**Ledger.** Every request the router handles, kept without its content in a file a day. The ledger is
part of the record, kept for 400 days unless you set another length.

**Limit.** The most of a window that Claude lets an account use. An account at a limit can't take
more requests in that window until it resets.

**Pin.** Your choice of account for new sessions, or for one running session, in place of the
router's choice. A pin gives way when its account hits a limit.

**Pool.** A set of accounts that sessions are routed across. Every account is in exactly one
pool, and nothing crosses from one pool to another. The `Σ pool` row, Runway's `Σ pool` lane and
the POOL chart sum the room of the pool's accounts. While the pool's pin is set, they sum only the
pinned accounts. See [Pools](pools.md).

**Primary.** The account whose token Claude Code holds in a pool. Each pool has one. The router
picks the account for each conversation. What Claude Code sends outside the conversation goes out
on the primary, for example artifacts, uploads and WebFetch's site checks. So artifacts open in a browser
signed into the primary.

**Priming.** Starting an account's 5-hour window at a time switchboard chooses, so your
accounts reset one after another rather than together.

**Project.** The repository a session runs in, with its worktrees folded in. A session in a
worktree or subfolder of the repository counts in that repository. A session started outside any
repository counts in the folder it started in. A session in a Claude Code scratchpad counts as the
project the scratchpad was made for. A session in any other temporary folder has no project: it
counts under `temporary folders`. A `claude` another session starts outside any repository joins
that session's project instead, if both run in one pool. `run` records the project as the session
starts. See [the ledger](reference/ledger.md).

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

**Scratchpad.** The temporary folder Claude Code makes for one session, named by the slug of the
folder the session started in. Tools often start `claude -p` there.

**Session.** One conversation in Claude Code, from start to exit. A resumed session carries on the
same conversation, but starts a new process.

**Session name.** What Sessions' List, Projects and the session page's heading call a session:
the name of the terminal session it started in, read when `claude` starts. A session started
outside a terminal session takes the name of the Claude Code worktree it runs in. Else the
session goes by its id, as much as tells it apart from another session's, at least 4 characters.
See [Sessions](dashboard/sessions.md).

**The service.** The LaunchAgent that runs the router at login and restarts it when needed.

**Slug.** The name Claude Code makes from a folder's path, with every character but an ASCII
letter or digit turned into `-`. For example, `-Users-you-Code-api` is the slug of
`/Users/you/Code/api`.

**Temporary folders.** The row of the Projects list for sessions that have no project:

- sessions started in a temporary folder outside a scratchpad, such as one under `/tmp`;
- sessions started in a scratchpad whose slug names nothing a project can be: a repository since
  removed, a volume that isn't mounted, or a folder above your home folder.

It isn't a project. In `sessions --json`, these sessions give `temporary folders` as `project`,
which isn't a path.

**Terminal session.** A named session of zmx or tmux, which keeps your shells running when you
close the terminal window. A tmux session that you didn't name, which tmux numbers `0`, `1` and
on, doesn't count.

**The unfenced pool.** The pool with no fence. A config holds at most one, and it takes every session
started outside all the fences. If every pool is fenced, a session started outside them doesn't
start.

**Window.** A span of time Claude counts usage over. Each account has a week. Pro and Max
accounts have a 5-hour window too. A verified Ultra account doesn't. Some models also have a week
of their own. When a window resets, its usage starts again from zero.

**Worth.** What a request would have cost at Anthropic's API prices. Switchboard sets it against
what your plans cost.
