# A still for the README's grid: t opens the theme picker over the Overview,
# in the routing scenario, and the cursor, three down from nord, on
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
Set Theme { "name": "nord", "background": "#2E3440", "foreground": "#D8DEE9", "cursor": "#2E3440", "selection": "#434C5E", "black": "#3B4252", "red": "#BF616A", "green": "#A3BE8C", "yellow": "#EBCB8B", "blue": "#81A1C1", "magenta": "#B48EAD", "cyan": "#88C0D0", "white": "#E5E9F0", "brightBlack": "#4C566A", "brightRed": "#BF616A", "brightGreen": "#A3BE8C", "brightYellow": "#EBCB8B", "brightBlue": "#81A1C1", "brightMagenta": "#B48EAD", "brightCyan": "#8FBCBB", "brightWhite": "#ECEFF4" }
Set WaitTimeout 2m

Hide
# In UTC, as the frames are drawn, whatever the recording machine's zone.
Type "TZ=UTC demo/out/capturetool --scenario routing"
Enter
Wait+Screen /SWITCHBOARD/
Sleep 3s
Type "t"
Sleep 300ms
# terminal, tokyo-night, then tokyo-night-day.
Down 3
# Shown once its keycaps have gone, 1.5s after the last.
Sleep 1700ms
Show

Sleep 600ms

Hide
Type "q"
