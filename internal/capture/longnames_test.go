package capture

import (
	"path"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

func TestLongNamesLabelsEveryAccountOfTheSceneLong(t *testing.T) {
	scene, long := worldsNow().scene(), worldsNow().longNames()
	if !slices.Equal(long.accounts.IDs(), scene.accounts.IDs()) {
		t.Fatalf("long names' accounts are %v, want the scene's %v", long.accounts.IDs(), scene.accounts.IDs())
	}
	for _, a := range long.accounts {
		if cells := utf8.RuneCountInString(a.Label); cells < 12 || cells > 22 {
			t.Errorf("%s is labelled %q, %d cells, want 12 to 22", a.ID, a.Label, cells)
		}
	}
}

func TestLongNamesLeavesTheScenesAccountsAsTheyAre(t *testing.T) {
	_ = worldsNow().longNames()
	for _, a := range worldsNow().scene().accounts {
		if a.Label != a.ID {
			t.Errorf("the scene's %s is labelled %q after long names were built, want its own", a.ID, a.Label)
		}
	}
}

func TestLongNamesRunsEverySessionInADirectoryTooLongForTheLog(t *testing.T) {
	long, now := worldsNow().longNames(), worldsNow().now
	tooShort := func(dir string) bool { return utf8.RuneCountInString(path.Base(dir)) <= views.SessionDirCells }
	listed := long.routed(now)
	if len(listed) != len(worldsNow().scene().routed(now)) {
		t.Fatalf("long names lists %d sessions, want the scene's %d", len(listed), len(worldsNow().scene().routed(now)))
	}
	for _, s := range listed {
		if i := slices.IndexFunc(s.Assignments, func(a status.Assignment) bool { return tooShort(a.Dir) }); i >= 0 {
			t.Errorf("listed %s runs in %q, want a directory longer than %d cells", s.ID, s.Assignments[i].Dir, views.SessionDirCells)
		}
	}
	lines := long.records().lines
	if len(lines) != len(worldsNow().scene().records().lines) {
		t.Fatalf("long names' ledger holds %d lines, want the scene's %d", len(lines), len(worldsNow().scene().records().lines))
	}
	for _, l := range lines {
		if l.Dir != "" && tooShort(l.Dir) {
			t.Errorf("line %s of %s runs in %q, want none or one longer than %d cells", l.Request, l.Session, l.Dir, views.SessionDirCells)
		}
	}
}
