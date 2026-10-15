package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// poolNow is when the pool's charts are drawn in their tests: 14:00, its
// 5-hour charted in five columns, an hour each, at 10:00, 11:00, 12:00,
// 13:00 and now.
var poolNow = clockOn(14, 0)

// poolDoc is the router's document at poolNow of two accounts, neither
// keeping a reserve: work, its session half used, resetting at workResets;
// and personal, its session a quarter used, resetting at personalResets.
func poolDoc(workResets, personalResets time.Time) status.Document {
	read := func(id string, used float64, resets time.Time) status.Account {
		return status.Account{
			ID: id, Label: id, TokenSet: true, FetchedAt: clockOn(13, 30),
			Windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: used, ResetsAt: resets}},
		}
	}
	return status.Document{
		GeneratedAt: poolNow, Source: status.SourceRouter, Router: status.Health{Healthy: true},
		Accounts: []status.Account{read("work", 0.5, workResets), read("personal", 0.25, personalResets)},
	}
}

// pooled is poolDoc of work's session started at 12:00, and personal's at
// 13:00.
func pooled() status.Document {
	return poolDoc(clockOn(17, 0), clockOn(18, 0))
}

// poolRead are work's and personal's readings of their sessions over the 5
// hours back from poolNow: each session before reset at 11:30 and 12:30.
var poolRead = map[string][]views.WindowReading{
	"work": {
		{At: clockOn(10, 0), Utilization: 0.4, ResetsAt: clockOn(11, 30)},
		{At: clockOn(13, 30), Utilization: 0.5, ResetsAt: clockOn(17, 0)},
	},
	"personal": {
		{At: clockOn(11, 0), Utilization: 0.75, ResetsAt: clockOn(12, 30)},
		{At: clockOn(13, 30), Utilization: 0.25, ResetsAt: clockOn(18, 0)},
	},
}

// poolChartIn is the pool's 5-hour chart of doc at poolNow from the
// readings given, drawn as burn has it in look, width cells wide, h rows of
// chart between its top row and its axis, on a canvas of its own.
func poolChartIn(doc status.Document, read map[string][]views.WindowReading, burn Burn, look Look, width, h int) *canvas {
	pc := poolChart{
		name: "5-HOUR", length: 5 * time.Hour, now: poolNow, burn: burn,
		room: views.PoolRoomFrom{Moment: doc.At(poolNow, printPolicy), Key: "5h", Read: read},
	}
	c := newCanvas(width, h+2)
	pc.draw(c, look, bounds{width: width, rows: h + 2})
	return c
}

// The pool's room, of 2 accounts, is 0.6 at 10:00, work's alone, personal's
// unknown; 0.85 at 11:00; 1.25 at 12:00, work's session started again; 2 at
// 13:00, personal's too; and 1.25 at now, as the moment has it: the shares
// 0.3, 0.425, 0.625, 1 and 0.625 of all of it.
func TestAPoolsChartFillsEachColumnToItsRoomAsItsBurnHasIt(t *testing.T) {
	tests := []struct {
		burn Burn
		want []int
	}{
		{burn: BurnUp, want: []int{22, 18, 12, 0, 12}},
		{burn: BurnDown, want: []int{10, 14, 20, 32, 20}},
	}
	nord := Screen(builtin(t, "nord"))
	for _, tt := range tests {
		t.Run(tt.burn.Name(), func(t *testing.T) {
			if got := filledOf(poolChartIn(pooled(), poolRead, tt.burn, nord, 5, 4), 4); !slices.Equal(got, tt.want) {
				t.Errorf("the pool's %s fills its columns to %v eighths, want %v", tt.burn.Name(), got, tt.want)
			}
		})
	}
}

// Its burn-down's level, of 25 columns of 12 minutes each, from 09:00:
// with nothing read, the pool's room is known from 12:00, as work's session
// started, half of it; from 13:00, as personal's did, all of it; and at
// now, as the moment has it, 0.625 of it.
func TestAPoolsChartIsBlankWhereItsRoomIsntKnownAndSaysSoWithNoHistory(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	known := filledOf(poolChartIn(pooled(), nil, BurnDown, nord, 25, 4), 4)
	want := slices.Concat(make([]int, 14), slices.Repeat([]int{16}, 5), slices.Repeat([]int{32}, 5), []int{20})
	if !slices.Equal(known, want) {
		t.Errorf("the pool's burn-down with nothing read fills its columns to %v eighths, want %v: from each session's start", known, want)
	}
	unplaced := poolChartIn(poolDoc(time.Time{}, time.Time{}), nil, BurnDown, nord, 25, 4)
	if got := filledOf(unplaced, 4); slices.ContainsFunc(got[:24], func(e int) bool { return e > 0 }) || got[24] != 20 {
		t.Errorf("the pool's burn-down of sessions read without resets fills its columns to %v eighths, want none but now's, the moment's 20", got)
	}
	if got := rowOf(unplaced, 3); !strings.Contains(got, untracedLabel) {
		t.Errorf("its middle row reads %q, want %q across it", got, untracedLabel)
	}
}

func TestAPoolsChartsAxisIsItsLengthBackFromNowAndNow(t *testing.T) {
	c := poolChartIn(pooled(), poolRead, BurnUp, Look{}, 25, 4)
	if got, want := rowOf(c, 5), "09:00"+strings.Repeat(" ", 17)+"now"; got != want {
		t.Errorf("the pool's chart's axis reads %q, want %q", got, want)
	}
}

func TestByAccountThePoolsChartsSitBesideItsBlock(t *testing.T) {
	f := besideOf(160)
	rows, _ := usageBody(f, trio(), printNow, 40)
	i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "Σ pool") })
	if i < 0 {
		t.Fatalf("USAGE by account reads\n%s\nwant the pool's block", strings.Join(rows, "\n"))
	}
	if got, want := colAt(rows[i], 107, 53), titlesRow(25, besideGap, "5-HOUR", "WEEK"); got != want {
		t.Errorf("beside the pool's block, the charts' top row reads\n%q\nwant\n%q", got, want)
	}
	if got := colAt(rows[i+5], 107, 5); got != "09:42" {
		t.Errorf("the row under the pool's block reads %q beside it, want the 5-hour's axis, from 5 hours back, 09:42", got)
	}
}
