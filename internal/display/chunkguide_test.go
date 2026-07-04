package display

import (
	"strings"
	"testing"
)

func linesOf(src string) (func(int) []byte, int) {
	lines := strings.Split(src, "\n")
	return func(i int) []byte { return []byte(lines[i]) }, len(lines)
}

func TestFindChunk(t *testing.T) {
	getLine, n := linesOf("func main() {\n\tif x {\n\t\ta()\n\n\t\tb()\n\t}\n}")

	for _, c := range []struct {
		cury, start, end, gcol int
		ok                     bool
	}{
		{2, 1, 5, 0, true},  // inside if block
		{3, 0, 0, 0, false}, // blank line draws nothing
		{1, 1, 5, 0, true},  // header line anchors the block it opens
		{0, 0, 5, 0, true},  // func chunk: corner retargets off the col-0 `}`
		{6, 0, 0, 0, false}, // closing brace opens nothing
	} {
		cg, ok := findChunk(getLine, n, c.cury, 8)
		if ok != c.ok || ok && (cg.start != c.start || cg.end != c.end || cg.gcol != c.gcol) {
			t.Errorf("findChunk(cury=%d) = %+v,%v, want %+v", c.cury, cg, ok, c)
		}
	}

	// missing lower boundary
	getLine, n = linesOf("if x {\n\ta()")
	if _, ok := findChunk(getLine, n, 1, 8); ok {
		t.Error("unclosed chunk reported")
	}

	// a dedent to column zero anchors the corner on the last code line
	getLine, n = linesOf("def f():\n\ta()\n\n\ndef g():")
	if cg, ok := findChunk(getLine, n, 1, 8); !ok || cg.end != 1 || cg.endIndent != 8 {
		t.Errorf("dedent to zero: end,endIndent,ok = %d,%d,%v, want 1,8,true", cg.end, cg.endIndent, ok)
	}

	// header whose block dedents to zero: corner lands on the body's
	// last line, not the next top-level statement
	getLine, n = linesOf("def f():\n\ttry:\n\t\ta()\n\n\ndef g():")
	if cg, ok := findChunk(getLine, n, 1, 8); !ok || cg.start != 1 || cg.end != 2 || cg.gcol != 0 {
		t.Errorf("header dedent: got %+v,%v, want start 1 end 2 gcol 0", cg, ok)
	}

	// boundaries beyond the scan cap
	huge := func(i int) []byte {
		if i == 0 || i == 2*chunkScanLimit+2 {
			return []byte("x")
		}
		return []byte("\tx")
	}
	if _, ok := findChunk(huge, 2*chunkScanLimit+3, chunkScanLimit+1, 8); ok {
		t.Error("chunk beyond scan limit reported")
	}
}

func TestChunkGeometry(t *testing.T) {
	// tabsize 4, space indent: guide on `if x:`(4) .. `return`(4), gcol 0
	getLine, n := linesOf("def f():\n    if x:\n        a()\n    return")
	cg, ok := findChunk(getLine, n, 2, 4)
	if !ok || cg.start != 1 || cg.end != 3 || cg.gcol != 0 {
		t.Fatalf("findChunk = %+v,%v, want start 1 end 3 gcol 0", cg, ok)
	}

	for _, c := range []struct {
		y, vcol int
		want    rune
	}{
		{1, 0, chunkCornerTop}, {1, 3, chunkHorizontal},
		{2, 0, chunkVertical},
		{3, 0, chunkCornerBot}, {3, 3, chunkArrow},
		{1, 4, 0}, {3, 4, 0}, {2, 1, 0}, {0, 0, 0}, // text cells and outside rows stay bare
	} {
		if got := cg.runeAt(c.y, c.vcol); got != c.want {
			t.Errorf("runeAt(%d,%d) = %q, want %q", c.y, c.vcol, got, c.want)
		}
	}
}
