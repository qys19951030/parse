package stream

import "testing"

// kindsAt returns the malformation kinds found at the given offsets.
func kindsAt(ms []Malformation) map[MalformationKind]int {
	out := map[MalformationKind]int{}
	for _, m := range ms {
		out[m.Kind]++
	}
	return out
}

func parseAll(t *testing.T, format Format, doc string, chunk int) *Parser {
	t.Helper()
	p := NewParser(format)
	b := []byte(doc)
	for off := 0; off < len(b); off += chunk {
		end := off + chunk
		if end > len(b) {
			end = len(b)
		}
		if err := p.Feed(b[off:end]); err != nil {
			t.Fatalf("Feed: %v", err)
		}
	}
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	return p
}

func TestMalformedUnclosedTag(t *testing.T) {
	doc := `<root><a><b>text</b><c>tail`
	for _, chunk := range []int{1, 3, 100} {
		p := parseAll(t, XML, doc, chunk)
		kinds := kindsAt(p.Malformations())
		if kinds[MalUnclosedTag] < 2 {
			t.Fatalf("chunk %d: want >=2 unclosed tags, got %+v", chunk, p.Malformations())
		}
		// recovery must still cover every normal token of the document
		got := streamLex(t, XML, doc, chunk)
		ref := referenceLex(t, XML, doc)
		if len(got) != len(ref) {
			t.Fatalf("chunk %d: token coverage got %d want %d", chunk, len(got), len(ref))
		}
		for i := range ref {
			if got[i].native != ref[i].native || got[i].data != ref[i].data {
				t.Fatalf("chunk %d token %d: got %q want %q", chunk, i, got[i].data, ref[i].data)
			}
		}
	}
}

func TestMalformedMissingQuote(t *testing.T) {
	doc := `<root><a href="unterminated><b>after</b></a></root>`
	p := parseAll(t, HTML, doc, 2)
	found := false
	for _, m := range p.Malformations() {
		if m.Kind == MalMissingQuote {
			found = true
			if m.Offset < 0 || m.Line < 1 || m.Col < 1 {
				t.Fatalf("malformation position invalid: %+v", m)
			}
		}
	}
	if !found {
		t.Fatalf("missing quote not recorded: %+v", p.Malformations())
	}
	// the document after the bad attribute is still tokenized completely
	got := streamLex(t, HTML, doc, 2)
	ref := referenceLex(t, HTML, doc)
	if len(got) != len(ref) {
		t.Fatalf("token coverage got %d want %d", len(got), len(ref))
	}
}

func TestMalformedUnclosedComment(t *testing.T) {
	doc := `<a><!-- this comment never ends and keeps going</a><b>x</b>`
	for _, chunk := range []int{1, 5, 50} {
		p := parseAll(t, XML, doc, chunk)
		kinds := kindsAt(p.Malformations())
		if kinds[MalUnclosedComment] != 1 {
			t.Fatalf("chunk %d: want 1 unclosed comment, got %+v", chunk, p.Malformations())
		}
		got := streamLex(t, XML, doc, chunk)
		ref := referenceLex(t, XML, doc)
		if len(got) != len(ref) {
			t.Fatalf("chunk %d: token coverage got %d want %d", chunk, len(got), len(ref))
		}
	}
}

func TestMalformedBadCharRef(t *testing.T) {
	doc := `<a>good &amp; and bad &; plus unterminated &amp and &#xZZ;</a>`
	p := parseAll(t, HTML, doc, 3)
	var bad []Malformation
	for _, m := range p.Malformations() {
		if m.Kind == MalBadCharRef {
			bad = append(bad, m)
		}
	}
	if len(bad) < 3 {
		t.Fatalf("want >=3 bad character references, got %+v", p.Malformations())
	}
	// the good reference must not be reported
	for _, m := range bad {
		if doc[m.Offset:m.Offset+5] == "&amp;" {
			t.Fatalf("well-formed &amp; wrongly reported at %d", m.Offset)
		}
	}
	// positions are document-relative and ascending
	prev := -1
	for _, m := range bad {
		if m.Offset <= prev {
			t.Fatalf("malformation offsets not ascending: %+v", bad)
		}
		prev = m.Offset
	}
}

func TestMalformedTruncatedEncoding(t *testing.T) {
	// complete prefix, then a dangling two-byte UTF-8 lead at the very end
	prefix := []byte("<a>")
	p := NewParser(XML)
	if err := p.Feed(prefix); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if err := p.Feed([]byte{0xC3}); err != nil { // lead byte, no continuation
		t.Fatalf("Feed2: %v", err)
	}
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	var trunc *Malformation
	for i := range p.Malformations() {
		if p.Malformations()[i].Kind == MalTruncatedEncodingUnit {
			trunc = &p.Malformations()[i]
		}
	}
	if trunc == nil {
		t.Fatalf("truncated encoding unit missing: %+v", p.Malformations())
	}
	if trunc.Offset != 3 {
		t.Fatalf("truncation offset %d, want 3", trunc.Offset)
	}
}

func TestMalformedStatsAndDeterminism(t *testing.T) {
	doc := `<root><a bad="q><!-- c &amp &; <b>x</b>`
	run := func() []Malformation {
		p := parseAll(t, HTML, doc, 4)
		return p.Malformations()
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("malformation count not deterministic: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("malformation %d differs across parses: %+v vs %+v", i, a[i], b[i])
		}
	}
	p := parseAll(t, HTML, doc, 4)
	if p.Stats().Malformations != len(p.Malformations()) {
		t.Fatalf("stats malformation count %d != events %d", p.Stats().Malformations, len(p.Malformations()))
	}
	// offsets are sorted in document order overall
	prev := -1
	for _, m := range p.Malformations() {
		if m.Offset < prev {
			t.Fatalf("malformations not in document order: %+v", p.Malformations())
		}
		prev = m.Offset
	}
}

func TestWellFormedHasNoMalformations(t *testing.T) {
	docs := map[Format]string{
		HTML: `<!doctype html><html><head></head><body><a href="x">t &amp; t</a><!-- c --></body></html>`,
		XML:  `<?xml version="1.0"?><root a="1"><b>x</b><!-- c --></root>`,
		CSS:  `/* c */ .a { color: red; content: "s"; }`,
		JS:   `var x = 1; /* c */ function f() { return x; }`,
	}
	for format, doc := range docs {
		p := parseAll(t, format, doc, 3)
		if len(p.Malformations()) != 0 {
			t.Fatalf("format %s: well-formed doc reported malformations: %+v", format, p.Malformations())
		}
	}
}
