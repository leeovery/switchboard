package ledger

import "testing"

// A day's parts file is held as its digest's head records it where it's as
// long as recorded, and where none is recorded, as none is there.
func TestAPartsFileIsHeldWhereItsAsLongAsItsHeadRecords(t *testing.T) {
	tests := []struct {
		name string
		// file is what the day's parts file holds: nil for none.
		file []byte
		size int64
		want bool
	}{
		{name: "none, none recorded", size: 0, want: true},
		{name: "none, of some bytes recorded", size: 40, want: false},
		{name: "as long as recorded", file: make([]byte, 40), size: 40, want: true},
		{name: "longer than recorded", file: make([]byte, 41), size: 40, want: false},
		{name: "shorter than recorded", file: make([]byte, 39), size: 40, want: false},
		{name: "of some bytes, none recorded", file: make([]byte, 40), size: 0, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if tt.file != nil {
				held(t, partsFile(d.files.Dir, digested), tt.file)
			}

			if got := d.partsHeld(digested, tt.size); got != tt.want {
				t.Errorf("partsHeld(%q, %d) = %t, want %t, the file of %d bytes, there %t", digested, tt.size, got, tt.want, len(tt.file), tt.file != nil)
			}
		})
	}
}
