# Milestone 7's loose ends

Milestone 7, the dashboard's redesign, went through a review of each pull request, of each stage,
and of the whole release, several rounds each. What those reviews found and left, as not worth
holding the release for, gathered here. Each is an edge case or a cost with no wrong answer.

## The owner's calls, to build

Decided by the owner on 15 October 2026, going through the calls made in the build. Each goes into
the design as it's built.

- **A young ledger's plans are priced from its first day, everywhere.** The period or month the
  ledger began in is charged only for the days it holds, on Accounts and the Year's BY MONTH as
  `history` and the Year's numbers already do: there's no worth for the days before, so charging
  for them understates the ratio. Every later period is whole either way. Today Accounts prices a
  fresh install's week whole ($46 for Max 20x) while `history` prices it from the install day
  ($33 from a Wednesday), and the Year's numbers come to $1,490 beside BY MONTH's $1,760 for a
  ledger begun on 16 March.
- **The all-accounts ratio says it's over the known plans,** where an account's plan isn't known:
  `4.4× its $92 week · 2 of 3 plans known`. The worth beside it stays every account's. The same
  words go on `history`'s totals and Tokens' rows. Today the worth counts every account and the
  ratio only those with a plan, so the two don't divide.
- **Help says `esc deselect`** on Sessions with a session chosen, in place of `esc end the choice`.
- **The Log says `unpriced`** where a move's context was written again on a model the price table
  can't price, as the session's page does (`· 182k cached again, unpriced`); today the Log says
  nothing of the cost there.
- **Back-to-back holds on Runway:** the earlier stretch says when it lifts, and only the last says
  `back`: `at its limit 14:00 · lifts 19:00   at its cap 14:00 · back Mon 21:00`, in place of
  `back Mon 21:00` on both.
- **An event's headline names the windows it began with:** `work reached its Fable week limit` at
  09:00, and the event's details list a window that joined later with its time, `5-hour · joined
  14:00`. Today the headline names the joined window too, as though reached at the event's start.
- **Cords never cross within an account** on Sessions' Switchboard: each account's jacks take the
  order of its calls. Today, where the two orders differ, one cord bends across another. If the
  design's patch rules don't allow it, back to the owner with why.
- **History's Year scrolls whole where it doesn't fit,** and says what's out of view, as every other
  view does. Today a short terminal shows BY MONTH's first rows and cuts the rest, with no line and
  no scrolling: at 160×24, October to December, the other nine months out of reach. Its signed-off
  frames fit at their sizes, so they don't change.
- **Quota checks are left out of worth everywhere.** The day's summary prices their usage while the
  views over a session's lines don't, a fraction of a cent a day apart, seen only in the JSON's
  exact worth. Checks aren't work done for you, as the Log's hiding them says, so no view prices
  them.

## On screen

- `↓ answering` stays frozen while the router is silent, until it's heard from again.
- A manual reset lifts its hold a little early.
- The routing card's foot is cut on short terminals.
- On Sessions, at its step 5 in a short terminal, an account with 17 or more calls can't show them
  all.
- COMING UP names the limit at a back that's a refusal's.
- The Overview's USAGE at 20 columns or fewer, scrolled.
- A session's page under 39 columns, below the phone's floor.
- A cent between a session's STORY, summed from its parts, and TURNS' total, each rounded.
- Click targets at 1 to 30 columns.
- The Log's panel names a session by the lines the Log holds, so across midnight it can name it
  otherwise than the List does.
- Sessions' Switchboard at 14 rows, where an account's sessions sit above its line: its scroll line
  counts an account more than it hides, and paging down cuts a call. An account's first row is
  taken as its line's or its group's, whichever is higher; the cords' layout needs its rule for an
  account's rows settled first.
- `↑` and `↓` on Sessions' Switchboard don't scroll it to the session chosen.
- The Overview at 14 rows with every section open: USAGE is left its heading alone, or nothing.
- At 11 to 13 rows: Compare says `▼ 25 more below` with nothing shown, Detail's `↑↓ choose` is
  clipped, and a phone's Days pages nothing.
- A view whose scroll changes what it counts, rows or accounts, goes back to its top; keeping the
  reader's place would take a mapping between the two both ways. Where a change of unit was still
  pending as a key moved the focus, the reset undoes that move for one key.
- `historyScrolling` ignores the offset asked of it for the Year, Weeks and Days; its one caller
  asks at the scroll as it stands.
- Help doesn't say a view goes back to its top when its scroll's unit changes.
- A limit that lifts early while a cap or a refused token holds the account writes no room, so the
  next limit is dated from the first one's start. Bringing the lifted limit's `Until` forward to
  the lift, as a refusal lifting early already does, would end it.
- Runway's live 5-hour view, with no event telling a hold, dates a cap that one window began and
  another took over from the window holding it now: the readings tell each window's own run alone.
- `switchboard dashboard --help`'s keys paragraph leaves out the keys of Accounts, Sessions, the
  Log and a session's page; the dashboard's own help has them all.
- With the theme picker open, the page narrows and ROUTING's traces keep their oldest minutes,
  dropping the newest and the `/min` rate, so a busy account's trace can read empty. A narrower
  trace should drop its oldest minutes. Seen in `art/theme-picker.webp` and `art/themes.webp`.

## Costs

- A session's page is built again every tenth of a second and on each progress event.
- The traffic copies every request in the stream on each batch (`took`); a map the views keep from
  batch to batch would take only what changed.
- In a busy hour, the traffic's tick copies the requests map about once a second, and the views
  hear the whole stream again for each copy. Tidying once a minute would change what shows: the
  views keep a request the router told of until its line comes, whatever its age, so they'd first
  need told requests let go by time.
- The Log's panel is built again at each look, as its events side reads every look; and a change to
  a row on the stream builds the Requests again over every line held, where keeping the lines
  placed as the read grows would take only the change.
- Events filtered by kind walk every day.
- capture's tests take 7 to 15 minutes under the race detector.

## Cleanups

- The tail's carry rule is chosen in capture's two readers as well as in readings.
- `views.Running.Models` is read by nothing but tests.
- `claude.Policy` and `claude.SharedWindows` are exported variables any package could reassign.
