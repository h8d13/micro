package display

// The hlchunk option highlights the indent chunk around the active cursor,
// in the spirit of hlchunk.nvim. A chunk is delimited by the nearest lines
// above and below the cursor with smaller visual indent (the opening
// statement and the closing token, or the next sibling statement); a
// cursor on a block-opening header anchors the chunk that header opens
// instead. The guide is drawn one indent level left of the boundary indent:
//
//	if x {     <- start row:   ╭──
//	    a()    <- middle row:  │
//	}          <- end row:     ╰─>
//
// Guide runes only ever replace whitespace cells, so text is never covered.
//
// A fresh chunk is animated: its cells appear one by one over
// chunkAnimDuration, sweeping from the opening line's text around to the
// closing line's text, like the original plugin.

import "time"

const (
	chunkCornerTop  = '╭'
	chunkCornerBot  = '╰'
	chunkVertical   = '│'
	chunkHorizontal = '─'
	chunkArrow      = '>'

	chunkAnimDuration = 200 * time.Millisecond
	chunkAnimFrame    = 16 * time.Millisecond

	// a new chunk stays hidden until the cursor settles on it, so
	// holding up/down does not fire a sweep at every step
	chunkSettleDelay = 200 * time.Millisecond

	// findChunk runs on every redraw (every 16ms while animating), so
	// boundary scans bail beyond this distance instead of walking a
	// huge uniformly-indented file end to end
	chunkScanLimit = 5000
)

type chunkGuide struct {
	start, end             int // boundary line numbers (corner rows)
	startIndent, endIndent int // visual indent width of the boundary lines
	gcol                   int // guide column, in visual columns
}

// visualIndent returns the display width of the line's leading whitespace
// and whether the line contains nothing else.
func visualIndent(line []byte, tabsize int) (int, bool) {
	w := 0
	for _, c := range line {
		switch c {
		case ' ':
			w++
		case '\t':
			w += tabsize - (w % tabsize)
		default:
			return w, false
		}
	}
	return w, true
}

// findChunk locates the indent chunk around line cury. It reports false
// when the cursor is at top level or a boundary is missing, or when a
// boundary lies further than chunkScanLimit lines away.
func findChunk(getLine func(int) []byte, nlines, cury, tabsize int) (chunkGuide, bool) {
	var cg chunkGuide

	ymin := cury - chunkScanLimit
	if ymin < 0 {
		ymin = 0
	}
	ymax := cury + chunkScanLimit
	if ymax > nlines-1 {
		ymax = nlines - 1
	}

	curIndent, blank := visualIndent(getLine(cury), tabsize)
	if blank {
		return cg, false
	}
	// a line opening a deeper block anchors the chunk it opens, not
	// the block enclosing it (what treesitter gives hlchunk.nvim):
	// the header is the top corner row and the chunk runs to the
	// first line back at the header's indent or less
	header := false
	for y := cury + 1; y <= ymax; y++ {
		if w, b := visualIndent(getLine(y), tabsize); !b {
			header = w > curIndent
			break
		}
	}
	if curIndent == 0 && !header {
		return cg, false
	}

	cg.start = -1
	cg.end = -1
	if header {
		cg.start, cg.startIndent = cury, curIndent
		for y := cury + 1; y <= ymax; y++ {
			if w, b := visualIndent(getLine(y), tabsize); !b && w <= curIndent {
				cg.end, cg.endIndent = y, w
				break
			}
		}
	} else {
		for y := cury - 1; y >= ymin; y-- {
			if w, b := visualIndent(getLine(y), tabsize); !b && w < curIndent {
				cg.start, cg.startIndent = y, w
				break
			}
		}
		for y := cury + 1; y <= ymax; y++ {
			if w, b := visualIndent(getLine(y), tabsize); !b && w < curIndent {
				cg.end, cg.endIndent = y, w
				break
			}
		}
	}
	if cg.start < 0 || cg.end < 0 {
		return cg, false
	}

	// a dedent straight to column zero has no whitespace to hold the
	// bottom corner, leaving the bars dangling, so anchor the corner
	// on the chunk's last code line instead (hlchunk's treesitter
	// ranges end there too: such blocks have no closing token)
	if cg.endIndent == 0 {
		for y := cg.end - 1; y > cg.start; y-- {
			if w, b := visualIndent(getLine(y), tabsize); !b {
				cg.end, cg.endIndent = y, w
				break
			}
		}
	}

	cg.gcol = cg.startIndent
	if cg.endIndent < cg.gcol {
		cg.gcol = cg.endIndent
	}
	cg.gcol -= tabsize
	if cg.gcol < 0 {
		cg.gcol = 0
	}

	// a column-zero opener likewise has no top corner: keep bars off
	// blank lines at the chunk's head (never past the cursor's line)
	if cg.startIndent == 0 {
		for cg.start+1 < cury {
			if _, b := visualIndent(getLine(cg.start+1), tabsize); !b {
				break
			}
			cg.start++
		}
	}
	return cg, true
}

// chunkAnim animates a guide by revealing its cells over chunkAnimDuration
type chunkAnim struct {
	shown chunkGuide
	start time.Time
}

// visible returns how many guide cells are revealed right now, and whether
// more frames are needed. A changed chunk restarts the clock, set in the
// future so the sweep only begins once the cursor has settled.
func (a *chunkAnim) visible(cg chunkGuide) (int, bool) {
	if cg != a.shown {
		a.shown = cg
		a.start = time.Now().Add(chunkSettleDelay)
	}
	elapsed := time.Since(a.start)
	if elapsed < 0 {
		return 0, true
	}
	total := cg.cells()
	if elapsed >= chunkAnimDuration {
		return total, false
	}
	return int(elapsed * time.Duration(total) / chunkAnimDuration), true
}

// cells returns the number of cells the guide occupies (animation steps)
func (cg *chunkGuide) cells() int {
	n := cg.end - cg.start - 1
	if cg.startIndent > cg.gcol {
		n += cg.startIndent - cg.gcol
	}
	if cg.endIndent > cg.gcol {
		n += cg.endIndent - cg.gcol
	}
	return n
}

// cellIndex returns the draw-order position of a cell the guide covers:
// the top corner row fills leftwards from the opening line's text, the
// bars run downwards, the bottom corner rightwards to the arrow.
func (cg *chunkGuide) cellIndex(y, vcol int) int {
	topLen := 0
	if cg.startIndent > cg.gcol {
		topLen = cg.startIndent - cg.gcol
	}
	switch {
	case y == cg.start:
		return cg.startIndent - 1 - vcol
	case y < cg.end:
		return topLen + y - cg.start - 1
	default:
		return topLen + cg.end - cg.start - 1 + vcol - cg.gcol
	}
}

// runeAt returns the guide rune for visual column vcol of line y, or 0 if
// the guide does not cover that cell. Corner rows are skipped when their
// boundary line has no leading whitespace to draw into.
func (cg *chunkGuide) runeAt(y, vcol int) rune {
	switch {
	case y > cg.start && y < cg.end:
		if vcol == cg.gcol {
			return chunkVertical
		}
	case y == cg.start && cg.startIndent > cg.gcol:
		switch {
		case vcol == cg.gcol:
			return chunkCornerTop
		case vcol > cg.gcol && vcol < cg.startIndent:
			return chunkHorizontal
		}
	case y == cg.end && cg.endIndent > cg.gcol:
		switch {
		case vcol == cg.gcol:
			return chunkCornerBot
		case vcol == cg.endIndent-1:
			return chunkArrow
		case vcol > cg.gcol && vcol < cg.endIndent:
			return chunkHorizontal
		}
	}
	return 0
}
