# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.2] - 2026-09-30

✨ Added
- Pin several accounts at once with `switchboard pin work side` — new sessions go to the best of them until all are out, and the dashboard's `1`–`9` keys now toggle accounts in and out of the pin.
- `switchboard usage --refresh` (`-r`) has the router read every account it may first, so a limit you reset by hand on claude.ai is seen at once.
- The router notices the Mac waking from sleep and sends requests upstream on fresh connections, so connections a sleep left dead no longer fail requests.
- The router restarts itself when the Mac's time zone changes, so the priming schedule keeps to the clock.
- Token files can be links to files kept elsewhere — switchboard writes a token through the link to where it leads.

🔧 Changed
- Accounts are primed 5 seconds after their 5-hour window resets, whether or not they were in use, so a prime always lands after the reset.
- Accounts that can take no request (limit reached, week spent, token refused) are no longer primed, and are probed instead so a manual reset is noticed.
- Account ids that differ only in case are refused, as macOS would give them one token file.
- The dashboard keeps showing the router's last view, saying since when, when the router stops answering, rather than probing every account at once.
- Usage readings off responses and repeated use of a session are saved to disk at most once a minute instead of every second or so.
- `claude` looks a moment later at a token file it finds empty before counting the account as having no token.
- `claude` version checks run with the real Claude Code's own directories first on `PATH`, and no longer hang on a CLI that leaves a process running.

🗑️ Removed
- `accounts remove` no longer deletes the file a linked token file leads to — only the link goes, and the command says so.

🐛 Fixed
- A 429 without usage headers is passed straight to Claude Code instead of being treated as the account's limit, retried or replayed.
- A session is remembered only once a request of it succeeds, so `claude --resume`'s startup quota check no longer leaves a stray assignment.
- A limit lifted early — by a manual reset or a request that then succeeds — is now noticed instead of holding the account back until its stated reset.
- A wrapper named `claude` that `exec`s switchboard, or another build of switchboard, no longer makes it start itself in a loop when looking for the real Claude Code.
- Repeated warnings about an ignored pin are logged once per session and account rather than on every request.
- Installing the service retries when launchd is still finishing unloading the old one, avoiding spurious "Input/output error" failures.
- Request failures that predate a wake from sleep no longer count against the router's health.
- Unknown log names are redacted in the error `switchboard logs` prints.

## [0.0.1] - 2026-09-29

Initial release.
