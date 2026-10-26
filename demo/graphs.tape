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
Set Theme { "name": "tokyo-night", "background": "#0B0C14", "foreground": "#C0CAF5", "cursor": "#0B0C14", "selection": "#28243A", "black": "#15161E", "red": "#F7768E", "green": "#9ECE6A", "yellow": "#E0AF68", "blue": "#7AA2F7", "magenta": "#BB9AF7", "cyan": "#7DCFFF", "white": "#A9B1D6", "brightBlack": "#414868", "brightRed": "#F7768E", "brightGreen": "#9ECE6A", "brightYellow": "#E0AF68", "brightBlue": "#7AA2F7", "brightMagenta": "#BB9AF7", "brightCyan": "#7DCFFF", "brightWhite": "#C0CAF5" }
Set WaitTimeout 2m

Hide
# In UTC, as the frames are drawn, whatever the recording machine's zone.
Type "TZ=UTC demo/out/capturetool --theme tokyo-night --fixture graphs-160x37"
Enter
Wait+Screen /SWITCHBOARD/
Sleep 1s
Show

Sleep 600ms

Hide
Type "q"
