package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	// pollEvery is how often Follow looks for new lines unless told otherwise.
	pollEvery = 500 * time.Millisecond
	// tailChunk is how much of a log lastLines reads at a time, back from its end.
	tailChunk = 64 << 10
)

// Tail writes the last n lines of the log at path to w. When the log holds
// fewer, the lines before them come from the file it was last rolled over to.
func Tail(w io.Writer, path string, n int) error {
	file, err := openToRead(path)
	if err != nil {
		return err
	}
	if file == nil && !exists(rotated(path, 1)) {
		return fmt.Errorf("no log at %s", path)
	}
	defer closeFile(file)
	return writeTail(w, file, path, n)
}

// Follow writes the last n lines of the log at path to w, as Tail does, then
// each line added after them as it comes, until ctx is done. It looks for new
// lines every interval given, or every half second when that's zero. When the
// file at path is replaced, as when it's rolled over, or shrinks, it carries
// on from the top of the file that's there; and it waits for a log that
// doesn't exist yet.
func Follow(ctx context.Context, w io.Writer, path string, n int, every time.Duration) error {
	file, err := openToRead(path)
	if err != nil {
		return err
	}
	f := &follower{path: path, w: w, file: file}
	defer f.close()
	if err := writeTail(w, file, path, n); err != nil {
		return err
	}
	if every <= 0 {
		every = pollEvery
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := f.poll(); err != nil {
				return err
			}
		}
	}
}

// writeTail writes the last n lines of file, the log at path, which is nil
// when there's none; and before them, when there are fewer, the last lines of
// the file the log was last rolled over to. It leaves file just past the last
// byte it read, to follow on from there.
func writeTail(w io.Writer, file *os.File, path string, n int) error {
	var lines []string
	if file != nil {
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("read log: %w", err)
		}
		if lines, err = lastLines(file, info.Size(), n); err != nil {
			return err
		}
		if _, err := file.Seek(info.Size(), io.SeekStart); err != nil {
			return fmt.Errorf("read log: %w", err)
		}
	}
	if short := n - len(lines); short > 0 {
		older, err := lastLinesOf(rotated(path, 1), short)
		if err != nil {
			return err
		}
		lines = append(older, lines...)
	}
	_, err := io.WriteString(w, strings.Join(lines, ""))
	return err
}

// lastLinesOf returns the last n lines of the file at path, or none when
// there's no such file.
func lastLinesOf(path string, n int) ([]string, error) {
	file, err := openToRead(path)
	if file == nil {
		return nil, err
	}
	defer closeFile(file)
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}
	return lastLines(file, info.Size(), n)
}

// lastLines returns the last n lines of the first size bytes of r, oldest
// first, each with its line ending. It reads back from the end a chunk at a
// time, so a long log costs no more than the lines asked for.
func lastLines(r io.ReaderAt, size int64, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	var chunks [][]byte
	newlines := 0
	start := size
	for start > 0 && newlines <= n {
		chunk := make([]byte, min(start, tailChunk))
		start -= int64(len(chunk))
		if _, err := r.ReadAt(chunk, start); err != nil {
			return nil, fmt.Errorf("read log: %w", err)
		}
		chunks = append(chunks, chunk)
		newlines += bytes.Count(chunk, []byte("\n"))
	}
	slices.Reverse(chunks)
	lines := strings.SplitAfter(string(bytes.Join(chunks, nil)), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if start > 0 {
		// It began part way into the file, maybe part way into a line.
		lines = lines[1:]
	}
	return lines[max(len(lines)-n, 0):], nil
}

// follower copies what's added to a log to w as it comes.
type follower struct {
	path string
	w    io.Writer
	// file is the log as last opened, read as far as it's been copied: nil
	// until there's a log to open.
	file *os.File
}

// poll copies what's been added to the log since the last poll. When the file
// at the path is no longer the one open, it copies the rest of the one open,
// which may have taken records as it was replaced, then the new one from the
// top. When the file has shrunk, it copies it from the top again.
func (f *follower) poll() error {
	if err := f.copy(); err != nil {
		return err
	}
	named, err := os.Stat(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	if f.file != nil {
		if open, err := f.file.Stat(); err == nil && os.SameFile(open, named) {
			return f.rewindIfShrunk(named.Size())
		}
	}
	return f.reopen()
}

// reopen moves on to the file at the path, and copies it from the top.
func (f *follower) reopen() error {
	file, err := openToRead(f.path)
	if file == nil {
		return err
	}
	f.close()
	f.file = file
	return f.copy()
}

// rewindIfShrunk copies the open file from the top again when it's now
// shorter than where copying reached, as after it's been truncated.
func (f *follower) rewindIfShrunk(size int64) error {
	at, err := f.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	if size >= at {
		return nil
	}
	if _, err := f.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	return f.copy()
}

// copy copies the open file from where copying reached to its end.
func (f *follower) copy() error {
	if f.file == nil {
		return nil
	}
	if _, err := io.Copy(f.w, f.file); err != nil {
		return fmt.Errorf("follow log: %w", err)
	}
	return nil
}

func (f *follower) close() {
	closeFile(f.file)
}

// openToRead opens the file at path to read. It returns no file and no error
// when there's no such file.
func openToRead(path string) (*os.File, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}
	return file, nil
}

// closeFile closes a file opened only to read, if one was: an error closing
// it loses nothing.
func closeFile(file *os.File) {
	if file != nil {
		_ = file.Close()
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
