// Package dashboard draws the status document as a terminal dashboard: full
// screen, as a watch keeps it, its title row, its heading, the view shown and
// its footer; or printed once, as usage prints it, without the footer. A
// frame is drawn glyph by glyph on a grid of cells, as the frames signed off
// for it were. Drawing is pure: a frame is drawn from the document and the
// clock, and what the watch knows besides.
package dashboard

// margin is the blank cells before every line.
const margin = 1
