package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The kinds of file a frame is kept as, by its stem, in a directory of
// frames: its colours and its text.
const (
	ansiExt = ".ansi"
	textExt = ".txt"
)

// WriteFrame writes the fixture's frame, at its own size, into dir, as a
// frame is kept there: <name>.ansi, as drawn, but for the newline after its
// last row, so cat in a terminal of its size doesn't scroll it; and
// <name>.txt, its text, as Plain gives it.
func (f Fixture) WriteFrame(dir string) error {
	frame := f.Frame(f.Size)
	if err := os.WriteFile(filepath.Join(dir, f.Name+ansiExt), []byte(strings.TrimSuffix(frame, "\n")), 0o644); err != nil {
		return fmt.Errorf("write %s's colours: %w", f.Name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, f.Name+textExt), []byte(Plain(frame)), 0o644); err != nil {
		return fmt.Errorf("write %s's text: %w", f.Name, err)
	}
	return nil
}

// FramesIn are the stems of the frames dir keeps, by their text files,
// sorted.
func FramesIn(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*"+textExt))
	if err != nil {
		return nil, fmt.Errorf("list the frames in %s: %w", dir, err)
	}
	stems := make([]string, len(paths))
	for i, p := range paths {
		stems[i] = strings.TrimSuffix(filepath.Base(p), textExt)
	}
	return stems, nil
}

// Plain is a frame as text alone: its escape codes stripped, and each line's
// trailing spaces trimmed, each ending in a newline.
func Plain(frame string) string {
	lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(ansi.Strip(line), " ")
	}
	return strings.Join(lines, "\n") + "\n"
}
