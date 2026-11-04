package service_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

	err = s.svc.MoveOver(t.Context(), running, func(own routercopy.Copy) error {
		restarted = append(restarted, own.Path())
		if plist, err := os.ReadFile(s.plist); err != nil || string(plist) != want {
			t.Errorf("as the router restarts into its copy, the plist reads\n%s(%v)\nwant it written afresh already:\n%s", plist, err, want)
		}
		return nil
	})

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

	err := s.svc.MoveOver(t.Context(), s.binary, refuseRestart(t))

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

	if err := s.svc.MoveOver(t.Context(), s.binary, refuseRestart(t)); err == nil {
		t.Error("MoveOver() succeeded, want it to fail, the plist unread without plutil")
	}
	checkPlistAsItWas(t, s, plist)
	checkNoCopy(t, s)
}

func TestMoveOverLeavesThePlistAsItWasWhereTheCopyCantBeMade(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	plist := s.plistBeforeTheCopy(t)
	// A file where the copy's directory goes.
	writeBinary(t, filepath.Dir(s.routerCopy))

	if err := s.svc.MoveOver(t.Context(), s.binary, refuseRestart(t)); err == nil {
		t.Error("MoveOver() succeeded, want it to fail, the router's copy not made")
	}
	checkPlistAsItWas(t, s, plist)
}

func TestMoveOverLeavesThePlistAsItWasWhereItCantBeWritten(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	plist := s.plistBeforeTheCopy(t)
	readOnly(t, filepath.Dir(s.plist))

	if err := s.svc.MoveOver(t.Context(), s.binary, refuseRestart(t)); err == nil {
		t.Error("MoveOver() succeeded, want it to fail, the plist not written")
	}
	checkPlistAsItWas(t, s, plist)
}

func TestMoveOverPutsThePlistBackWhereTheRouterCantRestartIntoTheCopy(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.refuse = true
	plist := s.plistBeforeTheCopy(t)
	refused := errors.New("exec refused")

	err := s.svc.MoveOver(t.Context(), s.binary, func(routercopy.Copy) error { return refused })

	if !errors.Is(err, refused) {
		t.Errorf("MoveOver() error = %v, want the restart's", err)
	}
	checkPlistAsItWas(t, s, plist)
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
