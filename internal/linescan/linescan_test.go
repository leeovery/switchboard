package linescan_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/leeovery/switchboard/internal/linescan"
)

// most is the most of a line the tests' scanners hold, its line ending
// included.
const most = 16

// scanned returns the lines s reads, in turn, and how many it passed over.
func scanned(s *linescan.Scanner) (lines []string, long int) {
	for s.Scan() {
		lines = append(lines, string(s.Bytes()))
	}
	return lines, s.Long()
}

// gzipped returns text compressed, as a gzip member, failing t where it can't
// be.
func gzipped(t *testing.T, text string) []byte {
	t.Helper()
	var member bytes.Buffer
	w := gzip.NewWriter(&member)
	if _, err := w.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return member.Bytes()
}

// fileOf returns a file holding text, opened to read, failing t where it
// can't be: it's closed as t ends.
func fileOf(t *testing.T, text string) io.Reader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lines")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestScanReadsEachLinePassingOverThoseTooLongToHold(t *testing.T) {
	long := strings.Repeat("x", 100)
	tests := []struct {
		name string
		text string
		want []string
		// wantLong is how many lines are passed over.
		wantLong int
	}{
		{name: "lines ending in a line feed", text: "one\ntwo\n", want: []string{"one", "two"}},
		{name: "lines ending in a carriage return and a line feed", text: "one\r\ntwo\r\n", want: []string{"one", "two"}},
		{name: "a last line without its ending", text: "one\ntwo", want: []string{"one", "two"}},
		{name: "empty lines", text: "\none\n\n", want: []string{"", "one", ""}},
		{name: "a line of the most, its line feed included", text: "fifteen bytes..\nnext\n", want: []string{"fifteen bytes..", "next"}},
		{name: "a line a byte longer", text: "sixteen bytes...\nnext\n", want: []string{"next"}, wantLong: 1},
		{
			name: "a line of the most, its carriage return and line feed included",
			text: "fourteen bytes\r\nnext\n",
			want: []string{"fourteen bytes", "next"},
		},
		{
			name:     "a line a byte longer, its carriage return and line feed included",
			text:     "fifteen bytes..\r\nnext\n",
			want:     []string{"next"},
			wantLong: 1,
		},
		{
			name:     "lines far too long, those between and after read",
			text:     "a\n" + long + "\nb\n" + long + "\nc\nd",
			want:     []string{"a", "b", "c", "d"},
			wantLong: 2,
		},
		{name: "a line too long last, without its ending", text: "a\n" + long, want: []string{"a"}, wantLong: 1},
		{
			name: "a line a byte short of the most last, without its ending",
			text: "a\n" + strings.Repeat("x", most-1),
			want: []string{"a", strings.Repeat("x", most-1)},
		},
		{name: "a line of the most last, without its ending", text: "a\n" + strings.Repeat("x", most), want: []string{"a"}, wantLong: 1},
	}
	// readers give the text as one reader or another does: whole, a byte at a
	// time, from a file, each giving the end of the text on a read of its own;
	// or with its last data its end, as a decompressing reader gives a
	// stream's last, or decompressed.
	readers := []struct {
		name string
		of   func(t *testing.T, text string) io.Reader
	}{
		{name: "whole", of: func(_ *testing.T, text string) io.Reader { return strings.NewReader(text) }},
		{name: "a byte at a time", of: func(_ *testing.T, text string) io.Reader { return iotest.OneByteReader(strings.NewReader(text)) }},
		{name: "from a file", of: fileOf},
		{name: "its last data with its end", of: func(_ *testing.T, text string) io.Reader { return iotest.DataErrReader(strings.NewReader(text)) }},
		{
			name: "decompressed",
			of: func(t *testing.T, text string) io.Reader {
				r, err := gzip.NewReader(bytes.NewReader(gzipped(t, text)))
				if err != nil {
					t.Fatal(err)
				}
				return r
			},
		},
	}
	for _, tt := range tests {
		for _, r := range readers {
			t.Run(tt.name+", read "+r.name, func(t *testing.T) {
				s := linescan.New(r.of(t, tt.text), most)

				got, long := scanned(s)
				if !slices.Equal(got, tt.want) || long != tt.wantLong {
					t.Errorf("read %q, %d passed over; want %q, %d passed over", got, long, tt.want, tt.wantLong)
				}
				if err := s.Err(); err != nil {
					t.Errorf("Err() = %v, want nil: it read to the end", err)
				}
			})
		}
	}
}

func TestScanStopsWhereReadingFailsSayingWhy(t *testing.T) {
	broken := errors.New("connection reset")
	tests := []struct {
		name     string
		text     string
		want     []string
		wantLong int
	}{
		{name: "partway through a line, read as far as it went", text: "one\ntw", want: []string{"one", "tw"}},
		{name: "partway through a line too long to hold", text: "one\n" + strings.Repeat("x", 100), want: []string{"one"}, wantLong: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := linescan.New(io.MultiReader(strings.NewReader(tt.text), iotest.ErrReader(broken)), most)

			got, long := scanned(s)
			if !slices.Equal(got, tt.want) || long != tt.wantLong {
				t.Errorf("read %q, %d passed over; want %q, %d passed over", got, long, tt.want, tt.wantLong)
			}
			if err := s.Err(); !errors.Is(err, broken) {
				t.Errorf("Err() = %v, want %v", err, broken)
			}
		})
	}
}
