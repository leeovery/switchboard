# A still for the Claude and switchboard page: usage's printout, drawn from
# the fixture named after its signed-off frame, usage-160: every account's
# windows as switchboard reports them, each with its use, the rate it can
# spend at and still last, and when it resets or runs out, as Claude reads
# them from status.
#
# A printout prints once, into the scrollback, so the tape clears the screen
# before it and holds the shell after it, its prompt unshown.
#
# Record it, and make art/claude.webp:
#   demo/record.sh claude

Output "demo/out/claude/"

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 17
Set LetterSpacing 0
Set LineHeight 1
# 160 × 14 cells of 10 × 22 px: the printout's 13 rows, and the row the
# shell holds after it.
Set Width 1660
Set Height 352
Set Padding 16
Set CursorBlink false
Set Framerate 30
Set Theme { "name": "tokyo-night", "background": "#0B0C14", "foreground": "#C0CAF5", "cursor": "#0B0C14", "selection": "#28243A", "black": "#15161E", "red": "#F7768E", "green": "#9ECE6A", "yellow": "#E0AF68", "blue": "#7AA2F7", "magenta": "#BB9AF7", "cyan": "#7DCFFF", "white": "#A9B1D6", "brightBlack": "#414868", "brightRed": "#F7768E", "brightGreen": "#9ECE6A", "brightYellow": "#E0AF68", "brightBlue": "#7AA2F7", "brightMagenta": "#BB9AF7", "brightCyan": "#7DCFFF", "brightWhite": "#C0CAF5" }
Set WaitTimeout 2m

Hide
# In UTC, as the frames are drawn, whatever the recording machine's zone.
Type "clear; TZ=UTC demo/out/capturetool --theme tokyo-night --fixture usage-160 --print --ansi; read -rs"
Enter
Wait+Screen /SWITCHBOARD/
Sleep 1s
Show

Sleep 600ms

Hide
Enter
