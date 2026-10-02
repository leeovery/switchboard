# Calibration: the accounts-3-auto frame's ANSI dump, cat in a vhs terminal at
# the calibrated settings, to compare with the frame's PNG.
# Run from the repository's root: vhs testdata/vhs/calibrate-accounts-3-auto.tape

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 15
Set LetterSpacing 0
Set LineHeight 1
# 160 columns: 160 × 9 + 65. 34 rows: 34 × 20 + 72.
Set Width 1505
Set Height 752
Set Padding 16
Set WindowBar Colorful
Set WindowBarSize 30
Set CursorBlink false
Set Theme { "name": "nord", "background": "#2E3440", "foreground": "#D8DEE9", "cursor": "#2E3440", "selection": "#434C5E", "black": "#3B4252", "red": "#BF616A", "green": "#A3BE8C", "yellow": "#EBCB8B", "blue": "#81A1C1", "magenta": "#B48EAD", "cyan": "#88C0D0", "white": "#E5E9F0", "brightBlack": "#4C566A", "brightRed": "#BF616A", "brightGreen": "#A3BE8C", "brightYellow": "#EBCB8B", "brightBlue": "#81A1C1", "brightMagenta": "#B48EAD", "brightCyan": "#8FBCBB", "brightWhite": "#ECEFF4" }

Hide
Type "clear; printf '\033[?25l'; cat testdata/vhs/reference/accounts-3-auto.ansi; sleep 60"
Enter
Sleep 2s
Show
Sleep 500ms
Screenshot "testdata/vhs/calibrate-accounts-3-auto.png"
Sleep 500ms
