# Glossary

The words switchboard uses in a sense of its own. Every page uses them in this sense and no other.

**Account.** One Claude subscription that you've added to switchboard. You see it by its label,
such as `work`, and commands name it by the id switchboard gives it, such as `b6s7`.

**Cap.** The point where the reserve of an account begins. With a reserve of 10%, the cap is at
90% of each window.

**Direct.** A session that starts without the router. `switchboard run --direct` starts one on
the account Claude Code itself is signed into. If you set `when_router_down = "direct"`, a session
started while the router is down goes direct on an account of its pool.

**Fence.** The folders a pool is tied to. A session started in one of them, in a folder beneath
it, or in a git worktree of a repository there, belongs to that pool.

**Label.** The name you give an account or a pool, shown on every screen. You can change it
whenever you like.

**Ledger.** The record of every request the router handles, without its content. It's kept as a
file a day, for 400 days unless you set another length.

**Limit.** The most of a window that Claude lets an account use. An account at a limit can't take
more requests in that window until it resets.

**Open accounts.** The accounts new sessions can go to right now: every account in the pool, or
only the pinned ones while a pin holds. The dashboard and `usage` still call these the pool, for
example in `Σ pool`. That's a second meaning of the word, and this term replaces it.

**Pin.** Your choice of account for new sessions, or for one running session, in place of the
choice of the router. A pin gives way when its account hits a limit.

**Pool.** A set of accounts that sessions are routed across. Every account is in exactly one
pool, and nothing crosses from one pool to another. See [Pools](pools.md).

**Primary.** The account that sessions in a pool start on. Artifacts and uploads go out on it, so
they open in a browser signed into that account. Each pool has one.

**Priming.** Starting the 5-hour window of an account at a time switchboard chooses, so your
accounts reset one after another rather than together.

**Reserve.** A share of every window that the router leaves unused on an account, for you to use
outside switchboard, for example in the Claude apps. The router never spends it, but a pin can.

**Room.** How much of a window an account has left before its limit or its cap.

**The router.** The program on your Mac that Claude Code sends every request through. It picks
the account, swaps in its token, and records the request. The service keeps it running.

**Session.** One conversation in Claude Code, from start to exit. A resumed session carries on the
same conversation, but starts a new process.

**The service.** The LaunchAgent that runs the router at login and restarts it when needed.

**The unfenced pool.** The pool with no fence. There's at most one, and it takes every session
started outside all the fences. If every pool is fenced, a session started outside them doesn't
start.

**Window.** A span of time Claude counts usage over. Each account has a week, and most have a
5-hour window too. Some models also have a week of their own. When a window resets, its usage
starts again from zero.
