# The README's one-account clip: work alone, its sessions busy. capturetool
# plays the dashboard scenario (internal/capture/scenarios.go) through the
# real dashboard, in real time, with every seam faked.
#
# The Overview of work alone at 14:42:07: USAGE, ROUTING's trace, COMING UP
# with its cap about 16:05, and LATELY; then g, GRAPHS; then tab four times,
# to Runway's 5-hour, work's lane alone, the hour past from the readings,
# its cap at its pace, sooner than 16:05 as its sessions keep busy, and back
# at 17:10.
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
Set Theme { "name": "nord", "background": "#2E3440", "foreground": "#D8DEE9", "cursor": "#2E3440", "selection": "#434C5E", "black": "#3B4252", "red": "#BF616A", "green": "#A3BE8C", "yellow": "#EBCB8B", "blue": "#81A1C1", "magenta": "#B48EAD", "cyan": "#88C0D0", "white": "#E5E9F0", "brightBlack": "#4C566A", "brightRed": "#BF616A", "brightGreen": "#A3BE8C", "brightYellow": "#EBCB8B", "brightBlue": "#81A1C1", "brightMagenta": "#B48EAD", "brightCyan": "#8FBCBB", "brightWhite": "#ECEFF4" }
Set WaitTimeout 2m

Hide
# In UTC, as the frames are drawn, whatever the recording machine's zone.
Type "TZ=UTC demo/out/capturetool --scenario dashboard"
Enter
Wait+Screen /SWITCHBOARD/
Show

# Work's sessions asking, a request a second.
Sleep 6s
Type "g"
# GRAPHS, work's windows charted.
Sleep 5s
Tab
Sleep 300ms
Tab
Sleep 300ms
Tab
Sleep 300ms
Tab
Sleep 300ms
Type ">"
# Runway's 5-hour from 12.2s, > from the week it opens on; work's 5-hour read
# up to 59% at 14.1s, shown at the look by 15s.
Sleep 7800ms

Hide
Type "q"
