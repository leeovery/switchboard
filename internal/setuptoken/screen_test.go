package setuptoken

import (
	"slices"
	"strings"
	"testing"
)

func TestScreenDrawsWhatATerminalShows(t *testing.T) {
	tests := []struct {
		name  string
		drawn string
		want  []string
	}{
		{name: "lines", drawn: " one\r\n two\r\n", want: []string{" one", " two"}},
		{name: "colours and modes passed over", drawn: "\x1b[?25l\x1b[2mdim\x1b[22m \x1b[38;5;214mbright\x1b[39m\x1b[?2004h", want: []string{"dim bright"}},
		{name: "a link's text, without its target", drawn: "\x1b]8;;https://claude.example/a\x07link\x1b]8;;\x07", want: []string{"link"}},
		{name: "a frame drawn over the last, up to its start and erased below", drawn: "Opening…\r\nwait\r\n\x1b[2A\x1b[J✓ made\r\n", want: []string{"✓ made"}},
		{name: "a frame's lines erased one by one, as log-update does", drawn: "one\r\ntwo\r\n\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K\x1b[Gthree\r\n", want: []string{"three"}},
		{name: "a line written over in part, as a diff does", drawn: "abcdefgh\r\x1b[3CXY", want: []string{"abcXYfgh"}},
		{name: "a line's end erased", drawn: "keep this, not this\x1b[10D\x1b[K", want: []string{"keep this,"}},
		{name: "a line's start erased", drawn: "drop, keep\x1b[5D\x1b[1K", want: []string{"      keep"}},
		{name: "a whole line erased", drawn: "gone\x1b[2K", want: nil},
		{name: "characters erased", drawn: "abcdef\x1b[4D\x1b[2X", want: []string{"ab  ef"}},
		{name: "a column chosen", drawn: "abc\x1b[2Gx", want: []string{"axc"}},
		{name: "a place chosen", drawn: "one\r\ntwo\x1b[1;2Hx", want: []string{"oxe", "two"}},
		{name: "a row chosen", drawn: "one\r\ntwo\x1b[1dx", want: []string{"onex", "two"}},
		{name: "the cursor saved and restored", drawn: "a\x1b7\r\nb\x1b8c", want: []string{"ac", "b"}},
		{name: "the cursor saved and restored by CSI", drawn: "a\x1b[s\r\nb\x1b[uc", want: []string{"ac", "b"}},
		{name: "back a column", drawn: "ab\bc", want: []string{"ac"}},
		{name: "to a tab stop", drawn: "a\tb", want: []string{"a       b"}},
		{name: "a row too long, wrapped", drawn: "0123456789ab", want: []string{"0123456789", "ab"}},
		{name: "a row filled, the cursor kept at its end", drawn: "0123456789\r\nx", want: []string{"0123456789", "x"}},
		{name: "moves kept on the screen", drawn: "a\x1b[99A\x1b[99Db", want: []string{"b"}},
		{name: "a move of 0, taken as 1", drawn: "one\r\ntwo\x1b[0A\x1b[0Dx\x1b[0By\x1b[0Cz", want: []string{"onx", "twoy z"}},
		{name: "a place of 0, taken as the first", drawn: "one\r\ntwo\x1b[0;0Hx", want: []string{"xne", "two"}},
		{name: "0 characters erased, taken as 1", drawn: "abc\x1b[2D\x1b[0X", want: []string{"a c"}},
		{name: "blank rows around it dropped", drawn: "\r\n\r\n x  \r\n\r\n", want: []string{" x"}},
		{name: "a full reset", drawn: "gone\r\n\x1bckept", want: []string{"kept"}},
		{name: "the whole screen erased", drawn: "gone\r\ngone\x1b[2J\x1b[Hkept", want: []string{"kept"}},
		{name: "the screen from its start erased", drawn: "gone\r\ngone kept\x1b[5D\x1b[1J", want: []string{"     kept"}},
		{name: "a line's characters set in another charset passed over", drawn: "\x1b(Bplain", want: []string{"plain"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newScreen(10, 4)
			s.write([]byte(tt.drawn))
			if got := s.drawing(); !slices.Equal(got, drawing(tt.want)) {
				t.Errorf("drawing %q shows %q, want %q", tt.drawn, got, tt.want)
			}
		})
	}
}

func TestScreenScrollsRowsOffTheTopIntoItsScrollback(t *testing.T) {
	s := newScreen(10, 2)

	s.write([]byte("one\r\ntwo\r\nthree\x1b[1;1Hx"))

	if got, want := s.drawing(), (drawing{"one", "xwo", "three"}); !slices.Equal(got, want) {
		t.Errorf("drawing shows %q, want %q: the place chosen on the screen as it stands", got, want)
	}
}

func TestScreenErasesItsScrollback(t *testing.T) {
	s := newScreen(10, 2)

	s.write([]byte("one\r\ntwo\r\nthree\x1b[3J"))

	if got, want := s.drawing(), (drawing{"two", "three"}); !slices.Equal(got, want) {
		t.Errorf("drawing shows %q, want %q", got, want)
	}
}

func TestScreenKeepsItsRowsBounded(t *testing.T) {
	s := newScreen(10, 2)

	s.write([]byte(strings.Repeat("x\r\n", maxRows*2) + "last"))

	got := s.drawing()
	if len(got) > maxRows || got[len(got)-1] != "last" {
		t.Errorf("drawing holds %d rows, ending %q; want %d at most, ending with the last", len(got), got[len(got)-1], maxRows)
	}
}

func TestScreenDrawsTheSameHoweverWhatsDrawnArrives(t *testing.T) {
	drawn := []byte("\x1b[?25l \x1b[2mOpening…\x1b[22m\r\n\x1b[1A\x1b[J \x1b[32m✓ Made\x1b[39m\r\n\x1b]8;;https://claude.example\x07 \x1b[33msk-ant-oat01-test-token-work\x1b[39m\x1b]8;;\x07\r\n")
	whole := newScreen(80, 10)
	whole.write(drawn)
	want := whole.drawing()
	for cut := 1; cut < len(drawn); cut++ {
		s := newScreen(80, 10)
		s.write(drawn[:cut])
		s.write(drawn[cut:])
		if got := s.drawing(); !slices.Equal(got, want) {
			t.Fatalf("drawing %q cut at %d shows %q, want %q, as drawn whole", drawn, cut, got, want)
		}
	}
}
