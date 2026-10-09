package cli

// RoleAnnotation is the annotation a command logs its role under, for tests
// that add a command of their own.
const RoleAnnotation = roleAnnotation

// ReadUnseen is how HiddenInput reads, given the terminal's part in it, for
// tests of how a read the user interrupts ends.
var ReadUnseen = readUnseen

// AskBackground is how TerminalBackground asks a terminal its background,
// given the terminal, for tests of how it asks, and what it hears.
var AskBackground = askBackground

// Took is how requests says how long a request took, for tests of each form
// it takes.
var Took = took

// WriteHistory is how history writes its days, for tests of days the ledger
// never gives it.
var WriteHistory = writeHistory

// Since is when --since's value starts, for tests of it in time zones of
// their own.
var Since = since

// WindowInProse is how events names a window by its key, for tests of each
// window Claude reports.
var WindowInProse = windowInProse
