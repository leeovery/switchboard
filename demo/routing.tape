# The README's lead clip: three accounts, routed among. capturetool plays the
# routing scenario (internal/capture/scenarios.go) through the real dashboard,
# in real time, with every seam faked; the keys below are timed to its cues.
#
# The Overview of the auto world at 14:42:07: work at its cap, personal and
# side, side the best; ROUTING auto, its traces; COMING UP and LATELY. b3e9
# starts on side at 2.5s. Then the routing card: p, 3 for side, m for new and
# running sessions, enter; personal's 41e0 and 9c2e move to side with their
# next requests, at 17s and 21s.
#
# Record it, and make art/routing.webp and art/routing.mp4:
#   demo/record.sh routing

Output "demo/out/routing/"

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
Type "TZ=UTC demo/out/capturetool --scenario routing"
Enter
Wait+Screen /SWITCHBOARD/
Show

# b3e9 starts on side at 2.5s, and shows at the look by 5s.
Sleep 9s
# The routing card.
Type "p"
Sleep 1500ms
# side picked.
Type "3"
Sleep 1500ms
# New and running sessions: the card counts 41e0 and 9c2e to move, at a cost.
Type "m"
Sleep 2500ms
# Pinned to side at 14.5s; 41e0 asks at 17s, 9c2e at 21s, each moving there,
# shown at the looks by 20s and 25s; hidden at 27s.
Enter
Sleep 12500ms

Hide
Type "q"
