package service_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/routercopy"
	"github.com/leeovery/switchboard/internal/service"
)

// proxyVariable is a variable added to a plist by hand, which only the plist
// in place sets.
const proxyVariable = "\t\t<key>HTTPS_PROXY</key>\n\t\t<string>http://proxy.example:8080</string>\n"

func TestMoveOverWritesAPlistFromBeforeTheCopyAfreshToRunIt(t *testing.T) {
	s := newSetup(t, map[string]string{"XDG_STATE_HOME": "<root>/state"}, nil)
	s.launchctl.refuse = true
	written, err := s.svc.PlistOf(service.InstallOptions{Executable: s.binary, Config: "/Users/tester/work.toml", LogLevel: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	// As install writes it now, ExitTimeOut and ProcessType among its keys,
	// with a variable added by hand.
	want := strings.Replace(string(written), "\t</dict>\n\t<key>RunAtLoad</key>", proxyVariable+"\t</dict>\n\t<key>RunAtLoad</key>", 1)
	s.putPlist(t, beforeTheCopy(want))
	running := writeVersion(t, filepath.Join(s.root, "Cellar", "switchboard", "1.2.3", "bin", "switchboard"), "switchboard 1.2.3")
	var restarted []string

	err = s.svc.MoveOver(t.Context(), moving(running, func(own routercopy.Copy) error {
		restarted = append(restarted, own.Path())
		if plist, err := os.ReadFile(s.plist); err != nil || string(plist) != want {
			t.Errorf("as the router restarts into its copy, the plist reads\n%s(%v)\nwant it written afresh already:\n%s", plist, err, want)
		}
		return nil
	}))

	if err != nil {
		t.Fatalf("MoveOver() error = %v", err)
	}
	if want := []string{s.routerCopy}; !slices.Equal(restarted, want) {
		t.Errorf("restarted the router into %q, want its copy, once: %q", restarted, want)
	}
	if plist, err := os.ReadFile(s.plist); err != nil || string(plist) != want {
		t.Errorf("the plist reads\n%s(%v)\nwant it as install writes it, running the copy with --from, every other key and variable kept:\n%s", plist, err, want)
	}
	if copied, err := os.ReadFile(s.routerCopy); err != nil || string(copied) != "switchboard 1.2.3" {
		t.Errorf("the router's copy holds %q (%v), want a copy of the binary the router runs", copied, err)
	}
	if matches, err := routercopy.At(s.routerCopy).Matches(running); err != nil || !matches {
		t.Errorf("the router's copy matches the binary it runs: %v (%v), want it last modified as that was", matches, err)
	}
}

func TestMoveOverWithoutAPlistMakesNoCopy(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true

	err := s.svc.MoveOver(t.Context(), moving(s.binary, refuseRestart(t)))

	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MoveOver() error = %v, want fs.ErrNotExist", err)
	}
	checkNoCopy(t, s)
}

func TestMoveOverLeavesAPlistItCantReadAsItWas(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	const plist = "bplist00\xd1\x01\x02"
	s.putPlist(t, plist)

	if err := s.svc.MoveOver(t.Context(), moving(s.binary, refuseRestart(t))); err == nil {
		t.Error("MoveOver() succeeded, want it to fail, the plist unread without plutil")
	}
	checkPlistAsItWas(t, s, plist)
	checkNoCopy(t, s)
}

func TestMoveOverLeavesThePlistAsItWasWhereTheCopyCantBeMade(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	plist := s.plistBeforeTheCopy(t)
	// The copy's directory there, its lock with it, but nothing more can be
	// written into it, as on a full disk.
	unlock, err := routercopy.At(s.routerCopy).Lock()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	readOnly(t, filepath.Dir(s.routerCopy))

	if err := s.svc.MoveOver(t.Context(), moving(s.binary, refuseRestart(t))); err == nil || errors.Is(err, service.ErrPutOff) {
		t.Errorf("MoveOver() error = %v, want it to fail, the router's copy not made", err)
	}
	checkPlistAsItWas(t, s, plist)
	checkNoCopy(t, s)
}

func TestMoveOverLeavesThePlistAsItWasWhereItCantBeWritten(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	plist := s.plistBeforeTheCopy(t)
	readOnly(t, filepath.Dir(s.plist))

	if err := s.svc.MoveOver(t.Context(), moving(s.binary, refuseRestart(t))); err == nil {
		t.Error("MoveOver() succeeded, want it to fail, the plist not written")
	}
	checkPlistAsItWas(t, s, plist)
}

func TestMoveOverPutsThePlistBackWhereTheRouterCantRestartIntoTheCopy(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	plist := s.plistBeforeTheCopy(t)
	refused := errors.New("exec refused")

	err := s.svc.MoveOver(t.Context(), moving(s.binary, func(routercopy.Copy) error { return refused }))

	if !errors.Is(err, refused) {
		t.Errorf("MoveOver() error = %v, want the restart's", err)
	}
	checkPlistAsItWas(t, s, plist)
}

func TestMoveOverIsPutOffWhereTheLinkMovedOnFromTheBinaryRunning(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	plist := s.plistBeforeTheCopy(t)
	running := writeVersion(t, filepath.Join(s.root, "Cellar", "switchboard", "1.0", "bin", "switchboard"), "switchboard 1.0")
	next := writeVersion(t, filepath.Join(s.root, "Cellar", "switchboard", "1.1", "bin", "switchboard"), "switchboard 1.1")
	link := filepath.Join(s.root, "opt", "bin", "switchboard")
	relinkTo(t, link, running)
	// Found as the router started, before another upgrade moved the link on.
	m := moving(link, refuseRestart(t))
	relinkTo(t, link, next)

	if err := s.svc.MoveOver(t.Context(), m); !errors.Is(err, service.ErrPutOff) {
		t.Errorf("MoveOver() error = %v, want service.ErrPutOff: the link leads to a version that isn't running", err)
	}
	checkPlistAsItWas(t, s, plist)
	checkNothingCopied(t, s)
}

func TestMoveOverIsPutOffWhileTheCopysLockIsHeld(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	plist := s.plistBeforeTheCopy(t)
	unlock, err := routercopy.At(s.routerCopy).Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	if err := s.svc.MoveOver(t.Context(), moving(s.binary, refuseRestart(t))); !errors.Is(err, service.ErrPutOff) {
		t.Errorf("MoveOver() error = %v, want service.ErrPutOff: install holds the copy's lock", err)
	}
	checkPlistAsItWas(t, s, plist)
	checkNothingCopied(t, s)
}

func TestMoveOverIsPutOffWhereThePlistKeepsTheCopyElsewhere(t *testing.T) {
	s := newSetup(t, map[string]string{"XDG_DATA_HOME": "<root>/data"}, nil)
	s.launchctl.refuse = true
	// Installed from a shell keeping switchboard's data elsewhere since the
	// router started.
	plist := s.plistBeforeTheCopy(t)
	cfg := s.cfg
	cfg.Getenv = func(key string) string {
		if key == "XDG_DATA_HOME" {
			return ""
		}
		return s.cfg.Getenv(key)
	}

	if err := newService(t, cfg).MoveOver(t.Context(), moving(s.binary, refuseRestart(t))); !errors.Is(err, service.ErrPutOff) {
		t.Errorf("MoveOver() error = %v, want service.ErrPutOff: the copy the plist keeps isn't the one whose lock it holds", err)
	}
	checkPlistAsItWas(t, s, plist)
	checkNoCopy(t, s)
}

func TestInstallWaitsForAMoveHoldingTheCopysLock(t *testing.T) {
	s := newSetup(t, nil, upOnceStarted(4242))
	unlock, err := routercopy.At(s.routerCopy).Lock()
	if err != nil {
		t.Fatal(err)
	}
	installed := make(chan error, 1)

	go func() {
		_, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary})
		installed <- err
	}()

	select {
	case err := <-installed:
		t.Fatalf("Install() = %v while a move held the copy's lock, want it waiting", err)
	case <-time.After(200 * time.Millisecond):
	}
	if _, err := os.Stat(s.plist); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the plist while a move held the copy's lock: %v, want none written yet", err)
	}
	checkNoCopy(t, s)
	unlock()
	select {
	case err := <-installed:
		if err != nil {
			t.Fatalf("Install() error = %v once the move let the lock go", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("gave up waiting for Install() once the move let the lock go")
	}
	s.checkPlist(t, s.binary, "")
}

// moving is what a router running binary, as it stands now, moves over with,
// restarting into its copy as restart does.
func moving(binary string, restart func(routercopy.Copy) error) service.MovingOver {
	return service.MovingOver{Binary: binary, Runs: router.Watch(binary).Is, Restart: restart}
}

// relinkTo has the link at path lead to target, replacing any link there
// whole, as an upgrade moves a link on.
func relinkTo(t *testing.T, path, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	moved := path + ".new"
	if err := os.Symlink(target, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, path); err != nil {
		t.Fatal(err)
	}
}

// checkNothingCopied checks the copy's directory holds nothing but its
// lock: no copy, and nothing staged left beside where it goes.
func checkNothingCopied(t *testing.T, s *setup) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(s.routerCopy))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != ".lock" {
			t.Errorf("the router's copy's directory holds %s, want nothing copied", e.Name())
		}
	}
}

// plistBeforeTheCopy puts the plist install wrote before the router's copy
// in the service's place, running the setup's binary itself, and returns it.
func (s *setup) plistBeforeTheCopy(t *testing.T) string {
	t.Helper()
	written, err := s.svc.PlistOf(service.InstallOptions{Executable: s.binary})
	if err != nil {
		t.Fatal(err)
	}
	plist := beforeTheCopy(string(written))
	s.putPlist(t, plist)
	return plist
}

// refuseRestart is a restart into the router's copy that fails the test,
// the router never to restart.
func refuseRestart(t *testing.T) func(routercopy.Copy) error {
	return func(own routercopy.Copy) error {
		t.Errorf("restarted the router into its copy, %s, want it carrying on as it is", own.Path())
		return errors.New("no restart here")
	}
}

// checkPlistAsItWas checks the plist in place reads as it was.
func checkPlistAsItWas(t *testing.T, s *setup, was string) {
	t.Helper()
	if plist, err := os.ReadFile(s.plist); err != nil || string(plist) != was {
		t.Errorf("the plist reads\n%s(%v)\nwant it as it was:\n%s", plist, err, was)
	}
}

// checkNoCopy checks the router's copy isn't there.
func checkNoCopy(t *testing.T, s *setup) {
	t.Helper()
	if _, err := os.Stat(s.routerCopy); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the router's copy: %v, want none made", err)
	}
}

// writeVersion writes a program at path holding content, and returns path.
func writeVersion(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// readOnly has nothing written into the directory dir till the test ends.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}
