package display

import (
	"strings"
	"testing"
)

func linesOf(src string) (func(int) []byte, int) {
	lines := strings.Split(src, "\n")
	return func(i int) []byte { return []byte(lines[i]) }, len(lines)
}

func TestFindChunkBraces(t *testing.T) {
	getLine, n := linesOf(
		"func main() {\n" +
			"\tif x {\n" +
			"\t\ta()\n" +
			"\n" +
			"\t\tb()\n" +
			"\t}\n" +
			"}")

	// cursor on a() (indent 16): boundaries are `if x {` and `\t}`
	cg, ok := findChunk(getLine, n, 2, 8)
	if !ok {
		t.Fatal("expected chunk at line 2")
	}
	if cg.start != 1 || cg.end != 5 {
		t.Errorf("boundaries = %d,%d, want 1,5", cg.start, cg.end)
	}
	if cg.gcol != 0 {
		t.Errorf("gcol = %d, want 0 (min(8,8)-8)", cg.gcol)
	}

	// cursor on the blank line inside the block keeps the same chunk
	cg, ok = findChunk(getLine, n, 3, 8)
	if !ok || cg.start != 1 || cg.end != 5 {
		t.Errorf("blank line: ok=%v boundaries=%d,%d, want 1,5", ok, cg.start, cg.end)
	}

	// cursor on `if x {` (indent 8): chunk is the func body
	cg, ok = findChunk(getLine, n, 1, 8)
	if !ok || cg.start != 0 || cg.end != 6 || cg.gcol != 0 {
		t.Errorf("outer: ok=%v boundaries=%d,%d gcol=%d, want 0,6,0", ok, cg.start, cg.end, cg.gcol)
	}

	// cursor at top level: no chunk
	if _, ok = findChunk(getLine, n, 0, 8); ok {
		t.Error("top level should have no chunk")
	}
}

func TestFindChunkNoClosing(t *testing.T) {
	getLine, n := linesOf("if x {\n\ta()\n\tb()")
	if _, ok := findChunk(getLine, n, 1, 8); ok {
		t.Error("chunk without lower boundary should not be reported")
	}
}

func TestChunkRuneAt(t *testing.T) {
	// python-ish, tabsize 4, spaces:
	//   0 def f():
	//   1     if x:
	//   2         a()
	//   3     return
	getLine, n := linesOf("def f():\n    if x:\n        a()\n    return")
	cg, ok := findChunk(getLine, n, 2, 4)
	if !ok {
		t.Fatal("expected chunk")
	}
	// boundaries `if x:` (indent 4) and `return` (indent 4), gcol 0
	if cg.start != 1 || cg.end != 3 || cg.gcol != 0 {
		t.Fatalf("boundaries=%d,%d gcol=%d, want 1,3,0", cg.start, cg.end, cg.gcol)
	}

	expect := map[[2]int]rune{
		{1, 0}: chunkCornerTop, {1, 1}: chunkHorizontal, {1, 3}: chunkHorizontal,
		{2, 0}: chunkVertical,
		{3, 0}: chunkCornerBot, {3, 1}: chunkHorizontal, {3, 3}: chunkArrow,
	}
	for pos, want := range expect {
		if got := cg.runeAt(pos[0], pos[1]); got != want {
			t.Errorf("runeAt(%d,%d) = %q, want %q", pos[0], pos[1], got, want)
		}
	}
	// never outside the guide: on text columns or unrelated rows
	for _, pos := range [][2]int{{1, 4}, {3, 4}, {2, 1}, {0, 0}, {2, 4}} {
		if got := cg.runeAt(pos[0], pos[1]); got != 0 {
			t.Errorf("runeAt(%d,%d) = %q, want none", pos[0], pos[1], got)
		}
	}
}

func TestVisualIndent(t *testing.T) {
	for _, c := range []struct {
		line  string
		tab   int
		width int
		blank bool
	}{
		{"\tx", 8, 8, false},
		{"    x", 8, 4, false},
		{"  \ty", 8, 8, false}, // tab snaps to next stop
		{"", 8, 0, true},
		{" \t ", 8, 9, true},
		{"x", 8, 0, false},
	} {
		w, b := visualIndent([]byte(c.line), c.tab)
		if w != c.width || b != c.blank {
			t.Errorf("visualIndent(%q) = %d,%v, want %d,%v", c.line, w, b, c.width, c.blank)
		}
	}
}
