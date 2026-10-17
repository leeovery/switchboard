package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// referenceDir is where the frames are kept, each by its stem, as the text
// and ANSI its fixture draws and a PNG.
const referenceDir = "../../testdata/vhs/reference"

func TestReferenceKeepsEachFrameAsTextANSIAndPNG(t *testing.T) {
	entries, err := os.ReadDir(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string][]string{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		stem := strings.TrimSuffix(e.Name(), ext)
		kinds[stem] = append(kinds[stem], ext)
	}
	for stem, exts := range kinds {
		slices.Sort(exts)
		if want := []string{".ansi", ".png", ".txt"}; !slices.Equal(exts, want) {
			t.Errorf("reference/ keeps %s as %v, want %v", stem, exts, want)
		}
	}
}

func TestEveryFrameIsDrawnByAFixture(t *testing.T) {
	names := Names()
	for _, stem := range stems(t) {
		if !slices.Contains(names, stem) {
			t.Errorf("no fixture draws %s: name one after it, or delete the frame", stem)
		}
	}
}

func TestEveryFramesANSIParsesToItsText(t *testing.T) {
	for _, stem := range stems(t) {
		t.Run(stem, func(t *testing.T) {
			g, text := reference(t, referenceDir, stem)
			if got := g.text(); !slices.Equal(got, text) {
				t.Errorf("its ANSI reads\n%s\nwant its .txt\n%s", strings.Join(got, "\n"), strings.Join(text, "\n"))
			}
			width, height := sizeOf(t, stem)
			if height != 0 && len(g) != height {
				t.Errorf("it's %d rows, want %d", len(g), height)
			}
			for r, row := range g {
				if len(row) > width {
					t.Errorf("row %d is %d columns, want %d at most", r+1, len(row), width)
				}
			}
		})
	}
}

func TestEveryFixtureNamedAfterAFrameDrawsItCellForCell(t *testing.T) {
	for _, stem := range stems(t) {
		t.Run(stem, func(t *testing.T) {
			t.Parallel()
			frame, _ := reference(t, referenceDir, stem)
			heldToItsFrame(t, stem, frame)
		})
	}
}

// A frame the capture tool writes is what the test holds its fixture to: a
// page the watch draws and a printout of usage, each read back from where
// it was written, its .txt its ANSI's text, and its ANSI ending on its last
// row's cells, with no newline after.
func TestAFrameWrittenFromItsFixtureIsHeldToIt(t *testing.T) {
	dir := t.TempDir()
	written := []string{"overview-default-160", "usage-52"}
	for _, name := range written {
		f, err := ByName(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.WriteFrame(dir); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := FramesIn(dir); err != nil || !slices.Equal(got, written) {
		t.Errorf("FramesIn lists %v (%v), want %v", got, err, written)
	}
	for _, name := range written {
		t.Run(name, func(t *testing.T) {
			frame, text := reference(t, dir, name)
			heldToItsFrame(t, name, frame)
			if got := frame.text(); !slices.Equal(got, text) {
				t.Errorf("its ANSI reads\n%s\nwant its .txt\n%s", strings.Join(got, "\n"), strings.Join(text, "\n"))
			}
			ansi, err := os.ReadFile(filepath.Join(dir, name+".ansi"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(string(ansi), "\n") {
				t.Error("its ANSI ends in a newline, want its last row's cells")
			}
		})
	}
}

// heldToItsFrame holds what the fixture called stem draws to frame, cell for
// cell.
func heldToItsFrame(t *testing.T, stem string, frame grid) {
	t.Helper()
	drawn, err := parse(frameOf(t, stem))
	if err != nil {
		t.Fatalf("parsing what %s draws: %v", stem, err)
	}
	canvas := rgb(terminalBackground.R, terminalBackground.G, terminalBackground.B)
	diffs, err := compare(frame, drawn, canvas)
	if err != nil {
		t.Fatalf("%s: %v", stem, err)
	}
	if len(diffs) > 0 {
		t.Error(describe(stem, diffs))
	}
}

// stems are the frames reference/ keeps, sorted.
func stems(t *testing.T) []string {
	t.Helper()
	stems, err := FramesIn(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(stems) == 0 {
		t.Fatalf("reference/ keeps no frames")
	}
	return stems
}

// reference is the frame dir keeps by the stem given, as its ANSI's cells,
// and its .txt's lines.
func reference(t *testing.T, dir, stem string) (grid, []string) {
	t.Helper()
	ansi, err := os.ReadFile(filepath.Join(dir, stem+".ansi"))
	if err != nil {
		t.Fatal(err)
	}
	txt, err := os.ReadFile(filepath.Join(dir, stem+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := parse(string(ansi))
	if err != nil {
		t.Fatalf("parsing %s.ansi: %v", stem, err)
	}
	return g, strings.Split(strings.TrimSuffix(string(txt), "\n"), "\n")
}

// sizeOf is the size a frame's stem ends with: its width, as in -160, and its
// height where it's given too, as in -160x39, else 0.
func sizeOf(t *testing.T, stem string) (width, height int) {
	t.Helper()
	size := stem[strings.LastIndex(stem, "-")+1:]
	if _, err := fmt.Sscanf(size, "%dx%d", &width, &height); err == nil {
		return width, height
	}
	if _, err := fmt.Sscanf(size, "%d", &width); err != nil {
		t.Fatalf("%s ends with no size: %v", stem, err)
	}
	return width, 0
}
