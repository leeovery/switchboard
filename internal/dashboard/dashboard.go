// Package dashboard draws the status document in the terminal: as a
// dashboard, full screen, as a watch keeps it, its title row, its heading,
// the view shown and its key line; or as usage prints every account's windows
// into the scrollback, once. Both are drawn glyph by glyph on a grid of
// cells, as the frames signed off for them were. Drawing is pure: from the
// document and the clock, and what the watch knows besides.
package dashboard

// margin is the blank cells before every line.
const margin = 1
