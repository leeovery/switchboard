# The Log: every request and event, as it happens

The Log shows the router's traffic, live and back through the days. Look at it to see what a
request cost, why a session moved, or what the router did while you were away. It has two tables
on sub-tabs, Requests and Events, with the newest line at the foot.

<div align="center">
<img src="../../art/log-requests.webp" alt="The Log's Requests, following the newest: a line a request, with its time, session and directory, model, account, status, time to first byte and in all, tokens read from the cache, written to it and put out, why its answer stopped, and a new session's routing, the requests in flight at the foot" width="840">
</div>

*Requests, following the newest line, with the requests in flight at the foot.*

Press `[` and `]` to move between the tables. Between two days, a rule sums the day that ended:
its requests, sessions, limits reached and worth.

## Requests: a line a request

Requests has a line for each request the router routes. Each line shows:

- when the request was made;
- the session, model and account;
- the status;
- the time to the first byte, and in all;
- the tokens by kind;
- why the answer stopped;
- how the router chose the account, with the cost of a move.

## Events: a line a decision

Events has a line for each thing the router decides or notices: a session starting, a limit, a
move, a window primed or reset. Every move the router makes is there, with the reason, for example
`personal reached its limit   61k cached again, $0.61`.

## Choosing what shows

Press `f` to open a panel that sets the columns and which lines show. It can add these columns to
Requests:

- the worth at API prices;
- the input tokens;
- the attempts;
- the size, messages, tools and thinking budget;
- the client that sent the request;
- the router's id for the request.

The panel can also show the quota checks and token counts that Claude Code sends. These stay hidden
until you choose them, because neither is work done for you.

In the panel:

- `←` and `→` move between its sides.
- `↑` and `↓` move through the ticks on a side.
- `space` ticks the one the cursor is on.
- `enter` applies the ticks, and `esc` closes the panel unchanged.
- `/` closes the panel and opens the find.

## Finding a session

Press `/` to find a session by the start of its id, the name of its folder, or its
[session name](sessions.md). The find looks at today's sessions first, then back a day at a time.

You type in it as in the fields of `switchboard setup`:

- `←`, `→`, `home` and `end` move the cursor;
- `backspace` and `delete` take out a character;
- a paste goes in at the cursor.

Then `↑`, `↓` and `enter` pick a session, and both tables filter to it. To clear the filter, press
`esc` with nothing open while the table follows.

## Keys on the Log

| Key | Does |
|---|---|
| `↑` `↓` | Chooses a line. While following, `↑` chooses the newest line and pauses the table. With details open, moves to the line before or after. |
| `end` | Follows again: the newest line at the foot, and nothing chosen. |
| `enter` | Opens the details of the chosen line. |
| `s` | Opens [the session page](session-page.md) for the session on the chosen line. Does nothing on an account event. |
| `g` | On the details of an event, shows Requests, paused at the moment of the event. |
| `f` | Opens the panel that sets what the Log shows. |
| `/` | Finds a session. |
| `esc` | With nothing open: while the table is paused, follows again, as `end` does. Otherwise, with a filter on, clears the filter. |

The keys every page shares are on [Every page](every-page.md).
