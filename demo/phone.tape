# A still for the README's grid: the routing scenario on a phone's terminal,
# 52 × 36, under 100 columns: the top of the Overview, the tabs on a row of
# their own, b3e9 started on side at 2.5s and listed at the look by 5s.
#
# Record it, and make art/phone.webp:
#   demo/record.sh phone

Output "demo/out/phone/"

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 17
Set LetterSpacing 0
Set LineHeight 1
# 52 × 36 cells of 10 × 22 px.
Set Width 578
Set Height 836
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
Sleep 6s
Show

Sleep 600ms

Hide
Type "q"
