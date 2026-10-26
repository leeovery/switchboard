# The dashboard guide's one-account clip: work alone, its sessions busy.
# capturetool plays the dashboard scenario (internal/capture/scenarios.go)
# through the real dashboard, in real time, with every seam faked.
#
# The Overview of work alone at 14:42:07: USAGE, ROUTING's trace, COMING UP
# with its cap about 16:05, and LATELY; then shift-tab twice, past the Log,
# to Graphs, work's week and 5-hour as burn-down cards; then shift-tab to
# Runway and > to its 5-hour, work's lane alone, the hour past from the
# readings, its cap at its pace, sooner than 16:05 as its sessions keep busy,
# and back at 17:10.
#
# Record it, and make art/dashboard.webp and art/dashboard.mp4:
#   demo/record.sh dashboard

Output "demo/out/dashboard/"

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 17
Set LetterSpacing 0
Set LineHeight 1
# 160 × 37 cells of 10 × 22 px: the frames' size.
Set Width 1660
Set Height 858
Set Padding 16
Set CursorBlink false
Set Framerate 30
Set Theme { "name": "tokyo-night", "background": "#0B0C14", "foreground": "#C0CAF5", "cursor": "#0B0C14", "selection": "#28243A", "black": "#15161E", "red": "#F7768E", "green": "#9ECE6A", "yellow": "#E0AF68", "blue": "#7AA2F7", "magenta": "#BB9AF7", "cyan": "#7DCFFF", "white": "#A9B1D6", "brightBlack": "#414868", "brightRed": "#F7768E", "brightGreen": "#9ECE6A", "brightYellow": "#E0AF68", "brightBlue": "#7AA2F7", "brightMagenta": "#BB9AF7", "brightCyan": "#7DCFFF", "brightWhite": "#C0CAF5" }
Set WaitTimeout 2m

Hide
# In UTC, as the frames are drawn, whatever the recording machine's zone.
Type "TZ=UTC demo/out/capturetool --theme tokyo-night --scenario dashboard"
Enter
Wait+Screen /SWITCHBOARD/
Show

# Work's sessions asking, a request a second.
Sleep 6s
Shift+Tab
Sleep 300ms
Shift+Tab
# Graphs from 6.3s, work's week and 5-hour cards.
Sleep 5s
Shift+Tab
Sleep 300ms
Type ">"
# Runway's 5-hour from 11.6s, > from the week it opens on; work's 5-hour read
# up to 59% at 14.1s, shown at the look by 15s.
Sleep 8400ms

Hide
Type "q"
