package atomicfile_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

func TestSetAsideKeepsTheNewestThreeOfAName(t *testing.T) {
	now := time.Unix(1790933400, 0)
	aside := func(at int64) string { return fmt.Sprintf("state.json.corrupt-%d", at) }
	tests := []struct {
		name string
		// before are the names in the directory before the file's set aside,
		// beside it, and want those after.
		before, want []string
	}{
		{
			name: "the first",
			want: []string{aside(now.Unix())},
		},
		{
			name:   "the third",
			before: []string{aside(100), aside(200)},
			want:   []string{aside(100), aside(200), aside(now.Unix())},
		},
		{
			name:   "the fourth, removing the oldest, by the times their names give",
			before: []string{aside(200), aside(1000), aside(30)},
			want:   []string{aside(1000), aside(200), aside(now.Unix())},
		},
		{
			name:   "one of many, as a build before this one left them",
			before: []string{aside(1), aside(2), aside(3), aside(4), aside(5)},
			want:   []string{aside(4), aside(5), aside(now.Unix())},
		},
		{
			name: "leaving alone what isn't one of the file's set aside",
			before: []string{
				aside(1), aside(2), aside(3),
				"prefs.json.corrupt-1", "state.json.corrupt-soon", "state.json.corrupt-1.bak", "state.json.1", "state.json.corrupt-",
			},
			want: []string{
				aside(2), aside(3), aside(now.Unix()),
				"prefs.json.corrupt-1", "state.json.corrupt-soon", "state.json.corrupt-1.bak", "state.json.1", "state.json.corrupt-",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tt.before {
				write(t, filepath.Join(dir, name))
			}
			path := filepath.Join(dir, "state.json")
			if err := os.WriteFile(path, []byte("{\"sessi"), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := atomicfile.SetAside(path, now)
			if want := filepath.Join(dir, aside(now.Unix())); got != want || err != nil {
				t.Fatalf("SetAside() = %q, %v; want %q", got, err, want)
			}
			if data, err := os.ReadFile(got); err != nil || string(data) != "{\"sessi" {
				t.Errorf("%s holds %q (%v), want the file set aside as it was", got, data, err)
			}
			if names := namesIn(t, dir); !slices.Equal(names, sorted(tt.want)) {
				t.Errorf("the directory holds %q, want %q", names, sorted(tt.want))
			}
		})
	}
}

func TestSetAsideLeavesEveryOtherAsItWasWhenTheFileCantGo(t *testing.T) {
	dir := t.TempDir()
	before := []string{"state.json.corrupt-1", "state.json.corrupt-2", "state.json.corrupt-3", "state.json.corrupt-4"}
	for _, name := range before {
		write(t, filepath.Join(dir, name))
	}

	got, err := atomicfile.SetAside(filepath.Join(dir, "state.json"), time.Unix(1790933400, 0))
	if got != "" || err == nil {
		t.Errorf("SetAside() of a file that isn't there = %q, %v; want no file set aside, and why", got, err)
	}
	if names := namesIn(t, dir); !slices.Equal(names, before) {
		t.Errorf("the directory holds %q, want what was there, as it was: %q", names, before)
	}
}

// namesIn returns the names of what dir holds, in order.
func namesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// sorted returns names in order.
func sorted(names []string) []string {
	return slices.Sorted(slices.Values(names))
}
