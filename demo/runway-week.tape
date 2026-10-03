# A still for the README's grid: Runway over the week, once work has reached
# its limit in the routing scenario: when each account has room, personal's
# week reaching its reserve on Saturday, back as it resets on Sunday.
#
# Record it, and make art/runway-week.webp:
#   demo/record.sh runway-week

Output "demo/out/runway-week/"

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
Sleep 14500ms
Tab
Tab
Type "w"
Sleep 1700ms
Show

Sleep 600ms

Hide
Type "q"
