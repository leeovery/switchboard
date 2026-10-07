package dayfile

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// asText is a line as the text it holds, which every line reads as.
func asText(line []byte) (string, bool) {
	return string(line), true
}

// readJSON returns the lines f holds of the local days with the given dates
// that are JSON, as a line written whole is and one cut short isn't, as Read
// gives them, and how many lines weren't.
func readJSON(f *Files, dates ...string) (lines []string, unread int) {
	asJSON := func(line []byte) (string, bool) { return string(line), json.Valid(line) }
	unread = Read(f, dates, asJSON, func(line string) bool {
		lines = append(lines, line)
		return true
	})
	return lines, unread
}

// readAll returns the lines f holds of the local days with the given dates,
// as Read gives them.
func readAll(f *Files, dates ...string) []string {
	var lines []string
	Read(f, dates, asText, func(line string) bool {
		lines = append(lines, line)
		return true
	})
	return lines
}

func TestReadGivesEachDaysLinesOnceInTheOrderTheyCame(t *testing.T) {
	tests := []struct {
		name string
		// files are the members each file holds, one after another.
		files map[dayFile][]string
		want  []string
	}{
		{
			name: "each day's after the day's before",
			files: map[dayFile][]string{
				plainFile("2026-09-27"): {linesOf("one", "two")},
				plainFile("2026-09-28"): {linesOf("three")},
			},
			want: []string{"one", "two", "three"},
		},
		{
			name: "a compressed file's members as one, before its day's plain file's, as when the clock was set back to the day",
			files: map[dayFile][]string{
				compressedFile("2026-09-27"): {linesOf("one"), linesOf("two")},
				plainFile("2026-09-27"):      {linesOf("three")},
			},
			want: []string{"one", "two", "three"},
		},
		{
			name: "a day's lines once, when its compressed file ends with its plain file's, as after a writer stopped compressing it",
			files: map[dayFile][]string{
				compressedFile("2026-09-27"): {linesOf("one"), linesOf("two", "three")},
				plainFile("2026-09-27"):      {linesOf("two", "three")},
			},
			want: []string{"one", "two", "three"},
		},
		{
			name: "none of a day without files",
			files: map[dayFile][]string{
				plainFile("2026-09-28"): {linesOf("one")},
			},
			want: []string{"one"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readingsHistory(t.TempDir())
			for file, members := range tt.files {
				writeDay(t, f, file, members...)
			}

			if got := readAll(f, "2026-09-27", "2026-09-28"); !slices.Equal(got, tt.want) {
				t.Errorf("read %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadPassesOverWhatDecodeMakesNothingOf(t *testing.T) {
	f := readingsHistory(t.TempDir())
	writeDay(t, f, plainFile("2026-09-28"), linesOf("one", "?", "", "two", "?"))
	answers := func(line []byte) (string, bool) {
		return string(line), len(line) > 0 && string(line) != "?"
	}

	var got []string
	unread := Read(f, []string{"2026-09-28"}, answers, func(line string) bool {
		got = append(got, line)
		return true
	})
	if want := []string{"one", "two"}; !slices.Equal(got, want) || unread != 3 {
		t.Errorf("read %q, %d unread; want %q, and the 3 made nothing of, the empty line among them, unread", got, unread, want)
	}
}

func TestReadStopsOnceTakeHasEnough(t *testing.T) {
	f := readingsHistory(t.TempDir())
	writeDay(t, f, plainFile("2026-09-27"), linesOf("one", "two", "three"))
	writeDay(t, f, plainFile("2026-09-28"), linesOf("four"))

	var got []string
	Read(f, []string{"2026-09-27", "2026-09-28"}, asText, func(line string) bool {
		got = append(got, line)
		return line != "two"
	})
	if want := []string{"one", "two"}; !slices.Equal(got, want) {
		t.Errorf("read %q, want %q, and nothing after take had enough", got, want)
	}
}

func TestALineLongerThanLineMaxIsSkippedWithoutBeingHeld(t *testing.T) {
	f := readingsHistory(t.TempDir())
	f.LineMax = 16
	// A line of 16 bytes, its line ending included, is held; one longer isn't.
	writeDay(t, f, plainFile("2026-09-28"), linesOf("fifteen bytes..", "sixteen bytes...", "short", strings.Repeat("x", 100)))

	var got []string
	unread := Read(f, []string{"2026-09-28"}, asText, func(line string) bool {
		got = append(got, line)
		return true
	})
	if want := []string{"fifteen bytes..", "short"}; !slices.Equal(got, want) || unread != 2 {
		t.Errorf("read %q, %d unread; want %q, and the 2 lines too long unread", got, unread, want)
	}
}

func TestNewestTakesTheNewestDaysByTheirFilesNames(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.Local)
	f := readingsHistory(t.TempDir())
	// Tomorrow's file, as a change of time zone names one, and one of a day
	// after tomorrow, as a clock once set ahead names one.
	for _, file := range []dayFile{compressedFile("2026-09-20"), compressedFile("2026-09-26"), plainFile("2026-09-26"), plainFile("2026-09-29"), plainFile("2026-09-30")} {
		writeDay(t, f, file, linesOf("a"))
	}
	if err := os.WriteFile(f.path(plainFile("2026-09-25"))+".tmp", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		n    int
		want []string
	}{
		{n: 2, want: []string{"2026-09-26", "2026-09-29"}},
		{n: 5, want: []string{"2026-09-20", "2026-09-26", "2026-09-29"}},
	}
	for _, tt := range tests {
		if got := f.Newest(now, tt.n); !slices.Equal(got, tt.want) {
			t.Errorf("Newest(%d) = %q, want %q: each day once, oldest first, none after tomorrow", tt.n, got, tt.want)
		}
	}
}

func TestFilesOfADayAfterTomorrowAreKeptButNotAmongTheNewest(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.Local)
	f := readingsHistory(t.TempDir())
	dated := func(offset int) string { return now.AddDate(0, 0, offset).Format(time.DateOnly) }
	// A clock once set days ahead named files for days to come, and compressed
	// one as more days went by.
	files := map[dayFile]string{
		plainFile(dated(-1)):     "yesterday",
		plainFile(dated(0)):      "today",
		plainFile(dated(5)):      "five days on",
		compressedFile(dated(6)): "six days on",
	}
	for file, text := range files {
		writeDay(t, f, file, linesOf(text))
	}

	if got := readAll(f, f.Newest(now, 2)...); !slices.Equal(got, []string{"yesterday", "today"}) {
		t.Errorf("the newest days hold %q, want yesterday's and today's, not those of days to come", got)
	}
	f.prune(now, twoWeeks)
	for file := range files {
		if _, err := os.Stat(f.path(file)); err != nil {
			t.Errorf("%s went (%v), want it kept: the clock may be the one that's wrong", file.name(f.Prefix), err)
		}
	}
}

func TestDatesStepADayAtATimeWhereTheClocksChangeAtMidnight(t *testing.T) {
	santiago, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatalf("load America/Santiago: %v", err)
	}
	// Chile's clocks went from 00:00 to 01:00 on 6 September 2026.
	got := datesFrom(time.Date(2026, 9, 4, 15, 0, 0, 0, santiago), time.Date(2026, 9, 8, 10, 0, 0, 0, santiago))
	if want := []string{"2026-09-04", "2026-09-05", "2026-09-06", "2026-09-07", "2026-09-08"}; !slices.Equal(got, want) {
		t.Errorf("datesFrom() = %q, want %q: each day once, the last included", got, want)
	}
}

func TestDatesTakeInTheLocalDayEitherSide(t *testing.T) {
	from := time.Date(2026, 9, 28, 23, 30, 0, 0, time.Local)
	to := time.Date(2026, 9, 30, 0, 30, 0, 0, time.Local)

	got := Dates(from.UTC(), to.UTC())
	if want := []string{"2026-09-27", "2026-09-28", "2026-09-29", "2026-09-30", "2026-10-01"}; !slices.Equal(got, want) {
		t.Errorf("Dates() = %q, want %q: the local days from the one before from's to the one after to's", got, want)
	}
}

func TestADamagedCompressedFileGivesTheLinesBeforeTheDamage(t *testing.T) {
	yesterday := compressedFile("2026-09-28")
	first, second := linesOf("the first line"), linesOf("the second line")
	tests := []struct {
		name string
		// data is what yesterday's compressed file holds.
		data []byte
		// want are the lines read back, today's last.
		want []string
		log  []string
	}{
		{
			name: "cut short in its second member",
			data: append(gzipOf(t, first), gzipOf(t, second)[:5]...),
			want: []string{"the first line", "today"},
			log:  []string{"level=WARN", `msg="readings history read short"`, "file=" + yesterday.name("readings"), `error="unexpected EOF"`},
		},
		{
			name: "not gzip at all",
			data: []byte(first),
			want: []string{"today"},
			log:  []string{"level=WARN", `msg="can't read the readings history"`, "file=" + yesterday.name("readings"), `error="gzip: invalid header"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			f := readingsHistory(t.TempDir())
			if err := os.WriteFile(f.path(yesterday), tt.data, 0o600); err != nil {
				t.Fatal(err)
			}
			writeDay(t, f, plainFile("2026-09-29"), linesOf("today"))

			if got := readAll(f, "2026-09-28", "2026-09-29"); !slices.Equal(got, tt.want) {
				t.Errorf("read %q, want %q: the lines before the damage, and today's", got, tt.want)
			}
			if !log.Has(tt.log...) {
				t.Errorf("log reads\n%s\nwant the damage noted, a line with %q", log, tt.log)
			}
		})
	}
}

func TestAFileThatCantBeReadIsLogged(t *testing.T) {
	log := logstest.Capture(t)
	f := readingsHistory(t.TempDir())
	if err := os.WriteFile(f.path(plainFile(dateOf(start))), []byte("{}\n"), 0o000); err != nil {
		t.Fatal(err)
	}

	if got := readAll(f, dateOf(start)); len(got) != 0 {
		t.Errorf("read %q, want none", got)
	}
	if !log.Has("level=WARN", `msg="can't read the readings history"`) {
		t.Errorf("log reads\n%s\nwant the file that can't be read noted", log)
	}
}

func TestADamagedFileIsWarnedOfOnceUntilItReads(t *testing.T) {
	yesterday := compressedFile(start.Local().AddDate(0, 0, -1).Format(time.DateOnly))
	lines := linesOf("a line read yesterday")
	tests := []struct {
		name string
		// data is what yesterday's compressed file holds, damaged, and
		// warning what it's warned of.
		data    []byte
		warning string
	}{
		{name: "one that can't be read", data: []byte(lines), warning: `msg="can't read the readings history"`},
		{name: "one cut short", data: append(gzipOf(t, lines), gzipOf(t, lines)[:5]...), warning: `msg="readings history read short"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			f := readingsHistory(t.TempDir())
			hold := func(data []byte) {
				if err := os.WriteFile(f.path(yesterday), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			read := func() { readAll(f, yesterday.date) }

			// Read again and again, as the readings history is, as the router
			// starts and at each GET /history.
			hold(tt.data)
			read()
			read()
			read()
			warned := linesWith(log, "file="+yesterday.name(f.Prefix))
			if len(warned) != 1 || !strings.Contains(warned[0], "level=WARN") || !strings.Contains(warned[0], tt.warning) {
				t.Fatalf("log reads\n%s\nwant the file warned of once, as first read, a line with %s", log, tt.warning)
			}

			// Mended, it reads to its end; damaged again, it's warned of again.
			hold(gzipOf(t, lines))
			read()
			hold(tt.data)
			read()
			read()
			if warned := linesWith(log, "file="+yesterday.name(f.Prefix)); len(warned) != 2 || !strings.Contains(warned[1], tt.warning) {
				t.Errorf("log reads\n%s\nwant the file warned of again once it's damaged after reading to its end, once", log)
			}
		})
	}
}
