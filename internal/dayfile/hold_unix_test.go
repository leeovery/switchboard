//go:build unix

package dayfile

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

// replacings are the ways a day's plain file is replaced or removed, holding
// it: each replaces the plain file of rewriteDate, of the one line
// {"a":"old"}, in f.
var replacings = []struct {
	name    string
	replace func(f *Files) error
	// then is what the day's files hold once a line, {"a":"late"}, another
	// process appended as the plain file was replaced, has landed.
	then map[dayFile]string
}{
	{
		name: "rewritten",
		replace: func(f *Files) error {
			_, err := f.Rewrite(rewriteDate, toNew)
			return err
		},
		then: map[dayFile]string{plainFile(rewriteDate): linesOf(`{"a":"new"}`, `{"a":"late"}`)},
	},
	{
		name:    "compressed",
		replace: func(f *Files) error { return f.compressHeld(rewriteDate) },
		then:    map[dayFile]string{compressedFile(rewriteDate): linesOf(`{"a":"old"}`), plainFile(rewriteDate): linesOf(`{"a":"late"}`)},
	},
}

func TestALineAnotherProcessAppendsAsADaysPlainFileIsReplacedLandsInTheFileAfter(t *testing.T) {
	for _, tt := range replacings {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			f, other := requestLedger(dir), requestLedger(dir)
			put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old"}`)))
			appended := make(chan error, 1)
			f.replacing = func(dayFile) {
				go func() { appended <- other.Append(Lines{rewriteDate: []byte(linesOf(`{"a":"late"}`))}) }()
				select {
				case err := <-appended:
					appended <- err
					t.Errorf("the other process appended (%v) as the plain file was about to be replaced, want it to wait", err)
				case <-time.After(100 * time.Millisecond):
				}
			}
			if err := tt.replace(f); err != nil {
				t.Fatalf("replacing the plain file = %v", err)
			}
			if err := <-appended; err != nil {
				t.Fatalf("Append() = %v", err)
			}
			for file, want := range tt.then {
				if got := heldIn(t, f, file); got != want {
					t.Errorf("%s holds %q, want %q: the line appended meanwhile lands after those replaced", file.name(f.Prefix), got, want)
				}
			}
		})
	}
}

func TestADaysPlainFileIsHeldFromOtherProcessesAsItsReplaced(t *testing.T) {
	for _, tt := range replacings {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			path := f.path(plainFile(rewriteDate))
			put(t, path, []byte(linesOf(`{"a":"old"}`)))
			var tried error
			f.replacing = func(dayFile) {
				file, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = file.Close() }()
				tried = syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
			}
			if err := tt.replace(f); err != nil {
				t.Fatalf("replacing the plain file = %v", err)
			}
			if !errors.Is(tried, syscall.EWOULDBLOCK) {
				t.Errorf("another process holding the plain file as it was replaced got %v, want EWOULDBLOCK: it's held", tried)
			}
		})
	}
}

func TestAnAppendOpensAfreshAFileRenamedIntoPlaceAsItWaitedToHoldIt(t *testing.T) {
	f := requestLedger(t.TempDir())
	path := f.path(plainFile(rewriteDate))
	put(t, path, []byte(linesOf(`{"a":"old"}`)))
	rewriter, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(rewriter.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	appended := make(chan error, 1)
	go func() { appended <- f.Append(Lines{rewriteDate: []byte(linesOf(`{"a":"late"}`))}) }()
	select {
	case err := <-appended:
		appended <- err
		t.Errorf("Append() = %v while another process held the file, want it to wait", err)
	case <-time.After(100 * time.Millisecond):
	}
	put(t, path+".rewritten", []byte(linesOf(`{"a":"new"}`)))
	if err := os.Rename(path+".rewritten", path); err != nil {
		t.Fatal(err)
	}
	if err := rewriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-appended; err != nil {
		t.Fatalf("Append() = %v", err)
	}
	if got, want := heldIn(t, f, plainFile(rewriteDate)), linesOf(`{"a":"new"}`, `{"a":"late"}`); got != want {
		t.Errorf("the plain file holds %q, want the line appended to the file renamed into place, %q", got, want)
	}
}
