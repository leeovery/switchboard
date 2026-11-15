package views

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// A day's digest keeps a scratchpad's session's directory and project as its
// lines named them, so the session restored from it reads its project as
// one read whole does, through the rule as it reads now: which is why the
// rule needs no new version of the digests.
func TestASessionRestoredFromADigestReadsItsScratchpadsProject(t *testing.T) {
	pad := "/scratch/tmp/claude-501/-Users-you-Code-api/5c0a0000-0000-4000-8000-000000001000/scratchpad"
	day, end := digestDay()
	tests := []struct {
		name, dir, project string
	}{
		{name: "a line from before projects", dir: pad},
		{name: "a line recording the scratchpad as its project, as v0.9.0's do", dir: pad, project: pad},
	}
	machine := fstest.MapFS{"Users/you/Code/api/.git": &fstest.MapFile{Mode: fs.ModeDir}}
	projects := status.NewProjects(status.ProjectsConfig{Root: machine, Home: "/Users/you", UID: 501})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const id = "5c0a0000-0000-4000-8000-000000000001"
			line := ledger.Line{
				At: day.Start.Add(10 * time.Hour).UTC(), Request: "r1", Session: id, Kind: ledger.KindMessage, Dir: tt.dir, Project: tt.project,
				Account: "work", Model: foldOpus, Status: 200, Class: ledger.ClassMain,
			}
			f, err := foldedAsOf(id, ledger.SessionFrom{Lines: []ledger.Line{line}}, nil, day)
			if err != nil {
				t.Fatal(err)
			}
			held, err := heldAsOf(id, ledger.SessionFrom{Base: baseOf(day.Date, f.State)}, nil, ledger.Pricing, Today(end), instantOf(end), false)
			if err != nil {
				t.Fatal(err)
			}
			listed := newLister(nil, false, Stream{}, projects, end).listed(id, held.story)
			if listed.Dir != tt.dir || listed.Project != "~/Code/api" {
				t.Errorf("the session restored is in %q of %q, want in %q of ~/Code/api", listed.Dir, listed.Project, tt.dir)
			}
		})
	}
}
