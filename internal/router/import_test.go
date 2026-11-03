package router_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
)

// stagedOn is when the files staged to import were last modified, as the Mac
// they were exported from left them: whole seconds, as any disk keeps them.
var stagedOn = time.Date(2026, 9, 20, 9, 30, 0, 0, time.UTC)

// keptFor is how long the tests' routers keep the ledger's lines, the
// router's events and the readings history, unless a test says otherwise.
const keptFor = 30 * 24 * time.Hour

// discard logs nothing of the files the tests read.
var discard = slog.New(slog.DiscardHandler)

// lineKind is a kind of line the history holds: its files in a state
// directory, or a folder staged, laid out alike, and a line of it.
type lineKind struct {
	name  string
	files func(root string) *dayfile.Files
	// line is the nth line of the kind, of the local day with the given
	// date.
	line func(n int, date string) string
}

var (
	requestLines = lineKind{name: "request ledger", files: func(root string) *dayfile.Files { return ledger.Files(ledger.Dir(root), discard) }, line: requestLine}
	eventLines   = lineKind{name: "router's events", files: func(root string) *dayfile.Files { return events.Files(ledger.Dir(root), discard) }, line: eventLine}
	readingLines = lineKind{name: "readings history", files: func(root string) *dayfile.Files { return readings.Files(readings.Dir(root), discard) }, line: readingLine}
	lineKinds    = []lineKind{requestLines, eventLines, readingLines}
)

// requestLine is the nth request's line of the local day with the given date.
func requestLine(n int, date string) string {
	return fmt.Sprintf(`{"at":%q,"request":"staged-%d","kind":"message","session":%q,"model":%q,"account":"work","reason":"sticky","status":200,"attempts":1,"total_ms":1000}`,
		atOn(date, n), n, sessionID, opus)
}

// eventLine is the nth event's line of the local day with the given date.
func eventLine(n int, date string) string {
	return fmt.Sprintf(`{"id":%d,"at":%q,"kind":"pin","account":"work","run":%q}`, n, atOn(date, n), atOn(date, 0))
}

// readingLine is the nth reading's line of the local day with the given date.
func readingLine(n int, date string) string {
	return fmt.Sprintf(`{"at":%q,"account":"work","window":"5h","utilization":0.%d,"resets_at":%q,"status":"allowed","source":"answer"}`,
		atOn(date, n), n, atOn(date, 300))
}

// atOn is n minutes past noon on the local day with the given date, in UTC.
func atOn(date string, n int) string {
	day, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		panic(err)
	}
	return day.Add(12*time.Hour + time.Duration(n)*time.Minute).UTC().Format(time.RFC3339)
}

// linesOf are the lines of the kind numbered ns, of the local day with the
// given date.
func (k lineKind) linesOf(date string, ns ...int) []string {
	lines := make([]string, len(ns))
	for i, n := range ns {
		lines[i] = k.line(n, date)
	}
	return lines
}

// dateOf is the date of the local day days before now's.
func dateOf(days int) string {
	return now.Local().AddDate(0, 0, -days).Format(time.DateOnly)
}

// importConfig is runConfig, keeping its history as long as keptFor says.
func importConfig(t *testing.T) router.Config {
	t.Helper()
	cfg := runConfig(t, "http://127.0.0.1:1")
	cfg.Ledger.Keep, cfg.History.Keep = keptFor, keptFor
	return cfg
}

// folder is the nth folder staged to import in the state directory of the
// router cfg configures.
func folder(cfg router.Config, n int) string {
	return filepath.Join(router.ImportDir(cfg.StateDir), strconv.Itoa(n))
}

// gzipOf is lines, each ended, as one gzip member, as export writes a day's.
func gzipOf(t *testing.T, lines ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	for _, line := range lines {
		if _, err := io.WriteString(zw, line+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// put writes data to a file at path, making its directory, last modified at
// modified.
func put(t *testing.T, path string, data []byte, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Time{}, modified); err != nil {
		t.Fatal(err)
	}
}

// stageLines stages lines of the kind, compressed, as the file of the local
// day with the given date in folder, last modified at stagedOn, and returns
// what it staged.
func stageLines(t *testing.T, folder string, kind lineKind, date string, lines ...string) []byte {
	t.Helper()
	data := gzipOf(t, lines...)
	put(t, kind.files(folder).CompressedPath(date), data, stagedOn)
	return data
}

// summaryOf is a summary of the local day with the given date, as the router
// writes one, of the lines given, marked with a compressed file of the size
// given.
func summaryOf(date string, lines int, compressed int) string {
	return fmt.Sprintf(`{"version":6,"day":%q,"lines":%d,"bytes":{"plain":0,"compressed":%d},"accounts":[]}`+"\n", date, lines, compressed)
}

// stamped is a summary's stamp, as the router stamps one of its version of a
// day whose compressed file was last modified at modified.
func stamped(modified time.Time) time.Time {
	return modified.Add(5 * time.Second)
}

// held returns the lines files hold of the local day with the given date,
// in either form, none where it holds none.
func held(t *testing.T, files *dayfile.Files, date string) []string {
	t.Helper()
	var lines []string
	_, _, err := dayfile.ReadDay(files, date, func(line []byte) (string, bool) { return string(line), true }, func(line string) bool {
		lines = append(lines, line)
		return true
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read the %s of %s: %v", files.Name, date, err)
	}
	return lines
}

// fileState is a file as it's written, and when it was last modified, in
// nanoseconds since the epoch.
type fileState struct {
	data     string
	modified int64
}

// stateOf returns the file at path as it is, failing the test where it can't
// be read.
func stateOf(t *testing.T, path string) fileState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fileState{data: string(data), modified: info.ModTime().UnixNano()}
}

// filesUnder returns each file under dir, by its path, as it is.
func filesUnder(t *testing.T, dir string) map[string]fileState {
	t.Helper()
	files := make(map[string]fileState)
	err := filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err == nil && e.Type().IsRegular() {
			files[path] = stateOf(t, path)
		}
		return err
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return files
}

// importNow runs a router with cfg, has it import what's staged in its state
// directory, and returns its answer, failing the test where the import
// fails.
func importNow(t *testing.T, cfg router.Config) router.Imported {
	t.Helper()
	runRouter(t, cfg)
	imported, err := router.NewClient(router.SocketPath(cfg.StateDir)).Import(t.Context())
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	return imported
}

// checkUnstaged checks the router whose state directory cfg configures has
// removed what was staged to import.
func checkUnstaged(t *testing.T, cfg router.Config) {
	t.Helper()
	if _, err := os.Stat(router.ImportDir(cfg.StateDir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("once the import was done, what was staged: %v, want it removed", err)
	}
}

func TestAnImportTakesInADayOfEachKindOfLine(t *testing.T) {
	tests := []struct {
		name string
		// days is how many days before now's the day is.
		days int
		// macs are the lines of the day the Mac holds, by their numbers, and
		// staged those staged.
		macs, staged []int
		want         router.Imported
		wantHeld     []int
	}{
		{
			name: "a day the Mac never saw is added", days: 10, staged: []int{1, 2},
			want: router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1}}, wantHeld: []int{1, 2},
		},
		{
			name: "a day not yet compressed the Mac never saw is added", days: 1, staged: []int{1, 2},
			want: router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1}}, wantHeld: []int{1, 2},
		},
		{
			name: "a day the Mac holds gains the lines it doesn't hold", days: 10, macs: []int{1}, staged: []int{1, 2},
			want: router.Imported{Folders: 1, Days: router.ImportedDays{Merged: 1}, Lines: 1}, wantHeld: []int{1, 2},
		},
		{
			name: "a day past keeping is left out", days: 60, staged: []int{1, 2},
			want: router.Imported{Folders: 1, Days: router.ImportedDays{PastKeep: 1}},
		},
	}
	for _, kind := range lineKinds {
		for _, tt := range tests {
			t.Run(kind.name+": "+tt.name, func(t *testing.T) {
				cfg := importConfig(t)
				date := dateOf(tt.days)
				if len(tt.macs) > 0 {
					put(t, kind.files(cfg.StateDir).CompressedPath(date), gzipOf(t, kind.linesOf(date, tt.macs...)...), now)
				}
				stageLines(t, folder(cfg, 1), kind, date, kind.linesOf(date, tt.staged...)...)

				if got := importNow(t, cfg); got != tt.want {
					t.Errorf("Import() = %+v, want %+v", got, tt.want)
				}
				if got, want := held(t, kind.files(cfg.StateDir), date), kind.linesOf(date, tt.wantHeld...); !slices.Equal(got, want) {
					t.Errorf("once imported, the %s of %s holds\n%s\nwant\n%s", kind.name, date, strings.Join(got, "\n"), strings.Join(want, "\n"))
				}
				checkUnstaged(t, cfg)
			})
		}
	}
}

func TestAnImportCountsADayOnceAcrossItsKindsAndFolders(t *testing.T) {
	cfg := importConfig(t)
	// The readings history is kept for 5 days, the ledger's lines and the
	// events for 30.
	cfg.History.Keep = 5 * 24 * time.Hour
	merged, added, pastKeep, empty := dateOf(10), dateOf(9), dateOf(60), dateOf(8)
	// Its readings past keeping, its requests added by the first folder, then
	// merged by the second, gaining a line: merged.
	stageLines(t, folder(cfg, 1), readingLines, merged, readingLines.linesOf(merged, 1)...)
	stageLines(t, folder(cfg, 1), requestLines, merged, requestLines.linesOf(merged, 1)...)
	stageLines(t, folder(cfg, 2), requestLines, merged, requestLines.linesOf(merged, 1, 2)...)
	// Its events added, its readings past keeping: added.
	stageLines(t, folder(cfg, 1), eventLines, added, eventLines.linesOf(added, 1)...)
	stageLines(t, folder(cfg, 2), readingLines, added, readingLines.linesOf(added, 1)...)
	// Its requests and its events past keeping.
	stageLines(t, folder(cfg, 2), requestLines, pastKeep, requestLines.linesOf(pastKeep, 1)...)
	stageLines(t, folder(cfg, 1), eventLines, pastKeep, eventLines.linesOf(pastKeep, 1)...)
	// A file of no lines, which takes nothing in.
	stageLines(t, folder(cfg, 1), requestLines, empty)

	want := router.Imported{Folders: 2, Days: router.ImportedDays{Added: 1, Merged: 1, PastKeep: 1}, Lines: 1}
	if got := importNow(t, cfg); got != want {
		t.Errorf("Import() = %+v, want %+v: each day once, as merged where any kind of it merged, else added where any was, else past keeping", got, want)
	}
}

func TestASummaryOfADayTheMacNeverSawIsCopiedInWithItsStamp(t *testing.T) {
	cfg := importConfig(t)
	date := dateOf(10)
	lines := stageLines(t, folder(cfg, 1), requestLines, date, requestLines.linesOf(date, 1, 2)...)
	staged := summaryOf(date, 2, len(lines))
	put(t, ledger.SummaryFile(ledger.Dir(folder(cfg, 1)), date), []byte(staged), stamped(stagedOn))

	if got := importNow(t, cfg); got.Summaries != 1 {
		t.Errorf("Import() = %+v, want 1 summary copied in", got)
	}
	want := fileState{data: staged, modified: stamped(stagedOn).UnixNano()}
	if got := stateOf(t, ledger.SummaryFile(ledger.Dir(cfg.StateDir), date)); got.data != want.data || got.modified != want.modified {
		t.Errorf("once imported, and the round made, the day's summary is %+v, want the one staged, with its stamp, standing against its day's file copied in: %+v", got, want)
	}
}

func TestASummaryOfADayPastKeepingIsCopiedIn(t *testing.T) {
	cfg := importConfig(t)
	date := dateOf(60)
	lines := stageLines(t, folder(cfg, 1), requestLines, date, requestLines.linesOf(date, 1)...)
	staged := summaryOf(date, 1, len(lines))
	put(t, ledger.SummaryFile(ledger.Dir(folder(cfg, 1)), date), []byte(staged), stamped(stagedOn))

	want := router.Imported{Folders: 1, Days: router.ImportedDays{PastKeep: 1}, Summaries: 1}
	if got := importNow(t, cfg); got != want {
		t.Errorf("Import() = %+v, want %+v", got, want)
	}
	if got := stateOf(t, ledger.SummaryFile(ledger.Dir(cfg.StateDir), date)); got.data != staged || got.modified != stamped(stagedOn).UnixNano() {
		t.Errorf("once imported, the day's summary is %+v, want the one staged, with its stamp: it's the day's only record", got)
	}
	if got := held(t, requestLines.files(cfg.StateDir), date); len(got) > 0 {
		t.Errorf("once imported, the request ledger holds %q of the day, want none: it's past keeping", got)
	}
}

func TestTheMacsOwnSummaryStandsAgainstOneStaged(t *testing.T) {
	cfg := importConfig(t)
	// Past keeping, so the day's lines aren't taken in, and the Mac's summary
	// is its only record.
	date := dateOf(60)
	macs := ledger.SummaryFile(ledger.Dir(cfg.StateDir), date)
	put(t, macs, []byte(summaryOf(date, 7, 0)), now)
	was := stateOf(t, macs)
	lines := stageLines(t, folder(cfg, 1), requestLines, date, requestLines.linesOf(date, 1)...)
	put(t, ledger.SummaryFile(ledger.Dir(folder(cfg, 1)), date), []byte(summaryOf(date, 1, len(lines))), stamped(stagedOn))

	if got := importNow(t, cfg); got.Summaries != 0 {
		t.Errorf("Import() = %+v, want no summary copied in", got)
	}
	if got := stateOf(t, macs); got != was {
		t.Errorf("once imported, the Mac's summary of the day is %+v, want its own, as it was: %+v", got, was)
	}
}

func TestADayTheMacHeldLinesOfIsSummarisedAgainFromThemAll(t *testing.T) {
	cfg := importConfig(t)
	date := dateOf(10)
	put(t, requestLines.files(cfg.StateDir).CompressedPath(date), gzipOf(t, requestLines.linesOf(date, 1)...), now)
	lines := stageLines(t, folder(cfg, 1), requestLines, date, requestLines.linesOf(date, 1, 2)...)
	// Of no account, so a summary made from the lines is told from it.
	put(t, ledger.SummaryFile(ledger.Dir(folder(cfg, 1)), date), []byte(summaryOf(date, 2, len(lines))), stamped(stagedOn))
	summary := ledger.SummaryFile(ledger.Dir(cfg.StateDir), date)

	runRouter(t, cfg)
	waitUntil(t, "the router summarises the day it holds", func() bool {
		_, err := os.Stat(summary)
		return err == nil
	})
	imported, err := router.NewClient(router.SocketPath(cfg.StateDir)).Import(t.Context())
	if want := (router.Imported{Folders: 1, Days: router.ImportedDays{Merged: 1}, Lines: 1}); err != nil || imported != want {
		t.Fatalf("Import() = %+v, %v, want %+v", imported, err, want)
	}
	data := stateOf(t, summary).data
	var got ledger.Summary
	if err := json.Unmarshal([]byte(data), &got); err != nil || got.Lines != 2 || len(got.Accounts) != 1 || got.Accounts[0].Account != "work" {
		t.Errorf("once imported, the day's summary is\n%s\nwant the Mac's own, made again by the round from both lines, of work's requests, not the one staged", data)
	}
}

func TestAStagedSummaryThatDoesntReadAsItsDaysIsLeftOut(t *testing.T) {
	date := dateOf(60)
	tests := []struct {
		name, summary string
	}{
		{name: "cut short, as it synced", summary: summaryOf(date, 1, 0)[:20]},
		{name: "of another day", summary: summaryOf(dateOf(61), 1, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := importConfig(t)
			put(t, ledger.SummaryFile(ledger.Dir(folder(cfg, 1)), date), []byte(tt.summary), stamped(stagedOn))

			if got := importNow(t, cfg); got.Summaries != 0 {
				t.Errorf("Import() = %+v, want no summary copied in", got)
			}
			if _, err := os.Stat(ledger.SummaryFile(ledger.Dir(cfg.StateDir), date)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("once imported, the Mac's summary of the day: %v, want none: one that isn't the day's would stand in the way of the whole one imported later", err)
			}
		})
	}
}

func TestTheDaysImportedAreSummarisedBeforeTheImportIsAnswered(t *testing.T) {
	cfg := importConfig(t)
	date := dateOf(10)
	stageLines(t, folder(cfg, 1), requestLines, date, requestLines.linesOf(date, 1, 2, 3)...)

	importNow(t, cfg)
	data, err := os.ReadFile(ledger.SummaryFile(ledger.Dir(cfg.StateDir), date))
	var got ledger.Summary
	if err == nil {
		err = json.Unmarshal(data, &got)
	}
	if err != nil || got.Lines != 3 {
		t.Errorf("once imported, the day's summary is %s, %v, want one of its 3 lines, made by the round the import makes", data, err)
	}
}

func TestAFailedImportLeavesWhatWasStagedAndWhatItTookInBefore(t *testing.T) {
	cfg := importConfig(t)
	taken, damaged := dateOf(9), dateOf(10)
	// The Mac's own file of the day is cut short, so no member can be added
	// after it.
	whole := gzipOf(t, requestLines.linesOf(damaged, 1)...)
	put(t, requestLines.files(cfg.StateDir).CompressedPath(damaged), whole[:len(whole)-4], now)
	stageLines(t, folder(cfg, 1), readingLines, taken, readingLines.linesOf(taken, 1)...)
	stageLines(t, folder(cfg, 1), requestLines, damaged, requestLines.linesOf(damaged, 1, 2)...)
	staged := filesUnder(t, router.ImportDir(cfg.StateDir))

	runRouter(t, cfg)
	_, err := router.NewClient(router.SocketPath(cfg.StateDir)).Import(t.Context())
	if err == nil || errors.Is(err, router.ErrNoImport) || !strings.Contains(err.Error(), damaged) {
		t.Errorf("Import() error = %v, want why the day damaged couldn't be taken in", err)
	}
	if got := filesUnder(t, router.ImportDir(cfg.StateDir)); !maps.Equal(got, staged) {
		t.Errorf("once the import failed, what's staged is %v, want it as it was, %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(staged)))
	}
	if got, want := held(t, readingLines.files(cfg.StateDir), taken), readingLines.linesOf(taken, 1); !slices.Equal(got, want) {
		t.Errorf("once the import failed, the readings history holds %q of the day taken in before, want %q", got, want)
	}
}

func TestAnImportWithNothingStagedIsRefused(t *testing.T) {
	tests := []struct {
		name string
		// stage stages what's in the import directory, given its path.
		stage map[string]string
	}{
		{name: "no import directory"},
		{name: "an import directory of nothing staged as a folder", stage: map[string]string{"1": "a file", "01/export.json": "{}", "a/export.json": "{}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := importConfig(t)
			for name, data := range tt.stage {
				put(t, filepath.Join(router.ImportDir(cfg.StateDir), name), []byte(data), stagedOn)
			}
			staged := filesUnder(t, router.ImportDir(cfg.StateDir))
			runRouter(t, cfg)

			_, err := router.NewClient(router.SocketPath(cfg.StateDir)).Import(t.Context())
			if err == nil || !strings.Contains(err.Error(), "there's nothing staged to import") {
				t.Errorf("Import() error = %v, want it saying there's nothing staged", err)
			}
			if got := filesUnder(t, router.ImportDir(cfg.StateDir)); !maps.Equal(got, staged) {
				t.Errorf("once refused, the import directory holds %v, want it as it was, %v", got, staged)
			}
		})
	}
}

func TestAnImportAskedOfARouterNotKeepingItsHistoryIsRefused(t *testing.T) {
	rec := httptest.NewRecorder()

	newRouter(t, "http://127.0.0.1:1").Control().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/import", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "isn't keeping its history yet") {
		t.Errorf("POST /import of a router not running answered %d %s, want 503, saying it isn't keeping its history yet", rec.Code, rec.Body)
	}
}

func TestImportingTheSameFolderAgainAddsNothing(t *testing.T) {
	cfg := importConfig(t)
	whole, appended, pastKeep := dateOf(10), dateOf(1), dateOf(60)
	stage := func() {
		for _, kind := range lineKinds {
			for _, date := range []string{whole, appended, pastKeep} {
				stageLines(t, folder(cfg, 1), kind, date, kind.linesOf(date, 1, 2)...)
			}
		}
		put(t, ledger.SummaryFile(ledger.Dir(folder(cfg, 1)), pastKeep), []byte(summaryOf(pastKeep, 2, 0)), stamped(stagedOn))
	}
	// The files of the days staged, as they are: the router's own of today
	// may change meanwhile.
	daysStaged := func() map[string]fileState {
		files := filesUnder(t, ledger.Dir(cfg.StateDir))
		maps.Copy(files, filesUnder(t, readings.Dir(cfg.StateDir)))
		maps.DeleteFunc(files, func(path string, _ fileState) bool {
			return !strings.Contains(path, whole) && !strings.Contains(path, appended) && !strings.Contains(path, pastKeep)
		})
		return files
	}
	stage()
	runRouter(t, cfg)
	client := router.NewClient(router.SocketPath(cfg.StateDir))
	first, err := client.Import(t.Context())
	if want := (router.Imported{Folders: 1, Days: router.ImportedDays{Added: 2, PastKeep: 1}, Summaries: 1}); err != nil || first != want {
		t.Fatalf("Import() = %+v, %v, want %+v", first, err, want)
	}
	before := daysStaged()

	stage()
	second, err := client.Import(t.Context())
	if want := (router.Imported{Folders: 1, Days: router.ImportedDays{Merged: 2, PastKeep: 1}}); err != nil || second != want {
		t.Errorf("Import() again = %+v, %v, want %+v: each day held merged, gaining nothing", second, err, want)
	}
	if after := daysStaged(); !maps.Equal(after, before) {
		t.Errorf("once imported again, the days' files are\n%v\nwant them as the first import left them\n%v", after, before)
	}
}

func TestAnImportBesideRoutedRequestsLosesNoLine(t *testing.T) {
	up := newUpstream(t, answerWith(http.StatusOK, session, week))
	cfg := importConfig(t)
	cfg.Upstream = up.URL
	today := dateOf(0)
	staged := requestLines.linesOf(today, numbers(50)...)
	stageLines(t, folder(cfg, 1), requestLines, today, staged...)
	// Days of every kind besides, so the import runs a while beside the
	// requests.
	for days := 1; days <= 20; days++ {
		for _, kind := range lineKinds {
			stageLines(t, folder(cfg, 1), kind, dateOf(days), kind.linesOf(dateOf(days), 1)...)
		}
	}
	stop := runRouter(t, cfg)
	const requests = 40
	answers := make([]string, requests)
	var imported error
	var wg sync.WaitGroup
	wg.Go(func() { _, imported = router.NewClient(router.SocketPath(cfg.StateDir)).Import(t.Context()) })
	for i := range requests {
		wg.Go(func() { answers[i] = post("http://" + cfg.Listen + "/v1/messages") })
	}
	wg.Wait()
	if imported != nil {
		t.Fatalf("Import() error = %v", imported)
	}
	for i, answer := range answers {
		if !strings.HasPrefix(answer, "200 ") {
			t.Fatalf("request %d was answered %s, want 200", i, answer)
		}
	}
	if err := stop(); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	routed := 0
	times := make(map[string]int)
	for _, line := range held(t, requestLines.files(cfg.StateDir), today) {
		if slices.Contains(staged, line) {
			times[line]++
		} else {
			routed++
		}
	}
	if routed != requests {
		t.Errorf("today's ledger holds %d lines of requests routed, want %d", routed, requests)
	}
	for _, line := range staged {
		if times[line] != 1 {
			t.Errorf("today's ledger holds %s %d times, want once", line, times[line])
		}
	}
}

// numbers are the numbers from 0 up to n, but n.
func numbers(n int) []int {
	ns := make([]int, n)
	for i := range ns {
		ns[i] = i
	}
	return ns
}

func TestClientImportOfARouterFromBeforeImport(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"ok": true}`) })
	serveOn(t, path, mux)

	if _, err := router.NewClient(path).Import(t.Context()); !errors.Is(err, router.ErrNoImport) {
		t.Errorf("Import() of a router from before POST /import: %v, want ErrNoImport", err)
	}
}
