package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// hardMoment is a document the router, or probing, builds as the clock
// starts, and a moment it's looked at after: one a view has got the document
// wrong at before.
type hardMoment struct {
	name string
	doc  status.Document
	now  time.Time
}

// hardMoments are status's hard moments, row for row: each document built as
// the router, or probing, builds its own, through AsOf, and the moment it's
// looked at.
func hardMoments() []hardMoment {
	side := account("side", "Side", session(0.1, 4*time.Hour), weekIn(0.2, 5*day))
	limited := builtBy(status.SourceRouter,
		limitedTo(account("work", "Work", refused(session(1, 40*time.Minute)), weekIn(0.6, 3*day)), 40*time.Minute, "5h"), side)
	capped := builtBy(status.SourceRouter, reserving(account("work", "Work", session(0.3, 3*time.Hour), weekIn(0.92, time.Hour)), 0.1), side)
	return []hardMoment{
		{
			name: "just after a reset, before the reread",
			doc: builtBy(status.SourceRouter,
				account("work", "Work", session(0.8, time.Hour), weekIn(0.5, 2*day)),
				account("side", "Side", session(0.3, 3*time.Hour), weekIn(0.7, 30*time.Minute))),
			now: start.Add(time.Hour + 5*time.Second),
		},
		{
			name: "a failed read after an earlier one",
			doc: builtBy(status.SourceRouter,
				failedAfter(account("work", "Work", session(0.7, 2*time.Hour), weekIn(0.4, 3*day)), 20*time.Minute), side),
			now: start.Add(5 * time.Minute),
		},
		{
			name: "its token refused",
			doc: builtBy(status.SourceRouter,
				tokenRefused(account("work", "Work", session(0.4, 2*time.Hour), weekIn(0.5, 2*day)), time.Hour), side),
			now: start.Add(10 * time.Minute),
		},
		{
			name: "a stale document, built three hours before now",
			doc: builtBy(status.SourceRouter,
				account("work", "Work", session(0.4, 2*time.Hour), weekIn(0.6, 4*day)),
				account("side", "Side", session(0.1, 4*time.Hour), weekIn(0.3, 5*day))),
			now: start.Add(3 * time.Hour),
		},
		{
			name: "its session lapsed",
			doc: builtBy(status.SourceRouter,
				readBefore(account("work", "Work", session(0.5, -time.Hour), weekIn(0.3, 3*day)), 6*time.Hour), side),
			now: start.Add(10 * time.Minute),
		},
		{name: "a week at its reserve when built", doc: capped, now: start.Add(10 * time.Minute)},
		{name: "a week at its reserve when built, then reset", doc: capped, now: start.Add(time.Hour + time.Minute)},
		{name: "a limit holding", doc: limited, now: start.Add(10 * time.Minute)},
		{name: "a limit holding, then lifted", doc: limited, now: start.Add(41 * time.Minute)},
		{
			name: "probed, of several accounts",
			doc: builtBy(status.SourceProbe,
				account("work", "Work", session(0.3, 3*time.Hour), weekIn(0.5, 2*day)),
				account("side", "Side", session(0.6, time.Hour), weekIn(0.05, 6*day))),
			now: start.Add(time.Minute),
		},
		{
			name: "probed, of one account, with no room",
			doc:  builtBy(status.SourceProbe, account("work", "Work", session(0.3, 3*time.Hour), refused(weekIn(1, 2*day)))),
			now:  start.Add(time.Minute),
		},
	}
}

// builtBy is the document source builds as the clock starts of the accounts
// given, each as it stands then, as AsOf has it.
func builtBy(source string, accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: start.UTC(), Source: source}
	if source == status.SourceRouter {
		doc.Router = status.Health{Healthy: true}
	}
	for _, a := range accounts {
		doc.Accounts = append(doc.Accounts, a.AsOf(policy, start))
	}
	return doc
}

// weekIn is the week, used as far as given, resetting resetsIn after the
// clock starts.
func weekIn(utilization float64, resetsIn time.Duration) quota.Window {
	return window("7d", "Week", utilization, resetsIn)
}

// limitedTo is a held back by the router's limit in the windows given, until
// lifts after the clock starts.
func limitedTo(a status.Account, lifts time.Duration, windows ...string) status.Account {
	a.Limit = status.Limit{ID: 4, Windows: windows, Until: start.Add(lifts).UTC()}
	return a
}

// tokenRefused is a whose token the upstream refuses, until lifts after the
// clock starts.
func tokenRefused(a status.Account, lifts time.Duration) status.Account {
	a.Refused = status.Refusal{Until: start.Add(lifts).UTC(), Status: 401}
	return a
}

// failedAfter is a as read ago before the clock starts, its last read since
// failed.
func failedAfter(a status.Account, ago time.Duration) status.Account {
	a = readBefore(a, ago)
	a.Error = "HTTP 529 · Overloaded"
	return a
}

// readBefore is a as read ago before the clock starts.
func readBefore(a status.Account, ago time.Duration) status.Account {
	a.FetchedAt = start.Add(-ago).UTC()
	return a
}
