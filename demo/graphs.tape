# A still for the README's grid: Graphs, drawn from the fixture named after
# its signed-off frame: the burn-down cards, a card an account over its own
# window, the week's and then the 5-hour's: work at its cap till 17:10,
# personal's and side's weeks running out at their pace, and the pool's room
# under them.
#
# Record it, and make art/graphs.webp:
#   demo/record.sh graphs

Output "demo/out/graphs/"

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
Type "TZ=UTC demo/out/capturetool --fixture graphs-160x37"
Enter
Wait+Screen /SWITCHBOARD/
Sleep 1s
Show

Sleep 600ms

Hide
Type "q"
