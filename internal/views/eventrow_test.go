package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

func TestAnEventsRowIsWhenItHappenedItsSessionAndItsLineAsTheLogTellsIt(t *testing.T) {
	east := time.FixedZone("east", 2*60*60)
	now := time.Date(2026, 10, 7, 13, 12, 0, 0, east)
	run := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
	pressed := events.Held{Run: run, ID: 1, At: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC), Kind: status.EventPressure, Account: "work",
		Windows: []string{"5h"}, Until: time.Date(2026, 10, 7, 10, 5, 0, 0, time.UTC)}
	moved := events.Held{Run: run, ID: 2, At: time.Date(2026, 10, 7, 9, 30, 0, 0, time.UTC), Kind: status.EventMoved, Session: "5b0e7c1a-1f2a",
		From: "work", To: "side", Reason: status.PassingOver(status.ReasonNew, "work")}

	telling := views.NewTelling(status.NameOfKey)
	if row := telling.Row(pressed, now); !row.At.Equal(pressed.At) || row.At.Location() != east || row.Session != "" ||
		row.Kind != "pressure" || wordsSaid(row.Account) != "[work]" || wordsSaid(row.What) != "its 5-hour to run out at 12:05" {
		t.Errorf("the pressure's row is %+v, want it at 11:00 in now's zone, of no session, told as the Log's Events tell it", row)
	}
	// The move's facts take in the events told before it: its choice passed
	// work over as it was to run out at 12:05.
	if row := telling.Row(moved, now); row.Session != "5b0e7c1a-1f2a" || wordsSaid(row.What) != "[work] came under pressure, its 5-hour to run out at 12:05" {
		t.Errorf("the move's row is %+v, want its session, and what happened, the pressure told before it taken in", row)
	}
}
