package capture

import (
	"encoding/json"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// d28cPlan is d28c as its page draws it (session2.D28C, session_final.D28C):
// in ~/Code/switchboard, started at 12:02:10, its 13 turns, on work until work
// reached its cap, then on side from 14:31, its newest turn's request out
// since 14:42:00, asking, as the Log draws it; turn 12's first five requests
// as its opened turn draws them.
func d28cPlan(now time.Time) sessionPlan {
	return sessionPlan{
		id: idD28C, dir: "~/Code/switchboard", start: clockAt(now, 0, 12, 2, 0), first: clockAt(now, 0, 12, 2, 10),
		turns: []turnPlan{
			{0, 4, 18, []toolCount{{"Read", 9}, {"Grep", 5}, {"Glob", 2}}}, {9, 11, 46, []toolCount{{"Read", 14}, {"Edit", 12}, {"Bash", 10}}},
			{26, 3, 9, []toolCount{{"Read", 4}, {"Bash", 2}}}, {33, 14, 61, []toolCount{{"Bash", 19}, {"Edit", 15}, {"Read", 13}}},
			{52, 6, 22, []toolCount{{"Read", 8}, {"Edit", 6}, {"Bash", 3}}}, {64, 2, 5, []toolCount{{"Read", 2}}},
			{70, 19, 83, []toolCount{{"Bash", 27}, {"Edit", 21}, {"Read", 17}}}, {95, 8, 31, []toolCount{{"Read", 11}, {"Edit", 8}, {"Bash", 6}}},
			{108, 5, 17, []toolCount{{"Bash", 7}, {"Read", 5}}}, {118, 12, 52, []toolCount{{"Edit", 16}, {"Bash", 14}, {"Read", 12}}},
			{133, 15, 58, []toolCount{{"Bash", 20}, {"Edit", 13}, {"Read", 13}}}, {147, 9, 37, []toolCount{{"Bash", 14}, {"Edit", 9}, {"Read", 8}}},
			{158, 3, 6, []toolCount{{"Read", 3}, {"Bash", 2}}},
		},
		stays: []stay{{"work", 0}, {"side", 149}},
		moves: []movePlan{{minute: 149, written: 182_000, reason: reasonCapped}},
		drawn: map[int][]ledger.Line{12: d28cTurn12(now)},
		asked: clockAt(now, 0, 14, 42, 0),
	}
}

// d28cTurn12 are the first five requests of d28c's turn 12 as its opened turn
// draws them (session_final.REQUESTS): two on work, then, work at its cap,
// one on side writing the whole context again there, as a request's details
// in the Log draw it, a side request, and one reading the context back.
func d28cTurn12(now time.Time) []ledger.Line {
	at := func(minute, second int) time.Time { return clockAt(now, 0, 14, minute, second).UTC() }
	asked := func(when time.Time, model, account, class string, first, took float64, tokens usageTokens, stop string, tools ...string) ledger.Line {
		firstMS := int64(first * 1000)
		l := ledger.Line{At: when, Model: model, Account: account, FirstMS: &firstMS, TotalMS: int64(took * 1000), Usage: tokens.usage()}
		l.Class, l.Answer.Stop, l.Answer.Tools = class, stop, tools
		return l
	}
	move := asked(at(31, 13), opus, "side", classMain, 2.9, 14.6, usageTokens{input: 3, written: 182_004, out: 1_214}, "tool_use", "Bash")
	move.Request = "3f2a91c4"
	move.Betas = []string{"context-1m-2025-08-07", "interleaved-thinking-2025-05-14"}
	maxTokens, budget, stream := int64(32_000), int64(31_999), true
	move.Shape = ledger.Shape{Bytes: 482 * 1024, Messages: 214, System: 3, Tools: 31, MaxTokens: &maxTokens,
		Thinking: ledger.Thinking{Type: "enabled", BudgetTokens: &budget}, Stream: &stream, OutputConfig: ledger.OutputConfig{Effort: "high"}}
	move.Answer.ID, move.Answer.Model = "req_011CQx4n7RkV2bW9pT3sYd6M", opus
	move.Answer.Blocks = map[string]int{"thinking": 2, "text": 1, "tool_use": 1}
	move.Limits = map[string]string{"5h-utilization": "0.09", "7d-utilization": "0.77", "status": "allowed"}
	return []ledger.Line{
		asked(at(29, 4), opus, "work", classMain, .8, 11.2, usageTokens{input: 3, read: 176_000, written: 2_100, out: 1_300}, "tool_use", "Bash"),
		asked(at(30, 42), opus, "work", classMain, .9, 12.4, usageTokens{input: 3, read: 181_000, written: 2_400, out: 1_100}, "tool_use", "Bash"),
		move,
		asked(at(31, 44), haikuShort, "side", classAuxiliary, .3, .6, usageTokens{input: 3, out: 40}, "end_turn"),
		asked(at(31, 58), opus, "side", classMain, 1.1, 9.8, usageTokens{input: 3, read: 182_000, written: 2_600, out: 900}, "tool_use", "Edit"),
	}
}

// quietPlan is 9c2e as its page draws it (session2.QUIET): in ~/Code/website,
// started at 11:55:31, on personal since, its ten turns sampled from seed 7,
// idle since its last.
func quietPlan(now time.Time) sessionPlan {
	return sessionPlan{
		id: id9C2E, dir: "~/Code/website", start: clockAt(now, 0, 11, 55, 0), first: clockAt(now, 0, 11, 55, 31),
		turns: turnsFrom([][2]int{{0, 158}}, 7),
		stays: []stay{{"personal", 0}},
	}
}

// endedPlan is b9c4 as its page draws it (session2.ENDED, session_final.ENDED):
// in ~/Code/switchboard, yesterday from 13:20, on work, then on side from
// 16:02, when work reached its cap, its 16 turns sampled from seed 11, its
// last from 18:36.
func endedPlan(now time.Time) sessionPlan {
	turns := turnsFrom([][2]int{{0, 322}}, 11)
	last := &turns[len(turns)-1]
	last.start = 327 - last.minutes
	return sessionPlan{
		id: idB9C4, dir: "~/Code/switchboard", start: clockAt(now, -1, 13, 20, 0),
		turns: turns,
		stays: []stay{{"work", 0}, {"side", 162}},
		moves: []movePlan{{minute: 162, written: 188_000, reason: reasonCapped}},
	}
}

// resumedPlan is f41a as its page draws it (session2.RESUMED,
// session_final.RESUMED): in ~/Code/api, yesterday from 16:40 on side, idle
// from 18:05, then today from 09:12 on personal, rescored after 15 hours
// idle, its cache gone, its 21 turns sampled from seed 3, an answer streaming
// in its newest, to a request out since 14:41:47.
func resumedPlan(now time.Time) sessionPlan {
	return sessionPlan{
		id: idF41A, dir: "~/Code/api", start: clockAt(now, -1, 16, 40, 0),
		turns: turnsFrom([][2]int{{0, 85}, {992, 1322}}, 3),
		stays: []stay{{"side", 0}, {"personal", 992}},
		moves: []movePlan{{minute: 992, written: 96_000, reason: status.Rescored("15h")}},
		asked: clockAt(now, 0, 14, 41, 47),
	}
}

// heldAsWritten is the lines as the ledger holds them, each with its JSON as it's
// written.
func heldAsWritten(lines []ledger.Line) []ledger.Held {
	kept := make([]ledger.Held, len(lines))
	for i, l := range lines {
		data, _ := json.Marshal(l)
		kept[i] = ledger.Held{Line: l, JSON: data}
	}
	return kept
}
