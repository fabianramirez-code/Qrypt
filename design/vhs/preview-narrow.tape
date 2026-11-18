# Qrypt narrow flow (one panel) on sample data.
# Build first: go build -o qrypt ./cmd/qrypt
# Record:      vhs design/vhs/preview-narrow.tape
Output design/vhs/out/preview-narrow.gif

Require ./qrypt

Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 18
Set Width 900
Set Height 620
Set Padding 12
Set TypingSpeed 60ms

Hide
Type "./qrypt --preview"
Enter
Sleep 1s
Show

# First screen with the animated logo.
Sleep 3s
Enter
Sleep 2s

# Spaces, then a space's folders and items.
Down
Sleep 1s
Enter
Sleep 1500ms
Enter
Sleep 1500ms
Enter
Sleep 2s

# Help, then leave.
Type "?"
Sleep 3s
Escape
Sleep 1s

Ctrl+C
Sleep 500ms
