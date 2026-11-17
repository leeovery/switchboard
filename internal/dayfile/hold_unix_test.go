//go:build unix

package dayfile

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestALineAnotherProcessAppendsAsADayIsRewrittenLandsInTheFileRewritten(t *testing.T) {
	dir := t.TempDir()
	f, other := requestLedger(dir), requestLedger(dir)
	put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old"}`)))
	appended := make(chan error, 1)
	f.renaming = func(dayFile) {
		go func() { appended <- other.Append(Lines{rewriteDate: []byte(linesOf(`{"a":"late"}`))}) }()
		select {
		case err := <-appended:
			appended <- err
			t.Errorf("the other process appended (%v) as the rewrite was about to rename its file into place, want it to wait", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	if err := <-appended; err != nil {
		t.Fatalf("Append() = %v", err)
	}
	if got, want := heldIn(t, f, plainFile(rewriteDate)), linesOf(`{"a":"new"}`, `{"a":"late"}`); got != want {
		t.Errorf("the plain file holds %q, want the line appended meanwhile after the lines rewritten, %q", got, want)
	}
}

func TestARewriteHoldsThePlainFileFromOtherProcessesAsItRenamesIt(t *testing.T) {
	f := requestLedger(t.TempDir())
	path := f.path(plainFile(rewriteDate))
	put(t, path, []byte(linesOf(`{"a":"old"}`)))
	var tried error
	f.renaming = func(dayFile) {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = file.Close() }()
		tried = syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	}
	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	if !errors.Is(tried, syscall.EWOULDBLOCK) {
		t.Errorf("another process holding the plain file as it was renamed got %v, want EWOULDBLOCK: the rewrite holds it", tried)
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
