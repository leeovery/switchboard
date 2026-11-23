# Past caps judged by the reserve recorded then

Each line of the readings history now carries `reserve`: the reserve the router held the account
to as it read. A day's minutes at cap are judged by it, so changing a reserve no longer rewrites
history on Accounts.

Other readers of past readings still judge a cap by the reserve in the config today:

- Runway's and Trends' past lanes (`internal/views/lanespast.go`), where a past column's cap band
  is drawn;
- the pool's past room (`internal/views/poolroom.go`);
- the capacity verdict's "its peak at its cap" in weeks gone (`internal/views/capacity.go`);
- the router's seed at restart, which judges pools' room in the past for the forecast.

After a reserve change, these redraw the past by the new reserve while Accounts' minutes at cap
keep the old one, so the two can disagree about the same stretch. Each would read the line's
`reserve` where it's there and fall back to today's where it isn't, as minutes at cap do.

Found by the reviews of the minutes-at-cap fix, 23 November 2026.
