// Package logstest captures what switchboard logs, for tests.
package logstest

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/logs"
)

// Log is what a test has logged since it called Capture.
type Log struct {
	t    testing.TB
	path string
}

// Capture logs every record from debug up to a file of the test's own, from
// now until the test ends. Logging is the process's, so a test that captures
// it mustn't run in parallel with another that logs.
func Capture(t testing.TB) *Log {
	t.Helper()
	dir := t.TempDir()
	logs.Init(logs.Options{Dir: dir, Role: logs.RoleCLI, Level: slog.LevelDebug})
	t.Cleanup(func() { logs.Close(0) })
	return &Log{t: t, path: logs.RoleCLI.Path(dir)}
}

// String returns everything logged so far.
func (l *Log) String() string {
	l.t.Helper()
	data, err := os.ReadFile(l.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		l.t.Fatalf("read the log: %v", err)
	}
	return string(data)
}

// Lines returns the lines logged so far, without their line endings.
func (l *Log) Lines() []string {
	l.t.Helper()
	logged := l.String()
	if logged == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(logged, "\n"), "\n")
}

// Has reports whether a line logged so far contains every one of parts, such
// as "level=WARN", `msg="probe failed"` and "account=side".
func (l *Log) Has(parts ...string) bool {
	l.t.Helper()
	for _, line := range l.Lines() {
		if containsAll(line, parts) {
			return true
		}
	}
	return false
}

func containsAll(line string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(line, part) {
			return false
		}
	}
	return true
}
