# A clip for the README's grid: side's card flipped to its sessions, as the
# routing scenario has them once work's limit has moved its sessions there,
# each streaming, waiting or idle, then one picked out with ↓.
#
# Record it, and make art/flipped.webp and art/flipped.mp4:
#   demo/record.sh flipped

Output "demo/out/flipped/"

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 17
Set LetterSpacing 0
Set LineHeight 1
# 160 × 34 cells of 10 × 22 px.
Set Width 1660
Set Height 792
Set Padding 16
Set CursorBlink false
Set Framerate 30
Set Theme { "name": "nord", "background": "#2E3440", "foreground": "#D8DEE9", "cursor": "#2E3440", "selection": "#434C5E", "black": "#3B4252", "red": "#BF616A", "green": "#A3BE8C", "yellow": "#EBCB8B", "blue": "#81A1C1", "magenta": "#B48EAD", "cyan": "#88C0D0", "white": "#E5E9F0", "brightBlack": "#4C566A", "brightRed": "#BF616A", "brightGreen": "#A3BE8C", "brightYellow": "#EBCB8B", "brightBlue": "#81A1C1", "brightMagenta": "#B48EAD", "brightCyan": "#8FBCBB", "brightWhite": "#ECEFF4" }
Set WaitTimeout 2m

Hide
Type "demo/out/capturetool --scenario routing"
Enter
Wait+Screen /SWITCHBOARD/
# Past work's limit, to the traffic on side from 24s, side's card flipped.
Sleep 24s
Right
Right
Right
Space
Sleep 1600ms
Show

Sleep 1100ms
Down
Sleep 2000ms

Hide
Type "q"
