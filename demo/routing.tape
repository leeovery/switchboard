# The README's lead clip: three accounts, routed among. capturetool plays the
# routing scenario (internal/capture/scenarios.go) through the real dashboard,
# in real time, with every seam faked; the keys below are timed to its cues.
#
# Accounts: a new session starts on side, the best. Sessions: requests travel
# the cords, then work reaches its limit at 12s, the 429 bouncing back red, and
# its sessions re-patch to side, LOG saying why. Runway: the day, then w, the
# week. Accounts again: side's card flipped to its sessions, one picked out
# and hand-patched to personal with 2, which it goes to with its next request.
#
# Record it, and make art/routing.webp and art/routing.mp4:
#   demo/record.sh routing

Output "demo/out/routing/"

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
Show

# Accounts: b3e9 starts on side at 2.5s, and shows at the next look.
Sleep 8s
Tab
# Sessions: pulses out, answers shimmering back; work's limit at 12s.
Sleep 9300ms
Tab
# Runway, over the day, then the week.
Sleep 3s
Type "w"
Sleep 3200ms
Tab
# Accounts: the focus across to side, its card flipped, a session picked out
# and pinned to personal, which it goes to with its next request; then esc,
# done with it.
Sleep 1500ms
Right
Sleep 400ms
Right
Sleep 400ms
Right
Sleep 800ms
Space
Sleep 1200ms
Down
Sleep 1200ms
Type "2"
Sleep 4600ms
Escape
Sleep 1500ms

Hide
Type "q"
