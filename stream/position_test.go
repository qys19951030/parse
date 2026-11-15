package stream

import (
	"strings"
	"testing"

	"github.com/tdewolff/parse/v2"
)

// TestCrossFormatPositionConventions verifies acceptance point 2: the same
// byte content, tokenized through all four entries, carries document-relative
// offsets computed from the document start, and the offsets line up with the
// actual bytes regardless of format-specific token boundaries.
func TestCrossFormatPositionConventions(t *testing.T) {
	// each format's own well-formed snippet over the same byte alphabet
	snippets := map[Format]string{
		HTML: `<a b="c">x</a>`,
		XML:  `<a b="c">x</a>`,
		CSS:  `a{b:c}`,
		JS:   `a={b:c}`,
	}
	for format, doc := range snippets {
		ref := referenceLex(t, format, doc)
		for _, size := range []int{1, 2, 3, 7} {
			got := streamLex(t, format, doc, size)
			if len(got) != len(ref) {
				t.Fatalf("format %s size %d: token count %d != %d", format, size, len(got), len(ref))
			}
			for i := range ref {
				if got[i].start != ref[i].start || got[i].end != ref[i].end ||
					got[i].line != ref[i].line || got[i].col != ref[i].col {
					t.Fatalf("format %s size %d token %d position: got (%d-%d %d:%d) want (%d-%d %d:%d)",
						format, size, i,
						got[i].start, got[i].end, got[i].line, got[i].col,
						ref[i].start, ref[i].end, ref[i].line, ref[i].col)
				}
				// offset must index the real document bytes
				if got[i].end > len(doc) || doc[got[i].start:got[i].end] != ref[i].data {
					t.Fatalf("format %s token %d offset does not index document bytes", format, i)
				}
			}
		}
	}
}

// TestMultilinePositionsAcrossChunks checks line and column when every line
// break and comment is split by a chunk boundary.
func TestMultilinePositionsAcrossChunks(t *testing.T) {
	doc := "line1\r\nline2\nline3\rlast <!-- c\r\nc --> end"
	ref := referenceLex(t, HTML, doc)
	for _, size := range []int{1, 2, 4, 9} {
		got := streamLex(t, HTML, doc, size)
		if len(got) != len(ref) {
			t.Fatalf("size %d: token count %d != %d", size, len(got), len(ref))
		}
		for i := range ref {
			if got[i].line != ref[i].line || got[i].col != ref[i].col ||
				got[i].start != ref[i].start {
				t.Fatalf("size %d token %d: got %d:%d@%d want %d:%d@%d (%q)",
					size, i, got[i].line, got[i].col, got[i].start,
					ref[i].line, ref[i].col, ref[i].start, ref[i].data)
			}
		}
	}
}

// TestIdenticalContentFourEntries feeds the exact same byte string through
// all four format entries and checks that every token's offsets index the
// same document bytes, i.e. the position convention is format-independent.
func TestIdenticalContentFourEntries(t *testing.T) {
	// words, spaces and punctuation every tokenizer accepts
	doc := `ab cd 12 x-y`
	seen := 0
	for _, format := range []Format{HTML, XML, CSS, JS} {
		ref := referenceLex(t, format, doc)
		got := streamLex(t, format, doc, 1)
		if len(got) != len(ref) {
			t.Fatalf("format %s: count %d != %d", format, len(got), len(ref))
		}
		for i := range ref {
			if got[i].start != ref[i].start || got[i].end != ref[i].end {
				t.Fatalf("format %s token %d: offset %d-%d != %d-%d",
					format, i, got[i].start, got[i].end, ref[i].start, ref[i].end)
			}
			// the single source of truth: offsets index the shared document
			if doc[got[i].start:got[i].end] != got[i].data {
				t.Fatalf("format %s token %d: offsets do not index the shared bytes", format, i)
			}
		}
		// the first token of every format starts at document offset zero
		if len(got) > 0 && got[0].start != 0 {
			t.Fatalf("format %s: first token starts at %d, want 0", format, got[0].start)
		}
		seen++
	}
	if seen != 4 {
		t.Fatalf("expected all four formats, ran %d", seen)
	}
}

// TestHalfUnitAndCommentBoundaries combines acceptance point 2's two hard
// boundaries: a chunk ending inside a multi-byte unit and chunks cutting
// through a comment, both must keep locating and continuing.
func TestHalfUnitAndCommentBoundaries(t *testing.T) {
	doc := "<!-- é comment --><a>x</a>"
	ref := referenceLex(t, HTML, doc)
	// cut at every single byte, including inside the two-byte rune
	for cut := 1; cut < len(doc); cut++ {
		p := NewParser(HTML)
		if err := p.Feed([]byte(doc[:cut])); err != nil {
			t.Fatalf("Feed1: %v", err)
		}
		if err := p.Feed([]byte(doc[cut:])); err != nil {
			t.Fatalf("Feed2: %v", err)
		}
		if err := p.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		var got []flatToken
		for {
			tok, ok := p.Next()
			if !ok {
				break
			}
			got = append(got, flatToken{
				native: tok.Native, data: string(tok.Data),
				text: string(tok.Text), key: string(tok.AttrKey), val: string(tok.AttrVal),
				start: tok.Start, end: tok.End, line: tok.Line, col: tok.Col,
			})
		}
		if len(got) != len(ref) {
			t.Fatalf("cut %d: count %d != %d", cut, len(got), len(ref))
		}
		for i := range ref {
			if got[i] != ref[i] {
				t.Fatalf("cut %d token %d:\n got %+v\nwant %+v", cut, i, got[i], ref[i])
			}
		}
	}
}

// TestPositionMatchesParsePackage cross-checks the stream package's
// line/column against the standard parse.Position helper for several
// offsets, so both position conventions stay aligned.
func TestPositionMatchesParsePackage(t *testing.T) {
	doc := "ab\ncd\r\nef\rg"
	p := NewParser(HTML)
	if err := p.Feed([]byte(doc)); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	for {
		tok, ok := p.Next()
		if !ok {
			break
		}
		wantLine, wantCol, _ := parse.Position(strings.NewReader(doc), tok.Start)
		if tok.Line != wantLine || tok.Col != wantCol {
			t.Fatalf("offset %d: stream %d:%d, parse.Position %d:%d",
				tok.Start, tok.Line, tok.Col, wantLine, wantCol)
		}
	}
}
