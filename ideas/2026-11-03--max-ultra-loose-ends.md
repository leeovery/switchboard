# The Max and Ultra build: what it left

The build that followed Anthropic's renamed plans, and verified Ultra accounts answering with no
5-hour window, ran #222 to #237 (see Observed in the design). The reviews of its slices accepted
these: an improvement, or an edge past ordinary use that fails safe. Gathered here for after the
release.

## Edges

- **Status's lacking rule is looser than the router's.** The router retires a window only off a
  whole reading: a 2xx answer that spends, or a probe the API admitted. Status's `lacking` takes any
  reading that carries another shared window. A refusal carrying the week alone would read as `no
  5-hour limit` until the next whole reading, and a probed document would drop that account's prime
  slot. No such refusal has been seen. Carrying whether the reading was whole on the account's
  status would give the two one rule.
- **A 5-hour limit reached before its window was retired** holds until its `until`, at most five
  hours, rather than lifting with the retirement.
- **The reserve's log line** says a retired 5-hour was let go at its reset.
- **A new Ultra subscriber's temporary 5-hour** isn't offered priming in setup, which skips the
  step when every account is on Ultra. The router primes it where the config sets a day.
- **A lacking week in the routing card's first slot** would crowd the figure after it. No account
  has been seen reporting the 5-hour without the week.
