# Keeping the price table current

Worth is priced from a table built into switchboard (`internal/ledger/pricing.go`), read from
Anthropic's pricing page on 7 October 2026. Each price carries the day it took effect, a model can
hold several, and worth is worked out as it's read, never stored: so a release with a new dated row
prices the whole ledger afresh, requests before a change at the old price and those after at the
new. The config's `[prices]` can stand in meanwhile, but an override prices every day alike, so it's
a stopgap.

Nothing notices when Anthropic changes a price. On 9 October 2026 the owner noticed Claude Sonnet
5.5's cache prices halved, which the table doesn't hold.

- **The change itself:** a dated row for Sonnet 5.5's cache prices, from the day they changed, and
  every other model's prices checked against the page while there.
- **Noticing changes:** a scheduled check that reads the pricing page, compares it with the table,
  and opens a pull request with the new dated rows, for review and a release. Either a GitHub
  Actions workflow on a cron, which needs the page parsed reliably, or a scheduled Claude Code
  routine, which reads it as a person does and copes with its layout changing. Either way, a change
  lands as a reviewed pull request, never straight into the table.
- **To settle with the owner:** which runs it, how often, and how it tells a price that changed
  from a page that did.
