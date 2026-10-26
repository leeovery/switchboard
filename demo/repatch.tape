# A clip for the README's grid: Sessions' Switchboard as personal reaches its
# limit. capturetool plays the repatch scenario (internal/capture/scenarios.go)
# from 16:05:26: 41e0's request reaches personal's limit at 14.5s, refused at
# 14.8s, the 429 bouncing back red along its cord, and goes on to side, its
# old cord hanging red from personal's jack at the look by 20s; 9c2e asks at
# 21s and follows it, the second cord hanging, a cell shorter, by 25s.
#
# Record it, and make art/repatch.webp and art/repatch.mp4:
#   demo/record.sh repatch

Output "demo/out/repatch/"

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
# Sessions, the Switchboard, its keycaps gone by 2s.
Tab
Tab
Tab
Sleep 2s
Show

# Hidden at 27s, the second cord hanging since the look by 25s.
Sleep 25s

Hide
Type "q"
