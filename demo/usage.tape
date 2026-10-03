# The README's one-account clip: work alone, at 160 × 27. capturetool plays
# the usage scenario (internal/capture/scenarios.go) through the real
# dashboard, in real time, with every seam faked.
#
# The wide card: the burn-down from work's history, the dotted projection to
# where it reaches its reserve, COMING UP and RECENT beside it, and 7f3a,
# idle as it starts, lighting its dot as it goes to work. Then g, the burn
# rate, and g again, the hourglass, its sand falling while work is busy; then
# tab past Sessions' list to Runway, the day ahead.
#
# Record it, and make art/usage.webp and art/usage.mp4:
#   demo/record.sh usage

Output "demo/out/usage/"

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 17
Set LetterSpacing 0
Set LineHeight 1
# 160 × 27 cells of 10 × 22 px.
Set Width 1660
Set Height 640
Set Padding 16
Set CursorBlink false
Set Framerate 30
Set Theme { "name": "nord", "background": "#2E3440", "foreground": "#D8DEE9", "cursor": "#2E3440", "selection": "#434C5E", "black": "#3B4252", "red": "#BF616A", "green": "#A3BE8C", "yellow": "#EBCB8B", "blue": "#81A1C1", "magenta": "#B48EAD", "cyan": "#88C0D0", "white": "#E5E9F0", "brightBlack": "#4C566A", "brightRed": "#BF616A", "brightGreen": "#A3BE8C", "brightYellow": "#EBCB8B", "brightBlue": "#81A1C1", "brightMagenta": "#B48EAD", "brightCyan": "#8FBCBB", "brightWhite": "#ECEFF4" }
Set WaitTimeout 2m

Hide
Type "demo/out/capturetool --scenario usage"
Enter
Wait+Screen /SWITCHBOARD/
Show

# The card, 7f3a's dot lighting at the look after it asks, at 2.5s.
Sleep 6500ms
Type "g"
# Its burn rate, use each 10 minutes.
Sleep 3500ms
Type "g"
# Its hourglass, sand falling while the account is busy.
Sleep 4500ms
Tab
# Sessions, one account's list, on the way to Runway.
Sleep 1500ms
Tab
Sleep 4500ms

Hide
Type "q"
