// Package linescan reads text a line at a time, as bufio.Scanner does, but
// holds a line only as far as the most it's given: a line that runs to the
// most without its ending is passed over, without being held, and counted,
// and the lines after it read, where a bufio.Scanner stops at one for good.
// The request ledger's and the readings history's files are read back this
// way, and an answer's stream of events is counted.
package linescan

import (
	"bufio"
	"bytes"
	"io"
)

// Scanner reads lines in turn, each ending in a line feed, or a carriage
// return and a line feed, or the end of what's read.
type Scanner struct {
	lines *bufio.Scanner
	most  int
	// passing is set while the rest of a line too long to hold is passed over.
	passing bool
	// long is how many lines were passed over.
	long int
}

// New returns a Scanner of the lines r gives, which holds as much of a line
// as most bytes, its line ending included: a line that runs to most bytes
// without its ending, the last of what's read included, is too long to hold.
// It holds a few kilobytes to start, and more only as a line needs them.
func New(r io.Reader, most int) *Scanner {
	s := &Scanner{lines: bufio.NewScanner(r), most: most}
	s.lines.Buffer(nil, most)
	s.lines.Split(s.split)
	return s
}

// Scan reads the next line, reporting false once there are none, at the end
// of what's read or where reading failed, as Err says.
func (s *Scanner) Scan() bool {
	return s.lines.Scan()
}

// Bytes returns the line Scan read, without its line ending, valid until the
// next Scan.
func (s *Scanner) Bytes() []byte {
	return s.lines.Bytes()
}

// Err returns why reading failed, nil where it read to the end.
func (s *Scanner) Err() error {
	return s.lines.Err()
}

// Long returns how many lines were passed over, too long to hold.
func (s *Scanner) Long() int {
	return s.long
}

// split splits data, what the scanner holds, into lines as bufio.ScanLines
// does, but for one too long to hold: once the scanner holds most bytes of a
// line without its ending, the line is passed over to its end, and what
// follows it split in the same call, as a scanner that has read to the end
// splits no more after a call that gives no line, and a decompressed
// stream's last data comes with its end. ScanLines gives the last of what's
// read as a line, ended or not: one that runs to most bytes without its
// ending is too long all the same, however the end of what's read came.
func (s *Scanner) split(data []byte, atEOF bool) (int, []byte, error) {
	passed := 0
	if s.passing {
		end := bytes.IndexByte(data, '\n')
		if end < 0 {
			return len(data), nil, nil
		}
		s.passing, passed = false, end+1
	}
	rest := data[passed:]
	advance, line, err := bufio.ScanLines(rest, atEOF)
	if ended := advance > 0 && rest[advance-1] == '\n'; !ended && len(rest) >= s.most {
		s.passing = true
		s.long++
		return len(data), nil, nil
	}
	return passed + advance, line, err
}
