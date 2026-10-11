# The page-log fixture on a phone, 52 × 36: the six tabs on a row of their
# own, under the clock, the Log's lit, as capturetool runs it.
# Run from the repository's root: vhs testdata/vhs/page-log-phone.tape

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 15
Set LetterSpacing 0
Set LineHeight 1
# 52 columns: 52 × 9 + 65. 36 rows: 36 × 20 + 72.
Set Width 533
Set Height 792
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 30
Set CursorBlink false
Set Theme { "name": "nord", "background": "#2E3440", "foreground": "#D8DEE9", "cursor": "#2E3440", "selection": "#434C5E", "black": "#3B4252", "red": "#BF616A", "green": "#A3BE8C", "yellow": "#EBCB8B", "blue": "#81A1C1", "magenta": "#B48EAD", "cyan": "#88C0D0", "white": "#E5E9F0", "brightBlack": "#4C566A", "brightRed": "#BF616A", "brightGreen": "#A3BE8C", "brightYellow": "#EBCB8B", "brightBlue": "#81A1C1", "brightMagenta": "#B48EAD", "brightCyan": "#8FBCBB", "brightWhite": "#ECEFF4" }
Set WaitTimeout 2m

Hide
Type "go run ./cmd/capturetool --fixture page-log"
Enter
Wait+Screen /14:42/
Show
Sleep 500ms
Screenshot "testdata/vhs/page-log-phone.png"
Sleep 500ms
