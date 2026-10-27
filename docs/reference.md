# Reference

These are the files switchboard writes and the JSON its commands print, for anyone reading them
directly, agents included. Each format is described field by field.

## Contents

- [Rules for every format](#rules-for-every-format)
- [Where things live](#where-things-live)
- [The readings history](#the-readings-history)
- [The request ledger](#the-request-ledger)
- [The day's summary](#the-days-summary)
- [The router's events](#the-routers-events)
- [The status document](#the-status-document)
- [`status --session`'s JSON](#status---sessions-json)
- [`history`'s JSON](#historys-json)
- [`sessions`' JSON](#sessions-json)

## Rules for every format

- **New fields:** Fields may be added to any format, but never renamed. When you read one, pass
  over the fields you don't know.
- **Accounts:** Accounts appear by their ids, never by a token. Their labels appear only in the
  status document's `accounts`.

## Where things live

| What | Where |
|---|---|
| The config | `$SWITCHBOARD_CONFIG`, else `$XDG_CONFIG_HOME/switchboard/config.toml`, else `~/.config/switchboard/config.toml` |
| The dashboard's themes | `$SWITCHBOARD_THEMES_DIR`, else `$XDG_CONFIG_HOME/switchboard/themes/`, else `~/.config/switchboard/themes/` |
| The state directory | `$XDG_STATE_HOME/switchboard/`, else `~/.local/state/switchboard/` |
| The `claude` link | `$XDG_DATA_HOME/switchboard/bin/claude`, else `~/.local/share/switchboard/bin/claude` |
| The LaunchAgent | `~/Library/LaunchAgents/io.github.leeovery.switchboard.plist` |
| The skill | `skills/switchboard/SKILL.md` in `$CLAUDE_CONFIG_DIR`, else in `~/.claude` |

The state directory holds these files:

| File | Holds |
|---|---|
| `state.json` | The router's state that outlasts a restart: each session's account, the pins, each account's last readings, and the SHA-256 hashes of the account's token and of the tokens it replaced. See [Tokens](configuration.md#tokens). |
| `control.sock` | The router's control API, which the commands and the dashboard talk to. |
| `prefs.json` | What the dashboard remembers. See [`dashboard`](dashboard.md) and [Themes](dashboard.md#themes). |
| `tokens/<id>` | Each account's token. See [Tokens](configuration.md#tokens). |
| `tokens/<id>.taken` | When switchboard took the account's token, with the token's SHA-256 hash. See [When each token was taken](configuration.md#when-each-token-was-taken). |
| `logs/` | `router.log`, `cli.log` and `launchd.log`. See [The logs](troubleshooting.md#the-logs). |
| `history/readings-<date>.jsonl` | The [readings history](#the-readings-history), one file for each day. |
| `ledger/requests-<date>.jsonl` | The [request ledger](#the-request-ledger)'s lines, one file for each day. |
| `ledger/day-<date>.json` | Each [day's summary](#the-days-summary). |
| `ledger/digest-<date>.json.gz` | Each day's digest: every session the day's summary names, as it stood at the day's end, its page among it, so the dashboard and `sessions` needn't read every day a session ran. Its form is switchboard's own, and can change with any release. Deleting it is safe: until the router writes it again, the day's lines are read instead. |
| `ledger/parts-<date>.gz` | Each day's parts, beside its digest: what each session's page gained that day, its turns and the subagents it first saw in one gzip stream, then when its requests went upstream in another, each of them pointed at by the digest, so the router, building the next day's digest, never reads those times. Its form is switchboard's own too. Deleting it is safe: the router writes it again with the day's digest. Until then, or where one is damaged, a session's page rests on an older digest, or reads the session's lines whole instead. |
| `ledger/logday-<date>.json.gz` | Each day's head: every session whose requests the day's lines hold, with the directory it ran in, so the Log's find needn't read every day's lines, and where each chunk of the day's rows lies in its rows file. Its form is switchboard's own too, and it's kept as long as the day's lines. Deleting it is safe: until the router writes it again, the find, the Log and a turn read the day's lines instead. |
| `ledger/rows-<date>.gz` | Each day's rows, beside its head: a row for each of the day's lines, as the Log shows it, without a price, 256 rows to a gzip stream, each checked against the CRC its head records, so the Log reads a day back, and a turn its own days, without reading the day's lines. Its form is switchboard's own too, and it's kept as long as the day's lines. Deleting it is safe: until the router writes it again with the day's head, the day's lines are read instead. |
| `ledger/events-<date>.jsonl` | The [router's events](#the-routers-events), one file for each day. |

Once a day ended two days ago, its files of lines are compressed to `<name>.jsonl.gz`. To read a
compressed file, use `gzip -dc`.

Each file of lines is kept as long as its `keep` setting says. See
[`[history]` keys](configuration.md#history-keys) and [`[ledger]` keys](configuration.md#ledger-keys).
The events are kept as long as the ledger's lines.

## The readings history

The router writes each reading as a JSON line in `history/readings-<date>.jsonl`:
`{"at", "account", "window", "utilization", "resets_at", "status", "source"}`.

| Field | Description |
|---|---|
| `at` | When it was read. |
| `account` | The account, by its id alone. |
| `window` | The window's key, as the API gives it, such as `5h`, `7d` or `7d_oi`. |
| `utilization` | How much of the window is used, as a fraction. |
| `resets_at` | When the window resets. |
| `status` | `allowed`, `allowed_warning` or `rejected`. |
| `source` | Where the reading came from: an `answer`, a `probe` or a `prime`. |

## The request ledger

The router writes a JSON line for each request it routes, in `ledger/requests-<date>.jsonl`. The
line keeps everything about a request except its content. It never holds any of these:

- The request's messages, its system prompt or its tools.
- Its metadata's ids.
- A setting or header that the line doesn't name.
- Anything shaped like a token.

| Field | Description |
|---|---|
| `at` | When the request arrived, in UTC, to the millisecond. |
| `request` | The router's id for the request, as the request's routed line in the log gives it. |
| `kind` | `message`, Claude Code's quota `check`, or a `count` of tokens. |
| `session` | The request's session id. |
| `dir` | The directory `run` started Claude Code in, with your home shown as `~`. |
| `model` | The model the request asked for. |
| `account` | The account whose answer the client got, by its id alone. Left out where the router answered the request itself. |
| `reason` | Why the request went to that account, in the words of the log's routed line. |
| `from` | The account the session was on before, where the request moved it. |
| `tried` | The accounts the request went out on that couldn't serve it, in the order tried, each `{account, why}`. |
| `status` | The answer's status. `0` where the request ended before an answer came. |
| `canceled` | Set where the client went away before the end. |
| `cut_off` | Set where the router cut the request off as it stopped. |
| `attempts` | How many times the request went upstream. `0` where the router answered it itself. |
| `first_ms` | How long after the request arrived its answer's first byte passed on to the client, in milliseconds. |
| `total_ms` | How long after the request arrived its answer ended, in milliseconds. |
| `agent` | The client's user agent. |
| `betas` | The features the request asked the API for beyond its version. |
| `prompt` | The prompt the request serves, by Claude Code's random id. The id is the same on every request of one prompt, the prompt's subagents' requests included. |
| `class` | `main`, `subagent`, `workflow`, `compaction` or `auxiliary`. |
| `agent_id` | The subagent that sent the request, by Claude Code's id for it. |
| `parent_agent_id` | The subagent that started that subagent, by Claude Code's id for it. |
| `agent_type` | The subagent's kind: a built-in agent's name, `custom`, `teammate` or `fork`. Never a name you chose. |
| `compaction` | On a compaction request, what started the compaction: `auto`, `manual` or `reactive`. |
| `compacted` | On the first request after a compaction, what started that compaction. |
| `tool_ms` | How long each tool call whose result the request carries ran, each `{tool, ms}`. |
| `shape` | The request's size, and those of its settings switchboard knows. See below. |
| `answer` | The answer the client got. See below. |
| `usage` | The answer's closing usage, as the API gave it, field for field. |
| `limits` | The answer's `anthropic-ratelimit-unified-*` headers, with the prefix taken off. |

- **`dir`:** `run` tells the router the directory in a header the API never sees. A `claude`
  started within that session other than through `run` carries the directory too.
- **`agent_id` and `parent_agent_id`:** Claude Code's id for a subagent is random. For a member of
  an agent team, the id comes from its name.

`shape` holds the request's size and settings:

- `bytes`: the request's size.
- How many `messages`, `system` blocks and `tools` it carried.
- Those of its settings switchboard knows: `max_tokens`, `thinking`, `stream`, `tool_choice`'s
  type, `temperature`, `top_k`, `top_p`, `service_tier`, `output_config`'s `effort`, `speed`,
  `inference_geo`, and the types of `context_management`'s edits.

`answer` holds these, of the answer the client got:

- `id`: Anthropic's id for the answer.
- `model`: the model that served it.
- `stop`: why it stopped.
- `blocks`: how many blocks of each kind it held.
- `tools`: the tools it called, by name.
- An `error`'s `type` and `message`.

The fields from `prompt` to `tool_ms` are what Claude Code's own headers say of the request. Each
is left out where Claude Code sends no such header. Claude Code always sends the router the agents'
ids, `agent_id` and `parent_agent_id`. It sends the rest because `run` has it send them, with `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`.
See [`claude`](commands.md#claude).

## The day's summary

An hour after each day ends, the router summarises the day beside its lines, in
`ledger/day-<date>.json`: `{"version", "day", "lines", "bytes", "accounts"}`.

- **`lines`:** How many lines the summary was made from.
- **`bytes`:** How large the day's files were then.
- **`accounts`:** Each entry is one account's day, with the fields below.

| Field | Description |
|---|---|
| `account` | The account, by its id alone. Left out of the requests no account answered. |
| `models` | The account's requests, by the model each asked for. See below. |
| `sessions`, `session_ids` | The sessions the account's requests were of, and their ids. |
| `moved_on` | The sessions its requests moved onto the account. |
| `moved_off` | The sessions its requests moved off the account. |
| `highest` | Each window's highest use that day, by the window's key. |
| `rise` | How far each window's use rose that day, from the history. |
| `resets` | The account's window resets that day, each `{window, at, before}`, where `before` is the use before the reset. |
| `limits` | The limits the account reached, each a window turning `rejected`: `{window, at, resets_at}`. |
| `minutes_at_cap` | The minutes the account spent at its cap. |
| `minutes_at_limit` | The minutes the account spent at a limit. |
| `read_before` | The windows the history read in the week before. |

Each entry in `models` counts its requests by how they went:

- `upstream`: those that went upstream.
- `hours`: the requests `upstream` counts, by the local hour each was made in, as its line's `at`
  gives it: 24 counts, from midnight's hour on, that add up to `upstream`. An hour the clocks skip
  has none, and one they repeat counts both. A day summarised before the router kept them gains
  them when it's summarised again from its lines; one whose lines are gone has none.
- `no_usage`: those of them that got no usage back.
- `unsent`: those the router answered itself.
- `checks`: Claude Code's quota checks. Their usage is left out of the model's `usage`, as they
  aren't work done for you, so nothing prices them.
- `counts`: Claude Code's counts of tokens.

Each entry also has the requests' `sessions` and `session_ids`, their `usage` summed field for
field, and `inference_geo` where they asked for one.

### Which accounts get a day

An account gets a day of its own in the summary when any of these is true:

- The history read the account that day, whether or not it served a request.
- One of the account's windows reset that day, going by the reset its last reading gave. This way,
  a week that ended on a quiet day keeps its peak.
- The history read the account in the week before, and its window ran on into the day without a
  reset, whether or not it was read that day.

### Which days get a summary

The router also summarises a day it served nothing on, if it read an account or saw a window reset
that day. It does this only once the history holds the week before that day. Otherwise, a window
already at its limit as the history began would read as one that reached its limit that day.

A day that ended while the router was stopped is summarised when the router starts again. This
happens before the history's readings of that day can go.

### Summarised again

A line can come to be filed under a day later, such as when a request still in flight past that
hour files one. The router then summarises the day again. The new summary knows no less than the
one before it. That holds even when the history has since let the day's readings go, or a damaged
file has lost some of its lines, which can't be told apart from the lines filed since. So the
summary's counts are then the least there were. A model's `hours` come whole from whichever summary
counted more of its requests upstream, so they still add up to `upstream`.

### Kept for good

Once a day's lines go, its summary is kept for good. It's summarised again only by a release that
summarises days in a different way, and then from the day's readings while the history keeps them.
So a day whose lines went before the router kept `hours` never gains them: the readings can't give
them.

## The router's events

The router writes a JSON line for each version of an event, in `ledger/events-<date>.jsonl`. An
event can change, so when you read one that changed, take its last version.

| Field | Description |
|---|---|
| `run` | When the router that told of the event started. |
| `id` | The event's number in that run, rising from 1. |
| `at` | When the event happened. |
| `kind` | What happened. See below. |
| `account` | The account the event befell. |
| `accounts` | The accounts a global pin names. |
| `session` | The session that started or moved, or whose own pin changed. |
| `model` | The model whose requests did so. |
| `from` | The account a session moved from. |
| `to` | The account a session moved to. Also, where the sessions a limit, a cap or a refusal moved all went to one account, that account. |
| `reason` | Why: why a session went to its account, why a restart is due, or why the router turned unhealthy. |
| `held` | On a restart, why the binary the router would become refuses to start, which holds the restart. |
| `windows` | The keys of the windows the event is of. |
| `joined` | On a limit or a cap, when each window that came to be held by it after it began did so, by the window's key. See below. |
| `until` | When a limit, cap or refusal ends, or a window runs out or resets. A limit or a refusal that lifts early has it brought forward to the lift. |
| `since` | When a pressure's rate is measured from. |
| `count` | How many sessions a limit, a cap or a refusal moved while it held. |
| `limit` | On a limit, its identity, kept while it holds. On a move, the id of the limit's event that counts the move among the sessions it moved. |
| `forced_by` | On a move, the id of the limit's, cap's or refusal's event that counts the move among the sessions it moved. |
| `reserve` | On a cap, the account's reserve. |
| `status` | On a refusal, the status the API refused with. |
| `family` | On a refusal of a request refused alone, the model family the refusal holds back. |
| `move` | On a pin, set where the pin moves the running sessions too. |
| `force` | On a pin, set where the pin cleared every session's own pin. |
| `by` | What set or cleared a pin: `cli` or `dashboard`. |

`kind` is one of these:

- `started`: a session first on an account.
- `moved`.
- `pressure`.
- `cap`: a window at its reserve.
- `limit`.
- `refused`.
- `primed`.
- `room`: an account with room again.
- `pin` and `auto`: routing set by hand, and routing given back.
- `restart`: a restart falling due, or held, with `held`, as the binary the router would become
  refuses to start.
- `health`: the router's own health turning.

`joined` shows, for example, the 5-hour window joining a cap of Fable's week. It's left out where
no window joined. A window that `joined` leaves out was held from `at`.

## The status document

[`status`](commands.md#status) and [`usage`](commands.md#usage) print this document as JSON:
`{"generated_at", "source", "fallback", "best", "primary", "prime", "pin", "router", "restart",
"sessions", "events", "pool", "coming_up", "accounts"}`.

- Times are in UTC, except the priming schedule's times of day, which are local.
- Utilizations are fractions, and can pass 1.
- A field only the router knows is left out of a document built by probing.

| Field | Description |
|---|---|
| `generated_at` | When the document was built. |
| `source` | `router`, or `probe` where the document was built by probing every account. |
| `fallback` | Why it was built by probing, where the router was asked first. |
| `best` | The account new sessions go to next. |
| `primary` | The primary's id. |
| `prime` | The priming schedule: the day, the window a prime starts, and each account's slot. |
| `pin` | The global pin: the accounts it names, since when, and whether it moved the running sessions. |
| `router` | The router's health over the last 5 minutes. |
| `restart` | A restart the router has due, and, as `held`, why the binary it would become refuses to start, where that holds it. |
| `sessions` | How many sessions the router has routed in the last hour. |
| `events` | What's happened lately, newest first. |
| `pool` | The room new sessions can go to, summed across the pool's accounts, and their mean use. |
| `coming_up` | Each limit, cap or refused token lifting, window running out or resetting, and prime, soonest first. |
| `accounts` | Each account, in the config's order. See below. |

Each entry in `accounts` holds these:

- The account's `id` and `label`, whether it's the `primary`, and its `reserve`.
- Whether its token is set, and when it was read.
- `token_taken` and `token_good_until`: when switchboard took its token, and when the token's year
  is up, each left out where that isn't known.
- Its `windows`, each with its use, reset and status, its even pace (`pace`) and its `allowance`.
- `lapsed` and `at_reserve`: the windows that have lapsed, and those at the reserve.
- Its `extra_usage`.
- What couldn't be read, and why.
- From the router only: a `limit` or refusal that holds the account, its `pressure` and `rates`,
  and its `sessions`.

## `status --session`'s JSON

`status --session`'s JSON is `{"session", "pin", "assignments", "account"}`, of one session the
router has seen.

- **`session`:** The session's whole id.
- **`pin`:** The session's own pin, where it has one.
- **`assignments`:** One for each model, the one used last first. See below.
- **`account`:** The account the session's last-used model went to, in the form the status
  document's `accounts` give each account.

Each assignment is `{model, family, account, auto, dir, in_flight, pinned, yielded, pinned_at,
reason, assigned_at, last_seen}`. These fields need saying more of:

- **`auto`:** The account the session's next request of the model would go to without a pin of
  its own.
- **`in_flight`:** `asking` or `answering`, while a request is.
- **`yielded`:** Set while the session's own pin has yielded.
- **`pinned_at`:** When the session was pinned while it ran.

## `history`'s JSON

`history`'s JSON is `{"prices_as_of", "days", "year", "months", "weeks", "tokens", "totals", "plans",
"capacity"}`. Each block covers the days asked for since the ledger began, oldest first and today's
last.

### `prices_as_of`

The day the prices were read. Beside it, `prices_from_config` is set where the config's
[`[prices]`](configuration.md#prices-keys) price a model or a plan.

### `days`

Each day's summary as the ledger holds it (see [The day's summary](#the-days-summary)), with these
added:

- Each model's `worth`, in US dollars.
- `unpriced`, naming what the worth leaves out, such as `no_usage`, requests that got no usage
  back.
- `by_family`: the day's worth by model family, of the models used: none of quota checks or counts
  of tokens alone.

A day of no requests, where the history tells of no account that day, has no `accounts`. An account
has a day there where its quota checks, its counts of tokens or the history give it one, with
requests or none.

### `year`

- Each day's `requests`, and the `level`, 0 to 4, the day is shaded at.
- `cuts`: the cuts between the levels, which are the active days' quarters.

### `months`

Each month's:

- `requests` and `worth`.
- `limits` reached, by window.
- `by_family`.
- `cost`: the cost of the month's plans. It's the whole month's, or, where the days asked for begin
  part-way through the month, its days' from the first asked for, or from the ledger's first day
  where it began later.
- `against`: the worth of the month's accounts against that cost, as a ratio. The month so far has
  none.
- `plans`: how many accounts' plans are `known`, of how many there are, as in `plans` below.

### `weeks`

Each week, starting on the day the config's `week_starts` names (Monday unless it says otherwise).
For each account, the week has these:

- `peaks`: by window, the highest use of the account's week, and of each model's own week, such as
  Fable's.
- `open`: the windows whose week is still running. Their peak is the peak so far.
- `limits`: the limits the account reached in that week, by window.

Then `all`, for every account together, where each window's peak is the mean of the accounts'
peaks.

### `tokens`

Each week's tokens:

- By kind: `input`, `output`, `cache_write`, `cache_read` and `total`.
- The week's `worth`, its plans' `cost`, `against`, and how many `plans` are known, as in `plans`
  below.
- `by_account`, in the config's order.
- `by_model`, each model version by its id, with the accounts it ran `on`, the account it was worth
  most on first. A model of quota checks or counts of tokens alone, which wasn't used, is in neither,
  nor is an account that sent them alone.

### `totals`

Each account's totals, then `all`, for every account together:

- `requests`.
- `sessions`, each counted once however many days and accounts it ran on.
- `moved_on` and `moved_off`.
- `limits`, by window.
- `minutes_at_cap` and `minutes_at_limit`.
- `left_at_reset`: each window's share left at its last reset.
- `ran_out`: the day the window last reached its limit in the run that its last reset ended, where
  it did.
- `worth`.
- `by_model`: each model version's `share` of use, weighed at API prices, and its `worth`, of the
  versions used alone.
- `busiest_day`.
- `longest_run`: the most days in a row with any request.
- `partial`: naming a count that some of the days never counted.

### `plans`

Each account's:

- `plan`, and its `price` a month.
- What the plan cost over those days, `cost`, and the account's worth `against` it. Where the
  ledger began after the first day asked for, the plan is priced from the ledger's first day, as
  there's no worth before it to set against it.
- The account's `worth`.

Then `all`, for every account on a plan together, with `plans`: how many accounts' plans are
`known`, of how many accounts there are, as `{"known": 2, "of": 3}`. Its cost, and the worth
against it, are those of the accounts whose plans are known alone.

### `capacity`

`capacity` covers the whole `weeks` in which every account's week is known and ended. It holds:

- `short`: how many of those weeks were short.
- `with_one_fewer` and `with_one_more`: how many would have been short with one account fewer, and
  with one more.
- `verdict`: `one too few`, `too few`, `one more than you need` or `about right`.
- `drop`: where the verdict is one more than you need, the account to drop.
- `with_two_fewer`: with three accounts or more, where the verdict is one more than you need, how
  many weeks are short without `drop` and without the account whose loss then makes the fewest
  weeks short.
- Each account's `verdict`, over the account's own whole `weeks`, with:
  - `limits`: the weeks the account hit its limit in.
  - `left_at_reset`: what it had left at a reset, on average.
  - `without_it`: how many weeks the rest would have been short in without it.
- Then `all`, for every account together, with the verdict `room to spare together` or
  `little to spare together`.

### Worth

Every `worth` is exact, with `unpriced` beside it naming what the worth leaves out:

- The counts a usage holds that the price table can't price.
- `no_usage`: requests that got no usage back.
- Of requests it prices none of: a model it doesn't know, by the model's id; a model it
  knows, in a geo it doesn't price that model in, as `inference_geo:eu`; and requests that named no
  model, as `no_model`.

`worth` is left out where nothing was priced, such as by `sessions`. Every share is exact.

### With `--windows`

`history --windows` prints `{"windows": [{"account", "window", "readings"}]}`. Each reading is
`{at, utilization, resets_at, status, source}`.

## `sessions`' JSON

`sessions`' JSON is `{"generated_at", "prices_as_of", "sessions", "today"}`.

`sessions` lists each session, with these:

- Its whole id, its directory, account and model.
- Whether it's `running`.
- For a running session: when it was last seen, each of its models' account and why, what it's
  doing, and its `move_cost`, what moving it now would cost.
- For an ended session: when it `ended`.
- When it `started`.
- Its `requests` today, their `worth` in US dollars, and what the worth leaves `unpriced`.
- `moved`: the move that brought it to its account.

`today` sums the sessions.

### A session's page

Given a session, `sessions` prints that session's page:

- **`session`, `dir`, `account`, `model`, `running`, `last_seen`, `models`, `state`, `move_cost`
  and `ended`:** As the list has them.
- **`pin`:** The session's own pin.
- **`started` and `resumed`.**
- **`kept_until`:** For an ended session, the last day the ledger keeps the lines of the session's
  first day.
- **`accounts`:** Each `{account, from, to, requests, worth, points}`, with `points` by window.
- **`moves`:** Each `{at, model, from, to, reason, written, cost}`.
- **`turns`:** Newest first, each `{turn, started, ended, requests, tools, read, written, out,
  worth, accounts}`. `ended` is left out of a turn still going.
- **`totals`:** `{turns, requests, from_cache, worth, unpriced}`.

Every `worth` is exact, and left out where nothing was priced. `unpriced` names what the worth
leaves out, as `history` names it.
