# The README's lead: a tour of the dashboard's pages, a moment on each.
# capturetool plays the routing scenario (internal/capture/scenarios.go)
# through the real dashboard, in real time, with every seam faked.
#
# The Overview of the auto world at 14:42:07; then shift-tab, back round the
# tabs: the Log's Requests, where ↑ chooses the newest line, db8a's, sent at
# 7s, the next at 8s, and s opens db8a's page; Graphs, its burn-down cards;
# Runway; Sessions' Switchboard, its cords carrying the requests; History's
# Year; Accounts' Compare; and the Overview, b3e9 started on side since the
# look by 5s.
#
# The Log and the session's page come first, while the request stream the
# Overview holds open has told of every request: a page without it, such as
# Runway, lets the stream go, and a request that ends meanwhile writes no
# line in a scenario, so where the stream opens again the Log and a
# session's page would draw it lost.
#
# Record it, and make art/tour.webp and art/tour.mp4:
#   demo/record.sh tour

Output "demo/out/tour/"

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
Type "TZ=UTC demo/out/capturetool --theme tokyo-night --scenario routing"
Enter
Wait+Screen /SWITCHBOARD/
Show

# b3e9 starts on side at 2.5s, its requests on the traces.
Sleep 4500ms
# The Log's Requests.
Shift+Tab
Sleep 2900ms
# db8a's line, the newest from 7s to 8s, chosen, then its session's page.
Up
Sleep 700ms
Type "s"
Sleep 3500ms
# Graphs, from the page over the Log: WEEK's and 5-HOUR's cards.
Shift+Tab
Sleep 3s
# Runway.
Shift+Tab
Sleep 3s
# Sessions' Switchboard, requests travelling the cords.
Shift+Tab
Sleep 4s
# History's Year.
Shift+Tab
Sleep 3s
# Accounts' Compare.
Shift+Tab
Sleep 3s
# Round to the Overview, as the clip began.
Shift+Tab
Sleep 2500ms

Hide
Type "q"
