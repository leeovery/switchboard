# Keeping the price table current

Worth is priced from a table built into switchboard (`internal/ledger/pricing.go`), read from
Anthropic's pricing page on 7 October 2026. Each price carries the day it took effect, a model can
hold several, and worth is worked out as it's read, never stored: so a release with a new dated row
prices the whole ledger afresh, requests before a change at the old price and those after at the
new. The config's `[prices]` can stand in meanwhile, but an override prices every day alike, so it's
a stopgap.

Nothing notices when Anthropic changes a price. On 9 October 2026 the owner noticed Claude Sonnet
5.5's cache prices halved, which the table doesn't hold.

- **The change itself, done on 11 October 2026:** Sonnet 5.5's cache reads, lowered from $0.20 to
  $0.10 a million on 7 October 2026, are a dated row from that day; Claude Haiku 5.5, launched the
  same day, joined the table; every other model's prices matched the page.
- **Haiku 5.5's two rates:** it prices a prompt of over 100,000 tokens higher, five times its rate
  for shorter ones, and the table holds one rate a model. Until this is settled, every Haiku 5.5
  request is priced at the shorter rate, the owner's call: about a fifth of its requests in the
  ledger's first five days were longer, $0.34 between the two. Keeping the higher rate means a day's
  summary keeps the longer prompts apart, as it keeps US-only inference apart.
- **How the table is kept, the owner (11 October 2026):** reliable price information, and how it's
  structured, to discuss, perhaps as a history of price updates; whatever is settled can be
  backdated, as the ledger keeps every request's tokens and worth is worked out as it's read.
- **Noticing changes:** a scheduled check that reads the pricing page, compares it with the table,
  and opens a pull request with the new dated rows, for review and a release. Either a GitHub
  Actions workflow on a cron, which needs the page parsed reliably, or a scheduled Claude Code
  routine, which reads it as a person does and copes with its layout changing. Either way, a change
  lands as a reviewed pull request, never straight into the table.
- **To settle with the owner:** which runs it, how often, and how it tells a price that changed
  from a page that did.
