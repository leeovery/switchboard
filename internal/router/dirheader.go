package router

import (
	"fmt"
	"net/url"
	"strings"
)

// EncodeDir returns dir as DirHeader carries it, percent-encoding what a
// header can't carry as it is, and nothing else, so ~/Code/my project reads
// as it is: a control character, such as a newline, which would end the
// header's line in ANTHROPIC_CUSTOM_HEADERS; each byte of a character past
// ASCII, which a header doesn't carry as UTF-8; a space at either end, which
// would be trimmed off; and a percent sign, which would read as an encoding.
func EncodeDir(dir string) string {
	var b strings.Builder
	for i := range len(dir) {
		c := dir[i]
		edge := i == 0 || i == len(dir)-1
		if c < ' ' || c > '~' || c == '%' || (c == ' ' && edge) {
			fmt.Fprintf(&b, "%%%02X", c)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// DecodeDir returns the directory a DirHeader value names, as EncodeDir
// encodes it: "" for one that doesn't decode, which run never sends.
func DecodeDir(value string) string {
	dir, err := url.PathUnescape(value)
	if err != nil {
		return ""
	}
	return dir
}
