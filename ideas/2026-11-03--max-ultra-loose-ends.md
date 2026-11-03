# The Max and Ultra build: what it left

The build that followed Anthropic's renamed plans, and verified Ultra accounts answering with no
5-hour window, ran #222 to #237 (see Observed in the design). The reviews of its slices accepted
these: an improvement, or an edge past ordinary use that fails safe. Gathered here for after the
release.

## GLANCE's key at the page's width

GLANCE's key flows in GLANCE's own width, so it can sit beside WHEN YOU WORK. Where GLANCE stands
alone and narrower than the page, the key wraps all the same. With every account on Ultra, GLANCE
has the week's column alone and takes 63 cells, so at 108 columns its key takes two rows. The page
is then a row short of the cards' words sitting apart: the cards take their compact form, and three
rows go blank above the legend (`graphs-glance-ultra-108x44`). Stacked layouts did this before the
build: `graphs-pool-73x61`, and `graphs-glance-108x44` at 85 columns. Flowing the key in the page's
width wherever GLANCE stands alone would give those rows back.

## Graphs reads and builds a little it doesn't draw

- **History for a 5-hour not offered.** Where no account has a 5-hour window, Graphs still counts
  it among the windows whose history it reads, so it reads that history and draws nothing of it.
- **The watch's spans.** `graphsSpans` builds a Σ pool span for each of Runway's windows, not only
  those Runway offers. With every account on Ultra, the 5-hour's key is empty, and the span returns
  at once, so nothing is read. Looping `views.RunwayWindowsOf` would say what's meant.

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
