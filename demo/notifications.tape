# A still for the Notifications page: the Overview, playing the repatch
# scenario, once personal has reached its 5-hour limit and its sessions have
# moved to side: what a limit's notification tells of, in LATELY, and when
# personal is back, in COMING UP.
#
# Record it, and make art/notifications.webp:
#   demo/record.sh notifications

Output "demo/out/notifications/"

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
Type "TZ=UTC demo/out/capturetool --theme tokyo-night --scenario repatch"
Enter
Wait+Screen /SWITCHBOARD/
# personal reaches its limit at 14.5s, and its second session moves at 21s;
# the Overview tells of both at its next look at the router's document,
# every 5s.
Wait+Screen /reached its 5-hour limit: 2 sessions/
Sleep 1s
Show

Sleep 600ms

Hide
Type "q"
