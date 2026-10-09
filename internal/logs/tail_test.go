package logs_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs"
)

func TestDefaultPath(t *testing.T) {
	dir := t.TempDir()
	if got, want := logs.DefaultPath(dir), logs.RoleCLI.Path(dir); got != want {
		t.Errorf("before the router has logged, DefaultPath() = %s, want the CLI's, %s", got, want)
	}
	if err := os.WriteFile(logs.RoleRouter.Path(dir), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := logs.DefaultPath(dir), logs.RoleRouter.Path(dir); got != want {
		t.Errorf("once the router has logged, DefaultPath() = %s, want the router's, %s", got, want)
	}
}

func TestTail(t *testing.T) {
	tests := []struct {
		name string
		log  string
		// rolled are the files the log rolled over to, newest first.
		rolled []string
		noLog  bool
		n      int
		want   string
	}{
		{name: "the last lines", log: "one\ntwo\nthree\nfour\n", n: 2, want: "three\nfour\n"},
		{name: "every line when the log holds fewer", log: "one\ntwo\n", n: 5, want: "one\ntwo\n"},
		{name: "none", log: "one\ntwo\n", n: 0, want: ""},
		{name: "a last line without its ending", log: "one\ntwo\nthree", n: 2, want: "two\nthree"},
		{name: "nothing from an empty log", log: "", n: 5, want: ""},
		{
			name:   "reaching into the file it last rolled over to",
			log:    "four\nfive\n",
			rolled: []string{"one\ntwo\nthree\n"},
			n:      4,
			want:   "two\nthree\nfour\nfive\n",
		},
		{
			name:   "reaching no further than that",
			log:    "five\n",
			rolled: []string{"three\nfour\n", "one\ntwo\n"},
			n:      10,
			want:   "three\nfour\nfive\n",
		},
		{
			name:   "from the file it rolled over to when there's no log",
			rolled: []string{"one\ntwo\n"},
			noLog:  true,
			n:      1,
			want:   "two\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeLogs(t, tt.log, tt.rolled...)
			if tt.noLog {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer

			if err := logs.Tail(&out, path, tt.n); err != nil {
				t.Fatalf("Tail() error = %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("Tail() wrote %q, want %q", out.String(), tt.want)
			}
		})
	}
}

func TestTailOfALongLog(t *testing.T) {
	// 220KB: Tail reads it back from the end in 64KB chunks.
	const lines = 20000
	path := writeLogs(t, numberedLines(0, lines))
	for _, n := range []int{1, 3, 7000, lines, lines + 5000} {
		var out bytes.Buffer
		if err := logs.Tail(&out, path, n); err != nil {
			t.Fatalf("Tail(%d) error = %v", n, err)
		}
		if want := numberedLines(max(lines-n, 0), lines); out.String() != want {
			t.Errorf("Tail(%d) wrote %d bytes, want the last %d lines, %d bytes", n, out.Len(), n, len(want))
		}
	}
}

func TestTailWithoutALog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.log")

	err := logs.Tail(&bytes.Buffer{}, path, 50)
	if want := "no log at " + path; errorText(err) != want {
		t.Errorf("Tail() error = %q, want %q", errorText(err), want)
	}
}

func TestFollow(t *testing.T) {
	path := writeLogs(t, "one\ntwo\n")
	out, done := follow(t, path, 1)
	out.waitFor(t, "two\n")

	appendTo(t, path, "three\n")
	out.waitFor(t, "two\nthree\n")

	// The log rolls over. A process still holding the old file adds a record
	// to it before any goes to the new one.
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	appendTo(t, path+".1", "four\n")
	appendTo(t, path, "five\n")
	out.waitFor(t, "two\nthree\nfour\nfive\n")

	// It's truncated, and written again, shorter than it was.
	if err := os.WriteFile(path, []byte("six\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.waitFor(t, "two\nthree\nfour\nfive\nsix\n")

	if err := done(); err != nil {
		t.Errorf("Follow() error = %v, want none once its context ends", err)
	}
}

func TestFollowWaitsForALog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.log")
	out, done := follow(t, path, 50)

	appendTo(t, path, "listening\n")
	out.waitFor(t, "listening\n")
	if err := done(); err != nil {
		t.Errorf("Follow() error = %v", err)
	}
}

// follow follows the log at path from its last n lines, looking for new ones
// every few milliseconds. It returns what it has written, and what ends it,
// returning its error.
func follow(t *testing.T, path string, n int) (*syncBuffer, func() error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	out := &syncBuffer{}
	result := make(chan error, 1)
	go func() { result <- logs.Follow(ctx, out, path, n, 5*time.Millisecond) }()
	var once sync.Once
	var err error
	done := func() error {
		once.Do(func() {
			cancel()
			err = <-result
		})
		return err
	}
	t.Cleanup(func() { _ = done() })
	return out, done
}

// writeLogs writes a log in a directory of the test's own, and the files it
// rolled over to, newest first. It returns the log's path.
func writeLogs(t *testing.T, log string, rolled ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cli.log")
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	for i, content := range rolled {
		if err := os.WriteFile(fmt.Sprintf("%s.%d", path, i+1), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// numberedLines returns the lines numbered from first up to but not
// including last, such as "line 00007".
func numberedLines(first, last int) string {
	var b strings.Builder
	for i := first; i < last; i++ {
		fmt.Fprintf(&b, "line %05d\n", i)
	}
	return b.String()
}

func appendTo(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// syncBuffer is a buffer one goroutine can write while another reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitFor waits a few seconds at most for the buffer to hold exactly want.
func (b *syncBuffer) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for b.String() != want {
		if time.Now().After(deadline) {
			t.Fatalf("followed %q, want %q", b.String(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAFollowerPollsEveryIntervalGivenOrElseEveryHalfSecond(t *testing.T) {
	for _, tt := range []struct{ given, want time.Duration }{
		{given: 5 * time.Millisecond, want: 5 * time.Millisecond},
		{given: 0, want: 500 * time.Millisecond},
		{given: -time.Second, want: 500 * time.Millisecond},
	} {
		if got := logs.PollEvery(tt.given); got != tt.want {
			t.Errorf("PollEvery(%v) = %v, want %v", tt.given, got, tt.want)
		}
	}
}
