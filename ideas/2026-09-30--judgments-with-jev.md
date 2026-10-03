# Judgments with Jev

TypeSafe's System One models, Jev among them, turn natural language and application state into
typed judgments and probabilities that code can combine like any other value. Switchboard makes
its judgment calls today with fixed rules: whether a 429 is a limit or throttling, whether a
refusal is the request's or the account's, whether an account is heading for its limit, where a new
session should go. Some of those might be better as judgments, or have a judgment beside the rule
to show where the rule is wrong.

For idea storming later, not a design. Places it might fit:

- Reading an upstream refusal's message to tell a request-specific refusal from an account's.
- Judging from a session's recent requests whether it's an agent swarm about to burn a window, to
  place it before the burn shows in the readings.
- Summarising a day's usage and the router's choices in plain words, from the readings history and
  a request ledger.
- Triaging this inbox: which ideas are worth doing next, given what the data says.

Anything on the routing path has to stay fast and keep working offline from everything but
Anthropic's API, so a judgment there would more likely tune a rule offline than sit in the path of
a request.
