package setuptoken

import (
	"os"
	"os/exec"
	"testing"
)

func TestShimHandsBackTheAddressItsRunWith(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	s, err := newShim()
	if err != nil {
		t.Fatal(err)
	}
	defer s.remove()
	if _, ok := s.address(); ok {
		t.Fatal("address() found one before the shim ran, want none")
	}
	handed := signIn + "&quote='\"&dollar=$HOME&space=a b"

	if out, err := exec.Command(s.browser(), handed).CombinedOutput(); err != nil {
		t.Fatalf("the shim failed: %v: %s", err, out)
	}

	if got, ok := s.address(); !ok || got != handed {
		t.Errorf("address() = %q, %v; want %q, as handed", got, ok, handed)
	}
}

func TestShimIsPrivate(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	s, err := newShim()
	if err != nil {
		t.Fatal(err)
	}
	defer s.remove()
	for path, want := range map[string]os.FileMode{s.dir: os.ModeDir | 0o700, s.browser(): 0o700} {
		if info, err := os.Stat(path); err != nil || info.Mode() != want {
			t.Errorf("%s is %v (%v), want %v", path, info.Mode(), err, want)
		}
	}
}

func TestShimIsRemovedWithAllInIt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	s, err := newShim()
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(s.browser(), signIn).CombinedOutput(); err != nil {
		t.Fatalf("the shim failed: %v: %s", err, out)
	}

	s.remove()

	if left, err := os.ReadDir(tmp); err != nil || len(left) > 0 {
		t.Errorf("the temporary directory holds %d (%v), want the shim gone", len(left), err)
	}
}
