# Money in your own currency

Switchboard shows money in US dollars everywhere: worth at API prices, plan prices, the ratio
between them, and `history`'s and the views' totals. The owner pays for their plans in pounds
sterling, and many people pay in a currency other than dollars.

- **A chosen currency:** one config key, such as `currency = "GBP"`, shows every figure of money in
  it: the dashboard, the verbs' text, and their JSON, where a field would name its currency.
- **The rate:** API prices are in dollars, so worth needs a rate. A rate in the config is simplest
  and needs no network; a dated rate table, kept like the price table, or one read now and then,
  would follow the rate as it moves. Which, to settle.
- **Plan prices differ by country,** not only by rate: a plan billed in pounds isn't its dollar
  price converted. `[prices.plans]` already sets what you pay; in a chosen currency, it would be
  read in that currency.

Raised by the owner on 15 October 2026.
