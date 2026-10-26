# A still for the README's grid: t opens the theme picker over the Overview,
# in the routing scenario, and the cursor, one down from tokyo-night, on
# tokyo-night-day, previews it: the dashboard redrawn in a light theme.
#
# Record it, and make art/theme-picker.webp:
#   demo/record.sh theme-picker

Output "demo/out/theme-picker/"

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
Sleep 3s
Type "t"
Sleep 300ms
# tokyo-night-day.
Down
# Shown once its keycaps have gone, 1.5s after the last.
Sleep 1700ms
Show

Sleep 600ms

Hide
Type "q"
