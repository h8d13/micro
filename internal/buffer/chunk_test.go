package buffer

import (
	"strings"
	"testing"
)

func linesOf(src string) (func(int) []byte, int) {
	lines := strings.Split(src, "\n")
	return func(i int) []byte { return []byte(lines[i]) }, len(lines)
}

func TestFindIndentChunk(t *testing.T) {
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
		cg, ok := findIndentChunk(getLine, n, c.cury, 8)
		if ok != c.ok || ok && (cg.Start != c.start || cg.End != c.end || cg.GuideCol != c.gcol) {
			t.Errorf("findIndentChunk(cury=%d) = %+v,%v, want %+v", c.cury, cg, ok, c)
		}
	}

	// missing lower boundary
	getLine, n = linesOf("if x {\n\ta()")
	if _, ok := findIndentChunk(getLine, n, 1, 8); ok {
		t.Error("unclosed chunk reported")
	}

	// a dedent to column zero anchors the corner on the last code line
	getLine, n = linesOf("def f():\n\ta()\n\n\ndef g():")
	if cg, ok := findIndentChunk(getLine, n, 1, 8); !ok || cg.End != 1 || cg.EndIndent != 8 {
		t.Errorf("dedent to zero: end,endIndent,ok = %d,%d,%v, want 1,8,true", cg.End, cg.EndIndent, ok)
	}

	// header whose block dedents to zero: corner lands on the body's
	// last line, not the next top-level statement
	getLine, n = linesOf("def f():\n\ttry:\n\t\ta()\n\n\ndef g():")
	if cg, ok := findIndentChunk(getLine, n, 1, 8); !ok || cg.Start != 1 || cg.End != 2 || cg.GuideCol != 0 {
		t.Errorf("header dedent: got %+v,%v, want start 1 end 2 gcol 0", cg, ok)
	}

	// boundaries beyond the scan cap
	huge := func(i int) []byte {
		if i == 0 || i == 2*chunkScanLimit+2 {
			return []byte("x")
		}
		return []byte("\tx")
	}
	if _, ok := findIndentChunk(huge, 2*chunkScanLimit+3, chunkScanLimit+1, 8); ok {
		t.Error("chunk beyond scan limit reported")
	}
}

func TestFindBraceChunk(t *testing.T) {
	//	0: func main() {
	//	1: 	if x >
	//	2: 		0 {
	//	3: 		a(b)
	//	4: 	}
	//	5: 	c(
	//	6: 		d,
	//	7: 	)
	//	8: }
	getLine, n := linesOf("func main() {\n\tif x >\n\t\t0 {\n\t\ta(b)\n\t}\n\tc(\n\t\td,\n\t)\n}")

	for _, c := range []struct {
		cur              Loc
		start, end, gcol int
		ok               bool
	}{
		{Loc{2, 3}, 2, 4, 0, true}, // inside if body (multi-line condition)
		{Loc{0, 6}, 5, 7, 0, true}, // inside paren args
		{Loc{0, 4}, 2, 4, 0, true}, // on `}` before the brace: the block it closes
		{Loc{4, 2}, 2, 4, 0, true}, // header rule: `0 {` anchors the block it opens
		{Loc{3, 5}, 5, 7, 0, true}, // header rule: `c(` anchors the paren chunk
		// same-line pair `(b)` is consumed, enclosing if block wins;
		// col-0 `}` retargets nothing here (endIndent 8 > 0)
		{Loc{4, 3}, 2, 4, 0, true},
	} {
		cg, ok := findBraceChunk(getLine, n, c.cur, 8)
		if ok != c.ok || ok && (cg.Start != c.start || cg.End != c.end || cg.GuideCol != c.gcol) {
			t.Errorf("findBraceChunk(%v) = %+v,%v, want %+v", c.cur, cg, ok, c)
		}
	}

	// func chunk: bottom corner retargets off the col-0 `}` to the last
	// code line
	if cg, ok := findBraceChunk(getLine, n, Loc{0, 1}, 8); !ok || cg.Start != 0 || cg.End != 7 || cg.EndIndent != 8 {
		t.Errorf("func chunk: got %+v,%v, want start 0 end 7 endIndent 8", cg, ok)
	}

	// no enclosing pair at top level
	getLine, n = linesOf("x := 1\ny := 2")
	if _, ok := findBraceChunk(getLine, n, Loc{0, 1}, 8); ok {
		t.Error("chunk reported at top level")
	}

	// unclosed opener is not a chunk
	getLine, n = linesOf("if x {\n\ta(")
	if _, ok := findBraceChunk(getLine, n, Loc{3, 1}, 8); ok {
		t.Error("unclosed chunk reported")
	}

	// blank line inside a block still resolves (indent mode cannot)
	getLine, n = linesOf("if x {\n\ta()\n\n\tb()\n}")
	if cg, ok := findBraceChunk(getLine, n, Loc{0, 2}, 8); !ok || cg.Start != 0 || cg.End != 3 {
		t.Errorf("blank line: got %+v,%v, want start 0 end 3", cg, ok)
	}

	// boundaries beyond the scan cap
	huge := func(i int) []byte {
		if i == 0 {
			return []byte("f(")
		}
		if i == 2*chunkScanLimit+2 {
			return []byte(")")
		}
		return []byte("\tx,")
	}
	if _, ok := findBraceChunk(huge, 2*chunkScanLimit+3, Loc{0, chunkScanLimit + 1}, 8); ok {
		t.Error("chunk beyond scan limit reported")
	}
}
