package stream

import (
	"io"
	"testing"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"github.com/tdewolff/parse/v2/html"
	"github.com/tdewolff/parse/v2/js"
	"github.com/tdewolff/parse/v2/xml"
)

// flatToken is the comparable view of one native lexer event.
type flatToken struct {
	native uint32
	data   string
	text   string
	key    string
	val    string
	start  int
	end    int
	line   int
	col    int
	kind   Kind
}

// referenceLex runs the native lexer over the whole document at once using
// exactly the Next/Text/AttrKey/AttrVal surface that the stream package uses.
func referenceLex(t *testing.T, format Format, doc string) []flatToken {
	in := parse.NewInputString(doc)
	var lex formatLexer
	switch format {
	case HTML:
		lex = htmlLexer{html.NewLexer(in)}
	case CSS:
		lex = cssLexer{css.NewLexer(in)}
	case JS:
		lex = jsLexer{js.NewLexer(in)}
	case XML:
		lex = xmlLexer{xml.NewLexer(in)}
	}
	var out []flatToken
	lines := newLineIndex()
	for {
		native, data := lex.next()
		if native == 0 {
			if err := lex.err(); err != io.EOF {
				t.Fatalf("reference lexer non-EOF error: %v", err)
			}
			break
		}
		start := in.Offset() - len(data)
		line, col := lines.position()
		lines.consume(data)
		out = append(out, flatToken{
			native: native,
			data:   string(data),
			text:   string(lex.text()),
			key:    string(lex.attrKey()),
			val:    string(lex.attrVal()),
			start:  start,
			end:    in.Offset(),
			line:   line,
			col:    col,
		})
	}
	return out
}

// streamLex feeds the document through the streaming parser in fixed-size
// chunks and collects every emitted token.
func streamLex(t *testing.T, format Format, doc string, chunkSize int) []flatToken {
	p := NewParser(format)
	docBytes := []byte(doc)
	for off := 0; off < len(docBytes); off += chunkSize {
		end := off + chunkSize
		if end > len(docBytes) {
			end = len(docBytes)
		}
		if err := p.Feed(docBytes[off:end]); err != nil {
			t.Fatalf("Feed: %v", err)
		}
	}
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	var out []flatToken
	for {
		tok, ok := p.Next()
		if !ok {
			break
		}
		out = append(out, flatToken{
			native: tok.Native,
			data:   string(tok.Data),
			text:   string(tok.Text),
			key:    string(tok.AttrKey),
			val:    string(tok.AttrVal),
			start:  tok.Start,
			end:    tok.End,
			line:   tok.Line,
			col:    tok.Col,
			kind:   tok.Kind,
		})
	}
	return out
}

// assertEquivalent compares the chunked stream output against the one-shot
// native lexer output token by token, including byte shapes and positions.
func assertEquivalent(t *testing.T, format Format, doc string, chunkSizes ...int) {
	t.Helper()
	ref := referenceLex(t, format, doc)
	for _, size := range chunkSizes {
		got := streamLex(t, format, doc, size)
		if len(got) != len(ref) {
			t.Fatalf("format %s chunk %d: token count got %d want %d", format, size, len(got), len(ref))
		}
		for i := range ref {
			g, w := got[i], ref[i]
			if g.native != w.native || g.data != w.data || g.text != w.text ||
				g.key != w.key || g.val != w.val ||
				g.start != w.start || g.end != w.end || g.line != w.line || g.col != w.col {
				t.Fatalf("format %s chunk %d token %d mismatch:\n got %+v\nwant %+v", format, size, i, g, w)
			}
		}
		// every emitted token must be contiguous over the decoded document
		prev := 0
		for _, tk := range got {
			if tk.start != prev {
				t.Fatalf("format %s chunk %d: non-contiguous token at offset %d, prev end %d", format, size, tk.start, prev)
			}
			prev = tk.end
		}
		if prev != len(doc) {
			t.Fatalf("format %s chunk %d: coverage %d of %d bytes", format, size, prev, len(doc))
		}
	}
}

var chunkSizes = []int{1, 2, 3, 5, 7, 16, 64, 1000}

func TestHTMLChunkEquivalence(t *testing.T) {
	docs := []string{
		``,
		`plain text`,
		`<a>text</a>`,
		`<div class="x" id='y' data-z=z><p>hello</p><br/></div>`,
		`<!-- a comment that spans several words --><a href="u">x</a>`,
		`<textarea>raw <content> kept</textarea><script>if(a<b){c();}</script>`,
		"<!doctype html>\n<html>\n<head><meta charset=utf-8></head>\n<body>\r\n<p>line1\r\nline2</p>\n</body></html>",
		`<a title="multi &#65; entity &amp; ref">t</a>`,
		`<svg:foreign><math><mrow>x</mrow></math></svg:foreign>`,
		`<a href="` + "long" + repeat("x", 100) + `">y</a>`,
	}
	for _, doc := range docs {
		assertEquivalent(t, HTML, doc, chunkSizes...)
	}
}

func TestXMLChunkEquivalence(t *testing.T) {
	docs := []string{
		`<?xml version="1.0" encoding="UTF-8"?><root a="1" b='2' c=3><child>text</child><empty/></root>`,
		`<!-- comment --><![CDATA[ cdata <data> ]]><!DOCTYPE note SYSTEM "n.dtd"><x/>`,
		`<root>` + repeat("<item n='v'>data</item>", 20) + `</root>`,
		`<a x="` + repeat("q", 120) + `"/>`,
	}
	for _, doc := range docs {
		assertEquivalent(t, XML, doc, chunkSizes...)
	}
}

func TestCSSChunkEquivalence(t *testing.T) {
	docs := []string{
		`a { color: red; }`,
		`@media screen and (min-width: 100px) { .x > .y:hover { content: "a string"; background: url(http://example.com/a.png); } }`,
		`/* a long comment that must be split *//* second */#id[data-x="v"]{margin:1.5em!important;--var:#abc;}`,
		`.a{width:calc(100% - 2rem);}`,
	}
	for _, doc := range docs {
		assertEquivalent(t, CSS, doc, chunkSizes...)
	}
}

func TestJSChunkEquivalence(t *testing.T) {
	docs := []string{
		`var x = 1;`,
		`function f(a, b) { return a + b; }`,
		"const s = \"a string with 'quotes'\"; const t = `template ${x} end`; const r = /ab+c/g;",
		`// line comment
/* block
 comment */ if (x === 1) { y(); } else { z(); }`,
		`let obj = {a: 1, "b": 2, [c]: 3}; arr.push(...items);`,
		repeat(`function longName(argumentOne, argumentTwo) { return argumentOne + argumentTwo; }`, 10),
	}
	for _, doc := range docs {
		assertEquivalent(t, JS, doc, chunkSizes...)
	}
}

// TestChunkBoundariesEverywhere checks every possible single cut position in
// a document, which is the exhaustive guarantee for mid-tag, mid-attribute,
// mid-string and mid-comment boundaries.
func TestChunkBoundariesEverywhere(t *testing.T) {
	docs := map[Format]string{
		HTML: `<div a="1" b='2'><!-- c --><p>txt</p></div>`,
		XML:  `<root a="1"><!-- c --><child>txt</child></root>`,
		CSS:  `/* c */ .a { content: "str"; color: #fff; }`,
		JS:   `var x = "str"; /* c */ function f() { return x; }`,
	}
	for format, doc := range docs {
		ref := referenceLex(t, format, doc)
		for cut := 1; cut < len(doc); cut++ {
			p := NewParser(format)
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
				t.Fatalf("format %s cut %d: token count got %d want %d", format, cut, len(got), len(ref))
			}
			for i := range ref {
				if got[i] != ref[i] {
					t.Fatalf("format %s cut %d token %d:\n got %+v\nwant %+v", format, cut, i, got[i], ref[i])
				}
			}
		}
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
