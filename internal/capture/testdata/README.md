# The sessions' world's traffic

`traffic.jsonl` is the Log's traffic of Thursday 1 October 2026 to 14:42:07, and of the day
before where its sessions began, of the sessions whose lines the session pages' model doesn't
make in Go (`plans.go`). It was drawn from Python's random numbers by the
mock generator's `log1.traffic`, so it's kept as data; `traffic.go` reads it into ledger lines.

A line a request, oldest first: `day` -1 for the day before, `at` its clock time to the second,
`session` its short id (a quota check's own, whole), `model` as the frames name it, `on` its
account, `kind` (a message where it's left out), `status` (200), `first` and `took` in
milliseconds, `in` (3), `read`, `wrote` (to the cache for an hour) and `out` tokens, and how its
answer stopped, `stop` and `tool`. Where a request went, and why, its prompt and its id are
worked out as it's read.

## How it was made

- **The Log's windows,** as drawn: `log1.tail`'s requests that had ended by 14:42:07, and
  `log1.around_the_cap`'s, of 41e0, 7f3a, db8a and c61b.
- **The rest of each session's day** in bursts, each `log1.traffic` over one session from a seed
  of its own: its count what the Log and Sessions' List give the session today; its first
  request at the second the router's started event names, or its last in the minute the List
  ends it; its last answer ending its turn. 7f3a and db8a fill the gap between the Log's windows,
  14:33 to 14:40, each with as many requests as the fastest of 300 seeds fits.
- **Each session's context** runs on from burst to burst, from a start that makes its lines today
  worth what the List gives, the cent made up on its last request's output. Where a move or the
  Log's windows pin the context, the outputs are scaled to that worth instead. db8a's move to
  work at 13:22:40 writes 48,000 tokens again, and 8a2c's to side yesterday at 19:12:52, 61,000.
- **Claude Code's quota checks,** 31 today, as it starts or resumes a session or starts and asks
  nothing, three overloaded (529) and sent again; and **its counts of tokens,** 4 today, of no
  session.
