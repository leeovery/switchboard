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

// A Spent read back from its stored form is the Spent written, of every
// generated set of requests: so it prices as it did, and takes in more
// alike.
func TestASpentReadBackFromItsStoredFormIsTheOneWritten(t *testing.T) {
	for seed := range uint64(spentSeeds) {
		spent := spentOfLines(spentLines(seed))
		stored, err := json.Marshal(spent)
		if err != nil {
			t.Fatalf("seed %d: writing the Spent: %v", seed, err)
		}
		var back ledger.Spent
		if err := json.Unmarshal(stored, &back); err != nil {
			t.Fatalf("seed %d: reading back %s: %v", seed, stored, err)
		}
		if !reflect.DeepEqual(back, spent) {
			t.Errorf("seed %d: stored as %s, read back as %#v; want %#v", seed, stored, back, spent)
		}
	}
}

// storedSpentForms are a Spent's stored forms: none; a model and geo
// holding nothing but a request; each of its keys alone, every key among
// them; and models and geos side by side, in order.
var storedSpentForms = []string{
	`[]`, `[{}]`,
	`[{"m":"a"}]`, `[{"g":"us"}]`,
	`[{"i":1}]`, `[{"5m":1}]`, `[{"1h":1}]`, `[{"r":1}]`, `[{"o":1}]`, `[{"s":1}]`, `[{"u":["a"]}]`,
	`[{"c":true}]`, `[{"n":true}]`,
	`[{"u":["a","b"]}]`, `[{},{"g":"us"}]`, `[{"m":"a"},{"m":"b"}]`, `[{"m":"a","g":"us"},{"m":"b"}]`,
}

// Each key of a Spent's stored form reads back into a Spent that writes it
// again as it was: so nothing stored is lost between the file and the
// Spent.
func TestEachKeyOfASpentsStoredFormIsKept(t *testing.T) {
	for _, stored := range storedSpentForms {
		var s ledger.Spent
		if err := json.Unmarshal([]byte(stored), &s); err != nil {
			t.Errorf("reading %s: %v", stored, err)
			continue
		}
		if again, err := json.Marshal(s); err != nil || string(again) != stored {
			t.Errorf("%s read back writes as %s, %v, want it as it was", stored, again, err)
		}
	}
}

// Fuzzed, whatever a Spent's stored form holds that reads, it reads as a
// Spent that writes, and reads back as itself.
func FuzzASpentsStoredFormReadsAsOneThatReadsBackAsItself(f *testing.F) {
	for _, stored := range storedSpentForms {
		f.Add([]byte(stored))
	}
	f.Add([]byte(`[{"m":"b"},{"m":"a","u":["b","a","a"]},{"m":"a","i":-1}]`))
	f.Fuzz(func(t *testing.T, stored []byte) {
		var s ledger.Spent
		if json.Unmarshal(stored, &s) != nil {
			return
		}
		again, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("%s read back doesn't write: %v", stored, err)
		}
		var back ledger.Spent
		if err := json.Unmarshal(again, &back); err != nil || !reflect.DeepEqual(back, s) {
			t.Errorf("%s reads back as %#v, which writes as %s and reads back as %#v, %v; want it as it was", stored, s, again, back, err)
		}
	})
}

// A Spent's stored form, of models and geos out of order or named more than
// once, or naming a path unpriced out of order or more than once, as a file
// edited by hand could, reads as the Spent Add would have made of the same
// requests.
func TestASpentStoredOutOfOrderReadsAsAddWouldHaveMadeIt(t *testing.T) {
	tests := []struct {
		name, stored, want string
	}{
		{name: "models out of order", stored: `[{"m":"b"},{"m":"a"}]`, want: `[{"m":"a"},{"m":"b"}]`},
		{name: "geos out of order", stored: `[{"g":"us"},{}]`, want: `[{},{"g":"us"}]`},
		{name: "a model and geo named twice", stored: `[{"m":"a","i":1,"u":["x"]},{"m":"b"},{"m":"a","i":2,"u":["w"],"n":true}]`,
			want: `[{"m":"a","i":3,"u":["w","x"],"n":true},{"m":"b"}]`},
		{name: "paths unpriced out of order and twice", stored: `[{"u":["b","a","b"]}]`, want: `[{"u":["a","b"]}]`},
		{name: "null, none", stored: `null`, want: `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s ledger.Spent
			if err := json.Unmarshal([]byte(tt.stored), &s); err != nil {
				t.Fatalf("reading %s: %v", tt.stored, err)
			}
			if got, err := json.Marshal(s); err != nil || string(got) != tt.want {
				t.Errorf("%s read back writes as %s, %v; want %s", tt.stored, got, err, tt.want)
			}
		})
	}
}

// What isn't a list of models' and geos' spends, each count a number and
// each path a string, isn't read as a Spent.
func TestWhatIsntASpentsStoredFormIsntReadAsOne(t *testing.T) {
	for _, stored := range []string{`{}`, `"x"`, `[1]`, `[{"i":"1"}]`, `[{"u":"a"}]`, `[{"m":1}]`, `[`} {
		t.Run(stored, func(t *testing.T) {
			var s ledger.Spent
			if err := json.Unmarshal([]byte(stored), &s); err == nil {
				t.Errorf("reading %s gave %s, want it refused", stored, spentJSON(s))
			}
		})
	}
}

// A Spent's stored form is what it holds of each model and geo, in order,
// each by short keys, every zero left out, and never a price.
func TestASpentsStoredFormIsItsCountsAlone(t *testing.T) {
	tests := []struct {
		name  string
		lines []ledger.Line
		want  string
	}{
		{name: "none", want: `[]`},
		{name: "a session's requests", lines: []ledger.Line{
			spentLine(opus, "", `{"input_tokens":10,"cache_read_input_tokens":100000,"output_tokens":500}`, answered),
			spentLine(opus, "", `{"input_tokens":5,"output_tokens":20,"server_tool_use":{"web_search_requests":1,"web_fetch_requests":2}}`, answered),
			spentLine(haiku, "", `{"input_tokens":8,"output_tokens":1}`, checked),
			spentLine("claude-opus-9", "us", "", cutOff),
		}, want: `[{"m":"claude-haiku-4-5"},{"m":"claude-opus-5-5","i":15,"r":100000,"o":520,"s":1,"c":true},{"m":"claude-opus-9","g":"us","n":true}]`},
		{name: "the cache's writes, and a count the prices don't know", lines: []ledger.Line{
			spentLine(opus, "", `{"cache_creation_input_tokens":3120,"cache_creation":{"ephemeral_5m_input_tokens":120,"ephemeral_1h_input_tokens":3000}}`, answered),
			spentLine(opus, "", `{"output_tokens":5,"server_tool_use":{"code_execution_requests":1}}`, answered),
		}, want: `[{"m":"claude-opus-5-5","5m":120,"1h":3000,"o":5,"u":["server_tool_use.code_execution_requests"],"c":true}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := json.Marshal(spentOfLines(tt.lines)); err != nil || string(got) != tt.want {
				t.Errorf("stored as %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

// A Spent that holds no request is left out of a stored form that leaves
// out its zeros, and so is one read back from its own stored form.
func TestASpentOfNoRequestIsLeftOutWhereZerosAre(t *testing.T) {
	type holding struct {
		Spent ledger.Spent `json:"s,omitzero"`
	}
	var read ledger.Spent
	if err := json.Unmarshal([]byte(`[]`), &read); err != nil {
		t.Fatalf("reading []: %v", err)
	}
	for _, spent := range []ledger.Spent{{}, read} {
		if got, err := json.Marshal(holding{Spent: spent}); err != nil || string(got) != `{}` {
			t.Errorf("a Spent of no request is stored within as %s, %v; want it left out, {}", got, err)
		}
	}
}
