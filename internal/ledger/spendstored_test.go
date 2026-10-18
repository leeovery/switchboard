package ledger_test

import (
	"encoding/json"
	"fmt"
	"hash/maphash"
	"reflect"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// spendSeed seeds the sums the Spend's tests take.
var spendSeed = maphash.MakeSeed()

// A Spend read back from its stored form is the Spend written, every usage
// on every kind of line: so every call priced over it gives what the line's
// own call gives, and it sums as the Spend written did.
func TestASpendReadBackFromItsStoredFormIsTheOneWritten(t *testing.T) {
	for _, u := range spendUsages {
		for _, k := range spendKinds {
			t.Run(u.name+", "+k.name, func(t *testing.T) {
				l := spendLine(u.usage, k.edit)
				for _, problem := range storedSpendDisagreements(&l) {
					t.Error(problem)
				}
			})
		}
	}
}

// Fuzzed, a Spend read back from its stored form is the Spend written,
// whatever the usage, on each kind of line.
func FuzzASpendReadBackFromItsStoredFormIsTheOneWritten(f *testing.F) {
	for _, u := range spendUsages {
		for i := range spendKinds {
			f.Add([]byte(u.usage), uint8(i))
		}
	}
	f.Fuzz(func(t *testing.T, usage []byte, kind uint8) {
		l := spendLine(string(usage), spendKinds[int(kind)%len(spendKinds)].edit)
		for _, problem := range storedSpendDisagreements(&l) {
			t.Error(problem)
		}
	})
}

// storedSpendDisagreements are where the Spend of the line l, as SpendOf
// decodes it, written in its stored form and read back, isn't the Spend
// written, sums otherwise, or gives otherwise than the line's own calls, as
// spendDisagreements has them: none where it agrees.
func storedSpendDisagreements(l *ledger.Line) []string {
	spend := ledger.SpendOf(l)
	stored, err := json.Marshal(spend)
	if err != nil {
		return []string{fmt.Sprintf("of %q, writing the spend: %v", l.Usage, err)}
	}
	var back ledger.Spend
	if err := json.Unmarshal(stored, &back); err != nil {
		return []string{fmt.Sprintf("of %q, reading back %s: %v", l.Usage, stored, err)}
	}
	var problems []string
	if !reflect.DeepEqual(back, spend) {
		problems = append(problems, fmt.Sprintf("of %q, stored as %s, read back as %#v, want %#v", l.Usage, stored, back, spend))
	}
	if back.Sum(spendSeed) != spend.Sum(spendSeed) {
		problems = append(problems, fmt.Sprintf("of %q, stored as %s, read back sums otherwise than the spend written", l.Usage, stored))
	}
	return append(problems, spendDisagreements(l, back)...)
}

// Two spends sum alike where they're stored alike, and otherwise not, of
// every usage on every kind of line: so a spend's sum stands for what's
// stored of it.
func TestSpendsSumAlikeWhereTheyreStoredAlike(t *testing.T) {
	type summed struct {
		name, stored string
		sum          uint64
	}
	var spends []summed
	for _, u := range spendUsages {
		for _, k := range spendKinds {
			l := spendLine(u.usage, k.edit)
			spend := ledger.SpendOf(&l)
			stored, err := json.Marshal(spend)
			if err != nil {
				t.Fatalf("%s, %s: writing the spend: %v", u.name, k.name, err)
			}
			spends = append(spends, summed{name: u.name + ", " + k.name, stored: string(stored), sum: spend.Sum(spendSeed)})
		}
	}
	for i, a := range spends {
		for _, b := range spends[i+1:] {
			if alike := a.sum == b.sum; alike != (a.stored == b.stored) {
				t.Errorf("%s, stored as %s, and %s, stored as %s, sum alike: %t, want %t", a.name, a.stored, b.name, b.stored, alike, !alike)
			}
		}
	}
}

// storedSpends are a Spend's stored forms, each holding one of its keys
// alone, every key among them, and the paths it leaves unpriced parted
// otherwise, and placed in its bill or in its writes' alone.
var storedSpends = []string{
	`{}`,
	`{"i":1}`, `{"5m":1}`, `{"1h":1}`, `{"r":1}`, `{"o":1}`, `{"s":1}`, `{"u":["a"]}`,
	`{"w":1}`, `{"b":1}`, `{"d":true}`, `{"c":true}`, `{"q":true}`, `{"n":true}`,
	`{"x":{}}`, `{"x":{"i":1}}`, `{"x":{"o":1}}`, `{"x":{"r":1}}`, `{"x":{"w":1}}`,
	`{"x":{"b":{"i":1}}}`, `{"x":{"b":{"5m":1}}}`, `{"x":{"b":{"1h":1}}}`, `{"x":{"b":{"r":1}}}`,
	`{"x":{"b":{"o":1}}}`, `{"x":{"b":{"s":1}}}`, `{"x":{"b":{"u":["a"]}}}`,
	`{"u":["a","b"]}`, `{"u":["ab"]}`, `{"u":[""]}`, `{"u":["a"],"x":{"b":{"u":["b"]}}}`, `{"u":["a","b"],"x":{}}`,
}

// Each key of a Spend's stored form reads back into a Spend that writes it
// again as it was, and each sums otherwise than every other: so nothing
// stored is lost between the file and the Spend, nor left out of its sum.
func TestEachKeyOfASpendsStoredFormIsKept(t *testing.T) {
	sums := make(map[uint64]string)
	for _, stored := range storedSpends {
		var s ledger.Spend
		if err := json.Unmarshal([]byte(stored), &s); err != nil {
			t.Errorf("reading %s: %v", stored, err)
			continue
		}
		if again, err := json.Marshal(s); err != nil || string(again) != stored {
			t.Errorf("%s read back writes as %s, %v, want it as it was", stored, again, err)
		}
		sum := s.Sum(spendSeed)
		if other, ok := sums[sum]; ok {
			t.Errorf("%s sums as %s does, want each otherwise", stored, other)
		}
		sums[sum] = stored
	}
}

// A Spend's stored form is the counts of its usage and its flags alone,
// each by a short key, every zero left out, and never a price: so a typical
// answer's is some seventy bytes.
func TestASpendsStoredFormIsItsCountsAlone(t *testing.T) {
	tests := []struct {
		name, usage string
		edit        func(l *ledger.Line)
		want        string
	}{
		{
			name: "a typical answer", edit: func(*ledger.Line) {},
			usage: `{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":100000,` +
				`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":1000},"output_tokens":500,` +
				`"service_tier":"standard","server_tool_use":{"web_search_requests":0,"web_fetch_requests":0}}`,
			want: `{"i":10,"1h":1000,"r":100000,"o":500,"w":1000,"b":1000,"d":true,"c":true}`,
		},
		{name: "never upstream", edit: func(l *ledger.Line) { l.Attempts, l.Status = 0, 429 }, want: `{}`},
		{name: "upstream, no usage given", edit: func(l *ledger.Line) { l.CutOff, l.Status = true, 0 }, want: `{"n":true}`},
		{name: "a quota check", usage: `{"input_tokens":8,"output_tokens":1}`, edit: func(l *ledger.Line) { l.Kind = ledger.KindCheck },
			want: `{"i":8,"o":1,"d":true,"c":true,"q":true}`},
		{name: "a count the prices don't know", usage: `{"output_tokens":5,"server_tool_use":{"code_execution_requests":1}}`, edit: func(*ledger.Line) {},
			want: `{"o":5,"u":["server_tool_use.code_execution_requests"],"d":true,"c":true}`},
		{name: "a count's name in capitals", usage: `{"Input_Tokens":12,"output_tokens":10}`, edit: func(*ledger.Line) {},
			want: `{"o":10,"u":["Input_Tokens"],"d":true,"c":true,"x":{"i":12,"o":10}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := spendLine(tt.usage, tt.edit)
			if got, err := json.Marshal(ledger.SpendOf(&l)); err != nil || string(got) != tt.want {
				t.Errorf("of %q, stored as %s, %v, want %s", tt.usage, got, err, tt.want)
			}
		})
	}
}
