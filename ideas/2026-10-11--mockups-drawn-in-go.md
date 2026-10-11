# Mockups drawn in Go

The redesigned dashboard was drawn in Paper from a generator written in Python: a second model of
the dashboard, drawing each frame on a grid of terminal cells and writing it out as HTML for Paper,
and as each frame's text and colours, which became the references the build is held to. A second
model can drift from the Go it stands for.

Instead, a design's mockups could be drawn with the dashboard's own components: Lip Gloss's output,
parsed from its ANSI into the same grid of cells, written out as HTML for Paper. Once the
redesigned dashboard is built, `cmd/capturetool` draws its real screens from fakes, so a new
mockup can start from the screens as built and change only what's new.

The owner raised it on 11 October 2026, as an idea for another time.
