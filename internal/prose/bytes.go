package prose

import "strconv"

// byteUnits are the units Bytes counts in past bytes, each 1,024 of the one
// before.
var byteUnits = []string{"KB", "MB", "GB"}

// Bytes counts n bytes as briefly as the Log's size has room for, in six
// cells, each unit 1,024 of the one before: as they are under a thousand, as
// "512 B"; then in the least unit that holds them under a thousand, to a
// tenth under ten of it, as "1.2 MB", and whole from ten, as "482 KB"; and
// past a thousand megabytes, in gigabytes. Each form is for what rounds to
// it, a half rounded up, so 1,000 bytes are "1.0 KB", and 999.5 KB "1.0 MB".
func Bytes(n int) string {
	if n < 1000 {
		return strconv.Itoa(n) + " B"
	}
	size := 1
	for _, unit := range byteUnits {
		size *= 1024
		switch {
		case rounded(n*10, size) < 100:
			return inTenths(rounded(n*10, size), " "+unit)
		case rounded(n, size) < 1000:
			return strconv.Itoa(rounded(n, size)) + " " + unit
		}
	}
	return strconv.Itoa(rounded(n, size)) + " " + byteUnits[len(byteUnits)-1]
}
