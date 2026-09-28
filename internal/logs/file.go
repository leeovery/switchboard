package logs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const (
	// maxSize is how large a log grows before it's rolled over.
	maxSize = 10 << 20
	// keep is how many rolled-over files a log keeps: .1, the newest, to .5.
	keep = 5

	dirMode  = 0o700
	fileMode = 0o600
)

// logFile appends records to a log, rolling it over to .1 before a record
// would take it past its cap. Several processes can share a log, as every
// CLI command writes cli.log: each record goes out in a single appending
// write, so none is ever split around another, and when another process
// rolls the log over, the next write moves on to the new file. Nothing is
// buffered, so a record is safe as soon as it's logged, even if the process
// execs or exits straight after.
type logFile struct {
	path    string
	maxSize int64
	keep    int

	mu     sync.Mutex
	file   *os.File
	closed bool
}

// openFile opens the role's log in dir, which it creates if it must.
func openFile(dir string, role Role) (*logFile, error) {
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("log directory %q isn't an absolute path", dir)
	}
	return openLog(role.Path(dir), maxSize, keep)
}

// openLog opens the log at path to append to, rolling it over first if it's
// full.
func openLog(path string, maxSize int64, keep int) (*logFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	l := &logFile{path: path, maxSize: maxSize, keep: keep}
	file, err := l.open()
	if err != nil {
		return nil, err
	}
	l.file = file
	return l, nil
}

// Write appends p to the log in a single write.
func (l *logFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, os.ErrClosed
	}
	if err := l.makeRoom(int64(len(p))); err != nil {
		return 0, err
	}
	return l.file.Write(p)
}

// Close closes the log. Writes after it fail.
func (l *logFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return l.file.Close()
}

// makeRoom readies the log for n more bytes. It moves on to the file at the
// path when that's no longer the one open, as when another process has rolled
// it over or it's been deleted; and it rolls the log over when n more bytes
// would take it past its cap, unless it's empty.
func (l *logFile) makeRoom(n int64) error {
	info, err := l.file.Stat()
	if err != nil || !l.isAtPath(info) {
		if err := l.reopen(); err != nil {
			return err
		}
		if info, err = l.file.Stat(); err != nil {
			return fmt.Errorf("write log: %w", err)
		}
	}
	if info.Size() > 0 && info.Size()+n > l.maxSize {
		l.roll(info)
		return l.reopen()
	}
	return nil
}

// isAtPath reports whether the file described by info, the one open, is
// still the file at the log's path.
func (l *logFile) isAtPath(info os.FileInfo) bool {
	named, err := os.Stat(l.path)
	return err == nil && os.SameFile(info, named)
}

// reopen opens the file at the log's path in place of the one open. When it
// can't, the one open stays, for the next write to try again.
func (l *logFile) reopen() error {
	file, err := l.open()
	if err != nil {
		return err
	}
	_ = l.file.Close()
	l.file = file
	return nil
}

// open opens the file at the log's path to append to, first rolling it over
// if it's full.
func (l *logFile) open() (*os.File, error) {
	if info, err := os.Stat(l.path); err == nil && info.Size() >= l.maxSize {
		l.roll(info)
	}
	file, err := os.OpenFile(l.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, fileMode)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	return file, nil
}

// roll renames the log to .1, first moving each rolled-over file up one,
// which drops the oldest; unless full, the file found full, is no longer the
// one at the path, because another process has rolled it over first.
// Processes sharing a log take turns to roll it over, holding a lock on its
// directory, since two at once can rename one's new log over the other's
// rolled-over one. A log that can't be rolled over is still written, past
// its cap, rather than losing records.
func (l *logFile) roll(full os.FileInfo) {
	unlock := lockDir(filepath.Dir(l.path))
	defer unlock()
	if named, err := os.Stat(l.path); err != nil || !os.SameFile(named, full) {
		return
	}
	for n := l.keep - 1; n > 0; n-- {
		_ = os.Rename(rotated(l.path, n), rotated(l.path, n+1))
	}
	_ = os.Rename(l.path, rotated(l.path, 1))
}

// rotated is where the log at path goes once it has been rolled over n times.
func rotated(path string, n int) string {
	return path + "." + strconv.Itoa(n)
}

// tee writes each record to every one of its writers, whether or not the
// others take it.
type tee []io.Writer

func (t tee) Write(p []byte) (int, error) {
	var errs []error
	for _, w := range t {
		if _, err := w.Write(p); err != nil {
			errs = append(errs, err)
		}
	}
	return len(p), errors.Join(errs...)
}
