# A flaky test of a summary an import copies in

`TestASummaryOfADayTheMacNeverSawIsCopiedInWithItsStamp`, in `internal/router/import_test.go`,
failed once in a full gate under load, early in the pools build, in November 2026: `Import()`
copied no summary in (`Summaries: 0`), and the day was summarised from the lines it took in
instead of the summary staged beside them. It passed five runs of five alone. It was on main
before pools.

The day it stages is ten days back, so its lines are still kept and taken in with it. A guess,
not yet shown: under load, a round of the router's summarises the day from those lines before the
import comes to its summary, which then finds the Mac's own and stands aside, as
`TestTheMacsOwnSummaryStandsAgainstOneStaged` says it should.

Suggested: find what orders the import's copy against the router's summarising round, and either
hold the test to that order, by a seam it waits on rather than by time, or, where the import can
truly meet the round so, decide which should win and hold the import to it. A test fix alone where
the order is the test's; nothing a user sees changes.
