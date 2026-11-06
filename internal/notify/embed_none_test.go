//go:build !notifyapp

package notify

import "testing"

func TestABuildWithoutTheTagHasNoApp(t *testing.T) {
	desktop := NewDesktop(func(string) string { return "" }, func() (string, error) { return t.TempDir(), nil })
	if desktop.app != nil {
		t.Errorf("a build without the tag notifyapp has an app of %d bytes, want none", len(desktop.app.zipped))
	}
}
