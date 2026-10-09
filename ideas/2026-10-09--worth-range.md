# Worth's range

Worth is counted in picodollars, as an `int64`, which holds some $9.2 million. Found while building
milestone 7's stage 2, 9 October 2026, as the config came to price models and plans itself.

- **A price too large to convert.** The config gives a price in dollars, a million tokens or a
  month, converted once (`perMTok`, `inDollars`). A price past some $9.2 trillion a million tokens
  doesn't fit, and converts to nonsense rather than failing. The design asks only that a price be a
  number, 0 or more, so nothing refuses it.
- **A sum too large to hold.** A period's worth is summed in the same `int64`, and wraps round past
  $9.2 million: a year of heavy use, over several accounts, priced by a config far above the
  table's prices, could reach it.

A fix: refuse a price too large to count, its bound settled with the owner, and sum a period's
worth so that one too large reads as unpriced, never wrapped round.
