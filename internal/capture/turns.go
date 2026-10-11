package capture

import (
	"cmp"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// haikuShort is the small model the frames name haiku-4-5, by the id the
// price table and the version table know it by too.
const haikuShort = "claude-haiku-4-5"

// The class of a request of a session's own conversation, and of a side
// request Claude Code makes for a prompt, as its headers say them.
const (
	classMain      = ledger.ClassMain
	classAuxiliary = "auxiliary"
)

// The usage a session's page's generator samples a request at
// (session2.PRICE's sample): 3 input tokens, 3,000 written to the cache for
// an hour, and 900 out, reading its context back, which grows from 40,000
// tokens by 750 a minute over its first 200 minutes.
const (
	sampleInput   = 3
	sampleWritten = 3_000
	sampleOut     = 900
)

// sampleContext is the context a session's request reads back from the cache
// at the given minute of its life, as session2.context gives it.
func sampleContext(minute float64) int {
	return 40_000 + int(150_000*min(minute, 200)/200)
}

// turnPlan is a turn as a session's page's generator samples it: its first
// minute, counted from the session's, how many minutes it takes, its
// requests, and the tools its answers called most, most first.
type turnPlan struct {
	start, minutes, requests int
	tools                    []toolCount
}

// toolCount is a tool its answers called, and how many times.
type toolCount struct {
	tool  string
	times int
}

// stay is an account a session ran on from a minute of its life.
type stay struct {
	account string
	from    int
}

// movePlan is a move of a session's: the minute of its life it came in, what
// the request that made it wrote to the cache, and why, as the router said.
type movePlan struct {
	minute, written int
	reason          string
}

// sessionPlan is a session as a session's page's generator samples it: its
// id and directory; its first minute, and, where its started event says, its
// first request; its turns, the accounts it ran on and its moves; the
// requests of a turn given as they're drawn, by its number from 1; and, of
// one whose newest turn is still going, when the request in flight now went
// out, its lines all ended before.
type sessionPlan struct {
	id, dir string
	start   time.Time
	first   time.Time
	turns   []turnPlan
	stays   []stay
	moves   []movePlan
	drawn   map[int][]ledger.Line
	asked   time.Time
}

// turnsFrom are the turns over the spans of a session's life given, in its
// minutes, as session2.turns_from samples them: bursts of a few to twenty
// minutes, a few to twenty apart, by the seed.
func turnsFrom(spans [][2]int, seed int) []turnPlan {
	names := [][3]string{{"Bash", "Edit", "Read"}, {"Read", "Edit", "Bash"}, {"Edit", "Bash", "Read"}, {"Read", "Grep", "Bash"}}
	next := func(k int) int { return (k*1103515245 + 12345) % (1 << 31) }
	var turns []turnPlan
	k := seed
	for _, span := range spans {
		for m := span[0]; m < span[1]-2; {
			k = next(k)
			minutes := min(2+k%17, span[1]-m)
			requests := max(3, minutes*(3+k%3))
			n := names[k%4]
			turns = append(turns, turnPlan{start: m, minutes: minutes, requests: requests,
				tools: []toolCount{{n[0], requests / 3}, {n[1], requests / 4}, {n[2], requests / 5}}})
			k = next(k)
			m += minutes + 3 + k%14
		}
	}
	return turns
}

// claudeCodeAgent is the user agent Claude Code's requests carry.
const claudeCodeAgent = "claude-cli/2.1.0 (external, cli)"

// lines are the session's lines, oldest first, each turn's requests spread
// over its minutes as the generator weighs them, and summing to what it
// samples the turn at: its context read back by each request, 3,000 written
// by each, and what a move wrote again by the request that made it, 900 out
// by each, and 3 in.
func (p sessionPlan) lines() []ledger.Line {
	var lines []ledger.Line
	routed := make(map[string]string)
	for i, t := range p.turns {
		for _, l := range p.turn(i+1, t) {
			l.Session, l.Dir, l.Kind, l.Status, l.Attempts, l.Agent = p.id, p.dir, ledger.KindMessage, 200, 1, claudeCodeAgent
			l.Request = cmp.Or(l.Request, requestID(p.id, len(lines)))
			l.Prompt = fmt.Sprintf("%s-prompt-%d", p.id[:8], i+1)
			p.route(&l, routed)
			lines = append(lines, l)
		}
	}
	if !p.first.IsZero() {
		lines[0].At = p.first.UTC()
	}
	return lines
}

// requestID is the router's id for the session's request numbered n.
func requestID(session string, n int) string {
	h := fnv.New32a()
	h.Write(fmt.Appendf(nil, "%s/%d", session, n))
	return fmt.Sprintf("%08x", h.Sum32())
}

// route says why the request l went where it did, as the router routes a
// session's models apart, routed holding the account each went to last: new,
// for its model's first; moved, where the session moved since its model's
// last; else sticky.
func (p sessionPlan) route(l *ledger.Line, routed map[string]string) {
	was, ok := routed[l.Model]
	switch {
	case !ok:
		l.Reason = status.ReasonNew
	case was != l.Account:
		l.From, l.Reason = was, p.moveAt(p.minuteOf(l.At)).reason
	default:
		l.Reason = status.ReasonSticky
	}
	routed[l.Model] = l.Account
}

// minuteOf is the minute of the session's life t falls in.
func (p sessionPlan) minuteOf(t time.Time) int {
	return int(t.Sub(p.start) / time.Minute)
}

// accountAt is the account the session ran on at t.
func (p sessionPlan) accountAt(t time.Time) string {
	minute := p.minuteOf(t)
	i := slices.IndexFunc(p.stays, func(s stay) bool { return s.from > minute })
	if i < 0 {
		i = len(p.stays)
	}
	return p.stays[i-1].account
}

// moveAt is the session's latest move at or before the minute of its life
// given.
func (p sessionPlan) moveAt(minute int) movePlan {
	var latest movePlan
	for _, m := range p.moves {
		if m.minute <= minute {
			latest = m
		}
	}
	return latest
}

// turn is the requests of the turn given, numbered n, oldest first: those
// drawn of it, then the rest spread over what's left of its minutes, or, of
// the turn still going, those before its request in flight. Each of the rest
// that calls a tool is of the session's own conversation and stopped to call
// it, as is its last, which ended the turn, or, of the turn still going,
// stopped to call a tool; and the others are side requests for the prompt,
// spread among them.
func (p sessionPlan) turn(n int, t turnPlan) []ledger.Line {
	drawn := p.drawn[n]
	turnStart := p.start.Add(time.Duration(t.start) * time.Minute)
	end := turnStart.Add(time.Duration(t.minutes) * time.Minute)
	going := !p.asked.IsZero() && n == len(p.turns)
	if going {
		end = p.asked
	}
	from := 0.0
	if len(drawn) > 0 {
		from = endOf(drawn[len(drawn)-1]).Sub(turnStart).Minutes()
	}
	at := spread(t.requests-len(drawn), from, end.Sub(turnStart).Minutes(), t.minutes)
	tools := toolsLeft(t.tools, drawn)
	before := len(at) - 1
	aside := before - len(tools)
	if going {
		aside++
	}
	lines := slices.Clone(drawn)
	for q, x := range at {
		when := turnStart.Add(time.Duration(x * float64(time.Minute)))
		l := ledger.Line{At: when.UTC().Truncate(time.Millisecond), Model: opus, Account: p.accountAt(when), Class: classMain}
		switch {
		case q == before && !going:
			l.Answer.Stop = "end_turn"
		case q < before && (q+1)*aside/before > q*aside/before:
			l.Class, l.Answer.Stop = classAuxiliary, "end_turn"
		default:
			l.Answer.Stop, l.Answer.Tools = "tool_use", []string{tools[0]}
			tools = tools[1:]
		}
		lines = append(lines, l)
	}
	timed(lines[len(drawn):], end)
	p.sample(lines, t, len(drawn))
	return lines
}

// toolsLeft are the calls of the tools given, those the requests drawn
// called taken off, one a request, the most called first, in turn.
func toolsLeft(tools []toolCount, drawn []ledger.Line) []string {
	left := slices.Clone(tools)
	for _, l := range drawn {
		for _, tool := range l.Answer.Tools {
			if i := slices.IndexFunc(left, func(c toolCount) bool { return c.tool == tool }); i >= 0 {
				left[i].times--
			}
		}
	}
	var calls []string
	for more := true; more; {
		more = false
		for i := range left {
			if left[i].times > 0 {
				calls, left[i].times, more = append(calls, left[i].tool), left[i].times-1, true
			}
		}
	}
	return calls
}

// spread is when n requests arrive, in a turn's minutes, from from to to, as
// the generator weighs the turn's minutes, a sine over them all: each
// request at the middle of its share of the weight.
func spread(n int, from, to float64, minutes int) []float64 {
	weight := func(i int) float64 { return math.Sin(math.Pi*(float64(i)+.5)/float64(minutes)) + .3 }
	cumulative := func(x float64) float64 {
		var c float64
		for i := range minutes {
			c += weight(i) * min(max(x-float64(i), 0), 1)
		}
		return c
	}
	low, high := cumulative(from), cumulative(to)
	at := make([]float64, n)
	for j := range n {
		target := low + (high-low)*(float64(j)+.5)/float64(n)
		i := int(from)
		for i+1 < minutes && cumulative(float64(i+1)) < target {
			i++
		}
		at[j] = float64(i) + (target-cumulative(float64(i)))/weight(i)
	}
	return at
}

// timed has each request take most of the time to the next, and the last
// most of the time to by, so each ends before the next comes, the last before
// by; and its answer's first byte come within its first second or so.
func timed(lines []ledger.Line, by time.Time) {
	for i := range lines {
		next := by
		if i+1 < len(lines) {
			next = lines[i+1].At
		}
		took := min(next.Sub(lines[i].At)*6/10, 18*time.Second).Milliseconds()
		first := min(took/4, 1200)
		lines[i].TotalMS, lines[i].FirstMS = took, &first
	}
}

// endOf is when the request l ended.
func endOf(l ledger.Line) time.Time {
	return l.At.Add(time.Duration(l.TotalMS) * time.Millisecond)
}

// sample gives the turn's requests, but the first drawn ones, what the
// generator samples the turn at. Each account the turn ran on takes the share
// of it its requests are of the turn's, as the generator shares a turn that
// moved, and the context the move wrote again; but what's drawn holds what it
// holds, and an account's share that only what's drawn ran on goes to the
// next. Within an account's share, each side request takes a little out,
// reading and writing nothing; the request that moved the session, the
// context it wrote again, reading nothing; and the rest of the turn's own
// conversation, the rest, alike.
func (p sessionPlan) sample(lines []ledger.Line, t turnPlan, drawn int) {
	turn := usageTokens{read: t.requests * sampleContext(float64(t.start)+float64(t.minutes)/2), written: t.requests * sampleWritten,
		out: t.requests * sampleOut}
	mover, again := p.mover(lines, t)
	var carried, given usageTokens
	runs := stretches(lines)
	for k, stretch := range runs {
		part := turn.part(len(stretch), t.requests)
		if k == len(runs)-1 {
			part = turn.less(given)
		}
		given = given.plus(part)
		left := part.plus(carried)
		var own, reading []int
		for _, i := range stretch {
			var u usageTokens
			switch {
			case i < drawn:
				u = tokensOf(lines[i])
				left.written += again * boolInt(i == mover)
			case lines[i].Class == classAuxiliary:
				u = usageTokens{input: sampleInput, out: 30 + 10*(i%7)}
			case i == mover:
				u = usageTokens{input: sampleInput, written: again}
				left.written, own = left.written+again, append(own, i)
			default:
				u = usageTokens{input: sampleInput}
				own, reading = append(own, i), append(reading, i)
			}
			left = left.less(u)
			lines[i].Usage = u.usage()
		}
		carried = usageTokens{}
		if len(reading) == 0 {
			carried = left
			continue
		}
		share(lines, reading, left.read, func(u *usageTokens, n int) { u.read += n })
		share(lines, reading, left.written, func(u *usageTokens, n int) { u.written += n })
		share(lines, own, left.out, func(u *usageTokens, n int) { u.out += n })
	}
}

// boolInt is 1 for true, else 0.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// stretches are the places of the turn's lines, a run of them for each time
// it was on an account, in order.
func stretches(lines []ledger.Line) [][]int {
	var runs [][]int
	for i, l := range lines {
		if i == 0 || l.Account != lines[i-1].Account {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], i)
	}
	return runs
}

// mover is the place among the turn's lines of the request of its own
// conversation that moved the session, the first in the minute of a move the
// turn holds or after it, and what the move wrote again: none where the turn
// holds no move.
func (p sessionPlan) mover(lines []ledger.Line, t turnPlan) (int, int) {
	for _, m := range p.moves {
		if m.minute < t.start || m.minute >= t.start+t.minutes {
			continue
		}
		at := slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Class == classMain && p.minuteOf(l.At) >= m.minute })
		return at, m.written
	}
	return -1, 0
}

// share shares n out among the lines at the places given, alike, the first
// taking what's left over, as add adds it to each one's usage.
func share(lines []ledger.Line, at []int, n int, add func(u *usageTokens, n int)) {
	if len(at) == 0 {
		return
	}
	each, over := n/len(at), n%len(at)
	for k, i := range at {
		u := tokensOf(lines[i])
		given := each
		if k < over {
			given++
		}
		add(&u, given)
		lines[i].Usage = u.usage()
	}
}

// usageTokens are the tokens of a request's usage: in, read back from the
// cache, written to it for an hour, and out.
type usageTokens struct {
	input, read, written, out int
}

// part is the share of u that n of total requests are.
func (u usageTokens) part(n, total int) usageTokens {
	return usageTokens{input: u.input * n / total, read: u.read * n / total, written: u.written * n / total, out: u.out * n / total}
}

// plus is u and v together.
func (u usageTokens) plus(v usageTokens) usageTokens {
	return usageTokens{input: u.input + v.input, read: u.read + v.read, written: u.written + v.written, out: u.out + v.out}
}

// less is u less v.
func (u usageTokens) less(v usageTokens) usageTokens {
	return usageTokens{input: u.input - v.input, read: u.read - v.read, written: u.written - v.written, out: u.out - v.out}
}

// tokensOf are the tokens l's usage counts.
func tokensOf(l ledger.Line) usageTokens {
	t, _ := l.Tokens()
	return usageTokens{input: t.Input, read: t.CacheRead, written: t.CacheWrite, out: t.Output}
}

// usage is the usage the API gives of the tokens, its writes to the cache
// lasting an hour, as Claude Code's on a subscription do.
func (u usageTokens) usage() json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"input_tokens": u.input, "cache_read_input_tokens": u.read, "cache_creation_input_tokens": u.written, "output_tokens": u.out,
		"cache_creation": map[string]int{"ephemeral_1h_input_tokens": u.written, "ephemeral_5m_input_tokens": 0},
	})
	return data
}
