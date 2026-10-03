# A reserve spent as a last resort

The router never spends a reserve of its own choice. An account with one set, as `reserve` sets
it, is passed over once a window reaches it, and once every account is out or at its reserve, the
router answers Claude Code with a 429 of its own, until the first has room again. A pin gets past
it, as a pin spends the reserves of the accounts it names: `pin <id> --move`, then `pin auto` to
hand back. But that's a step to remember at the moment work stops, and a pin outlives the moment:
while it holds, it spends the reserve, whatever else frees up.

An option in the config would have the router go past a reserve itself, when nothing else is
left:

- Only when no account has room outside its reserve. While any account has room, the reserve
  holds, as now.
- Once another account has room again, routing goes on by the usual rules, a session's warm cache
  and its model's bound thinking included.

To settle with the owner:

- Where the option lives: on each account, beside `reserve`, or once for the router.
- What a session already spending a reserve does once another account has room. The usual rules
  move a session off an account with no room, and the reserve holds again, so it would move,
  rebuilding its cache elsewhere. To keep its cache, it could stay until it idles.
- How `status`, the dashboard and the notifications say that a reserve is being spent, and why.

Next, a fast follow on milestone 5.
