package views_test

import (
	"reflect"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// detailsOf are the details of the last of the lines' requests, as the Log's
// table gives them, the ledger holding the lines alone.
func detailsOf(t *testing.T, lines ...ledger.Line) views.RequestDetails {
	t.Helper()
	last := lines[len(lines)-1]
	for _, l := range views.LogRows(views.LogSources{From: now, Now: now, Read: logRead(now, lines...)}).Lines {
		if d, ok := l.Details(); ok && l.Row.Request == last.Request {
			return d
		}
	}
	t.Fatalf("the table holds no row of %s", last.Request)
	return views.RequestDetails{}
}

func TestARequestsDetailsAreWhatItsLineGivesBeyondItsRow(t *testing.T) {
	first := withID("r0", line(mover, opus, "work", "new", october(7, 12, 0), hourUsage))
	answered := func(edit func(l *ledger.Line)) ledger.Line {
		l := withID("r1", line(mover, opus, "work", "sticky", october(7, 12, 10), hourUsage))
		edit(&l)
		return l
	}
	atLimit := ledger.Tried{Account: "work", Why: status.WhyLimit}
	tests := []struct {
		name string
		line ledger.Line
		want views.RequestDetails
		left string
	}{
		{
			name: "a turn's, each of its details given",
			line: answered(func(l *ledger.Line) {
				l.Betas = []string{"context-1m-2025-08-07", "interleaved-thinking-2025-05-14"}
				l.Shape = ledger.Shape{System: 3, MaxTokens: new(int64(32_000)), Stream: new(true), OutputConfig: ledger.OutputConfig{Effort: "high"}}
				l.Answer = ledger.Answer{ID: "msg_01", Model: "claude-opus-5-5-20261001", Blocks: map[string]int{"tool_use": 1, "text": 1, "thinking": 2}}
				l.Limits = map[string]string{"5h-utilization": "0.91", "5h-status": "allowed_warning", "status": "allowed_warning"}
			}),
			want: views.RequestDetails{
				Answer: "msg_01", Served: "claude-opus-5-5-20261001", Betas: []string{"context-1m", "interleaved-thinking"},
				LimitStatus: quota.StatusAllowedWarning, System: 3, MaxTokens: 32_000, Effort: "high", Stream: new(true),
				Blocks:    []views.DetailBlock{{Kind: "thinking", Count: 2}, {Kind: "text", Count: 1}, {Kind: "tool_use", Count: 1}},
				CacheLife: ledger.LongCache,
			},
		},
		{
			name: "a move: the account it left and why",
			line: movedFrom("work", withID("r1", line(mover, opus, "side", "moved: work hit its limit", october(7, 12, 10), hourUsage))),
			want: views.RequestDetails{CacheLife: ledger.LongCache},
			left: "[work], at its limit",
		},
		{
			name: "a move sent again after a limit: why it left, not where it went out before",
			line: answered(func(l *ledger.Line) {
				l.From, l.Account, l.Reason, l.Tried = "work", "side", "moved: work hit its limit", []ledger.Tried{atLimit}
			}),
			want: views.RequestDetails{CacheLife: ledger.LongCache},
			left: "[work], at its limit",
		},
		{
			name: "a move of a reason that reads as a session's first's: its routing's words whole",
			line: movedFrom("work", withID("r1", line(mover, opus, "side", "new", october(7, 12, 10), hourUsage))),
			want: views.RequestDetails{CacheLife: ledger.LongCache},
			left: "new session",
		},
		{
			name: "betas, dated and not: each date alone cut off",
			line: answered(func(l *ledger.Line) {
				l.Betas = []string{"oauth", "token-efficient-tools-2025-02-19", "files-api-2025-13-45", "-2025-08-07", "context-1m2025-08-07"}
			}),
			want: views.RequestDetails{
				Betas:     []string{"oauth", "token-efficient-tools", "files-api-2025-13-45", "-2025-08-07", "context-1m2025-08-07"},
				CacheLife: ledger.LongCache,
			},
		},
		{
			name: "an answer of other kinds of block: after those an answer streams, by their names",
			line: answered(func(l *ledger.Line) {
				l.Answer.Blocks = map[string]int{"citations": 1, "server_tool_use": 2, "redacted_thinking": 1, "text": 3, "mcp_tool_use": 1, "compaction": 1}
			}),
			want: views.RequestDetails{
				Blocks: []views.DetailBlock{
					{Kind: "redacted_thinking", Count: 1}, {Kind: "text", Count: 3}, {Kind: "server_tool_use", Count: 2}, {Kind: "mcp_tool_use", Count: 1},
					{Kind: "citations", Count: 1}, {Kind: "compaction", Count: 1},
				},
				CacheLife: ledger.LongCache,
			},
		},
		{
			name: "writing to the cache for five minutes",
			line: withID("r1", line(mover, opus, "work", "sticky", october(7, 12, 10), fiveMinuteUsage)),
			want: views.RequestDetails{CacheLife: ledger.ShortCache},
		},
		{
			name: "writing nothing to the cache: no lifetime",
			line: withID("r1", line(mover, opus, "work", "sticky", october(7, 12, 10), smallUsage)),
		},
		{
			name: "one the router answered itself: nothing beyond its row",
			line: answered(func(l *ledger.Line) { l.Status, l.Attempts, l.Account, l.Usage = 429, 0, "", nil }),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detailsOf(t, first, tt.line)
			if left := routingWords(views.RoutingSaid{Words: got.Left}); left != tt.left {
				t.Errorf("its details say it left %q, want %q", left, tt.left)
			}
			got.Row, got.Left = views.RequestRow{}, nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("its details are\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// TestARequestsDetailsAreItsRowAndItsOwnLineAtEachHardMoment holds each
// row's details, at each hard moment, every kind shown, to its row and, once
// it has come, its line: a request the stream told of alone has its row's
// details and no more, and a rule none.
func TestARequestsDetailsAreItsRowAndItsOwnLineAtEachHardMoment(t *testing.T) {
	lined, told := 0, 0
	for _, hm := range viewstest.LogMoments() {
		src := logSourcesOf(hm)
		src.Filter = views.LogFilter{}
		for _, l := range views.LogRows(src).Lines {
			d, ok := l.Details()
			if l.Rule != nil {
				if ok {
					t.Errorf("%s: %s has details", hm.Name, tableLines(views.LogTable{Lines: []views.LogLine{l}}))
				}
				continue
			}
			want := views.RequestDetails{Row: l.Row}
			if line, has := lineOf(hm.Lines, l.Row); has {
				want = views.LinedDetails(l.Row, line)
				lined++
			} else {
				told++
			}
			if !ok || !reflect.DeepEqual(d, want) {
				t.Errorf("%s: %s's details are\n%+v\nwant\n%+v", hm.Name, l.Row.Request, d, want)
			}
		}
	}
	if lined < 60 || told < 8 {
		t.Errorf("the moments hold the details of %d requests with lines and %d told of alone, want 60 and 8 at least", lined, told)
	}
}

// lineOf is the line of the request the row is of, among the lines given:
// false where the row's line hasn't come.
func lineOf(lines []ledger.Line, row views.RequestRow) (ledger.Line, bool) {
	for _, l := range lines {
		if row.Stage == views.RowLined && l.Request == row.Request && l.At.Equal(row.At) {
			return l, true
		}
	}
	return ledger.Line{}, false
}
