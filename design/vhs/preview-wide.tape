# Qrypt wide mode (three panes) on sample data.
# Build first: go build -o qrypt ./cmd/qrypt
# Record:      vhs design/vhs/preview-wide.tape
Output design/vhs/out/preview-wide.gif

Require ./qrypt

Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 16
Set Width 1400
Set Height 800
Set Padding 12
Set TypingSpeed 60ms

Hide
Type "./qrypt --preview"
Enter
Sleep 1s
Show

# First screen: language, theme, icons, animation.
Sleep 2s
Tab
Down
Sleep 1500ms
Up
Sleep 1s
Enter
Sleep 2s

# Spaces and folders tree, then the items of a folder.
Down
Sleep 1s
Tab
Sleep 1s
Down
Sleep 1s
Tab
Sleep 1500ms

# Reveal a hidden field in the inspector, then hide it again.
Enter
Sleep 2s
Enter
Sleep 1s

# Help for the current screen.
Type "?"
Sleep 3s
Escape
Sleep 1s

Ctrl+C
Sleep 500ms
