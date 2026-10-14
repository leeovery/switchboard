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
Set Theme { "name": "nord", "background": "#2E3440", "foreground": "#D8DEE9", "cursor": "#2E3440", "selection": "#434C5E", "black": "#3B4252", "red": "#BF616A", "green": "#A3BE8C", "yellow": "#EBCB8B", "blue": "#81A1C1", "magenta": "#B48EAD", "cyan": "#88C0D0", "white": "#E5E9F0", "brightBlack": "#4C566A", "brightRed": "#BF616A", "brightGreen": "#A3BE8C", "brightYellow": "#EBCB8B", "brightBlue": "#81A1C1", "brightMagenta": "#B48EAD", "brightCyan": "#8FBCBB", "brightWhite": "#ECEFF4" }
Set WaitTimeout 2m

Hide
# In UTC, as the frames are drawn, whatever the recording machine's zone.
Type "TZ=UTC demo/out/capturetool --scenario repatch"
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
