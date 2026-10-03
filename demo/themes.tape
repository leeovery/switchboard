# The README's themes clip: t opens the theme picker over the Accounts view,
# and each theme the cursor reaches redraws the dashboard in it, live. It
# previews nord, terminal on the way past, tokyo-night and tokyo-night-day,
# then exchange and amber, and esc puts nord back. capturetool plays the
# themes scenario (internal/capture/scenarios.go): the routing scenario's
# accounts, at work, at 160 × 34.
#
# Record it, and make art/themes.webp and art/themes.mp4:
#   demo/record.sh themes

Output "demo/out/themes/"

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
Type "demo/out/capturetool --scenario themes"
Enter
Wait+Screen /SWITCHBOARD/
Show

Sleep 1200ms
Type "t"
# nord, the theme in force.
Sleep 1000ms
Down
# terminal, on the way to tokyo-night.
Sleep 500ms
Down
Sleep 1300ms
Down
# tokyo-night-day, for a light terminal.
Sleep 1400ms
Up
Sleep 250ms
Up
Sleep 250ms
Up
Sleep 250ms
Up
# exchange, then amber.
Sleep 1300ms
Up
Sleep 1400ms
Escape
# nord again, as it was.
Sleep 1000ms

Hide
Type "q"
