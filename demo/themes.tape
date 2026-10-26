# The README's themes clip: t opens the theme picker over the Overview, and
# each theme the cursor reaches redraws the dashboard in it, live. It
# previews tokyo-night-day, terminal on the way past, nord, exchange and
# amber, and esc puts tokyo-night back. capturetool plays the
# routing scenario (internal/capture/scenarios.go) behind it: no key pins
# anything.
#
# Record it, and make art/themes.webp and art/themes.mp4:
#   demo/record.sh themes

Output "demo/out/themes/"

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
Show

Sleep 1200ms
Type "t"
# tokyo-night, the theme in force.
Sleep 1000ms
Down
# tokyo-night-day, for a light terminal.
Sleep 1400ms
Up
Sleep 250ms
Up
# terminal, on the way to nord.
Sleep 500ms
Up
Sleep 1300ms
Up
# exchange, then amber.
Sleep 1300ms
Up
Sleep 1400ms
Escape
# tokyo-night again, as it was.
Sleep 1000ms

Hide
Type "q"
